package ipc

import (
	"encoding/json"
	"time"
)

// MessageType IPC 메시지 타입
type MessageType string

const (
	// 로그 관련
	MessageTypeLogEnable  MessageType = "log_enable"
	MessageTypeLogDisable MessageType = "log_disable"
	MessageTypeLogStatus  MessageType = "log_status"
	MessageTypeLogStream  MessageType = "log_stream"
	MessageTypeLogConfig  MessageType = "log_config"
	MessageTypeGetLogs    MessageType = "get_logs"

	// 프로세스 관련
	MessageTypeProcessList    MessageType = "process_list"
	MessageTypeProcessStatus  MessageType = "process_status"
	MessageTypeProcessStart   MessageType = "process_start"
	MessageTypeProcessStop    MessageType = "process_stop"
	MessageTypeProcessRestart MessageType = "process_restart"

	// 시스템 관련
	MessageTypeSystemHealth MessageType = "system_health"
	MessageTypeSystemStats  MessageType = "system_stats"

	// 응답
	MessageTypeResponse MessageType = "response"
	MessageTypeError    MessageType = "error"
)

// Message IPC 메시지 구조체
type Message struct {
	ID        string                 `json:"id"`
	Type      MessageType            `json:"type"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// Response IPC 응답 구조체
type Response struct {
	ID      string      `json:"id"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// LogEntry 로그 엔트리 구조체
type LogEntry struct {
	Process   string    `json:"process"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// ProcessInfo 프로세스 정보 구조체
type ProcessInfo struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	PID       int               `json:"pid"`
	Uptime    time.Duration     `json:"uptime"`
	Memory    int64             `json:"memory"`
	CPU       float64           `json:"cpu"`
	Enabled   bool              `json:"enabled"`
	Logs      bool              `json:"logs"`
	StartTime time.Time         `json:"start_time"`
	Config    map[string]string `json:"config,omitempty"`
}

// LogConfig 로그 설정 구조체
type LogConfig struct {
	Enabled       bool          `json:"enabled"`
	Level         string        `json:"level"`          // debug, info, warn, error
	MaxFileSize   int64         `json:"max_file_size"`  // MB
	MaxFiles      int           `json:"max_files"`      // 보관할 파일 수
	MaxAge        time.Duration `json:"max_age"`        // 보관 기간
	Compress      bool          `json:"compress"`       // 압축 여부
	OutputFile    string        `json:"output_file"`    // 로그 파일 경로
	OutputConsole bool          `json:"output_console"` // 콘솔 출력 여부
	BufferSize    int           `json:"buffer_size"`    // 버퍼 크기
	FlushInterval time.Duration `json:"flush_interval"` // 플러시 간격
}

// SystemHealth 시스템 헬스 정보
type SystemHealth struct {
	Status     string            `json:"status"`
	Uptime     time.Duration     `json:"uptime"`
	Components map[string]string `json:"components"`
	Resources  SystemResources   `json:"resources"`
	LastCheck  time.Time         `json:"last_check"`
	Errors     []string          `json:"errors,omitempty"`
}

// SystemResources 시스템 리소스 정보
type SystemResources struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	NetworkIO   int64   `json:"network_io"`
	DiskIO      int64   `json:"disk_io"`
}

// NewMessage 새로운 메시지 생성
func NewMessage(msgType MessageType, data map[string]interface{}) *Message {
	return &Message{
		ID:        generateID(),
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// NewResponse 새로운 응답 생성
func NewResponse(id string, success bool, data interface{}, err string) *Response {
	return &Response{
		ID:      id,
		Success: success,
		Data:    data,
		Error:   err,
	}
}

// ToJSON 메시지를 JSON으로 변환
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON JSON에서 메시지로 변환
func (m *Message) FromJSON(data []byte) error {
	return json.Unmarshal(data, m)
}

// ToJSON 응답을 JSON으로 변환
func (r *Response) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// FromJSON JSON에서 응답으로 변환
func (r *Response) FromJSON(data []byte) error {
	return json.Unmarshal(data, r)
}

// generateID 고유 ID 생성
func generateID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

// randomString 랜덤 문자열 생성
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
