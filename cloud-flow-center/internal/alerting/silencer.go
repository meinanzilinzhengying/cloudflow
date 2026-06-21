// P2: 告警静默、抑制与维护窗口
//
// 功能：
//   - 静默规则：按标签匹配静默告警（静音发送）
//   - 维护窗口：指定时间段内静默所有告警
//   - 周期性静默：每天/每周的固定时间段
//   - 紧急覆盖：维护窗口内允许特定告警通过
//
package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 一、静默规则
// ============================================================================

// SilenceRule 静默规则
type SilenceRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Matchers    []*LabelMatcher   `json:"matchers"`      // 匹配的标签
	StartAt     time.Time         `json:"start_at"`      // 开始时间
	EndAt       time.Time         `json:"end_at"`        // 结束时间
	CreatedBy   string            `json:"created_by"`    // 创建人
	Comment     string            `json:"comment"`       // 备注
	Active      bool              `json:"active"`        // 是否启用
}

// IsActive 检查静默规则是否生效
func (sr *SilenceRule) IsActive() bool {
	if !sr.Active {
		return false
	}
	now := time.Now()
	return now.After(sr.StartAt) && now.Before(sr.EndAt)
}

// Match 检查告警是否匹配静默规则
func (sr *SilenceRule) Match(alert *Alert) bool {
	for _, matcher := range sr.Matchers {
		if !matcher.Match(alert.Labels) {
			return false
		}
	}
	return true
}

// RemainingDuration 获取剩余时间
func (sr *SilenceRule) RemainingDuration() time.Duration {
	if !sr.IsActive() {
		return 0
	}
	return sr.EndAt.Sub(time.Now())
}

// ============================================================================
// 二、静默管理器
// ============================================================================

// Silencer 静默管理器
type Silencer struct {
	rules  map[string]*SilenceRule
	mu     sync.RWMutex
}

// NewSilencer 创建静默管理器
func NewSilencer() *Silencer {
	return &Silencer{
		rules: make(map[string]*SilenceRule),
	}
}

// AddRule 添加静默规则
func (s *Silencer) AddRule(rule *SilenceRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules[rule.ID] = rule
}

// RemoveRule 移除静默规则
func (s *Silencer) RemoveRule(ruleID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rules, ruleID)
}

// GetRule 获取静默规则
func (s *Silencer) GetRule(ruleID string) *SilenceRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rules[ruleID]
}

// GetAllRules 获取所有规则
func (s *Silencer) GetAllRules() []*SilenceRule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SilenceRule, 0, len(s.rules))
	for _, rule := range s.rules {
		result = append(result, rule)
	}
	return result
}

// GetActiveRules 获取所有生效的规则
func (s *Silencer) GetActiveRules() []*SilenceRule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SilenceRule, 0)
	for _, rule := range s.rules {
		if rule.IsActive() {
			result = append(result, rule)
		}
	}
	return result
}

// IsSilenced 检查告警是否被静默
func (s *Silencer) IsSilenced(alert *Alert) (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rule := range s.rules {
		if rule.IsActive() && rule.Match(alert) {
			return true, rule.ID
		}
	}
	return false, ""
}

// CleanupExpired 清理过期的静默规则
func (s *Silencer) CleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	now := time.Now()
	for id, rule := range s.rules {
		if now.After(rule.EndAt) {
			delete(s.rules, id)
			removed++
		}
	}
	return removed
}

// ============================================================================
// 三、维护窗口
// ============================================================================

// MaintenanceWindow 维护窗口
type MaintenanceWindow struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	StartTime   time.Time     `json:"start_time"`    // 开始时间
	EndTime     time.Time     `json:"end_time"`      // 结束时间
	Timezone    string        `json:"timezone"`      // 时区
	Recurrence  RecurrenceType `json:"recurrence"`   // 重复类型
	CreatedBy   string        `json:"created_by"`
	Active      bool          `json:"active"`
}

// RecurrenceType 重复类型
type RecurrenceType string

