package dataconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/tmidb/tmidb-core/internal/database"
)

// DataConsumer 데이터 소비 및 처리를 담당하는 구조체
type DataConsumer struct {
	natsConn *nats.Conn
	subs     []*nats.Subscription
}

// DataPoint 처리할 데이터 포인트 구조체
type DataPoint struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`
	Category  string                 `json:"category"`
	Data      map[string]interface{} `json:"data"`
}

// New DataConsumer 인스턴스를 생성합니다
func New() *DataConsumer {
	dc := &DataConsumer{}

	// Go 1.24 기능: 자동 정리를 위한 cleanup 등록
	runtime.AddCleanup(&dc, func(consumer *DataConsumer) {
		consumer.cleanup()
	}, dc)

	return dc
}

// Start DataConsumer를 시작합니다
func (dc *DataConsumer) Start(ctx context.Context) error {
	log.Println("🔄 Initializing Data Consumer...")

	// 데이터베이스 연결
	if err := dc.connectDatabase(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// NATS 연결
	if err := dc.connectNATS(); err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// 데이터 구독 시작
	if err := dc.startSubscriptions(); err != nil {
		return fmt.Errorf("failed to start subscriptions: %w", err)
	}

	// 배치 처리 시작
	go dc.startBatchProcessor(ctx)

	log.Println("✅ Data Consumer started successfully")

	// 컨텍스트 완료까지 대기
	<-ctx.Done()

	// 정리 작업
	dc.cleanup()

	return nil
}

// connectDatabase 데이터베이스에 연결합니다
func (dc *DataConsumer) connectDatabase() error {
	for i := 0; i < 15; i++ {
		err := database.CheckDatabaseHealth()
		if err == nil {
			log.Println("✅ Connected to database")
			return nil
		}
		log.Printf("⏳ Waiting for database... (attempt %d/15)", i+1)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("failed to connect to database after 15 attempts")
}

// connectNATS NATS 서버에 연결합니다
func (dc *DataConsumer) connectNATS() error {
	var err error
	for i := 0; i < 10; i++ {
		dc.natsConn, err = nats.Connect("nats://localhost:4222")
		if err == nil {
			log.Println("✅ Connected to NATS server")
			return nil
		}
		log.Printf("⏳ Waiting for NATS server... (attempt %d/10)", i+1)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("failed to connect to NATS after 10 attempts: %w", err)
}

// startSubscriptions 데이터 구독을 시작합니다
func (dc *DataConsumer) startSubscriptions() error {
	// 모든 데이터 스트림 구독
	sub1, err := dc.natsConn.Subscribe("tmidb.data.>", dc.handleDataMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to data stream: %w", err)
	}
	dc.subs = append(dc.subs, sub1)

	// 시스템 메트릭 구독
	sub2, err := dc.natsConn.Subscribe("tmidb.data.system.>", dc.handleSystemMetrics)
	if err != nil {
		return fmt.Errorf("failed to subscribe to system metrics: %w", err)
	}
	dc.subs = append(dc.subs, sub2)

	log.Println("📡 Started NATS subscriptions")
	return nil
}

// handleDataMessage 일반 데이터 메시지를 처리합니다
func (dc *DataConsumer) handleDataMessage(msg *nats.Msg) {
	var dataPoint DataPoint
	if err := json.Unmarshal(msg.Data, &dataPoint); err != nil {
		log.Printf("❌ Failed to unmarshal data message: %v", err)
		return
	}

	log.Printf("📨 Received data: %s from %s.%s", dataPoint.ID, dataPoint.Source, dataPoint.Category)

	// 데이터베이스에 저장
	if err := dc.saveToDatabase(dataPoint); err != nil {
		log.Printf("❌ Failed to save data to database: %v", err)
		return
	}

	log.Printf("💾 Saved data: %s", dataPoint.ID)
}

// handleSystemMetrics 시스템 메트릭을 처리합니다
func (dc *DataConsumer) handleSystemMetrics(msg *nats.Msg) {
	var dataPoint DataPoint
	if err := json.Unmarshal(msg.Data, &dataPoint); err != nil {
		log.Printf("❌ Failed to unmarshal system metrics: %v", err)
		return
	}

	log.Printf("📊 Processing system metrics: %s", dataPoint.ID)

	// 시스템 메트릭 특별 처리
	if err := dc.processSystemMetrics(dataPoint); err != nil {
		log.Printf("❌ Failed to process system metrics: %v", err)
		return
	}

	// 데이터베이스에 저장
	if err := dc.saveToDatabase(dataPoint); err != nil {
		log.Printf("❌ Failed to save system metrics: %v", err)
		return
	}

	log.Printf("📈 Processed and saved system metrics: %s", dataPoint.ID)
}

// processSystemMetrics 시스템 메트릭을 특별 처리합니다
func (dc *DataConsumer) processSystemMetrics(dataPoint DataPoint) error {
	// CPU 사용률이 90% 이상인 경우 알림
	if cpuUsage, ok := dataPoint.Data["cpu_usage"].(float64); ok && cpuUsage > 90.0 {
		log.Printf("⚠️ HIGH CPU USAGE ALERT: %.1f%%", cpuUsage)
		// 여기서 알림 시스템으로 메시지를 보낼 수 있습니다
	}

	// 메모리 사용률이 85% 이상인 경우 알림
	if memUsage, ok := dataPoint.Data["memory_usage"].(float64); ok && memUsage > 85.0 {
		log.Printf("⚠️ HIGH MEMORY USAGE ALERT: %.1f%%", memUsage)
	}

	return nil
}

// saveToDatabase 데이터를 데이터베이스에 저장합니다
func (dc *DataConsumer) saveToDatabase(dataPoint DataPoint) error {
	if database.DB == nil {
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

	_, err = database.DB.Exec(query, dataPoint.ID, dataPoint.Timestamp,
		dataPoint.Source, dataPoint.Category, string(dataJSON))
	if err != nil {
		return fmt.Errorf("failed to insert data into database: %w", err)
	}

	return nil
}

// startBatchProcessor 배치 처리를 시작합니다
func (dc *DataConsumer) startBatchProcessor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	log.Println("🔄 Starting batch processor...")

	for {
		select {
		case <-ticker.C:
			dc.processBatch()
		case <-ctx.Done():
			log.Println("🛑 Stopping batch processor...")
			return
		}
	}
}

// processBatch 배치 처리를 수행합니다
func (dc *DataConsumer) processBatch() {
	log.Println("🔄 Running batch processing...")

	// 데이터 집계 작업
	if err := dc.aggregateData(); err != nil {
		log.Printf("❌ Failed to aggregate data: %v", err)
	}

	// 오래된 데이터 정리
	if err := dc.cleanupOldData(); err != nil {
		log.Printf("❌ Failed to cleanup old data: %v", err)
	}

	log.Println("✅ Batch processing completed")
}

// aggregateData 데이터 집계를 수행합니다
func (dc *DataConsumer) aggregateData() error {
	if database.DB == nil {
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

	_, err := database.DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to aggregate data: %w", err)
	}

	log.Println("📊 Data aggregation completed")
	return nil
}

// cleanupOldData 오래된 데이터를 정리합니다
func (dc *DataConsumer) cleanupOldData() error {
	if database.DB == nil {
		return fmt.Errorf("database connection not available")
	}

	// 30일 이상된 원시 데이터 삭제
	query := `DELETE FROM ts_obs WHERE timestamp < NOW() - INTERVAL '30 days'`

	result, err := database.DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to cleanup old data: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d old records", rowsAffected)
	}

	return nil
}

// cleanup 리소스를 정리합니다
func (dc *DataConsumer) cleanup() {
	log.Println("🧹 Cleaning up Data Consumer...")

	// NATS 구독 해제
	for _, sub := range dc.subs {
		if sub != nil {
			sub.Unsubscribe()
		}
	}

	// NATS 연결 종료
	if dc.natsConn != nil {
		dc.natsConn.Close()
	}

	// 데이터베이스 연결 종료는 전역 DB에서 관리됨

	log.Println("✅ Data Consumer cleanup completed")
}
