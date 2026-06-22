package alerting

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// 一、增强规则引擎测试
// ============================================================================

func TestNewEnhancedEngine(t *testing.T) {
	engine := NewEnhancedEngine(nil)
	assert.NotNil(t, engine)
	assert.NotNil(t, engine.rules)
}

func TestRegisterAndGetRule(t *testing.T) {
	engine := NewEnhancedEngine(nil)

	rule := &EnhancedRule{
		Rule: Rule{
			ID:      "test-rule",
			Name:    "Test Rule",
			Enabled: true,
		},
		Conditions: []*ConditionItem{
			{Metric: "cpu", Operator: OperatorGreaterThan, Value: 80},
		},
		Logic: LogicAnd,
	}
	engine.RegisterRule(rule)

	retrieved := engine.GetRule("test-rule")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "Test Rule", retrieved.Name)
	assert.Equal(t, 1, len(retrieved.Conditions))
}

func TestLabelMatcher(t *testing.T) {
	matcher := &LabelMatcher{Name: "env", Value: "prod"}
	assert.True(t, matcher.Match(map[string]string{"env": "prod"}))
	assert.False(t, matcher.Match(map[string]string{"env": "dev"}))
	assert.False(t, matcher.Match(map[string]string{}))

	// 正则匹配
	regexMatcher := &LabelMatcher{Name: "app", Value: "app-.*", Regex: true}
	assert.True(t, regexMatcher.Match(map[string]string{"app": "app-123"}))
	assert.False(t, regexMatcher.Match(map[string]string{"app": "other"}))

	// 反向匹配
	negativeMatcher := &LabelMatcher{Name: "env", Value: "test", Negative: true}
	assert.True(t, negativeMatcher.Match(map[string]string{"env": "prod"}))
	assert.False(t, negativeMatcher.Match(map[string]string{"env": "test"}))
}

func TestCompareValue(t *testing.T) {
	engine := NewEnhancedEngine(nil)

	assert.True(t, engine.compareValue(100, OperatorGreaterThan, 50))
	assert.False(t, engine.compareValue(30, OperatorGreaterThan, 50))

	assert.True(t, engine.compareValue(30, OperatorLessThan, 50))
	assert.False(t, engine.compareValue(100, OperatorLessThan, 50))

	assert.True(t, engine.compareValue(50, OperatorGreaterOrEqual, 50))
	assert.True(t, engine.compareValue(50, OperatorLessOrEqual, 50))
	assert.True(t, engine.compareValue(50, OperatorEqual, 50))
	assert.True(t, engine.compareValue(50, OperatorNotEqual, 60))
}

func TestEvalExpression(t *testing.T) {
	engine := NewEnhancedEngine(nil)

	assert.Equal(t, 150.0, engine.evalExpression("value + 50", 100))
	assert.Equal(t, 50.0, engine.evalExpression("value - 50", 100))
	assert.Equal(t, 200.0, engine.evalExpression("value * 2", 100))
	assert.Equal(t, 50.0, engine.evalExpression("value / 2", 100))
	assert.Equal(t, 100.0, engine.evalExpression("invalid expr", 100)) // 解析失败返回原值
}

func TestTransitionState(t *testing.T) {
	engine := NewEnhancedEngine(nil)

	rule := &EnhancedRule{
		Rule: Rule{ID: "test", Name: "Test"},
		ForDuration: Duration{1 * time.Second},
	}
	engine.RegisterRule(rule)

	groupKey := "test-group"

	// 空 → pending（满足条件）
	state := engine.transitionState(rule, groupKey, true, 100)
	assert.Equal(t, StatePending, state)

	// 等待超过 ForDuration → firing
	time.Sleep(1100 * time.Millisecond)
	state = engine.transitionState(rule, groupKey, true, 100)
	assert.Equal(t, StateFiring, state)

	// 继续满足 → firing
	state = engine.transitionState(rule, groupKey, true, 100)
	assert.Equal(t, StateFiring, state)

	// 不满足 → resolved（keep_firing_for = 0）
	state = engine.transitionState(rule, groupKey, false, 100)
	assert.Equal(t, StateResolved, state)
}

