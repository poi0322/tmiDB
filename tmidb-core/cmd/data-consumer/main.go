package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tmidb/tmidb-core/internal/dataconsumer"
)

func main() {
	log.Println("🚀 Starting tmiDB Data Consumer...")

	// 컨텍스트 생성
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 시그널 핸들링
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Data Consumer 인스턴스 생성
	dc := dataconsumer.New()

	// Data Consumer 시작
	go func() {
		if err := dc.Start(ctx); err != nil {
			log.Printf("❌ Data Consumer failed: %v", err)
			cancel()
		}
	}()

	// 시그널 대기
	select {
	case sig := <-sigChan:
		log.Printf("📡 Received signal: %v", sig)
		log.Println("🛑 Shutting down Data Consumer...")
		cancel()
	case <-ctx.Done():
		log.Println("🛑 Data Consumer context cancelled")
	}

	// 정리 시간 대기
	time.Sleep(1 * time.Second)
	log.Println("✅ Data Consumer stopped gracefully")
}
