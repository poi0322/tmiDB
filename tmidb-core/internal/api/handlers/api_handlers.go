package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tmidb/tmidb-core/internal/api/middleware"
	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
)

// UUID 패턴 검증을 위한 정규식
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// 카테고리 추출 함수
func categoryFromParams(c *fiber.Ctx) string {
	return c.Params("category")
}

// GetCategorySchema는 카테고리 스키마를 조회합니다.
func GetCategorySchema(c *fiber.Ctx) error {
	version := c.Locals("version").(string)
	category := c.Params("category")

	versionInt, err := strconv.Atoi(version)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid version"})
	}

	var schema string
	err = database.DB.QueryRow(`
		SELECT get_category_schema($1, $2)
	`, category, versionInt).Scan(&schema)

	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schema not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database query failed"})
	}

	var schemaData map[string]interface{}
	json.Unmarshal([]byte(schema), &schemaData)

	return c.JSON(fiber.Map{
		"category": category,
		"version":  version,
		"schema":   schemaData,
	})
}

// GetTargetByID는 특정 대상을 조회합니다.
func GetTargetByID(c *fiber.Ctx) error {
	version := c.Locals("version").(string)
	category := c.Params("category")
	targetID := c.Params("target_id")

	// schema 엔드포인트와 구분하기 위해 UUID 형식만 허용
	if !uuidPattern.MatchString(targetID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid target_id format. Must be a valid UUID."})
	}

	// target_id로 직접 조회
	var targetName, categoryData, updatedAt string
	err := database.DB.QueryRow(`
		SELECT t.name, tc.category_data, tc.updated_at
		FROM target_categories tc
		JOIN target t ON tc.target_id = t.target_id
		WHERE t.target_id = $1 AND tc.category_name = $2 AND tc.schema_version = $3
	`, targetID, category, version).Scan(&targetName, &categoryData, &updatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Target not found"})
		}
		log.Printf("Database query error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database query failed"})
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(categoryData), &data)

	return c.JSON(fiber.Map{
		"target_id":     targetID,
		"target_name":   targetName,
		"category":      category,
		"version":       version,
		"category_data": data,
		"updated_at":    updatedAt,
	})
}

// GetCategoryData는 카테고리 데이터를 조회합니다.
func GetCategoryData(c *fiber.Ctx) error {
	version := c.Locals("version").(string)
	category := c.Params("category")

	// 쿼리 파라미터 파싱
	queryParams := make(url.Values)
	c.Context().QueryArgs().VisitAll(func(key, value []byte) {
		queryParams.Add(string(key), string(value))
	})

	parser := &QueryParser{}
	filters, err := parser.ParseQueryParams(queryParams)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid query parameters"})
	}

	// JSON으로 변환하여 데이터베이스 함수에 전달
	var filtersParam interface{}
	if len(filters) == 0 {
		filtersParam = nil // NULL로 전달
	} else {
		filtersJSON, err := json.Marshal(filters)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to marshal filters"})
		}
		filtersParam = string(filtersJSON)
	}

	versionInt, _ := strconv.Atoi(version)

	// 데이터베이스 쿼리 실행
	rows, err := database.DB.Query(`
		SELECT target_id, target_name, category_data, updated_at 
		FROM get_category_targets_advanced($1, $2, $3)
	`, category, versionInt, filtersParam)

	if err != nil {
		log.Printf("Database query error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database query failed"})
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var targetID, targetName, categoryData, updatedAt string
		err := rows.Scan(&targetID, &targetName, &categoryData, &updatedAt)
		if err != nil {
			continue
		}

		var data map[string]interface{}
		json.Unmarshal([]byte(categoryData), &data)

		// category_data를 평면화하여 병합
		result := map[string]interface{}{
			"target_id":   targetID,
			"target_name": targetName,
			"updated_at":  updatedAt,
		}

		// category_data의 모든 필드를 결과에 병합
		for key, value := range data {
			result[key] = value
		}

		results = append(results, result)
	}

	// 새로운 응답 형태
	response := map[string]interface{}{
		"responseTime": time.Now().Format("2006-01-02 15:04:05"),
	}

	response[category] = map[string]interface{}{
		"version": version,
		"data":    results,
	}

	return c.JSON(response)
}