// ============================================================================
// 二、通知通道测试
// ============================================================================

func TestNewChannelManager(t *testing.T) {
	cm := NewChannelManager()
	assert.NotNil(t, cm)
	assert.Equal(t, 0, len(cm.GetChannelNames()))
}

func TestChannelManager_RegisterAndSend(t *testing.T) {
	cm := NewChannelManager()

	// 注册 mock 通道
	mockCh := &MockChannel{name: "mock"}
	cm.RegisterChannel("mock", mockCh)
	assert.Equal(t, 1, len(cm.GetChannelNames()))

	alert := &Alert{ID: "1", RuleName: "test", Severity: "warning"}
	tmpl := &AlertTemplate{Title: "Test Alert"}

	err := cm.Send(context.Background(), alert, tmpl)
	assert.NoError(t, err)
	assert.True(t, mockCh.sent)

	// 注销
	cm.UnregisterChannel("mock")
	assert.Equal(t, 0, len(cm.GetChannelNames()))
}

func TestSeverityColor(t *testing.T) {
	assert.Equal(t, "#FF0000", severityColor("critical"))
	assert.Equal(t, "#FF8C00", severityColor("warning"))
	assert.Equal(t, "#1890FF", severityColor("info"))
	assert.Equal(t, "#666666", severityColor("unknown"))
}

func TestParseSeverity(t *testing.T) {
	assert.Equal(t, PriorityCritical, ParseSeverity("critical"))
	assert.Equal(t, PriorityWarning, ParseSeverity("warning"))
	assert.Equal(t, PriorityInfo, ParseSeverity("info"))
}

// MockChannel 模拟通知通道
type MockChannel struct {
	name string
	sent bool
	err  error
}

func (m *MockChannel) Name() string { return m.name }
func (m *MockChannel) Send(ctx context.Context, alert *Alert, tmpl *AlertTemplate) error {
	m.sent = true
	return m.err
}
func (m *MockChannel) HealthCheck() error { return nil }

// ============================================================================
// 三、聚合与降噪测试
// ============================================================================

func TestNewAggregator(t *testing.T) {
	flushCh := make(chan *AlertGroup, 10)
	agg := NewAggregator(time.Minute, flushCh)
	assert.NotNil(t, agg)
}

func TestAggregator_AddAndFlush(t *testing.T) {
	flushCh := make(chan *AlertGroup, 10)
	agg := NewAggregator(100*time.Millisecond, flushCh)
	agg.Start()
	defer agg.Stop()

	alert1 := &Alert{ID: "1", RuleID: "r1", RuleName: "CPU", Severity: "warning", Labels: map[string]string{"host": "h1"}}
	alert2 := &Alert{ID: "2", RuleID: "r1", RuleName: "CPU", Severity: "warning", Labels: map[string]string{"host": "h1"}}

	agg.Add(alert1)
	agg.Add(alert2)

	assert.Equal(t, 1, agg.GetGroupCount())
	assert.Equal(t, 2, agg.GetGroup(agg.buildGroupKey(alert1)).Count)
}

func TestDeduplicator(t *testing.T) {
	dedup := NewDeduplicator(100 * time.Millisecond)

	alert1 := &Alert{ID: "1", RuleID: "r1", Severity: "warning", Message: "test", Value: 100}
	alert2 := &Alert{ID: "2", RuleID: "r1", Severity: "warning", Message: "test", Value: 100}

	assert.False(t, dedup.IsDuplicate(alert1))
	assert.True(t, dedup.IsDuplicate(alert2)) // 100ms 内重复

	time.Sleep(150 * time.Millisecond)
	assert.False(t, dedup.IsDuplicate(alert2)) // 超过窗口，不重复
}

