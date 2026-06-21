// notifier_test.go 通知渠道测试
package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 一、工厂测试
// ============================================================================

func TestNewFactory(t *testing.T) {
	f := NewFactory()
	assert.NotNil(t, f)
	assert.NotNil(t, f.creators)
	// 内置 4 个渠道
	assert.NotNil(t, f.creators["console"])
	assert.NotNil(t, f.creators["email"])
	assert.NotNil(t, f.creators["webhook"])
	assert.NotNil(t, f.creators["dingtalk"])
}

func TestFactory_Create(t *testing.T) {
	f := NewFactory()

	// console
	n, err := f.Create("console", nil)
	require.NoError(t, err)
	assert.Equal(t, "console", n.Name())

	// email
	n, err = f.Create("email", nil)
	require.NoError(t, err)
	assert.Equal(t, "email", n.Name())

	// webhook
	n, err = f.Create("webhook", nil)
	require.NoError(t, err)
	assert.Equal(t, "webhook", n.Name())

	// dingtalk
	n, err = f.Create("dingtalk", nil)
	require.NoError(t, err)
	assert.Equal(t, "dingtalk", n.Name())

	// unsupported
	_, err = f.Create("unknown", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestFactory_CreateMulti(t *testing.T) {
	f := NewFactory()
	configs := []ChannelConfig{
		{Type: "console"},
		{Type: "webhook"},
		{Type: "unknown"}, // 会被忽略并记录错误
	}
	notifiers, errs, err := f.CreateMulti(configs)
	require.NoError(t, err)
	assert.Len(t, notifiers, 2)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0], "unknown")
}

// ============================================================================
// 二、ParseChannels 测试
// ============================================================================

func TestParseChannels_Empty(t *testing.T) {
	configs, err := ParseChannels("")
	require.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Equal(t, "console", configs[0].Type)
}

func TestParseChannels_Null(t *testing.T) {
	configs, err := ParseChannels("null")
	require.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Equal(t, "console", configs[0].Type)
}

func TestParseChannels_Single(t *testing.T) {
	configs, err := ParseChannels(`{"type":"email","name":"运维邮箱"}`)
	require.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Equal(t, "email", configs[0].Type)
	assert.Equal(t, "运维邮箱", configs[0].Name)
}

func TestParseChannels_List(t *testing.T) {
	raw := `[{"type":"email","name":"运维邮箱"},{"type":"webhook","name":"告警平台"}]`
	configs, err := ParseChannels(raw)
	require.NoError(t, err)
	assert.Len(t, configs, 2)
	assert.Equal(t, "email", configs[0].Type)
	assert.Equal(t, "webhook", configs[1].Type)
}

func TestParseChannels_InvalidJSON(t *testing.T) {
	configs, err := ParseChannels("{invalid")
	require.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Equal(t, "console", configs[0].Type)
}

// ============================================================================
// 三、Console 通知器测试
// ============================================================================

func TestConsoleNotifier(t *testing.T) {
	n, err := newConsoleNotifier(nil)
	require.NoError(t, err)
	assert.Equal(t, "console", n.Name())

	msg := &Message{
		Title:    "CPU 使用率过高",
		Body:     "主机 192.168.1.1 CPU 使用率达到 95%",
		Severity: "critical",
		TenantID: "tenant-1",
		RuleID:   "rule-1",
		AlertID:  "alert-1",
	}
	ctx := context.Background()
	err = n.Send(ctx, msg)
	assert.NoError(t, err)

	assert.NoError(t, n.Close())
}

// ============================================================================
// 四、Email 通知器测试
// ============================================================================

func TestEmailNotifier_ConfigDefaults(t *testing.T) {
	n, err := newEmailNotifier(nil)
	require.NoError(t, err)
	assert.Equal(t, "email", n.Name())
	assert.NoError(t, n.Close())
}

