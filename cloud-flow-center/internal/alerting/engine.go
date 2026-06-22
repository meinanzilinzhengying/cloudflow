// P2: 增强告警规则引擎 — 支持多条件、复合表达式、PromQL 风格查询
//
// 功能：
//   - 单条件/多条件组合（AND/OR/NOT）
//   - 数学表达式计算（value * 100 > threshold）
//   - 时间窗口评估（连续 N 次满足 / 满足比例）
//   - 告警状态机（pending → firing → resolved）
//   - 支持标签匹配和分组
//
package alerting

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/center/pkg/logger"
	edge "github.com/meinanzilinzhengying/cloudflow/proto"
)

// ============================================================================
// 一、增强规则引擎核心
// ============================================================================

// AlertState 告警状态
type AlertState string

const (
	StatePending   AlertState = "pending"
	StateFiring    AlertState = "firing"
	StateResolved  AlertState = "resolved"
	StateSuppressed AlertState = "suppressed"
)

// ConditionLogic 条件逻辑组合
type ConditionLogic string

const (
	LogicAnd ConditionLogic = "and"
	LogicOr  ConditionLogic = "or"
	LogicNot ConditionLogic = "not"
)

// EnhancedRule 增强告警规则
type EnhancedRule struct {
	Rule

	// 多条件支持
	Conditions  []*ConditionItem `json:"conditions"`
	Logic       ConditionLogic   `json:"logic"`        // and/or

	// 时间窗口配置
	ForDuration      Duration  `json:"for_duration"`       // 持续满足才触发
	KeepFiringFor    Duration  `json:"keep_firing_for"`    // 恢复后保留告警时间

	// 标签匹配
	LabelMatchers    []*LabelMatcher `json:"label_matchers"` // 标签筛选

	// 分组
	GroupBy          []string        `json:"group_by"`       // 按标签分组

	// 评估策略
	EvalInterval     time.Duration   `json:"eval_interval"`  // 评估间隔

	// 状态机
	state            map[string]AlertState
	stateMu          sync.RWMutex
	pendingSince     map[string]time.Time
	resolvedSince    map[string]time.Time
}

// ConditionItem 单个条件项
type ConditionItem struct {
	Metric    string            `json:"metric"`
	Operator  ConditionOperator `json:"operator"`
	Value     float64           `json:"value"`      // 阈值或表达式结果
	Expr      string            `json:"expr"`       // 数学表达式（如 "value * 100"）
	Labels    map[string]string `json:"labels"`     // 此条件适用的标签过滤
}

// LabelMatcher 标签匹配器
type LabelMatcher struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Regex     bool   `json:"regex"`     // 是否正则匹配
	Negative  bool   `json:"negative"`  // 是否反向匹配
}

// Match 检查标签是否匹配
func (lm *LabelMatcher) Match(labels map[string]string) bool {
	val, ok := labels[lm.Name]
	if !ok {
		return lm.Negative
	}

	matched := false
	if lm.Regex {
		re, err := regexp.Compile(lm.Value)
		if err != nil {
			return false
		}
		matched = re.MatchString(val)
	} else {
		matched = val == lm.Value
	}

	if lm.Negative {
		return !matched
	}
	return matched
}

// ============================================================================
// 二、增强规则引擎（EnhancedEngine）
// ============================================================================

// EnhancedEngine 增强告警规则引擎
type EnhancedEngine struct {
	rules     map[string]*EnhancedRule
	rulesMu   sync.RWMutex
	logger    *logger.Logger

	// 评估结果回调
	alertCallback func(rule *EnhancedRule, groupKey string, state AlertState, value float64)
}

// NewEnhancedEngine 创建增强规则引擎
func NewEnhancedEngine(log *logger.Logger) *EnhancedEngine {
	return &EnhancedEngine{
		rules:         make(map[string]*EnhancedRule),
		logger:        log,
		alertCallback: func(rule *EnhancedRule, groupKey string, state AlertState, value float64) {},
	}
}

// SetAlertCallback 设置告警触发回调
func (e *EnhancedEngine) SetAlertCallback(cb func(rule *EnhancedRule, groupKey string, state AlertState, value float64)) {
	e.alertCallback = cb
}

