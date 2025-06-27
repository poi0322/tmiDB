package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	log.Println("🚀 Starting tmiDB Supervisor...")

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Start external services
	services := []*Service{
		{Name: "PostgreSQL", Command: "sudo", Args: []string{"-u", "postgres", "/usr/lib/postgresql/15/bin/postgres", "-D", "/var/lib/postgresql/15/main"}},
		{Name: "NATS", Command: "nats-server", Args: []string{"--jetstream", "--store_dir=/data/nats", "-p", "4222"}},
		{Name: "SeaweedFS", Command: "weed", Args: []string{"server", "-s3", "-dir=/data/seaweedfs", "-s3.port=8333", "-master.port=9333", "-volume.port=8081"}},
	}

	// Start all external services
	for _, service := range services {
		wg.Add(1)
		go func(svc *Service) {
			defer wg.Done()
			startService(ctx, svc)
		}(service)

		// Small delay between service starts
		time.Sleep(1 * time.Second)
	}

	// Wait for services to be ready
	log.Println("⏳ Waiting for external services to be ready...")
	time.Sleep(10 * time.Second)

	// Start API server
	wg.Add(1)
	go func() {
		defer wg.Done()
		startAPIServer(ctx)
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Printf("🛑 Received signal %v, shutting down...", sig)
	case <-ctx.Done():
		log.Println("🛑 Context cancelled, shutting down...")
	}

	// Cancel context to stop all services
	cancel()

	// Wait for all services to stop
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("✅ All services stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Println("⚠️ Timeout waiting for services to stop")
	}
}

type Service struct {
	Name    string
	Command string
	Args    []string
}

func startService(ctx context.Context, service *Service) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("🛑 Stopping %s", service.Name)
			return
		default:
		}

		log.Printf("🔄 Starting %s...", service.Name)

		cmd := exec.CommandContext(ctx, service.Command, service.Args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			log.Printf("❌ Failed to start %s: %v", service.Name, err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("✅ %s started (PID: %d)", service.Name, cmd.Process.Pid)

		// Wait for the process to exit
		if err := cmd.Wait(); err != nil {
			log.Printf("⚠️ %s exited with error: %v", service.Name, err)
		} else {
			log.Printf("ℹ️ %s exited normally", service.Name)
		}

		// Don't restart if context is cancelled
		select {
		case <-ctx.Done():
			return
		default:
			log.Printf("🔄 Restarting %s in 5 seconds...", service.Name)
			time.Sleep(5 * time.Second)
		}
	}
}

func startAPIServer(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Stopping API Server")
			return
		default:
		}

		log.Println("🌐 Starting API Server...")

		cmd := exec.CommandContext(ctx, "go", "run", "./cmd/api")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			log.Printf("❌ Failed to start API Server: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("✅ API Server started (PID: %d)", cmd.Process.Pid)

		// Wait for the process to exit
		if err := cmd.Wait(); err != nil {
			log.Printf("⚠️ API Server exited with error: %v", err)
		} else {
			log.Println("ℹ️ API Server exited normally")
		}

		// Don't restart if context is cancelled
		select {
		case <-ctx.Done():
			return
		default:
			log.Println("🔄 Restarting API Server in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}
}
