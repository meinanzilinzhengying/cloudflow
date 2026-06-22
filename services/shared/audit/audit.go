package audit

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// 常量
// ============================================================================

// Action 操作类型
type Action string

const (
	ActionLogin           Action = "login"
	ActionLogout          Action = "logout"
	ActionTokenRefresh    Action = "token_refresh"
	ActionTokenRevoke     Action = "token_revoke"
	ActionUserCreate      Action = "user_create"
	ActionUserUpdate      Action = "user_update"
	ActionUserDelete      Action = "user_delete"
	ActionRoleCreate      Action = "role_create"
	ActionRoleBind        Action = "role_bind"
	ActionRoleUnbind      Action = "role_unbind"
	ActionPolicyCreate    Action = "policy_create"
	ActionPolicyDelete    Action = "policy_delete"
	ActionAPIKeyCreate    Action = "api_key_create"
	ActionAPIKeyRevoke    Action = "api_key_revoke"
	ActionPermissionCheck Action = "permission_check"
	ActionTenantCreate    Action = "tenant_create"
	ActionTenantDelete    Action = "tenant_delete"
)

// ActorType 操作者类型
type ActorType string

const (
	ActorUser    ActorType = "user"
	ActorSystem  ActorType = "system"
	ActorAPIKey  ActorType = "api_key"
	ActorService ActorType = "service"
)

// ResourceType 资源类型
type ResourceType string

const (
	ResourceUser    ResourceType = "user"
	ResourceRole    ResourceType = "role"
	ResourcePolicy  ResourceType = "policy"
	ResourceToken   ResourceType = "token"
	ResourceAPIKey  ResourceType = "api_key"
	ResourceTenant  ResourceType = "tenant"
	ResourceProject ResourceType = "project"
)

// Status 操作状态
type Status string

const (
	StatusSuccess Status = "success"
	StatusFailure Status = "failure"
)

// ============================================================================
// Event 审计事件
// ============================================================================

// Event 审计日志事件
type Event struct {
	EventID      string       `json:"event_id"`
	Timestamp    time.Time    `json:"timestamp"`
	Action       Action       `json:"action"`
	ActorType    ActorType    `json:"actor_type"`
	ActorID      string       `json:"actor_id"`
	ActorName    string       `json:"actor_name"`
	ResourceType ResourceType `json:"resource_type"`
	ResourceID   string       `json:"resource_id"`
	TenantID     string       `json:"tenant_id"`
	Status       Status       `json:"status"`
	ErrorMessage string       `json:"error_message,omitempty"`
	ClientIP     string       `json:"client_ip,omitempty"`
	UserAgent    string       `json:"user_agent,omitempty"`
	RequestID    string       `json:"request_id,omitempty"`
	Details      interface{}  `json:"details,omitempty"`
}

// NewEvent 创建审计事件
func NewEvent(action Action, actorType ActorType, actorID string) Event {
	return Event{
		EventID:   uuid.New().String(),
		Timestamp: time.Now().UTC(),
		Action:    action,
		ActorType: actorType,
		ActorID:   actorID,
	}
}

// WithResource 设置资源信息
func (e Event) WithResource(rt ResourceType, rid string) Event {
	e.ResourceType = rt
	e.ResourceID = rid
	return e
}

// WithTenant 设置租户
func (e Event) WithTenant(tenantID string) Event {
	e.TenantID = tenantID
	return e
}

// WithStatus 设置状态
func (e Event) WithStatus(status Status, errMsg string) Event {
	e.Status = status
	e.ErrorMessage = errMsg
	return e
}

// WithClient 设置客户端信息
func (e Event) WithClient(ip, ua, reqID string) Event {
	e.ClientIP = ip
	e.UserAgent = ua
	e.RequestID = reqID
	return e
}

// WithDetails 设置详情
func (e Event) WithDetails(details interface{}) Event {
	e.Details = details
	return e
}

// ToJSON 转换为 JSON 字符串
func (e Event) ToJSON() (string, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ============================================================================
// Auditor 审计器
// ============================================================================

// Writer 审计日志写入器接口
type Writer interface {
	Write(event Event) error
	Close() error
}

// Config 审计器配置
type Config struct {
	Writer        Writer
	BufferSize    int
	FlushInterval time.Duration
	LogToStdout   bool
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		BufferSize:    1000,
		FlushInterval: 5 * time.Second,
		LogToStdout:   true,
	}
}

// Auditor 审计日志记录器
type Auditor struct {
	config   Config
	writer   Writer
	buffer   chan Event
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewAuditor 创建审计器
func NewAuditor(cfg Config) *Auditor {
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 1000
	}
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 5 * time.Second
	}

	writer := cfg.Writer
	if writer == nil {
		writer = NewStdoutWriter()
	}

	a := &Auditor{
		config: cfg,
		writer: writer,
		buffer: make(chan Event, cfg.BufferSize),
		stopCh: make(chan struct{}),
	}

	a.wg.Add(1)
	go a.flushLoop()

	return a
}

// Log 记录审计事件（异步）
func (a *Auditor) Log(event Event) {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	select {
	case a.buffer <- event:
	default:
		_ = a.writer.Write(event)
	}

	if a.config.LogToStdout {
		if jsonStr, err := event.ToJSON(); err == nil {
			log.Printf("[AUDIT] %s", jsonStr)
		}
	}
}

// LogSync 同步记录审计事件
func (a *Auditor) LogSync(event Event) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	return a.writer.Write(event)
}

// flushLoop 后台刷新循环
func (a *Auditor) flushLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case event := <-a.buffer:
			_ = a.writer.Write(event)
		case <-ticker.C:
			a.drainBuffer()
		case <-a.stopCh:
			a.drainBuffer()
			return
		}
	}
}

// drainBuffer 排空缓冲队列
func (a *Auditor) drainBuffer() {
	for {
		select {
		case event := <-a.buffer:
			_ = a.writer.Write(event)
		default:
			return
		}
	}
}

// Close 关闭审计器
func (a *Auditor) Close() error {
	a.stopOnce.Do(func() {
		close(a.stopCh)
		a.wg.Wait()
		_ = a.writer.Close()
	})
	return nil
}

// ============================================================================
// StdoutWriter 标准输出写入器
// ============================================================================

type StdoutWriter struct {
	mu sync.Mutex
}

func NewStdoutWriter() *StdoutWriter {
	return &StdoutWriter{}
}

func (w *StdoutWriter) Write(event Event) error {
	jsonStr, err := event.ToJSON()
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	fmt.Println(jsonStr)
	return nil
}

func (w *StdoutWriter) Close() error { return nil }

// ============================================================================
// FileWriter 文件写入器
// ============================================================================

type FileWriter struct {
	file *os.File
	mu   sync.Mutex
}

func NewFileWriter(path string) (*FileWriter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open audit log file: %w", err)
	}
	return &FileWriter{file: file}, nil
}

func (w *FileWriter) Write(event Event) error {
	jsonStr, err := event.ToJSON()
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = fmt.Fprintln(w.file, jsonStr)
	if err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return w.file.Sync()
}

func (w *FileWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// ExtractRequestID 从 HTTP 请求提取 Request ID
func ExtractRequestID(r *http.Request) string {
	return r.Header.Get("X-Request-ID")
}

// ExtractClientIP 从 HTTP 请求提取客户端 IP
func ExtractClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	xri := r.Header.Get("X-Real-Ip")
	if xri != "" {
		return xri
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
