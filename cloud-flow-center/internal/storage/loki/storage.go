// Package loki Loki 存储实现
//
// 用于存储 Logs 数据，支持:
//   - 高效日志压缩
//   - LogQL 查询
//   - 标签索引

package loki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 配置
// ============================================================================

// Config Loki 配置
type Config struct {
	// 连接配置
	Addr     string // e.g., "http://loki:3100"
	TenantID string // 多租户 ID (X-Scope-OrgID header)

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
		Addr:          "http://loki:3100",
		BatchSize:     10000,
		FlushInterval: time.Second,
		QueueSize:     100000,
		Timeout:       10 * time.Second,
		MaxRetries:    3,
		RetryInterval: 100 * time.Millisecond,
	}
}

// ============================================================================
// Loki Storage
// ============================================================================

// Storage Loki 存储
type Storage struct {
	config *Config
	client *http.Client

	// 批量写入 (按 stream 分组)
	streams map[string]*LogStream
	streamMu sync.Mutex

	// 队列
	queue chan *LogEntry

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
	LogsWritten  uint64
	LogsDropped  uint64
	WriteErrors  uint64
	QueryCount   uint64
}

// LogEntry 日志条目
type LogEntry struct {
	// 标签 (stream)
	Labels map[string]string

	// 时间戳 (纳秒)
	Timestamp int64

	// 日志内容
	Line string

	// 租户
	TenantID string
}

// LogStream 日志流
type LogStream struct {
	Labels  map[string]string
	Entries []*LogEntry
}

// LokiPushRequest Loki 推送请求
type LokiPushRequest struct {
	Streams []LokiStream `json:"streams"`
}

// LokiStream Loki 流格式
type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]interface{}   `json:"values"` // [timestamp, line]
}

// LokiQueryResult Loki 查询结果
type LokiQueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"` // [timestamp, line]
		} `json:"result"`
	} `json:"data"`
}

// New 创建 Loki 存储
func New(config *Config) (*Storage, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Storage{
		config:  config,
		client:  &http.Client{Timeout: config.Timeout},
		streams: make(map[string]*LogStream),
		queue:   make(chan *LogEntry, config.QueueSize),
		ctx:     ctx,
		cancel:  cancel,
	}

	// 启动批量写入 worker
	s.wg.Add(1)
	go s.batchWriter()

	s.ready.Store(true)
	return s, nil
}

// Name 返回名称
func (s *Storage) Name() string {
	return "loki"
}

// Type 返回类型
func (s *Storage) Type() string {
	return "logs"
}

// Ready 检查是否就绪
func (s *Storage) Ready() bool {
	return s.ready.Load()
}

// Write 写入日志
func (s *Storage) Write(ctx context.Context, entry *LogEntry) error {
	select {
	case s.queue <- entry:
		return nil
	default:
		s.stats.LogsDropped++
		return errors.New("queue full")
	}
}

// WriteBatch 批量写入
func (s *Storage) WriteBatch(ctx context.Context, entries []*LogEntry) error {
	for _, e := range entries {
		if err := s.Write(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// Query 查询日志
func (s *Storage) Query(ctx context.Context, query string, start, end time.Time, limit int) (*LokiQueryResult, error) {
	s.stats.QueryCount++

	// 构建 URL
	url := fmt.Sprintf("%s/loki/api/v1/query_range?query=%s&start=%d&end=%d&limit=%d",
		s.config.Addr,
		query,
		start.UnixNano(),
		end.UnixNano(),
		limit,
	)

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 设置租户 header
	if s.config.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", s.config.TenantID)
	}

	// 发送请求
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
	var result LokiQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// QueryInstant 即时查询
func (s *Storage) QueryInstant(ctx context.Context, query string, limit int) (*LokiQueryResult, error) {
	s.stats.QueryCount++

	url := fmt.Sprintf("%s/loki/api/v1/query?query=%s&limit=%d",
		s.config.Addr,
		query,
		limit,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if s.config.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", s.config.TenantID)
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

	var result LokiQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// batchWriter 批量写入 worker
func (s *Storage) batchWriter() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]*LogEntry, 0, s.config.BatchSize)

	for {
		select {
		case <-s.ctx.Done():
			if len(batch) > 0 {
				s.flush(batch)
			}
			return

		case entry := <-s.queue:
			batch = append(batch, entry)
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
func (s *Storage) flush(entries []*LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// 按 stream 分组
	streams := make(map[string]*LokiStream)
	for _, e := range entries {
		key := labelsKey(e.Labels)
		if _, exists := streams[key]; !exists {
			streams[key] = &LokiStream{
				Stream: e.Labels,
				Values: make([][]interface{}, 0),
			}
		}
		// Loki 需要 nanosecond timestamp as string
		streams[key].Values = append(streams[key].Values, []interface{}{
			fmt.Sprintf("%d", e.Timestamp),
			e.Line,
		})
	}

	// 构建请求
	req := LokiPushRequest{
		Streams: make([]LokiStream, 0, len(streams)),
	}
	for _, stream := range streams {
		req.Streams = append(req.Streams, *stream)
	}

	body, err := json.Marshal(req)
	if err != nil {
		s.stats.WriteErrors++
		return err
	}

	// 发送到 Loki
	url := fmt.Sprintf("%s/loki/api/v1/push", s.config.Addr)

	httpReq, err := http.NewRequestWithContext(s.ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		s.stats.WriteErrors++
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if s.config.TenantID != "" {
		httpReq.Header.Set("X-Scope-OrgID", s.config.TenantID)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		s.stats.WriteErrors++
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		s.stats.WriteErrors++
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("write failed: %s", string(respBody))
	}

	s.stats.LogsWritten += uint64(len(entries))
	return nil
}

// labelsKey 生成标签 key
func labelsKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	// 简单实现：拼接所有键值对
	// 生产环境应该使用更高效的哈希
	var key string
	for k, v := range labels {
		key += k + "=" + v + ","
	}
	return key
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

// BuildLogQL 构建 LogQL 查询
func BuildLogQL(labels map[string]string, filter string) string {
	var ql string

	// 标签选择器
	if len(labels) > 0 {
		ql = "{"
		first := true
		for k, v := range labels {
			if !first {
				ql += ", "
			}
			ql += fmt.Sprintf(`%s="%s"`, k, v)
			first = false
		}
		ql += "}"
	} else {
		ql = `{}` 
	}

	// 过滤表达式
	if filter != "" {
		ql += " " + filter
	}

	return ql
}

// FlowToLogEntry 将 Flow 转换为 LogEntry
func FlowToLogEntry(f interface{}, tenantID string) *LogEntry {
	// TODO: 实现 Flow 到 LogEntry 的转换
	// 提取: L7 请求/响应内容、异常信息等
	return nil
}
