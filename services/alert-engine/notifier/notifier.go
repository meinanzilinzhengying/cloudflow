// Package notifier 告警通知渠道抽象层
//
// 支持的通知渠道：
//   - console: 控制台/系统内通知（仅写入数据库）
//   - email:   SMTP 邮件通知
//   - webhook: HTTP Webhook 回调
//   - dingtalk: 钉钉群机器人
//
// 使用方式：
//   factory := notifier.NewFactory()
//   n, err := factory.Create("email", configJSON)
//   err := n.Send(ctx, &notifier.Message{Title: "...", Body: "..."})
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Message 通知消息
type Message struct {
	Title    string            // 告警标题
	Body     string            // 告警内容
	Severity string            // 严重级别: critical/warning/info
	TenantID string            // 租户ID
	RuleID   string            // 规则ID
	AlertID  string            // 告警ID
	Labels   map[string]string // 额外标签
}

// Notifier 通知发送器接口
type Notifier interface {
	// Name 返回渠道名称
	Name() string
	// Send 发送通知，返回错误表示发送失败
	Send(ctx context.Context, msg *Message) error
	// Close 释放资源
	Close() error
}

// ChannelConfig 通知渠道配置（JSON 序列化）
type ChannelConfig struct {
	Type   string          `json:"type"`   // email/webhook/dingtalk/console
	Name   string          `json:"name"`   // 显示名称
	Config json.RawMessage `json:"config"` // 渠道专属配置
}

// ChannelConfigList 规则的通知渠道列表
type ChannelConfigList []ChannelConfig

// Factory 通知工厂
type Factory struct {
	mu       sync.RWMutex
	creators map[string]Creator
}

// Creator 创建通知器的函数签名
type Creator func(config json.RawMessage) (Notifier, error)

// NewFactory 创建通知工厂
func NewFactory() *Factory {
	f := &Factory{
		creators: make(map[string]Creator),
	}
	// 注册内置渠道
	f.Register("console", newConsoleNotifier)
	f.Register("email", newEmailNotifier)
	f.Register("webhook", newWebhookNotifier)
	f.Register("dingtalk", newDingtalkNotifier)
	return f
}

// Register 注册通知渠道构造器
func (f *Factory) Register(channelType string, creator Creator) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creators[channelType] = creator
}

// Create 根据渠道类型和配置创建通知器
func (f *Factory) Create(channelType string, config json.RawMessage) (Notifier, error) {
	f.mu.RLock()
	creator, ok := f.creators[channelType]
	f.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported notification channel: %s", channelType)
	}
	return creator(config)
}

// CreateMulti 根据配置列表创建多个通知器
func (f *Factory) CreateMulti(configs []ChannelConfig) ([]Notifier, []string, error) {
	var notifiers []Notifier
	var errs []string
	for _, cfg := range configs {
		if cfg.Type == "" {
			continue
		}
		n, err := f.Create(cfg.Type, cfg.Config)
		if err != nil {
			errs = append(errs, fmt.Sprintf("channel %s: %v", cfg.Type, err))
			continue
		}
		notifiers = append(notifiers, n)
	}
	return notifiers, errs, nil
}

// ParseChannels 解析 notify_channels JSON 字符串为配置列表
func ParseChannels(rawJSON string) ([]ChannelConfig, error) {
	if rawJSON == "" || rawJSON == "null" {
		return []ChannelConfig{{Type: "console"}}, nil
	}
	var configs []ChannelConfig
	if err := json.Unmarshal([]byte(rawJSON), &configs); err != nil {
		// 尝试解析为单个配置
		var single ChannelConfig
		if err := json.Unmarshal([]byte(rawJSON), &single); err != nil {
			return []ChannelConfig{{Type: "console"}}, nil
		}
		return []ChannelConfig{single}, nil
	}
	return configs, nil
}

// SendAll 并发发送通知到所有渠道
func SendAll(ctx context.Context, notifiers []Notifier, msg *Message) []string {
	if len(notifiers) == 0 {
		return nil
	}
	var wg sync.WaitGroup
	errCh := make(chan string, len(notifiers))
	for _, n := range notifiers {
		wg.Add(1)
		go func(notifier Notifier) {
			defer wg.Done()
			if err := notifier.Send(ctx, msg); err != nil {
				errCh <- fmt.Sprintf("%s: %v", notifier.Name(), err)
			}
		}(n)
	}
	wg.Wait()
	close(errCh)
	var errs []string
	for e := range errCh {
		errs = append(errs, e)
	}
	return errs
}