// RegisterRule 注册增强规则
func (e *EnhancedEngine) RegisterRule(rule *EnhancedRule) {
	if rule.ForDuration.Duration == 0 {
		rule.ForDuration = Duration{1 * time.Minute}
	}
	if rule.EvalInterval == 0 {
		rule.EvalInterval = 15 * time.Second
	}
	if rule.state == nil {
		rule.state = make(map[string]AlertState)
	}
	if rule.pendingSince == nil {
		rule.pendingSince = make(map[string]time.Time)
	}
	if rule.resolvedSince == nil {
		rule.resolvedSince = make(map[string]time.Time)
	}

	e.rulesMu.Lock()
	e.rules[rule.ID] = rule
	e.rulesMu.Unlock()
}

// UnregisterRule 注销规则
func (e *EnhancedEngine) UnregisterRule(ruleID string) {
	e.rulesMu.Lock()
	delete(e.rules, ruleID)
	e.rulesMu.Unlock()
}

// GetRule 获取规则
func (e *EnhancedEngine) GetRule(ruleID string) *EnhancedRule {
	e.rulesMu.RLock()
	defer e.rulesMu.RUnlock()
	return e.rules[ruleID]
}

// GetAllRules 获取所有规则
func (e *EnhancedEngine) GetAllRules() []*EnhancedRule {
	e.rulesMu.RLock()
	defer e.rulesMu.RUnlock()

	result := make([]*EnhancedRule, 0, len(e.rules))
	for _, rule := range e.rules {
		result = append(result, rule)
	}
	return result
}

// ============================================================================
// 三、规则评估
// ============================================================================

// Evaluate 评估单条规则
func (e *EnhancedEngine) Evaluate(ctx context.Context, rule *EnhancedRule, metrics []*edge.MetricData) {
	// 按标签分组
	groups := e.groupMetrics(rule, metrics)

	for groupKey, groupMetrics := range groups {
		e.evaluateGroup(ctx, rule, groupKey, groupMetrics)
	}
}

// evaluateGroup 评估单组指标
func (e *EnhancedEngine) evaluateGroup(ctx context.Context, rule *EnhancedRule, groupKey string, metrics []*edge.MetricData) {
	if len(metrics) == 0 {
		return
	}

	// 计算最新值
	latestMetric := metrics[0]
	value := e.extractValue(rule, latestMetric)
	if value == nil {
		return
	}

	// 评估条件
	satisfied := e.evaluateConditions(rule, metrics)

	// 状态机转换
	oldState := e.getState(rule, groupKey)
	newState := e.transitionState(rule, groupKey, satisfied, *value)

	if oldState != newState {
		e.logger.Infof("规则 %s 组 %s 状态变化: %s → %s (value=%.2f)",
			rule.Name, groupKey, oldState, newState, *value)
		if e.alertCallback != nil {
			e.alertCallback(rule, groupKey, newState, *value)
		}
	}
}

// evaluateConditions 评估多条件
func (e *EnhancedEngine) evaluateConditions(rule *EnhancedRule, metrics []*edge.MetricData) bool {
	if len(rule.Conditions) == 0 {
		// 回退到单条件
		return e.evaluateSingleCondition(rule, metrics)
	}

	results := make([]bool, 0, len(rule.Conditions))
	for _, cond := range rule.Conditions {
		result := e.evaluateCondition(cond, metrics)
		results = append(results, result)
	}

	switch rule.Logic {
	case LogicAnd:
		for _, r := range results {
			if !r {
				return false
			}
		}
		return true
	case LogicOr:
		for _, r := range results {
			if r {
				return true
			}
		}
		return false
	case LogicNot:
		if len(results) > 0 {
			return !results[0]
		}
		return true
	default:
		// 默认 AND
		for _, r := range results {
			if !r {
				return false
			}
		}
		return true
	}
}

// evaluateSingleCondition 回退单条件评估
func (e *EnhancedEngine) evaluateSingleCondition(rule *EnhancedRule, metrics []*edge.MetricData) bool {
	if len(metrics) == 0 {
		return false
	}
	value := e.extractValue(rule, metrics[0])
	if value == nil {
		return false
	}
	return e.compareValue(*value, rule.Condition.Operator, rule.Threshold)
}

// evaluateCondition 评估单个条件
func (e *EnhancedEngine) evaluateCondition(cond *ConditionItem, metrics []*edge.MetricData) bool {
	if len(metrics) == 0 {
		return false
	}

	latest := metrics[0]

	// 如果条件有标签过滤，先检查标签
	if len(cond.Labels) > 0 {
		for k, v := range cond.Labels {
			if latest.Tags[k] != v {
				return false
			}
		}
	}

	var value float64
	if cond.Expr != "" {
		// 使用表达式计算
		raw := e.extractRawValue(cond.Metric, latest)
		if raw == nil {
			return false
		}
		value = e.evalExpression(cond.Expr, *raw)
	} else {
		val := e.extractRawValue(cond.Metric, latest)
		if val == nil {
			return false
		}
		value = *val
	}

	return e.compareValue(value, cond.Operator, cond.Value)
}

