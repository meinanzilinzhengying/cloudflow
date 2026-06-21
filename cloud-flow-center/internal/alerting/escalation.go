// P2: 告警升级机制 — 按时间/未确认/严重级别升级通知
//
// 功能：
//   - 按时间升级：首次通知 → 15分钟 → 1小时 → 4小时
//   - 按未确认升级：未确认告警升级严重级别和通知范围
//   - 按严重级别升级：critical 立即升级
//   - 升级通知通道：邮件 → 钉钉 → 短信 → 电话
//
package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 一、升级策略
// ============================================================================

// EscalationPolicy 升级策略
type EscalationPolicy struct {
	Name        string               `json:"name"`
	Steps       []EscalationStep     `json:"steps"`       // 升级步骤
	Repeat      bool                 `json:"repeat"`      // 是否循环升级
	RepeatInterval time.Duration     `json:"repeat_interval"` // 循环间隔
}

// EscalationStep 升级步骤
type EscalationStep struct {
	Delay        time.Duration `json:"delay"`          // 等待时间
	Channels     []string      `json:"channels"`       // 通知通道
	Severity     string        `json:"severity"`       // 升级后的严重级别
	RequireAck   bool          `json:"require_ack"`    // 是否需要确认
	NotifyUsers  []string      `json:"notify_users"`   // 通知用户列表
}

// DefaultEscalationPolicy 默认升级策略
func DefaultEscalationPolicy() *EscalationPolicy {
	return &EscalationPolicy{
		Name: "default",
		Steps: []EscalationStep{
			{Delay: 0, Channels: []string{"email"}, Severity: "", RequireAck: true},
			{Delay: 15 * time.Minute, Channels: []string{"email", "dingtalk"}, Severity: "warning", RequireAck: true},
			{Delay: 1 * time.Hour, Channels: []string{"email", "dingtalk", "wechat"}, Severity: "critical", RequireAck: true},
			{Delay: 4 * time.Hour, Channels: []string{"email", "dingtalk", "wechat", "sms"}, Severity: "critical", RequireAck: false},
		},
		Repeat:         true,
		RepeatInterval: 24 * time.Hour,
	}
}

// ============================================================================
// 二、升级管理器
// ============================================================================

// EscalationRecord 升级记录
type EscalationRecord struct {
	AlertID        string
	Policy         string
	CurrentStep    int
	TotalSteps     int
	StartedAt      time.Time
	LastEscalatedAt time.Time
	Acknowledged   bool
	AcknowledgedAt *time.Time
	AcknowledgedBy string
}

// EscalationManager 升级管理器
type EscalationManager struct {
	policies  map[string]*EscalationPolicy
	records   map[string]*EscalationRecord
	mu        sync.RWMutex

	channelMgr *ChannelManager
	ackTimeout time.Duration
}

// NewEscalationManager 创建升级管理器
func NewEscalationManager(channelMgr *ChannelManager) *EscalationManager {
	em := &EscalationManager{
		policies:   make(map[string]*EscalationPolicy),
		records:    make(map[string]*EscalationRecord),
		channelMgr: channelMgr,
		ackTimeout: 15 * time.Minute,
	}
	em.RegisterPolicy(DefaultEscalationPolicy())
	return em
}

// RegisterPolicy 注册升级策略
func (em *EscalationManager) RegisterPolicy(policy *EscalationPolicy) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.policies[policy.Name] = policy
}

// StartEscalation 开始升级流程
func (em *EscalationManager) StartEscalation(alert *Alert, policyName string) *EscalationRecord {
	em.mu.Lock()
	defer em.mu.Unlock()

	policy, ok := em.policies[policyName]
	if !ok {
		policy = em.policies["default"]
	}

	record := &EscalationRecord{
		AlertID:         alert.ID,
		Policy:          policy.Name,
		CurrentStep:     0,
		TotalSteps:      len(policy.Steps),
		StartedAt:       time.Now(),
		LastEscalatedAt: time.Now(),
	}
	em.records[alert.ID] = record

	return record
}

// Acknowledge 确认告警（停止升级）
func (em *EscalationManager) Acknowledge(alertID, user string) bool {
	em.mu.Lock()
	defer em.mu.Unlock()

	record, ok := em.records[alertID]
	if !ok {
		return false
	}

	now := time.Now()
	record.Acknowledged = true
	record.AcknowledgedAt = &now
	record.AcknowledgedBy = user
	return true
}

// IsAcknowledged 检查是否已确认
func (em *EscalationManager) IsAcknowledged(alertID string) bool {
	em.mu.RLock()
	defer em.mu.RUnlock()

	record, ok := em.records[alertID]
	if !ok {
		return false
	}
	return record.Acknowledged
}

// GetRecord 获取升级记录
func (em *EscalationManager) GetRecord(alertID string) *EscalationRecord {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.records[alertID]
}

// CheckAndEscalate 检查并执行升级
func (em *EscalationManager) CheckAndEscalate(alert *Alert) (*EscalationStep, bool) {
	em.mu.Lock()
	defer em.mu.Unlock()

	record, ok := em.records[alert.ID]
	if !ok {
		return nil, false
	}

	// 已确认，不升级
	if record.Acknowledged {
		return nil, false
	}

	policy := em.policies[record.Policy]
	if policy == nil {
		return nil, false
	}

	// 检查是否需要升级
	elapsed := time.Since(record.LastEscalatedAt)
	if record.CurrentStep < len(policy.Steps)-1 {
		nextStep := policy.Steps[record.CurrentStep+1]
		if elapsed >= nextStep.Delay {
			record.CurrentStep++
			record.LastEscalatedAt = time.Now()
			return &nextStep, true
		}
	} else if policy.Repeat {
		// 最后一步，检查是否循环
		if elapsed >= policy.RepeatInterval {
			record.LastEscalatedAt = time.Now()
			return &policy.Steps[len(policy.Steps)-1], true
		}
	}

	return nil, false
}

