// Package alerting 提供告警规则引擎和通知功能
package alerting

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud-flow-center/pkg/logger"
)

// Notifier 告警通知器接口
type Notifier interface {
	Notify(alert *Alert) error
}

// MultiNotifier 多渠道通知器
type MultiNotifier struct {
	mu        sync.Mutex
	notifiers []Notifier
	logger    *logger.Logger
}

// NewMultiNotifier 创建多渠道通知器
func NewMultiNotifier(log *logger.Logger) *MultiNotifier {
	return &MultiNotifier{
		notifiers: make([]Notifier, 0),
		logger:    log,
	}
}

// AddNotifier 添加通知器
func (mn *MultiNotifier) AddNotifier(notifier Notifier) {
	mn.mu.Lock()
	defer mn.mu.Unlock()
	mn.notifiers = append(mn.notifiers, notifier)
}

// Notify 发送告警通知
func (mn *MultiNotifier) Notify(alert *Alert) error {
	mn.mu.Lock()
	notifiers := make([]Notifier, len(mn.notifiers))
	copy(notifiers, mn.notifiers)
	mn.mu.Unlock()

	var errors []string
	for _, notifier := range notifiers {
		if err := notifier.Notify(alert); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("部分通知发送失败: %s", strings.Join(errors, "; "))
	}
	return nil
}

// WebhookNotifier Webhook 通知器
type WebhookNotifier struct {
	url     string
	logger  *logger.Logger
	client  *http.Client
}

// NewWebhookNotifier 创建 Webhook 通知器
func NewWebhookNotifier(url string, log *logger.Logger) *WebhookNotifier {
	parsedURL, err := neturl.Parse(url)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		log.Warnf("Webhook URL 无效: %s，通知器可能无法正常工作", url)
	} else if parsedURL.Scheme != "https" {
		log.Warnf("Webhook URL 使用不安全的 HTTP 协议: %s，建议使用 HTTPS", url)
	}

	return &WebhookNotifier{
		url:    url,
		logger: log,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify 发送 Webhook 通知
func (wn *WebhookNotifier) Notify(alert *Alert) error {
	payload := map[string]interface{}{
		"alert": map[string]interface{}{
			"id":        alert.ID,
			"rule_id":   alert.RuleID,
			"rule_name": alert.RuleName,
			"severity":  alert.Severity,
			"message":   alert.Message,
			"labels":    alert.Labels,
			"value":     alert.Value,
			"threshold": alert.Threshold,
			"created_at": alert.CreatedAt.Format(time.RFC3339),
			"resolved":   alert.Resolved,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 payload 失败: %w", err)
	}

	resp, err := wn.client.Post(wn.url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("发送 Webhook 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Webhook 响应失败: %s", resp.Status)
	}

	wn.logger.Infof("Webhook 通知已发送: %s", alert.Message)
	return nil
}

// EmailNotifier 邮件通知器
type EmailNotifier struct {
	smtpServer string
	smtpPort   string
	username   string
	password   string `json:"-"`
	from       string
	to         []string
	logger     *logger.Logger
}

// NewEmailNotifier 创建邮件通知器
func NewEmailNotifier(smtpServer, smtpPort, username, password, from string, to []string, log *logger.Logger) *EmailNotifier {
	return &EmailNotifier{
		smtpServer: smtpServer,
		smtpPort:   smtpPort,
		username:   username,
		password:   password,
		from:       from,
		to:         to,
		logger:     log,
	}
}

// Notify 发送邮件通知
func (en *EmailNotifier) Notify(alert *Alert) error {
	subject := fmt.Sprintf("[告警] %s", alert.RuleName)
	body := fmt.Sprintf(`告警 ID: %s
规则: %s
严重程度: %s
消息: %s
值: %.2f
阈值: %.2f
状态: %s
时间: %s
`,
		alert.ID,
		alert.RuleName,
		alert.Severity,
		alert.Message,
		alert.Value,
		alert.Threshold,
		func() string { if alert.Resolved { return "已解决" } else { return "已触发" } }(),
		alert.CreatedAt.Format("2006-01-02 15:04:05"),
	)

	message := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\n\n%s",
		en.from,
		strings.Join(en.to, ", "),
		subject,
		body,
	)

	conn, err := smtp.Dial(en.smtpServer + ":" + en.smtpPort)
	if err != nil {
		return fmt.Errorf("连接 SMTP 服务器失败: %w", err)
	}
	defer conn.Close()

	if err := conn.StartTLS(&tls.Config{
		ServerName: en.smtpServer,
		MinVersion: tls.VersionTLS12,
	}); err != nil {
		return fmt.Errorf("STARTTLS 失败: %w", err)
	}

	auth := smtp.PlainAuth("", en.username, en.password, en.smtpServer)
	if err := conn.Auth(auth); err != nil {
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}

	if err := conn.Mail(en.from); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	for _, to := range en.to {
		if err := conn.Rcpt(to); err != nil {
			return fmt.Errorf("设置收件人失败: %w", err)
		}
	}

	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("开始发送数据失败: %w", err)
	}
	_, err = w.Write([]byte(message))
	if err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	err = w.Close()
	if err != nil {
		return fmt.Errorf("关闭数据连接失败: %w", err)
	}

	err = conn.Quit()
	if err != nil {
		return fmt.Errorf("退出 SMTP 会话失败: %w", err)
	}

	en.logger.Infof("邮件通知已发送: %s", alert.Message)
	return nil
}

