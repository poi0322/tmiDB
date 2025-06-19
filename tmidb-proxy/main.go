package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/websocket/v2"
	_ "github.com/lib/pq"
)

type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

type QueryParser struct{}	// 클라이언트 쿼리를 파싱하여 데이터베이스 함수에 전달할 형태로 변환
func (qp *QueryParser) ParseQueryParams(queryParams url.Values) ([]Filter, error) {
	var filters []Filter
	
	for key, values := range queryParams {
		if len(values) == 0 {
			continue
		}
		
		value := values[0]
		
		// 다양한 연산자 패턴 매칭
		operators := []struct {
			pattern string
			op      string
		}{
			{`^(.+)>=(.+)$`, ">="},
			{`^(.+)<=(.+)$`, "<="},
			{`^(.+)>(.+)$`, ">"},
			{`^(.+)<(.+)$`, "<"},
			{`^(.+)!=(.+)$`, "!="},
			{`^(.+)~(.+)$`, "~"},      // LIKE 검색
			{`^(.+)!~(.+)$`, "!~"},    // NOT LIKE 검색
			{`^(.+)!in(.+)$`, "!in"},  // NOT IN 연산자
		}
		
		matched := false
		for _, op := range operators {
			re := regexp.MustCompile(op.pattern)
			if matches := re.FindStringSubmatch(key + value); len(matches) == 3 {
				filters = append(filters, Filter{
					Field: strings.TrimSpace(matches[1]),
					Op:    op.op,
					Value: strings.TrimSpace(matches[2]),
				})
				matched = true
				break
			}
		}
		
		// 연산자가 없으면 특수 케이스 및 기본 처리
		if !matched {
			// 배열 관련 연산자들
			if strings.HasSuffix(key, "[]contains") {
				fieldName := strings.TrimSuffix(key, "[]contains")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "array_includes",
					Value: value,
				})
			} else if strings.HasSuffix(key, "[]!contains") {
				fieldName := strings.TrimSuffix(key, "[]!contains")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "!array_includes",
					Value: value,
				})
			} else if strings.HasSuffix(key, "[]includes_any") {
				fieldName := strings.TrimSuffix(key, "[]includes_any")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "array_includes_any",
					Value: value,
				})
			} else if strings.HasSuffix(key, "[]includes_all") {
				fieldName := strings.TrimSuffix(key, "[]includes_all")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "array_includes_all",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size") {
				fieldName := strings.TrimSuffix(key, ".size")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size>") {
				fieldName := strings.TrimSuffix(key, ".size>")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size>",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size>=") {
				fieldName := strings.TrimSuffix(key, ".size>=")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size>=",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size<") {
				fieldName := strings.TrimSuffix(key, ".size<")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size<",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size<=") {
				fieldName := strings.TrimSuffix(key, ".size<=")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size<=",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".exists") {
				fieldName := strings.TrimSuffix(key, ".exists")
				if value == "true" || value == "1" {
					filters = append(filters, Filter{
						Field: fieldName,
						Op:    "exists",
						Value: "true",
					})
				} else {
					filters = append(filters, Filter{
						Field: fieldName,
						Op:    "!exists",
						Value: "true",
					})
				}
			} else if strings.HasSuffix(key, ".empty") {
				fieldName := strings.TrimSuffix(key, ".empty")
				if value == "true" || value == "1" {
					filters = append(filters, Filter{
						Field: fieldName,
						Op:    "empty",
						Value: "true",
					})
				} else {
					filters = append(filters, Filter{
						Field: fieldName,
						Op:    "!empty",
						Value: "true",
					})
				}
			} else if strings.HasSuffix(key, ".like") {
				fieldName := strings.TrimSuffix(key, ".like")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "like",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".regex") {
				fieldName := strings.TrimSuffix(key, ".regex")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "regex",
					Value: value,
				})
			} else if strings.Contains(value, ",") {
				// 쉼표가 포함된 값은 IN 연산자로 처리
				filters = append(filters, Filter{
					Field: key,
					Op:    "in",
					Value: value,
				})
			} else {
				// 기본 등호 연산자
				filters = append(filters, Filter{
					Field: key,
					Op:    "=",
					Value: value,
				})
			}
		}
	}
	
	return filters, nil
}

