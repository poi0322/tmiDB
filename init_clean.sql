-- tmiDB 초기 스키마 (v0.1)
-- 이 파일은 Docker Compose가 처음 실행될 때 자동으로 적용됩니다.

-- 필요한 PostgreSQL 확장 활성화
CREATE EXTENSION IF NOT EXISTS "uuid-ossp"; -- UUID 생성을 위해
CREATE EXTENSION IF NOT EXISTS timescaledb; -- 시계열 데이터를 위해
CREATE EXTENSION IF NOT EXISTS cube;        -- 다차원 데이터 (위치) 를 위해
CREATE EXTENSION IF NOT EXISTS earthdistance; -- 위치 기반 거리 계산을 위해

----------------------------------------------------------------
-- 1. 카테고리 스키마 정의 (버전 관리)
-- 각 카테고리의 데이터 구조를 사전에 정의하고 버전을 관리합니다.
----------------------------------------------------------------
CREATE TABLE public.category_schemas (
    schema_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    category_name TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    schema_definition JSONB NOT NULL, -- 카테고리의 필드 정의 (타입, 필수 여부 등)
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(category_name, version)
);

CREATE INDEX idx_category_schemas_active ON public.category_schemas(category_name, version) WHERE is_active = true;

COMMENT ON TABLE public.category_schemas IS '카테고리별 데이터 스키마 정의 및 버전 관리';

