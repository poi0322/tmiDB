package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tmidb/tmidb-core/internal/datamanager"
)

func main() {
	log.Println("🚀 Starting tmiDB Data Manager...")

	// 컨텍스트 생성
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 시그널 핸들링
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Data Manager 인스턴스 생성
	dm := datamanager.New()

	// Data Manager 시작
	go func() {
		if err := dm.Start(ctx); err != nil {
			log.Printf("❌ Data Manager failed: %v", err)
			cancel()
		}
	}()

	// 시그널 대기
	select {
	case sig := <-sigChan:
		log.Printf("📡 Received signal: %v", sig)
		log.Println("🛑 Shutting down Data Manager...")
		cancel()
	case <-ctx.Done():
		log.Println("🛑 Data Manager context cancelled")
	}

	// 정리 시간 대기
	time.Sleep(1 * time.Second)
	log.Println("✅ Data Manager stopped gracefully")
}
