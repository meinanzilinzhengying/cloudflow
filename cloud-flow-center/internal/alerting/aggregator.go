// P2: 告警聚合与降噪 — 按标签分组聚合、去重、抖动检测、抑制
//
// 功能：
//   - 告警聚合：按标签/规则分组，合并相似告警
//   - 去重：相同告警在窗口内只发送一次
//   - 抖动检测：防止告警频繁翻转
//   - 抑制：抑制依赖告警（父告警触发时抑制子告警）
//
package alerting

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 一、告警聚合
// ============================================================================

// AlertGroup 告警聚合组
type AlertGroup struct {
	Key           string
	RuleID        string
	RuleName      string
	Severity      string
	Labels        map[string]string
	Alerts        []*Alert
	FirstAlertAt  time.Time
	LastAlertAt   time.Time
	Count         int
}

// GroupKey 计算聚合键
func (ag *AlertGroup) GroupKey() string {
	return ag.Key
}

// Summary 生成聚合摘要
func (ag *AlertGroup) Summary() string {
	if ag.Count == 1 {
		return ag.Alerts[0].Message
	}
	return fmt.Sprintf("%s 等 %d 条告警", ag.Alerts[0].Message, ag.Count)
}

// Aggregator 告警聚合器
type Aggregator struct {
	window     time.Duration
	groups     map[string]*AlertGroup
	mu         sync.RWMutex
	flushTimer *time.Timer
	flushCh    chan *AlertGroup
	stopCh     chan struct{}
}

// NewAggregator 创建聚合器
// window: 聚合窗口大小，窗口内的告警会被聚合
func NewAggregator(window time.Duration, flushCh chan *AlertGroup) *Aggregator {
	return &Aggregator{
		window:  window,
		groups:  make(map[string]*AlertGroup),
		flushCh: flushCh,
		stopCh:  make(chan struct{}),
	}
}

// Start 启动聚合器
func (a *Aggregator) Start() {
	go a.flushLoop()
}

// Stop 停止聚合器
func (a *Aggregator) Stop() {
	close(a.stopCh)
}

// Add 添加告警到聚合器
func (a *Aggregator) Add(alert *Alert) {
	key := a.buildGroupKey(alert)

	a.mu.Lock()
	group, ok := a.groups[key]
	if !ok {
		group = &AlertGroup{
			Key:          key,
			RuleID:       alert.RuleID,
			RuleName:     alert.RuleName,
			Severity:     alert.Severity,
			Labels:       alert.Labels,
			Alerts:       make([]*Alert, 0),
			FirstAlertAt: time.Now(),
		}
		a.groups[key] = group
	}
	group.Alerts = append(group.Alerts, alert)
	group.LastAlertAt = time.Now()
	group.Count++
	a.mu.Unlock()
}

// buildGroupKey 构建聚合键（按规则ID + 标签组合）
func (a *Aggregator) buildGroupKey(alert *Alert) string {
	parts := []string{alert.RuleID}
	// 按标签排序，确保一致性
	keys := make([]string, 0, len(alert.Labels))
	for k := range alert.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, alert.Labels[k]))
	}
	return strings.Join(parts, ",")
}

// flushLoop 定期刷新聚合组
func (a *Aggregator) flushLoop() {
	ticker := time.NewTicker(a.window)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.flush()
		case <-a.stopCh:
			return
		}
	}
}

// flush 刷新所有聚合组
func (a *Aggregator) flush() {
	a.mu.Lock()
	groups := make([]*AlertGroup, 0, len(a.groups))
	for _, group := range a.groups {
		groups = append(groups, group)
	}
	a.groups = make(map[string]*AlertGroup)
	a.mu.Unlock()

	for _, group := range groups {
		select {
		case a.flushCh <- group:
		case <-a.stopCh:
			return
		}
	}
}

// GetGroup 获取指定聚合组
func (a *Aggregator) GetGroup(key string) *AlertGroup {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.groups[key]
}

// GetGroupCount 获取当前聚合组数量
func (a *Aggregator) GetGroupCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.groups)
}

// ============================================================================
// 二、告警去重
// ============================================================================

// Deduplicator 告警去重器
type Deduplicator struct {
	window   time.Duration
	hashes   map[string]time.Time
	mu       sync.RWMutex
}

// NewDeduplicator 创建去重器
// window: 去重窗口，窗口内的相同告警会被去重
func NewDeduplicator(window time.Duration) *Deduplicator {
	return &Deduplicator{
		window: window,
		hashes: make(map[string]time.Time),
	}
}