// compareValue 比较值
func (e *EnhancedEngine) compareValue(value float64, op ConditionOperator, threshold float64) bool {
	switch op {
	case OperatorGreaterThan:
		return value > threshold
	case OperatorLessThan:
		return value < threshold
	case OperatorGreaterOrEqual:
		return value >= threshold
	case OperatorLessOrEqual:
		return value <= threshold
	case OperatorEqual:
		return math.Abs(value-threshold) < 1e-9
	case OperatorNotEqual:
		return math.Abs(value-threshold) >= 1e-9
	default:
		return false
	}
}

// extractValue 提取规则相关值（兼容旧规则）
func (e *EnhancedEngine) extractValue(rule *EnhancedRule, metric *edge.MetricData) *float64 {
	return extractMetricValue(&rule.Rule, metric)
}

// extractRawValue 从指标中提取原始值
func (e *EnhancedEngine) extractRawValue(metricName string, metric *edge.MetricData) *float64 {
	var value float64
	switch strings.ToLower(metricName) {
	case "cpu", "cpu_usage":
		if v, ok := metric.Tags["cpu_usage"]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				value = f
				return &value
			}
		}
	case "memory", "mem", "memory_usage":
		if v, ok := metric.Tags["memory_usage"]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				value = f
				return &value
			}
		}
	case "disk", "disk_usage":
		if v, ok := metric.Tags["disk_usage"]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				value = f
				return &value
			}
		}
	case "network", "traffic", "bytes":
		value = float64(metric.Bytes)
		return &value
	case "packets":
		value = float64(metric.Packets)
		return &value
	case "latency":
		value = float64(metric.Latency)
		return &value
	default:
		// 尝试从 tags 中查找
		if v, ok := metric.Tags[metricName]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				value = f
				return &value
			}
		}
	}
	return nil
}

// evalExpression 评估数学表达式
// 支持: value + n, value - n, value * n, value / n, value % n
func (e *EnhancedEngine) evalExpression(expr string, value float64) float64 {
	expr = strings.TrimSpace(strings.ToLower(expr))
	expr = strings.ReplaceAll(expr, "value", fmt.Sprintf("%.10f", value))

	// 简单表达式解析
	parts := strings.Fields(expr)
	if len(parts) == 3 {
		a, err1 := strconv.ParseFloat(parts[0], 64)
		op := parts[1]
		b, err2 := strconv.ParseFloat(parts[2], 64)
		if err1 == nil && err2 == nil {
			switch op {
			case "+":
				return a + b
			case "-":
				return a - b
			case "*":
				return a * b
			case "/":
				if b != 0 {
					return a / b
				}
				return math.MaxFloat64
			case "%":
				if b != 0 {
					return math.Mod(a, b)
				}
				return 0
			}
		}
	}

	// 如果解析失败，返回原值
	return value
}

// ============================================================================
// 四、状态机管理
// ============================================================================

// transitionState 状态机转换
func (e *EnhancedEngine) transitionState(rule *EnhancedRule, groupKey string, satisfied bool, value float64) AlertState {
	rule.stateMu.Lock()
	defer rule.stateMu.Unlock()

	currentState := rule.state[groupKey]
	now := time.Now()

	switch currentState {
	case "":
		if satisfied {
			rule.pendingSince[groupKey] = now
			rule.state[groupKey] = StatePending
			return StatePending
		}
		return StateResolved

	case StatePending:
		if !satisfied {
			delete(rule.pendingSince, groupKey)
			delete(rule.resolvedSince, groupKey)
			rule.state[groupKey] = StateResolved
			return StateResolved
		}
		// 检查是否满足持续时间
		if since, ok := rule.pendingSince[groupKey]; ok {
			if now.Sub(since) >= rule.ForDuration.Duration {
				rule.state[groupKey] = StateFiring
				return StateFiring
			}
		}
		return StatePending

	case StateFiring:
		if !satisfied {
			if rule.KeepFiringFor.Duration > 0 {
				if _, ok := rule.resolvedSince[groupKey]; !ok {
					rule.resolvedSince[groupKey] = now
				}
				if now.Sub(rule.resolvedSince[groupKey]) >= rule.KeepFiringFor.Duration {
					delete(rule.pendingSince, groupKey)
					delete(rule.resolvedSince, groupKey)
					rule.state[groupKey] = StateResolved
					return StateResolved
				}
				return StateFiring
			}
			delete(rule.pendingSince, groupKey)
			delete(rule.resolvedSince, groupKey)
			rule.state[groupKey] = StateResolved
			return StateResolved
		}
		// 仍然满足，重置 resolvedSince
		delete(rule.resolvedSince, groupKey)
		return StateFiring

	case StateResolved:
		if satisfied {
			rule.pendingSince[groupKey] = now
			rule.state[groupKey] = StatePending
			return StatePending
		}
		return StateResolved
	}

	return currentState
}