func TestFlapDetector(t *testing.T) {
	fd := NewFlapDetector(3, 500*time.Millisecond)

	alertID := "alert-1"
	fd.RecordChange(alertID)
	assert.False(t, fd.IsFlapping(alertID))
	fd.RecordChange(alertID)
	assert.False(t, fd.IsFlapping(alertID))
	fd.RecordChange(alertID)
	assert.True(t, fd.IsFlapping(alertID))

	fd.Reset(alertID)
	assert.False(t, fd.IsFlapping(alertID))
}

func TestInhibitor(t *testing.T) {
	inhibitor := NewInhibitor()

	// 添加抑制规则：父告警抑制子告警
	inhibitor.AddRule(&InhibitRule{
		SourceMatchers: []*LabelMatcher{{Name: "severity", Value: "critical"}},
		TargetMatchers: []*LabelMatcher{{Name: "severity", Value: "warning"}},
		EqualLabels:    []string{"host"},
	})

	sourceAlert := &Alert{ID: "s1", Labels: map[string]string{"severity": "critical", "host": "h1"}}
	targetAlert := &Alert{ID: "t1", Labels: map[string]string{"severity": "warning", "host": "h1"}}

	assert.False(t, inhibitor.Inhibit(targetAlert)) // 还没触发源告警

	inhibitor.ProcessSourceAlert(sourceAlert)
	assert.True(t, inhibitor.Inhibit(targetAlert))

	inhibitor.ReleaseSourceAlert(sourceAlert)
	assert.False(t, inhibitor.Inhibit(targetAlert))
}

func TestNoiseReducer(t *testing.T) {
	reducer := NewNoiseReducer(100*time.Millisecond, 50*time.Millisecond, 3, 500*time.Millisecond)
	reducer.Start()
	defer reducer.Stop()

	alert := &Alert{ID: "1", RuleID: "r1", RuleName: "CPU", Severity: "warning", Labels: map[string]string{"host": "h1"}, Value: 90}

	// 第一次处理应通过
	assert.True(t, reducer.Process(alert))

	// 重复处理应被去重
	assert.False(t, reducer.Process(alert))
}

// ============================================================================
// 四、升级机制测试
// ============================================================================

func TestNewEscalationManager(t *testing.T) {
	em := NewEscalationManager(nil)
	assert.NotNil(t, em)
	assert.NotNil(t, em.policies["default"])
}

func TestEscalationManager_StartAndAcknowledge(t *testing.T) {
	em := NewEscalationManager(nil)

	alert := &Alert{ID: "a1", RuleName: "Test"}
	record := em.StartEscalation(alert, "default")
	assert.NotNil(t, record)
	assert.Equal(t, 0, record.CurrentStep)
	assert.False(t, record.Acknowledged)

	// 确认
	assert.True(t, em.Acknowledge("a1", "user-1"))
	assert.True(t, em.IsAcknowledged("a1"))

	// 重复确认
	assert.True(t, em.Acknowledge("a1", "user-2")) // 仍然返回 true

	// 不存在的告警
	assert.False(t, em.Acknowledge("unknown", "user"))
}

func TestEscalationManager_Resolve(t *testing.T) {
	em := NewEscalationManager(nil)
	alert := &Alert{ID: "a1", RuleName: "Test"}
	em.StartEscalation(alert, "default")

	assert.NotNil(t, em.GetRecord("a1"))
	em.Resolve("a1")
	assert.Nil(t, em.GetRecord("a1"))
}

func TestEscalationManager_CheckAndEscalate(t *testing.T) {
	em := NewEscalationManager(nil)

	alert := &Alert{ID: "a1", RuleName: "Test"}
	em.StartEscalation(alert, "default")

	// 立即检查（0 分钟）—— 不应升级
	step, escalated := em.CheckAndEscalate(alert)
	assert.False(t, escalated)
	assert.Nil(t, step)

	// 模拟已确认
	em.Acknowledge("a1", "user")
	step, escalated = em.CheckAndEscalate(alert)
	assert.False(t, escalated)
}

