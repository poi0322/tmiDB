package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/tmidb/tmidb-core/internal/ipc"
	"github.com/tmidb/tmidb-core/internal/logger"
)

// ProcessState 프로세스 상태
type ProcessState string

const (
	StateRunning    ProcessState = "running"
	StateStopped    ProcessState = "stopped"
	StateStarting   ProcessState = "starting"
	StateStopping   ProcessState = "stopping"
	StateError      ProcessState = "error"
	StateRestarting ProcessState = "restarting"
)

// ProcessType 프로세스 타입
type ProcessType string

const (
	TypeInternal ProcessType = "internal" // 내부 Go 프로세스
	TypeExternal ProcessType = "external" // 외부 바이너리
	TypeService  ProcessType = "service"  // 시스템 서비스
)

// Manager 프로세스 관리자
type Manager struct {
	processes    map[string]*Process
	processesMux sync.RWMutex
	ipcServer    *ipc.Server
	logManager   *logger.Manager
	ctx          context.Context
	cancel       context.CancelFunc

	// Go 1.24 기능: 자원 관리
	cleanupFuncs []func()
	cleanupMux   sync.Mutex
}

// Process 프로세스 정보
type Process struct {
	Name         string            `json:"name"`
	Type         ProcessType       `json:"type"`
	Command      string            `json:"command"`
	Args         []string          `json:"args"`
	WorkDir      string            `json:"work_dir"`
	Env          map[string]string `json:"env"`
	State        ProcessState      `json:"state"`
	PID          int               `json:"pid"`
	StartTime    time.Time         `json:"start_time"`
	Uptime       time.Duration     `json:"uptime"`
	RestartCount int               `json:"restart_count"`
	AutoRestart  bool              `json:"auto_restart"`
	MaxRestarts  int               `json:"max_restarts"`

	// 프로세스 제어
	cmd    *exec.Cmd
	cancel context.CancelFunc
	stdout io.ReadCloser
	stderr io.ReadCloser

	// 통계
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage int64   `json:"memory_usage"`
	LastError   string  `json:"last_error,omitempty"`

	// 동기화
	mutex sync.RWMutex

	// Go 1.24 기능: 프로세스별 정리
	cleanup func()
}