// getState 获取状态
func (e *EnhancedEngine) getState(rule *EnhancedRule, groupKey string) AlertState {
	rule.stateMu.RLock()
	defer rule.stateMu.RUnlock()
	return rule.state[groupKey]
}

// GetRuleStates 获取规则的所有状态
func (e *EnhancedEngine) GetRuleStates(ruleID string) map[string]AlertState {
	e.rulesMu.RLock()
	rule, ok := e.rules[ruleID]
	e.rulesMu.RUnlock()
	if !ok {
		return nil
	}

	rule.stateMu.RLock()
	defer rule.stateMu.RUnlock()

	result := make(map[string]AlertState)
	for k, v := range rule.state {
		result[k] = v
	}
	return result
}

// ============================================================================
// 五、指标分组
// ============================================================================

// groupMetrics 按规则分组配置对指标分组
func (e *EnhancedEngine) groupMetrics(rule *EnhancedRule, metrics []*edge.MetricData) map[string][]*edge.MetricData {
	if len(rule.GroupBy) == 0 {
		// 不分组，所有指标一个组
		return map[string][]*edge.MetricData{"": metrics}
	}

	groups := make(map[string][]*edge.MetricData)
	for _, metric := range metrics {
		// 先检查标签匹配器
		if !e.matchLabelMatchers(rule, metric.Tags) {
			continue
		}

		key := e.buildGroupKey(rule, metric.Tags)
		groups[key] = append(groups[key], metric)
	}

	return groups
}

// matchLabelMatchers 检查标签是否匹配所有匹配器
func (e *EnhancedEngine) matchLabelMatchers(rule *EnhancedRule, labels map[string]string) bool {
	for _, matcher := range rule.LabelMatchers {
		if !matcher.Match(labels) {
			return false
		}
	}
	return true
}

// buildGroupKey 构建分组键
func (e *EnhancedEngine) buildGroupKey(rule *EnhancedRule, labels map[string]string) string {
	parts := make([]string, 0, len(rule.GroupBy))
	for _, label := range rule.GroupBy {
		if v, ok := labels[label]; ok {
			parts = append(parts, fmt.Sprintf("%s=%s", label, v))
		} else {
			parts = append(parts, fmt.Sprintf("%s=_", label))
		}
	}
	return strings.Join(parts, ",")
}

// ============================================================================
// 六、评估循环
// ============================================================================

// EvalLoop 评估循环（在 goroutine 中运行）
func (e *EnhancedEngine) EvalLoop(ctx context.Context, fetchMetrics func(ctx context.Context, ruleType RuleType, duration time.Duration) ([]*edge.MetricData, error)) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.evalAllRules(ctx, fetchMetrics)
		case <-ctx.Done():
			return
		}
	}
}

func (e *EnhancedEngine) evalAllRules(ctx context.Context, fetchMetrics func(ctx context.Context, ruleType RuleType, duration time.Duration) ([]*edge.MetricData, error)) {
	rules := e.GetAllRules()
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		metrics, err := fetchMetrics(ctx, rule.Type, rule.ForDuration.Duration*2)
		if err != nil {
			e.logger.Warnf("获取规则 %s 指标失败: %v", rule.Name, err)
			continue
		}

		e.Evaluate(ctx, rule, metrics)
	}
}

// ResetRuleState 重置规则状态（用于规则变更或手动恢复）
func (e *EnhancedEngine) ResetRuleState(ruleID string) {
	e.rulesMu.RLock()
	rule, ok := e.rules[ruleID]
	e.rulesMu.RUnlock()
	if !ok {
		return
	}

	rule.stateMu.Lock()
	rule.state = make(map[string]AlertState)
	rule.pendingSince = make(map[string]time.Time)
	rule.resolvedSince = make(map[string]time.Time)
	rule.stateMu.Unlock()
}