// IsDuplicate 检查是否为重复告警
func (d *Deduplicator) IsDuplicate(alert *Alert) bool {
	hash := d.hashAlert(alert)

	d.mu.Lock()
	defer d.mu.Unlock()

	if lastSeen, ok := d.hashes[hash]; ok {
		if time.Since(lastSeen) < d.window {
			return true
		}
	}

	d.hashes[hash] = time.Now()
	return false
}

// hashAlert 计算告警指纹
func (d *Deduplicator) hashAlert(alert *Alert) string {
	return fmt.Sprintf("%s:%s:%s:%.2f",
		alert.RuleID, alert.Severity, alert.Message, alert.Value)
}

// Cleanup 清理过期的去重记录
func (d *Deduplicator) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for hash, lastSeen := range d.hashes {
		if now.Sub(lastSeen) > d.window*2 {
			delete(d.hashes, hash)
		}
	}
}

// ============================================================================
// 三、抖动检测
// ============================================================================

// FlapDetector 抖动检测器
type FlapDetector struct {
	threshold   int           // 窗口内状态变化次数阈值
	window      time.Duration // 检测窗口
	changes     map[string][]time.Time
	mu          sync.RWMutex
}

// NewFlapDetector 创建抖动检测器
// threshold: 窗口内状态变化次数阈值，超过则判定为抖动
// window: 检测窗口大小
func NewFlapDetector(threshold int, window time.Duration) *FlapDetector {
	return &FlapDetector{
		threshold: threshold,
		window:    window,
		changes:   make(map[string][]time.Time),
	}
}

// RecordChange 记录状态变化
func (f *FlapDetector) RecordChange(alertID string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	f.changes[alertID] = append(f.changes[alertID], now)

	// 清理过期记录
	cutoff := now.Add(-f.window)
	valid := make([]time.Time, 0)
	for _, t := range f.changes[alertID] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	f.changes[alertID] = valid
}

// IsFlapping 检查是否正在抖动
func (f *FlapDetector) IsFlapping(alertID string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	changes := f.changes[alertID]
	if len(changes) < f.threshold {
		return false
	}

	// 检查窗口内变化次数
	cutoff := time.Now().Add(-f.window)
	count := 0
	for _, t := range changes {
		if t.After(cutoff) {
			count++
		}
	}
	return count >= f.threshold
}

// GetFlapCount 获取抖动次数
func (f *FlapDetector) GetFlapCount(alertID string) int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.changes[alertID])
}

// Reset 重置指定告警的抖动记录
func (f *FlapDetector) Reset(alertID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.changes, alertID)
}

// ============================================================================
// 四、告警抑制（Inhibition）
// ============================================================================

// InhibitRule 抑制规则
type InhibitRule struct {
	SourceMatchers      []*LabelMatcher `json:"source_matchers"`      // 源告警匹配
	TargetMatchers      []*LabelMatcher `json:"target_matchers"`      // 目标告警匹配
	EqualLabels         []string        `json:"equal_labels"`         // 必须相等的标签
	Duration            time.Duration   `json:"duration"`             // 抑制持续时间
}

// Inhibitor 告警抑制器
type Inhibitor struct {
	rules     []*InhibitRule
	active    map[string]bool // 被抑制的告警指纹
	mu        sync.RWMutex
}

// NewInhibitor 创建抑制器
func NewInhibitor() *Inhibitor {
	return &Inhibitor{
		rules:  make([]*InhibitRule, 0),
		active: make(map[string]bool),
	}
}

// AddRule 添加抑制规则
func (i *Inhibitor) AddRule(rule *InhibitRule) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.rules = append(i.rules, rule)
}

// RemoveRule 移除抑制规则
func (i *Inhibitor) RemoveRule(index int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if index >= 0 && index < len(i.rules) {
		i.rules = append(i.rules[:index], i.rules[index+1:]...)
	}
}

// Inhibit 检查告警是否应该被抑制
func (i *Inhibitor) Inhibit(alert *Alert) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, rule := range i.rules {
		if i.matchAlert(rule.TargetMatchers, alert) {
			fingerprint := i.buildInhibitFingerprint(rule, alert)
			if i.active[fingerprint] {
				return true
			}
		}
	}
	return false
}

// ProcessSourceAlert 处理源告警（触发抑制）
func (i *Inhibitor) ProcessSourceAlert(sourceAlert *Alert) {
	i.mu.Lock()
	defer i.mu.Unlock()

	for _, rule := range i.rules {
		if i.matchAlert(rule.SourceMatchers, sourceAlert) {
			// 找到匹配的源告警，标记对应的抑制
			fingerprint := i.buildInhibitFingerprint(rule, sourceAlert)
			i.active[fingerprint] = true
		}
	}
}

