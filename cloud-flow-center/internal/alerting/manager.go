// Package alerting 提供告警规则引擎和通知功能
package alerting

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"cloudflow-center/pkg/logger"
	"cloudflow-center/internal/storage"
	"cloudflow/pkg/utils"
	edge "cloudflow/proto"
)

// Alert 告警信息
type Alert struct {
	ID        string            `json:"id"`
	RuleID    string            `json:"rule_id"`
	RuleName  string            `json:"rule_name"`
	Severity  string            `json:"severity"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels"`
	Value     float64           `json:"value"`
	Threshold float64           `json:"threshold"`
	CreatedAt time.Time         `json:"created_at"`
	Resolved  bool              `json:"resolved"`
	ResolvedAt time.Time        `json:"resolved_at"`
}

// AlertManager 告警管理器
type AlertManager struct {
	mu            sync.RWMutex
	ruleManager   *RuleManager
	storage       storage.StorageEngine
	db            *sql.DB
	notifier      Notifier
	alerts        map[string]*Alert
	alertHistory  []*Alert
	logger        *logger.Logger
	stopCh        chan struct{}
	stopped       sync.Once
	checkInterval time.Duration
}

// NewAlertManager 创建告警管理器
func NewAlertManager(ruleDir string, store storage.StorageEngine, db *sql.DB, notifier Notifier, log *logger.Logger, checkInterval time.Duration) *AlertManager {
	ruleManager := NewRuleManager(ruleDir, log)
	if err := ruleManager.LoadRules(); err != nil {
		log.Warnf("加载告警规则失败: %v", err)
	}

	if checkInterval <= 0 {
		checkInterval = 10 * time.Second
	}

	return &AlertManager{
		ruleManager:   ruleManager,
		storage:       store,
		db:            db,
		notifier:      notifier,
		alerts:        make(map[string]*Alert),
		alertHistory:  make([]*Alert, 0),
		logger:        log,
		stopCh:        make(chan struct{}),
		checkInterval: checkInterval,
	}
}

// Start 启动告警管理器
func (am *AlertManager) Start() {
	am.loadAlertHistory()
	go am.checkLoop()
	am.logger.Info("告警管理器已启动")
}

func (am *AlertManager) loadAlertHistory() {
	if am.db == nil {
		am.logger.Warn("数据库连接不可用，跳过加载告警历史记录")
		return
	}

	rows, err := am.db.Query(
		`SELECT alert_id, rule_id, rule_name, severity, message, labels, value, threshold, created_at, resolved, resolved_at
		 FROM alert_history ORDER BY created_at DESC LIMIT 1000`)
	if err != nil {
		am.logger.Warnf("加载告警历史记录失败: %v", err)
		return
	}
	defer rows.Close()

	am.mu.Lock()
	defer am.mu.Unlock()

	loaded := 0
	for rows.Next() {
		var alert Alert
		var labelsJSON []byte
		var resolvedAt sql.NullTime

		if err := rows.Scan(
			&alert.ID, &alert.RuleID, &alert.RuleName, &alert.Severity,
			&alert.Message, &labelsJSON, &alert.Value, &alert.Threshold,
			&alert.CreatedAt, &alert.Resolved, &resolvedAt,
		); err != nil {
			am.logger.Warnf("扫描告警历史记录失败: %v", err)
			continue
		}

		if len(labelsJSON) > 0 {
			if err := json.Unmarshal(labelsJSON, &alert.Labels); err != nil {
				am.logger.Warnf("反序列化告警 labels 失败: %v, 原始数据: %s", err, string(labelsJSON))
			}
		}
		if alert.Labels == nil {
			alert.Labels = make(map[string]string)
		}
		if resolvedAt.Valid {
			alert.ResolvedAt = resolvedAt.Time
		}

		am.alertHistory = append(am.alertHistory, &alert)

		if !alert.Resolved {
			am.alerts[alert.ID] = &alert
		}
		loaded++
	}
	if err := rows.Err(); err != nil {
		am.logger.Warnf("遍历告警历史记录失败: %v", err)
	}

	am.logger.Infof("从数据库加载了 %d 条告警历史记录", loaded)
}

// Stop 停止告警管理器
func (am *AlertManager) Stop() {
	am.stopped.Do(func() {
		close(am.stopCh)
		am.logger.Info("告警管理器已停止")
	})
}

func (am *AlertManager) checkLoop() {
	ticker := time.NewTicker(am.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			am.checkRules()
		case <-am.stopCh:
			return
		}
	}
}

