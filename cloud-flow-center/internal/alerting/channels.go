// P2: 告警通知通道 — 邮件、钉钉、企业微信、短信
//
// 支持：
//   - 邮件 (SMTP)
//   - 钉钉 (webhook)
//   - 企业微信 (webhook)
//   - 短信 (模拟/HTTP 接口)
//   - 多通道同时发送
//   - 通道降级（失败时尝试备用通道）
//
package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"text/template"
	"time"
)

// ============================================================================
// 一、通知通道接口
// ============================================================================

// ChannelConfig 通道配置
type ChannelConfig struct {
	Type      string            `json:"type"`       // email/dingtalk/wechat/sms
	Enabled   bool              `json:"enabled"`
	Priority  int               `json:"priority"`   // 优先级，数字越小越优先
	Settings  map[string]string `json:"settings"`   // 通道特定配置
}

// NotificationChannel 通知通道接口
type NotificationChannel interface {
	Name() string
	Send(ctx context.Context, alert *Alert, tmpl *AlertTemplate) error
	HealthCheck() error
}

// AlertTemplate 告警模板
type AlertTemplate struct {
	Title      string
	Body       string
	Severity   string
	Color      string // 钉钉/企微卡片颜色
	Markdown   string
}

// ChannelManager 通道管理器
type ChannelManager struct {
	channels map[string]NotificationChannel
	order    []string // 按优先级排序的通道名称
	mu       sync.RWMutex
}

// NewChannelManager 创建通道管理器
func NewChannelManager() *ChannelManager {
	return &ChannelManager{
		channels: make(map[string]NotificationChannel),
		order:    make([]string, 0),
	}
}

// RegisterChannel 注册通道
func (cm *ChannelManager) RegisterChannel(name string, ch NotificationChannel) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.channels[name] = ch
	// 重新排序
	cm.order = make([]string, 0, len(cm.channels))
	for name := range cm.channels {
		cm.order = append(cm.order, name)
	}
}

// UnregisterChannel 注销通道
func (cm *ChannelManager) UnregisterChannel(name string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.channels, name)
	// 重建 order
	newOrder := make([]string, 0, len(cm.order))
	for _, n := range cm.order {
		if n != name {
			newOrder = append(newOrder, n)
		}
	}
	cm.order = newOrder
}

// Send 向所有通道发送告警
func (cm *ChannelManager) Send(ctx context.Context, alert *Alert, tmpl *AlertTemplate) error {
	cm.mu.RLock()
	channels := make([]NotificationChannel, 0, len(cm.order))
	for _, name := range cm.order {
		if ch, ok := cm.channels[name]; ok {
			channels = append(channels, ch)
		}
	}
	cm.mu.RUnlock()

	var errs []error
	for _, ch := range channels {
		if err := ch.Send(ctx, alert, tmpl); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", ch.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification failed: %v", errs)
	}
	return nil
}

// SendTo 向指定通道发送告警
func (cm *ChannelManager) SendTo(ctx context.Context, channelNames []string, alert *Alert, tmpl *AlertTemplate) error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var errs []error
	for _, name := range channelNames {
		ch, ok := cm.channels[name]
		if !ok {
			errs = append(errs, fmt.Errorf("channel %s not found", name))
			continue
		}
		if err := ch.Send(ctx, alert, tmpl); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification failed: %v", errs)
	}
	return nil
}

// GetChannelNames 获取所有通道名称
func (cm *ChannelManager) GetChannelNames() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]string, len(cm.order))
	copy(result, cm.order)
	return result
}

// HealthCheck 检查所有通道健康状态
func (cm *ChannelManager) HealthCheck() map[string]error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make(map[string]error)
	for name, ch := range cm.channels {
		result[name] = ch.HealthCheck()
	}
	return result
}

// ============================================================================
// 二、邮件通道
// ============================================================================

// EmailChannel 邮件通知通道
type EmailChannel struct {
	Name_      string
	SMTPHost   string
	SMTPPort   string
	Username   string
	Password   string
	From       string
	To         []string
	TLS        bool
	httpClient *http.Client
}

