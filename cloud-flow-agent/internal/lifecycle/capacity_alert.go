// P7: 存储容量预警引擎
package lifecycle

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// ============================================================================
// 核心类型定义
// ============================================================================

// StorageType 存储类型
type StorageType string

const (
	StorageLocal    StorageType = "local"     // 本地磁盘
	StorageSSD      StorageType = "ssd"       // SSD
	StorageHDD      StorageType = "hdd"       // HDD
	StorageObject   StorageType = "object"    // 对象存储
	StorageMemory   StorageType = "memory"    // 内存
)

// CapacityAlert 容量预警
type CapacityAlert struct {
	ID          string      `json:"id"`
	StorageType StorageType `json:"storage_type"`
	Category    DataCategory `json:"category,omitempty"`
	Level       AlertLevel  `json:"level"`
	Message     string      `json:"message"`
	CurrentGB   float64     `json:"current_gb"`
	LimitGB     float64     `json:"limit_gb"`
	UsagePct    float64     `json:"usage_pct"`
	Timestamp   time.Time   `json:"timestamp"`
	Resolved    bool        `json:"resolved"`
	Recommendations []string `json:"recommendations"`
}

// AlertLevel 预警级别
type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "info"      // 信息提示(>60%)
	AlertLevelWarning  AlertLevel = "warning"   // 警告(>75%)
	AlertLevelCritical AlertLevel = "critical"  // 紧急(>85%)
	AlertLevelEmergency AlertLevel = "emergency" // 紧急(>95%)
)

// CapacityThreshold 容量阈值配置
type CapacityThreshold struct {
	StorageType    StorageType `json:"storage_type"`
	Category       DataCategory `json:"category,omitempty"`
	
	// 多级阈值
	InfoPct        float64 `json:"info_pct"`       // 信息阈值(默认60%)
	WarningPct     float64 `json:"warning_pct"`    // 警告阈值(默认75%)
	CriticalPct    float64 `json:"critical_pct"`   // 紧急阈值(默认85%)
	EmergencyPct   float64 `json:"emergency_pct"`  // 灾难阈值(默认95%)
	
	// 增长预警
	GrowthThresholdGB float64 `json:"growth_threshold_gb"` // 日增长阈值GB
	GrowthThresholdPct float64 `json:"growth_threshold_pct"` // 日增长百分比阈值
	
	// 容量限制
	MaxCapacityGB  float64 `json:"max_capacity_gb"`  // 最大容量(GB)
	
	// 通知配置
	NotifyChannels []string `json:"notify_channels"` // 通知渠道
}

// DefaultCapacityThreshold 返回默认容量阈值
func DefaultCapacityThreshold(st StorageType) *CapacityThreshold {
	return &CapacityThreshold{
		StorageType:       st,
		InfoPct:           60,
		WarningPct:        75,
		CriticalPct:         85,
		EmergencyPct:        95,
		GrowthThresholdGB:   10,
		GrowthThresholdPct:  10,
		NotifyChannels:      []string{"log", "webhook"},
	}
}

// Validate 验证阈值配置
func (t *CapacityThreshold) Validate() error {
	if t.InfoPct <= 0 || t.InfoPct > 100 {
		t.InfoPct = 60
	}
	if t.WarningPct <= t.InfoPct {
		t.WarningPct = math.Min(100, t.InfoPct+15)
	}
	if t.CriticalPct <= t.WarningPct {
		t.CriticalPct = math.Min(100, t.WarningPct+10)
	}
	if t.EmergencyPct <= t.CriticalPct {
		t.EmergencyPct = math.Min(100, t.CriticalPct+10)
	}
	return nil
}

// GetLevel 根据使用率获取预警级别
func (t *CapacityThreshold) GetLevel(usagePct float64) AlertLevel {
	switch {
	case usagePct >= t.EmergencyPct:
		return AlertLevelEmergency
	case usagePct >= t.CriticalPct:
		return AlertLevelCritical
	case usagePct >= t.WarningPct:
		return AlertLevelWarning
	case usagePct >= t.InfoPct:
		return AlertLevelInfo
	default:
		return "" // 无预警
	}
}

// ============================================================================
// 容量监控器
// ============================================================================

// StorageUsage 存储使用情况
type StorageUsage struct {
	StorageType    StorageType
	Category       DataCategory
	TotalGB        float64
	UsedGB         float64
	FreeGB         float64
	UsagePct       float64
	RecordCount    int64
	OldestTime     time.Time
	NewestTime     time.Time
	Timestamp      time.Time
	
	// 历史趋势
	HistorySamples []UsageSample
}

// UsageSample 使用样本
type UsageSample struct {
	Timestamp time.Time
	UsedGB    float64
	UsagePct  float64
}

