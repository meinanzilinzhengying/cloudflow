package metrics

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// ApplicationMetrics 应用指标数据
type ApplicationMetrics struct {
	Timestamp    time.Time `json:"timestamp"`
	ServiceName  string    `json:"service_name"`
	RequestCount uint64    `json:"request_count"`   // 请求数
	SuccessCount uint64    `json:"success_count"`   // 成功数
	ErrorCount   uint64    `json:"error_count"`     // 错误数
	LatencySum   float64   `json:"latency_sum"`     // 时延总和(ms)
	LatencyCount uint64    `json:"latency_count"`   // 时延统计次数
	LatencyMax   float64   `json:"latency_max_ms"`  // 最大时延(ms)
	LatencyMin   float64   `json:"latency_min_ms"`  // 最小时延(ms)
}

// RequestMetrics 请求指标
type RequestMetrics struct {
	Timestamps   []time.Time `json:"timestamps"`
	RequestCount []uint64    `json:"request_count"`   // 请求数
	QPS          []float64   `json:"qps"`             // 每秒查询率
	Throughput   []float64   `json:"throughput"`      // 吞吐量(req/s)
}

// LatencyMetrics 时延指标
type LatencyMetrics struct {
	Timestamps []time.Time `json:"timestamps"`
	AvgLatency []float64   `json:"avg_latency"`     // 平均时延
	P50Latency []float64   `json:"p50_latency"`     // P50时延
	P95Latency []float64   `json:"p95_latency"`     // P95时延
	P99Latency []float64   `json:"p99_latency"`     // P99时延
}

// ErrorMetrics 异常指标
type ErrorMetrics struct {
	Timestamps   []time.Time       `json:"timestamps"`
	ErrorCount   []uint64          `json:"error_count"`   // 错误数
	ErrorRate    []float64         `json:"error_rate"`    // 错误率(%)
	ErrorTypes   map[string]uint64 `json:"error_types"`   // 按类型分布
	StatusCodes  map[int]uint64    `json:"status_codes"`  // 按状态码分布
}

// MoMComparison 环比比较
type MoMComparison struct {
	CurrentValue  float64 `json:"current_value"`
	PreviousValue float64 `json:"previous_value"`
	Change        float64 `json:"change"`          // 变化量
	ChangePercent float64 `json:"change_percent"`  // 变化百分比
	Trend         string  `json:"trend"`           // up/down/stable
}

// ApplicationSummary 应用指标汇总
type ApplicationSummary struct {
	ServiceName   string                 `json:"service_name"`
	RequestMoM    MoMComparison          `json:"request_mom"`      // 请求数环比
	LatencyMoM    MoMComparison          `json:"latency_mom"`      // 时延环比
	ErrorMoM      MoMComparison          `json:"error_mom"`        // 异常环比
	SuccessRate   float64                `json:"success_rate"`     // 成功率
	CurrentQPS    float64                `json:"current_qps"`      // 当前QPS
	AvgLatency    float64                `json:"avg_latency_ms"`   // 平均时延
	ErrorRate     float64                `json:"error_rate"`       // 错误率
	LastUpdated   time.Time              `json:"last_updated"`
}

// ApplicationCollector 应用指标采集器
type ApplicationCollector struct {
	mu       sync.RWMutex
	metrics  map[string][]ApplicationMetrics // 按服务名分组
	maxSize  int
	interval time.Duration
	
	// 实时统计
	requestMetrics  RequestMetrics
	latencyMetrics  LatencyMetrics
	errorMetrics    ErrorMetrics
}

// NewApplicationCollector 创建应用指标采集器
func NewApplicationCollector(maxSize int, interval time.Duration) *ApplicationCollector {
	if maxSize <= 0 {
		maxSize = 1440
	}
	if interval <= 0 {
		interval = time.Minute
	}
	
	return &ApplicationCollector{
		maxSize:  maxSize,
		interval: interval,
		metrics:  make(map[string][]ApplicationMetrics),
		requestMetrics: RequestMetrics{
			Timestamps:   make([]time.Time, 0),
			RequestCount: make([]uint64, 0),
			QPS:          make([]float64, 0),
			Throughput:   make([]float64, 0),
		},
		latencyMetrics: LatencyMetrics{
			Timestamps: make([]time.Time, 0),
			AvgLatency: make([]float64, 0),
			P50Latency: make([]float64, 0),
			P95Latency: make([]float64, 0),
			P99Latency: make([]float64, 0),
		},
		errorMetrics: ErrorMetrics{
			Timestamps:  make([]time.Time, 0),
			ErrorCount:  make([]uint64, 0),
			ErrorRate:   make([]float64, 0),
			ErrorTypes:  make(map[string]uint64),
			StatusCodes: make(map[int]uint64),
		},
	}
}

