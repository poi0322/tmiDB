package handlers

import (
	"log"

	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
)

// SetupPage는 초기 설정 페이지를 렌더링합니다.
func SetupPage(c *fiber.Ctx) error {
	return c.Render("setup.html", fiber.Map{
		"title": "Initial Setup",
	})
}

// SetupProcess는 초기 설정 폼 제출을 처리합니다.
func SetupProcess(c *fiber.Ctx) error {
	var req struct {
		OrgName  string `json:"org_name"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 기본 관리자 및 조직 생성
	token, err := database.CreateOrgAndAdminUser(req.OrgName, req.Username, req.Password)
	if err != nil {
		log.Printf("Initial setup failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Setup failed: " + err.Error()})
	}

	// 설정 완료 플래그 설정
	if err := database.SetSetupCompleted(); err != nil {
		log.Printf("Failed to set setup completed flag: %v", err)
		// 여기서 실패해도 일단 진행
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"token": token})
}

// SetupStatus는 설정 상태를 확인합니다.
func SetupStatus(c *fiber.Ctx) error {
	completed, err := database.IsSetupCompleted()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error"})
	}
	return c.JSON(fiber.Map{"setup_completed": completed})
}
