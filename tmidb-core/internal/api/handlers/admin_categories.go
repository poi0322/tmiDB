package handlers

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tmidb/tmidb-core/internal/api/middleware"
	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
)

// CategoriesPage는 카테고리 관리 페이지를 렌더링합니다.
func CategoriesPage(c *fiber.Ctx) error {
	return c.Render("admin/categories.html", fiber.Map{
		"title":  "Category Management",
		"layout": "main",
	})
}

// GetCategoriesAPI는 현재 조직의 모든 카테고리를 반환합니다.
func GetCategoriesAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	categories, err := database.GetCategories(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not fetch categories"})
	}
	return c.JSON(categories)
}

// CreateCategoryAPI는 현재 조직에 새 카테고리를 생성합니다.
func CreateCategoryAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	var category database.CategorySchema
	if err := c.BodyParser(&category); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	category.OrgID = orgID

	if err := database.CreateCategory(&category); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not create category"})
	}
	return c.Status(201).JSON(category)
}

// UpdateCategoryAPI는 현재 조직의 카테고리를 업데이트합니다.
func UpdateCategoryAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	var category database.CategorySchema
	if err := c.BodyParser(&category); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	category.OrgID = orgID
	category.CategoryName = c.Params("name")

	if err := database.UpdateCategory(&category); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not update category"})
	}

	return c.Status(200).JSON(category)
}

// DeleteCategoryAPI는 현재 조직의 카테고리를 삭제합니다.
func DeleteCategoryAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}
	categoryName := c.Params("name")

	if err := database.DeleteCategory(categoryName, orgID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not delete category: " + err.Error()})
	}
	return c.SendStatus(204)
}

// GetCategorySchemaAPI는 현재 조직의 특정 카테고리 스키마를 반환합니다.
func GetCategorySchemaAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}
	categoryName := c.Params("name")

	schema, err := database.GetCategorySchema(categoryName, orgID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "category schema not found"})
	}

	return c.JSON(schema)
}

// 웹 페이지용 핸들러들 (HTML 렌더링)

// CreateCategoryHandler는 카테고리 생성 페이지를 렌더링합니다.
func CreateCategoryHandler(c *fiber.Ctx) error {
	return c.Render("admin/category_form", fiber.Map{
		"title": "Create Category",
		"schema": map[string]interface{}{
			"SchemaID":         "",
			"CategoryName":     "",
			"Version":          1,
			"SchemaDefinition": "",
			"IsActive":         true,
		},
	})
}

// EditCategoryHandler는 카테고리 수정 페이지를 렌더링합니다.
func EditCategoryHandler(c *fiber.Ctx) error {
	categoryName := c.Params("id") // URL에서 카테고리 이름을 가져옴

	// 카테고리 정보 조회
	var version int
	var schemaDefinition string
	var isActive bool

	err := database.DB.QueryRow(`
		SELECT version, schema_definition, is_active
		FROM category_schemas 
		WHERE category_name = $1 
		ORDER BY version DESC 
		LIMIT 1
	`, categoryName).Scan(&version, &schemaDefinition, &isActive)

	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).SendString("Category not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Database error")
	}

	return c.Render("admin/category_form", fiber.Map{
		"title": "Edit Category",
		"schema": map[string]interface{}{
			"SchemaID":         categoryName, // 편집 모드에서는 SchemaID가 있음
			"CategoryName":     categoryName,
			"Version":          version,
			"SchemaDefinition": schemaDefinition,
			"IsActive":         isActive,
		},
	})
}

// SaveCategoryHandler는 카테고리를 저장합니다 (생성/수정).
func SaveCategoryHandler(c *fiber.Ctx) error {
	schemaID := c.FormValue("schema_id")
	categoryName := c.FormValue("category_name")
	versionStr := c.FormValue("version")
	schemaDefinition := c.FormValue("schema_definition")
	isActive := c.FormValue("is_active") == "on"

	// 버전을 정수로 변환
	version := 1
	if versionStr != "" {
		if v, err := strconv.Atoi(versionStr); err == nil {
			version = v
		}
	}

	// JSON 유효성 검사
	var jsonTest interface{}
	if err := json.Unmarshal([]byte(schemaDefinition), &jsonTest); err != nil {
		return c.Render("admin/category_form", fiber.Map{
			"title": "Create Category",
			"error": "Invalid JSON format in schema definition",
			"schema": map[string]interface{}{
				"SchemaID":         schemaID,
				"CategoryName":     categoryName,
				"Version":          version,
				"SchemaDefinition": schemaDefinition,
				"IsActive":         isActive,
			},
		})
	}

	var err error
	if schemaID == "" {
		// 새 카테고리 생성
		_, err = database.DB.Exec(`
			INSERT INTO category_schemas (category_name, version, schema_definition, is_active) 
			VALUES ($1, $2, $3, $4)
		`, categoryName, version, schemaDefinition, isActive)
	} else {
		// 기존 카테고리 업데이트 (새 버전 생성)
		// 현재 최대 버전 조회
		var maxVersion int
		database.DB.QueryRow(`
			SELECT COALESCE(MAX(version), 0) 
			FROM category_schemas 
			WHERE category_name = $1
		`, categoryName).Scan(&maxVersion)

		newVersion := maxVersion + 1
		_, err = database.DB.Exec(`
			INSERT INTO category_schemas (category_name, version, schema_definition, is_active) 
			VALUES ($1, $2, $3, $4)
		`, categoryName, newVersion, schemaDefinition, isActive)
	}

	if err != nil {
		errorMsg := "Failed to save category"
		if strings.Contains(err.Error(), "duplicate key") {
			errorMsg = "Category already exists"
		}

		return c.Render("admin/category_form", fiber.Map{
			"title": "Create Category",
			"error": errorMsg,
			"schema": map[string]interface{}{
				"SchemaID":         schemaID,
				"CategoryName":     categoryName,
				"Version":          version,
				"SchemaDefinition": schemaDefinition,
				"IsActive":         isActive,
			},
		})
	}

	return c.Redirect("/categories")
}
