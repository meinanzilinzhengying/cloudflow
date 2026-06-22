// P6: 预测性分析引擎（容量预测 / 故障预测）
package prediction

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

// PredictionEngine 预测引擎
type PredictionEngine struct {
	mu sync.RWMutex

	// 时间序列数据
	timeSeries map[string]*TimeSeries

	// 预测模型参数
	models map[string]*PredictionModel

	// 可配置的告警阈值因子
	AlertThresholdFactor float64
}

// TimeSeries 时间序列数据
type TimeSeries struct {
	Key     string
	Points  []DataPoint
	MaxSize int
	Unit    string
	Label   string
}

// DataPoint 数据点
type DataPoint struct {
	Timestamp time.Time
	Value     float64
}

// PredictionModel 预测模型
type PredictionModel struct {
	Key         string
	Type        ModelType
	TrendSlope  float64
	Seasonality []float64
	LastUpdated time.Time
}

// ModelType 预测模型类型
type ModelType string

const (
	ModelLinear      ModelType = "linear"
	ModelExponential ModelType = "exponential"
	ModelSeasonal    ModelType = "seasonal"
)

// PredictionResult 预测结果
type PredictionResult struct {
	Key            string
	CurrentValue   float64
	PredictedValue float64
	Confidence     float64
	Horizon        time.Duration
	Trend          TrendDirection
	RiskLevel      RiskLevel
	Reasoning      string
	PredictedAt    time.Time
}

// TrendDirection 趋势方向
type TrendDirection string

const (
	TrendUp      TrendDirection = "up"
	TrendDown    TrendDirection = "down"
	TrendStable  TrendDirection = "stable"
	TrendUnknown TrendDirection = "unknown"
)

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// CapacityPrediction 容量预测
type CapacityPrediction struct {
	Resource        string
	CurrentUsage    float64
	CapacityLimit   float64
	GrowthRate      float64
	DaysUntilFull   float64
	DaysUntilAlert  float64
	RiskLevel       RiskLevel
	Recommendations []string
}

// FailurePrediction 故障预测
type FailurePrediction struct {
	Service         string
	FailureType     string
	Probability     float64
	ETA             time.Time
	Indicators      []RiskIndicator
	Confidence      float64
	Recommendations []string
}

// RiskIndicator 风险指标
type RiskIndicator struct {
	Metric    string
	Current   float64
	Threshold float64
	Deviation float64
	Severity  string
}

// NewPredictionEngine 创建预测引擎
func NewPredictionEngine() *PredictionEngine {
	return &PredictionEngine{
		timeSeries:           make(map[string]*TimeSeries),
		models:               make(map[string]*PredictionModel),
		AlertThresholdFactor: 0.8,
	}
}

// ============================================================================
// 数据录入
// ============================================================================

// Record 记录时间序列数据
func (e *PredictionEngine) Record(key string, timestamp time.Time, value float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ts, exists := e.timeSeries[key]
	if !exists {
		ts = &TimeSeries{Key: key, MaxSize: 1000}
		e.timeSeries[key] = ts
	}

	ts.Points = append(ts.Points, DataPoint{Timestamp: timestamp, Value: value})
	if ts.MaxSize > 0 && len(ts.Points) > ts.MaxSize {
		ts.Points = ts.Points[len(ts.Points)-ts.MaxSize:]
	}
}

// RecordBatch 批量记录
func (e *PredictionEngine) RecordBatch(key string, points []DataPoint) {
	for _, p := range points {
		e.Record(key, p.Timestamp, p.Value)
	}
}

// ============================================================================
// 线性预测
// ============================================================================

