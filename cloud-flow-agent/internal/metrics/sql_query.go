package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// SQLMetric SQL聚合指标
type SQLMetric struct {
	Timestamp       time.Time `json:"timestamp"`
	SQLHash         string    `json:"sql_hash"`           // SQL指纹
	SQLTemplate     string    `json:"sql_template"`       // SQL模板
	Database        string    `json:"database"`           // 数据库名
	Table           string    `json:"table"`              // 表名
	RequestCount    uint64    `json:"request_count"`      // 请求数
	SuccessCount    uint64    `json:"success_count"`      // 成功数
	ErrorCount      uint64    `json:"error_count"`        // 错误数
	AvgLatencyMs    float64   `json:"avg_latency_ms"`     // 平均时延
	MaxLatencyMs    float64   `json:"max_latency_ms"`     // 最大时延
	MinLatencyMs    float64   `json:"min_latency_ms"`     // 最小时延
	TotalLatencyMs  float64   `json:"total_latency_ms"`   // 总时延
}

// SQLProcessCorrelation SQL与进程关联
type SQLProcessCorrelation struct {
	SQLHash         string  `json:"sql_hash"`
	ProcessID       uint32  `json:"process_id"`
	ProcessName     string  `json:"process_name"`
	CPUUsage        float64 `json:"cpu_usage"`         // CPU使用率
	MemoryMB        float64 `json:"memory_mb"`         // 内存使用(MB)
	ConnectionCount uint64  `json:"connection_count"`  // 连接数
	QPS             float64 `json:"qps"`               // 每秒查询数
}

// SQLQueryResult SQL查询结果
type SQLQueryResult struct {
	Total       int         `json:"total"`
	Page        int         `json:"page"`
	PageSize    int         `json:"page_size"`
	Metrics     []SQLMetric `json:"metrics"`
	Aggregations map[string]interface{} `json:"aggregations"`
}

