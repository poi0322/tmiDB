package handlers

import (
	"log"

	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
)

// DashboardPage는 대시보드를 렌더링합니다.
func DashboardPage(c *fiber.Ctx) error {
	var tableCount, totalRecords int
	var dbSize, status string

	// DB 상태 확인
	if err := database.CheckDatabaseHealth(); err != nil {
		status = "Connection Failed"
		log.Printf("Database health check failed: %v", err)
	} else {
		status = "Connected"
	}

	// DB 통계
	err := database.DB.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&tableCount)
	if err != nil {
		log.Printf("Failed to get table count: %v", err)
	}

	rows, err := database.DB.Query(`SELECT COALESCE(SUM(n_tup_ins - n_tup_del), 0) as total_records FROM pg_stat_user_tables`)
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			rows.Scan(&totalRecords)
		}
	} else {
		log.Printf("Failed to get total records: %v", err)
	}

	err = database.DB.QueryRow("SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&dbSize)
	if err != nil {
		dbSize = "N/A"
		log.Printf("Failed to get db size: %v", err)
	}

	// 최근 사용자 목록
	recentUsers, err := getRecentUsers()
	if err != nil {
		log.Printf("Failed to get recent users: %v", err)
	}

	// 최근 토큰 목록
	recentTokens, err := getRecentTokens()
	if err != nil {
		log.Printf("Failed to get recent tokens: %v", err)
	}

	// 사용자 수와 토큰 수 계산
	var userCount, tokenCount int
	database.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	database.DB.QueryRow("SELECT COUNT(*) FROM auth_tokens").Scan(&tokenCount)

	return c.Render("admin/dashboard.html", fiber.Map{
		"title": "Dashboard",
		"stats": fiber.Map{
			"status":        status,
			"database_size": dbSize,
			"table_count":   tableCount,
			"total_records": totalRecords,
		},
		"recent_users":  recentUsers,
		"recent_tokens": recentTokens,
		"user_count":    userCount,
		"token_count":   tokenCount,
	}, "main.html")
}

func getRecentUsers() ([]fiber.Map, error) {
	rows, err := database.DB.Query(`
		SELECT username, role, created_at FROM users ORDER BY created_at DESC LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []fiber.Map
	for rows.Next() {
		var username, role string
		var createdAt interface{}
		if err := rows.Scan(&username, &role, &createdAt); err != nil {
			continue
		}
		users = append(users, fiber.Map{
			"username":   username,
			"role":       role,
			"created_at": createdAt,
		})
	}
	return users, nil
}

func getRecentTokens() ([]fiber.Map, error) {
	rows, err := database.DB.Query(`
		SELECT description, is_admin, created_at FROM auth_tokens ORDER BY created_at DESC LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []fiber.Map
	for rows.Next() {
		var description string
		var isAdmin bool
		var createdAt interface{}
		if err := rows.Scan(&description, &isAdmin, &createdAt); err != nil {
			continue
		}
		tokens = append(tokens, fiber.Map{
			"description": description,
			"is_admin":    isAdmin,
			"created_at":  createdAt,
		})
	}
	return tokens, nil
}