// GetMultiListenerData는 다중 리스너 데이터를 조회합니다.
func GetMultiListenerData(c *fiber.Ctx) error {
	version := c.Locals("version").(string)

	// URL 경로에서 리스너 ID 목록 추출
	listenerIDs := ParseMultiListenerPath(c.Path())
	if len(listenerIDs) == 0 {
		// 단일 리스너 경로일 수 있으므로 다음 핸들러로 전달
		return c.Next()
	}

	// --- 다중 리스너 인증 시작 ---
	authHeader := c.Get(middleware.HEADER_AUTHORIZATION)
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Authorization header is required"})
	}
	if !strings.HasPrefix(authHeader, middleware.HEADER_BEARER_PREFIX) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token format, must be Bearer token"})
	}
	token := strings.TrimPrefix(authHeader, middleware.HEADER_BEARER_PREFIX)
	tokenHash := middleware.HashToken(token)

	// 각 리스너에 대한 권한 확인
	for _, listenerID := range listenerIDs {
		var categoryName string
		err := database.DB.QueryRow("SELECT category_name FROM listeners WHERE listener_id = $1", listenerID).Scan(&categoryName)
		if err != nil {
			// 리스너가 존재하지 않거나 DB 오류
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": fmt.Sprintf("Permission denied for listener: %s", listenerID)})
		}

		var hasPermission bool
		err = database.DB.QueryRow("SELECT verify_token($1, 'read', $2)", tokenHash, categoryName).Scan(&hasPermission)
		if err != nil || !hasPermission {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": fmt.Sprintf("Permission denied for category: %s (from listener: %s)", categoryName, listenerID)})
		}
	}
	// --- 다중 리스너 인증 끝 ---

	// 클라이언트의 쿼리 파라미터 파싱
	queryParams := make(url.Values)
	c.Context().QueryArgs().VisitAll(func(key, value []byte) {
		queryParams.Add(string(key), string(value))
	})

	parser := &QueryParser{}
	filters, err := parser.ParseQueryParams(queryParams)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid query parameters"})
	}

	// 필터를 JSON으로 변환
	filtersJSON, _ := json.Marshal(filters)

	// PostgreSQL 배열 형식으로 변환
	listenerIDsStr := "{" + strings.Join(listenerIDs, ",") + "}"

	// 다중 리스너 데이터 조회
	var resultJSON string
	err = database.DB.QueryRow(`
		SELECT get_multi_listener_data($1, $2, $3)
	`, listenerIDsStr, "v"+version, string(filtersJSON)).Scan(&resultJSON)

	if err != nil {
		log.Printf("Database query error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database query failed"})
	}

	// JSON 파싱하여 응답
	var result map[string]interface{}
	json.Unmarshal([]byte(resultJSON), &result)

	return c.JSON(result)
}

// GetSingleListenerData는 단일 리스너 데이터를 조회합니다.
func GetSingleListenerData(c *fiber.Ctx) error {
	version := c.Locals("version").(string)
	listenerID := c.Params("listener_id")
	var categoryName string // 인증 및 응답에 사용할 변수

	// --- 단일 리스너 인증 시작 ---
	authHeader := c.Get(middleware.HEADER_AUTHORIZATION)
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Authorization header is required"})
	}
	if !strings.HasPrefix(authHeader, middleware.HEADER_BEARER_PREFIX) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token format, must be Bearer token"})
	}
	token := strings.TrimPrefix(authHeader, middleware.HEADER_BEARER_PREFIX)
	tokenHash := middleware.HashToken(token)

	err := database.DB.QueryRow("SELECT category_name FROM listeners WHERE listener_id = $1", listenerID).Scan(&categoryName)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Permission denied: listener not found or invalid"})
	}

	var hasPermission bool
	err = database.DB.QueryRow("SELECT verify_token($1, 'read', $2)", tokenHash, categoryName).Scan(&hasPermission)
	if err != nil || !hasPermission {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Permission denied for this listener's category"})
	}
	// --- 단일 리스너 인증 끝 ---

	// 클라이언트의 쿼리 파라미터 파싱
	queryParams := make(url.Values)
	c.Context().QueryArgs().VisitAll(func(key, value []byte) {
		queryParams.Add(string(key), string(value))
	})

	parser := &QueryParser{}
	filters, err := parser.ParseQueryParams(queryParams)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid query parameters"})
	}

	// 필터를 JSON으로 변환
	filtersJSON, _ := json.Marshal(filters)

	// 단일 리스너 데이터 조회
	rows, err := database.DB.Query(`
		SELECT target_id, target_name, category_data, updated_at, category_name
		FROM get_listener_filtered_data($1, $2, $3)
	`, listenerID, "v"+version, string(filtersJSON))

	if err != nil {
		log.Printf("Database query error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database query failed"})
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var targetID, targetName, categoryData, updatedAt, catName string
		err := rows.Scan(&targetID, &targetName, &categoryData, &updatedAt, &catName)
		if err != nil {
			continue
		}

		categoryName = catName // 카테고리 이름 저장

		var data map[string]interface{}
		json.Unmarshal([]byte(categoryData), &data)

		// category_data를 평면화하여 병합
		result := map[string]interface{}{
			"target_id":   targetID,
			"target_name": targetName,
			"updated_at":  updatedAt,
		}

		// category_data의 모든 필드를 결과에 병합
		for key, value := range data {
			result[key] = value
		}

		results = append(results, result)
	}

	// 새로운 응답 형태 - 리스너용
	response := map[string]interface{}{
		"responseTime": time.Now().Format("2006-01-02 15:04:05"),
	}

	// 리스너 ID로 감싸고, 그 안에 카테고리로 구조화
	listenerData := map[string]interface{}{}
	if categoryName != "" {
		listenerData[categoryName] = map[string]interface{}{
			"version": version,
			"data":    results,
		}
	}

	response[listenerID] = listenerData

	return c.JSON(response)
}
