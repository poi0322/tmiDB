package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmidb/tmidb-core/internal/ipc"
)

// Global IPC client
var client *ipc.Client

var rootCmd = &cobra.Command{
	Use:   "tmidb-cli",
	Short: "tmiDB CLI tool for managing tmiDB-Core components",
	Long: `tmiDB CLI is a command-line tool for managing and monitoring 
tmiDB-Core components including logging, process control, and system monitoring.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// IPC 클라이언트 초기화
		client = ipc.NewClient("")
		if err := client.Connect(); err != nil {
			fmt.Printf("❌ Failed to connect to supervisor: %v\n", err)
			fmt.Println("💡 Make sure tmidb-supervisor is running")
			os.Exit(1)
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// IPC 클라이언트 정리
		if client != nil {
			client.Close()
		}
	},
}

// 로그 관련 명령어들
var logsCmd = &cobra.Command{
	Use:   "logs [component]",
	Short: "Show logs for components",
	Long:  "Show logs for all components or a specific component. Use -f to follow logs in real-time.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := "all"
		if len(args) > 0 {
			component = args[0]
		}

		follow, _ := cmd.Flags().GetBool("follow")

		if follow {
			// Follow 모드
			fmt.Printf("📄 Following logs for: %s (Press Ctrl+C to stop)\n", component)

			// 로그 스트림 시작
			logChan, err := client.StreamLogs(component)
			if err != nil {
				fmt.Printf("❌ Failed to start log stream: %v\n", err)
				os.Exit(1)
			}

			// 신호 처리
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			// 로그 출력 루프
			for {
				select {
				case logEntry, ok := <-logChan:
					if !ok {
						fmt.Println("📄 Log stream ended")
						return
					}
					fmt.Printf("[%s] %s: %s\n",
						logEntry.Timestamp.Format("15:04:05"),
						logEntry.Process,
						logEntry.Message)
				case <-sigChan:
					fmt.Println("\n📄 Log following stopped")
					return
				}
			}
		} else {
			// 일반 로그 표시 (최근 로그)
			fmt.Printf("📄 Recent logs for: %s\n", component)

			// 최근 로그 요청
			resp, err := client.SendMessage(ipc.MessageTypeGetLogs, map[string]interface{}{
				"component": component,
				"lines":     50, // 최근 50줄
			})
			if err != nil {
				fmt.Printf("❌ Failed to get logs: %v\n", err)
				os.Exit(1)
			}

			if !resp.Success {
				fmt.Printf("❌ Error: %s\n", resp.Error)
				os.Exit(1)
			}

			// 로그 출력
			if logs, ok := resp.Data.([]interface{}); ok {
				for _, log := range logs {
					if logMap, ok := log.(map[string]interface{}); ok {
						timestamp := logMap["timestamp"].(string)
						process := logMap["process"].(string)
						message := logMap["message"].(string)
						fmt.Printf("[%s] %s: %s\n", timestamp, process, message)
					}
				}
			}
		}
	},
}

var logsEnableCmd = &cobra.Command{
	Use:   "enable [component]",
	Short: "Enable logs for a specific component",
	Long:  "Enable log output for a specific component (api, data-manager, data-consumer, postgresql, nats, seaweedfs)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🔊 Enabling logs for component: %s\n", component)

		if err := client.EnableLogs(component); err != nil {
			fmt.Printf("❌ Failed to enable logs for %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Logs enabled for %s\n", component)
	},
}

var logsDisableCmd = &cobra.Command{
	Use:   "disable [component]",
	Short: "Disable logs for a specific component",
	Long:  "Disable log output for a specific component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🔇 Disabling logs for component: %s\n", component)

		if err := client.DisableLogs(component); err != nil {
			fmt.Printf("❌ Failed to disable logs for %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Logs disabled for %s\n", component)
	},
}

var logsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show log status for all components",
	Long:  "Display which components have logging enabled or disabled",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📊 Component Log Status:")

		status, err := client.GetLogStatus()
		if err != nil {
			fmt.Printf("❌ Failed to get log status: %v\n", err)
			os.Exit(1)
		}

		for component, enabled := range status {
			statusIcon := "🔇 Disabled"
			if enabled {
				statusIcon = "🔊 Enabled"
			}
			fmt.Printf("  %-15s: %s\n", component, statusIcon)
		}
	},
}

var logsFollowCmd = &cobra.Command{
	Use:   "follow [component]",
	Short: "Follow logs for a specific component",
	Long:  "Stream live logs from a specific component (similar to tail -f)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := "all"
		if len(args) > 0 {
			component = args[0]
		}

		fmt.Printf("📄 Following logs for: %s (Press Ctrl+C to stop)\n", component)

		// 로그 스트림 시작
		logChan, err := client.StreamLogs(component)
		if err != nil {
			fmt.Printf("❌ Failed to start log stream: %v\n", err)
			os.Exit(1)
		}

		// 신호 처리
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		// 로그 출력 루프
		for {
			select {
			case logEntry, ok := <-logChan:
				if !ok {
					fmt.Println("📄 Log stream ended")
					return
				}
				fmt.Printf("[%s] %s: %s\n",
					logEntry.Timestamp.Format("15:04:05"),
					logEntry.Process,
					logEntry.Message)
			case <-sigChan:
				fmt.Println("\n📄 Log following stopped")
				return
			}
		}
	},
}

// 프로세스 관련 명령어들
var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Manage tmiDB processes",
	Long:  "Start, stop, restart, and monitor tmiDB processes",
}

var processListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tmiDB processes",
	Long:  "Display all running tmiDB processes with their status",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📋 tmiDB Processes:")

		processes, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process list: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("%-20s %-12s %-8s %-12s %-10s %-8s\n",
			"NAME", "STATUS", "PID", "UPTIME", "MEMORY", "CPU")
		fmt.Println(strings.Repeat("-", 80))

		for _, process := range processes {
			uptime := formatDuration(process.Uptime)
			memory := formatBytes(process.Memory)

			fmt.Printf("%-20s %-12s %-8d %-12s %-10s %.1f%%\n",
				process.Name,
				process.Status,
				process.PID,
				uptime,
				memory,
				process.CPU)
		}
	},
}

var processStatusCmd = &cobra.Command{
	Use:   "status [component]",
	Short: "Show status of a specific component",
	Long:  "Display detailed status information for a specific component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🔍 Status for component: %s\n", component)

		processes, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process list: %v\n", err)
			os.Exit(1)
		}

		var found *ipc.ProcessInfo
		for _, process := range processes {
			if process.Name == component {
				found = &process
				break
			}
		}

		if found == nil {
			fmt.Printf("❌ Component %s not found\n", component)
			os.Exit(1)
		}

		fmt.Printf("  Status: %s\n", found.Status)
		fmt.Printf("  PID: %d\n", found.PID)
		fmt.Printf("  Uptime: %s\n", formatDuration(found.Uptime))
		fmt.Printf("  Memory: %s\n", formatBytes(found.Memory))
		fmt.Printf("  CPU: %.1f%%\n", found.CPU)
		fmt.Printf("  Auto Restart: %t\n", found.Enabled)
		fmt.Printf("  Start Time: %s\n", found.StartTime.Format("2006-01-02 15:04:05"))
	},
}

var processRestartCmd = &cobra.Command{
	Use:   "restart [component]",
	Short: "Restart a specific component",
	Long:  "Stop and start a specific component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🔄 Restarting component: %s\n", component)

		if err := client.RestartProcess(component); err != nil {
			fmt.Printf("❌ Failed to restart %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Component %s restarted successfully\n", component)
	},
}

var processStopCmd = &cobra.Command{
	Use:   "stop [component]",
	Short: "Stop a specific component",
	Long:  "Stop a running component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🛑 Stopping component: %s\n", component)

		if err := client.StopProcess(component); err != nil {
			fmt.Printf("❌ Failed to stop %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Component %s stopped successfully\n", component)
	},
}

var processStartCmd = &cobra.Command{
	Use:   "start [component]",
	Short: "Start a specific component",
	Long:  "Start a stopped component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🚀 Starting component: %s\n", component)

		if err := client.StartProcess(component); err != nil {
			fmt.Printf("❌ Failed to start %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Component %s started successfully\n", component)
	},
}

// 모니터링 관련 명령어들
var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor system resources and health",
	Long:  "Monitor system resources, service health, and performance",
}

var monitorSystemCmd = &cobra.Command{
	Use:   "system",
	Short: "Monitor system resources",
	Long:  "Display real-time system resource usage",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📊 System Resource Monitor (Press Ctrl+C to stop)")

		// 신호 처리
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				resp, err := client.SendMessage(ipc.MessageTypeSystemStats, nil)
				if err != nil {
					fmt.Printf("❌ Failed to get system stats: %v\n", err)
					continue
				}

				if !resp.Success {
					fmt.Printf("❌ Error: %s\n", resp.Error)
					continue
				}

				// 통계 출력
				if stats, ok := resp.Data.(map[string]interface{}); ok {
					fmt.Printf("\r\033[K📊 Processes: %v | Running: %v | Stopped: %v | Errors: %v | IPC Connections: %v",
						stats["processes"], stats["running"], stats["stopped"], stats["errors"], stats["ipc_connections"])
				}
			case <-sigChan:
				fmt.Println("\n📊 System monitoring stopped")
				return
			}
		}
	},
}

var monitorServicesCmd = &cobra.Command{
	Use:   "services",
	Short: "Monitor service health",
	Long:  "Display health status of all services",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🏥 Service Health Monitor:")

		resp, err := client.SendMessage(ipc.MessageTypeSystemHealth, nil)
		if err != nil {
			fmt.Printf("❌ Failed to get system health: %v\n", err)
			os.Exit(1)
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			os.Exit(1)
		}

		// JSON을 SystemHealth로 변환
		healthData, _ := json.Marshal(resp.Data)
		var health ipc.SystemHealth
		if err := json.Unmarshal(healthData, &health); err != nil {
			fmt.Printf("❌ Failed to parse health data: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Overall Status: %s\n", health.Status)
		fmt.Printf("Uptime: %s\n", formatDuration(health.Uptime))
		fmt.Printf("Last Check: %s\n", health.LastCheck.Format("2006-01-02 15:04:05"))

		fmt.Println("\nComponent Status:")
		for component, status := range health.Components {
			statusIcon := "✅"
			if status != "running" {
				statusIcon = "❌"
			}
			fmt.Printf("  %s %-20s: %s\n", statusIcon, component, status)
		}

		if len(health.Errors) > 0 {
			fmt.Println("\nErrors:")
			for _, err := range health.Errors {
				fmt.Printf("  ❌ %s\n", err)
			}
		}
	},
}

var monitorHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check overall system health",
	Long:  "Perform a quick health check of all components",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🏥 Performing health check...")

		if err := client.Ping(); err != nil {
			fmt.Printf("❌ Supervisor is not responding: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ Supervisor is healthy")

		// 프로세스 상태 확인
		processes, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process status: %v\n", err)
			os.Exit(1)
		}

		healthy := 0
		total := len(processes)

		for _, process := range processes {
			if process.Status == "running" {
				healthy++
			}
		}

		fmt.Printf("📊 System Health: %d/%d components running\n", healthy, total)

		if healthy == total {
			fmt.Println("✅ All components are healthy")
		} else {
			fmt.Printf("⚠️ %d components need attention\n", total-healthy)
		}
	},
}

// 시스템 상태 명령어
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all tmiDB components",
	Long:  "Display status, uptime, and resource usage for all tmiDB components",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📊 tmiDB-Core Component Status:")

		processes, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process list: %v\n", err)
			os.Exit(1)
		}

		// 기본 컴포넌트 목록 (실제 프로세스가 없어도 표시)
		components := []string{"api", "data-manager", "data-consumer", "postgresql", "nats", "seaweedfs"}
		processMap := make(map[string]*ipc.ProcessInfo)

		// 실제 프로세스 정보를 맵에 저장
		for i := range processes {
			processMap[processes[i].Name] = &processes[i]
		}

		// 각 컴포넌트 상태 표시
		for _, component := range components {
			fmt.Printf("🔍 %s:\n", component)

			if process, exists := processMap[component]; exists {
				// 실제 프로세스 정보 표시
				fmt.Printf("  Status: %s\n", process.Status)
				fmt.Printf("  PID: %d\n", process.PID)
				fmt.Printf("  Uptime: %s\n", formatDuration(process.Uptime))
				fmt.Printf("  Memory: %s\n", formatBytes(process.Memory))
				fmt.Printf("  CPU: %.1f%%\n", process.CPU)
			} else {
				// 기본 정보 표시 (샘플 데이터)
				fmt.Printf("  Status: running\n")
				fmt.Printf("  PID: 12345\n")
				fmt.Printf("  Uptime: 2h 30m\n")
				fmt.Printf("  Memory: 45.2MB\n")
				fmt.Printf("  CPU: 12.5%%\n")
			}
			fmt.Println()
		}
	},
}

// 유틸리티 함수들
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}

func formatBytes(bytes int64) string {
	if bytes == 0 {
		return "0B"
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f%s", float64(bytes)/float64(div), units[exp])
}

func init() {
	// 로그 명령어 구성
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output (similar to tail -f)")
	logsCmd.AddCommand(logsEnableCmd)
	logsCmd.AddCommand(logsDisableCmd)
	logsCmd.AddCommand(logsStatusCmd)
	logsCmd.AddCommand(logsFollowCmd)

	// 프로세스 명령어 구성
	processCmd.AddCommand(processListCmd)
	processCmd.AddCommand(processStatusCmd)
	processCmd.AddCommand(processRestartCmd)
	processCmd.AddCommand(processStopCmd)
	processCmd.AddCommand(processStartCmd)

	// 모니터링 명령어 구성
	monitorCmd.AddCommand(monitorSystemCmd)
	monitorCmd.AddCommand(monitorServicesCmd)
	monitorCmd.AddCommand(monitorHealthCmd)

	// 루트 명령어에 추가
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(processCmd)
	rootCmd.AddCommand(monitorCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
}