// CapacityMonitor 容量监控器
type CapacityMonitor struct {
	mu         sync.RWMutex
	thresholds map[string]*CapacityThreshold // key: storageType:category
	usage      map[string]*StorageUsage      // key: storageType:category
	alerts     []*CapacityAlert
	history    map[string][]*CapacityAlert
	
	// 告警回调
	onAlert func(alert *CapacityAlert)
	
	// 采样窗口
	sampleWindow time.Duration
	maxSamples   int
}

// NewCapacityMonitor 创建容量监控器
func NewCapacityMonitor() *CapacityMonitor {
	return &CapacityMonitor{
		thresholds:   make(map[string]*CapacityThreshold),
		usage:        make(map[string]*StorageUsage),
		alerts:       []*CapacityAlert{},
		history:      make(map[string][]*CapacityAlert),
		sampleWindow: 7 * 24 * time.Hour,
		maxSamples:   168, // 7天 * 24小时
	}
}

// RegisterThreshold 注册容量阈值
func (cm *CapacityMonitor) RegisterThreshold(threshold *CapacityThreshold) error {
	if err := threshold.Validate(); err != nil {
		return err
	}
	
	cm.mu.Lock()
	defer cm.mu.Unlock()
	key := cm.makeKey(threshold.StorageType, threshold.Category)
	cm.thresholds[key] = threshold
	return nil
}

// RecordUsage 记录存储使用情况
func (cm *CapacityMonitor) RecordUsage(usage *StorageUsage) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	key := cm.makeKey(usage.StorageType, usage.Category)
	
	// 保留历史样本
	oldUsage, ok := cm.usage[key]
	if ok && oldUsage != nil {
		usage.HistorySamples = append(oldUsage.HistorySamples, UsageSample{
			Timestamp: time.Now(),
			UsedGB:    oldUsage.UsedGB,
			UsagePct:  oldUsage.UsagePct,
		})
		if len(usage.HistorySamples) > cm.maxSamples {
			usage.HistorySamples = usage.HistorySamples[len(usage.HistorySamples)-cm.maxSamples:]
		}
	}
	
	cm.usage[key] = usage
}

// CheckAlerts 检查容量预警
func (cm *CapacityMonitor) CheckAlerts() []*CapacityAlert {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	var newAlerts []*CapacityAlert
	now := time.Now()
	
	for key, usage := range cm.usage {
		threshold, ok := cm.thresholds[key]
		if !ok {
			threshold = cm.thresholds[string(usage.StorageType)+":"]
			if threshold == nil {
				threshold = DefaultCapacityThreshold(usage.StorageType)
			}
		}
		
		level := threshold.GetLevel(usage.UsagePct)
		if level == "" {
			// 检查已解决
			cm.resolveAlerts(key, usage.UsagePct)
			continue
		}
		
		// 检查是否已存在同级别的活跃告警
		if cm.hasActiveAlert(key, level) {
			continue
		}
		
		alert := &CapacityAlert{
			ID:          fmt.Sprintf("alert-%d", now.UnixNano()),
			StorageType: usage.StorageType,
			Category:    usage.Category,
			Level:       level,
			Message:     cm.generateAlertMessage(usage, level),
			CurrentGB:   usage.UsedGB,
			LimitGB:     usage.TotalGB,
			UsagePct:    usage.UsagePct,
			Timestamp:   now,
			Recommendations: cm.generateRecommendations(usage, level),
		}
		
		cm.alerts = append(cm.alerts, alert)
		cm.history[key] = append(cm.history[key], alert)
		newAlerts = append(newAlerts, alert)
		
		if cm.onAlert != nil {
			cm.onAlert(alert)
		}
	}
	
	// 检查增长预警
	growthAlerts := cm.checkGrowthAlerts()
	newAlerts = append(newAlerts, growthAlerts...)
	
	return newAlerts
}

func (cm *CapacityMonitor) resolveAlerts(key string, usagePct float64) {
	for _, alert := range cm.alerts {
		if !alert.Resolved && cm.makeKey(alert.StorageType, alert.Category) == key {
			// 如果当前使用率低于该告警级别阈值 - 10%
			threshold := cm.thresholds[key]
			if threshold == nil {
				continue
			}
			var thresholdPct float64
			switch alert.Level {
			case AlertLevelInfo:
				thresholdPct = threshold.InfoPct
			case AlertLevelWarning:
				thresholdPct = threshold.WarningPct
			case AlertLevelCritical:
				thresholdPct = threshold.CriticalPct
			case AlertLevelEmergency:
				thresholdPct = threshold.EmergencyPct
			}
			if usagePct < thresholdPct-10 {
				alert.Resolved = true
			}
		}
	}
}