// RecordMetrics 记录应用指标
func (c *ApplicationCollector) RecordMetrics(m ApplicationMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	service := m.ServiceName
	if service == "" {
		service = "unknown"
	}
	
	// 添加到对应服务的指标列表
	c.metrics[service] = append(c.metrics[service], m)
	
	// 限制大小
	if len(c.metrics[service]) > c.maxSize {
		c.metrics[service] = c.metrics[service][len(c.metrics[service])-c.maxSize:]
	}
	
	// 更新实时统计
	c.updateRequestMetrics(m)
	c.updateLatencyMetrics(m)
	c.updateErrorMetrics(m)
}

// updateRequestMetrics 更新请求指标
func (c *ApplicationCollector) updateRequestMetrics(m ApplicationMetrics) {
	c.requestMetrics.Timestamps = append(c.requestMetrics.Timestamps, m.Timestamp)
	c.requestMetrics.RequestCount = append(c.requestMetrics.RequestCount, m.RequestCount)
	
	// 计算QPS (假设interval为1分钟)
	qps := float64(m.RequestCount) / 60.0
	c.requestMetrics.QPS = append(c.requestMetrics.QPS, qps)
	c.requestMetrics.Throughput = append(c.requestMetrics.Throughput, qps)
	
	// 限制大小
	if len(c.requestMetrics.Timestamps) > c.maxSize {
		c.requestMetrics.Timestamps = c.requestMetrics.Timestamps[1:]
		c.requestMetrics.RequestCount = c.requestMetrics.RequestCount[1:]
		c.requestMetrics.QPS = c.requestMetrics.QPS[1:]
		c.requestMetrics.Throughput = c.requestMetrics.Throughput[1:]
	}
}

// updateLatencyMetrics 更新时延指标
func (c *ApplicationCollector) updateLatencyMetrics(m ApplicationMetrics) {
	c.latencyMetrics.Timestamps = append(c.latencyMetrics.Timestamps, m.Timestamp)
	
	var avgLatency float64
	if m.LatencyCount > 0 {
		avgLatency = m.LatencySum / float64(m.LatencyCount)
	}
	c.latencyMetrics.AvgLatency = append(c.latencyMetrics.AvgLatency, avgLatency)
	
	// 简化的分位数计算 (实际应从原始数据计算)
	c.latencyMetrics.P50Latency = append(c.latencyMetrics.P50Latency, avgLatency)
	c.latencyMetrics.P95Latency = append(c.latencyMetrics.P95Latency, m.LatencyMax*0.95)
	c.latencyMetrics.P99Latency = append(c.latencyMetrics.P99Latency, m.LatencyMax*0.99)
	
	// 限制大小
	if len(c.latencyMetrics.Timestamps) > c.maxSize {
		c.latencyMetrics.Timestamps = c.latencyMetrics.Timestamps[1:]
		c.latencyMetrics.AvgLatency = c.latencyMetrics.AvgLatency[1:]
		c.latencyMetrics.P50Latency = c.latencyMetrics.P50Latency[1:]
		c.latencyMetrics.P95Latency = c.latencyMetrics.P95Latency[1:]
		c.latencyMetrics.P99Latency = c.latencyMetrics.P99Latency[1:]
	}
}

// updateErrorMetrics 更新异常指标
func (c *ApplicationCollector) updateErrorMetrics(m ApplicationMetrics) {
	c.errorMetrics.Timestamps = append(c.errorMetrics.Timestamps, m.Timestamp)
	c.errorMetrics.ErrorCount = append(c.errorMetrics.ErrorCount, m.ErrorCount)
	
	// 计算错误率
	var errorRate float64
	if m.RequestCount > 0 {
		errorRate = float64(m.ErrorCount) / float64(m.RequestCount) * 100
	}
	c.errorMetrics.ErrorRate = append(c.errorMetrics.ErrorRate, errorRate)
	
	// 限制大小
	if len(c.errorMetrics.Timestamps) > c.maxSize {
		c.errorMetrics.Timestamps = c.errorMetrics.Timestamps[1:]
		c.errorMetrics.ErrorCount = c.errorMetrics.ErrorCount[1:]
		c.errorMetrics.ErrorRate = c.errorMetrics.ErrorRate[1:]
	}
}

