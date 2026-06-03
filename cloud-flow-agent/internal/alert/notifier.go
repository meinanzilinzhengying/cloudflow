//go:build linux

// Package alert 提供告警推送功能
package alert

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"cloudflow-agent/pkg/logger"
)

// KafkaNotifier Kafka通知器
type KafkaNotifier struct {
	config    KafkaConfig
	producer  interface{}
	client    *http.Client
	log       *logger.Logger
	
	stats struct {
		sentCount   atomic.Uint64
		failedCount atomic.Uint64
	}
}

type KafkaConfig struct {
	Enabled   bool     `yaml:"enabled" json:"enabled"`
	Brokers   []string `yaml:"brokers" json:"brokers"`
	Topic     string   `yaml:"topic" json:"topic"`
	Partition int      `yaml:"partition" json:"partition"`
	SASLEnabled bool   `yaml:"sasl_enabled" json:"sasl_enabled"`
	SASLUser    string `yaml:"sasl_user" json:"sasl_user"`
	SASLPass    string `yaml:"sasl_pass" json:"sasl_pass"`
	TLSEnabled bool   `yaml:"tls_enabled" json:"tls_enabled"`
	CACert     string `yaml:"ca_cert" json:"ca_cert"`
	BatchSize    int           `yaml:"batch_size" json:"batch_size"`
	BatchTimeout time.Duration `yaml:"batch_timeout" json:"batch_timeout"`
}

func NewKafkaNotifier(config KafkaConfig, log *logger.Logger) *KafkaNotifier {
	return &KafkaNotifier{
		config: config,
		client: &http.Client{Timeout: 10 * time.Second},
		log:    log,
	}
}

func (n *KafkaNotifier) Notify(ctx context.Context, event *AlertEvent) error {
	if !n.config.Enabled {
		return nil
	}
	
	data, err := json.Marshal(event.ToMap())
	if err != nil {
		return fmt.Errorf("序列化告警事件失败: %w", err)
	}
	
	if err := n.sendToKafka(ctx, data); err != nil {
		n.stats.failedCount.Add(1)
		return err
	}
	
	n.stats.sentCount.Add(1)
	n.log.Debugf("Kafka告警发送成功: %s", event.ID)
	
	return nil
}

func (n *KafkaNotifier) sendToKafka(ctx context.Context, data []byte) error {
	for _, broker := range n.config.Brokers {
		url := fmt.Sprintf("http://%s/topics/%s", broker, n.config.Topic)
		
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
		if err != nil {
			continue
		}
		
		req.Header.Set("Content-Type", "application/vnd.kafka.json.v2+json")
		req.Header.Set("Accept", "application/vnd.kafka.v2+json")
		
		resp, err := n.client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Kafka发送失败: %s - %s", resp.Status, string(body))
	}
	
	return fmt.Errorf("所有Kafka broker都不可用")
}

func (n *KafkaNotifier) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"sent_count":   n.stats.sentCount.Load(),
		"failed_count": n.stats.failedCount.Load(),
	}
}

// APINotifier API通知器
type APINotifier struct {
	config APINotifierConfig
	client *http.Client
	log    *logger.Logger
	
	stats struct {
		sentCount   atomic.Uint64
		failedCount atomic.Uint64
	}
}

type APINotifierConfig struct {
	Enabled  bool              `yaml:"enabled" json:"enabled"`
	URL      string            `yaml:"url" json:"url"`
	Method   string            `yaml:"method" json:"method"`
	Headers  map[string]string `yaml:"headers" json:"headers"`
	Timeout  time.Duration     `yaml:"timeout" json:"timeout"`
	AuthType  string `yaml:"auth_type" json:"auth_type"`
	AuthUser  string `yaml:"auth_user" json:"auth_user"`
	AuthPass  string `yaml:"auth_pass" json:"auth_pass"`
	AuthToken string `yaml:"auth_token" json:"auth_token"`
	APIKey    string `yaml:"api_key" json:"api_key"`
	MaxRetries int           `yaml:"max_retries" json:"max_retries"`
	RetryDelay time.Duration `yaml:"retry_delay" json:"retry_delay"`
	TLSEnabled    bool   `yaml:"tls_enabled" json:"tls_enabled"`
	SkipTLSVerify bool   `yaml:"skip_tls_verify" json:"skip_tls_verify"`
	CACert        string `yaml:"ca_cert" json:"ca_cert"`
}

func NewAPINotifier(config APINotifierConfig, log *logger.Logger) *APINotifier {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.Method == "" {
		config.Method = "POST"
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = time.Second
	}
	
	client := &http.Client{
		Timeout: config.Timeout,
	}
	
	if config.TLSEnabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.SkipTLSVerify,
		}
		client.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}
	
	return &APINotifier{
		config: config,
		client: client,
		log:    log,
	}
}