// Predict 执行预测
func (e *PredictionEngine) Predict(key string, horizon time.Duration) *PredictionResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ts, ok := e.timeSeries[key]
	if !ok || len(ts.Points) < 2 {
		return &PredictionResult{Key: key, Trend: TrendUnknown, Reasoning: "数据不足"}
	}

	points := ts.Points
	current := points[len(points)-1].Value

	// 线性回归
	slope, intercept := linearRegression(points)

	// 预测未来值
	elapsedHours := float64(horizon) / float64(time.Hour)

	predicted := intercept + slope*elapsedHours
	if predicted < 0 {
		predicted = 0
	}

	// 计算置信度
	confidence := calculateConfidence(points, slope, intercept)

	// 趋势方向
	trend := TrendStable
	if slope > 0.01*current {
		trend = TrendUp
	} else if slope < -0.01*current {
		trend = TrendDown
	}

	// 风险等级
	risk := RiskLow
	if current > 0 {
		growthRatio := predicted / current
		switch {
		case growthRatio > 2.0 || predicted > 90:
			risk = RiskCritical
		case growthRatio > 1.5 || predicted > 80:
			risk = RiskHigh
		case growthRatio > 1.2 || predicted > 70:
			risk = RiskMedium
		}
	}

	reasoning := fmt.Sprintf("基于 %d 个数据点线性回归，斜率=%.4f", len(points), slope)
	if trend == TrendUp {
		reasoning += "，呈上升趋势"
	} else if trend == TrendDown {
		reasoning += "，呈下降趋势"
	}

	return &PredictionResult{
		Key:            key,
		CurrentValue:   current,
		PredictedValue: predicted,
		Confidence:     confidence,
		Horizon:        horizon,
		Trend:          trend,
		RiskLevel:      risk,
		Reasoning:      reasoning,
		PredictedAt:    time.Now(),
	}
}

// linearRegression 执行最小二乘线性回归
func linearRegression(points []DataPoint) (slope, intercept float64) {
	n := float64(len(points))
	if n < 2 {
		return 0, 0
	}

	var sumX, sumY, sumXY, sumX2 float64

	// 以第一个点时间为基准
	baseTime := points[0].Timestamp

	for _, p := range points {
		x := float64(p.Timestamp.Sub(baseTime)) / float64(time.Hour) // hours
		y := p.Value
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// 最小二乘法
	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0, sumY / n
	}

	slope = (n*sumXY - sumX*sumY) / denominator
	intercept = (sumY - slope*sumX) / n

	return slope, intercept
}

// calculateConfidence 计算预测置信度
func calculateConfidence(points []DataPoint, slope, intercept float64) float64 {
	if len(points) < 2 {
		return 0
	}

	baseTime := points[0].Timestamp
	var sumSquaredError, sumSquaredTotal float64
	var meanY float64

	for _, p := range points {
		meanY += p.Value
	}
	meanY /= float64(len(points))

	for _, p := range points {
		x := float64(p.Timestamp.Sub(baseTime)) / float64(time.Hour)
		predicted := intercept + slope*x
		actual := p.Value
		sumSquaredError += (actual - predicted) * (actual - predicted)
		sumSquaredTotal += (actual - meanY) * (actual - meanY)
	}

	if sumSquaredTotal == 0 {
		return 1.0
	}

	// R² = 1 - SSE/SST
	r2 := 1 - sumSquaredError/sumSquaredTotal
	if r2 < 0 {
		r2 = 0
	}
	if r2 > 1 {
		r2 = 1
	}

	// 数据点越多置信度越高
	dataFactor := math.Min(1.0, float64(len(points))/100.0)

	return (r2 + dataFactor) / 2.0
}

// ============================================================================
// 容量预测
// ============================================================================

// PredictCapacity 容量预测
func (e *PredictionEngine) PredictCapacity(resource string, capacityLimit float64) *CapacityPrediction {
	result := &CapacityPrediction{
		Resource:        resource,
		CapacityLimit:   capacityLimit,
		RiskLevel:       RiskLow,
		Recommendations: []string{},
	}

	// 获取当前使用率
	currentUsage := e.getCurrentValue(resource)
	result.CurrentUsage = currentUsage

	if currentUsage <= 0 || capacityLimit <= 0 {
		result.Recommendations = append(result.Recommendations, "数据不足，无法预测容量")
		return result
	}

	// 计算增长率
	growthRate := e.calculateGrowthRate(resource)
	result.GrowthRate = growthRate

	// 预测满容时间
	if growthRate > 0 && currentUsage < capacityLimit {
		diff := capacityLimit - currentUsage
		result.DaysUntilFull = diff / (growthRate * 24) // growthRate 是每小时增长
		if result.DaysUntilFull < 0 {
			result.DaysUntilFull = 0
		}
	}

	// 可配置的告警阈值
	factor := e.AlertThresholdFactor
	if factor <= 0 || factor > 1.0 {
		factor = 0.8
	}
	alertThreshold := capacityLimit * factor
	if growthRate > 0 && currentUsage < alertThreshold {
		diff := alertThreshold - currentUsage
		result.DaysUntilAlert = diff / (growthRate * 24)
		if result.DaysUntilAlert < 0 {
			result.DaysUntilAlert = 0
		}
	}

	// 风险等级
	usageRatio := currentUsage / capacityLimit
	switch {
	case usageRatio > 0.95 || result.DaysUntilFull < 1:
		result.RiskLevel = RiskCritical
	case usageRatio > 0.85 || result.DaysUntilFull < 7:
		result.RiskLevel = RiskHigh
	case usageRatio > 0.70 || result.DaysUntilFull < 30:
		result.RiskLevel = RiskMedium
	default:
		result.RiskLevel = RiskLow
	}

	// 建议
	result.Recommendations = e.generateCapacityRecommendations(result)

	return result
}

