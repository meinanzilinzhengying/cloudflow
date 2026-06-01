//go:build linux

// Package alert 提供完整的告警系统
package alert

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// AlertLevel 告警级别
type AlertLevel int

const (
	AlertLevelInfo AlertLevel = iota
	AlertLevelWarning
	AlertLevelCritical
	AlertLevelEmergency
)

func (l AlertLevel) String() string {
	switch l {
	case AlertLevelInfo:
		return "info"
	case AlertLevelWarning:
		return "warning"
	case AlertLevelCritical:
		return "critical"
	case AlertLevelEmergency:
		return "emergency"
	default:
		return "unknown"
	}
}

func ParseAlertLevel(s string) AlertLevel {
	switch s {
	case "info":
		return AlertLevelInfo
	case "warning":
		return AlertLevelWarning
	case "critical":
		return AlertLevelCritical
	case "emergency":
		return AlertLevelEmergency
	default:
		return AlertLevelWarning
	}
}

// AlertState 告警状态
type AlertState int

const (
	AlertStatePending AlertState = iota
	AlertStateFiring
	AlertStateResolved
)

func (s AlertState) String() string {
	switch s {
	case AlertStatePending:
		return "pending"
	case AlertStateFiring:
		return "firing"
	case AlertStateResolved:
		return "resolved"
	default:
		return "unknown"
	}
}

// Operator 比较运算符
type Operator string

const (
	OpEqual        Operator = "=="
	OpNotEqual     Operator = "!="
	OpGreaterThan  Operator = ">"
	OpGreaterEqual Operator = ">="
	OpLessThan     Operator = "<"
	OpLessEqual    Operator = "<="
)

func (op Operator) Evaluate(actual, threshold float64) bool {
	switch op {
	case OpEqual:
		return actual == threshold
	case OpNotEqual:
		return actual != threshold
	case OpGreaterThan:
		return actual > threshold
	case OpGreaterEqual:
		return actual >= threshold
	case OpLessThan:
		return actual < threshold
	case OpLessEqual:
		return actual <= threshold
	default:
		return false
	}
}

// RuleCondition 规则条件
type RuleCondition struct {
	Metric    string            `json:"metric"`
	Operator  Operator          `json:"operator"`
	Threshold float64           `json:"threshold"`
	For       string            `json:"for"`
	Labels    map[string]string `json:"labels"`
}

func (c *RuleCondition) Evaluate(value float64, labels map[string]string) bool {
	for k, v := range c.Labels {
		if lv, ok := labels[k]; !ok || lv != v {
			return false
		}
	}
	
	return c.Operator.Evaluate(value, c.Threshold)
}