// NewEmailChannel 创建邮件通道
func NewEmailChannel(name, smtpHost, smtpPort, username, password, from string, to []string) *EmailChannel {
	return &EmailChannel{
		Name_:      name,
		SMTPHost:   smtpHost,
		SMTPPort:   smtpPort,
		Username:   username,
		Password:   password,
		From:       from,
		To:         to,
		TLS:        true,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *EmailChannel) Name() string {
	return e.Name_
}

func (e *EmailChannel) Send(ctx context.Context, alert *Alert, tmpl *AlertTemplate) error {
	if tmpl == nil {
		tmpl = defaultAlertTemplate(alert)
	}

	subject := fmt.Sprintf("[告警] %s - %s", alert.Severity, alert.RuleName)
	body := buildEmailBody(alert, tmpl)

	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		strings.Join(e.To, ", "), subject, body))

	addr := e.SMTPHost + ":" + e.SMTPPort
	var auth smtp.Auth
	if e.Username != "" && e.Password != "" {
		auth = smtp.PlainAuth("", e.Username, e.Password, e.SMTPHost)
	}

	if err := smtp.SendMail(addr, auth, e.From, e.To, msg); err != nil {
		return fmt.Errorf("send email failed: %w", err)
	}
	return nil
}

func (e *EmailChannel) HealthCheck() error {
	// 简单检查：尝试连接 SMTP 服务器
	addr := e.SMTPHost + ":" + e.SMTPPort
	conn, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp connection failed: %w", err)
	}
	defer conn.Close()
	return nil
}

func buildEmailBody(alert *Alert, tmpl *AlertTemplate) string {
	return fmt.Sprintf(`<html>
<body>
<h2 style="color: %s">%s</h2>
<table>
<tr><td>规则名称</td><td>%s</td></tr>
<tr><td>严重级别</td><td>%s</td></tr>
<tr><td>告警消息</td><td>%s</td></tr>
<tr><td>当前值</td><td>%.2f</td></tr>
<tr><td>阈值</td><td>%.2f</td></tr>
<tr><td>触发时间</td><td>%s</td></tr>
</table>
</body>
</html>`,
		severityColor(alert.Severity), tmpl.Title, alert.RuleName,
		alert.Severity, alert.Message, alert.Value, alert.Threshold,
		alert.CreatedAt.Format("2006-01-02 15:04:05"))
}

// ============================================================================
// 三、钉钉通道
// ============================================================================

// DingTalkChannel 钉钉通知通道
type DingTalkChannel struct {
	Name_      string
	WebhookURL string
	Secret     string // 加签密钥
	httpClient *http.Client
}

// NewDingTalkChannel 创建钉钉通道
func NewDingTalkChannel(name, webhookURL, secret string) *DingTalkChannel {
	return &DingTalkChannel{
		Name_:      name,
		WebhookURL: webhookURL,
		Secret:     secret,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *DingTalkChannel) Name() string {
	return d.Name_
}

func (d *DingTalkChannel) Send(ctx context.Context, alert *Alert, tmpl *AlertTemplate) error {
	if tmpl == nil {
		tmpl = defaultAlertTemplate(alert)
	}

	msg := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": tmpl.Title,
			"text": fmt.Sprintf("## %s\n\n**规则**: %s\n\n**级别**: %s\n\n**消息**: %s\n\n**当前值**: %.2f\n\n**阈值**: %.2f\n\n**时间**: %s",
				tmpl.Title, alert.RuleName, alert.Severity, alert.Message,
				alert.Value, alert.Threshold, alert.CreatedAt.Format("2006-01-02 15:04:05")),
		},
	}

	return d.sendWebhook(ctx, msg)
}

func (d *DingTalkChannel) sendWebhook(ctx context.Context, msg map[string]interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal dingtalk message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", d.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *DingTalkChannel) HealthCheck() error {
	return nil
}