func (cm *CapacityMonitor) hasActiveAlert(key string, level AlertLevel) bool {
	for _, alert := range cm.alerts {
		if !alert.Resolved && cm.makeKey(alert.StorageType, alert.Category) == key {
			if alert.Level == level {
				return true
			}
			// 已有更高级别的告警
			if cm.alertLevelWeight(alert.Level) >= cm.alertLevelWeight(level) {
				return true
			}
		}
	}
	return false
}

func (cm *CapacityMonitor) alertLevelWeight(level AlertLevel) int {
	switch level {
	case AlertLevelEmergency:
		return 4
	case AlertLevelCritical:
		return 3
	case AlertLevelWarning:
		return 2
	case AlertLevelInfo:
		return 1
	default:
		return 0
	}
}

func (cm *CapacityMonitor) checkGrowthAlerts() []*CapacityAlert {
	var alerts []*CapacityAlert
	now := time.Now()
	
	for key, usage := range cm.usage {
		if len(usage.HistorySamples) < 2 {
			continue
		}
		
		threshold := cm.thresholds[key]
		if threshold == nil {
			continue
		}
		
		// 计算24小时增长
		var dayAgoUsage *UsageSample
		for i := len(usage.HistorySamples) - 1; i >= 0; i-- {
			if now.Sub(usage.HistorySamples[i].Timestamp) >= 24*time.Hour {
				dayAgoUsage = &usage.HistorySamples[i]
				break
			}
		}
		if dayAgoUsage == nil {
			continue
		}
		
		growthGB := usage.UsedGB - dayAgoUsage.UsedGB
		growthPct := 0.0
		if dayAgoUsage.UsedGB > 0 {
			growthPct = (growthGB / dayAgoUsage.UsedGB) * 100
		}
		
		if growthGB > threshold.GrowthThresholdGB || growthPct > threshold.GrowthThresholdPct {
			alert := &CapacityAlert{
				ID:          fmt.Sprintf("growth-%d", now.UnixNano()),
				StorageType: usage.StorageType,
				Category:    usage.Category,
				Level:       AlertLevelWarning,
				Message:     fmt.Sprintf("存储 %s 24小时增长 %.1fGB (%.1f%%)", key, growthGB, growthPct),
				CurrentGB:   usage.UsedGB,
				LimitGB:     usage.TotalGB,
				UsagePct:    usage.UsagePct,
				Timestamp:   now,
				Recommendations: []string{
					fmt.Sprintf("检查 %s 数据增长来源", key),
					"考虑调整保留策略或增加存储容量",
				},
			}
			alerts = append(alerts, alert)
			cm.alerts = append(cm.alerts, alert)
		}
	}
	
	return alerts
}

func (cm *CapacityMonitor) generateAlertMessage(usage *StorageUsage, level AlertLevel) string {
	return fmt.Sprintf("[%s] %s 存储使用率 %.1f%% (%.1fGB / %.1fGB)",
		level, cm.makeKey(usage.StorageType, usage.Category), usage.UsagePct, usage.UsedGB, usage.TotalGB)
}

func (cm *CapacityMonitor) generateRecommendations(usage *StorageUsage, level AlertLevel) []string {
	var recs []string
	key := cm.makeKey(usage.StorageType, usage.Category)
	
	switch level {
	case AlertLevelInfo:
		recs = append(recs, "监控存储使用趋势")
	case AlertLevelWarning:
		recs = append(recs, "建议启用数据压缩")
		recs = append(recs, "检查是否有数据冗余可以清理")
	case AlertLevelCritical:
		recs = append(recs, "紧急：建议立即清理过期数据")
		recs = append(recs, "考虑将数据迁移到冷存储")
		recs = append(recs, "扩容存储或调整保留策略")
	case AlertLevelEmergency:
		recs = append(recs, "【紧急】立即执行数据清理")
		recs = append(recs, "暂停非必要数据写入")
		recs = append(recs, "立即扩容或归档历史数据")
	}
	
	recs = append(recs, fmt.Sprintf("当前 %s 数据量 %.1fGB, 记录数 %d", key, usage.UsedGB, usage.RecordCount))
	return recs
}

// GetActiveAlerts 获取活跃告警
func (cm *CapacityMonitor) GetActiveAlerts() []*CapacityAlert {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	var active []*CapacityAlert
	for _, alert := range cm.alerts {
		if !alert.Resolved {
			active = append(active, alert)
		}
	}
	
	sort.Slice(active, func(i, j int) bool {
		wi := cm.alertLevelWeight(active[i].Level)
		wj := cm.alertLevelWeight(active[j].Level)
		if wi != wj {
			return wi > wj
		}
		return active[i].Timestamp.After(active[j].Timestamp)
	})
	
	return active
}

// GetAlertHistory 获取告警历史
func (cm *CapacityMonitor) GetAlertHistory(st StorageType, category DataCategory) []*CapacityAlert {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	key := cm.makeKey(st, category)
	return cm.history[key]
}