// 다중 리스너 API 파싱을 위한 함수
func parseMultiListenerPath(path string) []string {
	// "/api/v1/listener/server_monitor/sensor_broken/air_sensor" -> ["server_monitor", "sensor_broken", "air_sensor"]
	parts := strings.Split(path, "/")
	if len(parts) < 5 || parts[1] != "api" || parts[3] != "listener" {
		return nil
	}
	return parts[4:] // listener 이후의 모든 부분
}

func main() {
	// 데이터베이스 연결
	dbURL := fmt.Sprintf("postgres://%s:%s@db:5432/%s?sslmode=disable",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_DB"))
	
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()
	
	app := fiber.New(fiber.Config{
		AppName: "tmiDB Proxy v1.0",
	})
	
	// 미들웨어
	app.Use(logger.New())
	app.Use(cors.New())
	
	parser := &QueryParser{}
	
	// WebSocket 엔드포인트 먼저 등록 (라우팅 우선순위 때문)
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	
	// WebSocket: /ws/v{version}/{category}[/{target_id}]
	app.Get("/ws/v:version/:category", websocket.New(func(c *websocket.Conn) {
		// TODO: 실시간 스트리밍 구현
		defer c.Close()
		
		version := c.Params("version")
		category := c.Params("category")
		
		log.Printf("WebSocket connected: v%s/%s", version, category)
		
		for {
			messageType, msg, err := c.ReadMessage()
			if err != nil {
				log.Println("WebSocket read error:", err)
				break
			}
			
			// Echo back for now
			if err := c.WriteMessage(messageType, msg); err != nil {
				log.Println("WebSocket write error:", err)
				break
			}
		}
	}))
	
	// API 라우팅: GET /v{version}/{category}[/{target_id}]
	app.Get("/v:version/:category", func(c *fiber.Ctx) error {
		version := c.Params("version")
		category := c.Params("category")
		
		// 버전은 현재 1만 지원
		if version != "1" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Only version 1 is supported",
			})
		}
		
		// 쿼리 파라미터 파싱
		queryParams := make(url.Values)
		c.Context().QueryArgs().VisitAll(func(key, value []byte) {
			queryParams.Add(string(key), string(value))
		})
		
		filters, err := parser.ParseQueryParams(queryParams)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid query parameters",
			})
		}
			// JSON으로 변환하여 데이터베이스 함수에 전달
		var filtersParam interface{}
		if len(filters) == 0 {
			filtersParam = nil // NULL로 전달
		} else {
			filtersJSON, err := json.Marshal(filters)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Failed to marshal filters",
				})
			}
			filtersParam = string(filtersJSON)
		}

		// 데이터베이스 쿼리 실행
		rows, err := db.Query(`
			SELECT target_id, target_name, category_data, updated_at 
			FROM get_category_targets_advanced($1, $2, $3)
		`, category, 1, filtersParam)
		
		if err != nil {
			log.Printf("Database query error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Database query failed",
			})
		}
		defer rows.Close()
		
		var results []map[string]interface{}
		for rows.Next() {
			var targetID, targetName, categoryData, updatedAt string
			err := rows.Scan(&targetID, &targetName, &categoryData, &updatedAt)
			if err != nil {
				continue
			}
			
			var data map[string]interface{}
			json.Unmarshal([]byte(categoryData), &data)
			
			// category_data를 평면화하여 병합
			result := map[string]interface{}{
				"target_id":   targetID,
				"target_name": targetName,
				"updated_at":  updatedAt,
			}
			
			// category_data의 모든 필드를 결과에 병합
			for key, value := range data {
				result[key] = value
			}
			
			results = append(results, result)
		}
		
		// 새로운 응답 형태
		response := map[string]interface{}{
			"responseTime": time.Now().Format("2006-01-02 15:04:05"),
		}
		
		response[category] = map[string]interface{}{
			"version": version,
			"data":    results,
		}
		
		return c.JSON(response)
	})
	
	// 카테고리 스키마 조회: GET /v{version}/{category}/schema (최우선 등록)
	app.Get("/v:version/:category/schema", func(c *fiber.Ctx) error {
		version := c.Params("version")
		category := c.Params("category")
		
		versionInt, err := strconv.Atoi(version)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid version",
			})
		}
		
		var schema string
		err = db.QueryRow(`
			SELECT get_category_schema($1, $2)
		`, category, versionInt).Scan(&schema)
		
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Schema not found",
			})
		}
		
		var schemaData map[string]interface{}
		json.Unmarshal([]byte(schema), &schemaData)
		
		return c.JSON(fiber.Map{
			"category": category,
			"version":  version,
			"schema":   schemaData,
		})
	})

	// UUID 패턴 검증을 위한 정규식
	uuidPattern := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	
	// 특정 대상 조회: GET /v{version}/{category}/{target_id} (UUID 패턴만 허용)
	app.Get("/v:version/:category/:target_id", func(c *fiber.Ctx) error {
		version := c.Params("version")
		category := c.Params("category")
		targetID := c.Params("target_id")
		
		// schema 엔드포인트와 구분하기 위해 UUID 형식만 허용
		if !uuidPattern.MatchString(targetID) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid target_id format. Must be a valid UUID.",
			})
		}
		
		if version != "1" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Only version 1 is supported",
			})
		}
		
		// target_id로 직접 조회
		var targetName, categoryData, updatedAt string
		err := db.QueryRow(`
			SELECT t.name, tc.category_data, tc.updated_at
			FROM target_categories tc
			JOIN target t ON tc.target_id = t.target_id
			WHERE t.target_id = $1 AND tc.category_name = $2
		`, targetID, category).Scan(&targetName, &categoryData, &updatedAt)
		
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Target not found",
			})
		}
		
		var data map[string]interface{}
		json.Unmarshal([]byte(categoryData), &data)
		
		return c.JSON(fiber.Map{
			"target_id":     targetID,
			"target_name":   targetName,
			"category":      category,
			"version":       version,
			"category_data": data,
			"updated_at":    updatedAt,
		})
	})
	
	// 다중 리스너 API: GET /api/v{version}/listener/{listener1}/{listener2}/...
	app.Get("/api/v:version/listener/*", func(c *fiber.Ctx) error {
		version := c.Params("version")
		
		if version != "1" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Only version 1 is supported",
			})
		}
		
		// URL 경로에서 리스너 ID 목록 추출
		listenerIDs := parseMultiListenerPath(c.Path())
		if len(listenerIDs) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid listener path",
			})
		}
		
		// 클라이언트의 쿼리 파라미터 파싱
		queryParams := make(url.Values)
		c.Context().QueryArgs().VisitAll(func(key, value []byte) {
			queryParams.Add(string(key), string(value))
		})
		
		filters, err := parser.ParseQueryParams(queryParams)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid query parameters",
			})
		}
		
		// 필터를 JSON으로 변환
		filtersJSON, _ := json.Marshal(filters)
		
		// PostgreSQL 배열 형식으로 변환
		listenerIDsStr := "{" + strings.Join(listenerIDs, ",") + "}"
		
		// 다중 리스너 데이터 조회
		var resultJSON string
		err = db.QueryRow(`
			SELECT get_multi_listener_data($1, $2, $3)
		`, listenerIDsStr, "v"+version, string(filtersJSON)).Scan(&resultJSON)
		
		if err != nil {
			log.Printf("Database query error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Database query failed",
			})
		}
		
		// JSON 파싱하여 응답
		var result map[string]interface{}
		json.Unmarshal([]byte(resultJSON), &result)
		
		return c.JSON(result)
	})
	
	// 단일 리스너 API (기존 호환성 유지): GET /api/v{version}/listener/{listener_id}
	app.Get("/api/v:version/listener/:listener_id", func(c *fiber.Ctx) error {
		version := c.Params("version")
		listenerID := c.Params("listener_id")
		
		if version != "1" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Only version 1 is supported",
			})
		}
		
		// 클라이언트의 쿼리 파라미터 파싱
		queryParams := make(url.Values)
		c.Context().QueryArgs().VisitAll(func(key, value []byte) {
			queryParams.Add(string(key), string(value))
		})
		
		filters, err := parser.ParseQueryParams(queryParams)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid query parameters",
			})
		}
		
		// 필터를 JSON으로 변환
		filtersJSON, _ := json.Marshal(filters)
		
		// 단일 리스너 데이터 조회
		rows, err := db.Query(`
			SELECT target_id, target_name, category_data, updated_at, category_name
			FROM get_listener_filtered_data($1, $2, $3)
		`, listenerID, "v"+version, string(filtersJSON))
		
		if err != nil {
			log.Printf("Database query error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Database query failed",
			})
		}
		defer rows.Close()
		
		var results []map[string]interface{}
		var categoryName string
		for rows.Next() {
			var targetID, targetName, categoryData, updatedAt, catName string
			err := rows.Scan(&targetID, &targetName, &categoryData, &updatedAt, &catName)
			if err != nil {
				continue
			}
			
			categoryName = catName // 카테고리 이름 저장
			
			var data map[string]interface{}
			json.Unmarshal([]byte(categoryData), &data)
			
			// category_data를 평면화하여 병합
			result := map[string]interface{}{
				"target_id":   targetID,
				"target_name": targetName,
				"updated_at":  updatedAt,
			}
			
			// category_data의 모든 필드를 결과에 병합
			for key, value := range data {
				result[key] = value
			}
			
			results = append(results, result)
		}
		
		// 새로운 응답 형태 - 리스너용
		response := map[string]interface{}{
			"responseTime": time.Now().Format("2006-01-02 15:04:05"),
		}
		
		// 리스너 ID로 감싸고, 그 안에 카테고리로 구조화
		listenerData := map[string]interface{}{}
		if categoryName != "" {
			listenerData[categoryName] = map[string]interface{}{
				"version": version,
				"data":    results,
			}
		}
		
		response[listenerID] = listenerData
		
		return c.JSON(response)
	})
	
	// Admin Panel - Custom Database Management Interface
	app.Get("/admin", func(c *fiber.Ctx) error {
		// Try to read admin.html file first, fallback to embedded HTML
		if htmlContent, err := os.ReadFile("admin.html"); err == nil {
			return c.Type("html").Send(htmlContent)
		}
		
		// Fallback to basic HTML if file not found
		return c.Type("html").SendString(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>tmiDB Admin Panel</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 { color: #333; }
        .error { background: #ff6b6b; color: white; padding: 10px; border-radius: 4px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🗄️ tmiDB Admin Panel</h1>
        <p>Admin HTML file not found. Please ensure admin.html is in the same directory as the binary.</p>
        <p><a href="/admin/api/stats">View Stats API</a> | <a href="/">API Info</a></p>
    </div>
</body>
</html>`)
	})

	// Admin API endpoints
	app.Get("/admin/api/stats", func(c *fiber.Ctx) error {
            backdrop-filter: blur(10px);
            border-radius: 12px;
            padding: 2rem;
            box-shadow: 0 4px 20px rgba(0,0,0,0.1);
        }
        
        .query-input {
            width: 100%;
            min-height: 120px;
            padding: 1rem;
            border: 2px solid #e2e8f0;
            border-radius: 8px;
            font-family: 'Monaco', 'Menlo', monospace;
            font-size: 0.9rem;
            resize: vertical;
            margin-bottom: 1rem;
        }
        
        .query-input:focus {
            outline: none;
            border-color: #667eea;
        }
        
        .btn {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            padding: 0.8rem 2rem;
            border-radius: 8px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s ease;
        }
        
        .btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 15px rgba(102, 126, 234, 0.4);
        }
        
        .results {
            margin-top: 2rem;
            background: #f7fafc;
            border-radius: 8px;
            padding: 1rem;
            display: none;
        }
        
        .results.show {
            display: block;
        }
        
        .results-table {
            width: 100%;
            border-collapse: collapse;
            background: white;
            border-radius: 8px;
            overflow: hidden;
        }
        
        .results-table th {
            background: #4a5568;
            color: white;
            padding: 1rem;
            text-align: left;
        }
        
        .results-table td {
            padding: 0.8rem 1rem;
            border-bottom: 1px solid #e2e8f0;
        }
        
        .results-table tr:hover {
            background: #f7fafc;
        }
        
        .error {
            background: #fed7d7;
            color: #c53030;
            padding: 1rem;
            border-radius: 8px;
            margin-top: 1rem;
        }
        
        .loading {
            text-align: center;
            padding: 2rem;
            color: #718096;
        }
        
        h2 {
            color: #2d3748;
            margin-bottom: 1rem;
            font-size: 1.5rem;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>🗄️ tmiDB Admin Panel</h1>
    </div>
    
    <div class="container">
        <div class="stats-grid" id="statsGrid">
            <div class="stat-card">
                <h3>Database Status</h3>
                <div class="value" id="dbStatus">Loading...</div>
            </div>
            <div class="stat-card">
                <h3>Total Tables</h3>
                <div class="value" id="tableCount">Loading...</div>
            </div>
            <div class="stat-card">
                <h3>Total Records</h3>
                <div class="value" id="recordCount">Loading...</div>
            </div>
            <div class="stat-card">
                <h3>Database Size</h3>
                <div class="value" id="dbSize">Loading...</div>
            </div>
        </div>
        
        <div class="tables-section">
            <h2>📊 Tables Overview</h2>
            <div class="table-list" id="tableList">
                <div class="loading">Loading tables...</div>
            </div>
        </div>
        
        <div class="query-section">
            <h2>🔍 Query Interface</h2>
            <textarea class="query-input" id="queryInput" placeholder="Enter your SQL query here...
Example: SELECT * FROM server WHERE cpu_cores > 4 LIMIT 10;"></textarea>
            <button class="btn" onclick="executeQuery()">Execute Query</button>
            
            <div class="results" id="results">
                <div id="resultsContent"></div>
            </div>
        </div>
    </div>
    
    <script>
        // Load dashboard data
        async function loadDashboard() {
            try {
                const [stats, tables] = await Promise.all([
                    fetch('/admin/api/stats').then(r => r.json()),
                    fetch('/admin/api/tables').then(r => r.json())
                ]);
                
                // Update stats
                document.getElementById('dbStatus').textContent = stats.status || 'Connected';
                document.getElementById('tableCount').textContent = stats.table_count || '0';
                document.getElementById('recordCount').textContent = stats.total_records || '0';
                document.getElementById('dbSize').textContent = stats.database_size || 'N/A';
                
                // Update tables list
                const tableList = document.getElementById('tableList');
                if (tables && tables.length > 0) {
                    tableList.innerHTML = tables.map(table => 
                        '<div class="table-item" onclick="showTableData(\'' + table.name + '\')">' +
                        '<h4>' + table.name + '</h4>' +
                        '<div class="row-count">' + (table.row_count || '0') + ' rows</div>' +
                        '</div>'
                    ).join('');
                } else {
                    tableList.innerHTML = '<div class="loading">No tables found</div>';
                }
            } catch (error) {
                console.error('Failed to load dashboard:', error);
                document.getElementById('dbStatus').textContent = 'Error';
            }
        }
        
        // Execute custom query
        async function executeQuery() {
            const query = document.getElementById('queryInput').value.trim();
            if (!query) return;
            
            const results = document.getElementById('results');
            const content = document.getElementById('resultsContent');
            
            results.classList.add('show');
            content.innerHTML = '<div class="loading">Executing query...</div>';
            
            try {
                const response = await fetch('/admin/api/query', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ query: query })
                });
                
                const data = await response.json();
                
                if (!response.ok) {
                    throw new Error(data.error || 'Query failed');
                }
                
                if (data.rows && data.rows.length > 0) {
                    const columns = data.columns || Object.keys(data.rows[0]);
                    let html = '<table class="results-table"><thead><tr>';
                    
                    columns.forEach(col => {
                        html += '<th>' + escapeHtml(col) + '</th>';
                    });
                    
                    html += '</tr></thead><tbody>';
                    
                    data.rows.forEach(row => {
                        html += '<tr>';
                        columns.forEach(col => {
                            const value = row[col];
                            html += '<td>' + escapeHtml(value != null ? String(value) : '') + '</td>';
                        });
                        html += '</tr>';
                    });
                    
                    html += '</tbody></table>';
                    content.innerHTML = html;
                } else {
                    content.innerHTML = '<div class="loading">Query executed successfully. No results returned.</div>';
                }
            } catch (error) {
                content.innerHTML = '<div class="error">Error: ' + escapeHtml(error.message) + '</div>';
            }
        }
        
        // Show table data
        async function showTableData(tableName) {
            document.getElementById('queryInput').value = 'SELECT * FROM ' + tableName + ' LIMIT 100;';
            executeQuery();
        }
        
        // Utility function to escape HTML
        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
        
        // Load dashboard on page load
        loadDashboard();
        
        // Auto-refresh every 30 seconds
        setInterval(loadDashboard, 30000);
    </script>
</body>
</html>
		`)
	})

	// Admin API endpoints
	app.Get("/admin/api/stats", func(c *fiber.Ctx) error {
		// Use the same database connection as the main application
		var tableCount int
		err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&tableCount)
		if err != nil {
			tableCount = 0
		}

		// Get total records (sum of all table row counts)
		var totalRecords int
		rows, err := db.Query(`
			SELECT COALESCE(SUM(n_tup_ins - n_tup_del), 0) as total_records
			FROM pg_stat_user_tables
		`)
		if err == nil {
			defer rows.Close()
			if rows.Next() {
				rows.Scan(&totalRecords)
			}
		}

		// Get database size
		var dbSize string
		err = db.QueryRow("SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&dbSize)
		if err != nil {
			dbSize = "N/A"
		}

		return c.JSON(fiber.Map{
			"status": "Connected",
			"table_count": tableCount,
			"total_records": totalRecords,
			"database_size": dbSize,
		})
	})

	app.Get("/admin/api/tables", func(c *fiber.Ctx) error {
		// Use the same database connection as the main application
		rows, err := db.Query(`
			SELECT t.table_name, COALESCE(s.n_tup_ins - s.n_tup_del, 0) as row_count
			FROM information_schema.tables t
			LEFT JOIN pg_stat_user_tables s ON t.table_name = s.relname
			WHERE t.table_schema = 'public'
			ORDER BY t.table_name
		`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to fetch tables",
				"details": err.Error(),
			})
		}
		defer rows.Close()

		var tables []map[string]interface{}
		for rows.Next() {
			var tableName string
			var rowCount int
			if err := rows.Scan(&tableName, &rowCount); err != nil {
				continue
			}
			tables = append(tables, map[string]interface{}{
				"name": tableName,
				"row_count": rowCount,
			})
		}

		return c.JSON(tables)
	})

	app.Post("/admin/api/query", func(c *fiber.Ctx) error {
		var request struct {
			Query string `json:"query"`
		}
		
		if err := c.BodyParser(&request); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		if request.Query == "" {
			return c.Status(400).JSON(fiber.Map{
				"error": "Query is required",
			})
		}

		// Use the same database connection as the main application
		rows, err := db.Query(request.Query)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Query execution failed",
				"details": err.Error(),
			})
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to get columns",
				"details": err.Error(),
			})
		}

		var results []map[string]interface{}
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to scan row",
					"details": err.Error(),
				})
			}

			row := make(map[string]interface{})
			for i, column := range columns {
				if values[i] != nil {
					if b, ok := values[i].([]byte); ok {
						row[column] = string(b)
					} else {
						row[column] = values[i]
					}
				} else {
					row[column] = nil
				}
			}
			results = append(results, row)
		}

		return c.JSON(fiber.Map{
			"columns": columns,
			"rows": results,
		})
	})
	
	// Root endpoint - API Info
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"name": "tmiDB Proxy",
			"version": "1.0.0",
			"description": "Multi-category data management system with listener-based subscriptions",
			"endpoints": map[string]interface{}{
				"data_query": map[string]string{
					"url": "/v1/{category}[?filters]",
					"description": "Query category data with advanced filtering",
					"example": "/v1/server?cpu_cores>=4&status!=offline",
				},
				"schema": map[string]string{
					"url": "/v1/{category}/schema",
					"description": "Get category schema definition",
					"example": "/v1/server/schema",
				},
				"single_item": map[string]string{
					"url": "/v1/{category}/{target_id}",
					"description": "Get specific target by UUID",
					"example": "/v1/server/550e8400-e29b-41d4-a716-446655440000",
				},
				"listener": map[string]string{
					"url": "/api/v1/listener/{listener_id}[?filters]",
					"description": "Get data via listener subscription",
					"example": "/api/v1/listener/server_monitor?cpu_cores>=4",
				},
				"multi_listener": map[string]string{
					"url": "/api/v1/listener/{listener_id1}/{listener_id2}/.../{listener_idN}",
					"description": "Get data from multiple listeners",
					"example": "/api/v1/listener/server_monitor/sensor_alerts",
				},
				"websocket": map[string]string{
					"url": "/ws/v1/{category}",
					"description": "Real-time updates for category",
					"example": "ws://localhost:8080/ws/v1/server",
				},
				"admin_panel": map[string]string{
					"url": "/admin",
					"description": "Database administration interface",
					"example": "/admin",
				},
			},
			"filtering": map[string]interface{}{
				"operators": []string{"=", "!=", ">", ">=", "<", "<=", "~", "!~", "like", "regex", "in", "!in", "contains", "exists", "empty"},
				"examples": []string{
					"?status=online",
					"?cpu_cores>=4",
					"?hostname~web",
					"?tags[]contains=production",
					"?status.in=online,maintenance",
				},
			},
			"categories": []string{"server", "sensor"},
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	
	log.Printf("tmiDB Proxy starting on port %s", port)
	log.Printf("Example endpoints:")
	log.Printf("  GET /v1/server?cpu_cores>=4&status!=offline")
	log.Printf("  GET /v1/sensor?sensor_type=temperature")
	log.Printf("  GET /v1/server/{target_id}")
	log.Printf("  GET /v1/server/schema")
	log.Printf("  GET /api/v1/listener/server_monitor?cpu_cores>=4")
	log.Printf("  GET /api/v1/listener/server_monitor/sensor_broken/air_sensor")
	log.Printf("  WS  /ws/v1/server")
	log.Printf("  WS  /ws/api/v1/listener/server_monitor")
	log.Printf("  GET /admin - Database Admin Panel")
	log.Printf("  GET / - API Information")
	
	log.Fatal(app.Listen(":" + port))
}
