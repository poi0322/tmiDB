package supervisor

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/tmidb/tmidb-core/internal/ipc"
	"github.com/tmidb/tmidb-core/internal/logger"
	"github.com/tmidb/tmidb-core/internal/process"
)

// Supervisor manages all tmiDB components and external services
type Supervisor struct {
	ctx            context.Context
	cancel         context.CancelFunc
	ipcServer      *ipc.Server
	logManager     *logger.Manager
	processManager *process.Manager

	// External services
	postgresql *exec.Cmd
	nats       *exec.Cmd
	seaweedfs  *exec.Cmd

	// Configuration
	config *Config

	// Status
	started  bool
	stopping bool

	// Go 1.24 cleanup management
	cleanup runtime.Cleanup
}

// Config holds supervisor configuration
type Config struct {
	// IPC settings
	SocketPath string `json:"socket_path"`

	// External services
	PostgreSQLPath string `json:"postgresql_path"`
	NATSPath       string `json:"nats_path"`
	SeaweedFSPath  string `json:"seaweedfs_path"`

	// Service ports for health checks
	PostgreSQLPort int `json:"postgresql_port"`
	NATSPort       int `json:"nats_port"`
	SeaweedFSPort  int `json:"seaweedfs_port"`

	// Timeouts
	StartupTimeout  time.Duration `json:"startup_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`

	// Log settings
	LogDir   string `json:"log_dir"`
	LogLevel string `json:"log_level"`
}

// DefaultConfig returns default supervisor configuration
func DefaultConfig() *Config {
	return &Config{
		SocketPath:      "/tmp/tmidb-supervisor.sock",
		PostgreSQLPath:  "postgres",
		NATSPath:        "nats-server",
		SeaweedFSPath:   "weed",
		PostgreSQLPort:  5432,
		NATSPort:        4222,
		SeaweedFSPort:   9333,
		StartupTimeout:  30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		LogDir:          "./logs",
		LogLevel:        "INFO",
	}
}

// parseLogLevel converts string log level to logger.LogLevel
func parseLogLevel(level string) logger.LogLevel {
	switch level {
	case "DEBUG":
		return logger.LogLevelDebug
	case "INFO":
		return logger.LogLevelInfo
	case "WARN":
		return logger.LogLevelWarn
	case "ERROR":
		return logger.LogLevelError
	default:
		return logger.LogLevelInfo
	}
}

// New creates a new supervisor instance
func New(config *Config) (*Supervisor, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create log directory
	if err := os.MkdirAll(config.LogDir, 0755); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Initialize IPC server first
	ipcServer := ipc.NewServer(config.SocketPath)

	// Initialize log manager
	logManager := logger.NewManager(&logger.LogConfig{
		BaseDir: config.LogDir,
		Level:   parseLogLevel(config.LogLevel),
	}, ipcServer)

	// Initialize process manager
	processManager := process.NewManager(ipcServer, logManager)

	supervisor := &Supervisor{
		ctx:            ctx,
		cancel:         cancel,
		ipcServer:      ipcServer,
		logManager:     logManager,
		processManager: processManager,
		config:         config,
	}

	// Go 1.24 기능: 자동 정리를 위한 cleanup 등록
	supervisor.cleanup = runtime.AddCleanup(&supervisor, func(s *Supervisor) {
		if !s.stopping {
			s.Stop()
		}
	}, supervisor)

	// Setup IPC handlers
	supervisor.setupIPCHandlers()

	return supervisor, nil
}

// Start starts the supervisor and all managed services
func (s *Supervisor) Start() error {
	if s.started {
		return fmt.Errorf("supervisor already started")
	}

	log.Println("Starting tmiDB Supervisor...")

	// Start IPC server
	if err := s.ipcServer.Start(); err != nil {
		return fmt.Errorf("failed to start IPC server: %w", err)
	}

	// Start external services
	if err := s.startExternalServices(); err != nil {
		return fmt.Errorf("failed to start external services: %w", err)
	}

	// Wait for external services to be ready
	if err := s.waitForServices(); err != nil {
		return fmt.Errorf("external services failed to start: %w", err)
	}

	// Register and start internal components
	if err := s.startInternalComponents(); err != nil {
		return fmt.Errorf("failed to start internal components: %w", err)
	}

	s.started = true
	log.Println("tmiDB Supervisor started successfully")

	return nil
}