-- 예시 카테고리 스키마들
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
-- 하나 이상의 카테고리에 속하는 정보 수집의 기본 단위입니다.
----------------------------------------------------------------
CREATE TABLE public.target (
    target_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE public.target IS '정보 수집의 기본 단위';

----------------------------------------------------------------
-- 3. 대상-카테고리 매핑
-- 각 대상이 어떤 카테고리에 속하는지, 그리고 해당 카테고리의 데이터를 저장합니다.
----------------------------------------------------------------
CREATE TABLE public.target_categories (
    target_id UUID NOT NULL,
    category_name TEXT NOT NULL,
    schema_version INTEGER NOT NULL DEFAULT 1,
    category_data JSONB NOT NULL, -- 해당 카테고리 스키마에 맞는 데이터
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

CREATE INDEX idx_target_categories_category ON public.target_categories(category_name);
CREATE INDEX idx_target_categories_data ON public.target_categories USING gin(category_data);

COMMENT ON TABLE public.target_categories IS '대상과 카테고리의 매핑 및 카테고리별 데이터';

----------------------------------------------------------------
-- 4. 시계열 관측 데이터 (TimescaleDB Hypertable)
-- 카테고리별 시계열 데이터를 저장합니다.
----------------------------------------------------------------
CREATE TABLE public.ts_obs (
    obs_id BIGSERIAL PRIMARY KEY,
    target_id UUID NOT NULL,
    category_name TEXT NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL, -- 해당 카테고리의 시계열 데이터
    CONSTRAINT fk_target_category
        FOREIGN KEY(target_id, category_name)
        REFERENCES public.target_categories(target_id, category_name)
        ON DELETE CASCADE
);

-- ts(시간)와 target_id(대상)를 기준으로 하이퍼테이블 생성
SELECT create_hypertable('public.ts_obs', 'ts',
  partitioning_column => 'target_id', number_partitions => 4);

-- 카테고리별 조회 성능을 위한 인덱스
CREATE INDEX idx_ts_obs_category ON public.ts_obs(category_name, ts DESC);
CREATE INDEX idx_ts_obs_target_category ON public.ts_obs(target_id, category_name, ts DESC);

COMMENT ON TABLE public.ts_obs IS '카테고리별 시계열 데이터를 저장하는 하이퍼테이블';

----------------------------------------------------------------
-- 5. 위치 추적 데이터
-- 대상의 지리적 위치 정보를 저장합니다.
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

-- GIST 인덱스를 사용하여 위치 기반 쿼리 성능을 최적화합니다.
CREATE INDEX idx_geo_trace_gist ON public.geo_trace USING gist (ll_to_earth(lat, lon));

COMMENT ON TABLE public.geo_trace IS '대상의 위치(좌표) 이력을 저장하는 테이블';

----------------------------------------------------------------
-- 6. 원본 데이터 버킷
-- 정제되지 않은 원본 데이터를 저장합니다.
----------------------------------------------------------------
CREATE TABLE public.raw_bucket (
    raw_id BIGSERIAL PRIMARY KEY,
    ts TIMESTAMPTZ NOT NULL DEFAULT now(),
    source TEXT, -- 데이터 출처 (e.g., 'pollingd', 'api_v1')
    payload JSONB
);

COMMENT ON TABLE public.raw_bucket IS '정제되지 않은 원본 데이터를 저장하기 위한 테이블';

----------------------------------------------------------------
-- 7. 리스너(Listener) 구독 관리 (개선된 다중 카테고리 지원)
-- 리스너별로 여러 카테고리를 구독하고, 각각 다른 버전과 필터를 적용할 수 있습니다.
----------------------------------------------------------------
CREATE TABLE public.listener_configs (
    config_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    listener_id TEXT NOT NULL, -- 리스너의 고유 식별자
    api_version TEXT NOT NULL DEFAULT 'v1', -- 이 리스너가 사용하는 API 버전
    category_name TEXT NOT NULL, -- 구독할 카테고리
    schema_version INTEGER NOT NULL DEFAULT 1, -- 사용할 스키마 버전
    base_filters JSONB, -- 리스너의 기본 필터 조건
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_category_config
        FOREIGN KEY(category_name, schema_version)
        REFERENCES public.category_schemas(category_name, version)
);

-- 리스너별 구독 정보 조회 최적화
CREATE INDEX idx_listener_configs_listener ON public.listener_configs(listener_id);
CREATE INDEX idx_listener_configs_category ON public.listener_configs(category_name);
CREATE INDEX idx_listener_configs_api_version ON public.listener_configs(api_version);

COMMENT ON TABLE public.listener_configs IS '리스너별 카테고리 구독 설정 (API 버전별 관리)';

----------------------------------------------------------------
-- 8. 파일 첨부 관리 테이블
-- 대상(Target)에 첨부된 파일의 메타데이터를 관리합니다.
----------------------------------------------------------------
CREATE TABLE public.file_attachments (
    attachment_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    target_id UUID NOT NULL,
    filename TEXT NOT NULL,
    s3_path TEXT NOT NULL,     -- SeaweedFS의 파일 경로
    size_bytes BIGINT,
    mime_type TEXT,
    uploaded_by TEXT,          -- 업로더 정보
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_target_attachment
        FOREIGN KEY(target_id)
        REFERENCES public.target(target_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_file_attachments_target ON public.file_attachments(target_id);
CREATE INDEX idx_file_attachments_created ON public.file_attachments(created_at);

COMMENT ON TABLE public.file_attachments IS '대상에 첨부된 파일의 메타데이터 관리';

----------------------------------------------------------------
-- 9. 자동 갱신 트리거 함수 및 적용
----------------------------------------------------------------
-- 데이터 변경 시 updated_at 컬럼을 자동으로 갱신하는 함수
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 각 테이블에 트리거 적용
CREATE TRIGGER set_timestamp_target
BEFORE UPDATE ON public.target
FOR EACH ROW
EXECUTE PROCEDURE trigger_set_timestamp();

CREATE TRIGGER set_timestamp_target_categories
BEFORE UPDATE ON public.target_categories
FOR EACH ROW
EXECUTE PROCEDURE trigger_set_timestamp();

CREATE TRIGGER set_timestamp_listener_configs
BEFORE UPDATE ON public.listener_configs
FOR EACH ROW
EXECUTE PROCEDURE trigger_set_timestamp();

----------------------------------------------------------------
-- 10. 데이터 보관 정책 (TTL - Time To Live)
----------------------------------------------------------------
-- 90일이 지난 원본 데이터는 자동 삭제
SELECT add_retention_policy('public.raw_bucket', INTERVAL '90 days');

-- 365일이 지난 시계열 데이터는 자동 삭제
SELECT add_retention_policy('public.ts_obs', INTERVAL '365 days');

----------------------------------------------------------------
-- 11. 카테고리 기반 API 뷰 및 함수
----------------------------------------------------------------

-- 카테고리별 최신 데이터를 조회하는 뷰
CREATE OR REPLACE VIEW public.category_latest_data AS
    SELECT
        tc.category_name,
        tc.schema_version,
        t.target_id,
        t.name as target_name,
        tc.category_data,
        tc.updated_at
    FROM public.target_categories tc
    JOIN public.target t ON tc.target_id = t.target_id;

-- 특정 카테고리의 스키마 정보를 반환하는 함수
CREATE OR REPLACE FUNCTION get_category_schema(
    p_category_name TEXT,
    p_version INTEGER DEFAULT NULL
)
RETURNS JSONB AS $$
DECLARE
    schema_result JSONB;
    target_version INTEGER;
BEGIN
    -- 버전이 지정되지 않았으면 최신 활성 버전 사용
    IF p_version IS NULL THEN
        SELECT version INTO target_version
        FROM public.category_schemas
        WHERE category_name = p_category_name AND is_active = true
        ORDER BY version DESC
        LIMIT 1;
    ELSE
        target_version := p_version;
    END IF;

    SELECT schema_definition INTO schema_result
    FROM public.category_schemas
    WHERE category_name = p_category_name AND version = target_version;

    RETURN COALESCE(schema_result, '{}'::jsonb);
END;
$$ LANGUAGE plpgsql;

-- 카테고리별 대상 목록을 조건으로 필터링하는 함수
CREATE OR REPLACE FUNCTION get_category_targets(
    p_category_name TEXT,
    p_version INTEGER DEFAULT 1,
    p_filter JSONB DEFAULT NULL
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
      AND tc.schema_version = p_version
      AND (p_filter IS NULL OR tc.category_data @> p_filter);
END;
$$ LANGUAGE plpgsql;

-- 고급 쿼리 연산자를 위한 필터 적용 함수
CREATE OR REPLACE FUNCTION apply_filter_condition(
    data_value JSONB,
    field_path TEXT,
    operator TEXT,
    compare_value TEXT
)
RETURNS BOOLEAN AS $$
DECLARE
    actual_value TEXT;
    actual_jsonb JSONB;
    numeric_actual NUMERIC;
    numeric_compare NUMERIC;
    in_values TEXT[];
    array_size INTEGER;
BEGIN
    -- JSON 경로에서 값 추출
    actual_value := data_value #>> string_to_array(field_path, '.');
    actual_jsonb := data_value #> string_to_array(field_path, '.');
    
    IF actual_value IS NULL AND actual_jsonb IS NULL THEN
        RETURN FALSE;
    END IF;
    
    CASE operator
        WHEN '=' THEN
            RETURN actual_value = compare_value;
        WHEN '!=' THEN
            RETURN actual_value != compare_value;
        WHEN '~' THEN
            RETURN actual_value ILIKE '%' || compare_value || '%';
        WHEN '!~' THEN
            RETURN NOT (actual_value ILIKE '%' || compare_value || '%');
        WHEN 'like' THEN
            RETURN actual_value ILIKE compare_value;
        WHEN '!like' THEN
            RETURN NOT (actual_value ILIKE compare_value);
        WHEN 'regex' THEN
            RETURN actual_value ~ compare_value;
        WHEN '!regex' THEN
            RETURN NOT (actual_value ~ compare_value);
        WHEN 'in' THEN
            -- IN 연산자: "Ready,Standby,Online" 형태를 배열로 분리
            in_values := string_to_array(compare_value, ',');
            RETURN actual_value = ANY(in_values);
        WHEN '!in' THEN
            -- NOT IN 연산자
            in_values := string_to_array(compare_value, ',');
            RETURN NOT (actual_value = ANY(in_values));
        WHEN '>', '>=', '<', '<=' THEN
            -- 숫자 비교
            BEGIN
                numeric_actual := actual_value::NUMERIC;
                numeric_compare := compare_value::NUMERIC;
                CASE operator
                    WHEN '>' THEN RETURN numeric_actual > numeric_compare;
                    WHEN '>=' THEN RETURN numeric_actual >= numeric_compare;
                    WHEN '<' THEN RETURN numeric_actual < numeric_compare;
                    WHEN '<=' THEN RETURN numeric_actual <= numeric_compare;
                END CASE;
            EXCEPTION WHEN OTHERS THEN
                RETURN FALSE;
            END;
        WHEN 'contains' THEN
            -- 배열 포함 검색 (JSON 배열에서 값이 포함되어 있는지 확인)
            RETURN actual_jsonb ? compare_value;
        WHEN '!contains' THEN
            RETURN NOT (actual_jsonb ? compare_value);
        WHEN 'array_includes' THEN
            -- 배열에 특정 값이 포함되어 있는지 확인
            IF jsonb_typeof(actual_jsonb) = 'array' THEN
                RETURN actual_jsonb ? compare_value;
            END IF;
            RETURN FALSE;
        WHEN '!array_includes' THEN
            -- 배열에 특정 값이 포함되어 있지 않은지 확인
            IF jsonb_typeof(actual_jsonb) = 'array' THEN
                RETURN NOT (actual_jsonb ? compare_value);
            END IF;
            RETURN TRUE;
        WHEN 'size' THEN
            -- 배열이나 객체의 크기 비교
            IF jsonb_typeof(actual_jsonb) = 'array' THEN
                array_size := jsonb_array_length(actual_jsonb);
                RETURN array_size = compare_value::INTEGER;
            ELSIF jsonb_typeof(actual_jsonb) = 'object' THEN
                RETURN (SELECT count(*) FROM jsonb_object_keys(actual_jsonb)) = compare_value::INTEGER;
            END IF;
            RETURN FALSE;
        WHEN 'size>', 'size>=', 'size<', 'size<=' THEN
            -- 배열이나 객체의 크기 비교 (부등호)
            IF jsonb_typeof(actual_jsonb) = 'array' THEN
                array_size := jsonb_array_length(actual_jsonb);
                CASE operator
                    WHEN 'size>' THEN RETURN array_size > compare_value::INTEGER;
                    WHEN 'size>=' THEN RETURN array_size >= compare_value::INTEGER;
                    WHEN 'size<' THEN RETURN array_size < compare_value::INTEGER;
                    WHEN 'size<=' THEN RETURN array_size <= compare_value::INTEGER;
                END CASE;
            END IF;
            RETURN FALSE;
        WHEN 'exists' THEN
            -- 필드의 존재 여부 확인
            RETURN actual_value IS NOT NULL OR actual_jsonb IS NOT NULL;
        WHEN '!exists' THEN
            -- 필드의 부재 확인
            RETURN actual_value IS NULL AND actual_jsonb IS NULL;
        WHEN 'empty' THEN
            -- 값이 비어있는지 확인 (빈 문자열, 빈 배열, 빈 객체, null)
            IF actual_value IS NULL AND actual_jsonb IS NULL THEN
                RETURN TRUE;
            ELSIF actual_value = '' THEN
                RETURN TRUE;
            ELSIF jsonb_typeof(actual_jsonb) = 'array' AND jsonb_array_length(actual_jsonb) = 0 THEN
                RETURN TRUE;
            ELSIF jsonb_typeof(actual_jsonb) = 'object' AND actual_jsonb = '{}'::jsonb THEN
                RETURN TRUE;
            END IF;
            RETURN FALSE;
        WHEN '!empty' THEN
            -- 값이 비어있지 않은지 확인
            IF actual_value IS NULL AND actual_jsonb IS NULL THEN
                RETURN FALSE;
            ELSIF actual_value = '' THEN
                RETURN FALSE;
            ELSIF jsonb_typeof(actual_jsonb) = 'array' AND jsonb_array_length(actual_jsonb) = 0 THEN
                RETURN FALSE;
            ELSIF jsonb_typeof(actual_jsonb) = 'object' AND actual_jsonb = '{}'::jsonb THEN
                RETURN FALSE;
            END IF;
            RETURN TRUE;
        ELSE
            RETURN FALSE;
    END CASE;
END;
$$ LANGUAGE plpgsql;

-- 개선된 카테고리 대상 조회 함수 (고급 필터 지원)
CREATE OR REPLACE FUNCTION get_category_targets_advanced(
    p_category_name TEXT,
    p_version INTEGER DEFAULT 1,
    p_filters JSONB DEFAULT NULL  -- [{"field": "temperature", "op": ">", "value": "25"}] 형태
)
RETURNS TABLE(
    target_id UUID,
    target_name TEXT,
    category_data JSONB,
    updated_at TIMESTAMPTZ
) AS $$
DECLARE
    filter_item JSONB;
    field_name TEXT;
    operator TEXT;
    compare_value TEXT;
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
      AND tc.schema_version = p_version
      AND (
        p_filters IS NULL OR 
        (
          SELECT bool_and(
            apply_filter_condition(tc.category_data, filter_item->>'field', filter_item->>'op', filter_item->>'value')
          )
          FROM jsonb_array_elements(p_filters) AS filter_item
        )
      );
END;
$$ LANGUAGE plpgsql;

-- 다중 리스너 조회를 위한 함수
CREATE OR REPLACE FUNCTION get_multi_listener_data(
    p_listener_ids TEXT[], -- 조회할 리스너 ID 배열
    p_api_version TEXT DEFAULT 'v1', -- 사용할 API 버전
    p_additional_filters JSONB DEFAULT NULL -- 추가 필터 (리스너별로 다른 조건)
)
RETURNS JSONB AS $$
DECLARE
    result JSONB := '{}';
    listener_id TEXT;
    listener_data JSONB;
BEGIN
    -- 각 리스너별로 데이터 조회
    FOREACH listener_id IN ARRAY p_listener_ids
    LOOP
        SELECT jsonb_agg(
            jsonb_build_object(
                'target_id', target_id,
                'target_name', target_name,
                'category_data', category_data,
                'updated_at', updated_at
            )
        ) INTO listener_data
        FROM get_listener_filtered_data(listener_id, p_api_version, p_additional_filters);
        
        result := result || jsonb_build_object(listener_id, COALESCE(listener_data, '[]'::jsonb));
    END LOOP;
    
    RETURN result;
END;
$$ LANGUAGE plpgsql;

-- 단일 리스너의 필터링된 데이터를 조회하는 함수
CREATE OR REPLACE FUNCTION get_listener_filtered_data(
    p_listener_id TEXT,
    p_api_version TEXT DEFAULT 'v1',
    p_additional_filters JSONB DEFAULT NULL
)
RETURNS TABLE(
    target_id UUID,
    target_name TEXT,
    category_data JSONB,
    updated_at TIMESTAMPTZ
) AS $$
DECLARE
    listener_config RECORD;
    combined_filters JSONB;
    listener_prefix TEXT;
    additional_filter JSONB;
    field_name TEXT;
    field_has_listener_prefix BOOLEAN;
BEGIN
    -- 리스너 설정 조회 (API 버전 고려)
    SELECT category_name, schema_version, COALESCE(base_filters, '[]'::jsonb) as base_filters
    INTO listener_config
    FROM public.listener_configs
    WHERE listener_id = p_listener_id AND api_version = p_api_version;
    
    IF NOT FOUND THEN
        RETURN;
    END IF;
    
    -- 기본 필터로 시작
    combined_filters := listener_config.base_filters;
    
    -- 추가 필터가 있다면 리스너별로 필터링
    IF p_additional_filters IS NOT NULL THEN
        listener_prefix := p_listener_id || '.';
        
        -- p_additional_filters에서 해당 리스너에 해당하는 필터 처리
        FOR additional_filter IN SELECT value FROM jsonb_array_elements(p_additional_filters)
        LOOP
            field_name := additional_filter->>'field';
            field_has_listener_prefix := (field_name LIKE listener_prefix || '%');
            
            -- 필터 조건 확인
            IF field_has_listener_prefix THEN
                -- "listener_id." 접두사 제거
                additional_filter := jsonb_set(
                    additional_filter,
                    '{field}',
                    to_jsonb(substring(field_name from length(listener_prefix) + 1))
                );
                combined_filters := combined_filters || jsonb_build_array(additional_filter);
            ELSIF position('.' in field_name) = 0 THEN
                -- 점이 없는 필드는 전역 필터로 적용
                combined_filters := combined_filters || jsonb_build_array(additional_filter);
            END IF;
            -- 다른 리스너 접두사를 가진 필드는 무시
        END LOOP;
    END IF;
    
    -- 최종 데이터 조회
    RETURN QUERY
    SELECT t.target_id, t.name, tc.category_data, tc.updated_at
    FROM public.target_categories tc
    JOIN public.target t ON tc.target_id = t.target_id
    WHERE tc.category_name = listener_config.category_name
      AND tc.schema_version = listener_config.schema_version
      AND (
        combined_filters = '[]'::jsonb OR
        (
          SELECT bool_and(
            apply_filter_condition(
              tc.category_data,
              (filter_condition->>'field')::TEXT,
              (filter_condition->>'op')::TEXT,
              (filter_condition->>'value')::TEXT
            )
          )
          FROM jsonb_array_elements(combined_filters) AS filter_condition
        )
      );
END;
$$ LANGUAGE plpgsql;

----------------------------------------------------------------
-- 12. Supabase Realtime을 위한 설정
----------------------------------------------------------------
-- Supabase Realtime이 변경사항을 감지할 수 있도록 publication을 생성합니다.
CREATE PUBLICATION supabase_realtime FOR ALL TABLES;

----------------------------------------------------------------
-- 13. 초기 테스트 데이터
----------------------------------------------------------------

-- 대상 데이터
INSERT INTO public.target (name) VALUES 
('web-server-01'),
('web-server-02'),
('db-server-01'),
('temperature-sensor-01'),
('humidity-sensor-01'),
('air-quality-sensor-01'),
('mobile-sensor-01');

-- 서버 데이터 
INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'server',
    1,
    '{"hostname": "web-server-01", "ip": "192.168.1.100", "cpu_cores": 8, "memory_gb": 32, "status": "online", "tags": ["web", "production"], "register_at": 1750316640}'::jsonb
FROM public.target t WHERE t.name = 'web-server-01';

INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'server',
    1,
    '{"hostname": "web-server-02", "ip": "192.168.1.101", "cpu_cores": 4, "memory_gb": 16, "status": "maintenance", "tags": ["web", "staging"], "register_at": 1750316700}'::jsonb
FROM public.target t WHERE t.name = 'web-server-02';

INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'server',
    1,
    '{"hostname": "db-server-01", "ip": "192.168.1.200", "cpu_cores": 16, "memory_gb": 64, "status": "online", "tags": ["database", "production"], "register_at": 1750316800}'::jsonb
FROM public.target t WHERE t.name = 'db-server-01';

-- 센서 데이터
INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'sensor',
    1,
    '{"device_id": "TEMP001", "location": "server-room", "sensor_type": "temperature", "unit": "celsius", "register_at": 1750336640}'::jsonb
FROM public.target t WHERE t.name = 'temperature-sensor-01';

INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'sensor',
    1,
    '{"device_id": "HUM001", "location": "server-room", "sensor_type": "humidity", "unit": "percent", "register_at": 1750336700}'::jsonb
FROM public.target t WHERE t.name = 'humidity-sensor-01';

INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'sensor',
    1,
    '{"device_id": "AIR001", "location": "office", "sensor_type": "air_quality", "unit": "ppm", "register_at": 1750336760}'::jsonb
FROM public.target t WHERE t.name = 'air-quality-sensor-01';

INSERT INTO public.target_categories (target_id, category_name, schema_version, category_data)
SELECT 
    t.target_id,
    'sensor',
    1,
    '{"device_id": "MOB001", "location": "mobile-unit-01", "sensor_type": "temperature", "unit": "celsius", "register_at": 1750336800}'::jsonb
FROM public.target t WHERE t.name = 'mobile-sensor-01';

-- 리스너 설정 예시 데이터
INSERT INTO public.listener_configs (listener_id, api_version, category_name, schema_version, base_filters) VALUES
('server_monitor', 'v1', 'server', 1, '[
  {"field": "status", "op": "in", "value": "online,maintenance"}
]'),
('sensor_monitor', 'v1', 'sensor', 1, '[
  {"field": "sensor_type", "op": "!=", "value": "offline"}
]'),
('admin_all_servers', 'v1', 'server', 1, '[]'),
('mobile_sensors', 'v1', 'sensor', 1, '[
  {"field": "location", "op": "~", "value": "mobile"}
]') 
ON CONFLICT DO NOTHING;

-- 시계열 테스트 데이터 추가
INSERT INTO public.ts_obs (target_id, category_name, ts, payload)
SELECT 
    t.target_id,
    'server',
    '2024-06-19 12:00:00+00'::timestamptz,
    '{"cpu_usage": 45.2, "memory_usage": 67.8, "disk_io": 1200, "created_at": 1750316640}'::jsonb
FROM public.target t WHERE t.name = 'web-server-01';

INSERT INTO public.ts_obs (target_id, category_name, ts, payload)
SELECT 
    t.target_id,
    'server',
    '2024-06-19 12:05:00+00'::timestamptz,
    '{"cpu_usage": 52.1, "memory_usage": 71.2, "disk_io": 1450, "created_at": 1750316700}'::jsonb
FROM public.target t WHERE t.name = 'web-server-01';

INSERT INTO public.ts_obs (target_id, category_name, ts, payload)
SELECT 
    t.target_id,
    'sensor',
    '2024-06-19 12:00:00+00'::timestamptz,
    '{"temperature": 22.5, "humidity": 45.2, "readings": ["22.4", "22.5", "22.6"], "created_at": 1750336640}'::jsonb
FROM public.target t WHERE t.name = 'temperature-sensor-01';

INSERT INTO public.ts_obs (target_id, category_name, ts, payload)
SELECT 
    t.target_id,
    'sensor',
    '2024-06-19 12:05:00+00'::timestamptz,
    '{"temperature": 23.1, "humidity": 46.8, "readings": ["23.0", "23.1", "23.2"], "created_at": 1750336700}'::jsonb
FROM public.target t WHERE t.name = 'temperature-sensor-01';

-- 끝 --