const (
	RecurrenceNone   RecurrenceType = "none"    // 不重复
	RecurrenceDaily  RecurrenceType = "daily"   // 每天
	RecurrenceWeekly RecurrenceType = "weekly"  // 每周
	RecurrenceMonthly RecurrenceType = "monthly" // 每月
)

// IsActive 检查维护窗口是否生效
func (mw *MaintenanceWindow) IsActive() bool {
	if !mw.Active {
		return false
	}
	now := time.Now()
	return now.After(mw.StartTime) && now.Before(mw.EndTime)
}

// IsInWindow 检查当前时间是否在维护窗口内（支持周期性）
func (mw *MaintenanceWindow) IsInWindow(t time.Time) bool {
	if !mw.Active {
		return false
	}

	switch mw.Recurrence {
	case RecurrenceNone, "":
		return t.After(mw.StartTime) && t.Before(mw.EndTime)
	case RecurrenceDaily:
		return mw.isInDailyWindow(t)
	case RecurrenceWeekly:
		return mw.isInWeeklyWindow(t)
	case RecurrenceMonthly:
		return mw.isInMonthlyWindow(t)
	default:
		return false
	}
}

func (mw *MaintenanceWindow) isInDailyWindow(t time.Time) bool {
	startH, startM, _ := mw.StartTime.Clock()
	endH, endM, _ := mw.EndTime.Clock()

	nowH, nowM, _ := t.Clock()
	nowMinutes := nowH*60 + nowM
	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM

	if endMinutes >= startMinutes {
		return nowMinutes >= startMinutes && nowMinutes <= endMinutes
	}
	// 跨午夜
	return nowMinutes >= startMinutes || nowMinutes <= endMinutes
}

func (mw *MaintenanceWindow) isInWeeklyWindow(t time.Time) bool {
	startWeekday := mw.StartTime.Weekday()
	endWeekday := mw.EndTime.Weekday()
	nowWeekday := t.Weekday()

	if endWeekday >= startWeekday {
		return nowWeekday >= startWeekday && nowWeekday <= endWeekday && mw.isInDailyWindow(t)
	}
	return (nowWeekday >= startWeekday || nowWeekday <= endWeekday) && mw.isInDailyWindow(t)
}

func (mw *MaintenanceWindow) isInMonthlyWindow(t time.Time) bool {
	startDay := mw.StartTime.Day()
	endDay := mw.EndTime.Day()
	nowDay := t.Day()

	if endDay >= startDay {
		return nowDay >= startDay && nowDay <= endDay && mw.isInDailyWindow(t)
	}
	return (nowDay >= startDay || nowDay <= endDay) && mw.isInDailyWindow(t)
}

// ============================================================================
// 四、维护窗口管理器
// ============================================================================

// MaintenanceManager 维护窗口管理器
type MaintenanceManager struct {
	windows map[string]*MaintenanceWindow
	mu      sync.RWMutex
}

// NewMaintenanceManager 创建维护窗口管理器
func NewMaintenanceManager() *MaintenanceManager {
	return &MaintenanceManager{
		windows: make(map[string]*MaintenanceWindow),
	}
}

// AddWindow 添加维护窗口
func (mm *MaintenanceManager) AddWindow(window *MaintenanceWindow) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.windows[window.ID] = window
}

// RemoveWindow 移除维护窗口
func (mm *MaintenanceManager) RemoveWindow(windowID string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	delete(mm.windows, windowID)
}

// GetWindow 获取维护窗口
func (mm *MaintenanceManager) GetWindow(windowID string) *MaintenanceWindow {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.windows[windowID]
}

// GetAllWindows 获取所有维护窗口
func (mm *MaintenanceManager) GetAllWindows() []*MaintenanceWindow {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	result := make([]*MaintenanceWindow, 0, len(mm.windows))
	for _, w := range mm.windows {
		result = append(result, w)
	}
	return result
}

// IsInMaintenanceWindow 检查当前是否在维护窗口内
func (mm *MaintenanceManager) IsInMaintenanceWindow() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	now := time.Now()
	for _, window := range mm.windows {
		if window.IsInWindow(now) {
			return true
		}
	}
	return false
}

