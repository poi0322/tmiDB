package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"
)

// Client IPC 클라이언트 구조체
type Client struct {
	socketPath  string
	conn        net.Conn
	reader      *bufio.Reader
	writer      *bufio.Writer
	responses   map[string]chan *Response
	responseMux sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	connected   bool
	connMux     sync.RWMutex

	// Go 1.24 기능: 자원 관리
	cleanup func()
}

// NewClient 새로운 IPC 클라이언트 생성
func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		socketPath: socketPath,
		responses:  make(map[string]chan *Response),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Go 1.24 기능: 클라이언트 정리를 위한 finalizer 설정
	client.cleanup = func() {
		client.Close()
	}
	runtime.SetFinalizer(client, func(c *Client) {
		if c.cleanup != nil {
			c.cleanup()
		}
	})

	return client
}

// Connect 서버에 연결
func (c *Client) Connect() error {
	c.connMux.Lock()
	defer c.connMux.Unlock()

	if c.connected {
		return nil
	}

	// Unix Domain Socket 연결
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to supervisor: %w", err)
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.writer = bufio.NewWriter(conn)
	c.connected = true

	// 응답 처리 고루틴 시작
	go c.handleResponses()

	return nil
}

// Close 연결 종료
func (c *Client) Close() error {
	c.cancel()

	c.connMux.Lock()
	defer c.connMux.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.connected = false
	}

	// 모든 대기 중인 응답 채널 닫기
	c.responseMux.Lock()
	for _, ch := range c.responses {
		close(ch)
	}
	c.responses = make(map[string]chan *Response)
	c.responseMux.Unlock()

	return nil
}

// SendMessage 메시지 전송
func (c *Client) SendMessage(msgType MessageType, data map[string]interface{}) (*Response, error) {
	if !c.isConnected() {
		if err := c.Connect(); err != nil {
			return nil, err
		}
	}

	msg := NewMessage(msgType, data)

	// 응답 채널 생성
	respChan := make(chan *Response, 1)
	c.responseMux.Lock()
	c.responses[msg.ID] = respChan
	c.responseMux.Unlock()

	// 응답 채널 정리 함수
	defer func() {
		c.responseMux.Lock()
		delete(c.responses, msg.ID)
		c.responseMux.Unlock()
		close(respChan)
	}()

	// 메시지 전송
	if err := c.sendMessage(msg); err != nil {
		return nil, err
	}

	// 응답 대기 (타임아웃 포함)
	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("request timeout")
	case <-c.ctx.Done():
		return nil, fmt.Errorf("client closed")
	}
}

// SendMessageAsync 비동기 메시지 전송
func (c *Client) SendMessageAsync(msgType MessageType, data map[string]interface{}) error {
	if !c.isConnected() {
		if err := c.Connect(); err != nil {
			return err
		}
	}

	msg := NewMessage(msgType, data)
	return c.sendMessage(msg)
}

// StreamLogs 로그 스트림 시작
func (c *Client) StreamLogs(component string) (<-chan LogEntry, error) {
	if !c.isConnected() {
		if err := c.Connect(); err != nil {
			return nil, err
		}
	}

	// 로그 스트림 요청
	data := map[string]interface{}{
		"component": component,
		"action":    "start",
	}

	resp, err := c.SendMessage(MessageTypeLogStream, data)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("failed to start log stream: %s", resp.Error)
	}

	// 로그 엔트리 채널 생성
	logChan := make(chan LogEntry, 100)

	// 로그 스트림 처리 고루틴 시작
	go c.handleLogStream(logChan)

	return logChan, nil
}

// sendMessage 실제 메시지 전송
func (c *Client) sendMessage(msg *Message) error {
	c.connMux.RLock()
	defer c.connMux.RUnlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// JSON 직렬화
	data, err := msg.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 쓰기 타임아웃 설정
	c.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))

	// 메시지 전송 (개행 문자 추가)
	_, err = c.writer.Write(append(data, '\n'))
	if err != nil {
		c.connected = false
		return fmt.Errorf("failed to send message: %w", err)
	}

	return c.writer.Flush()
}

// handleResponses 응답 처리
func (c *Client) handleResponses() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.connMux.RLock()
		if !c.connected || c.conn == nil {
			c.connMux.RUnlock()
			return
		}

		// 읽기 타임아웃 설정
		c.conn.SetReadDeadline(time.Now().Add(ReadTimeout))

		// 응답 읽기
		line, err := c.reader.ReadString('\n')
		c.connMux.RUnlock()

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // 타임아웃은 정상적인 상황
			}
			// 연결 오류
			c.connMux.Lock()
			c.connected = false
			c.connMux.Unlock()
			return
		}

		// 응답 파싱
		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue // 파싱 오류 무시
		}

		// 해당 응답 채널로 전송
		c.responseMux.RLock()
		if respChan, exists := c.responses[resp.ID]; exists {
			select {
			case respChan <- &resp:
			default:
				// 채널이 가득 찬 경우 무시
			}
		}
		c.responseMux.RUnlock()
	}
}

