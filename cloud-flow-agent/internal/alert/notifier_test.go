package alert

import (
	"context"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/pkg/logger"
	"github.com/stretchr/testify/assert"
)

func TestWebhookNotifierSign(t *testing.T) {
	log := logger.New(logger.Config{
		Level:  "info",
		Format: "text",
	})

	notifier := NewWebhookNotifier(WebhookConfig{
		Enabled: true,
		URL:     "http://example.com/webhook",
		Secret:  "test-secret-key",
		Timeout: 5 * time.Second,
	}, log)

	event := &AlertEvent{
		ID:        "test-event-123",
		Timestamp: time.Now().Unix(),
		Level:     AlertLevelCritical,
		State:     AlertStateFiring,
		RuleName:  "test-rule",
		Metric:    "cpu_usage",
		Value:     95.5,
		Threshold: 90.0,
	}

	// 测试签名生成
	signature := notifier.sign(event)
	
	// 验证签名不为空
	assert.NotEmpty(t, signature, "签名不应为空")
	
	// 验证签名长度（SHA256 hex 应该是 64 字符）
	assert.Equal(t, 64, len(signature), "SHA256 签名长度应为 64")
	
	// 验证相同输入产生相同签名
	signature2 := notifier.sign(event)
	assert.Equal(t, signature, signature2, "相同输入应产生相同签名")
	
	// 验证不同输入产生不同签名
	event2 := &AlertEvent{
		ID:        "different-event",
		Timestamp: time.Now().Unix(),
		Level:     AlertLevelWarning,
	}
	signature3 := notifier.sign(event2)
	assert.NotEqual(t, signature, signature3, "不同输入应产生不同签名")
}

func TestWebhookNotifierNotifyDisabled(t *testing.T) {
	log := logger.New(logger.Config{
		Level:  "info",
		Format: "text",
	})

	notifier := NewWebhookNotifier(WebhookConfig{
		Enabled: false, // 禁用
		URL:     "http://example.com/webhook",
		Secret:  "test-secret",
	}, log)

	event := &AlertEvent{
		ID: "test-event",
	}

	// 禁用的通知器应该直接返回 nil
	err := notifier.Notify(context.Background(), event)
	assert.NoError(t, err, "禁用的通知器不应返回错误")
}

func TestKafkaNotifierSASLPasswordEnv(t *testing.T) {
	log := logger.New(logger.Config{
		Level:  "info",
		Format: "text",
	})

	// 设置环境变量
	t.Setenv("CLOUD_FLOW_KAFKA_SASL_PASS", "env-password-123")

	config := KafkaConfig{
		Enabled:     true,
		Brokers:     []string{"localhost:9092"},
		Topic:       "alerts",
		SASLEnabled: true,
		SASLUser:    "user",
		SASLPass:    "config-password", // 这个应该被环境变量覆盖
	}

	notifier := NewKafkaNotifier(config, log)
	
	// 验证密码已从环境变量加载
	assert.Equal(t, "env-password-123", notifier.config.SASLPass, 
		"SASL 密码应从环境变量加载")
}

func TestAPINotifierTLSWarning(t *testing.T) {
	log := logger.New(logger.Config{
		Level:  "warn",
		Format: "text",
	})

	config := APINotifierConfig{
		Enabled:         true,
		URL:             "https://example.com/api",
		TLSEnabled:      true,
		SkipTLSVerify:   true, // 跳过 TLS 验证
	}

	// 创建通知器时会记录警告
	notifier := NewAPINotifier(config, log)
	assert.NotNil(t, notifier, "通知器应成功创建")
	assert.True(t, notifier.config.SkipTLSVerify, "SkipTLSVerify 应保持为 true")
}

func TestMultiNotifier(t *testing.T) {
	multi := NewMultiNotifier()
	
	// 添加多个通知器
	logNotifier := NewLogNotifier(logger.New(logger.Config{Level: "info"}))
	multi.AddNotifier("log", logNotifier)
	
	// 验证通知器已注册
	assert.NotNil(t, multi.GetNotifier("log"), "log 通知器应已注册")
	assert.Nil(t, multi.GetNotifier("nonexistent"), "不存在的通知器应返回 nil")
}

func TestAlertEventToMap(t *testing.T) {
	event := &AlertEvent{
		ID:        "event-123",
		Timestamp: 1234567890,
		Level:     AlertLevelCritical,
		State:     AlertStateFiring,
		RuleName:  "high-cpu",
		Metric:    "cpu_usage",
		Value:     95.5,
		Threshold: 90.0,
		TenantID:  "tenant-1",
		AssetID:   "asset-1",
	}

	data := event.ToMap()
	
	// 验证关键字段存在
	assert.Contains(t, data, "id")
	assert.Contains(t, data, "timestamp")
	assert.Contains(t, data, "level")
	assert.Contains(t, data, "state")
	assert.Contains(t, data, "rule_name")
	assert.Contains(t, data, "metric")
	assert.Contains(t, data, "value")
	assert.Contains(t, data, "threshold")
	
	// 验证值正确
	assert.Equal(t, "event-123", data["id"])
	assert.Equal(t, "critical", data["level"])
	assert.Equal(t, "firing", data["state"])
}
