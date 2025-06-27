package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/tmidb/tmidb-core/internal/config" // config 패키지 임포트

	_ "github.com/lib/pq"
)

// 전역 DB 인스턴스
var DB *sql.DB

// InitDatabase는 데이터베이스 연결을 초기화합니다.
func InitDatabase(cfg *config.Config) error {
	// 1단계: 관리자 권한으로 연결하여 tmiDB 전용 사용자 및 데이터베이스 생성
	if err := setupDatabaseAndUser(cfg); err != nil {
		return fmt.Errorf("failed to setup database and user: %v", err)
	}

	// 2단계: tmiDB 전용 사용자로 연결
	if err := connectAsTmiDBUser(cfg); err != nil {
		return fmt.Errorf("failed to connect as tmiDB user: %v", err)
	}

	log.Println("Database connection completed successfully")
	return nil
}

// setupDatabaseAndUser는 관리자 권한으로 데이터베이스와 사용자를 생성합니다.
func setupDatabaseAndUser(cfg *config.Config) error {
	log.Printf("Connecting to PostgreSQL as admin user '%s' for initial setup", cfg.PostgresUser)

	// postgres 데이터베이스에 관리자로 연결
	adminDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable",
		cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort)

	adminDB, err := sql.Open("postgres", adminDBURL)
	if err != nil {
		return fmt.Errorf("failed to connect as admin: %v", err)
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping admin database: %v", err)
	}

	// tmiDB 데이터베이스 생성 (존재하지 않는 경우)
	_, err = adminDB.Exec(fmt.Sprintf(`
		CREATE DATABASE %s
		WITH ENCODING = 'UTF8'
		LC_COLLATE = 'en_US.utf8'
		LC_CTYPE = 'en_US.utf8'
		TEMPLATE = template0
	`, cfg.PostgresDBName))
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("failed to create database: %v", err)
	}

	// tmiDB 전용 사용자 생성
	_, err = adminDB.Exec(fmt.Sprintf(`
		DO $$ 
		BEGIN
			IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '%s') THEN
				CREATE USER %s WITH PASSWORD '%s';
			END IF;
		END $$
	`, cfg.TmiDBUser, cfg.TmiDBUser, cfg.TmiDBPassword))
	if err != nil {
		return fmt.Errorf("failed to create tmiDB user: %v", err)
	}

	// tmiDB 사용자에게 데이터베이스 권한 부여
	_, err = adminDB.Exec(fmt.Sprintf(`
		GRANT ALL PRIVILEGES ON DATABASE %s TO %s;
		ALTER USER %s CREATEDB;
	`, cfg.PostgresDBName, cfg.TmiDBUser, cfg.TmiDBUser))
	if err != nil {
		return fmt.Errorf("failed to grant database privileges: %v", err)
	}

	// tmiDB 데이터베이스에 연결하여 스키마 권한 부여
	tmidbDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDBName)

	tmidbDB, err := sql.Open("postgres", tmidbDBURL)
	if err != nil {
		return fmt.Errorf("failed to connect to tmidb database: %v", err)
	}
	defer tmidbDB.Close()

	// public 스키마 권한 부여
	_, err = tmidbDB.Exec(fmt.Sprintf(`
		GRANT ALL ON SCHEMA public TO %s;
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO %s;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO %s;
		ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO %s;
		ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO %s;
	`, cfg.TmiDBUser, cfg.TmiDBUser, cfg.TmiDBUser, cfg.TmiDBUser, cfg.TmiDBUser))
	if err != nil {
		return fmt.Errorf("failed to grant privileges: %v", err)
	}

	log.Printf("Database '%s' and user '%s' setup completed", cfg.PostgresDBName, cfg.TmiDBUser)
	return nil
}

// connectAsTmiDBUser는 tmiDB 전용 사용자로 연결합니다.
func connectAsTmiDBUser(cfg *config.Config) error {
	var err error
	DB, err = sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// 연결 테스트
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	// 연결 풀 설정
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)

	log.Printf("Connected to database as user '%s'", cfg.TmiDBUser)
	return nil
}

// initializeSchema는 데이터베이스 스키마를 초기화합니다.
func initializeSchema() error {
	log.Println("Initializing database schema...")

	// 1. 스키마 생성
	if _, err := DB.Exec(schemaSQL); err != nil {
		return fmt.Errorf("failed to create schema: %v", err)
	}

	// 2. 트리거 생성
	if _, err := DB.Exec(triggersSQL); err != nil {
		return fmt.Errorf("failed to create triggers: %v", err)
	}

	// 3. TimescaleDB 하이퍼테이블 생성
	if _, err := DB.Exec(timescaleSQL); err != nil {
		log.Printf("Warning: TimescaleDB setup failed (this is OK if TimescaleDB is not installed): %v", err)
	}

	// 4. 함수들 생성
	if _, err := DB.Exec(functionsSQL); err != nil {
		return fmt.Errorf("failed to create functions: %v", err)
	}

	// 5. 기본 사용자 생성
	if err := CreateDefaultUsers(); err != nil {
		return fmt.Errorf("failed to create default users: %v", err)
	}

	log.Println("Schema initialization completed successfully")
	return nil
}

// CloseDatabase는 데이터베이스 연결을 종료합니다.
func CloseDatabase() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// ExecuteFunction은 데이터베이스 함수를 실행하는 헬퍼 함수입니다.
func ExecuteFunction(functionName string, args ...interface{}) (*sql.Rows, error) {
	placeholders := make([]string, len(args))
	for i := range args {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	query := fmt.Sprintf("SELECT * FROM %s(%s)", functionName, strings.Join(placeholders, ", "))
	return DB.Query(query, args...)
}

// CheckDatabaseHealth는 데이터베이스 상태를 확인합니다.
func CheckDatabaseHealth() error {
	if DB == nil {
		return fmt.Errorf("database connection is nil")
	}

	return DB.Ping()
}

// Close는 데이터베이스 연결을 닫습니다
func Close() {
	CloseDatabase()
}

// InitializeSchema는 데이터베이스 스키마를 초기화합니다
func InitializeSchema() error {
	return initializeSchema()
}