func TestAutoEscalateBySeverity(t *testing.T) {
	policy := AutoEscalateBySeverity("critical", nil)
	assert.Equal(t, 2, len(policy.Steps)) // critical 立即升级到更短步骤

	policy = AutoEscalateBySeverity("warning", nil)
	assert.Equal(t, 4, len(policy.Steps)) // warning 使用默认步骤
}

func TestEscalationManager_GetStats(t *testing.T) {
	em := NewEscalationManager(nil)
	alert := &Alert{ID: "a1", RuleName: "Test"}
	em.StartEscalation(alert, "default")
	em.Acknowledge("a1", "user-1")

	stats := em.GetStats()
	assert.Equal(t, 1, stats.TotalEscalations)
	assert.Equal(t, 1, stats.AcknowledgedCount)
	assert.Equal(t, 0, stats.UnacknowledgedCount)
}

// ============================================================================
// 五、静默与维护窗口测试
// ============================================================================

func TestSilenceRule(t *testing.T) {
	rule := &SilenceRule{
		ID:      "s1",
		Name:    "Silence CPU",
		Matchers: []*LabelMatcher{{Name: "rule", Value: "cpu"}},
		StartAt: time.Now().Add(-time.Hour),
		EndAt:   time.Now().Add(time.Hour),
		Active:  true,
	}
	assert.True(t, rule.IsActive())

	alert := &Alert{ID: "a1", Labels: map[string]string{"rule": "cpu"}}
	assert.True(t, rule.Match(alert))

	alert2 := &Alert{ID: "a2", Labels: map[string]string{"rule": "memory"}}
	assert.False(t, rule.Match(alert2))

	assert.Greater(t, rule.RemainingDuration(), time.Duration(0))
}

func TestSilencer(t *testing.T) {
	silencer := NewSilencer()

	rule := &SilenceRule{
		ID:      "s1",
		Matchers: []*LabelMatcher{{Name: "env", Value: "prod"}},
		StartAt: time.Now().Add(-time.Hour),
		EndAt:   time.Now().Add(time.Hour),
		Active:  true,
	}
	silencer.AddRule(rule)
	assert.Equal(t, 1, len(silencer.GetAllRules()))

	alert := &Alert{ID: "a1", Labels: map[string]string{"env": "prod"}}
	silenced, ruleID := silencer.IsSilenced(alert)
	assert.True(t, silenced)
	assert.Equal(t, "s1", ruleID)

	alert2 := &Alert{ID: "a2", Labels: map[string]string{"env": "dev"}}
	silenced, _ = silencer.IsSilenced(alert2)
	assert.False(t, silenced)

	// 清理过期
	silencer.RemoveRule("s1")
	assert.Equal(t, 0, len(silencer.GetAllRules()))
}

func TestMaintenanceWindow(t *testing.T) {
	now := time.Now()
	window := &MaintenanceWindow{
		ID:        "mw1",
		Name:      "Daily Maintenance",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		Active:    true,
		Recurrence: RecurrenceNone,
	}
	assert.True(t, window.IsActive())
	assert.True(t, window.IsInWindow(now))
	assert.False(t, window.IsInWindow(now.Add(2*time.Hour)))
}

func TestMaintenanceWindow_Daily(t *testing.T) {
	now := time.Now()
	window := &MaintenanceWindow{
		ID:         "daily",
		StartTime:  now.Add(-2 * time.Hour),
		EndTime:    now.Add(2 * time.Hour),
		Active:     true,
		Recurrence: RecurrenceDaily,
	}
	assert.True(t, window.IsInWindow(now))
}

