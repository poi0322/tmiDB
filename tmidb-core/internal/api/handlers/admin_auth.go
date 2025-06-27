package handlers

import (
	"log"

	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
)

// LoginPage는 로그인 페이지를 렌더링합니다.
func LoginPage(c *fiber.Ctx) error {
	store := c.Locals("session_store").(*session.Store)
	sess, _ := store.Get(c)

	// 이미 로그인된 경우 대시보드로 리디렉션
	if sess.Get("user_id") != nil {
		return c.Redirect("/dashboard")
	}

	// 플래시 메시지 처리
	errMsg := sess.Get("error_flash")
	if errMsg != nil {
		sess.Delete("error_flash")
		sess.Save()
	}

	return c.Render("login.html", fiber.Map{
		"title": "Login",
		"error": errMsg,
	})
}

// LoginProcess는 로그인 요청을 처리합니다.
func LoginProcess(c *fiber.Ctx) error {
	store := c.Locals("session_store").(*session.Store)
	sess, _ := store.Get(c)

	var req struct {
		Username string `form:"username"`
		Password string `form:"password"`
	}

	if err := c.BodyParser(&req); err != nil {
		sess.Set("error_flash", "Invalid request")
		sess.Save()
		return c.Redirect("/login")
	}

	// 사용자 인증
	userID, orgID, role, err := database.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		log.Printf("Login failed for user '%s': %v", req.Username, err)
		sess.Set("error_flash", "Invalid username or password.")
		sess.Save()
		return c.Redirect("/login")
	}

	// 세션에 사용자 정보 저장
	sess.Set("user_id", userID)
	sess.Set("org_id", orgID)
	sess.Set("username", req.Username)
	sess.Set("role", role)
	sess.Set("authenticated", true)

	if err := sess.Save(); err != nil {
		log.Printf("Failed to save session: %v", err)
		sess.Set("error_flash", "Failed to save session.")
		sess.Save()
		return c.Redirect("/login")
	}

	return c.Redirect("/dashboard")
}

// Logout은 로그아웃을 처리합니다.
func Logout(c *fiber.Ctx) error {
	store := c.Locals("session_store").(*session.Store)
	sess, err := store.Get(c)
	if err != nil {
		return c.Redirect("/login")
	}
	sess.Destroy()
	return c.Redirect("/login")
}
