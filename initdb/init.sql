-- tmiDB 깨끗한 초기화 스크립트 (기본 PostgreSQL + TimescaleDB만 사용)

-- 필요한 PostgreSQL 확장 활성화
CREATE EXTENSION IF NOT EXISTS "uuid-ossp"; -- UUID 생성을 위해
CREATE EXTENSION IF NOT EXISTS timescaledb; -- 시계열 데이터를 위해

----------------------------------------------------------------
-- 1. 카테고리 스키마 정의
----------------------------------------------------------------
CREATE TABLE public.category_schemas (
    schema_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    category_name TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    schema_definition JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(category_name, version)
);

-- 기본 카테고리 스키마
INSERT INTO public.category_schemas (category_name, version, schema_definition) VALUES
('server', 1, '{
  "fields": {
    "hostname": {"type": "string", "required": true},
    "ip": {"type": "string", "required": true}, 
    "cpu_cores": {"type": "integer", "required": false},
    "memory_gb": {"type": "integer", "required": false},
    "status": {"type": "string", "enum": ["online", "offline", "maintenance"], "required": true}
  }
}'),
('sensor', 1, '{
  "fields": {
    "device_id": {"type": "string", "required": true},
    "location": {"type": "string", "required": true},
    "sensor_type": {"type": "string", "enum": ["temperature", "humidity", "pressure"], "required": true},
    "unit": {"type": "string", "required": true}
  }
}');