// Run starts the supervisor and waits for shutdown signals
func (s *Supervisor) Run() error {
	if err := s.Start(); err != nil {
		return err
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down...", sig)
	case <-s.ctx.Done():
		log.Println("Context cancelled, shutting down...")
	}

	return s.Stop()
}

// Stop gracefully stops all services and the supervisor
func (s *Supervisor) Stop() error {
	if s.stopping {
		return nil
	}
	s.stopping = true

	log.Println("Stopping tmiDB Supervisor...")

	// Stop internal components first
	if err := s.processManager.Stop(); err != nil {
		log.Printf("Error stopping internal components: %v", err)
	}

	// Stop IPC server
	if err := s.ipcServer.Stop(); err != nil {
		log.Printf("Error stopping IPC server: %v", err)
	}

	// Stop log manager
	if err := s.logManager.Stop(); err != nil {
		log.Printf("Error stopping log manager: %v", err)
	}

	// Cancel main context
	s.cancel()

	// Stop cleanup
	s.cleanup.Stop()

	log.Println("tmiDB Supervisor stopped")
	return nil
}

// startExternalServices starts PostgreSQL, NATS, and SeaweedFS
func (s *Supervisor) startExternalServices() error {
	log.Println("Starting external services...")

	// Register PostgreSQL
	if err := s.processManager.RegisterProcess(&process.ProcessConfig{
		Name:        "postgresql",
		Type:        process.TypeExternal,
		Command:     s.config.PostgreSQLPath,
		Args:        []string{"-D", "./data/postgresql", "-p", fmt.Sprintf("%d", s.config.PostgreSQLPort)},
		AutoRestart: true,
		MaxRestarts: 3,
	}); err != nil {
		log.Printf("Warning: failed to register PostgreSQL: %v", err)
	}

	// Register NATS
	if err := s.processManager.RegisterProcess(&process.ProcessConfig{
		Name:        "nats",
		Type:        process.TypeExternal,
		Command:     s.config.NATSPath,
		Args:        []string{"-p", fmt.Sprintf("%d", s.config.NATSPort), "-m", "8222"},
		AutoRestart: true,
		MaxRestarts: 3,
	}); err != nil {
		log.Printf("Warning: failed to register NATS: %v", err)
	}

	// Register SeaweedFS
	if err := s.processManager.RegisterProcess(&process.ProcessConfig{
		Name:        "seaweedfs",
		Type:        process.TypeExternal,
		Command:     s.config.SeaweedFSPath,
		Args:        []string{"master", "-port", fmt.Sprintf("%d", s.config.SeaweedFSPort), "-mdir", "./data/seaweedfs/master"},
		AutoRestart: true,
		MaxRestarts: 3,
	}); err != nil {
		log.Printf("Warning: failed to register SeaweedFS: %v", err)
	}

	return nil
}

