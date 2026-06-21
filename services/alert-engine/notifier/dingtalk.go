// dingtalk.go 钉钉群机器人通知器
package notifier

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DingtalkConfig 钉钉机器人配置
type DingtalkConfig struct {
	WebhookURL string `json:"webhook_url"` // 钉钉 Webhook 地址
	Secret     string `json:"secret"`      // 加签密钥（安全设置中的密钥）
	AtMobiles  []string `json:"at_mobiles"` // @ 指定手机号
	AtAll      bool   `json:"at_all"`      // 是否 @所有人
}

// dingtalkNotifier 钉钉通知器
type dingtalkNotifier struct {
	cfg    DingtalkConfig
	client *http.Client
}

func newDingtalkNotifier(rawConfig json.RawMessage) (Notifier, error) {
	var cfg DingtalkConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		cfg = DingtalkConfig{}
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	return &dingtalkNotifier{cfg: cfg, client: client}, nil
}

func (d *dingtalkNotifier) Name() string {
	return "dingtalk"
}

func (d *dingtalkNotifier) Send(ctx context.Context, msg *Message) error {
	if d.cfg.WebhookURL == "" {
		return fmt.Errorf("dingtalk webhook URL not configured")
	}

	webhookURL := d.cfg.WebhookURL
	if d.cfg.Secret != "" {
		webhookURL = d.signURL(webhookURL, d.cfg.Secret)
	}

	payload, err := d.buildPayload(msg)
	if err != nil {
		return fmt.Errorf("build payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("send dingtalk: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dingtalk HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse dingtalk response: %w", err)
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("dingtalk error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// signURL 钉钉加签（HMAC-SHA256 + Base64 + URL encode）
func (d *dingtalkNotifier) signURL(webhookURL, secret string) string {
	timestamp := time.Now().UnixMilli()
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(h.Sum(nil))

	u, _ := url.Parse(webhookURL)
	q := u.Query()
	q.Set("timestamp", fmt.Sprintf("%d", timestamp))
	q.Set("sign", sign)
	u.RawQuery = q.Encode()
	return u.String()
}

func (d *dingtalkNotifier) buildPayload(msg *Message) ([]byte, error) {
	// 钉钉 markdown 消息格式

	text := fmt.Sprintf("#### CloudFlow 告警通知\n\n"+
		"> **%s**\n\n"+
		"**严重级别**: %s  \n"+
		"**租户ID**: %s  \n"+
		"**规则ID**: %s  \n"+
		"**告警ID**: %s  \n"+
		"**时间**: %s  \n\n"+
		"**告警详情**:\n%s",
		msg.Title, msg.Severity, msg.TenantID, msg.RuleID, msg.AlertID,
		time.Now().Format("2006-01-02 15:04:05"), msg.Body)

	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": msg.Title,
			"text":  text,
		},
		"at": map[string]interface{}{
			"atMobiles": d.cfg.AtMobiles,
			"isAtAll":   d.cfg.AtAll,
		},
	}
	return json.Marshal(payload)
}

func (d *dingtalkNotifier) Close() error {
	return nil
}
