//go:build linux

// Package alert 提供告警管理功能
package alert

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"cloudflow-agent/pkg/logger"
)

// AlertManager 告警管理器
type AlertManager struct {
	mu          sync.RWMutex
	engine      *RuleEngine
	notifier    Notifier
	log         *logger.Logger
	
	activeAlerts  map[string]*AlertEvent
	alertHistory  []*AlertEvent
	silences      map[string]*Silence
	
	config AlertManagerConfig
	
	stats struct {
		totalAlerts    atomic.Uint64
		activeAlerts   atomic.Uint64
		resolvedAlerts atomic.Uint64
		silencedAlerts atomic.Uint64
		notifySuccess  atomic.Uint64
		notifyFailed   atomic.Uint64
	}
	
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// AlertManagerConfig 告警管理器配置
type AlertManagerConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	EvaluationInterval time.Duration `yaml:"evaluation_interval" json:"evaluation_interval"`
	ResolveTimeout    time.Duration `yaml:"resolve_timeout" json:"resolve_timeout"`
	MaxActiveAlerts   int           `yaml:"max_active_alerts" json:"max_active_alerts"`
	MaxHistoryAlerts  int           `yaml:"max_history_alerts" json:"max_history_alerts"`
	EnableAutoResolve bool          `yaml:"enable_auto_resolve" json:"enable_auto_resolve"`
}

// DefaultAlertManagerConfig 默认配置
func DefaultAlertManagerConfig() AlertManagerConfig {
	return AlertManagerConfig{
		Enabled:           true,
		EvaluationInterval: time.Minute,
		ResolveTimeout:    5 * time.Minute,
		MaxActiveAlerts:   1000,
		MaxHistoryAlerts:  10000,
		EnableAutoResolve: true,
	}
}

// Silence 静默规则
type Silence struct {
	ID        string            `json:"id"`
	Matchers  map[string]string `json:"matchers"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time"`
	Reason    string            `json:"reason"`
	CreatedBy string            `json:"created_by"`
}

// Matches 检查是否匹配静默规则
func (s *Silence) Matches(labels map[string]string) bool {
	for k, v := range s.Matchers {
		if lv, ok := labels[k]; !ok || lv != v {
			return false
		}
	}
	return true
}

// IsActive 检查静默规则是否活跃
func (s *Silence) IsActive() bool {
	now := time.Now()
	return now.After(s.StartTime) && now.Before(s.EndTime)
}

// NewAlertManager 创建告警管理器
func NewAlertManager(config AlertManagerConfig, notifier Notifier, log *logger.Logger) *AlertManager {
	if !config.Enabled {
		return &AlertManager{config: config, log: log}
	}
	
	return &AlertManager{
		engine:       NewRuleEngine(),
		notifier:     notifier,
		log:          log,
		config:       config,
		activeAlerts: make(map[string]*AlertEvent),
		alertHistory: make([]*AlertEvent, 0),
		silences:     make(map[string]*Silence),
		stopCh:       make(chan struct{}),
	}
}

// Start 启动告警管理器
func (m *AlertManager) Start() error {
	if !m.config.Enabled {
		return nil
	}
	
	m.wg.Add(1)
	go m.autoResolveLoop()
	
	m.log.Infof("告警管理器已启动: 评估间隔=%v, 自动消警=%v", 
		m.config.EvaluationInterval, m.config.EnableAutoResolve)
	
	return nil
}

// Stop 停止告警管理器
func (m *AlertManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	m.log.Info("告警管理器已停止")
}

// ProcessMetrics 处理指标数据
func (m *AlertManager) ProcessMetrics(metrics []MetricData) error {
	if !m.config.Enabled || m.engine == nil {
		return nil
	}
	
	for _, metric := range metrics {
		m.processMetric(metric)
	}
	
	return nil
}

