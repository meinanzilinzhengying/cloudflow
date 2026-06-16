package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/golang/snappy"
)

// LokiStorage Loki日志存储
type LokiStorage struct {
	endpoint       string
	client         *http.Client
	timeout        time.Duration
	maxRetries     int
	initialBackoff time.Duration
}

// LogStream Loki日志流
type LogStream struct {
	Stream  map[string]string `json:"stream"`
	Values  [][]interface{}   `json:"values"` // [[timestamp_nano, line]]
}

// LokiPushRequest Loki推送请求
type LokiPushRequest struct {
	Streams []LogStream `json:"streams"`
}

// NewLokiStorage 创建Loki存储
func NewLokiStorage(endpoint string) *LokiStorage {
	return &LokiStorage{
		endpoint:       strings.TrimSuffix(endpoint, "/"),
		client:         &http.Client{Timeout: 30 * time.Second},
		timeout:        30 * time.Second,
		maxRetries:     3,
		initialBackoff: 1 * time.Second,
	}
}

// PushLogs 推送日志到Loki
func (l *LokiStorage) PushLogs(ctx context.Context, streams []LogStream) error {
	if len(streams) == 0 {
		return nil
	}

	// 构建请求
	reqBody := LokiPushRequest{Streams: streams}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}

	// snappy压缩
	compressed := snappy.Encode(nil, jsonData)

	// 指数退避重试
	var lastErr error
	backoff := l.initialBackoff

	for attempt := 0; attempt <= l.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, "POST",
			l.endpoint+"/loki/api/v1/push", bytes.NewReader(compressed))
		if err != nil {
			lastErr = err
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "snappy")

		resp, err := l.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(body))

		// 4xx错误不重试
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			break
		}
	}

	return fmt.Errorf("failed after %d retries: %w", l.maxRetries, lastErr)
}

// PushFlow 将Flow推送到Loki
func (l *LokiStorage) PushFlow(ctx context.Context, flow *Flow) error {
	stream := l.flowToLogStream(flow)
	return l.PushLogs(ctx, []LogStream{stream})
}

// PushFlows 批量推送Flow
func (l *LokiStorage) PushFlows(ctx context.Context, flows []*Flow) error {
	// 按标签分组
	streamMap := make(map[string]LogStream)

	for _, flow := range flows {
		stream := l.flowToLogStream(flow)
		key := streamKey(stream.Stream)

		if existing, ok := streamMap[key]; ok {
			existing.Values = append(existing.Values, stream.Values...)
			streamMap[key] = existing
		} else {
			streamMap[key] = stream
		}
	}

	streams := make([]LogStream, 0, len(streamMap))
	for _, s := range streamMap {
		// 按时间排序
		sort.Slice(s.Values, func(i, j int) bool {
			ti, _ := s.Values[i][0].(string)
			tj, _ := s.Values[j][0].(string)
			return ti < tj
		})
		streams = append(streams, s)
	}

	return l.PushLogs(ctx, streams)
}

// flowToLogStream Flow转换为Loki日志流
func (l *LokiStorage) flowToLogStream(flow *Flow) LogStream {
	// 标签
	labels := map[string]string{
		"job":      "cloudflow",
		"src_ip":   flow.SrcIP,
		"dst_ip":   flow.DstIP,
		"protocol": protocolToString(flow.Protocol),
	}

	if flow.VNI > 0 {
		labels["vni"] = fmt.Sprintf("%d", flow.VNI)
	}
	if flow.TenantID != "" {
		labels["tenant_id"] = flow.TenantID
	}
	if flow.SrcPort > 0 {
		labels["src_port"] = fmt.Sprintf("%d", flow.SrcPort)
	}
	if flow.DstPort > 0 {
		labels["dst_port"] = fmt.Sprintf("%d", flow.DstPort)
	}

	// 日志内容（JSON格式）
	logLine := map[string]interface{}{
		"src_ip":      flow.SrcIP,
		"dst_ip":      flow.DstIP,
		"src_port":    flow.SrcPort,
		"dst_port":    flow.DstPort,
		"protocol":    protocolToString(flow.Protocol),
		"bytes":       flow.Bytes,
		"packets":     flow.Packets,
		"duration_ms": flow.DurationMs,
		"vni":         flow.VNI,
		"tenant_id":   flow.TenantID,
	}

	logJSON, _ := json.Marshal(logLine)
	timestampNano := fmt.Sprintf("%d", flow.Timestamp*1000000)

	return LogStream{
		Stream: labels,
		Values: [][]interface{}{{
			timestampNano,
			string(logJSON),
		}},
	}
}

// streamKey 生成流的唯一键
func streamKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(parts, ",")
}

// HealthCheck 健康检查
func (l *LokiStorage) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", l.endpoint+"/ready", nil)
	if err != nil {
		return err
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Loki health check failed: %d", resp.StatusCode)
	}
	return nil
}

// FormatFlowAsText Flow转文本日志格式
func (l *LokiStorage) FormatFlowAsText(flow *Flow) string {
	return fmt.Sprintf(
		"FLOW: %s:%d -> %s:%d (%s) %d bytes %d packets %dms VNI=%d",
		flow.SrcIP, flow.SrcPort,
		flow.DstIP, flow.DstPort,
		protocolToString(flow.Protocol),
		flow.Bytes, flow.Packets, flow.DurationMs,
		flow.VNI,
	)
}

// BatchSize 批量大小配置
const (
	DefaultLokiBatchSize    = 1000
	DefaultLokiFlushInterval = 1000 // 毫秒
)

// BufferedLokiWriter 带缓冲的Loki写入器
type BufferedLokiWriter struct {
	storage     *LokiStorage
	buffer      []*Flow
	batchSize   int
	flushTicker *time.Ticker
}

// NewBufferedLokiWriter 创建带缓冲的写入器
func NewBufferedLokiWriter(endpoint string, batchSize int, flushIntervalMs int) *BufferedLokiWriter {
	if batchSize <= 0 {
		batchSize = DefaultLokiBatchSize
	}
	if flushIntervalMs <= 0 {
		flushIntervalMs = DefaultLokiFlushInterval
	}

	return &BufferedLokiWriter{
		storage:     NewLokiStorage(endpoint),
		buffer:      make([]*Flow, 0, batchSize),
		batchSize:   batchSize,
		flushTicker: time.NewTicker(time.Duration(flushIntervalMs) * time.Millisecond),
	}
}

// Write 写入Flow（带缓冲）
func (b *BufferedLokiWriter) Write(ctx context.Context, flow *Flow) error {
	b.buffer = append(b.buffer, flow)

	if len(b.buffer) >= b.batchSize {
		return b.Flush(ctx)
	}

	return nil
}

// Flush 手动刷新缓冲
func (b *BufferedLokiWriter) Flush(ctx context.Context) error {
	if len(b.buffer) == 0 {
		return nil
	}

	err := b.storage.PushFlows(ctx, b.buffer)
	b.buffer = b.buffer[:0]
	return err
}

// Start 启动自动刷新
func (b *BufferedLokiWriter) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				b.flushTicker.Stop()
				return
			case <-b.flushTicker.C:
				_ = b.Flush(ctx)
			}
		}
	}()
}