func TestMaintenanceManager(t *testing.T) {
	mm := NewMaintenanceManager()

	window := &MaintenanceWindow{
		ID:        "mw1",
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now().Add(time.Hour),
		Active:    true,
	}
	mm.AddWindow(window)
	assert.True(t, mm.IsInMaintenanceWindow())
	assert.Equal(t, 1, len(mm.GetActiveWindows()))

	mm.RemoveWindow("mw1")
	assert.False(t, mm.IsInMaintenanceWindow())
}

func TestAlertController(t *testing.T) {
	ac := NewAlertController()

	// 添加维护窗口
	ac.AddMaintenanceWindow(&MaintenanceWindow{
		ID:        "mw1",
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now().Add(time.Hour),
		Active:    true,
	})

	alert := &Alert{ID: "a1", Labels: map[string]string{"env": "prod"}}
	suppressed, reason := ac.ShouldSuppress(alert)
	assert.True(t, suppressed)
	assert.Equal(t, "in_maintenance_window", reason)

	// 移除维护窗口
	ac.RemoveMaintenanceWindow("mw1")
	suppressed, _ = ac.ShouldSuppress(alert)
	assert.False(t, suppressed)
}

// ============================================================================
// 六、历史与统计测试
// ============================================================================

func TestNewAlertStats(t *testing.T) {
	stats := NewAlertStats()
	assert.NotNil(t, stats)
	assert.Equal(t, 0, stats.TotalCount)
}

func TestAlertStats_AddRecord(t *testing.T) {
	stats := NewAlertStats()

	now := time.Now()
	record := &AlertHistoryRecord{
		RuleID:      "r1",
		Severity:    "warning",
		TriggeredAt: now,
		ResolvedAt:  &now,
		Acknowledged: true,
	}
	stats.AddRecord(record)

	assert.Equal(t, 1, stats.TotalCount)
	assert.Equal(t, 1, stats.ResolvedCount)
	assert.Equal(t, 1, stats.AcknowledgedCount)
	assert.Equal(t, 1, stats.ByRule["r1"])
	assert.Equal(t, 1, stats.BySeverity["warning"])
	assert.Equal(t, 1, stats.ByHour[now.Hour()])
	assert.Equal(t, 1, stats.ByDay[now.Format("2006-01-02")])
}