// processMetric 处理单个指标
func (m *AlertManager) processMetric(metric MetricData) {
	matchedRules := m.engine.Evaluate(metric.Name, metric.Value, metric.Labels)
	
	for _, rule := range matchedRules {
		var matchedCond *RuleCondition
		for i := range rule.Conditions {
			if rule.Conditions[i].Metric == metric.Name {
				matchedCond = &rule.Conditions[i]
				break
			}
		}
		
		if matchedCond == nil {
			continue
		}
		
		event := &AlertEvent{
			ID:          generateEventID(),
			RuleID:      rule.ID,
			RuleName:    rule.Name,
			Level:       rule.Level,
			Metric:      metric.Name,
			Value:       metric.Value,
			Threshold:   matchedCond.Threshold,
			Labels:      mergeLabels(rule.Labels, metric.Labels),
			Annotations: rule.Annotations,
		}
		
		fingerprint := event.GenerateFingerprint()
		
		m.mu.Lock()
		existing, hasExisting := m.activeAlerts[fingerprint]
		
		if hasExisting {
			existing.Value = metric.Value
			existing.Duration = time.Since(existing.FiredAt)
		} else {
			if rule.ShouldFire(true) {
				event.State = AlertStateFiring
				event.FiredAt = time.Now()
				
				if m.isSilenced(event) {
					m.stats.silencedAlerts.Add(1)
					m.log.Debugf("告警被静默: %s", fingerprint)
				} else {
					if err := m.sendNotification(event); err != nil {
						m.log.Warnf("发送告警通知失败: %v", err)
					}
				}
				
				m.activeAlerts[fingerprint] = event
				m.stats.totalAlerts.Add(1)
				m.stats.activeAlerts.Add(1)
				
				rule.SetFireTime(time.Now())
				
				m.log.Infof("告警触发: [%s] %s - %s=%s (阈值=%s)",
					event.Level.String(), event.RuleName, event.Metric,
					FormatValue(event.Value), FormatValue(event.Threshold))
			}
		}
		m.mu.Unlock()
	}
}

// ResolveAlert 恢复告警
func (m *AlertManager) ResolveAlert(fingerprint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	event, ok := m.activeAlerts[fingerprint]
	if !ok {
		return fmt.Errorf("告警不存在: %s", fingerprint)
	}
	
	return m.resolveAlertInternal(event)
}

func (m *AlertManager) resolveAlertInternal(event *AlertEvent) error {
	event.State = AlertStateResolved
	event.ResolvedAt = time.Now()
	event.Duration = event.ResolvedAt.Sub(event.FiredAt)
	
	if err := m.sendNotification(event); err != nil {
		m.log.Warnf("发送恢复通知失败: %v", err)
	}
	
	m.addToHistory(event)
	delete(m.activeAlerts, event.GenerateFingerprint())
	
	m.stats.activeAlerts.Add(^uint64(0))
	m.stats.resolvedAlerts.Add(1)
	
	m.log.Infof("告警恢复: [%s] %s - 持续时间=%v",
		event.Level.String(), event.RuleName, event.Duration)
	
	return nil
}

func (m *AlertManager) autoResolveLoop() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(m.config.EvaluationInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAutoResolve()
		}
	}
}

