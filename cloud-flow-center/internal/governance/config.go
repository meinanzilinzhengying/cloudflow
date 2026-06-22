//go:build linux

package governance

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 一、统一配置管理（限流 + 熔断）
// ============================================================================

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	Name             string        `json:"name"`
	FailureThreshold int           `json:"failure_threshold"`    // 失败阈值
	SuccessThreshold int           `json:"success_threshold"`    // 成功阈值（半开恢复）
	Timeout          time.Duration `json:"timeout"`              // 熔断超时
	HalfOpenMaxCalls int           `json:"half_open_max_calls"`  // 半开最大请求数
	Enabled          bool          `json:"enabled"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Name      string        `json:"name"`
	QPS       int           `json:"qps"`       // 每秒请求数
	Burst     int           `json:"burst"`     // 突发流量
	Window    time.Duration `json:"window"`    // 时间窗口
	KeyBy     string        `json:"key_by"`    // 限流 key 来源：ip/user/service
	Enabled   bool          `json:"enabled"`
}

// GovernanceConfig 统一治理配置
type GovernanceConfig struct {
	ServiceName      string                `json:"service_name"`
	CircuitBreaker   *CircuitBreakerConfig `json:"circuit_breaker,omitempty"`
	RateLimit        *RateLimitConfig      `json:"rate_limit,omitempty"`
	Retry            *RetryConfig          `json:"retry,omitempty"`
	Timeout          time.Duration         `json:"timeout"`
	Version          string                `json:"version"`
	UpdatedAt        time.Time             `json:"updated_at"`
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries  int           `json:"max_retries"`
	BaseDelay   time.Duration `json:"base_delay"`
	MaxDelay    time.Duration `json:"max_delay"`
	Multiplier  float64       `json:"multiplier"`
	RetryableErrors []string  `json:"retryable_errors,omitempty"`
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		Multiplier: 2.0,
	}
}

// DefaultCircuitBreakerConfig 默认熔断配置
func DefaultCircuitBreakerConfig(name string) *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		Name:             name,
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
		HalfOpenMaxCalls: 3,
		Enabled:          true,
	}
}

// DefaultRateLimitConfig 默认限流配置
func DefaultRateLimitConfig(name string) *RateLimitConfig {
	return &RateLimitConfig{
		Name:    name,
		QPS:     100,
		Burst:   200,
		Window:  1 * time.Second,
		KeyBy:   "ip",
		Enabled: true,
	}
}

// ConfigManager 统一配置管理器
type ConfigManager struct {
	mu       sync.RWMutex
	configs  map[string]*GovernanceConfig // serviceName -> config
	watchers []chan *GovernanceConfig
	stopCh   chan struct{}
}

// NewConfigManager 创建配置管理器
func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		configs:  make(map[string]*GovernanceConfig),
		watchers: make([]chan *GovernanceConfig, 0),
		stopCh:   make(chan struct{}),
	}
}

// SetConfig 设置配置
func (cm *ConfigManager) SetConfig(config *GovernanceConfig) error {
	if config.ServiceName == "" {
		return fmt.Errorf("service_name required")
	}
	config.UpdatedAt = time.Now()

	cm.mu.Lock()
	cm.configs[config.ServiceName] = config
	cm.mu.Unlock()

	// 通知 watcher
	cm.notifyWatchers(config)
	return nil
}

// GetConfig 获取配置
func (cm *ConfigManager) GetConfig(serviceName string) (*GovernanceConfig, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	config, ok := cm.configs[serviceName]
	return config, ok
}

// GetAllConfigs 获取所有配置
func (cm *ConfigManager) GetAllConfigs() []*GovernanceConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]*GovernanceConfig, 0, len(cm.configs))
	for _, c := range cm.configs {
		result = append(result, c)
	}
	return result
}

// DeleteConfig 删除配置
func (cm *ConfigManager) DeleteConfig(serviceName string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.configs, serviceName)
}

// Watch 监听配置变化
func (cm *ConfigManager) Watch() chan *GovernanceConfig {
	ch := make(chan *GovernanceConfig, 10)
	cm.mu.Lock()
	cm.watchers = append(cm.watchers, ch)
	cm.mu.Unlock()
	return ch
}

// notifyWatchers 通知所有 watcher
func (cm *ConfigManager) notifyWatchers(config *GovernanceConfig) {
	cm.mu.RLock()
	watchers := make([]chan *GovernanceConfig, len(cm.watchers))
	copy(watchers, cm.watchers)
	cm.mu.RUnlock()

	for _, ch := range watchers {
		select {
		case ch <- config:
		default:
		}
	}
}

// ValidateCircuitBreaker 验证熔断配置
func ValidateCircuitBreaker(cfg *CircuitBreakerConfig) error {
	if cfg == nil {
		return fmt.Errorf("circuit breaker config is nil")
	}
	if cfg.Name == "" {
		return fmt.Errorf("circuit breaker name required")
	}
	if cfg.FailureThreshold <= 0 {
		return fmt.Errorf("failure_threshold must be > 0")
	}
	if cfg.SuccessThreshold <= 0 {
		return fmt.Errorf("success_threshold must be > 0")
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("timeout must be > 0")
	}
	return nil
}

// ValidateRateLimit 验证限流配置
func ValidateRateLimit(cfg *RateLimitConfig) error {
	if cfg == nil {
		return fmt.Errorf("rate limit config is nil")
	}
	if cfg.Name == "" {
		return fmt.Errorf("rate limit name required")
	}
	if cfg.QPS <= 0 {
		return fmt.Errorf("qps must be > 0")
	}
	if cfg.Burst <= 0 {
		return fmt.Errorf("burst must be > 0")
	}
	if cfg.Window <= 0 {
		return fmt.Errorf("window must be > 0")
	}
	return nil
}

// MergeConfig 合并配置（新配置覆盖旧配置）
func MergeConfig(base, override *GovernanceConfig) *GovernanceConfig {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	merged := &GovernanceConfig{
		ServiceName: base.ServiceName,
		Timeout:     base.Timeout,
		Version:     base.Version,
	}

	if override.CircuitBreaker != nil {
		merged.CircuitBreaker = override.CircuitBreaker
	} else {
		merged.CircuitBreaker = base.CircuitBreaker
	}

	if override.RateLimit != nil {
		merged.RateLimit = override.RateLimit
	} else {
		merged.RateLimit = base.RateLimit
	}

	if override.Retry != nil {
		merged.Retry = override.Retry
	} else {
		merged.Retry = base.Retry
	}

	if override.Timeout > 0 {
		merged.Timeout = override.Timeout
	}

	if override.Version != "" {
		merged.Version = override.Version
	}

	merged.UpdatedAt = time.Now()
	return merged
}

// Stats 获取配置统计
func (cm *ConfigManager) Stats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	cbEnabled := 0
	rlEnabled := 0
	for _, c := range cm.configs {
		if c.CircuitBreaker != nil && c.CircuitBreaker.Enabled {
			cbEnabled++
		}
		if c.RateLimit != nil && c.RateLimit.Enabled {
			rlEnabled++
		}
	}

	return map[string]interface{}{
		"total_services":   len(cm.configs),
		"cb_enabled":       cbEnabled,
		"rl_enabled":       rlEnabled,
		"watcher_count":    len(cm.watchers),
	}
}