// DingTalkNotifier 钉钉通知器
type DingTalkNotifier struct {
	webhookURL string
	secret     string
	logger     *logger.Logger
	client     *http.Client
}

// NewDingTalkNotifier 创建钉钉通知器
func NewDingTalkNotifier(webhookURL, secret string, log *logger.Logger) *DingTalkNotifier {
	parsedURL, err := neturl.Parse(webhookURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		log.Warnf("钉钉 Webhook URL 无效: %s，通知器可能无法正常工作", webhookURL)
	} else if parsedURL.Scheme != "https" {
		log.Warnf("钉钉 Webhook URL 使用不安全的 HTTP 协议: %s，建议使用 HTTPS", webhookURL)
	}

	return &DingTalkNotifier{
		webhookURL: webhookURL,
		secret:     secret,
		logger:     log,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify 发送钉钉通知
func (dn *DingTalkNotifier) Notify(alert *Alert) error {
	requestURL := dn.webhookURL
	if dn.secret != "" {
		timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
		signature := calculateDingTalkSignature(timestamp, dn.secret)
		if strings.Contains(dn.webhookURL, "?") {
			requestURL += "&timestamp=" + timestamp + "&sign=" + neturl.QueryEscape(signature)
		} else {
			requestURL += "?timestamp=" + timestamp + "&sign=" + neturl.QueryEscape(signature)
		}
	}

	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": fmt.Sprintf("[告警] %s", alert.RuleName),
			"text": fmt.Sprintf(`### 告警通知
- **告警 ID**: %s
- **规则名称**: %s
- **严重程度**: %s
- **告警消息**: %s
- **指标值**: %.2f
- **阈值**: %.2f
- **状态**: %s
- **触发时间**: %s
`,
				alert.ID,
				alert.RuleName,
				alert.Severity,
				alert.Message,
				alert.Value,
				alert.Threshold,
				func() string { if alert.Resolved { return "已解决" } else { return "已触发" } }(),
				alert.CreatedAt.Format("2006-01-02 15:04:05"),
			),
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 payload 失败: %w", err)
	}

	resp, err := dn.client.Post(requestURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("发送钉钉请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("钉钉响应失败: %s", resp.Status)
	}

	dn.logger.Infof("钉钉通知已发送: %s", alert.Message)
	return nil
}

// calculateDingTalkSignature 计算钉钉 webhook 签名
func calculateDingTalkSignature(timestamp, secret string) string {
	message := timestamp + "\n" + secret
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return signature
}
