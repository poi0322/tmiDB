package main

import (
	"log"
	"os"

	"github.com/tmidb/tmidb-core/internal/supervisor"
)

func main() {
	log.Println("🚀 Starting tmiDB Supervisor...")

	// Create supervisor with default config
	config := supervisor.DefaultConfig()

	// Override with environment variables if set
	if socketPath := os.Getenv("TMIDB_SOCKET_PATH"); socketPath != "" {
		config.SocketPath = socketPath
	}
	if logDir := os.Getenv("TMIDB_LOG_DIR"); logDir != "" {
		config.LogDir = logDir
	}
	if logLevel := os.Getenv("TMIDB_LOG_LEVEL"); logLevel != "" {
		config.LogLevel = logLevel
	}

	// Create and run supervisor
	sup, err := supervisor.New(config)
	if err != nil {
		log.Fatalf("❌ Failed to create supervisor: %v", err)
	}

	// Run supervisor (blocks until shutdown signal)
	if err := sup.Run(); err != nil {
		log.Fatalf("❌ Supervisor error: %v", err)
	}

	log.Println("✅ tmiDB Supervisor stopped")
}