// AlertRule 告警规则
type AlertRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Level       AlertLevel        `json:"level"`
	Conditions  []RuleCondition   `json:"conditions"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Enabled     bool              `json:"enabled"`
	
	FireThreshold int    `json:"fire_threshold"`
	ResolveAfter  string `json:"resolve_after"`
	NotifyChannels []string `json:"notify_channels"`
	SilencePeriod  string   `json:"silence_period"`
	
	mu            sync.RWMutex
	pendingCount  int
	lastFireTime  time.Time
	fireStartTime time.Time
}

func (r *AlertRule) ShouldFire(conditionMet bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if !r.Enabled {
		return false
	}
	
	if conditionMet {
		r.pendingCount++
		if r.pendingCount >= r.FireThreshold {
			return true
		}
	} else {
		r.pendingCount = 0
	}
	
	return false
}

func (r *AlertRule) ResetPending() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingCount = 0
}

func (r *AlertRule) SetFireTime(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastFireTime = t
	if r.fireStartTime.IsZero() {
		r.fireStartTime = t
	}
}

func (r *AlertRule) ClearFireTime() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fireStartTime = time.Time{}
}

func (r *AlertRule) GetFireDuration() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.fireStartTime.IsZero() {
		return 0
	}
	return time.Since(r.fireStartTime)
}

// BuiltInTemplate 内置模板类型
type BuiltInTemplate string

const (
	TemplateLatency    BuiltInTemplate = "latency"
	TemplatePacketLoss BuiltInTemplate = "packet_loss"
	TemplateRetransmit BuiltInTemplate = "retransmit"
	TemplateCPU        BuiltInTemplate = "cpu"
	TemplateMemory     BuiltInTemplate = "memory"
	TemplateDisk       BuiltInTemplate = "disk"
	TemplateConnection BuiltInTemplate = "connection"
	TemplateErrorRate  BuiltInTemplate = "error_rate"
)

var BuiltInTemplates = map[BuiltInTemplate]*AlertRule{
	TemplateLatency: {
		ID:   "builtin-latency",
		Name: "网络时延告警",
		Description: "检测网络延迟异常，当平均时延超过阈值时触发告警",
		Level: AlertLevelWarning,
		Conditions: []RuleCondition{
			{Metric: "latency_avg_ms", Operator: OpGreaterThan, Threshold: 100, For: "2m"},
			{Metric: "latency_avg_ms", Operator: OpGreaterThan, Threshold: 500, For: "1m"},
		},
		Labels: map[string]string{"category": "network", "type": "latency"},
		Annotations: map[string]string{
			"summary":     "网络时延过高",
			"description": "平均时延 {{ $value }}ms 超过阈值 {{ $threshold }}ms",
		},
		Enabled:       true,
		FireThreshold: 3,
		ResolveAfter:  "5m",
		NotifyChannels: []string{"default"},
	},
	TemplatePacketLoss: {
		ID:   "builtin-packet-loss",
		Name: "丢包率告警",
		Description: "检测网络丢包异常，当丢包率超过阈值时触发告警",
		Level: AlertLevelCritical,
		Conditions: []RuleCondition{
			{Metric: "packet_loss_rate", Operator: OpGreaterThan, Threshold: 1, For: "1m"},
			{Metric: "packet_loss_rate", Operator: OpGreaterThan, Threshold: 5, For: "30s"},
		},
		Labels: map[string]string{"category": "network", "type": "packet_loss"},
		Annotations: map[string]string{
			"summary":     "网络丢包率过高",
			"description": "丢包率 {{ $value }}% 超过阈值 {{ $threshold }}%",
		},
		Enabled:       true,
		FireThreshold: 2,
		ResolveAfter:  "3m",
		NotifyChannels: []string{"default", "critical"},
	},
	TemplateRetransmit: {
		ID:   "builtin-retransmit",
		Name: "TCP重传告警",
		Description: "检测TCP重传异常，当重传率超过阈值时触发告警",
		Level: AlertLevelWarning,
		Conditions: []RuleCondition{
			{Metric: "tcp_retransmit_rate", Operator: OpGreaterThan, Threshold: 2, For: "2m"},
			{Metric: "tcp_retransmit_rate", Operator: OpGreaterThan, Threshold: 10, For: "1m"},
		},
		Labels: map[string]string{"category": "network", "type": "retransmit"},
		Annotations: map[string]string{
			"summary":     "TCP重传率过高",
			"description": "重传率 {{ $value }}% 超过阈值 {{ $threshold }}%",
		},
		Enabled:       true,
		FireThreshold: 3,
		ResolveAfter:  "5m",
		NotifyChannels: []string{"default"},
	},
	TemplateCPU: {
		ID:   "builtin-cpu",
		Name: "CPU使用率告警",
		Description: "检测CPU使用率异常，当使用率超过阈值时触发告警",
		Level: AlertLevelWarning,
		Conditions: []RuleCondition{
			{Metric: "cpu_usage_percent", Operator: OpGreaterThan, Threshold: 80, For: "5m"},
			{Metric: "cpu_usage_percent", Operator: OpGreaterThan, Threshold: 95, For: "1m"},
		},
		Labels: map[string]string{"category": "system", "type": "cpu"},
		Annotations: map[string]string{
			"summary":     "CPU使用率过高",
			"description": "CPU使用率 {{ $value }}% 超过阈值 {{ $threshold }}%",
		},
		Enabled:       true,
		FireThreshold: 3,
		ResolveAfter:  "5m",
		NotifyChannels: []string{"default"},
	},
	TemplateMemory: {
		ID:   "builtin-memory",
		Name: "内存使用率告警",
		Description: "检测内存使用率异常，当使用率超过阈值时触发告警",
		Level: AlertLevelWarning,
		Conditions: []RuleCondition{
			{Metric: "memory_usage_percent", Operator: OpGreaterThan, Threshold: 85, For: "5m"},
			{Metric: "memory_usage_percent", Operator: OpGreaterThan, Threshold: 95, For: "1m"},
		},
		Labels: map[string]string{"category": "system", "type": "memory"},
		Annotations: map[string]string{
			"summary":     "内存使用率过高",
			"description": "内存使用率 {{ $value }}% 超过阈值 {{ $threshold }}%",
		},
		Enabled:       true,
		FireThreshold: 3,
		ResolveAfter:  "5m",
		NotifyChannels: []string{"default"},
	},
	TemplateErrorRate: {
		ID:   "builtin-error-rate",
		Name: "错误率告警",
		Description: "检测HTTP/SQL错误率异常，当错误率超过阈值时触发告警",
		Level: AlertLevelCritical,
		Conditions: []RuleCondition{
			{Metric: "error_rate_percent", Operator: OpGreaterThan, Threshold: 5, For: "1m"},
			{Metric: "error_rate_percent", Operator: OpGreaterThan, Threshold: 20, For: "30s"},
		},
		Labels: map[string]string{"category": "application", "type": "error_rate"},
		Annotations: map[string]string{
			"summary":     "错误率过高",
			"description": "错误率 {{ $value }}% 超过阈值 {{ $threshold }}%",
		},
		Enabled:       true,
		FireThreshold: 2,
		ResolveAfter:  "3m",
		NotifyChannels: []string{"default", "critical"},
	},
}

// RuleEngine 规则引擎
type RuleEngine struct {
	mu       sync.RWMutex
	rules    map[string]*AlertRule
	templates map[BuiltInTemplate]*AlertRule
}

func NewRuleEngine() *RuleEngine {
	engine := &RuleEngine{
		rules:    make(map[string]*AlertRule),
		templates: make(map[BuiltInTemplate]*AlertRule),
	}
	
	for t, rule := range BuiltInTemplates {
		engine.templates[t] = rule
		engine.rules[rule.ID] = rule
	}
	
	return engine
}

func (e *RuleEngine) AddRule(rule *AlertRule) error {
	if rule.ID == "" {
		return fmt.Errorf("规则ID不能为空")
	}
	
	e.mu.Lock()
	defer e.mu.Unlock()
	
	newRule := *rule
	newRule.Labels = copyMap(rule.Labels)
	newRule.Annotations = copyMap(rule.Annotations)
	newRule.Conditions = make([]RuleCondition, len(rule.Conditions))
	copy(newRule.Conditions, rule.Conditions)
	
	e.rules[rule.ID] = &newRule
	return nil
}

func (e *RuleEngine) RemoveRule(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.rules, id)
}

func (e *RuleEngine) GetRule(id string) *AlertRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if rule, ok := e.rules[id]; ok {
		return rule
	}
	return nil
}

func (e *RuleEngine) GetRules() []*AlertRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	rules := make([]*AlertRule, 0, len(e.rules))
	for _, rule := range e.rules {
		rules = append(rules, rule)
	}
	
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Level > rules[j].Level
	})
	
	return rules
}

func (e *RuleEngine) EnableRule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if rule, ok := e.rules[id]; ok {
		rule.Enabled = true
		return nil
	}
	return fmt.Errorf("规则不存在: %s", id)
}

func (e *RuleEngine) DisableRule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if rule, ok := e.rules[id]; ok {
		rule.Enabled = false
		return nil
	}
	return fmt.Errorf("规则不存在: %s", id)
}

func (e *RuleEngine) EnableBuiltIn(template BuiltInTemplate) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if rule, ok := e.templates[template]; ok {
		rule.Enabled = true
		return nil
	}
	return fmt.Errorf("模板不存在: %s", template)
}

func (e *RuleEngine) DisableBuiltIn(template BuiltInTemplate) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if rule, ok := e.templates[template]; ok {
		rule.Enabled = false
		return nil
	}
	return fmt.Errorf("模板不存在: %s", template)
}

func (e *RuleEngine) Evaluate(metric string, value float64, labels map[string]string) []*AlertRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	var matchedRules []*AlertRule
	
	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}
		
		for _, cond := range rule.Conditions {
			if cond.Metric != metric {
				continue
			}
			
			if cond.Evaluate(value, labels) {
				matchedRules = append(matchedRules, rule)
				break
			}
		}
	}
	
	return matchedRules
}

// AlertEvent 告警事件
type AlertEvent struct {
	ID          string            `json:"id"`
	RuleID      string            `json:"rule_id"`
	RuleName    string            `json:"rule_name"`
	Level       AlertLevel        `json:"level"`
	State       AlertState        `json:"state"`
	Metric      string            `json:"metric"`
	Value       float64           `json:"value"`
	Threshold   float64           `json:"threshold"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	
	FiredAt     time.Time     `json:"fired_at"`
	ResolvedAt  time.Time     `json:"resolved_at,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	
	Notified    bool      `json:"notified"`
	NotifyAt    time.Time `json:"notify_at,omitempty"`
	NotifyError string    `json:"notify_error,omitempty"`
}

func (e *AlertEvent) GenerateFingerprint() string {
	return fmt.Sprintf("%s:%s:%v", e.RuleID, e.Metric, e.Labels)
}

func (e *AlertEvent) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"id":          e.ID,
		"rule_id":     e.RuleID,
		"rule_name":   e.RuleName,
		"level":       e.Level.String(),
		"state":       e.State.String(),
		"metric":      e.Metric,
		"value":       e.Value,
		"threshold":   e.Threshold,
		"labels":      e.Labels,
		"annotations": e.Annotations,
		"fired_at":    e.FiredAt.Format(time.RFC3339),
		"notified":    e.Notified,
	}
	
	if !e.ResolvedAt.IsZero() {
		m["resolved_at"] = e.ResolvedAt.Format(time.RFC3339)
	}
	if e.Duration > 0 {
		m["duration"] = e.Duration.String()
	}
	
	return m
}

// MetricData 指标数据
type MetricData struct {
	Name      string
	Value     float64
	Timestamp time.Time
	Labels    map[string]string
}

// RuleConfig 规则配置
type RuleConfig struct {
	ID          string            `yaml:"id" json:"id"`
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Level       string            `yaml:"level" json:"level"`
	Enabled     bool              `yaml:"enabled" json:"enabled"`
	Conditions  []ConditionConfig `yaml:"conditions" json:"conditions"`
	Labels      map[string]string `yaml:"labels" json:"labels"`
	Annotations map[string]string `yaml:"annotations" json:"annotations"`
	
	FireThreshold  int      `yaml:"fire_threshold" json:"fire_threshold"`
	ResolveAfter   string   `yaml:"resolve_after" json:"resolve_after"`
	NotifyChannels []string `yaml:"notify_channels" json:"notify_channels"`
	SilencePeriod  string   `yaml:"silence_period" json:"silence_period"`
}

type ConditionConfig struct {
	Metric    string            `yaml:"metric" json:"metric"`
	Operator  string            `yaml:"operator" json:"operator"`
	Threshold float64           `yaml:"threshold" json:"threshold"`
	For       string            `yaml:"for" json:"for"`
	Labels    map[string]string `yaml:"labels" json:"labels"`
}

func (c *RuleConfig) ToRule() *AlertRule {
	rule := &AlertRule{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		Level:       ParseAlertLevel(c.Level),
		Enabled:     c.Enabled,
		Labels:      c.Labels,
		Annotations: c.Annotations,
		FireThreshold: c.FireThreshold,
		ResolveAfter:  c.ResolveAfter,
		NotifyChannels: c.NotifyChannels,
		SilencePeriod:  c.SilencePeriod,
	}
	
	if rule.FireThreshold == 0 {
		rule.FireThreshold = 1
	}
	
	rule.Conditions = make([]RuleCondition, len(c.Conditions))
	for i, cc := range c.Conditions {
		rule.Conditions[i] = RuleCondition{
			Metric:    cc.Metric,
			Operator:  Operator(cc.Operator),
			Threshold: cc.Threshold,
			For:       cc.For,
			Labels:    cc.Labels,
		}
	}
	
	return rule
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func ParseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

func FormatValue(v float64) string {
	if v == math.Floor(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2f", v)
}
