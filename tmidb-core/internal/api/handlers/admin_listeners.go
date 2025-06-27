package handlers

import (
	"log"

	"github.com/tmidb/tmidb-core/internal/api/middleware"
	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
)

// ListenersPage는 리스너 관리 페이지를 렌더링합니다.
func ListenersPage(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	listeners, err := database.GetListeners(orgID)
	if err != nil {
		log.Printf("could not get listeners: %v", err)
		return c.Render("admin/listeners.html", fiber.Map{
			"title":     "Listeners",
			"layout":    "main",
			"error":     "Could not load listeners.",
			"listeners": []database.Listener{},
		})
	}
	return c.Render("admin/listeners.html", fiber.Map{
		"title":     "Listeners",
		"layout":    "main",
		"listeners": listeners,
	})
}

// CreateListener는 새 리스너를 생성합니다.
func CreateListener(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	var listener database.Listener
	if err := c.BodyParser(&listener); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	listener.OrgID = orgID

	if err := database.CreateListener(&listener); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not create listener"})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true})
}

// DeleteListener는 리스너를 삭제합니다.
func DeleteListener(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	id := c.Params("id")
	if err := database.DeleteListener(id, orgID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not delete listener"})
	}
	return c.JSON(fiber.Map{"success": true})
}
