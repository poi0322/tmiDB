package handlers

import (
	"encoding/json"
	"log"
	"time"

	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
)

// GetTargetsByCategory는 카테고리별 타겟 목록을 반환합니다.
func GetTargetsByCategory(c *fiber.Ctx) error {
	category := c.Params("category")

	type TargetInfo struct {
		TargetID string `json:"target_id"`
		Name     string `json:"name"`
	}

	rows, err := database.DB.Query("SELECT target_id, name FROM get_targets_by_category($1)", category)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	var targets []TargetInfo
	for rows.Next() {
		var t TargetInfo
		if err := rows.Scan(&t.TargetID, &t.Name); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "scan error"})
		}
		targets = append(targets, t)
	}
	return c.JSON(targets)
}

// GetTargetDetails는 특정 타겟의 상세 정보를 반환합니다.
func GetTargetDetails(c *fiber.Ctx) error {
	targetID := c.Params("id")
	category := c.Query("category")

	var targetName, categoryData, updatedAt string
	err := database.DB.QueryRow(`
		SELECT t.name, tc.category_data, tc.updated_at
		FROM target_categories tc
		JOIN target t ON tc.target_id = t.target_id
		WHERE t.target_id = $1 AND tc.category_name = $2
	`, targetID, category).Scan(&targetName, &categoryData, &updatedAt)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "target not found"})
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(categoryData), &data)

	return c.JSON(fiber.Map{
		"target_id":     targetID,
		"target_name":   targetName,
		"category":      category,
		"category_data": data,
		"updated_at":    updatedAt,
	})
}

// GetTimeSeriesData는 특정 타겟의 시계열 데이터를 반환합니다.
func GetTimeSeriesData(c *fiber.Ctx) error {
	targetID := c.Params("id")
	category := c.Query("category")

	type TsData struct {
		Ts      time.Time       `json:"ts"`
		Payload json.RawMessage `json:"payload"`
	}

	rows, err := database.DB.Query(`
		SELECT ts, payload FROM public.ts_obs 
		WHERE target_id = $1 AND category_name = $2 
		ORDER BY ts DESC LIMIT 100
	`, targetID, category)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	var results []TsData
	for rows.Next() {
		var d TsData
		if err := rows.Scan(&d.Ts, &d.Payload); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "scan error"})
		}
		results = append(results, d)
	}

	return c.JSON(results)
}

// InsertTimeSeriesData는 시계열 데이터를 추가합니다.
func InsertTimeSeriesData(c *fiber.Ctx) error {
	var req struct {
		TargetID     string `json:"target_id"`
		CategoryName string `json:"category_name"`
		Ts           string `json:"ts"`
		Payload      string `json:"payload"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Payload JSON 유효성 검사
	if !json.Valid([]byte(req.Payload)) {
		return c.Status(400).JSON(fiber.Map{"error": "Payload is not valid JSON"})
	}

	_, err := database.DB.Exec("SELECT insert_ts_obs($1, $2, $3, $4)",
		req.TargetID, req.CategoryName, req.Ts, req.Payload)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to insert data", "details": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "success"})
}

// GetSchemaAPI는 데이터 탐색을 위한 스키마 정보를 반환합니다.
func GetSchemaAPI(c *fiber.Ctx) error {
	// TODO: org_id를 세션에서 가져와야 함.
	// schema, err := database.GetFullSchemaForOrg("some-org-id")
	// if err != nil {
	// 	 return c.Status(500).JSON(fiber.Map{"error": "could not get schema"})
	// }
	// return c.JSON(schema)
	return c.JSON(fiber.Map{"message": "Schema API not implemented yet"})
}

// ExecuteQueryAPI는 데이터 탐색기에서 받은 쿼리를 실행합니다.
func ExecuteQueryAPI(c *fiber.Ctx) error {
	var req struct {
		Query string `json:"query"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// 쿼리 파싱 및 실행 로직 필요
	log.Printf("Received query: %s", req.Query)

	// result, err := ParseAndExecute(req.Query)
	// if err != nil {
	// 	 return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	// }
	// return c.JSON(result)
	return c.JSON(fiber.Map{"message": "Query execution not implemented yet"})
}