// GetActiveWindows 获取当前生效的维护窗口
func (mm *MaintenanceManager) GetActiveWindows() []*MaintenanceWindow {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	now := time.Now()
	result := make([]*MaintenanceWindow, 0)
	for _, window := range mm.windows {
		if window.IsInWindow(now) {
			result = append(result, window)
		}
	}
	return result
}

// CleanupExpired 清理过期的维护窗口（仅非周期性）
func (mm *MaintenanceManager) CleanupExpired() int {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	removed := 0
	now := time.Now()
	for id, window := range mm.windows {
		if window.Recurrence == RecurrenceNone && now.After(window.EndTime) {
			delete(mm.windows, id)
			removed++
		}
	}
	return removed
}

// ============================================================================
// 五、综合告警控制（Silencer + Maintenance + Inhibitor）
// ============================================================================

// AlertController 综合告警控制器
type AlertController struct {
	silencer    *Silencer
	maintenance *MaintenanceManager
	inhibitor   *Inhibitor
}

// NewAlertController 创建综合告警控制器
func NewAlertController() *AlertController {
	return &AlertController{
		silencer:    NewSilencer(),
		maintenance: NewMaintenanceManager(),
		inhibitor:   NewInhibitor(),
	}
}

// ShouldSuppress 检查告警是否应该被抑制/静默
// 返回: (是否抑制, 原因)
func (ac *AlertController) ShouldSuppress(alert *Alert) (bool, string) {
	// 1. 检查维护窗口
	if ac.maintenance.IsInMaintenanceWindow() {
		return true, "in_maintenance_window"
	}

	// 2. 检查静默规则
	if silenced, ruleID := ac.silencer.IsSilenced(alert); silenced {
		return true, fmt.Sprintf("silenced_by_rule_%s", ruleID)
	}

	// 3. 检查抑制规则
	if ac.inhibitor.Inhibit(alert) {
		return true, "inhibited"
	}

	return false, ""
}

// AddSilenceRule 添加静默规则
func (ac *AlertController) AddSilenceRule(rule *SilenceRule) {
	ac.silencer.AddRule(rule)
}

// RemoveSilenceRule 移除静默规则
func (ac *AlertController) RemoveSilenceRule(ruleID string) {
	ac.silencer.RemoveRule(ruleID)
}

// AddMaintenanceWindow 添加维护窗口
func (ac *AlertController) AddMaintenanceWindow(window *MaintenanceWindow) {
	ac.maintenance.AddWindow(window)
}

// RemoveMaintenanceWindow 移除维护窗口
func (ac *AlertController) RemoveMaintenanceWindow(windowID string) {
	ac.maintenance.RemoveWindow(windowID)
}

// AddInhibitRule 添加抑制规则
func (ac *AlertController) AddInhibitRule(rule *InhibitRule) {
	ac.inhibitor.AddRule(rule)
}

// ProcessSourceAlert 处理源告警（触发抑制）
func (ac *AlertController) ProcessSourceAlert(alert *Alert) {
	ac.inhibitor.ProcessSourceAlert(alert)
}

// ReleaseSourceAlert 释放源告警（解除抑制）
func (ac *AlertController) ReleaseSourceAlert(alert *Alert) {
	ac.inhibitor.ReleaseSourceAlert(alert)
}

// GetSilencer 获取静默管理器
func (ac *AlertController) GetSilencer() *Silencer {
	return ac.silencer
}

// GetMaintenanceManager 获取维护窗口管理器
func (ac *AlertController) GetMaintenanceManager() *MaintenanceManager {
	return ac.maintenance
}

// GetInhibitor 获取抑制器
func (ac *AlertController) GetInhibitor() *Inhibitor {
	return ac.inhibitor
}

// CleanupExpired 清理所有过期规则
func (ac *AlertController) CleanupExpired() (silenceRemoved, windowRemoved int) {
	return ac.silencer.CleanupExpired(), ac.maintenance.CleanupExpired()
}

// CleanupLoop 定期清理过期规则
func (ac *AlertController) CleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ac.CleanupExpired()
		case <-ctx.Done():
			return
		}
	}
}