// waitForServices waits for external services to be ready
func (s *Supervisor) waitForServices() error {
	log.Println("Waiting for external services to be ready...")

	timeout := time.After(s.config.StartupTimeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	services := map[string]int{
		"PostgreSQL": s.config.PostgreSQLPort,
		"NATS":       s.config.NATSPort,
		"SeaweedFS":  s.config.SeaweedFSPort,
	}

	for {
		select {
		case <-timeout:
			log.Println("Timeout waiting for services, continuing anyway...")
			return nil // Continue even if services aren't ready
		case <-ticker.C:
			allReady := true
			for name, port := range services {
				if !s.isPortReady(port) {
					log.Printf("Waiting for %s on port %d...", name, port)
					allReady = false
				}
			}
			if allReady {
				log.Println("All external services are ready")
				return nil
			}
		}
	}
}

// isPortReady checks if a port is ready to accept connections
func (s *Supervisor) isPortReady(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// startInternalComponents registers and starts internal tmiDB components
func (s *Supervisor) startInternalComponents() error {
	log.Println("Starting internal components...")

	// Check if we're in development mode
	devMode := os.Getenv("TMIDB_DEV_MODE") == "true"
	hotReload := os.Getenv("TMIDB_HOT_RELOAD") == "true"

	if devMode && hotReload {
		log.Println("Development mode: Using Air for hot reload")

		// Register API component with Air
		if err := s.processManager.RegisterProcess(&process.ProcessConfig{
			Name:        "api",
			Type:        process.TypeInternal,
			Command:     "air",
			Args:        []string{"-c", ".air.api.toml"},
			WorkDir:     "./cmd/api",
			AutoRestart: true,
			MaxRestarts: 5,
		}); err != nil {
			log.Printf("Warning: failed to register API with Air: %v", err)
		}

		// Register Data Manager with Air
		if err := s.processManager.RegisterProcess(&process.ProcessConfig{
			Name:        "data-manager",
			Type:        process.TypeInternal,
			Command:     "air",
			Args:        []string{"-c", ".air.data-manager.toml"},
			WorkDir:     "./cmd/data-manager",
			AutoRestart: true,
			MaxRestarts: 5,
		}); err != nil {
			log.Printf("Warning: failed to register Data Manager with Air: %v", err)
		}

		// Register Data Consumer with Air
		if err := s.processManager.RegisterProcess(&process.ProcessConfig{
			Name:        "data-consumer",
			Type:        process.TypeInternal,
			Command:     "air",
			Args:        []string{"-c", ".air.data-consumer.toml"},
			WorkDir:     "./cmd/data-consumer",
			AutoRestart: true,
			MaxRestarts: 5,
		}); err != nil {
			log.Printf("Warning: failed to register Data Consumer with Air: %v", err)
		}
	} else {
		log.Println("Production mode: Using compiled binaries")

		// Register API component
		if err := s.processManager.RegisterProcess(&process.ProcessConfig{
			Name:        "api",
			Type:        process.TypeInternal,
			Command:     "./bin/tmidb-api",
			AutoRestart: true,
			MaxRestarts: 5,
		}); err != nil {
			log.Printf("Warning: failed to register API: %v", err)
		}

		// Register Data Manager
		if err := s.processManager.RegisterProcess(&process.ProcessConfig{
			Name:        "data-manager",
			Type:        process.TypeInternal,
			Command:     "./bin/tmidb-data-manager",
			AutoRestart: true,
			MaxRestarts: 5,
		}); err != nil {
			log.Printf("Warning: failed to register Data Manager: %v", err)
		}

		// Register Data Consumer
		if err := s.processManager.RegisterProcess(&process.ProcessConfig{
			Name:        "data-consumer",
			Type:        process.TypeInternal,
			Command:     "./bin/tmidb-data-consumer",
			AutoRestart: true,
			MaxRestarts: 5,
		}); err != nil {
			log.Printf("Warning: failed to register Data Consumer: %v", err)
		}
	}

	return nil
}

// setupIPCHandlers sets up IPC message handlers
func (s *Supervisor) setupIPCHandlers() {
	// Log control handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeLogEnable, s.handleEnableLogs)
	s.ipcServer.RegisterHandler(ipc.MessageTypeLogDisable, s.handleDisableLogs)
	s.ipcServer.RegisterHandler(ipc.MessageTypeLogStatus, s.handleGetLogStatus)
	s.ipcServer.RegisterHandler(ipc.MessageTypeGetLogs, s.handleGetLogs)

	// Process control handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessList, s.handleGetProcessList)
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessStatus, s.handleGetProcessStatus)
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessStart, s.handleStartProcess)
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessStop, s.handleStopProcess)
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessRestart, s.handleRestartProcess)

	// System handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeSystemHealth, s.handleGetSystemHealth)
	s.ipcServer.RegisterHandler(ipc.MessageTypeSystemStats, s.handleGetSystemResources)
}

// IPC handler implementations with correct signatures
func (s *Supervisor) handleEnableLogs(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	// Simple implementation - just return success
	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    "logs enabled",
	}
}

func (s *Supervisor) handleDisableLogs(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    "logs disabled",
	}
}