// handleLogStream 로그 스트림 처리
func (c *Client) handleLogStream(logChan chan<- LogEntry) {
	defer close(logChan)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.connMux.RLock()
		if !c.connected || c.conn == nil {
			c.connMux.RUnlock()
			return
		}

		// 로그 엔트리 읽기
		line, err := c.reader.ReadString('\n')
		c.connMux.RUnlock()

		if err != nil {
			return
		}

		// 로그 엔트리 파싱 시도
		var logEntry LogEntry
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			continue // 로그가 아닌 다른 메시지일 수 있음
		}

		// 로그 채널로 전송
		select {
		case logChan <- logEntry:
		case <-c.ctx.Done():
			return
		default:
			// 버퍼가 가득 찬 경우 무시
		}
	}
}

// isConnected 연결 상태 확인
func (c *Client) isConnected() bool {
	c.connMux.RLock()
	defer c.connMux.RUnlock()

	return c.connected
}

// Ping 서버 연결 확인
func (c *Client) Ping() error {
	resp, err := c.SendMessage(MessageTypeSystemHealth, nil)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("ping failed: %s", resp.Error)
	}

	return nil
}

// 편의 메서드들

// EnableLogs 로그 활성화
func (c *Client) EnableLogs(component string) error {
	data := map[string]interface{}{
		"component": component,
		"enabled":   true,
	}

	resp, err := c.SendMessage(MessageTypeLogEnable, data)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to enable logs: %s", resp.Error)
	}

	return nil
}

// DisableLogs 로그 비활성화
func (c *Client) DisableLogs(component string) error {
	data := map[string]interface{}{
		"component": component,
		"enabled":   false,
	}

	resp, err := c.SendMessage(MessageTypeLogDisable, data)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to disable logs: %s", resp.Error)
	}

	return nil
}

// GetLogStatus 로그 상태 조회
func (c *Client) GetLogStatus() (map[string]bool, error) {
	resp, err := c.SendMessage(MessageTypeLogStatus, nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("failed to get log status: %s", resp.Error)
	}

	// 응답 데이터를 map[string]bool로 변환
	if statusMap, ok := resp.Data.(map[string]interface{}); ok {
		result := make(map[string]bool)
		for k, v := range statusMap {
			if boolVal, ok := v.(bool); ok {
				result[k] = boolVal
			}
		}
		return result, nil
	}

	return nil, fmt.Errorf("invalid response format")
}

// GetProcessList 프로세스 목록 조회
func (c *Client) GetProcessList() ([]ProcessInfo, error) {
	resp, err := c.SendMessage(MessageTypeProcessList, nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("failed to get process list: %s", resp.Error)
	}

	// 응답 데이터를 []ProcessInfo로 변환
	if dataList, ok := resp.Data.([]interface{}); ok {
		var processes []ProcessInfo
		for _, item := range dataList {
			if itemMap, ok := item.(map[string]interface{}); ok {
				// JSON 재직렬화/역직렬화를 통한 변환
				jsonData, _ := json.Marshal(itemMap)
				var process ProcessInfo
				if json.Unmarshal(jsonData, &process) == nil {
					processes = append(processes, process)
				}
			}
		}
		return processes, nil
	}

	return nil, fmt.Errorf("invalid response format")
}

// RestartProcess 프로세스 재시작
func (c *Client) RestartProcess(component string) error {
	data := map[string]interface{}{
		"component": component,
	}

	resp, err := c.SendMessage(MessageTypeProcessRestart, data)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to restart process: %s", resp.Error)
	}

	return nil
}

// StopProcess 프로세스 정지
func (c *Client) StopProcess(component string) error {
	data := map[string]interface{}{
		"component": component,
	}

	resp, err := c.SendMessage(MessageTypeProcessStop, data)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to stop process: %s", resp.Error)
	}

	return nil
}

// StartProcess 프로세스 시작
func (c *Client) StartProcess(component string) error {
	data := map[string]interface{}{
		"component": component,
	}

	resp, err := c.SendMessage(MessageTypeProcessStart, data)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to start process: %s", resp.Error)
	}

	return nil
}