// GetRequestMetrics 获取请求指标
func (c *ApplicationCollector) GetRequestMetrics(ctx context.Context, duration time.Duration) (*RequestMetrics, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// 根据duration过滤数据
	cutoff := time.Now().Add(-duration)
	startIdx := 0
	for i, ts := range c.requestMetrics.Timestamps {
		if ts.After(cutoff) {
			startIdx = i
			break
		}
	}
	
	metrics := RequestMetrics{
		Timestamps:   c.requestMetrics.Timestamps[startIdx:],
		RequestCount: c.requestMetrics.RequestCount[startIdx:],
		QPS:          c.requestMetrics.QPS[startIdx:],
		Throughput:   c.requestMetrics.Throughput[startIdx:],
	}
	
	return &metrics, nil
}

// GetLatencyMetrics 获取时延指标
func (c *ApplicationCollector) GetLatencyMetrics(ctx context.Context, duration time.Duration) (*LatencyMetrics, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	cutoff := time.Now().Add(-duration)
	startIdx := 0
	for i, ts := range c.latencyMetrics.Timestamps {
		if ts.After(cutoff) {
			startIdx = i
			break
		}
	}
	
	metrics := LatencyMetrics{
		Timestamps: c.latencyMetrics.Timestamps[startIdx:],
		AvgLatency: c.latencyMetrics.AvgLatency[startIdx:],
		P50Latency: c.latencyMetrics.P50Latency[startIdx:],
		P95Latency: c.latencyMetrics.P95Latency[startIdx:],
		P99Latency: c.latencyMetrics.P99Latency[startIdx:],
	}
	
	return &metrics, nil
}

// GetErrorMetrics 获取异常指标
func (c *ApplicationCollector) GetErrorMetrics(ctx context.Context, duration time.Duration) (*ErrorMetrics, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	cutoff := time.Now().Add(-duration)
	startIdx := 0
	for i, ts := range c.errorMetrics.Timestamps {
		if ts.After(cutoff) {
			startIdx = i
			break
		}
	}
	
	metrics := ErrorMetrics{
		Timestamps:  c.errorMetrics.Timestamps[startIdx:],
		ErrorCount:  c.errorMetrics.ErrorCount[startIdx:],
		ErrorRate:   c.errorMetrics.ErrorRate[startIdx:],
		ErrorTypes:  make(map[string]uint64),
		StatusCodes: make(map[int]uint64),
	}
	
	// 复制错误类型分布
	for k, v := range c.errorMetrics.ErrorTypes {
		metrics.ErrorTypes[k] = v
	}
	for k, v := range c.errorMetrics.StatusCodes {
		metrics.StatusCodes[k] = v
	}
	
	return &metrics, nil
}

// GetApplicationSummary 获取应用指标汇总(带环比)
func (c *ApplicationCollector) GetApplicationSummary(ctx context.Context, serviceName string) (*ApplicationSummary, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	metrics, ok := c.metrics[serviceName]
	if !ok || len(metrics) == 0 {
		return nil, nil
	}
	
	// 获取当前周期和上一周期数据
	now := time.Now()
	currentWindow := now.Add(-time.Hour)
	previousWindow := now.Add(-2 * time.Hour)
	
	var current, previous ApplicationMetrics
	var currentCount, previousCount int
	
	for _, m := range metrics {
		if m.Timestamp.After(currentWindow) {
			current.RequestCount += m.RequestCount
			current.SuccessCount += m.SuccessCount
			current.ErrorCount += m.ErrorCount
			current.LatencySum += m.LatencySum
			current.LatencyCount += m.LatencyCount
			currentCount++
		} else if m.Timestamp.After(previousWindow) && m.Timestamp.Before(currentWindow) {
			previous.RequestCount += m.RequestCount
			previous.SuccessCount += m.SuccessCount
			previous.ErrorCount += m.ErrorCount
			previous.LatencySum += m.LatencySum
			previous.LatencyCount += m.LatencyCount
			previousCount++
		}
	}
	
	// 计算当前指标
	var currentSuccessRate, currentAvgLatency, currentErrorRate float64
	if current.RequestCount > 0 {
		currentSuccessRate = float64(current.SuccessCount) / float64(current.RequestCount) * 100
		currentErrorRate = float64(current.ErrorCount) / float64(current.RequestCount) * 100
	}
	if current.LatencyCount > 0 {
		currentAvgLatency = current.LatencySum / float64(current.LatencyCount)
	}
	
	// 计算上一周期指标
	var previousSuccessRate, previousAvgLatency, previousErrorRate float64
	if previous.RequestCount > 0 {
		previousSuccessRate = float64(previous.SuccessCount) / float64(previous.RequestCount) * 100
		previousErrorRate = float64(previous.ErrorCount) / float64(previous.RequestCount) * 100
	}
	if previous.LatencyCount > 0 {
		previousAvgLatency = previous.LatencySum / float64(previous.LatencyCount)
	}
	
	// 计算环比
	requestMoM := calculateMoM(float64(current.RequestCount), float64(previous.RequestCount))
	latencyMoM := calculateMoM(currentAvgLatency, previousAvgLatency)
	errorMoM := calculateMoM(currentErrorRate, previousErrorRate)
	
	summary := &ApplicationSummary{
		ServiceName: serviceName,
		RequestMoM:  requestMoM,
		LatencyMoM:  latencyMoM,
		ErrorMoM:    errorMoM,
		SuccessRate: currentSuccessRate,
		CurrentQPS:  float64(current.RequestCount) / 3600, // 假设1小时窗口
		AvgLatency:  currentAvgLatency,
		ErrorRate:   currentErrorRate,
		LastUpdated: now,
	}
	
	return summary, nil
}