// ProcessConfig 프로세스 설정
type ProcessConfig struct {
	Name        string            `json:"name"`
	Type        ProcessType       `json:"type"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	WorkDir     string            `json:"work_dir"`
	Env         map[string]string `json:"env"`
	AutoRestart bool              `json:"auto_restart"`
	MaxRestarts int               `json:"max_restarts"`
}

// NewManager 새로운 프로세스 관리자 생성
func NewManager(ipcServer *ipc.Server, logManager *logger.Manager) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		processes:    make(map[string]*Process),
		ipcServer:    ipcServer,
		logManager:   logManager,
		ctx:          ctx,
		cancel:       cancel,
		cleanupFuncs: make([]func(), 0),
	}

	// Go 1.24 기능: 자원 정리를 위한 finalizer 설정
	runtime.SetFinalizer(manager, func(m *Manager) {
		m.cleanup()
	})

	return manager
}

// Start 프로세스 관리자 시작
func (m *Manager) Start() error {
	// IPC 핸들러 등록
	m.registerIPCHandlers()

	// 모니터링 고루틴 시작
	go m.monitorProcesses()

	log.Printf("🔧 Process Manager started")
	return nil
}

// Stop 프로세스 관리자 정지
func (m *Manager) Stop() error {
	m.cancel()

	// 모든 프로세스 정지
	m.processesMux.RLock()
	var processes []*Process
	for _, proc := range m.processes {
		processes = append(processes, proc)
	}
	m.processesMux.RUnlock()

	// 병렬로 프로세스 정지
	var wg sync.WaitGroup
	for _, proc := range processes {
		wg.Add(1)
		go func(p *Process) {
			defer wg.Done()
			m.StopProcess(p.Name)
		}(proc)
	}

	// 최대 30초 대기
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("✅ All processes stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Printf("⚠️ Process shutdown timeout, forcing termination")
		m.forceStopAll()
	}

	return nil
}

// RegisterProcess 프로세스 등록
func (m *Manager) RegisterProcess(config *ProcessConfig) error {
	m.processesMux.Lock()
	defer m.processesMux.Unlock()

	if _, exists := m.processes[config.Name]; exists {
		return fmt.Errorf("process %s already registered", config.Name)
	}

	process := &Process{
		Name:         config.Name,
		Type:         config.Type,
		Command:      config.Command,
		Args:         config.Args,
		WorkDir:      config.WorkDir,
		Env:          config.Env,
		State:        StateStopped,
		AutoRestart:  config.AutoRestart,
		MaxRestarts:  config.MaxRestarts,
		RestartCount: 0,
	}

	// Go 1.24 기능: 프로세스별 정리 함수 설정
	process.cleanup = func() {
		if process.cmd != nil && process.cmd.Process != nil {
			process.cmd.Process.Kill()
		}
	}
	runtime.SetFinalizer(process, func(p *Process) {
		if p.cleanup != nil {
			p.cleanup()
		}
	})

	m.processes[config.Name] = process

	log.Printf("📋 Process registered: %s (%s)", config.Name, config.Type)
	return nil
}

// StartProcess 프로세스 시작
func (m *Manager) StartProcess(name string) error {
	m.processesMux.RLock()
	process, exists := m.processes[name]
	m.processesMux.RUnlock()

	if !exists {
		return fmt.Errorf("process %s not found", name)
	}

	process.mutex.Lock()
	defer process.mutex.Unlock()

	if process.State == StateRunning || process.State == StateStarting {
		return fmt.Errorf("process %s is already running or starting", name)
	}

	process.State = StateStarting

	// 프로세스 컨텍스트 생성
	ctx, cancel := context.WithCancel(m.ctx)
	process.cancel = cancel

	// 명령어 생성
	cmd := exec.CommandContext(ctx, process.Command, process.Args...)

	// 작업 디렉토리 설정
	if process.WorkDir != "" {
		cmd.Dir = process.WorkDir
	}

	// 환경 변수 설정
	if len(process.Env) > 0 {
		env := os.Environ()
		for k, v := range process.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	// 표준 출력/오류 파이프 생성
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		process.State = StateError
		process.LastError = fmt.Sprintf("failed to create stdout pipe: %v", err)
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		process.State = StateError
		process.LastError = fmt.Sprintf("failed to create stderr pipe: %v", err)
		return err
	}

	process.cmd = cmd
	process.stdout = stdout
	process.stderr = stderr

	// 프로세스 시작
	if err := cmd.Start(); err != nil {
		process.State = StateError
		process.LastError = fmt.Sprintf("failed to start process: %v", err)
		return err
	}

	process.PID = cmd.Process.Pid
	process.StartTime = time.Now()
	process.State = StateRunning
	process.LastError = ""

	log.Printf("🚀 Process started: %s (PID: %d)", name, process.PID)

	// 로그 캡처 고루틴 시작
	go m.captureOutput(process, stdout, "stdout")
	go m.captureOutput(process, stderr, "stderr")

	// 프로세스 모니터링 고루틴 시작
	go m.watchProcess(process)

	return nil
}

// StopProcess 프로세스 정지
func (m *Manager) StopProcess(name string) error {
	m.processesMux.RLock()
	process, exists := m.processes[name]
	m.processesMux.RUnlock()

	if !exists {
		return fmt.Errorf("process %s not found", name)
	}

	process.mutex.Lock()
	defer process.mutex.Unlock()

	if process.State != StateRunning {
		return fmt.Errorf("process %s is not running", name)
	}

	process.State = StateStopping

	// Graceful shutdown 시도
	if process.cmd != nil && process.cmd.Process != nil {
		// SIGTERM 전송
		if err := process.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			log.Printf("⚠️ Failed to send SIGTERM to %s: %v", name, err)
		}

		// 10초 대기
		done := make(chan error, 1)
		go func() {
			done <- process.cmd.Wait()
		}()

		select {
		case err := <-done:
			if err != nil && err.Error() != "signal: terminated" {
				log.Printf("⚠️ Process %s exited with error: %v", name, err)
			}
		case <-time.After(10 * time.Second):
			// 강제 종료
			log.Printf("🔨 Force killing process %s", name)
			process.cmd.Process.Kill()
			<-done // Wait for the process to actually exit
		}
	}

	// 컨텍스트 취소
	if process.cancel != nil {
		process.cancel()
	}

	process.State = StateStopped
	process.PID = 0
	process.Uptime = 0

	log.Printf("🛑 Process stopped: %s", name)
	return nil
}

// RestartProcess 프로세스 재시작
func (m *Manager) RestartProcess(name string) error {
	m.processesMux.RLock()
	process, exists := m.processes[name]
	m.processesMux.RUnlock()

	if !exists {
		return fmt.Errorf("process %s not found", name)
	}

	process.mutex.Lock()
	process.State = StateRestarting
	process.RestartCount++
	process.mutex.Unlock()

	log.Printf("🔄 Restarting process: %s", name)

	// 정지 후 시작
	if err := m.StopProcess(name); err != nil {
		log.Printf("⚠️ Failed to stop process %s during restart: %v", name, err)
	}

	// 잠시 대기
	time.Sleep(2 * time.Second)

	return m.StartProcess(name)
}

// GetProcessList 프로세스 목록 조회
func (m *Manager) GetProcessList() []ipc.ProcessInfo {
	m.processesMux.RLock()
	defer m.processesMux.RUnlock()

	var processes []ipc.ProcessInfo
	for _, proc := range m.processes {
		proc.mutex.RLock()

		uptime := time.Duration(0)
		if proc.State == StateRunning && !proc.StartTime.IsZero() {
			uptime = time.Since(proc.StartTime)
		}

		processInfo := ipc.ProcessInfo{
			Name:      proc.Name,
			Status:    string(proc.State),
			PID:       proc.PID,
			Uptime:    uptime,
			Memory:    proc.MemoryUsage,
			CPU:       proc.CPUUsage,
			Enabled:   proc.AutoRestart,
			Logs:      true, // 로그는 항상 활성화
			StartTime: proc.StartTime,
		}

		processes = append(processes, processInfo)
		proc.mutex.RUnlock()
	}

	return processes
}

// GetProcessStatus 특정 프로세스 상태 조회
func (m *Manager) GetProcessStatus(name string) (*ipc.ProcessInfo, error) {
	m.processesMux.RLock()
	process, exists := m.processes[name]
	m.processesMux.RUnlock()

	if !exists {
		return nil, fmt.Errorf("process %s not found", name)
	}

	process.mutex.RLock()
	defer process.mutex.RUnlock()

	uptime := time.Duration(0)
	if process.State == StateRunning && !process.StartTime.IsZero() {
		uptime = time.Since(process.StartTime)
	}

	return &ipc.ProcessInfo{
		Name:      process.Name,
		Status:    string(process.State),
		PID:       process.PID,
		Uptime:    uptime,
		Memory:    process.MemoryUsage,
		CPU:       process.CPUUsage,
		Enabled:   process.AutoRestart,
		Logs:      true,
		StartTime: process.StartTime,
	}, nil
}

// captureOutput 프로세스 출력 캡처
func (m *Manager) captureOutput(process *Process, reader io.ReadCloser, streamType string) {
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		// 로그 레벨 결정
		var level logger.LogLevel
		if streamType == "stderr" {
			level = logger.LogLevelError
		} else {
			level = logger.LogLevelInfo
		}

		// 로그 매니저에 전달
		if m.logManager != nil {
			m.logManager.WriteLog(process.Name, level, line)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("❌ Error reading %s from %s: %v", streamType, process.Name, err)
	}
}

// watchProcess 프로세스 감시
func (m *Manager) watchProcess(process *Process) {
	if process.cmd == nil {
		return
	}

	// 프로세스 종료 대기
	err := process.cmd.Wait()

	process.mutex.Lock()
	defer process.mutex.Unlock()

	if process.State == StateStopping {
		// 정상적인 종료
		process.State = StateStopped
		return
	}

	// 예상치 못한 종료
	process.State = StateError
	if err != nil {
		process.LastError = err.Error()
		log.Printf("❌ Process %s exited unexpectedly: %v", process.Name, err)
	} else {
		log.Printf("⚠️ Process %s exited unexpectedly", process.Name)
	}

	// 자동 재시작 확인
	if process.AutoRestart && process.RestartCount < process.MaxRestarts {
		log.Printf("🔄 Auto-restarting process: %s (attempt %d/%d)",
			process.Name, process.RestartCount+1, process.MaxRestarts)

		// 잠시 대기 후 재시작
		go func() {
			time.Sleep(5 * time.Second)
			m.RestartProcess(process.Name)
		}()
	}
}

// monitorProcesses 프로세스 모니터링
func (m *Manager) monitorProcesses() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateProcessStats()
		}
	}
}

// updateProcessStats 프로세스 통계 업데이트
func (m *Manager) updateProcessStats() {
	m.processesMux.RLock()
	defer m.processesMux.RUnlock()

	for _, process := range m.processes {
		if process.State != StateRunning || process.PID == 0 {
			continue
		}

		// CPU와 메모리 사용량 업데이트 (간단한 구현)
		// 실제 구현에서는 /proc/[pid]/stat 파일을 읽거나 시스템 API를 사용
		process.mutex.Lock()
		process.Uptime = time.Since(process.StartTime)
		// TODO: 실제 CPU/메모리 사용량 계산 구현
		process.mutex.Unlock()
	}
}

// forceStopAll 모든 프로세스 강제 종료
func (m *Manager) forceStopAll() {
	m.processesMux.RLock()
	defer m.processesMux.RUnlock()

	for _, process := range m.processes {
		if process.cmd != nil && process.cmd.Process != nil {
			process.cmd.Process.Kill()
		}
	}
}

// registerIPCHandlers IPC 핸들러 등록
func (m *Manager) registerIPCHandlers() {
	if m.ipcServer == nil {
		return
	}

	m.ipcServer.RegisterHandler(ipc.MessageTypeProcessList, m.handleProcessList)
	m.ipcServer.RegisterHandler(ipc.MessageTypeProcessStatus, m.handleProcessStatus)
	m.ipcServer.RegisterHandler(ipc.MessageTypeProcessStart, m.handleProcessStart)
	m.ipcServer.RegisterHandler(ipc.MessageTypeProcessStop, m.handleProcessStop)
	m.ipcServer.RegisterHandler(ipc.MessageTypeProcessRestart, m.handleProcessRestart)
}

// handleProcessList 프로세스 목록 핸들러
func (m *Manager) handleProcessList(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	processes := m.GetProcessList()
	return ipc.NewResponse(msg.ID, true, processes, "")
}

// handleProcessStatus 프로세스 상태 핸들러
func (m *Manager) handleProcessStatus(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component parameter required")
	}

	status, err := m.GetProcessStatus(component)
	if err != nil {
		return ipc.NewResponse(msg.ID, false, nil, err.Error())
	}

	return ipc.NewResponse(msg.ID, true, status, "")
}

// handleProcessStart 프로세스 시작 핸들러
func (m *Manager) handleProcessStart(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component parameter required")
	}

	if err := m.StartProcess(component); err != nil {
		return ipc.NewResponse(msg.ID, false, nil, err.Error())
	}

	return ipc.NewResponse(msg.ID, true, map[string]interface{}{
		"component": component,
		"action":    "started",
	}, "")
}

// handleProcessStop 프로세스 정지 핸들러
func (m *Manager) handleProcessStop(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component parameter required")
	}

	if err := m.StopProcess(component); err != nil {
		return ipc.NewResponse(msg.ID, false, nil, err.Error())
	}

	return ipc.NewResponse(msg.ID, true, map[string]interface{}{
		"component": component,
		"action":    "stopped",
	}, "")
}

// handleProcessRestart 프로세스 재시작 핸들러
func (m *Manager) handleProcessRestart(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component parameter required")
	}

	if err := m.RestartProcess(component); err != nil {
		return ipc.NewResponse(msg.ID, false, nil, err.Error())
	}

	return ipc.NewResponse(msg.ID, true, map[string]interface{}{
		"component": component,
		"action":    "restarted",
	}, "")
}

// cleanup Go 1.24 기능: 자원 정리
func (m *Manager) cleanup() {
	m.cleanupMux.Lock()
	defer m.cleanupMux.Unlock()

	for _, cleanupFunc := range m.cleanupFuncs {
		cleanupFunc()
	}

	m.Stop()
}

// addCleanupFunc 정리 함수 추가
func (m *Manager) addCleanupFunc(fn func()) {
	m.cleanupMux.Lock()
	defer m.cleanupMux.Unlock()

	m.cleanupFuncs = append(m.cleanupFuncs, fn)
}