func (e *PredictionEngine) getCurrentValue(key string) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if ts, ok := e.timeSeries[key]; ok && len(ts.Points) > 0 {
		return ts.Points[len(ts.Points)-1].Value
	}
	return 0
}

func (e *PredictionEngine) calculateGrowthRate(key string) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ts, ok := e.timeSeries[key]
	if !ok || len(ts.Points) < 2 {
		return 0
	}

	// 计算最近24小时的平均增长率
	now := ts.Points[len(ts.Points)-1].Timestamp
	cutoff := now.Add(-24 * time.Hour)

	var recentPoints []DataPoint
	for _, p := range ts.Points {
		if p.Timestamp.After(cutoff) {
			recentPoints = append(recentPoints, p)
		}
	}

	if len(recentPoints) < 2 {
		return 0
	}

	slope, _ := linearRegression(recentPoints)
	if slope < 0 {
		return 0
	}
	return slope
}

func (e *PredictionEngine) generateCapacityRecommendations(pred *CapacityPrediction) []string {
	var recs []string

	if pred.CurrentUsage/pred.CapacityLimit > 0.8 {
		recs = append(recs, fmt.Sprintf("当前 %s 使用率已达 %.1f%%，建议立即扩容", pred.Resource, pred.CurrentUsage))
	}

	if pred.DaysUntilFull > 0 && pred.DaysUntilFull < 7 {
		recs = append(recs, fmt.Sprintf("%s 预计 %.1f 天后满载，请紧急安排扩容", pred.Resource, pred.DaysUntilFull))
	} else if pred.DaysUntilFull > 0 && pred.DaysUntilFull < 30 {
		recs = append(recs, fmt.Sprintf("%s 预计 %.1f 天后满载，请提前规划扩容", pred.Resource, pred.DaysUntilFull))
	}

	if pred.GrowthRate > 0 {
		recs = append(recs, fmt.Sprintf("%s 每小时增长 %.2f%%，请评估业务增长是否预期内", pred.Resource, pred.GrowthRate))
	}

	if len(recs) == 0 {
		recs = append(recs, fmt.Sprintf("%s 容量充足，当前使用率 %.1f%%", pred.Resource, pred.CurrentUsage))
	}

	return recs
}

// ============================================================================
// 故障预测
// ============================================================================

// PredictFailure 故障预测
func (e *PredictionEngine) PredictFailure(service string, indicators []RiskIndicator) *FailurePrediction {
	result := &FailurePrediction{
		Service:         service,
		FailureType:     "unknown",
		Probability:     0,
		Indicators:      indicators,
		Recommendations: []string{},
	}

	if len(indicators) == 0 {
		result.Probability = 0
		result.Recommendations = append(result.Recommendations, "缺乏指标数据，无法评估故障风险")
		return result
	}

	// 计算故障概率
	totalRisk := 0.0
	var riskIndicators []RiskIndicator

	for _, ind := range indicators {
		if ind.Threshold > 0 {
			deviation := ind.Current / ind.Threshold
			ind.Deviation = deviation

			if deviation > 1.0 {
				totalRisk += math.Min(1.0, (deviation-1.0)*2)
				ind.Severity = "critical"
			} else if deviation > 0.8 {
				totalRisk += (deviation - 0.8) * 2
				ind.Severity = "warning"
			} else {
				ind.Severity = "normal"
			}
			riskIndicators = append(riskIndicators, ind)
		}
	}

	result.Indicators = riskIndicators

	// 综合概率 = 加权平均风险
	if len(indicators) > 0 {
		result.Probability = math.Min(1.0, totalRisk/float64(len(indicators)))
	}

	// 判断故障类型
	result.FailureType = e.classifyFailureType(indicators)

	// 估算 ETA (基于趋势)
	result.ETA = e.estimateFailureETA(service, result.Probability)

	// 置信度
	result.Confidence = math.Min(1.0, float64(len(indicators))/5.0) * 0.8

	// 建议
	result.Recommendations = e.generateFailureRecommendations(result)

	return result
}

