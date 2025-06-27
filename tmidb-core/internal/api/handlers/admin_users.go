package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/tmidb/tmidb-core/internal/api/middleware"
	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"golang.org/x/crypto/bcrypt"
)

// GetUsersHandler는 모든 사용자 목록을 조회합니다.
func GetUsersHandler(c *fiber.Ctx, store *session.Store) error {
	// 관리자 권한 확인
	sess, err := store.Get(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Session error",
		})
	}

	role := sess.Get("role")
	if role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Admin access required",
		})
	}

	// 사용자 목록 조회
	rows, err := database.DB.Query(`
		SELECT user_id, username, role, permissions, is_active, created_at, updated_at 
		FROM users 
		ORDER BY created_at DESC
	`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to query users",
		})
	}
	defer rows.Close()

	var users []fiber.Map
	for rows.Next() {
		var userID, username, role, permissions string
		var isActive bool
		var createdAt, updatedAt string

		err := rows.Scan(&userID, &username, &role, &permissions, &isActive, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		users = append(users, fiber.Map{
			"user_id":     userID,
			"username":    username,
			"role":        role,
			"permissions": permissions,
			"is_active":   isActive,
			"created_at":  createdAt,
			"updated_at":  updatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"users": users,
	})
}

// CreateUserHandler는 새 사용자를 생성합니다.
func CreateUserHandler(c *fiber.Ctx, store *session.Store) error {
	// 관리자 권한 확인
	sess, err := store.Get(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Session error",
		})
	}

	role := sess.Get("role")
	if role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Admin access required",
		})
	}

	// 요청 데이터 파싱
	type CreateUserRequest struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Role        string `json:"role"`
		Permissions string `json:"permissions"`
	}

	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// 입력 검증
	if req.Username == "" || req.Password == "" || req.Role == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Username, password, and role are required",
		})
	}

	// 허용된 역할 확인
	if req.Role != "admin" && req.Role != "editor" && req.Role != "viewer" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Role must be admin, editor, or viewer",
		})
	}

	// 기본 권한 설정
	if req.Permissions == "" {
		switch req.Role {
		case "admin":
			req.Permissions = `{"read": ["*"], "write": ["*"]}`
		case "editor":
			req.Permissions = `{"read": ["*"], "write": ["*"]}`
		case "viewer":
			req.Permissions = `{"read": ["*"], "write": []}`
		}
	}

	// 사용자 중복 확인
	var exists bool
	err = database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", req.Username).Scan(&exists)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database error",
		})
	}

	if exists {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Username already exists",
		})
	}

	// 비밀번호 해시
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to hash password",
		})
	}

	// 사용자 생성
	var userID string
	err = database.DB.QueryRow(`
		INSERT INTO users (username, password_hash, role, permissions, is_active) 
		VALUES ($1, $2, $3, $4, true) 
		RETURNING user_id
	`, req.Username, string(hashedPassword), req.Role, req.Permissions).Scan(&userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":  "User created successfully",
		"user_id":  userID,
		"username": req.Username,
		"role":     req.Role,
	})
}

// UpdateUserHandler는 사용자 정보를 업데이트합니다.
func UpdateUserHandler(c *fiber.Ctx, store *session.Store) error {
	// 관리자 권한 확인
	sess, err := store.Get(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Session error",
		})
	}

	role := sess.Get("role")
	if role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Admin access required",
		})
	}

	userID := c.Params("id")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User ID is required",
		})
	}

	// 요청 데이터 파싱
	type UpdateUserRequest struct {
		Role        string `json:"role"`
		Permissions string `json:"permissions"`
		IsActive    *bool  `json:"is_active"`
	}

	var req UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// 업데이트할 필드 구성
	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.Role != "" {
		if req.Role != "admin" && req.Role != "editor" && req.Role != "viewer" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Role must be admin, editor, or viewer",
			})
		}
		updates = append(updates, "role = $"+fmt.Sprintf("%d", argIndex))
		args = append(args, req.Role)
		argIndex++
	}

	if req.Permissions != "" {
		updates = append(updates, "permissions = $"+fmt.Sprintf("%d", argIndex))
		args = append(args, req.Permissions)
		argIndex++
	}

	if req.IsActive != nil {
		updates = append(updates, "is_active = $"+fmt.Sprintf("%d", argIndex))
		args = append(args, *req.IsActive)
		argIndex++
	}

	if len(updates) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No fields to update",
		})
	}

	// 업데이트 실행
	query := fmt.Sprintf("UPDATE users SET %s WHERE user_id = $%d",
		strings.Join(updates, ", "), argIndex)
	args = append(args, userID)

	result, err := database.DB.Exec(query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update user",
		})
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(fiber.Map{
		"message": "User updated successfully",
	})
}