func TestEmailNotifier_Send_NoConfig(t *testing.T) {
	n, err := newEmailNotifier(nil)
	require.NoError(t, err)
	msg := &Message{Title: "Test", Body: "Test body", Severity: "info"}
	err = n.Send(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no recipient")
}

func TestEmailNotifier_Send_NoHost(t *testing.T) {
	cfg, _ := json.Marshal(EmailConfig{ToAddrs: []string{"test@example.com"}})
	n, err := newEmailNotifier(cfg)
	require.NoError(t, err)
	msg := &Message{Title: "Test", Body: "Test body", Severity: "info"}
	err = n.Send(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP host")
}

func TestEmailNotifier_BuildBody(t *testing.T) {
	n, _ := newEmailNotifier(nil)
	msg := &Message{
		Title:    "CPU 告警",
		Body:     "CPU 95%",
		Severity:   "critical",
		TenantID: "t1",
		RuleID:   "r1",
		AlertID:  "a1",
		Labels:   map[string]string{"host": "192.168.1.1"},
	}
	body := n.(*emailNotifier).buildBody(msg)
	assert.Contains(t, body, "CPU 告警")
	assert.Contains(t, body, "critical")
	assert.Contains(t, body, "host: 192.168.1.1")
}

// ============================================================================
// 五、Webhook 通知器测试
// ============================================================================

func TestWebhookNotifier_ConfigDefaults(t *testing.T) {
	n, err := newWebhookNotifier(nil)
	require.NoError(t, err)
	assert.Equal(t, "webhook", n.Name())
	assert.Equal(t, 30, n.(*webhookNotifier).cfg.TimeoutSec)
	assert.Equal(t, 3, n.(*webhookNotifier).cfg.RetryCount)
	assert.Equal(t, 5, n.(*webhookNotifier).cfg.RetryInterval)
	assert.Equal(t, "POST", n.(*webhookNotifier).cfg.Method)
	assert.NoError(t, n.Close())
}

func TestWebhookNotifier_Send_NoURL(t *testing.T) {
	n, err := newWebhookNotifier(nil)
	require.NoError(t, err)
	msg := &Message{Title: "Test", Body: "Test body", Severity: "info"}
	err = n.Send(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "URL not configured")
}

func TestWebhookNotifier_Send_Success(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	cfg, _ := json.Marshal(WebhookConfig{URL: server.URL, RetryCount: 0})
	n, err := newWebhookNotifier(cfg)
	require.NoError(t, err)

	msg := &Message{
		Title:    "CPU 告警",
		Body:     "CPU 95%",
		Severity:   "critical",
		TenantID: "t1",
		RuleID:   "r1",
		AlertID:  "a1",
	}
	err = n.Send(context.Background(), msg)
	assert.NoError(t, err)
	assert.Equal(t, "application/json", receivedContentType)
	assert.Contains(t, string(receivedBody), "CPU 告警")
	assert.Contains(t, string(receivedBody), "cloudflow-alert-engine")
}

func TestWebhookNotifier_Send_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	cfg, _ := json.Marshal(WebhookConfig{URL: server.URL, RetryCount: 0, TimeoutSec: 5})
	n, err := newWebhookNotifier(cfg)
	require.NoError(t, err)

	msg := &Message{Title: "Test", Body: "Test", Severity: "info"}
	err = n.Send(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestWebhookNotifier_BuildPayload(t *testing.T) {
	n, _ := newWebhookNotifier(nil)
	msg := &Message{
		Title:    "Test Alert",
		Body:     "Body content",
		Severity:   "warning",
		TenantID: "t1",
		RuleID:   "r1",
		AlertID:  "a1",
		Labels:   map[string]string{"host": "192.168.1.1"},
	}
	payload, err := n.(*webhookNotifier).buildPayload(msg)
	require.NoError(t, err)
	var data map[string]interface{}
	err = json.Unmarshal(payload, &data)
	require.NoError(t, err)
	assert.Equal(t, "alert", data["event"])
	assert.Equal(t, "Test Alert", data["title"])
	assert.Equal(t, "warning", data["severity"])
	assert.Equal(t, "t1", data["tenant_id"])
}

// ============================================================================
// 六、钉钉通知器测试
// ============================================================================

func TestDingtalkNotifier_ConfigDefaults(t *testing.T) {
	n, err := newDingtalkNotifier(nil)
	require.NoError(t, err)
	assert.Equal(t, "dingtalk", n.Name())
	assert.NoError(t, n.Close())
}

func TestDingtalkNotifier_Send_NoURL(t *testing.T) {
	n, err := newDingtalkNotifier(nil)
	require.NoError(t, err)
	msg := &Message{Title: "Test", Body: "Test body", Severity: "info"}
	err = n.Send(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook URL not configured")
}

func TestDingtalkNotifier_Send_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证加签参数
		q := r.URL.Query()
		assert.NotEmpty(t, q.Get("timestamp"))
		assert.NotEmpty(t, q.Get("sign"))

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	cfg, _ := json.Marshal(DingtalkConfig{WebhookURL: server.URL, Secret: "test-secret"})
	n, err := newDingtalkNotifier(cfg)
	require.NoError(t, err)

	msg := &Message{
		Title:    "CPU 告警",
		Body:     "CPU 95%",
		Severity:   "critical",
		TenantID: "t1",
		RuleID:   "r1",
		AlertID:  "a1",
	}
	err = n.Send(context.Background(), msg)
	assert.NoError(t, err)
}

func TestDingtalkNotifier_Send_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":310000,"errmsg":"invalid timestamp"}`))
	}))
	defer server.Close()

	cfg, _ := json.Marshal(DingtalkConfig{WebhookURL: server.URL})
	n, err := newDingtalkNotifier(cfg)
	require.NoError(t, err)

	msg := &Message{Title: "Test", Body: "Test", Severity: "info"}
	err = n.Send(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "310000")
}

func TestDingtalkNotifier_BuildPayload(t *testing.T) {
	cfg, _ := json.Marshal(DingtalkConfig{AtMobiles: []string{"13800138000"}, AtAll: true})
	n, _ := newDingtalkNotifier(cfg)
	msg := &Message{
		Title:    "Test Alert",
		Body:     "Body content",
		Severity:   "warning",
		TenantID: "t1",
		RuleID:   "r1",
		AlertID:  "a1",
	}
	payload, err := n.(*dingtalkNotifier).buildPayload(msg)
	require.NoError(t, err)
	var data map[string]interface{}
	err = json.Unmarshal(payload, &data)
	require.NoError(t, err)
	assert.Equal(t, "markdown", data["msgtype"])
	assert.Contains(t, data, "markdown")
	assert.Contains(t, data, "at")
}

