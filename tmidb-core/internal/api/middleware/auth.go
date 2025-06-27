package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
)

// 인증 관련 상수
const (
	HEADER_AUTHORIZATION = "Authorization"
	HEADER_BEARER_PREFIX = "Bearer "
	ADMIN_PERMISSION     = "admin"
)

// HashToken은 클라이언트가 보낸 토큰을 SHA256으로 해싱합니다.
func HashToken(token string) string {
	hasher := sha256.New()
	hasher.Write([]byte(token))
	return hex.EncodeToString(hasher.Sum(nil))
}

// TokenAuthRequired는 API 요청에 대한 토큰 인증을 처리하는 미들웨어입니다.
func TokenAuthRequired(requiredPermission string, getCategory func(*fiber.Ctx) string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get(HEADER_AUTHORIZATION)
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Authorization header is required"})
		}

		if !strings.HasPrefix(authHeader, HEADER_BEARER_PREFIX) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token format, must be Bearer token"})
		}

		token := strings.TrimPrefix(authHeader, HEADER_BEARER_PREFIX)
		tokenHash := HashToken(token)

		var categoryName string
		if getCategory != nil {
			categoryName = getCategory(c)
		}

		var hasPermission bool
		err := database.DB.QueryRow("SELECT verify_token($1, $2, $3)", tokenHash, requiredPermission, categoryName).Scan(&hasPermission)
		if err != nil || !hasPermission {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Permission denied"})
		}

		return c.Next()
	}
}

// VerifyTokenForLogin은 로그인 시 토큰을 검증합니다.
func VerifyTokenForLogin(token string) (bool, error) {
	tokenHash := HashToken(token)
	var hasPermission bool
	err := database.DB.QueryRow("SELECT verify_token($1, 'admin', NULL)", tokenHash).Scan(&hasPermission)
	return hasPermission, err
}

// AuthRequired는 인증이 필요한 경로를 보호하는 미들웨어입니다.
func AuthRequired(store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return c.Redirect("/login")
		}

		if sess.Get("authenticated") != true {
			return c.Redirect("/login")
		}

		return c.Next()
	}
}

// AdminRequired는 관리자 권한이 필요한 경로를 보호하는 미들웨어입니다.
func AdminRequired(store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		if sess.Get("authenticated") != true {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		role := sess.Get("role")
		if role != "admin" {
			return c.Status(fiber.StatusForbidden).SendString("Admin privileges required")
		}

		return c.Next()
	}
}

// GetUserID는 세션에서 사용자 ID를 가져옵니다.
func GetUserID(c *fiber.Ctx, store *session.Store) (string, error) {
	sess, err := store.Get(c)
	if err != nil {
		return "", err
	}

	if sess.Get("authenticated") != true {
		return "", fiber.NewError(fiber.StatusUnauthorized, "Not authenticated")
	}

	userID := sess.Get("user_id")
	if userID == nil {
		return "", fiber.NewError(fiber.StatusUnauthorized, "User ID not found in session")
	}

	return userID.(string), nil
}

// GetUserRole은 세션에서 사용자 역할을 가져옵니다.
func GetUserRole(c *fiber.Ctx, store *session.Store) (string, error) {
	sess, err := store.Get(c)
	if err != nil {
		return "", err
	}

	if sess.Get("authenticated") != true {
		return "", fiber.NewError(fiber.StatusUnauthorized, "Not authenticated")
	}

	role := sess.Get("role")
	if role == nil {
		return "", fiber.NewError(fiber.StatusUnauthorized, "User role not found in session")
	}

	return role.(string), nil
}

// IsAuthenticated는 현재 사용자가 인증되었는지 확인합니다.
func IsAuthenticated(c *fiber.Ctx, store *session.Store) bool {
	sess, err := store.Get(c)
	if err != nil {
		return false
	}

	authenticated := sess.Get("authenticated")
	return authenticated == true
}

// IsAdmin은 현재 사용자가 관리자인지 확인합니다.
func IsAdmin(c *fiber.Ctx, store *session.Store) bool {
	role, err := GetUserRole(c, store)
	if err != nil {
		return false
	}
	return role == "admin"
}

// GetOrgID는 세션에서 현재 사용자의 조직 ID를 반환합니다.
func GetOrgID(c *fiber.Ctx) (string, error) {
	store := c.Locals("session_store").(*session.Store)
	sess, err := store.Get(c)
	if err != nil {
		return "", fmt.Errorf("failed to get session")
	}

	orgID := sess.Get("org_id")
	if orgID == nil {
		return "", fmt.Errorf("org_id not found in session")
	}

	return orgID.(string), nil
}