// Resolve 解决告警（结束升级流程）
func (em *EscalationManager) Resolve(alertID string) {
	em.mu.Lock()
	defer em.mu.Unlock()
	delete(em.records, alertID)
}

// GetActiveRecords 获取所有活跃升级记录
func (em *EscalationManager) GetActiveRecords() []*EscalationRecord {
	em.mu.RLock()
	defer em.mu.RUnlock()

	result := make([]*EscalationRecord, 0)
	for _, record := range em.records {
		if !record.Acknowledged {
			result = append(result, record)
		}
	}
	return result
}

// GetUnacknowledgedAlerts 获取未确认告警数量
func (em *EscalationManager) GetUnacknowledgedCount() int {
	em.mu.RLock()
	defer em.mu.RUnlock()

	count := 0
	for _, record := range em.records {
		if !record.Acknowledged {
			count++
		}
	}
	return count
}

// ============================================================================
// 三、升级执行循环
// ============================================================================

// EscalationLoop 升级检查循环
func (em *EscalationManager) EscalationLoop(ctx context.Context, alertProvider func() []*Alert) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			em.checkEscalations(ctx, alertProvider)
		case <-ctx.Done():
			return
		}
	}
}

func (em *EscalationManager) checkEscalations(ctx context.Context, alertProvider func() []*Alert) {
	alerts := alertProvider()
	for _, alert := range alerts {
		if alert.Resolved {
			em.Resolve(alert.ID)
			continue
		}

		step, escalated := em.CheckAndEscalate(alert)
		if escalated && step != nil && em.channelMgr != nil {
			// 发送升级通知
			tmpl := &AlertTemplate{
				Title:    fmt.Sprintf("【升级】告警: %s", alert.RuleName),
				Body:     fmt.Sprintf("告警已升级，当前级别: %s", step.Severity),
				Severity: step.Severity,
			}
			if err := em.channelMgr.SendTo(ctx, step.Channels, alert, tmpl); err != nil {
				// 记录升级发送失败
			}
		}
	}
}

// ============================================================================
// 四、按严重级别自动升级
// ============================================================================

// SeverityEscalationPolicy 严重级别升级策略
type SeverityEscalationPolicy struct {
	CriticalImmediate bool          `json:"critical_immediate"` // critical 是否立即升级
	WarningDelay      time.Duration `json:"warning_delay"`       // warning 升级延迟
	InfoDelay         time.Duration `json:"info_delay"`          // info 升级延迟
}

// DefaultSeverityEscalationPolicy 默认严重级别升级策略
func DefaultSeverityEscalationPolicy() *SeverityEscalationPolicy {
	return &SeverityEscalationPolicy{
		CriticalImmediate: true,
		WarningDelay:      15 * time.Minute,
		InfoDelay:         1 * time.Hour,
	}
}

// AutoEscalateBySeverity 根据严重级别自动选择升级策略
func AutoEscalateBySeverity(severity string, policy *SeverityEscalationPolicy) *EscalationPolicy {
	if policy == nil {
		policy = DefaultSeverityEscalationPolicy()
	}

	base := DefaultEscalationPolicy()

	switch severity {
	case "critical":
		if policy.CriticalImmediate {
			// critical 直接跳到第三步
			base.Steps = []EscalationStep{
				{Delay: 0, Channels: []string{"email", "dingtalk", "wechat"}, Severity: "critical", RequireAck: true},
				{Delay: 15 * time.Minute, Channels: []string{"email", "dingtalk", "wechat", "sms"}, Severity: "critical", RequireAck: false},
			}
		}
	case "warning":
		base.Steps[1].Delay = policy.WarningDelay
	case "info":
		base.Steps[1].Delay = policy.InfoDelay
	}

	return base
}

// ============================================================================
// 五、升级统计
// ============================================================================

// EscalationStats 升级统计
type EscalationStats struct {
	TotalEscalations   int            `json:"total_escalations"`
	AcknowledgedCount  int            `json:"acknowledged_count"`
	UnacknowledgedCount int           `json:"unacknowledged_count"`
	AverageAckTime     time.Duration  `json:"average_ack_time"`
	MaxAckTime         time.Duration  `json:"max_ack_time"`
	PolicyUsage        map[string]int `json:"policy_usage"`
}

// GetStats 获取升级统计
func (em *EscalationManager) GetStats() *EscalationStats {
	em.mu.RLock()
	defer em.mu.RUnlock()

	stats := &EscalationStats{
		PolicyUsage: make(map[string]int),
	}

	var totalAckTime time.Duration
	maxAckTime := time.Duration(0)

	for _, record := range em.records {
		stats.TotalEscalations++
		stats.PolicyUsage[record.Policy]++

		if record.Acknowledged && record.AcknowledgedAt != nil {
			stats.AcknowledgedCount++
			ackTime := record.AcknowledgedAt.Sub(record.StartedAt)
			totalAckTime += ackTime
			if ackTime > maxAckTime {
				maxAckTime = ackTime
			}
		} else {
			stats.UnacknowledgedCount++
		}
	}

	if stats.AcknowledgedCount > 0 {
		stats.AverageAckTime = totalAckTime / time.Duration(stats.AcknowledgedCount)
	}
	stats.MaxAckTime = maxAckTime

	return stats
}
