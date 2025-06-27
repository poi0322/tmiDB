package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// DataExplorerPage는 데이터 탐색기 페이지를 렌더링합니다.
func DataExplorerPage(c *fiber.Ctx) error {
	return c.Render("admin/data_explorer.html", fiber.Map{
		"title":  "Data Explorer",
		"layout": "main",
	})
}