func TestDingtalkNotifier_SignURL(t *testing.T) {
	n, _ := newDingtalkNotifier(nil)
	signed := n.(*dingtalkNotifier).signURL("https://oapi.dingtalk.com/robot/send?access_token=xxx", "test-secret")
	assert.Contains(t, signed, "timestamp=")
	assert.Contains(t, signed, "sign=")
	assert.Contains(t, signed, "access_token=xxx")
}

// ============================================================================
// 七、SendAll 并发测试
// ============================================================================

func TestSendAll(t *testing.T) {
	f := NewFactory()
	var notifiers []Notifier

	// 添加 3 个 console
	for i := 0; i < 3; i++ {
		n, _ := f.Create("console", nil)
		notifiers = append(notifiers, n)
	}

	msg := &Message{Title: "Test", Body: "Body", Severity: "info"}
	errs := SendAll(context.Background(), notifiers, msg)
	assert.Empty(t, errs)
}

func TestSendAll_Empty(t *testing.T) {
	errs := SendAll(context.Background(), nil, &Message{})
	assert.Empty(t, errs)
}

func TestSendAll_WithError(t *testing.T) {
	f := NewFactory()
	// 一个 console（成功）+ 一个 webhook（无 URL，失败）
	consoleN, _ := f.Create("console", nil)
	webhookCfg, _ := json.Marshal(WebhookConfig{URL: "", RetryCount: 0})
	webhookN, _ := f.Create("webhook", webhookCfg)

	errs := SendAll(context.Background(), []Notifier{consoleN, webhookN}, &Message{Title: "Test", Body: "Body", Severity: "info"})
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0], "webhook")
}

// ============================================================================
// 八、Message 结构体测试
// ============================================================================

func TestMessage_String(t *testing.T) {
	msg := &Message{
		Title:    "CPU High",
		Body:     "CPU 95%",
		Severity: "critical",
		TenantID: "t1",
		RuleID:   "r1",
		AlertID:  "a1",
		Labels:   map[string]string{"host": "192.168.1.1"},
	}
	assert.Equal(t, "CPU High", msg.Title)
	assert.Equal(t, "CPU 95%", msg.Body)
	assert.Equal(t, "critical", msg.Severity)
	assert.Equal(t, "192.168.1.1", msg.Labels["host"])
}

// ============================================================================
// 九、集成场景测试
// ============================================================================

func TestIntegration_MultiChannel(t *testing.T) {
	// 模拟一个真实场景：创建邮件 + Webhook 通知器
	f := NewFactory()

	emailCfg, _ := json.Marshal(EmailConfig{
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
		SMTPUser: "alert@example.com",
		FromAddr: "alert@example.com",
		ToAddrs:  []string{"ops@example.com"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	webhookCfg, _ := json.Marshal(WebhookConfig{URL: server.URL, RetryCount: 0})

	configs := []ChannelConfig{
		{Type: "email", Name: "运维邮件", Config: emailCfg},
		{Type: "webhook", Name: "告警平台", Config: webhookCfg},
		{Type: "console", Name: "系统通知"},
	}

	notifiers, errs, err := f.CreateMulti(configs)
	require.NoError(t, err)
	assert.Len(t, notifiers, 3)
	assert.Empty(t, errs)

	msg := &Message{
		Title:    "磁盘空间不足",
		Body:     "/var 分区使用率 95%",
		Severity: "warning",
		TenantID: "tenant-1",
		RuleID:   "rule-disk-1",
		AlertID:  "alert-disk-001",
		Labels:   map[string]string{"host": "db-server-01", "partition": "/var"},
	}

	// 发送所有通知（email 会失败因为 SMTP 不可达，但 webhook 和 console 会成功）
	errs = SendAll(context.Background(), notifiers, msg)
	// email 发送失败（无法连接真实 SMTP），但 webhook 和 console 成功
	assert.NotEmpty(t, errs) // email 连接失败
	assert.Len(t, errs, 1)   // 只有 email 失败
}

func TestParseChannels_Integration(t *testing.T) {
	// 模拟数据库中的 notify_channels 字段格式
	raw := `[{"type":"email","name":"运维邮箱","config":{"smtp_host":"smtp.example.com","smtp_port":587,"to_addrs":["ops@example.com"]}},{"type":"dingtalk","name":"运维群","config":{"webhook_url":"https://oapi.dingtalk.com/robot/send","secret":"SECxxx"}}]`
	configs, err := ParseChannels(raw)
	require.NoError(t, err)
	assert.Len(t, configs, 2)
	assert.Equal(t, "email", configs[0].Type)
	assert.Equal(t, "dingtalk", configs[1].Type)

	f := NewFactory()
	notifiers, errs, err := f.CreateMulti(configs)
	require.NoError(t, err)
	assert.Len(t, notifiers, 2)
	assert.Empty(t, errs)
}