// DeleteUserHandler는 사용자를 삭제합니다.
func DeleteUserHandler(c *fiber.Ctx, store *session.Store) error {
	// 관리자 권한 확인
	sess, err := store.Get(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Session error",
		})
	}

	role := sess.Get("role")
	if role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Admin access required",
		})
	}

	userID := c.Params("id")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User ID is required",
		})
	}

	// 현재 로그인한 사용자 확인 (자신을 삭제하지 못하도록)
	currentUserID := sess.Get("user_id")
	if currentUserID == userID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot delete your own account",
		})
	}

	// 삭제하려는 사용자의 역할 확인
	var targetRole string
	err = database.DB.QueryRow("SELECT role FROM users WHERE user_id = $1", userID).Scan(&targetRole)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// 관리자를 삭제하려는 경우, 최소 2명의 관리자가 있는지 확인
	if targetRole == "admin" {
		var adminCount int
		err = database.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin' AND is_active = true").Scan(&adminCount)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to check admin count",
			})
		}

		if adminCount <= 1 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Cannot delete the last admin user",
			})
		}
	}

	// 사용자 삭제
	result, err := database.DB.Exec("DELETE FROM users WHERE user_id = $1", userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete user",
		})
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(fiber.Map{
		"message": "User deleted successfully",
	})
}

// UsersPage는 사용자 관리 페이지를 렌더링합니다.
func UsersPage(c *fiber.Ctx) error {
	return c.Render("admin/users.html", fiber.Map{
		"title": "User Management",
	}, "main.html")
}

// GetUsersAPI는 현재 조직의 모든 사용자 목록을 반환합니다.
func GetUsersAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	users, err := database.GetUsers(orgID)
	if err != nil {
		log.Printf("Error getting users: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve users"})
	}
	return c.JSON(users)
}

// CreateUserAPI는 현재 조직에 새 사용자를 생성합니다.
func CreateUserAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
		IsActive bool   `json:"is_active"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	user := database.User{
		OrgID:    orgID,
		Username: req.Username,
		Password: req.Password,
		Role:     req.Role,
		IsActive: req.IsActive,
	}
	createdUser, err := database.CreateUser(user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create user: %v", err)})
	}

	return c.Status(fiber.StatusCreated).JSON(createdUser)
}

// UpdateUserAPI는 현재 조직의 기존 사용자를 업데이트합니다.
func UpdateUserAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	id := c.Params("id")
	var req struct {
		Role     string `json:"role"`
		IsActive *bool  `json:"is_active"`
		Password string `json:"password,omitempty"` // For password changes
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// is_active 필드가 nil일 때 의도치 않게 false로 업데이트되는 것을 방지하기 위해
	// 먼저 현재 사용자 정보를 가져옵니다.
	users, err := database.GetUsers(orgID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get users"})
	}

	var userToUpdate database.User
	var found bool
	for _, u := range users {
		if u.UserID == id {
			userToUpdate = u
			found = true
			break
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	// 요청에 따라 사용자 정보를 업데이트합니다.
	userToUpdate.Role = req.Role
	userToUpdate.Password = req.Password // 비밀번호는 비어있을 수 있습니다. DB 계층에서 처리됩니다.
	if req.IsActive != nil {
		userToUpdate.IsActive = *req.IsActive
	}

	updatedUser, err := database.UpdateUser(userToUpdate)
	if err != nil {
		log.Printf("Error updating user: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update user"})
	}

	return c.JSON(updatedUser)
}

// DeleteUserAPI는 현재 조직의 사용자를 삭제합니다.
func DeleteUserAPI(c *fiber.Ctx) error {
	orgID, err := middleware.GetOrgID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: " + err.Error()})
	}

	id := c.Params("id")
	err = database.DeleteUser(id, orgID)
	if err != nil {
		log.Printf("Error deleting user: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete user"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// GetUserProfileHandler는 현재 사용자의 프로필 정보를 반환합니다.
func GetUserProfileHandler(c *fiber.Ctx, store *session.Store) error {
	// 인증 확인
	userID, err := middleware.GetUserID(c, store)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Authentication required",
		})
	}

	// 사용자 정보 조회
	var user struct {
		UserID    string `json:"user_id"`
		Username  string `json:"username"`
		Role      string `json:"role"`
		IsActive  bool   `json:"is_active"`
		CreatedAt string `json:"created_at"`
	}

	err = database.DB.QueryRow(`
		SELECT user_id, username, role, is_active, created_at 
		FROM users 
		WHERE user_id = $1
	`, userID).Scan(&user.UserID, &user.Username, &user.Role, &user.IsActive, &user.CreatedAt)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve user profile",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"user":    user,
	})
}