// ReleaseSourceAlert 释放源告警（解除抑制）
func (i *Inhibitor) ReleaseSourceAlert(sourceAlert *Alert) {
	i.mu.Lock()
	defer i.mu.Unlock()

	for _, rule := range i.rules {
		if i.matchAlert(rule.SourceMatchers, sourceAlert) {
			fingerprint := i.buildInhibitFingerprint(rule, sourceAlert)
			delete(i.active, fingerprint)
		}
	}
}

// matchAlert 检查告警是否匹配匹配器列表
func (i *Inhibitor) matchAlert(matchers []*LabelMatcher, alert *Alert) bool {
	for _, matcher := range matchers {
		if !matcher.Match(alert.Labels) {
			return false
		}
	}
	return true
}

// buildInhibitFingerprint 构建抑制指纹
func (i *Inhibitor) buildInhibitFingerprint(rule *InhibitRule, sourceAlert *Alert) string {
	parts := []string{}
	for _, label := range rule.EqualLabels {
		if v, ok := sourceAlert.Labels[label]; ok {
			parts = append(parts, fmt.Sprintf("%s=%s", label, v))
		}
	}
	return strings.Join(parts, ",")
}

// alertFingerprint 计算告警指纹
func alertFingerprint(alert *Alert) string {
	keys := make([]string, 0, len(alert.Labels))
	for k := range alert.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := []string{alert.RuleID}
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, alert.Labels[k]))
	}
	return strings.Join(parts, ",")
}

// ============================================================================
// 五、综合降噪处理器
// ============================================================================

// NoiseReducer 综合降噪处理器
type NoiseReducer struct {
	aggregator    *Aggregator
	deduplicator  *Deduplicator
	flapDetector  *FlapDetector
	inhibitor     *Inhibitor
	outputCh      chan *AlertGroup
}

// NewNoiseReducer 创建综合降噪处理器
func NewNoiseReducer(aggWindow, dedupWindow time.Duration, flapThreshold int, flapWindow time.Duration) *NoiseReducer {
	outputCh := make(chan *AlertGroup, 100)
	return &NoiseReducer{
		aggregator:   NewAggregator(aggWindow, outputCh),
		deduplicator: NewDeduplicator(dedupWindow),
		flapDetector: NewFlapDetector(flapThreshold, flapWindow),
		inhibitor:    NewInhibitor(),
		outputCh:     outputCh,
	}
}

// Start 启动降噪处理器
func (nr *NoiseReducer) Start() {
	nr.aggregator.Start()
	go nr.dedupLoop()
}

// Stop 停止降噪处理器
func (nr *NoiseReducer) Stop() {
	nr.aggregator.Stop()
}

// Process 处理单条告警
func (nr *NoiseReducer) Process(alert *Alert) bool {
	// 1. 去重检查
	if nr.deduplicator.IsDuplicate(alert) {
		return false
	}

	// 2. 抖动检查
	nr.flapDetector.RecordChange(alert.ID)
	if nr.flapDetector.IsFlapping(alert.ID) {
		// 抖动告警，延迟处理
		return false
	}

	// 3. 抑制检查
	if nr.inhibitor.Inhibit(alert) {
		return false
	}

	// 4. 加入聚合
	nr.aggregator.Add(alert)
	return true
}

// dedupLoop 定期清理去重记录
func (nr *NoiseReducer) dedupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			nr.deduplicator.Cleanup()
		case <-nr.outputCh:
			// 消费聚合输出
		}
	}
}

// OutputChannel 获取聚合输出通道
func (nr *NoiseReducer) OutputChannel() <-chan *AlertGroup {
	return nr.outputCh
}

// AddInhibitRule 添加抑制规则
func (nr *NoiseReducer) AddInhibitRule(rule *InhibitRule) {
	nr.inhibitor.AddRule(rule)
}

// ProcessSourceAlert 处理源告警（触发抑制）
func (nr *NoiseReducer) ProcessSourceAlert(alert *Alert) {
	nr.inhibitor.ProcessSourceAlert(alert)
}

// ReleaseSourceAlert 释放源告警
func (nr *NoiseReducer) ReleaseSourceAlert(alert *Alert) {
	nr.inhibitor.ReleaseSourceAlert(alert)
}

// IsFlapping 检查告警是否抖动
func (nr *NoiseReducer) IsFlapping(alertID string) bool {
	return nr.flapDetector.IsFlapping(alertID)
}

// GetFlapCount 获取抖动次数
func (nr *NoiseReducer) GetFlapCount(alertID string) int {
	return nr.flapDetector.GetFlapCount(alertID)
}