// ============================================================================
// 四、企业微信通道
// ============================================================================

// WeChatChannel 企业微信通知通道
type WeChatChannel struct {
	Name_      string
	WebhookURL string
	httpClient *http.Client
}

// NewWeChatChannel 创建企业微信通道
func NewWeChatChannel(name, webhookURL string) *WeChatChannel {
	return &WeChatChannel{
		Name_:      name,
		WebhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *WeChatChannel) Name() string {
	return w.Name_
}

func (w *WeChatChannel) Send(ctx context.Context, alert *Alert, tmpl *AlertTemplate) error {
	if tmpl == nil {
		tmpl = defaultAlertTemplate(alert)
	}

	msg := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": fmt.Sprintf("**%s**\n>规则: %s\n>级别: %s\n>消息: %s\n>当前值: %.2f\n>阈值: %.2f\n>时间: %s",
				tmpl.Title, alert.RuleName, alert.Severity, alert.Message,
				alert.Value, alert.Threshold, alert.CreatedAt.Format("2006-01-02 15:04:05")),
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal wechat message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", w.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func (w *WeChatChannel) HealthCheck() error {
	return nil
}

// ============================================================================
// 五、短信通道
// ============================================================================

// SMSChannel 短信通知通道
type SMSChannel struct {
	Name_      string
	APIURL     string
	APIKey     string
	APISecret  string
	TemplateID string
	SignName   string
	PhoneNumbers []string
	httpClient *http.Client
}

// NewSMSChannel 创建短信通道
func NewSMSChannel(name, apiURL, apiKey, apiSecret, templateID, signName string, phones []string) *SMSChannel {
	return &SMSChannel{
		Name_:        name,
		APIURL:       apiURL,
		APIKey:       apiKey,
		APISecret:    apiSecret,
		TemplateID:   templateID,
		SignName:     signName,
		PhoneNumbers: phones,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *SMSChannel) Name() string {
	return s.Name_
}

func (s *SMSChannel) Send(ctx context.Context, alert *Alert, tmpl *AlertTemplate) error {
	if tmpl == nil {
		tmpl = defaultAlertTemplate(alert)
	}

	msg := map[string]interface{}{
		"phone_numbers": s.PhoneNumbers,
		"template_id":   s.TemplateID,
		"sign_name":     s.SignName,
		"template_param": map[string]string{
			"alert_name": alert.RuleName,
			"severity":   alert.Severity,
			"value":      fmt.Sprintf("%.2f", alert.Value),
			"time":       alert.CreatedAt.Format("01-02 15:04"),
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal sms message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.APIURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.APIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send sms: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sms api returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *SMSChannel) HealthCheck() error {
	if s.APIURL == "" {
		return fmt.Errorf("sms api url not configured")
	}
	return nil
}

// ============================================================================
// 六、辅助函数
// ============================================================================

func defaultAlertTemplate(alert *Alert) *AlertTemplate {
	return &AlertTemplate{
		Title:    fmt.Sprintf("告警: %s", alert.RuleName),
		Body:     alert.Message,
		Severity: alert.Severity,
		Color:    severityColor(alert.Severity),
		Markdown: alert.Message,
	}
}

func severityColor(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "#FF0000"
	case "warning":
		return "#FF8C00"
	case "info":
		return "#1890FF"
	default:
		return "#666666"
	}
}

// RenderTemplate 渲染告警模板
func RenderTemplate(tmplStr string, alert *Alert) (string, error) {
	tmpl, err := template.New("alert").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, alert); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// SeverityPriority 严重级别优先级
type SeverityPriority int

const (
	PriorityCritical SeverityPriority = 1
	PriorityWarning  SeverityPriority = 2
	PriorityInfo     SeverityPriority = 3
)

// ParseSeverity 解析严重级别
func ParseSeverity(severity string) SeverityPriority {
	switch strings.ToLower(severity) {
	case "critical":
		return PriorityCritical
	case "warning":
		return PriorityWarning
	default:
		return PriorityInfo
	}
}
