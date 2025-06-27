package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
)

func main() {
	log.Println("🌐 Starting tmiDB API Server...")

	// 웹 콘솔 템플릿 엔진 초기화
	engine := html.New("./cmd/api/views", ".html")

	// Fiber 앱 생성
	app := fiber.New(fiber.Config{
		Views: engine,
	})

	// 미들웨어 설정
	app.Use(cors.New())
	app.Use(logger.New())

	// 정적 파일 서빙 (CSS, JS, 이미지 등)
	app.Static("/static", "./cmd/api/static")

	// 웹 콘솔 라우트 설정
	setupWebConsoleRoutes(app)

	// API 라우트 설정
	setupAPIRoutes(app)

	// 서버 시작
	go func() {
		log.Println("🌐 API Server listening on :8020")
		if err := app.Listen(":8020"); err != nil {
			log.Fatalf("❌ Failed to start server: %v", err)
		}
	}()

	// 종료 시그널 대기
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down API Server...")

	// 서버 종료
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("❌ Server forced to shutdown: %v", err)
	}

	log.Println("✅ API Server stopped")
}

// setupWebConsoleRoutes 웹 콘솔 라우트를 설정합니다
func setupWebConsoleRoutes(app *fiber.App) {
	// 메인 대시보드
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Render("main", fiber.Map{
			"Title": "tmiDB Console",
		})
	})

	// 관리자 대시보드
	app.Get("/admin", func(c *fiber.Ctx) error {
		return c.Render("admin/dashboard", fiber.Map{
			"Title": "Admin Dashboard",
		})
	})

	// 데이터 탐색기
	app.Get("/admin/data-explorer", func(c *fiber.Ctx) error {
		return c.Render("admin/data_explorer", fiber.Map{
			"Title": "Data Explorer",
		})
	})

	// 사용자 관리
	app.Get("/admin/users", func(c *fiber.Ctx) error {
		return c.Render("admin/users", fiber.Map{
			"Title": "User Management",
		})
	})

	// 카테고리 관리
	app.Get("/admin/categories", func(c *fiber.Ctx) error {
		return c.Render("admin/categories", fiber.Map{
			"Title": "Category Management",
		})
	})

	// 리스너 관리
	app.Get("/admin/listeners", func(c *fiber.Ctx) error {
		return c.Render("admin/listeners", fiber.Map{
			"Title": "Listener Management",
		})
	})

	// 토큰 관리
	app.Get("/admin/tokens", func(c *fiber.Ctx) error {
		return c.Render("admin/tokens", fiber.Map{
			"Title": "Token Management",
		})
	})

	// 로그인 페이지
	app.Get("/login", func(c *fiber.Ctx) error {
		return c.Render("login", fiber.Map{
			"Title": "Login",
		})
	})

	// 설정 페이지
	app.Get("/setup", func(c *fiber.Ctx) error {
		return c.Render("setup", fiber.Map{
			"Title": "Setup",
		})
	})
}

// setupAPIRoutes API 라우트를 설정합니다
func setupAPIRoutes(app *fiber.App) {
	api := app.Group("/api")

	// 헬스체크
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":    "healthy",
			"timestamp": time.Now(),
			"service":   "tmidb-api",
			"version":   "1.0.0",
		})
	})

	// 시스템 정보
	api.Get("/system/info", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"system": fiber.Map{
				"uptime":     time.Since(time.Now()).String(),
				"components": []string{"api", "data-manager", "data-consumer"},
				"external":   []string{"postgresql", "nats", "seaweedfs"},
			},
		})
	})

	// 데이터 쿼리 API
	api.Get("/data/query", handleDataQuery)
	api.Post("/data/query", handleDataQuery)

	// 데이터 통계 API
	api.Get("/data/stats", handleDataStats)

	// 실시간 데이터 스트림 API
	api.Get("/data/stream", handleDataStream)

	// 관리자 API
	admin := api.Group("/admin")
	admin.Get("/users", handleAdminUsers)
	admin.Post("/users", handleAdminCreateUser)
	admin.Get("/categories", handleAdminCategories)
	admin.Post("/categories", handleAdminCreateCategory)
	admin.Get("/listeners", handleAdminListeners)
	admin.Post("/listeners", handleAdminCreateListener)
	admin.Get("/tokens", handleAdminTokens)
	admin.Post("/tokens", handleAdminCreateToken)
}

// handleDataQuery 데이터 쿼리 요청을 처리합니다
func handleDataQuery(c *fiber.Ctx) error {
	// TODO: 데이터베이스 연결 및 쿼리 실행
	return c.JSON(fiber.Map{
		"status": "success",
		"data":   []interface{}{},
		"count":  0,
	})
}

// handleDataStats 데이터 통계 요청을 처리합니다
func handleDataStats(c *fiber.Ctx) error {
	// TODO: 데이터베이스에서 통계 정보 조회
	return c.JSON(fiber.Map{
		"total_records": 0,
		"categories":    0,
		"sources":       0,
		"last_updated":  time.Now(),
	})
}

// handleDataStream 실시간 데이터 스트림 요청을 처리합니다
func handleDataStream(c *fiber.Ctx) error {
	// TODO: WebSocket 또는 Server-Sent Events 구현
	return c.JSON(fiber.Map{
		"status":  "not_implemented",
		"message": "Real-time data streaming will be implemented",
	})
}

// handleAdminUsers 사용자 관리 API
func handleAdminUsers(c *fiber.Ctx) error {
	// TODO: 사용자 목록 조회
	return c.JSON(fiber.Map{
		"users": []interface{}{},
	})
}

// handleAdminCreateUser 사용자 생성 API
func handleAdminCreateUser(c *fiber.Ctx) error {
	// TODO: 사용자 생성 로직
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "User created successfully",
	})
}

// handleAdminCategories 카테고리 관리 API
func handleAdminCategories(c *fiber.Ctx) error {
	// TODO: 카테고리 목록 조회
	return c.JSON(fiber.Map{
		"categories": []interface{}{},
	})
}

// handleAdminCreateCategory 카테고리 생성 API
func handleAdminCreateCategory(c *fiber.Ctx) error {
	// TODO: 카테고리 생성 로직
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Category created successfully",
	})
}

// handleAdminListeners 리스너 관리 API
func handleAdminListeners(c *fiber.Ctx) error {
	// TODO: 리스너 목록 조회
	return c.JSON(fiber.Map{
		"listeners": []interface{}{},
	})
}

// handleAdminCreateListener 리스너 생성 API
func handleAdminCreateListener(c *fiber.Ctx) error {
	// TODO: 리스너 생성 로직
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Listener created successfully",
	})
}

// handleAdminTokens 토큰 관리 API
func handleAdminTokens(c *fiber.Ctx) error {
	// TODO: 토큰 목록 조회
	return c.JSON(fiber.Map{
		"tokens": []interface{}{},
	})
}

// handleAdminCreateToken 토큰 생성 API
func handleAdminCreateToken(c *fiber.Ctx) error {
	// TODO: 토큰 생성 로직
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Token created successfully",
	})
}