----------------------------------------------------------------
-- 2. 대상 (Target)
----------------------------------------------------------------
CREATE TABLE public.target (
    target_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

----------------------------------------------------------------
-- 3. 대상-카테고리 매핑
----------------------------------------------------------------
CREATE TABLE public.target_categories (
    target_id UUID NOT NULL,
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
        FOREIGN KEY(category_name, schema_version)
        REFERENCES public.category_schemas(category_name, version)
);

----------------------------------------------------------------
-- 4. 시계열 관측 데이터 (TimescaleDB Hypertable)
----------------------------------------------------------------
CREATE TABLE public.ts_obs (
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

-- TimescaleDB 하이퍼테이블로 변환 (기본 키에 ts가 포함되어야 함)
SELECT create_hypertable('public.ts_obs', 'ts');

----------------------------------------------------------------
-- 5. 위치 추적 데이터 (간단한 좌표만)
----------------------------------------------------------------
CREATE TABLE public.geo_trace (
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
CREATE TABLE public.raw_bucket (
    raw_id BIGSERIAL PRIMARY KEY,
    ts TIMESTAMPTZ NOT NULL DEFAULT now(),
    source TEXT,
    payload JSONB
);

----------------------------------------------------------------
-- 7. 파일 첨부 관리
----------------------------------------------------------------
CREATE TABLE public.file_attachments (
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

-- 트리거 적용
CREATE TRIGGER set_timestamp_target
BEFORE UPDATE ON public.target
FOR EACH ROW
EXECUTE PROCEDURE trigger_set_timestamp();

CREATE TRIGGER set_timestamp_target_categories
BEFORE UPDATE ON public.target_categories
FOR EACH ROW
EXECUTE PROCEDURE trigger_set_timestamp();

----------------------------------------------------------------
-- 9. 리스너 설정 테이블
----------------------------------------------------------------
CREATE TABLE public.listeners (
    listener_id TEXT PRIMARY KEY,
    category_name TEXT NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_listener_category
        FOREIGN KEY(category_name)
        REFERENCES public.category_schemas(category_name)
);

-- 기본 리스너 설정
INSERT INTO public.listeners (listener_id, category_name, description) VALUES
('server_monitor', 'server', 'Server monitoring listener'),
('sensor_alerts', 'sensor', 'Sensor alert listener');

----------------------------------------------------------------
-- 10. API 함수 (Go 프록시가 기대하는 시그니처)
----------------------------------------------------------------
CREATE OR REPLACE FUNCTION get_category_targets_advanced(
    p_category_name TEXT,
    p_version INTEGER DEFAULT 1,
    p_filters JSONB DEFAULT NULL
)
RETURNS TABLE(
    target_id UUID,
    target_name TEXT,
    category_data JSONB,
    updated_at TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        t.target_id,
        t.name,
        tc.category_data,
        tc.updated_at
    FROM public.target_categories tc
    JOIN public.target t ON tc.target_id = t.target_id
    WHERE tc.category_name = p_category_name
      AND tc.schema_version = p_version;
END;
$$ LANGUAGE plpgsql;

-- 리스너 데이터 조회 함수
CREATE OR REPLACE FUNCTION get_listener_filtered_data(
    p_listener_id TEXT,
    p_version TEXT DEFAULT 'v1',
    p_filters JSONB DEFAULT NULL
)
RETURNS TABLE(
    target_id UUID,
    target_name TEXT,
    category_data JSONB,
    updated_at TIMESTAMPTZ,
    category_name TEXT
) AS $$
DECLARE
    listener_category TEXT;
BEGIN
    -- 리스너의 카테고리 조회
    SELECT l.category_name INTO listener_category
    FROM public.listeners l
    WHERE l.listener_id = p_listener_id AND l.is_active = true;
    
    IF listener_category IS NULL THEN
        RAISE EXCEPTION 'Listener not found or inactive: %', p_listener_id;
    END IF;
    
    RETURN QUERY
    SELECT
        t.target_id,
        t.name,
        tc.category_data,
        tc.updated_at,
        tc.category_name
    FROM public.target_categories tc
    JOIN public.target t ON tc.target_id = t.target_id
    WHERE tc.category_name = listener_category
      AND tc.schema_version = 1;
END;
$$ LANGUAGE plpgsql;

-- 다중 리스너 데이터 조회 함수
CREATE OR REPLACE FUNCTION get_multi_listener_data(
    p_listener_ids TEXT[],
    p_version TEXT DEFAULT 'v1',
    p_filters JSONB DEFAULT NULL
)
RETURNS JSONB AS $$
DECLARE
    result JSONB := '{}';
    listener_id TEXT;
    listener_category TEXT;
    listener_data JSONB;
BEGIN
    FOREACH listener_id IN ARRAY p_listener_ids
    LOOP
        -- 각 리스너의 카테고리 및 데이터 조회
        SELECT l.category_name INTO listener_category
        FROM public.listeners l
        WHERE l.listener_id = listener_id AND l.is_active = true;
        
        IF listener_category IS NOT NULL THEN
            SELECT jsonb_agg(
                jsonb_build_object(
                    'target_id', t.target_id,
                    'target_name', t.name,
                    'category_data', tc.category_data,  
                    'updated_at', tc.updated_at
                )
            ) INTO listener_data
            FROM public.target_categories tc
            JOIN public.target t ON tc.target_id = t.target_id
            WHERE tc.category_name = listener_category
              AND tc.schema_version = 1;
              
            result := result || jsonb_build_object(listener_id, listener_data);
        END IF;
    END LOOP;
    
    RETURN result;
END;
$$ LANGUAGE plpgsql;

----------------------------------------------------------------
-- 11. 초기 테스트 데이터
----------------------------------------------------------------
-- 대상 생성
INSERT INTO public.target (name) VALUES 
('web-server-01'),
('web-server-02'),
('db-server-01'),
('temperature-sensor-01'),
('humidity-sensor-01');

-- 서버 데이터 
INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'server',
    1,
    '{"hostname": "web-server-01", "ip": "192.168.1.100", "cpu_cores": 8, "memory_gb": 32, "status": "online", "tags": ["web", "production"]}'::jsonb
FROM public.target t WHERE t.name = 'web-server-01';

INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'server',
    1,
    '{"hostname": "web-server-02", "ip": "192.168.1.101", "cpu_cores": 4, "memory_gb": 16, "status": "maintenance", "tags": ["web", "staging"]}'::jsonb
FROM public.target t WHERE t.name = 'web-server-02';

INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'server',
    1,
    '{"hostname": "db-server-01", "ip": "192.168.1.200", "cpu_cores": 16, "memory_gb": 64, "status": "online", "tags": ["database", "production"]}'::jsonb
FROM public.target t WHERE t.name = 'db-server-01';

-- 센서 데이터
INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'sensor',
    1,
    '{"device_id": "TEMP001", "location": "server-room", "sensor_type": "temperature", "unit": "celsius"}'::jsonb
FROM public.target t WHERE t.name = 'temperature-sensor-01';

INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'sensor',
    1,
    '{"device_id": "HUM001", "location": "server-room", "sensor_type": "humidity", "unit": "percent"}'::jsonb
FROM public.target t WHERE t.name = 'humidity-sensor-01';

-- 시계열 테스트 데이터
INSERT INTO public.ts_obs (target_id, category_name, ts, payload)
SELECT 
    t.target_id,
    'server',
    '2024-06-19 12:00:00+00'::timestamptz,
    '{"cpu_usage": 45.2, "memory_usage": 67.8, "disk_io": 1200}'::jsonb
FROM public.target t WHERE t.name = 'web-server-01';

INSERT INTO public.ts_obs (target_id, category_name, ts, payload)
SELECT 
    t.target_id,
    'sensor',
    '2024-06-19 12:00:00+00'::timestamptz,
    '{"temperature": 22.5, "humidity": 45.2}'::jsonb
FROM public.target t WHERE t.name = 'temperature-sensor-01';

-- 끝