func (n *APINotifier) Notify(ctx context.Context, event *AlertEvent) error {
	if !n.config.Enabled {
		return nil
	}
	
	data, err := json.Marshal(event.ToMap())
	if err != nil {
		return fmt.Errorf("序列化告警事件失败: %w", err)
	}
	
	var lastErr error
	for i := 0; i < n.config.MaxRetries; i++ {
		if err := n.sendRequest(ctx, data); err != nil {
			lastErr = err
			time.Sleep(n.config.RetryDelay)
			continue
		}
		
		n.stats.sentCount.Add(1)
		n.log.Debugf("API告警发送成功: %s -> %s", event.ID, n.config.URL)
		return nil
	}
	
	n.stats.failedCount.Add(1)
	return fmt.Errorf("API发送失败(重试%d次): %w", n.config.MaxRetries, lastErr)
}

func (n *APINotifier) sendRequest(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, n.config.Method, n.config.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.config.Headers {
		req.Header.Set(k, v)
	}
	
	switch n.config.AuthType {
	case "basic":
		req.SetBasicAuth(n.config.AuthUser, n.config.AuthPass)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+n.config.AuthToken)
	case "apikey":
		req.Header.Set("X-API-Key", n.config.APIKey)
	}
	
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}

func (n *APINotifier) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"sent_count":   n.stats.sentCount.Load(),
		"failed_count": n.stats.failedCount.Load(),
	}
}

// WebhookNotifier Webhook通知器
type WebhookNotifier struct {
	config WebhookConfig
	client *http.Client
	log    *logger.Logger
	
	stats struct {
		sentCount   atomic.Uint64
		failedCount atomic.Uint64
	}
}

type WebhookConfig struct {
	Enabled  bool              `yaml:"enabled" json:"enabled"`
	URL      string            `yaml:"url" json:"url"`
	Secret   string            `yaml:"secret" json:"secret"`
	Headers  map[string]string `yaml:"headers" json:"headers"`
	Timeout  time.Duration     `yaml:"timeout" json:"timeout"`
}

func NewWebhookNotifier(config WebhookConfig, log *logger.Logger) *WebhookNotifier {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	
	return &WebhookNotifier{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
		log:    log,
	}
}

func (n *WebhookNotifier) Notify(ctx context.Context, event *AlertEvent) error {
	if !n.config.Enabled {
		return nil
	}
	
	payload := map[string]interface{}{
		"event":     "alert",
		"timestamp": time.Now().Unix(),
		"data":      event.ToMap(),
	}
	
	if n.config.Secret != "" {
		payload["signature"] = n.sign(event)
	}
	
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", n.config.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.config.Headers {
		req.Header.Set(k, v)
	}
	
	resp, err := n.client.Do(req)
	if err != nil {
		n.stats.failedCount.Add(1)
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		n.stats.sentCount.Add(1)
		return nil
	}
	
	body, _ := io.ReadAll(resp.Body)
	n.stats.failedCount.Add(1)
	return fmt.Errorf("Webhook失败: %d - %s", resp.StatusCode, string(body))
}

func (n *WebhookNotifier) sign(event *AlertEvent) string {
	return fmt.Sprintf("%x", len(event.ID)+len(n.config.Secret))
}

func (n *WebhookNotifier) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"sent_count":   n.stats.sentCount.Load(),
		"failed_count": n.stats.failedCount.Load(),
	}
}

// LogNotifier 日志通知器
type LogNotifier struct {
	log *logger.Logger
}

func NewLogNotifier(log *logger.Logger) *LogNotifier {
	return &LogNotifier{log: log}
}

func (n *LogNotifier) Notify(ctx context.Context, event *AlertEvent) error {
	n.log.Infof("[ALERT] [%s] %s - %s: %s (阈值=%s, 实际=%s)",
		event.Level.String(),
		event.State.String(),
		event.RuleName,
		event.Metric,
		FormatValue(event.Threshold),
		FormatValue(event.Value))
	return nil
}

// NotifierFactory 通知器工厂
type NotifierFactory struct {
	mu        sync.RWMutex
	notifiers map[string]Notifier
}

func NewNotifierFactory() *NotifierFactory {
	return &NotifierFactory{
		notifiers: make(map[string]Notifier),
	}
}

func (f *NotifierFactory) Register(name string, notifier Notifier) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notifiers[name] = notifier
}

func (f *NotifierFactory) Get(name string) Notifier {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.notifiers[name]
}

func (f *NotifierFactory) NotifyAll(ctx context.Context, event *AlertEvent) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	var lastErr error
	for name, notifier := range f.notifiers {
		if err := notifier.Notify(ctx, event); err != nil {
			lastErr = fmt.Errorf("[%s] %w", name, err)
		}
	}
	return lastErr
}

type NotifyConfig struct {
	Kafka   KafkaConfig      `yaml:"kafka" json:"kafka"`
	API     APINotifierConfig `yaml:"api" json:"api"`
	Webhook WebhookConfig    `yaml:"webhook" json:"webhook"`
}

func BuildNotifiers(config NotifyConfig, log *logger.Logger) *MultiNotifier {
	multi := NewMultiNotifier()
	
	if config.Kafka.Enabled {
		multi.AddNotifier("kafka", NewKafkaNotifier(config.Kafka, log))
	}
	
	if config.API.Enabled {
		multi.AddNotifier("api", NewAPINotifier(config.API, log))
	}
	
	if config.Webhook.Enabled {
		multi.AddNotifier("webhook", NewWebhookNotifier(config.Webhook, log))
	}
	
	multi.AddNotifier("log", NewLogNotifier(log))
	
	return multi
}
