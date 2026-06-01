// Package victoria VictoriaMetrics 存储实现
//
// 用于存储 Metrics 数据，支持:
//   - 高效时序压缩
//   - PromQL 查询
//   - 多租户隔离

package victoria

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 配置
// ============================================================================

// Config VictoriaMetrics 配置
type Config struct {
	// 连接配置
	Addr        string // e.g., "http://victoriametrics:8428"
	TenantID    string // 多租户 ID (可选)

	// 批量写入
	BatchSize     int
	FlushInterval time.Duration
	QueueSize     int

	// HTTP 配置
	Timeout       time.Duration
	MaxRetries    int
	RetryInterval time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Addr:          "http://victoriametrics:8428",
		BatchSize:     10000,
		FlushInterval: time.Second,
		QueueSize:     100000,
		Timeout:       10 * time.Second,
		MaxRetries:    3,
		RetryInterval: 100 * time.Millisecond,
	}
}

// ============================================================================
// VictoriaMetrics Storage
// ============================================================================

// Storage VictoriaMetrics 存储
type Storage struct {
	config *Config
	client *http.Client

	// 批量写入
	queue    chan *Metric
	batch    []*Metric
	batchMu  sync.Mutex

	// Worker
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	// 统计
	stats Stats

	// 状态
	ready atomic.Bool
}

// Stats 统计
type Stats struct {
	MetricsWritten uint64
	MetricsDropped uint64
	WriteErrors    uint64
	QueryCount     uint64
}

// Metric 指标数据
type Metric struct {
	// 名称
	Name string

	// 标签
	Labels map[string]string

	// 值
	Value float64

	// 时间戳 (毫秒)
	Timestamp int64

	// 租户
	TenantID string
}

// MetricRow Prometheus 格式指标行
type MetricRow struct {
	Metric     map[string]string `json:"metric"`
	Values     []float64         `json:"values"`
	Timestamps []int64           `json:"timestamps"`
}

// New 创建 VictoriaMetrics 存储
func New(config *Config) (*Storage, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Storage{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		queue:  make(chan *Metric, config.QueueSize),
		batch:  make([]*Metric, 0, config.BatchSize),
		ctx:    ctx,
		cancel: cancel,
	}

	// 启动批量写入 worker
	s.wg.Add(1)
	go s.batchWriter()

	s.ready.Store(true)
	return s, nil
}

// Name 返回名称
func (s *Storage) Name() string {
	return "victoriametrics"
}

// Type 返回类型
func (s *Storage) Type() string {
	return "metrics"
}

// Ready 检查是否就绪
func (s *Storage) Ready() bool {
	return s.ready.Load()
}

// Write 写入指标
func (s *Storage) Write(ctx context.Context, metric *Metric) error {
	select {
	case s.queue <- metric:
		return nil
	default:
		s.stats.MetricsDropped++
		return errors.New("queue full")
	}
}

// WriteBatch 批量写入
func (s *Storage) WriteBatch(ctx context.Context, metrics []*Metric) error {
	for _, m := range metrics {
		if err := s.Write(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// Query 查询指标
func (s *Storage) Query(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]MetricRow, error) {
	s.stats.QueryCount++

	// 构建 URL
	url := fmt.Sprintf("%s/api/v1/query_range?query=%s&start=%d&end=%d&step=%s",
		s.config.Addr,
		query,
		start.Unix(),
		end.Unix(),
		step.String(),
	)

	if s.config.TenantID != "" {
		url += "&tenant=" + s.config.TenantID
	}

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query failed: %s", string(body))
	}

	// 解析响应
	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string       `json:"resultType"`
			Result     []MetricRow  `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data.Result, nil
}

// QueryInstant 即时查询
func (s *Storage) QueryInstant(ctx context.Context, query string) ([]MetricRow, error) {
	s.stats.QueryCount++

	url := fmt.Sprintf("%s/api/v1/query?query=%s", s.config.Addr, query)

	if s.config.TenantID != "" {
		url += "&tenant=" + s.config.TenantID
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query failed: %s", string(body))
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string      `json:"resultType"`
			Result     []MetricRow `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data.Result, nil
}

// batchWriter 批量写入 worker
func (s *Storage) batchWriter() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]*Metric, 0, s.config.BatchSize)

	for {
		select {
		case <-s.ctx.Done():
			if len(batch) > 0 {
				s.flush(batch)
			}
			return

		case m := <-s.queue:
			batch = append(batch, m)
			if len(batch) >= s.config.BatchSize {
				s.flush(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

// flush 刷新批量数据
func (s *Storage) flush(metrics []*Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	// 构建 Prometheus 格式数据
	var buf bytes.Buffer
	for _, m := range metrics {
		// 格式: metric_name{label1="value1",label2="value2"} value timestamp
		buf.WriteString(m.Name)

		if len(m.Labels) > 0 {
			buf.WriteString("{")
			first := true
			for k, v := range m.Labels {
				if !first {
					buf.WriteString(",")
				}
				buf.WriteString(fmt.Sprintf(`%s="%s"`, k, v))
				first = false
			}
			buf.WriteString("}")
		}

		buf.WriteString(fmt.Sprintf(" %g %d\n", m.Value, m.Timestamp/1000000)) // 转换为毫秒
	}

	// 发送到 VictoriaMetrics
	url := fmt.Sprintf("%s/api/v1/import/prometheus", s.config.Addr)

	if s.config.TenantID != "" {
		url += "?tenant=" + s.config.TenantID
	}

	req, err := http.NewRequestWithContext(s.ctx, "POST", url, &buf)
	if err != nil {
		s.stats.WriteErrors++
		return err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := s.client.Do(req)
	if err != nil {
		s.stats.WriteErrors++
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		s.stats.WriteErrors++
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("write failed: %s", string(body))
	}

	s.stats.MetricsWritten += uint64(len(metrics))
	return nil
}

// Close 关闭
func (s *Storage) Close() error {
	s.ready.Store(false)
	s.cancel()
	s.wg.Wait()
	return nil
}

// GetStats 获取统计
func (s *Storage) GetStats() Stats {
	return s.stats
}

// ============================================================================
// 辅助函数
// ============================================================================

// FlowToMetrics 将 Flow 转换为 Metrics
func FlowToMetrics(f interface{}, tenantID string) []*Metric {
	// TODO: 实现 Flow 到 Metrics 的转换
	// 提取: bytes, packets, latency, error_rate 等
	return nil
}

// BuildPromQL 构建 PromQL 查询
func BuildPromQL(metricName string, labels map[string]string) string {
	var sb strings.Builder
	sb.WriteString(metricName)

	if len(labels) > 0 {
		sb.WriteString("{")
		first := true
		for k, v := range labels {
			if !first {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf(`%s="%s"`, k, v))
			first = false
		}
		sb.WriteString("}")
	}

	return sb.String()
}
