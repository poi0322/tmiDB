package database

// 데이터베이스 함수들
const functionsSQL = `
-- tmiDB 유틸리티 함수들

-- 1. 카테고리 스키마 검증 함수
CREATE OR REPLACE FUNCTION validate_category_data(
    p_category_name TEXT,
    p_data JSONB
) RETURNS BOOLEAN AS $$
DECLARE
    schema_def JSONB;
    field_name TEXT;
    field_def JSONB;
    field_value JSONB;
BEGIN
    -- 활성화된 최신 스키마 가져오기
    SELECT schema_definition INTO schema_def
    FROM category_schemas 
    WHERE category_name = p_category_name 
      AND is_active = true
    ORDER BY version DESC 
    LIMIT 1;
    
    IF NOT FOUND THEN
        RETURN FALSE;
    END IF;
    
    -- 각 필드 검증
    FOR field_name IN SELECT jsonb_object_keys(schema_def->'fields')
    LOOP
        field_def := schema_def->'fields'->field_name;
        field_value := p_data->field_name;
        
        -- 필수 필드 검사
        IF (field_def->>'required')::boolean = true AND field_value IS NULL THEN
            RETURN FALSE;
        END IF;
        
        -- enum 값 검사
        IF field_def ? 'enum' AND field_value IS NOT NULL THEN
            IF NOT (field_value #>> '{}' = ANY(ARRAY(SELECT jsonb_array_elements_text(field_def->'enum')))) THEN
                RETURN FALSE;
            END IF;
        END IF;
    END LOOP;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- 2. 대상 생성 함수
CREATE OR REPLACE FUNCTION create_target_with_categories(
    p_name TEXT,
    p_categories JSONB
) RETURNS UUID AS $$
DECLARE
    new_target_id UUID;
    category_item JSONB;
    category_name TEXT;
    category_data JSONB;
BEGIN
    -- 새 대상 생성
    INSERT INTO target (name) VALUES (p_name) RETURNING target_id INTO new_target_id;
    
    -- 각 카테고리 데이터 삽입
    FOR category_item IN SELECT * FROM jsonb_array_elements(p_categories)
    LOOP
        category_name := category_item->>'category_name';
        category_data := category_item->'data';
        
        -- 스키마 검증
        IF NOT validate_category_data(category_name, category_data) THEN
            RAISE EXCEPTION 'Invalid data for category: %', category_name;
        END IF;
        
        -- 카테고리 데이터 삽입
        INSERT INTO target_categories (target_id, category_name, category_data)
        VALUES (new_target_id, category_name, category_data);
    END LOOP;
    
    RETURN new_target_id;
END;
$$ LANGUAGE plpgsql;

-- 3. 관측 데이터 삽입 함수
CREATE OR REPLACE FUNCTION insert_observation(
    p_target_id UUID,
    p_category_name TEXT,
    p_payload JSONB,
    p_timestamp TIMESTAMPTZ DEFAULT NULL
) RETURNS BOOLEAN AS $$
DECLARE
    obs_timestamp TIMESTAMPTZ;
BEGIN
    -- 타임스탬프 설정
    obs_timestamp := COALESCE(p_timestamp, NOW());
    
    -- 대상-카테고리 존재 확인
    IF NOT EXISTS (
        SELECT 1 FROM target_categories 
        WHERE target_id = p_target_id AND category_name = p_category_name
    ) THEN
        RAISE EXCEPTION 'Target-category combination does not exist: % - %', p_target_id, p_category_name;
    END IF;
    
    -- 관측 데이터 삽입
    INSERT INTO ts_obs (target_id, category_name, ts, payload)
    VALUES (p_target_id, p_category_name, obs_timestamp, p_payload);
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- 4. 위치 추적 데이터 삽입 함수
CREATE OR REPLACE FUNCTION insert_geo_trace(
    p_target_id UUID,
    p_lon DOUBLE PRECISION,
    p_lat DOUBLE PRECISION,
    p_timestamp TIMESTAMPTZ DEFAULT NULL
) RETURNS BOOLEAN AS $$
DECLARE
    trace_timestamp TIMESTAMPTZ;
BEGIN
    -- 타임스탬프 설정
    trace_timestamp := COALESCE(p_timestamp, NOW());
    
    -- 대상 존재 확인
    IF NOT EXISTS (SELECT 1 FROM target WHERE target_id = p_target_id) THEN
        RAISE EXCEPTION 'Target does not exist: %', p_target_id;
    END IF;
    
    -- 위치 데이터 삽입
    INSERT INTO geo_trace (target_id, ts, lon, lat)
    VALUES (p_target_id, trace_timestamp, p_lon, p_lat)
    ON CONFLICT (target_id, ts) DO UPDATE SET
        lon = EXCLUDED.lon,
        lat = EXCLUDED.lat;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- 5. 대상 검색 함수 (동적 쿼리)
CREATE OR REPLACE FUNCTION search_targets(
    p_category_name TEXT DEFAULT NULL,
    p_filters JSONB DEFAULT NULL,
    p_limit INTEGER DEFAULT 100,
    p_offset INTEGER DEFAULT 0
) RETURNS TABLE (
    target_id UUID,
    name TEXT,
    category_data JSONB,
    created_at TIMESTAMPTZ
) AS $$
DECLARE
    query_sql TEXT;
    where_conditions TEXT[] := ARRAY[]::TEXT[];
    filter_key TEXT;
    filter_value TEXT;
BEGIN
    -- 기본 쿼리
    query_sql := 'SELECT t.target_id, t.name, tc.category_data, t.created_at 
                  FROM target t 
                  LEFT JOIN target_categories tc ON t.target_id = tc.target_id';
    
    -- 카테고리 필터
    IF p_category_name IS NOT NULL THEN
        where_conditions := array_append(where_conditions, format('tc.category_name = %L', p_category_name));
    END IF;
    
    -- 동적 필터 적용
    IF p_filters IS NOT NULL THEN
        FOR filter_key IN SELECT jsonb_object_keys(p_filters)
        LOOP
            filter_value := p_filters ->> filter_key;
            where_conditions := array_append(where_conditions, 
                format('tc.category_data ->> %L = %L', filter_key, filter_value));
        END LOOP;
    END IF;
    
    -- WHERE 절 추가
    IF array_length(where_conditions, 1) > 0 THEN
        query_sql := query_sql || ' WHERE ' || array_to_string(where_conditions, ' AND ');
    END IF;
    
    -- 정렬 및 제한
    query_sql := query_sql || format(' ORDER BY t.created_at DESC LIMIT %s OFFSET %s', p_limit, p_offset);
    
    -- 동적 쿼리 실행
    RETURN QUERY EXECUTE query_sql;
END;
$$ LANGUAGE plpgsql;

-- 6. 최근 관측 데이터 조회 함수
CREATE OR REPLACE FUNCTION get_recent_observations(
    p_target_id UUID DEFAULT NULL,
    p_category_name TEXT DEFAULT NULL,
    p_hours INTEGER DEFAULT 24,
    p_limit INTEGER DEFAULT 1000
) RETURNS TABLE (
    target_id UUID,
    category_name TEXT,
    ts TIMESTAMPTZ,
    payload JSONB
) AS $$
DECLARE
    since_timestamp TIMESTAMPTZ;
BEGIN
    since_timestamp := NOW() - (p_hours || ' hours')::INTERVAL;
    
    RETURN QUERY
    SELECT o.target_id, o.category_name, o.ts, o.payload
    FROM ts_obs o
    WHERE (p_target_id IS NULL OR o.target_id = p_target_id)
      AND (p_category_name IS NULL OR o.category_name = p_category_name)
      AND o.ts >= since_timestamp
    ORDER BY o.ts DESC
    LIMIT p_limit;
END;
$$ LANGUAGE plpgsql;

-- 7. 통계 조회 함수
CREATE OR REPLACE FUNCTION get_database_stats()
RETURNS TABLE (
    total_targets BIGINT,
    total_categories BIGINT,
    total_observations BIGINT,
    database_size TEXT,
    last_observation TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        (SELECT COUNT(*) FROM target) as total_targets,
        (SELECT COUNT(DISTINCT category_name) FROM target_categories) as total_categories,
        (SELECT COUNT(*) FROM ts_obs) as total_observations,
        pg_size_pretty(pg_database_size(current_database())) as database_size,
        (SELECT MAX(ts) FROM ts_obs) as last_observation;
END;
$$ LANGUAGE plpgsql;

-- 8. 카테고리별 통계 함수
CREATE OR REPLACE FUNCTION get_category_stats(p_category_name TEXT)
RETURNS TABLE (
    category_name TEXT,
    target_count BIGINT,
    observation_count BIGINT,
    latest_observation TIMESTAMPTZ,
    oldest_observation TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        p_category_name as category_name,
        COUNT(DISTINCT tc.target_id) as target_count,
        COUNT(o.target_id) as observation_count,
        MAX(o.ts) as latest_observation,
        MIN(o.ts) as oldest_observation
    FROM target_categories tc
    LEFT JOIN ts_obs o ON tc.target_id = o.target_id AND tc.category_name = o.category_name
    WHERE tc.category_name = p_category_name
    GROUP BY tc.category_name;
END;
$$ LANGUAGE plpgsql;

-- 9. 원본 데이터 처리 함수
CREATE OR REPLACE FUNCTION process_raw_data(
    p_source TEXT,
    p_payload JSONB
) RETURNS UUID AS $$
DECLARE
    raw_id UUID;
BEGIN
    -- 원본 데이터 저장
    INSERT INTO raw_bucket (source, payload) 
    VALUES (p_source, p_payload) 
    RETURNING raw_id INTO raw_id;
    
    -- 여기에 데이터 파싱 및 분류 로직 추가 가능
    -- 예: payload 구조에 따라 적절한 target과 category로 분류
    
    RETURN raw_id;
END;
$$ LANGUAGE plpgsql;

-- 10. 권한 확인 함수
CREATE OR REPLACE FUNCTION check_user_permission(
    p_username TEXT,
    p_category_name TEXT,
    p_operation TEXT -- 'read' or 'write'
) RETURNS BOOLEAN AS $$
DECLARE
    user_permissions JSONB;
    allowed_categories TEXT[];
BEGIN
    -- 사용자 권한 조회
    SELECT permissions INTO user_permissions
    FROM users 
    WHERE username = p_username AND is_active = true;
    
    IF NOT FOUND THEN
        RETURN FALSE;
    END IF;
    
    -- 권한 배열 추출
    allowed_categories := ARRAY(SELECT jsonb_array_elements_text(user_permissions->p_operation));
    
    -- 전체 권한(*) 또는 특정 카테고리 권한 확인
    RETURN '*' = ANY(allowed_categories) OR p_category_name = ANY(allowed_categories);
END;
$$ LANGUAGE plpgsql;

-- 11. API 토큰 권한 확인 함수
CREATE OR REPLACE FUNCTION check_token_permission(
    p_token_hash TEXT,
    p_category_name TEXT,
    p_operation TEXT -- 'read' or 'write'
) RETURNS BOOLEAN AS $$
DECLARE
    token_permissions JSONB;
    allowed_categories TEXT[];
    token_active BOOLEAN;
BEGIN
    -- 토큰 권한 조회
    SELECT permissions, is_active INTO token_permissions, token_active
    FROM auth_tokens 
    WHERE token_hash = p_token_hash 
      AND (expires_at IS NULL OR expires_at > NOW());
    
    IF NOT FOUND OR NOT token_active THEN
        RETURN FALSE;
    END IF;
    
    -- 권한 배열 추출
    allowed_categories := ARRAY(SELECT jsonb_array_elements_text(token_permissions->p_operation));
    
    -- 전체 권한(*) 또는 특정 카테고리 권한 확인
    RETURN '*' = ANY(allowed_categories) OR p_category_name = ANY(allowed_categories);
END;
$$ LANGUAGE plpgsql;

-- 12. 데이터 정리 함수
CREATE OR REPLACE FUNCTION cleanup_old_data(
    p_category_name TEXT DEFAULT NULL,
    p_days_to_keep INTEGER DEFAULT 30
) RETURNS BIGINT AS $$
DECLARE
    deleted_count BIGINT;
    cutoff_date TIMESTAMPTZ;
BEGIN
    cutoff_date := NOW() - (p_days_to_keep || ' days')::INTERVAL;
    
    -- 오래된 관측 데이터 삭제
    DELETE FROM ts_obs 
    WHERE ts < cutoff_date
      AND (p_category_name IS NULL OR category_name = p_category_name);
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    -- 오래된 위치 추적 데이터 삭제
    DELETE FROM geo_trace 
    WHERE ts < cutoff_date;
    
    -- 오래된 원본 데이터 삭제
    DELETE FROM raw_bucket 
    WHERE ts < cutoff_date;
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;
`