func TestMemoryHistoryStore(t *testing.T) {
	store := NewMemoryHistoryStore(100)

	record := &AlertHistoryRecord{
		AlertID:     "a1",
		RuleID:      "r1",
		RuleName:    "CPU High",
		Severity:    "warning",
		TriggeredAt: time.Now(),
	}
	assert.NoError(t, store.Save(record))

	// 查询
	records, err := store.Query(context.Background(), &HistoryFilter{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(records))

	// 计数
	count, err := store.Count(context.Background(), &HistoryFilter{})
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	// 更新记录
	now := time.Now()
	record.ResolvedAt = &now
	assert.NoError(t, store.Save(record))
	records, _ = store.Query(context.Background(), &HistoryFilter{})
	assert.True(t, records[0].IsResolved())
}

func TestHistoryFilter(t *testing.T) {
	filter := &HistoryFilter{
		RuleID:   "r1",
		Severity: "warning",
	}

	record := &AlertHistoryRecord{
		RuleID:   "r1",
		Severity: "warning",
	}
	assert.True(t, filter.Match(record))

	record2 := &AlertHistoryRecord{
		RuleID:   "r2",
		Severity: "warning",
	}
	assert.False(t, filter.Match(record2))
}

func TestHistoryManager(t *testing.T) {
	store := NewMemoryHistoryStore(100)
	hm := NewHistoryManager(store)

	alert := &Alert{ID: "a1", RuleID: "r1", RuleName: "CPU", Severity: "warning", CreatedAt: time.Now()}
	assert.NoError(t, hm.Record(alert, []string{"email"}))

	// 查询
	records, err := hm.Query(context.Background(), &HistoryFilter{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(records))

	// 记录解决
	assert.NoError(t, hm.RecordResolved("a1"))
	records, _ = hm.Query(context.Background(), &HistoryFilter{Resolved: boolPtr(true)})
	assert.Equal(t, 1, len(records))

	// 记录确认
	assert.NoError(t, hm.RecordAcknowledged("a1"))
	records, _ = hm.Query(context.Background(), &HistoryFilter{Acknowledged: boolPtr(true)})
	assert.Equal(t, 1, len(records))

	// 统计
	stats, err := hm.GetStats(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	assert.NoError(t, err)
	assert.Equal(t, 1, stats.TotalCount)
}

func TestHistoryManager_GetTrend(t *testing.T) {
	store := NewMemoryHistoryStore(100)
	hm := NewHistoryManager(store)

	now := time.Now()
	for i := 0; i < 5; i++ {
		alert := &Alert{
			ID:      fmt.Sprintf("a%d", i),
			RuleID:  "r1",
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
		}
		hm.Record(alert, nil)
	}

	trend, err := hm.GetTrend(context.Background(), now.Add(-time.Hour), now.Add(time.Hour), 10*time.Minute)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(trend), 1)
}

func TestHistoryManager_GetTopRules(t *testing.T) {
	store := NewMemoryHistoryStore(100)
	hm := NewHistoryManager(store)

	now := time.Now()
	for i := 0; i < 10; i++ {
		hm.Record(&Alert{ID: fmt.Sprintf("a%d", i), RuleID: "r1", RuleName: "CPU", CreatedAt: now}, nil)
	}
	for i := 0; i < 5; i++ {
		hm.Record(&Alert{ID: fmt.Sprintf("b%d", i), RuleID: "r2", RuleName: "Memory", CreatedAt: now}, nil)
	}

	top, err := hm.GetTopRules(context.Background(), now.Add(-time.Hour), now.Add(time.Hour), 2)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(top))
	assert.Equal(t, "r1", top[0].RuleID)
	assert.Equal(t, 10, top[0].Count)
}

func boolPtr(b bool) *bool {
	return &b
}

// ============================================================================
// 七、综合集成测试
// ============================================================================

func TestAlertController_Integration(t *testing.T) {
	ac := NewAlertController()

	// 添加静默规则
	ac.AddSilenceRule(&SilenceRule{
		ID:      "s1",
		Matchers: []*LabelMatcher{{Name: "host", Value: "host1"}},
		StartAt: time.Now().Add(-time.Hour),
		EndAt:   time.Now().Add(time.Hour),
		Active:  true,
	})

	// 添加抑制规则
	ac.AddInhibitRule(&InhibitRule{
		SourceMatchers: []*LabelMatcher{{Name: "severity", Value: "critical"}},
		TargetMatchers: []*LabelMatcher{{Name: "severity", Value: "warning"}},
		EqualLabels:    []string{"host"},
	})

	// 静默告警
	alert1 := &Alert{ID: "a1", Labels: map[string]string{"host": "host1"}}
	suppressed, reason := ac.ShouldSuppress(alert1)
	assert.True(t, suppressed)
	assert.Equal(t, "silenced_by_rule_s1", reason)

	// 非静默告警
	alert2 := &Alert{ID: "a2", Labels: map[string]string{"host": "host2"}}
	suppressed, _ = ac.ShouldSuppress(alert2)
	assert.False(t, suppressed)

	// 抑制告警
	source := &Alert{ID: "s1", Labels: map[string]string{"severity": "critical", "host": "h1"}}
	target := &Alert{ID: "t1", Labels: map[string]string{"severity": "warning", "host": "h1"}}
	ac.ProcessSourceAlert(source)
	suppressed, _ = ac.ShouldSuppress(target)
	assert.True(t, suppressed)

	ac.ReleaseSourceAlert(source)
	suppressed, _ = ac.ShouldSuppress(target)
	assert.False(t, suppressed)
}
