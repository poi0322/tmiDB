package handlers

import (
	"fmt"
	"log"

	"github.com/tmidb/tmidb-core/internal/api/middleware"
	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
)

// TokensPage는 API 토큰 관리 페이지를 렌더링합니다.
func TokensPage(c *fiber.Ctx) error {
	return c.Render("admin/tokens.html", fiber.Map{
		"title": "Token Management",
	}, "main.html")
}

// GetAuthTokensAPI는 역할에 따라 사용자의 토큰 또는 조직의 모든 토큰을 조회합니다.
func GetAuthTokensAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}
	userID, role, err := getUserInfoFromSession(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Session error"})
	}

	var tokens []database.AuthToken
	if role == "admin" {
		tokens, err = database.GetAllUserTokens(orgID)
	} else {
		tokens, err = database.GetUserTokens(userID, orgID)
	}

	if err != nil {
		log.Printf("Error getting auth tokens: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get tokens"})
	}

	return c.JSON(tokens)
}

// CreateAuthTokenAPI는 현재 사용자를 위한 새로운 인증 토큰을 생성합니다.
func CreateAuthTokenAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}
	userID, _, err := getUserInfoFromSession(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Session error"})
	}

	var req struct {
		Description string `json:"description"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	rawToken, createdToken, err := database.CreateUserToken(userID, orgID, req.Description)
	if err != nil {
		log.Printf("Error creating auth token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create token"})
	}
	createdToken.DecryptedToken = rawToken // 응답에만 원본 토큰 포함

	return c.Status(fiber.StatusCreated).JSON(createdToken)
}

// DeleteAuthTokenAPI는 역할에 따라 사용자의 토큰 또는 조직의 토큰을 삭제합니다.
func DeleteAuthTokenAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}
	userID, role, err := getUserInfoFromSession(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Session error"})
	}

	tokenID := c.Params("id") // URL 파라미터에서 ID를 가져옵니다.

	if role == "admin" {
		err = database.DeleteUserTokenAsAdmin(tokenID, orgID)
	} else {
		err = database.DeleteUserToken(tokenID, userID, orgID)
	}

	if err != nil {
		log.Printf("Error deleting token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// getUserInfoFromSession은 세션에서 사용자 ID와 역할을 추출하는 헬퍼 함수입니다.
func getUserInfoFromSession(c *fiber.Ctx) (string, string, error) {
	store := c.Locals("session_store").(*session.Store)
	sess, err := store.Get(c)
	if err != nil {
		return "", "", fmt.Errorf("failed to get session: %w", err)
	}

	userID := sess.Get("user_id")
	role := sess.Get("role")

	if userID == nil || role == nil {
		return "", "", fmt.Errorf("user information not found in session")
	}

	return userID.(string), role.(string), nil
}