func (am *AlertManager) checkRules() {
	rules := am.ruleManager.GetRules()
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		am.checkRule(rule)
	}
}

func (am *AlertManager) checkRule(rule *Rule) {
	if am.storage == nil {
		am.logger.Warnf("存储引擎不可用，跳过规则检查: %s", rule.Name)
		return
	}

	sampleInterval := 10 * time.Second
	dataPoints := int(rule.Duration.Duration / sampleInterval)
	
	if dataPoints < 5 {
		dataPoints = 5
	}
	if dataPoints > 100 {
		dataPoints = 100
	}

	metrics, err := am.storage.GetRecentMetrics(string(rule.Type), dataPoints, rule.Duration.Duration)
	if err != nil {
		am.logger.Warnf("获取指标数据失败: %v", err)
		return
	}

	if am.evaluateRuleWithDuration(rule, metrics) {
		if len(metrics) > 0 {
			latestMetric := metrics[0]
			value := extractMetricValue(rule, latestMetric)
			if value != nil {
				am.triggerAlert(rule, latestMetric, *value)
			}
		}
	} else {
		am.resolveAlertsByRule(rule.ID)
	}
}

func (am *AlertManager) evaluateRuleWithDuration(rule *Rule, metrics []*edge.MetricData) bool {
	if len(metrics) == 0 {
		return false
	}

	timeWindowStart := time.Now().Add(-rule.Duration.Duration)

	satisfiedCount := 0
	totalCount := 0

	for _, metric := range metrics {
		metricTime := time.Unix(metric.Timestamp, 0)
		if metricTime.Before(timeWindowStart) {
			continue
		}

		totalCount++
		if am.evaluateRule(rule, metric) {
			satisfiedCount++
		}
	}

	satisfyThreshold := rule.SatisfyThreshold
	if satisfyThreshold <= 0 {
		satisfyThreshold = 1.0
	}
	if satisfyThreshold > 1.0 {
		satisfyThreshold = 1.0
	}

	satisfyRatio := float64(satisfiedCount) / float64(totalCount)
	return totalCount > 0 && satisfyRatio >= satisfyThreshold
}

func (am *AlertManager) evaluateRule(rule *Rule, metric *edge.MetricData) bool {
	value := extractMetricValue(rule, metric)
	if value == nil {
		return false
	}

	switch rule.Condition.Operator {
	case OperatorGreaterThan:
		return *value > rule.Threshold
	case OperatorLessThan:
		return *value < rule.Threshold
	case OperatorGreaterOrEqual:
		return *value >= rule.Threshold
	case OperatorLessOrEqual:
		return *value <= rule.Threshold
	case OperatorEqual:
		return *value == rule.Threshold
	case OperatorNotEqual:
		return *value != rule.Threshold
	default:
		return false
	}
}

func extractMetricValue(rule *Rule, metric *edge.MetricData) *float64 {
	var value float64
	switch rule.Type {
	case RuleTypeCPU:
		if cpuUsage, ok := metric.Tags["cpu_usage"]; ok {
			if cpu, err := utils.ParseFloat(cpuUsage); err == nil {
				value = cpu
				return &value
			}
		}
	case RuleTypeMemory:
		if memUsage, ok := metric.Tags["memory_usage"]; ok {
			if mem, err := utils.ParseFloat(memUsage); err == nil {
				value = mem
				return &value
			}
		}
	case RuleTypeNetwork:
		value = float64(metric.Bytes)
		return &value
	case RuleTypeDisk:
		if diskUsage, ok := metric.Tags["disk_usage"]; ok {
			if du, err := utils.ParseFloat(diskUsage); err == nil {
				value = du
				return &value
			}
		}
		value = float64(metric.Bytes)
		return &value
	case RuleTypeTraffic:
		value = float64(metric.Bytes)
		return &value
	}
	return nil
}

