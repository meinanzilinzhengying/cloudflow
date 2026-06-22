// email.go SMTP 邮件通知器
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

// EmailConfig SMTP 邮件配置
type EmailConfig struct {
	SMTPHost     string   `json:"smtp_host"`     // SMTP 服务器地址
	SMTPPort     int      `json:"smtp_port"`     // SMTP 端口（默认 587）
	SMTPUser     string   `json:"smtp_user"`     // SMTP 用户名
	SMTPPassword string   `json:"smtp_password"` // SMTP 密码/授权码
	FromAddr     string   `json:"from_addr"`     // 发件人地址
	FromName     string   `json:"from_name"`     // 发件人显示名称
	ToAddrs      []string `json:"to_addrs"`      // 收件人列表
	CCAddrs      []string `json:"cc_addrs"`      // 抄送列表
	TLSRequired  bool     `json:"tls_required"`  // 是否强制 TLS（默认 true）
}

// emailNotifier 邮件通知器
type emailNotifier struct {
	cfg    EmailConfig
	client *smtp.Client // 复用连接（可选）
}

func newEmailNotifier(rawConfig json.RawMessage) (Notifier, error) {
	var cfg EmailConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		// 空配置也允许，但发送会失败
		cfg = EmailConfig{}
	}
	// 默认值
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}
	if cfg.FromAddr == "" && cfg.SMTPUser != "" {
		cfg.FromAddr = cfg.SMTPUser
	}
	return &emailNotifier{cfg: cfg}, nil
}

func (e *emailNotifier) Name() string {
	return "email"
}

func (e *emailNotifier) Send(ctx context.Context, msg *Message) error {
	if len(e.cfg.ToAddrs) == 0 {
		return fmt.Errorf("no recipient configured")
	}
	if e.cfg.SMTPHost == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	body := e.buildBody(msg)
	addr := fmt.Sprintf("%s:%d", e.cfg.SMTPHost, e.cfg.SMTPPort)

	// 构建邮件头
	from := fmt.Sprintf("%s <%s>", e.cfg.FromName, e.cfg.FromAddr)
	if e.cfg.FromName == "" {
		from = e.cfg.FromAddr
	}
	subject := fmt.Sprintf("[CloudFlow Alert] %s - %s", msg.Severity, msg.Title)

	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = strings.Join(e.cfg.ToAddrs, ", ")
	if len(e.cfg.CCAddrs) > 0 {
		headers["Cc"] = strings.Join(e.cfg.CCAddrs, ", ")
	}
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/plain; charset=\"utf-8\""
	headers["Date"] = time.Now().Format(time.RFC1123)

	var headerLines []string
	for k, v := range headers {
		headerLines = append(headerLines, fmt.Sprintf("%s: %s", k, v))
	}
	fullBody := strings.Join(headerLines, "\r\n") + "\r\n\r\n" + body

	// 支持 TLS 端口（465/587）和明文端口（25）
	// 简化：使用 smtp.SendMail 直接发送
	auth := smtp.PlainAuth("", e.cfg.SMTPUser, e.cfg.SMTPPassword, e.cfg.SMTPHost)

	allRecipients := append([]string{}, e.cfg.ToAddrs...)
	allRecipients = append(allRecipients, e.cfg.CCAddrs...)

	// 带超时的发送（使用 goroutine + select 模拟）
	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, e.cfg.FromAddr, allRecipients, []byte(fullBody))
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *emailNotifier) buildBody(msg *Message) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("告警标题: %s\n", msg.Title))
	sb.WriteString(fmt.Sprintf("严重级别: %s\n", msg.Severity))
	sb.WriteString(fmt.Sprintf("租户ID: %s\n", msg.TenantID))
	sb.WriteString(fmt.Sprintf("规则ID: %s\n", msg.RuleID))
	sb.WriteString(fmt.Sprintf("告警ID: %s\n", msg.AlertID))
	sb.WriteString(fmt.Sprintf("告警时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("\n告警详情:\n")
	sb.WriteString(msg.Body)
	if len(msg.Labels) > 0 {
		sb.WriteString("\n\n标签:\n")
		for k, v := range msg.Labels {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}
	return sb.String()
}

func (e *emailNotifier) Close() error {
	return nil
}