// SQLQueryFilter SQL查询过滤条件
type SQLQueryFilter struct {
	Database        string    `json:"database"`
	Table           string    `json:"table"`
	SQLPattern      string    `json:"sql_pattern"`
	MinLatencyMs    float64   `json:"min_latency_ms"`
	MaxLatencyMs    float64   `json:"max_latency_ms"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	SortBy          string    `json:"sort_by"`
	SortOrder       string    `json:"sort_order"`
	Page            int       `json:"page"`
	PageSize        int       `json:"page_size"`
}

// SQLAggregation SQL聚合统计
type SQLAggregation struct {
	TotalQueries      uint64             `json:"total_queries"`
	TotalErrors       uint64             `json:"total_errors"`
	AvgLatencyMs      float64            `json:"avg_latency_ms"`
	MaxLatencyMs      float64            `json:"max_latency_ms"`
	SlowQueryCount    uint64             `json:"slow_query_count"`
	TopSlowQueries    []SQLMetric        `json:"top_slow_queries"`
	TopFrequentQueries []SQLMetric       `json:"top_frequent_queries"`
	ErrorDistribution map[string]uint64  `json:"error_distribution"`
	LatencyHistogram  map[string]uint64  `json:"latency_histogram"`
}

// SQLMetricsStore SQL指标存储
type SQLMetricsStore struct {
	mu      sync.RWMutex
	metrics []SQLMetric
	maxSize int
	
	// 索引
	byHash     map[string][]*SQLMetric
	byDatabase map[string][]*SQLMetric
	byTable    map[string][]*SQLMetric
}

// NewSQLMetricsStore 创建SQL指标存储
func NewSQLMetricsStore(maxSize int) *SQLMetricsStore {
	if maxSize <= 0 {
		maxSize = 10000
	}
	
	return &SQLMetricsStore{
		maxSize:    maxSize,
		metrics:    make([]SQLMetric, 0, maxSize),
		byHash:     make(map[string][]*SQLMetric),
		byDatabase: make(map[string][]*SQLMetric),
		byTable:    make(map[string][]*SQLMetric),
	}
}

// Store 存储SQL指标
func (s *SQLMetricsStore) Store(m SQLMetric) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 添加到列表
	s.metrics = append(s.metrics, m)
	
	// 限制大小
	if len(s.metrics) > s.maxSize {
		removed := s.metrics[0]
		s.metrics = s.metrics[1:]
		
		// 从索引中移除
		s.removeFromIndex(&removed)
	}
	
	// 添加到索引
	ptr := &s.metrics[len(s.metrics)-1]
	s.addToIndex(ptr)
}

// addToIndex 添加到索引
func (s *SQLMetricsStore) addToIndex(m *SQLMetric) {
	s.byHash[m.SQLHash] = append(s.byHash[m.SQLHash], m)
	
	if m.Database != "" {
		s.byDatabase[m.Database] = append(s.byDatabase[m.Database], m)
	}
	
	if m.Table != "" {
		s.byTable[m.Table] = append(s.byTable[m.Table], m)
	}
}

// removeFromIndex 从索引移除
func (s *SQLMetricsStore) removeFromIndex(m *SQLMetric) {
	// 简化处理：重建索引
	// 实际生产环境应使用更高效的数据结构
}

// Query 查询SQL指标
func (s *SQLMetricsStore) Query(ctx context.Context, filter SQLQueryFilter) (*SQLQueryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// 过滤数据
	filtered := s.filterMetrics(filter)
	
	// 排序
	s.sortMetrics(filtered, filter.SortBy, filter.SortOrder)
	
	// 分页
	total := len(filtered)
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	
	// 计算聚合
	aggregations := s.calculateAggregations(filtered)
	
	return &SQLQueryResult{
		Total:        total,
		Page:         page,
		PageSize:     pageSize,
		Metrics:      filtered[start:end],
		Aggregations: aggregations,
	}, nil
}

// filterMetrics 过滤指标
func (s *SQLMetricsStore) filterMetrics(filter SQLQueryFilter) []SQLMetric {
	var result []SQLMetric
	
	for _, m := range s.metrics {
		// 时间范围过滤
		if !filter.StartTime.IsZero() && m.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && m.Timestamp.After(filter.EndTime) {
			continue
		}
		
		// 数据库过滤
		if filter.Database != "" && m.Database != filter.Database {
			continue
		}
		
		// 表过滤
		if filter.Table != "" && m.Table != filter.Table {
			continue
		}
		
		// SQL模式过滤
		if filter.SQLPattern != "" && !strings.Contains(m.SQLTemplate, filter.SQLPattern) {
			continue
		}
		
		// 时延过滤
		if filter.MinLatencyMs > 0 && m.AvgLatencyMs < filter.MinLatencyMs {
			continue
		}
		if filter.MaxLatencyMs > 0 && m.AvgLatencyMs > filter.MaxLatencyMs {
			continue
		}
		
		result = append(result, m)
	}
	
	return result
}

// sortMetrics 排序指标
func (s *SQLMetricsStore) sortMetrics(metrics []SQLMetric, sortBy, order string) {
	if sortBy == "" {
		sortBy = "timestamp"
	}
	
	ascending := order != "desc"
	
	sort.Slice(metrics, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "latency":
			less = metrics[i].AvgLatencyMs < metrics[j].AvgLatencyMs
		case "count":
			less = metrics[i].RequestCount < metrics[j].RequestCount
		case "error":
			less = metrics[i].ErrorCount < metrics[j].ErrorCount
		default:
			less = metrics[i].Timestamp.Before(metrics[j].Timestamp)
		}
		
		if ascending {
			return less
		}
		return !less
	})
}

// calculateAggregations 计算聚合
func (s *SQLMetricsStore) calculateAggregations(metrics []SQLMetric) map[string]interface{} {
	if len(metrics) == 0 {
		return map[string]interface{}{
			"total_queries": 0,
			"total_errors":  0,
			"avg_latency":   0,
		}
	}
	
	var totalQueries, totalErrors uint64
	var totalLatency float64
	var maxLatency float64
	
	for _, m := range metrics {
		totalQueries += m.RequestCount
		totalErrors += m.ErrorCount
		totalLatency += m.TotalLatencyMs
		if m.MaxLatencyMs > maxLatency {
			maxLatency = m.MaxLatencyMs
		}
	}
	
	avgLatency := totalLatency / float64(totalQueries)
	
	return map[string]interface{}{
		"total_queries":   totalQueries,
		"total_errors":    totalErrors,
		"avg_latency_ms":  avgLatency,
		"max_latency_ms":  maxLatency,
		"error_rate":      float64(totalErrors) / float64(totalQueries) * 100,
		"data_points":     len(metrics),
	}
}

// GetAggregation 获取SQL聚合统计
func (s *SQLMetricsStore) GetAggregation(ctx context.Context, duration time.Duration) (*SQLAggregation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// 过滤时间范围
	cutoff := time.Now().Add(-duration)
	var filtered []SQLMetric
	
	for _, m := range s.metrics {
		if m.Timestamp.After(cutoff) {
			filtered = append(filtered, m)
		}
	}
	
	if len(filtered) == 0 {
		return &SQLAggregation{
			ErrorDistribution: make(map[string]uint64),
			LatencyHistogram:  make(map[string]uint64),
		}, nil
	}
	
	// 计算聚合
	agg := &SQLAggregation{
		ErrorDistribution:  make(map[string]uint64),
		LatencyHistogram:   make(map[string]uint64),
		TopSlowQueries:     make([]SQLMetric, 0),
		TopFrequentQueries: make([]SQLMetric, 0),
	}
	
	// 按SQL Hash聚合
	hashMap := make(map[string]*SQLMetric)
	for i := range filtered {
		m := &filtered[i]
		if existing, ok := hashMap[m.SQLHash]; ok {
			existing.RequestCount += m.RequestCount
			existing.ErrorCount += m.ErrorCount
			existing.TotalLatencyMs += m.TotalLatencyMs
			if m.MaxLatencyMs > existing.MaxLatencyMs {
				existing.MaxLatencyMs = m.MaxLatencyMs
			}
		} else {
			copy := *m
			hashMap[m.SQLHash] = &copy
		}
	}
	
	// 转换为切片
	var aggregated []SQLMetric
	for _, m := range hashMap {
		if m.RequestCount > 0 {
			m.AvgLatencyMs = m.TotalLatencyMs / float64(m.RequestCount)
		}
		aggregated = append(aggregated, *m)
		
		agg.TotalQueries += m.RequestCount
		agg.TotalErrors += m.ErrorCount
	}
	
	// 计算平均时延
	if agg.TotalQueries > 0 {
		var totalLatency float64
		for _, m := range aggregated {
			totalLatency += m.TotalLatencyMs
		}
		agg.AvgLatencyMs = totalLatency / float64(agg.TotalQueries)
	}
	
	// 找出最慢的查询
	sort.Slice(aggregated, func(i, j int) bool {
		return aggregated[i].AvgLatencyMs > aggregated[j].AvgLatencyMs
	})
	if len(aggregated) > 10 {
		agg.TopSlowQueries = aggregated[:10]
	} else {
		agg.TopSlowQueries = aggregated
	}
	if len(aggregated) > 0 {
		agg.MaxLatencyMs = aggregated[0].MaxLatencyMs
	}
	
	// 找出最频繁的查询
	sort.Slice(aggregated, func(i, j int) bool {
		return aggregated[i].RequestCount > aggregated[j].RequestCount
	})
	if len(aggregated) > 10 {
		agg.TopFrequentQueries = aggregated[:10]
	} else {
		agg.TopFrequentQueries = aggregated
	}
	
	// 计算慢查询数 (>100ms)
	for _, m := range aggregated {
		if m.AvgLatencyMs > 100 {
			agg.SlowQueryCount += m.RequestCount
		}
	}
	
	// 时延分布直方图
	for _, m := range aggregated {
		bucket := getLatencyBucket(m.AvgLatencyMs)
		agg.LatencyHistogram[bucket] += m.RequestCount
	}
	
	return agg, nil
}

// getLatencyBucket 获取时延分桶
func getLatencyBucket(latencyMs float64) string {
	switch {
	case latencyMs < 1:
		return "<1ms"
	case latencyMs < 10:
		return "1-10ms"
	case latencyMs < 50:
		return "10-50ms"
	case latencyMs < 100:
		return "50-100ms"
	case latencyMs < 500:
		return "100-500ms"
	case latencyMs < 1000:
		return "500ms-1s"
	default:
		return ">1s"
	}
}

// GetProcessCorrelation 获取SQL与进程关联
func (s *SQLMetricsStore) GetProcessCorrelation(ctx context.Context, sqlHash string) ([]SQLProcessCorrelation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// 模拟进程关联数据
	// 实际应从进程监控模块获取
	correlations := []SQLProcessCorrelation{
		{
			SQLHash:         sqlHash,
			ProcessID:       1234,
			ProcessName:     "mysqld",
			CPUUsage:        15.5,
			MemoryMB:        512.0,
			ConnectionCount: 50,
			QPS:             1000.0,
		},
	}
	
	return correlations, nil
}

// SQLQueryAPI SQL查询API
type SQLQueryAPI struct {
	store *SQLMetricsStore
}

// NewSQLQueryAPI 创建SQL查询API
func NewSQLQueryAPI(store *SQLMetricsStore) *SQLQueryAPI {
	return &SQLQueryAPI{store: store}
}

// RegisterRoutes 注册路由
func (api *SQLQueryAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/metrics/sql/query", api.handleSQLQuery)
	mux.HandleFunc("/api/v1/metrics/sql/aggregation", api.handleSQLAggregation)
	mux.HandleFunc("/api/v1/metrics/sql/slow", api.handleSlowQueries)
	mux.HandleFunc("/api/v1/metrics/sql/correlation", api.handleProcessCorrelation)
}

// handleSQLQuery 处理SQL查询
func (api *SQLQueryAPI) handleSQLQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var filter SQLQueryFilter
	
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&filter); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		// GET请求从URL参数解析
		filter.Database = r.URL.Query().Get("database")
		filter.Table = r.URL.Query().Get("table")
		filter.SQLPattern = r.URL.Query().Get("pattern")
		filter.SortBy = r.URL.Query().Get("sort_by")
		filter.SortOrder = r.URL.Query().Get("sort_order")
		filter.Page = 1
		filter.PageSize = 20
	}
	
	result, err := api.store.Query(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, result)
}

// handleSQLAggregation 处理SQL聚合统计
func (api *SQLQueryAPI) handleSQLAggregation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	duration := parseDuration(r.URL.Query().Get("duration"), time.Hour)
	
	agg, err := api.store.GetAggregation(r.Context(), duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, agg)
}

// handleSlowQueries 处理慢查询
func (api *SQLQueryAPI) handleSlowQueries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 默认查询慢查询 (>100ms)
	filter := SQLQueryFilter{
		MinLatencyMs: 100,
		SortBy:       "latency",
		SortOrder:    "desc",
		Page:         1,
		PageSize:     20,
	}
	
	result, err := api.store.Query(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, result)
}

// handleProcessCorrelation 处理进程关联查询
func (api *SQLQueryAPI) handleProcessCorrelation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	sqlHash := r.URL.Query().Get("sql_hash")
	if sqlHash == "" {
		http.Error(w, "Missing sql_hash parameter", http.StatusBadRequest)
		return
	}
	
	correlations, err := api.store.GetProcessCorrelation(r.Context(), sqlHash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, correlations)
}

// SQLMetricsConfig SQL指标配置
type SQLMetricsConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	MaxStoreSize      int           `yaml:"max_store_size" json:"max_store_size"`
	SlowQueryThresholdMs float64    `yaml:"slow_query_threshold_ms" json:"slow_query_threshold_ms"`
	RetentionTime     time.Duration `yaml:"retention_time" json:"retention_time"`
}

// DefaultSQLMetricsConfig 默认配置
func DefaultSQLMetricsConfig() SQLMetricsConfig {
	return SQLMetricsConfig{
		Enabled:              true,
		MaxStoreSize:         10000,
		SlowQueryThresholdMs: 100,
		RetentionTime:        time.Hour * 24,
	}
}
