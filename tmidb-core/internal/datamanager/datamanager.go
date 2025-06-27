package datamanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/tmidb/tmidb-core/internal/database"
)

// DataManager 데이터 수집 및 데이터베이스 관리를 담당하는 구조체
type DataManager struct {
	natsConn *nats.Conn
	db       database.DBTX
	subs     []*nats.Subscription
}

// DataPoint 수집되는 데이터 포인트 구조체
type DataPoint struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`
	Category  string                 `json:"category"`
	Data      map[string]interface{} `json:"data"`
}

// New DataManager 인스턴스를 생성합니다
func New() *DataManager {
	return &DataManager{}
}

// Start DataManager를 시작합니다
func (dm *DataManager) Start(ctx context.Context) error {
	log.Println("📊 Initializing Data Manager...")

	// 데이터베이스 연결
	if err := dm.connectDatabase(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// NATS 연결
	if err := dm.connectNATS(); err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// 데이터 구독 시작
	if err := dm.startSubscriptions(); err != nil {
		return fmt.Errorf("failed to start subscriptions: %w", err)
	}

	// 데이터 수집 프로세스 시작
	go dm.startDataCollection(ctx)

	// 배치 처리 시작
	go dm.startBatchProcessor(ctx)

	log.Println("✅ Data Manager started successfully")

	// 컨텍스트 완료까지 대기
	<-ctx.Done()

	// 정리 작업
	dm.cleanup()

	return nil
}

// connectDatabase 데이터베이스에 연결합니다
func (dm *DataManager) connectDatabase() error {
	for i := 0; i < 15; i++ {
		if err := database.CheckDatabaseHealth(); err == nil {
			dm.db = database.DB
			log.Println("✅ Data Manager connected to database")
			return nil
		}
		log.Printf("⏳ Data Manager waiting for database... (attempt %d/15)", i+1)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("failed to connect to database after 15 attempts")
}

// connectNATS NATS 서버에 연결합니다
func (dm *DataManager) connectNATS() error {
	var err error
	for i := 0; i < 10; i++ {
		dm.natsConn, err = nats.Connect("nats://localhost:4222")
		if err == nil {
			log.Println("✅ Data Manager connected to NATS server")
			return nil
		}
		log.Printf("⏳ Data Manager waiting for NATS server... (attempt %d/10)", i+1)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("failed to connect to NATS after 10 attempts: %w", err)
}

// startSubscriptions 데이터 구독을 시작합니다
func (dm *DataManager) startSubscriptions() error {
	// 모든 데이터 스트림 구독
	sub1, err := dm.natsConn.Subscribe("tmidb.data.>", dm.handleDataMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to data stream: %w", err)
	}
	dm.subs = append(dm.subs, sub1)

	// 시스템 메트릭 구독
	sub2, err := dm.natsConn.Subscribe("tmidb.data.system.>", dm.handleSystemMetrics)
	if err != nil {
		return fmt.Errorf("failed to subscribe to system metrics: %w", err)
	}
	dm.subs = append(dm.subs, sub2)

	log.Println("📡 Data Manager started NATS subscriptions")
	return nil
}

// handleDataMessage 일반 데이터 메시지를 처리합니다
func (dm *DataManager) handleDataMessage(msg *nats.Msg) {
	var dataPoint DataPoint
	if err := json.Unmarshal(msg.Data, &dataPoint); err != nil {
		log.Printf("❌ Failed to unmarshal data message: %v", err)
		return
	}

	log.Printf("📨 Data Manager received data: %s from %s.%s", dataPoint.ID, dataPoint.Source, dataPoint.Category)

	// 데이터베이스에 저장
	if err := dm.saveToDatabase(dataPoint); err != nil {
		log.Printf("❌ Failed to save data to database: %v", err)
		return
	}

	log.Printf("💾 Data Manager saved data: %s", dataPoint.ID)
}

// handleSystemMetrics 시스템 메트릭을 처리합니다
func (dm *DataManager) handleSystemMetrics(msg *nats.Msg) {
	var dataPoint DataPoint
	if err := json.Unmarshal(msg.Data, &dataPoint); err != nil {
		log.Printf("❌ Failed to unmarshal system metrics: %v", err)
		return
	}

	log.Printf("📊 Data Manager processing system metrics: %s", dataPoint.ID)

	// 시스템 메트릭 특별 처리
	if err := dm.processSystemMetrics(dataPoint); err != nil {
		log.Printf("❌ Failed to process system metrics: %v", err)
		return
	}

	// 데이터베이스에 저장
	if err := dm.saveToDatabase(dataPoint); err != nil {
		log.Printf("❌ Failed to save system metrics: %v", err)
		return
	}

	log.Printf("📈 Data Manager processed and saved system metrics: %s", dataPoint.ID)
}

// processSystemMetrics 시스템 메트릭을 특별 처리합니다
func (dm *DataManager) processSystemMetrics(dataPoint DataPoint) error {
	// CPU 사용률이 90% 이상인 경우 알림
	if cpuUsage, ok := dataPoint.Data["cpu_usage"].(float64); ok && cpuUsage > 90.0 {
		log.Printf("⚠️ HIGH CPU USAGE ALERT: %.1f%%", cpuUsage)
	}

	// 메모리 사용률이 85% 이상인 경우 알림
	if memUsage, ok := dataPoint.Data["memory_usage"].(float64); ok && memUsage > 85.0 {
		log.Printf("⚠️ HIGH MEMORY USAGE ALERT: %.1f%%", memUsage)
	}

	return nil
}

// saveToDatabase 데이터를 데이터베이스에 저장합니다
func (dm *DataManager) saveToDatabase(dataPoint DataPoint) error {
	if dm.db == nil {
		return fmt.Errorf("database connection not available")
	}

	// JSON 데이터를 문자열로 변환
	dataJSON, err := json.Marshal(dataPoint.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal data JSON: %w", err)
	}

	// ts_obs 테이블에 저장 (시계열 데이터)
	query := `
		INSERT INTO ts_obs (id, timestamp, source, category, data) 
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			timestamp = EXCLUDED.timestamp,
			source = EXCLUDED.source,
			category = EXCLUDED.category,
			data = EXCLUDED.data
	`

	_, err = dm.db.Exec(query, dataPoint.ID, dataPoint.Timestamp,
		dataPoint.Source, dataPoint.Category, string(dataJSON))
	if err != nil {
		return fmt.Errorf("failed to insert data into database: %w", err)
	}

	return nil
}

// startDataCollection 주기적인 데이터 수집을 시작합니다
func (dm *DataManager) startDataCollection(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("🔄 Data Manager starting periodic data collection...")

	for {
		select {
		case <-ticker.C:
			dm.collectSystemMetrics()
		case <-ctx.Done():
			log.Println("🛑 Data Manager stopping data collection...")
			return
		}
	}
}

// collectSystemMetrics 시스템 메트릭을 수집합니다
func (dm *DataManager) collectSystemMetrics() {
	dataPoint := DataPoint{
		ID:        fmt.Sprintf("system-metrics-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Source:    "system",
		Category:  "metrics",
		Data: map[string]interface{}{
			"cpu_usage":    85.5,
			"memory_usage": 67.2,
			"disk_usage":   45.8,
			"network_io":   1024.0,
		},
	}

	if err := dm.publishData(dataPoint); err != nil {
		log.Printf("❌ Failed to publish system metrics: %v", err)
	} else {
		log.Printf("📤 Data Manager published system metrics: %s", dataPoint.ID)
	}
}

// publishData 데이터를 NATS로 발행합니다
func (dm *DataManager) publishData(dataPoint DataPoint) error {
	if dm.natsConn == nil {
		return fmt.Errorf("NATS connection not available")
	}

	data, err := json.Marshal(dataPoint)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	subject := fmt.Sprintf("tmidb.data.%s.%s", dataPoint.Source, dataPoint.Category)
	return dm.natsConn.Publish(subject, data)
}

// startBatchProcessor 배치 처리를 시작합니다
func (dm *DataManager) startBatchProcessor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	log.Println("🔄 Data Manager starting batch processor...")

	for {
		select {
		case <-ticker.C:
			dm.processBatch()
		case <-ctx.Done():
			log.Println("🛑 Data Manager stopping batch processor...")
			return
		}
	}
}

// processBatch 배치 처리를 수행합니다
func (dm *DataManager) processBatch() {
	log.Println("🔄 Data Manager running batch processing...")

	// 데이터 집계 작업
	if err := dm.aggregateData(); err != nil {
		log.Printf("❌ Failed to aggregate data: %v", err)
	}

	// 오래된 데이터 정리
	if err := dm.cleanupOldData(); err != nil {
		log.Printf("❌ Failed to cleanup old data: %v", err)
	}

	log.Println("✅ Data Manager batch processing completed")
}

// aggregateData 데이터 집계를 수행합니다
func (dm *DataManager) aggregateData() error {
	if dm.db == nil {
		return fmt.Errorf("database connection not available")
	}

	// 시간별 평균 계산 (예시)
	query := `
		INSERT INTO hourly_aggregates (hour, source, category, avg_value, count)
		SELECT 
			date_trunc('hour', timestamp) as hour,
			source,
			category,
			AVG((data->>'cpu_usage')::numeric) as avg_value,
			COUNT(*) as count
		FROM ts_obs 
		WHERE timestamp >= NOW() - INTERVAL '1 hour'
		  AND data->>'cpu_usage' IS NOT NULL
		GROUP BY date_trunc('hour', timestamp), source, category
		ON CONFLICT (hour, source, category) DO UPDATE SET
			avg_value = EXCLUDED.avg_value,
			count = EXCLUDED.count
	`

	_, err := dm.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to aggregate data: %w", err)
	}

	log.Println("📊 Data Manager data aggregation completed")
	return nil
}

// cleanupOldData 오래된 데이터를 정리합니다
func (dm *DataManager) cleanupOldData() error {
	if dm.db == nil {
		return fmt.Errorf("database connection not available")
	}

	// 30일 이상된 원시 데이터 삭제
	query := `DELETE FROM ts_obs WHERE timestamp < NOW() - INTERVAL '30 days'`

	result, err := dm.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to cleanup old data: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("🧹 Data Manager cleaned up %d old records", rowsAffected)
	}

	return nil
}

// cleanup 리소스를 정리합니다
func (dm *DataManager) cleanup() {
	log.Println("🧹 Cleaning up Data Manager...")

	// NATS 구독 해제
	for _, sub := range dm.subs {
		if sub != nil {
			sub.Unsubscribe()
		}
	}

	// NATS 연결 종료
	if dm.natsConn != nil {
		dm.natsConn.Close()
	}

	// 데이터베이스 연결은 전역 인스턴스를 사용하므로 여기서 닫지 않음
	// database.Close()는 supervisor에서 처리

	log.Println("✅ Data Manager cleanup completed")
}
