// webhook.go HTTP Webhook 通知器
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookConfig Webhook 配置
type WebhookConfig struct {
	URL            string            `json:"url"`             // Webhook 地址
	Method         string            `json:"method"`          // HTTP 方法（默认 POST）
	Headers        map[string]string `json:"headers"`         // 自定义请求头
	TimeoutSec     int               `json:"timeout_sec"`     // 超时秒数（默认 30）
	RetryCount     int               `json:"retry_count"`     // 重试次数（默认 3）
	RetryInterval  int               `json:"retry_interval"`  // 重试间隔秒数（默认 5）
	SkipVerifyTLS  bool              `json:"skip_verify_tls"` // 跳过 TLS 证书验证
}

// webhookNotifier Webhook 通知器
type webhookNotifier struct {
	cfg    WebhookConfig
	client *http.Client
}

func newWebhookNotifier(rawConfig json.RawMessage) (Notifier, error) {
	var cfg WebhookConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		cfg = WebhookConfig{}
	}
	// 默认值
	if cfg.Method == "" {
		cfg.Method = "POST"
	}
	if cfg.TimeoutSec == 0 {
		cfg.TimeoutSec = 30
	}
	if cfg.RetryCount == 0 {
		cfg.RetryCount = 3
	}
	if cfg.RetryInterval == 0 {
		cfg.RetryInterval = 5
	}
	client := &http.Client{
		Timeout: time.Duration(cfg.TimeoutSec) * time.Second,
	}
	return &webhookNotifier{cfg: cfg, client: client}, nil
}

func (w *webhookNotifier) Name() string {
	return "webhook"
}

func (w *webhookNotifier) Send(ctx context.Context, msg *Message) error {
	if w.cfg.URL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	payload, err := w.buildPayload(msg)
	if err != nil {
		return fmt.Errorf("build payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= w.cfg.RetryCount; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(w.cfg.RetryInterval) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		lastErr = w.sendOnce(ctx, payload)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("webhook failed after %d retries: %w", w.cfg.RetryCount, lastErr)
}

func (w *webhookNotifier) sendOnce(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, w.cfg.Method, w.cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "CloudFlow-AlertEngine/1.0")
	for k, v := range w.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (w *webhookNotifier) buildPayload(msg *Message) ([]byte, error) {
	payload := map[string]interface{}{
		"event":       "alert",
		"title":       msg.Title,
		"body":        msg.Body,
		"severity":    msg.Severity,
		"tenant_id":   msg.TenantID,
		"rule_id":     msg.RuleID,
		"alert_id":    msg.AlertID,
		"labels":      msg.Labels,
		"timestamp":   time.Now().Unix(),
		"source":      "cloudflow-alert-engine",
	}
	return json.Marshal(payload)
}

func (w *webhookNotifier) Close() error {
	return nil
}