func (s *Supervisor) handleGetLogStatus(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    map[string]interface{}{"status": "active"},
	}
}

func (s *Supervisor) handleGetLogs(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		component = "all"
	}

	lines, ok := msg.Data["lines"].(float64) // JSON numbers are float64
	if !ok {
		lines = 50
	}

	// For now, return dummy log data
	// In a real implementation, this would fetch from log files or log manager
	logs := []map[string]interface{}{
		{
			"timestamp": time.Now().Add(-5 * time.Minute).Format("15:04:05"),
			"process":   "api",
			"message":   "Server started on port 8020",
		},
		{
			"timestamp": time.Now().Add(-3 * time.Minute).Format("15:04:05"),
			"process":   "data-manager",
			"message":   "Connected to database",
		},
		{
			"timestamp": time.Now().Add(-1 * time.Minute).Format("15:04:05"),
			"process":   "data-consumer",
			"message":   "Processing queue messages",
		},
	}

	// Filter by component if specified
	if component != "all" {
		var filteredLogs []map[string]interface{}
		for _, log := range logs {
			if log["process"] == component {
				filteredLogs = append(filteredLogs, log)
			}
		}
		logs = filteredLogs
	}

	// Limit number of lines
	if int(lines) < len(logs) {
		logs = logs[len(logs)-int(lines):]
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    logs,
	}
}

func (s *Supervisor) handleGetProcessList(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	processes := s.processManager.GetProcessList()
	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    processes,
	}
}

func (s *Supervisor) handleGetProcessStatus(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	// Extract process name from message data
	processName, ok := msg.Data["name"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "invalid process name",
		}
	}

	status, err := s.processManager.GetProcessStatus(processName)
	if err != nil {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   err.Error(),
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    status,
	}
}

func (s *Supervisor) handleStartProcess(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	processName, ok := msg.Data["name"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "invalid process name",
		}
	}

	if err := s.processManager.StartProcess(processName); err != nil {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   err.Error(),
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    "process started",
	}
}

func (s *Supervisor) handleStopProcess(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	processName, ok := msg.Data["name"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "invalid process name",
		}
	}

	if err := s.processManager.StopProcess(processName); err != nil {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   err.Error(),
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    "process stopped",
	}
}

func (s *Supervisor) handleRestartProcess(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	processName, ok := msg.Data["name"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "invalid process name",
		}
	}

	if err := s.processManager.RestartProcess(processName); err != nil {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   err.Error(),
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    "process restarted",
	}
}

func (s *Supervisor) handleGetSystemHealth(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	health := &ipc.SystemHealth{
		Status:     "healthy",
		Uptime:     time.Since(time.Now().Add(-time.Hour)), // Placeholder
		Components: make(map[string]string),
		Resources: ipc.SystemResources{
			CPUUsage:    0.0,
			MemoryUsage: 0.0,
			DiskUsage:   0.0,
		},
		LastCheck: time.Now(),
	}

	// Check services
	services := map[string]int{
		"postgresql": s.config.PostgreSQLPort,
		"nats":       s.config.NATSPort,
		"seaweedfs":  s.config.SeaweedFSPort,
	}

	for name, port := range services {
		if s.isPortReady(port) {
			health.Components[name] = "healthy"
		} else {
			health.Components[name] = "unhealthy"
			health.Status = "degraded"
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    health,
	}
}

func (s *Supervisor) handleGetSystemResources(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	resources := &ipc.SystemResources{
		CPUUsage:    0.0, // TODO: implement CPU usage calculation
		MemoryUsage: float64(m.Alloc) / float64(m.Sys) * 100,
		DiskUsage:   0.0, // TODO: implement disk usage calculation
		NetworkIO:   0,
		DiskIO:      0,
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    resources,
	}
}

// GetLogManager returns the log manager instance
func (s *Supervisor) GetLogManager() *logger.Manager {
	return s.logManager
}

// GetProcessManager returns the process manager instance
func (s *Supervisor) GetProcessManager() *process.Manager {
	return s.processManager
}

// GetIPCServer returns the IPC server instance
func (s *Supervisor) GetIPCServer() *ipc.Server {
	return s.ipcServer
}
