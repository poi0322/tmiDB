package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config는 애플리케이션의 모든 설정을 담는 구조체입니다.
type Config struct {
	// 데이터베이스 관련 설정
	DatabaseURL      string // tmiDB 전용 사용자 연결 DSN
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string // 초기 설정을 위한 Postgres 관리자 비밀번호
	PostgresDBName   string
	TmiDBUser        string
	TmiDBPassword    string

	// NATS 관련 설정
	NatsURL string

	// 기타
	IsProduction  bool
	EncryptionKey string
	// 필요에 따라 다른 설정 추가...
}

// Load는 환경 변수(.env 파일 포함)에서 설정을 로드합니다.
func Load() (*Config, error) {
	// .env 파일을 로드합니다. 파일이 없어도 오류가 발생하지 않습니다.
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := &Config{
		PostgresHost:     getEnv("DB_HOST", "localhost"),
		PostgresPort:     getEnv("DB_PORT", "5432"),
		PostgresUser:     getEnv("POSTGRES_USER", "postgres"),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", "postgres"),
		PostgresDBName:   getEnv("POSTGRES_DB", "tmidb"),
		TmiDBUser:        getEnv("TMIDB_USER", "tmidb_admin"),
		TmiDBPassword:    getEnv("TMIDB_PASSWORD", "tmidb_secure_2024!"), // 이 비밀번호는 안전하게 관리해야 합니다.
		NatsURL:          getEnv("NATS_URL", "nats://localhost:4222"),
		IsProduction:     getEnvAsBool("IS_PRODUCTION", false),
		EncryptionKey:    getEnv("ENCRYPTION_KEY", "e8e1694709a47355153cf11794252386a683d789a781b5399583643f82862e63"), // 32바이트 AES 키(64 hex chars)
	}

	cfg.DatabaseURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.TmiDBUser, cfg.TmiDBPassword, cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDBName)

	return cfg, nil
}

// getEnv는 환경 변수를 읽거나, 없을 경우 기본값을 반환합니다.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvAsBool는 환경 변수를 bool 값으로 읽습니다.
func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := getEnv(key, "")
	if valueStr == "true" || valueStr == "1" {
		return true
	}
	if valueStr == "false" || valueStr == "0" {
		return false
	}
	return defaultValue
}