// GetUsage 获取使用情况
func (cm *CapacityMonitor) GetUsage(st StorageType, category DataCategory) *StorageUsage {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	key := cm.makeKey(st, category)
	return cm.usage[key]
}

// GetAllUsage 获取所有使用情况
func (cm *CapacityMonitor) GetAllUsage() []*StorageUsage {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	var usages []*StorageUsage
	for _, u := range cm.usage {
		usages = append(usages, u)
	}
	
	sort.Slice(usages, func(i, j int) bool {
		if usages[i].StorageType != usages[j].StorageType {
			return usages[i].StorageType < usages[j].StorageType
		}
		return usages[i].Category < usages[j].Category
	})
	
	return usages
}

// PredictFullTime 预测存储满载时间
func (cm *CapacityMonitor) PredictFullTime(st StorageType, category DataCategory) (time.Time, float64) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	key := cm.makeKey(st, category)
	usage, ok := cm.usage[key]
	if !ok || len(usage.HistorySamples) < 2 {
		return time.Time{}, -1
	}
	
	// 线性回归预测
	samples := usage.HistorySamples
	n := float64(len(samples))
	var sumX, sumY, sumXY, sumX2 float64
	baseTime := samples[0].Timestamp
	
	for _, s := range samples {
		x := float64(s.Timestamp.Sub(baseTime)) / float64(time.Hour)
		y := s.UsedGB
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	
	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return time.Time{}, -1
	}
	
	slope := (n*sumXY - sumX*sumY) / denominator
	intercept := (sumY - slope*sumX) / n
	
	if slope <= 0 {
		return time.Time{}, -1
	}
	
	// 预测达到 totalGB 的时间
	remainingGB := usage.TotalGB - usage.UsedGB
	hoursToFull := remainingGB / slope
	fullTime := time.Now().Add(time.Duration(hoursToFull) * time.Hour)
	confidence := calculateR2(samples, slope, intercept, baseTime)
	
	return fullTime, confidence
}

func calculateR2(samples []UsageSample, slope, intercept float64, baseTime time.Time) float64 {
	if len(samples) == 0 {
		return 0
	}
	
	var sumY, sumSquaredError, sumSquaredTotal float64
	for _, s := range samples {
		sumY += s.UsedGB
	}
	meanY := sumY / float64(len(samples))
	
	for _, s := range samples {
		x := float64(s.Timestamp.Sub(baseTime)) / float64(time.Hour)
		predicted := intercept + slope*x
		actual := s.UsedGB
		sumSquaredError += (actual - predicted) * (actual - predicted)
		sumSquaredTotal += (actual - meanY) * (actual - meanY)
	}
	
	if sumSquaredTotal == 0 {
		return 1.0
	}
	
	r2 := 1 - sumSquaredError/sumSquaredTotal
	if r2 < 0 {
		r2 = 0
	}
	if r2 > 1 {
		r2 = 1
	}
	return r2
}

// SetOnAlert 设置告警回调
func (cm *CapacityMonitor) SetOnAlert(callback func(alert *CapacityAlert)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.onAlert = callback
}

func (cm *CapacityMonitor) makeKey(st StorageType, category DataCategory) string {
	if category != "" {
		return fmt.Sprintf("%s:%s", st, category)
	}
	return string(st) + ":"
}

// ============================================================================
// 容量报告
// ============================================================================

// CapacityReport 容量报告
type CapacityReport struct {
	GeneratedAt    time.Time
	TotalUsage     []*StorageUsage
	ActiveAlerts   []*CapacityAlert
	Predictions    map[string]time.Time
	Confidences    map[string]float64
	Recommendations []string
}

// GenerateCapacityReport 生成容量报告
func (cm *CapacityMonitor) GenerateCapacityReport() *CapacityReport {
	report := &CapacityReport{
		GeneratedAt:     time.Now(),
		TotalUsage:      cm.GetAllUsage(),
		ActiveAlerts:    cm.GetActiveAlerts(),
		Predictions:     make(map[string]time.Time),
		Confidences:     make(map[string]float64),
		Recommendations: []string{},
	}
	
	for _, usage := range report.TotalUsage {
		key := cm.makeKey(usage.StorageType, usage.Category)
		fullTime, confidence := cm.PredictFullTime(usage.StorageType, usage.Category)
		if !fullTime.IsZero() {
			report.Predictions[key] = fullTime
			report.Confidences[key] = confidence
		}
		
		if confidence > 0.5 && !fullTime.IsZero() {
			daysToFull := fullTime.Sub(time.Now()).Hours() / 24
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("%s 预计 %.1f 天后满载 (置信度 %.0f%%)", key, daysToFull, confidence*100))
		}
	}
	
	return report
}