func (m *AlertManager) checkAutoResolve() {
	if !m.config.EnableAutoResolve {
		return
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	
	for fingerprint, event := range m.activeAlerts {
		if now.Sub(event.FiredAt) > m.config.ResolveTimeout {
			rule := m.engine.GetRule(event.RuleID)
			if rule == nil {
				m.resolveAlertInternal(event)
				continue
			}
			_ = fingerprint
		}
	}
}

func (m *AlertManager) sendNotification(event *AlertEvent) error {
	if m.notifier == nil {
		return nil
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := m.notifier.Notify(ctx, event); err != nil {
		m.stats.notifyFailed.Add(1)
		event.NotifyError = err.Error()
		return err
	}
	
	m.stats.notifySuccess.Add(1)
	event.Notified = true
	event.NotifyAt = time.Now()
	
	return nil
}

func (m *AlertManager) isSilenced(event *AlertEvent) bool {
	for _, silence := range m.silences {
		if silence.IsActive() && silence.Matches(event.Labels) {
			return true
		}
	}
	return false
}

func (m *AlertManager) addToHistory(event *AlertEvent) {
	m.alertHistory = append(m.alertHistory, event)
	
	if len(m.alertHistory) > m.config.MaxHistoryAlerts {
		m.alertHistory = m.alertHistory[len(m.alertHistory)-m.config.MaxHistoryAlerts:]
	}
}

func (m *AlertManager) AddRule(rule *AlertRule) error {
	if m.engine == nil {
		return fmt.Errorf("规则引擎未初始化")
	}
	return m.engine.AddRule(rule)
}

func (m *AlertManager) RemoveRule(id string) {
	if m.engine != nil {
		m.engine.RemoveRule(id)
	}
}

func (m *AlertManager) GetRules() []*AlertRule {
	if m.engine == nil {
		return nil
	}
	return m.engine.GetRules()
}

func (m *AlertManager) EnableRule(id string) error {
	if m.engine == nil {
		return fmt.Errorf("规则引擎未初始化")
	}
	return m.engine.EnableRule(id)
}

func (m *AlertManager) DisableRule(id string) error {
	if m.engine == nil {
		return fmt.Errorf("规则引擎未初始化")
	}
	return m.engine.DisableRule(id)
}

func (m *AlertManager) EnableBuiltInTemplate(template BuiltInTemplate) error {
	if m.engine == nil {
		return fmt.Errorf("规则引擎未初始化")
	}
	return m.engine.EnableBuiltIn(template)
}

func (m *AlertManager) DisableBuiltInTemplate(template BuiltInTemplate) error {
	if m.engine == nil {
		return fmt.Errorf("规则引擎未初始化")
	}
	return m.engine.DisableBuiltIn(template)
}

func (m *AlertManager) AddSilence(silence *Silence) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if silence.ID == "" {
		silence.ID = generateSilenceID()
	}
	
	m.silences[silence.ID] = silence
	m.log.Infof("添加静默规则: %s, 原因: %s", silence.ID, silence.Reason)
	
	return nil
}

func (m *AlertManager) RemoveSilence(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	delete(m.silences, id)
	m.log.Infof("移除静默规则: %s", id)
}

func (m *AlertManager) GetSilences() []*Silence {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	silences := make([]*Silence, 0, len(m.silences))
	for _, s := range m.silences {
		silences = append(silences, s)
	}
	return silences
}

func (m *AlertManager) GetActiveAlerts() []*AlertEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	alerts := make([]*AlertEvent, 0, len(m.activeAlerts))
	for _, event := range m.activeAlerts {
		alerts = append(alerts, event)
	}
	return alerts
}

func (m *AlertManager) GetAlertHistory(limit int) []*AlertEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if limit <= 0 || limit > len(m.alertHistory) {
		limit = len(m.alertHistory)
	}
	
	start := len(m.alertHistory) - limit
	if start < 0 {
		start = 0
	}
	
	result := make([]*AlertEvent, limit)
	copy(result, m.alertHistory[start:])
	return result
}

func (m *AlertManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	activeCount := len(m.activeAlerts)
	silenceCount := len(m.silences)
	m.mu.RUnlock()
	
	return map[string]interface{}{
		"enabled":         m.config.Enabled,
		"total_alerts":    m.stats.totalAlerts.Load(),
		"active_alerts":   activeCount,
		"resolved_alerts": m.stats.resolvedAlerts.Load(),
		"silenced_alerts": m.stats.silencedAlerts.Load(),
		"notify_success":  m.stats.notifySuccess.Load(),
		"notify_failed":   m.stats.notifyFailed.Load(),
		"silence_count":   silenceCount,
		"rule_count":      len(m.GetRules()),
	}
}

func generateEventID() string {
	return fmt.Sprintf("alert-%d", time.Now().UnixNano())
}

func generateSilenceID() string {
	return fmt.Sprintf("silence-%d", time.Now().UnixNano())
}

func mergeLabels(base, extra map[string]string) map[string]string {
	result := make(map[string]string)
	
	for k, v := range base {
		result[k] = v
	}
	
	for k, v := range extra {
		result[k] = v
	}
	
	return result
}

type Notifier interface {
	Notify(ctx context.Context, event *AlertEvent) error
}

type MultiNotifier struct {
	notifiers map[string]Notifier
	mu        sync.RWMutex
}

func NewMultiNotifier() *MultiNotifier {
	return &MultiNotifier{
		notifiers: make(map[string]Notifier),
	}
}

func (n *MultiNotifier) AddNotifier(name string, notifier Notifier) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notifiers[name] = notifier
}

func (n *MultiNotifier) RemoveNotifier(name string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.notifiers, name)
}

func (n *MultiNotifier) Notify(ctx context.Context, event *AlertEvent) error {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	var lastErr error
	for name, notifier := range n.notifiers {
		if err := notifier.Notify(ctx, event); err != nil {
			log.Printf("[%s] 通知发送失败: %v", name, err)
			lastErr = err
		}
	}
	
	return lastErr
}
