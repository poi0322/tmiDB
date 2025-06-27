package database

import (
	"fmt"
	"log"
	"time"
)

// CategorySchema는 카테고리 스키마 테이블의 Go 표현입니다.
type CategorySchema struct {
	SchemaID         string    `json:"schema_id"`
	OrgID            string    `json:"org_id"`
	CategoryName     string    `json:"category_name"`
	Version          int       `json:"version"`
	SchemaDefinition string    `json:"schema_definition"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
}

// GetCategories는 특정 조직의 모든 카테고리를 조회합니다.
func GetCategories(orgID string) ([]CategorySchema, error) {
	rows, err := DB.Query("SELECT schema_id, org_id, category_name, version, is_active, created_at FROM category_schemas WHERE org_id = $1 ORDER BY category_name, version DESC", orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []CategorySchema
	for rows.Next() {
		var c CategorySchema
		if err := rows.Scan(&c.SchemaID, &c.OrgID, &c.CategoryName, &c.Version, &c.IsActive, &c.CreatedAt); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, nil
}

// CreateCategory는 새 카테고리 스키마를 생성합니다.
func CreateCategory(category *CategorySchema) error {
	// 새 버전은 항상 1로 시작
	category.Version = 1
	err := DB.QueryRow(
		`INSERT INTO category_schemas (org_id, category_name, version, schema_definition, is_active)
		 VALUES ($1, $2, $3, $4, TRUE)
		 RETURNING schema_id, created_at`,
		category.OrgID, category.CategoryName, category.Version, category.SchemaDefinition,
	).Scan(&category.SchemaID, &category.CreatedAt)
	return err
}

// UpdateCategory는 기존 카테고리 스키마의 새 버전을 생성합니다.
func UpdateCategory(category *CategorySchema) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 현재 최신 버전 조회
	var currentVersion int
	err = tx.QueryRow(
		"SELECT version FROM category_schemas WHERE org_id = $1 AND category_name = $2 ORDER BY version DESC LIMIT 1",
		category.OrgID, category.CategoryName,
	).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("could not find current version for category %s: %w", category.CategoryName, err)
	}

	// 새 버전 설정
	category.Version = currentVersion + 1

	// 새 버전 삽입
	err = tx.QueryRow(
		`INSERT INTO category_schemas (org_id, category_name, version, schema_definition, is_active)
		 VALUES ($1, $2, $3, $4, TRUE)
		 RETURNING schema_id, created_at`,
		category.OrgID, category.CategoryName, category.Version, category.SchemaDefinition,
	).Scan(&category.SchemaID, &category.CreatedAt)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteCategory는 특정 조직에서 카테고리를 삭제합니다.
func DeleteCategory(name, orgID string) error {
	// TODO: 해당 카테고리를 사용하는 타겟이 있는지 확인하는 로직 추가 필요
	_, err := DB.Exec("DELETE FROM category_schemas WHERE category_name = $1 AND org_id = $2", name, orgID)
	return err
}

// GetCategorySchema는 특정 조직의 카테고리 스키마(최신 버전)를 조회합니다.
func GetCategorySchema(name, orgID string) (*CategorySchema, error) {
	var c CategorySchema
	err := DB.QueryRow(
		`SELECT schema_id, org_id, category_name, version, schema_definition, is_active, created_at
		 FROM category_schemas 
		 WHERE org_id = $1 AND category_name = $2
		 ORDER BY version DESC LIMIT 1`,
		orgID, name,
	).Scan(&c.SchemaID, &c.OrgID, &c.CategoryName, &c.Version, &c.SchemaDefinition, &c.IsActive, &c.CreatedAt)

	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Listener는 리스너 테이블의 Go 표현입니다.
type Listener struct {
	ListenerID   string    `json:"listener_id"`
	OrgID        string    `json:"org_id"`
	CategoryName string    `json:"category_name"`
	Description  string    `json:"description"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// GetListeners는 특정 조직의 모든 리스너를 조회합니다.
func GetListeners(orgID string) ([]Listener, error) {
	rows, err := DB.Query("SELECT listener_id, org_id, category_name, description, is_active, created_at FROM listeners WHERE org_id = $1 ORDER BY created_at DESC", orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var listeners []Listener
	for rows.Next() {
		var l Listener
		if err := rows.Scan(&l.ListenerID, &l.OrgID, &l.CategoryName, &l.Description, &l.IsActive, &l.CreatedAt); err != nil {
			return nil, err
		}
		listeners = append(listeners, l)
	}
	return listeners, nil
}

// CreateListener는 새 리스너를 생성합니다.
func CreateListener(listener *Listener) error {
	_, err := DB.Exec(
		`INSERT INTO listeners (listener_id, org_id, category_name, description, is_active)
		 VALUES ($1, $2, $3, $4, TRUE)`,
		listener.ListenerID, listener.OrgID, listener.CategoryName, listener.Description,
	)
	return err
}

// DeleteListener는 특정 조직에서 리스너를 삭제합니다.
func DeleteListener(id, orgID string) error {
	_, err := DB.Exec("DELETE FROM listeners WHERE listener_id = $1 AND org_id = $2", id, orgID)
	return err
}

// 데이터베이스 스키마 초기화 SQL
const schemaSQL = `
-- tmiDB 스키마, 테이블, 초기 데이터 정의

-- 필요한 PostgreSQL 확장 활성화
CREATE EXTENSION IF NOT EXISTS "uuid-ossp"; -- UUID 생성을 위해
CREATE EXTENSION IF NOT EXISTS timescaledb; -- 시계열 데이터를 위해

----------------------------------------------------------------
-- 0. 조직 (Organization/Database)
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.organizations (
    org_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

----------------------------------------------------------------
-- 1. 카테고리 스키마 정의
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.category_schemas (
    schema_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id UUID NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    category_name TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    schema_definition JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, category_name, version)
);

----------------------------------------------------------------
-- 2. 대상 (Target)
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.target (
    target_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

----------------------------------------------------------------
-- 3. 대상-카테고리 매핑
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.target_categories (
    target_id UUID NOT NULL,
    org_id UUID NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    category_name TEXT NOT NULL,
    schema_version INTEGER NOT NULL DEFAULT 1,
    category_data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (target_id, category_name),
    CONSTRAINT fk_target
        FOREIGN KEY(target_id)
        REFERENCES public.target(target_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_category_schema
        FOREIGN KEY(org_id, category_name, schema_version)
        REFERENCES public.category_schemas(org_id, category_name, version)
);

----------------------------------------------------------------
-- 4. 시계열 관측 데이터 (TimescaleDB Hypertable)
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.ts_obs (
    target_id UUID NOT NULL,
    category_name TEXT NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    PRIMARY KEY (target_id, category_name, ts),
    CONSTRAINT fk_target_category
        FOREIGN KEY(target_id, category_name)
        REFERENCES public.target_categories(target_id, category_name)
        ON DELETE CASCADE
);

----------------------------------------------------------------
-- 5. 위치 추적 데이터 (간단한 좌표만)
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.geo_trace (
    target_id UUID NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    lon DOUBLE PRECISION NOT NULL,
    lat DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (target_id, ts),
    CONSTRAINT fk_target_geo
        FOREIGN KEY(target_id)
        REFERENCES public.target(target_id)
        ON DELETE CASCADE
);

----------------------------------------------------------------
-- 6. 원본 데이터 버킷
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.raw_bucket (
    raw_id BIGSERIAL PRIMARY KEY,
    ts TIMESTAMPTZ NOT NULL DEFAULT now(),
    source TEXT,
    payload JSONB
);

----------------------------------------------------------------
-- 7. 파일 첨부 관리
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.file_attachments (
    attachment_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    target_id UUID NOT NULL,
    filename TEXT NOT NULL,
    s3_path TEXT NOT NULL,
    size_bytes BIGINT,
    mime_type TEXT,
    uploaded_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_target_attachment
        FOREIGN KEY(target_id)
        REFERENCES public.target(target_id)
        ON DELETE CASCADE
);

----------------------------------------------------------------
-- 8. 트리거 함수
----------------------------------------------------------------
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

----------------------------------------------------------------
-- 9. 리스너 설정 테이블
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.listeners (
    listener_id TEXT PRIMARY KEY,
    category_name TEXT NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

----------------------------------------------------------------
-- 10. 인증 관련 테이블
----------------------------------------------------------------
-- 사용자 계정 테이블
CREATE TABLE IF NOT EXISTS public.users (
    user_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id UUID NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL, -- bcrypt 해시된 비밀번호
    role TEXT NOT NULL DEFAULT 'viewer', -- 'admin', 'editor', 'viewer'
    permissions JSONB NOT NULL DEFAULT '{"read": [], "write": []}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, username)
);

-- API 키와 권한을 관리하는 테이블
CREATE TABLE IF NOT EXISTS public.auth_tokens (
    token_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id UUID NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    encrypted_token TEXT NOT NULL UNIQUE, -- 암호화된 토큰 문자열
    description TEXT,
    permissions JSONB NOT NULL DEFAULT '{"read": [], "write": []}',
    is_admin BOOLEAN NOT NULL DEFAULT false,
    is_active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

----------------------------------------------------------------
-- 11. 시스템 설정 테이블
----------------------------------------------------------------
-- 시스템 초기 설정 상태 관리
CREATE TABLE IF NOT EXISTS public.system_config (
    config_key TEXT PRIMARY KEY,
    config_value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 사용자별 액세스 토큰 테이블
CREATE TABLE IF NOT EXISTS public.user_access_tokens (
    token_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    org_id UUID NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_user_token
        FOREIGN KEY(user_id)
        REFERENCES public.users(user_id)
        ON DELETE CASCADE
);
`

// 트리거 생성 SQL
const triggersSQL = `
-- 트리거 적용
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_timestamp_target') THEN
        CREATE TRIGGER set_timestamp_target
        BEFORE UPDATE ON public.target
        FOR EACH ROW
        EXECUTE PROCEDURE trigger_set_timestamp();
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_timestamp_target_categories') THEN
        CREATE TRIGGER set_timestamp_target_categories
        BEFORE UPDATE ON public.target_categories
        FOR EACH ROW
        EXECUTE PROCEDURE trigger_set_timestamp();
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_timestamp_users') THEN
        CREATE TRIGGER set_timestamp_users
        BEFORE UPDATE ON public.users
        FOR EACH ROW
        EXECUTE PROCEDURE trigger_set_timestamp();
    END IF;
END $$;
`

// TimescaleDB 하이퍼테이블 생성 SQL
const timescaleSQL = `
-- TimescaleDB 하이퍼테이블로 변환
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        IF NOT EXISTS (SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'ts_obs') THEN
            PERFORM create_hypertable('public.ts_obs', 'ts', if_not_exists => TRUE);
        END IF;
    END IF;
END $$;
`

// 기본 사용자 생성 함수
func CreateDefaultUsers() error {
	// 시스템 초기화 상태 확인
	var setupCompleted bool
	err := DB.QueryRow("SELECT EXISTS(SELECT 1 FROM system_config WHERE config_key = 'setup_completed')").Scan(&setupCompleted)
	if err != nil {
		return err
	}

	if !setupCompleted {
		// 초기 설정 시작 시간 기록
		_, err = DB.Exec(`
			INSERT INTO system_config (config_key, config_value) 
			VALUES ('setup_started_at', $1)
			ON CONFLICT (config_key) DO NOTHING
		`, time.Now().Format(time.RFC3339))
		if err != nil {
			return err
		}

		log.Println("System initialization required - no admin users will be created automatically")
		log.Println("Please complete setup through web console within 30 minutes")
		return nil
	}

	// 이미 설정이 완료된 경우, 기존 관리자 확인
	var adminExists bool
	err = DB.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE role = 'admin' AND is_active = true)").Scan(&adminExists)
	if err != nil {
		return err
	}

	if !adminExists {
		log.Println("Warning: No active admin users found but setup is marked as completed")
	}

	return nil
}

// CheckSetupTimeout은 설정 제한시간을 확인합니다
func CheckSetupTimeout() error {
	var startTimeStr string
	err := DB.QueryRow("SELECT config_value FROM system_config WHERE config_key = 'setup_started_at'").Scan(&startTimeStr)
	if err != nil {
		// setup_started_at가 없으면 이미 설정 완료된 것으로 간주
		return nil
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		return err
	}

	// 30분 경과 확인
	if time.Since(startTime) > 30*time.Minute {
		return fmt.Errorf("setup timeout exceeded - system is locked")
	}

	return nil
}

// SetSetupCompleted은 초기 설정을 완료합니다
func SetSetupCompleted() error {
	_, err := DB.Exec(`
		INSERT INTO system_config (config_key, config_value) 
		VALUES ('setup_completed', 'true')
		ON CONFLICT (config_key) DO UPDATE SET 
			config_value = EXCLUDED.config_value,
			updated_at = now()
	`)
	return err
}

// IsSetupCompleted는 초기 설정이 완료되었는지 확인합니다
func IsSetupCompleted() (bool, error) {
	var exists bool
	err := DB.QueryRow("SELECT EXISTS(SELECT 1 FROM system_config WHERE config_key = 'setup_completed')").Scan(&exists)
	return exists, err
}