func (e *PredictionEngine) classifyFailureType(indicators []RiskIndicator) string {
	hasCPU := false
	hasMem := false
	hasDisk := false
	hasError := false
	hasLatency := false

	for _, ind := range indicators {
		metric := ind.Metric
		switch {
		case metric == "cpu" || metric == "cpu_usage":
			hasCPU = true
		case metric == "memory" || metric == "memory_usage":
			hasMem = true
		case metric == "disk" || metric == "disk_usage":
			hasDisk = true
		case metric == "error_rate":
			hasError = true
		case metric == "latency" || metric == "response_time":
			hasLatency = true
		}
	}

	switch {
	case hasCPU && hasMem && hasDisk:
		return "resource_exhaustion"
	case hasError && hasLatency:
		return "cascade_failure"
	case hasDisk:
		return "disk_failure"
	case hasMem:
		return "oom_risk"
	case hasError:
		return "service_error"
	case hasLatency:
		return "performance_degradation"
	default:
		return "unknown"
	}
}

func (e *PredictionEngine) estimateFailureETA(service string, probability float64) time.Time {
	now := time.Now()

	if probability < 0.3 {
		return now.Add(7 * 24 * time.Hour)
	} else if probability < 0.6 {
		return now.Add(24 * time.Hour)
	} else if probability < 0.8 {
		return now.Add(4 * time.Hour)
	}
	return now.Add(30 * time.Minute)
}

func (e *PredictionEngine) generateFailureRecommendations(pred *FailurePrediction) []string {
	var recs []string

	if pred.Probability > 0.7 {
		recs = append(recs, fmt.Sprintf("【紧急】%s 故障概率 %.0f%%，建议立即介入", pred.Service, pred.Probability*100))
	} else if pred.Probability > 0.4 {
		recs = append(recs, fmt.Sprintf("【预警】%s 故障概率 %.0f%%，建议加强监控", pred.Service, pred.Probability*100))
	}

	switch pred.FailureType {
	case "resource_exhaustion":
		recs = append(recs, "资源即将耗尽，建议立即扩容或优化资源使用")
	case "oom_risk":
		recs = append(recs, "内存风险高，建议检查内存泄漏并增加内存限制")
	case "cascade_failure":
		recs = append(recs, "存在级联故障风险，建议检查上游依赖健康状态")
	case "performance_degradation":
		recs = append(recs, "性能持续下降，建议分析瓶颈并优化")
	case "disk_failure":
		recs = append(recs, "磁盘空间不足，建议清理日志或扩容")
	case "service_error":
		recs = append(recs, "错误率升高，建议检查最近代码变更")
	}

	for _, ind := range pred.Indicators {
		if ind.Severity == "critical" {
			recs = append(recs, fmt.Sprintf("指标 %s 超出阈值 %.1f%%，当前 %.2f，建议立即处理", ind.Metric, ind.Threshold, ind.Current))
		}
	}

	return recs
}

// ============================================================================
// 批量预测
// ============================================================================

// PredictAll 对所有时间序列执行预测
func (e *PredictionEngine) PredictAll(horizon time.Duration) []*PredictionResult {
	e.mu.RLock()
	keys := make([]string, 0, len(e.timeSeries))
	for k := range e.timeSeries {
		keys = append(keys, k)
	}
	e.mu.RUnlock()

	sort.Strings(keys)

	var results []*PredictionResult
	for _, key := range keys {
		results = append(results, e.Predict(key, horizon))
	}
	return results
}

// GetTimeSeriesKeys 获取所有时间序列键
func (e *PredictionEngine) GetTimeSeriesKeys() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	keys := make([]string, 0, len(e.timeSeries))
	for k := range e.timeSeries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// GetTimeSeries 获取时间序列数据
func (e *PredictionEngine) GetTimeSeries(key string) *TimeSeries {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.timeSeries[key]
}