// GetAllServicesSummary 获取所有服务汇总
func (c *ApplicationCollector) GetAllServicesSummary(ctx context.Context) ([]ApplicationSummary, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	summaries := make([]ApplicationSummary, 0, len(c.metrics))
	
	for serviceName := range c.metrics {
		summary, err := c.GetApplicationSummary(ctx, serviceName)
		if err != nil {
			continue
		}
		if summary != nil {
			summaries = append(summaries, *summary)
		}
	}
	
	// 按请求数排序
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].RequestMoM.CurrentValue > summaries[j].RequestMoM.CurrentValue
	})
	
	return summaries, nil
}

// calculateMoM 计算环比
func calculateMoM(current, previous float64) MoMComparison {
	if previous == 0 {
		return MoMComparison{
			CurrentValue:  current,
			PreviousValue: previous,
			Change:        current,
			ChangePercent: 100,
			Trend:         "up",
		}
	}
	
	change := current - previous
	changePercent := (change / previous) * 100
	
	trend := "stable"
	if math.Abs(changePercent) > 5 {
		if change > 0 {
			trend = "up"
		} else {
			trend = "down"
		}
	}
	
	return MoMComparison{
		CurrentValue:  current,
		PreviousValue: previous,
		Change:        change,
		ChangePercent: changePercent,
		Trend:         trend,
	}
}

// RecordErrorType 记录错误类型
func (c *ApplicationCollector) RecordErrorType(errorType string, count uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.errorMetrics.ErrorTypes[errorType] += count
}

// RecordStatusCode 记录HTTP状态码
func (c *ApplicationCollector) RecordStatusCode(code int, count uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.errorMetrics.StatusCodes[code] += count
}

// ApplicationMetricsAPI 应用指标API
type ApplicationMetricsAPI struct {
	collector *ApplicationCollector
}

// NewApplicationMetricsAPI 创建应用指标API
func NewApplicationMetricsAPI(collector *ApplicationCollector) *ApplicationMetricsAPI {
	return &ApplicationMetricsAPI{collector: collector}
}

// RegisterRoutes 注册路由
func (api *ApplicationMetricsAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/metrics/application/requests", api.handleRequestMetrics)
	mux.HandleFunc("/api/v1/metrics/application/latency", api.handleLatencyMetrics)
	mux.HandleFunc("/api/v1/metrics/application/errors", api.handleErrorMetrics)
	mux.HandleFunc("/api/v1/metrics/application/summary", api.handleApplicationSummary)
	mux.HandleFunc("/api/v1/metrics/application/services", api.handleServicesList)
}

// handleRequestMetrics 处理请求指标
func (api *ApplicationMetricsAPI) handleRequestMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	duration := parseDuration(r.URL.Query().Get("duration"), time.Hour)
	
	metrics, err := api.collector.GetRequestMetrics(r.Context(), duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, metrics)
}

// handleLatencyMetrics 处理时延指标
func (api *ApplicationMetricsAPI) handleLatencyMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	duration := parseDuration(r.URL.Query().Get("duration"), time.Hour)
	
	metrics, err := api.collector.GetLatencyMetrics(r.Context(), duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, metrics)
}

// handleErrorMetrics 处理异常指标
func (api *ApplicationMetricsAPI) handleErrorMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	duration := parseDuration(r.URL.Query().Get("duration"), time.Hour)
	
	metrics, err := api.collector.GetErrorMetrics(r.Context(), duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, metrics)
}

// handleApplicationSummary 处理应用汇总
func (api *ApplicationMetricsAPI) handleApplicationSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	serviceName := r.URL.Query().Get("service")
	if serviceName == "" {
		http.Error(w, "Missing service parameter", http.StatusBadRequest)
		return
	}
	
	summary, err := api.collector.GetApplicationSummary(r.Context(), serviceName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, summary)
}

// handleServicesList 处理服务列表
func (api *ApplicationMetricsAPI) handleServicesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	summaries, err := api.collector.GetAllServicesSummary(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, summaries)
}