func (am *AlertManager) triggerAlert(rule *Rule, metric *edge.MetricData, value float64) {
	am.mu.Lock()
	for _, existingAlert := range am.alerts {
		if existingAlert.RuleID == rule.ID && !existingAlert.Resolved {
			am.mu.Unlock()
			am.logger.Debugf("规则 %s 已有活跃告警，跳过重复触发", rule.Name)
			return
		}
	}

	alertID := fmt.Sprintf("%s-%d", rule.ID, time.Now().Unix())
	alert := &Alert{
		ID:        alertID,
		RuleID:    rule.ID,
		RuleName:  rule.Name,
		Severity:  rule.Severity,
		Message:   fmt.Sprintf("规则 %s 触发: 指标值 %.2f %s 阈值 %.2f", rule.Name, value, getOperatorString(rule.Condition.Operator), rule.Threshold),
		Labels:    rule.Labels,
		Value:     value,
		Threshold: rule.Threshold,
		CreatedAt: time.Now(),
		Resolved:  false,
	}

	am.alerts[alertID] = alert
	am.alertHistory = append(am.alertHistory, alert)
	if len(am.alertHistory) > 1000 {
		newHistory := make([]*Alert, 1000)
		copy(newHistory, am.alertHistory[len(am.alertHistory)-1000:])
		am.alertHistory = newHistory
	}
	am.mu.Unlock()

	if am.db != nil {
		labelsJSON, err := json.Marshal(alert.Labels)
		if err != nil {
			am.logger.Warnf("序列化告警 labels 失败: %v", err)
			labelsJSON = []byte("{}")
		}
		_, err = am.db.Exec(
			`INSERT INTO alert_history (alert_id, rule_id, rule_name, severity, message, labels, value, threshold, created_at, resolved)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			alert.ID, alert.RuleID, alert.RuleName, alert.Severity,
			alert.Message, string(labelsJSON), alert.Value, alert.Threshold,
			alert.CreatedAt, false,
		)
		if err != nil {
			am.logger.Warnf("写入告警历史到数据库失败: %v", err)
		}
	}

	if am.notifier != nil {
		if err := am.notifier.Notify(alert); err != nil {
			am.logger.Warnf("发送告警通知失败: %v", err)
		}
	}

	am.logger.Infof("触发告警: %s, 严重程度: %s", alert.Message, alert.Severity)
}

// GetActiveAlerts 获取活跃告警
func (am *AlertManager) GetActiveAlerts() []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make([]*Alert, 0)
	for _, alert := range am.alerts {
		if !alert.Resolved {
			result = append(result, alert)
		}
	}
	return result
}

// GetAlertHistory 获取告警历史
func (am *AlertManager) GetAlertHistory() []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make([]*Alert, len(am.alertHistory))
	copy(result, am.alertHistory)
	return result
}

// ResolveAlert 解决告警
func (am *AlertManager) ResolveAlert(alertID string) error {
	am.mu.Lock()
	alert, ok := am.alerts[alertID]
	if !ok {
		am.mu.Unlock()
		return fmt.Errorf("告警不存在: %s", alertID)
	}

	alert.Resolved = true
	delete(am.alerts, alertID)
	am.mu.Unlock()

	if am.db != nil {
		_, err := am.db.Exec(
			`UPDATE alert_history SET resolved = ?, resolved_at = ? WHERE alert_id = ?`,
			true, time.Now(), alertID,
		)
		if err != nil {
			am.logger.Warnf("更新告警历史到数据库失败: %v", err)
		}
	}

	if am.notifier != nil {
		if err := am.notifier.Notify(alert); err != nil {
			am.logger.Warnf("发送告警解决通知失败: %v", err)
		}
	}

	am.logger.Infof("解决告警: %s", alert.Message)
	return nil
}

func (am *AlertManager) resolveAlertsByRule(ruleID string) {
	am.mu.Lock()
	var toResolve []*Alert
	for _, alert := range am.alerts {
		if alert.RuleID == ruleID && !alert.Resolved {
			alert.Resolved = true
			toResolve = append(toResolve, alert)
			delete(am.alerts, alert.ID)
		}
	}
	am.mu.Unlock()

	for _, alert := range toResolve {
		if am.db != nil {
			_, err := am.db.Exec(
				`UPDATE alert_history SET resolved = ?, resolved_at = ? WHERE alert_id = ?`,
				true, time.Now(), alert.ID,
			)
			if err != nil {
				am.logger.Warnf("更新告警历史到数据库失败: %v", err)
			}
		}

		if am.notifier != nil {
			if err := am.notifier.Notify(alert); err != nil {
				am.logger.Warnf("发送告警解决通知失败: %v", err)
			}
		}

		am.logger.Infof("告警自动恢复: %s", alert.Message)
	}
}

// GetRuleManager 获取规则管理器
func (am *AlertManager) GetRuleManager() *RuleManager {
	return am.ruleManager
}

func getOperatorString(operator ConditionOperator) string {
	switch operator {
	case OperatorGreaterThan:
		return "超过"
	case OperatorLessThan:
		return "低于"
	case OperatorGreaterOrEqual:
		return "大于等于"
	case OperatorLessOrEqual:
		return "小于等于"
	case OperatorEqual:
		return "等于"
	case OperatorNotEqual:
		return "不等于"
	default:
		return ""
	}
}
