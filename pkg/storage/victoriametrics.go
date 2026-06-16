package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

// VictoriaMetricsStorage VictoriaMetrics远程写入存储
type VictoriaMetricsStorage struct {
	endpoint       string
	client         *http.Client
	timeout        time.Duration
	maxRetries     int
	initialBackoff time.Duration
}

// Metric 指标结构
type Metric struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"` // 毫秒时间戳
}

// NewVictoriaMetricsStorage 创建VictoriaMetrics存储
func NewVictoriaMetricsStorage(endpoint string) *VictoriaMetricsStorage {
	return &VictoriaMetricsStorage{
		endpoint:       strings.TrimSuffix(endpoint, "/"),
		client:         &http.Client{Timeout: 30 * time.Second},
		timeout:        30 * time.Second,
		maxRetries:     3,
		initialBackoff: 1 * time.Second,
	}
}

// WriteMetrics 批量写入指标
func (v *VictoriaMetricsStorage) WriteMetrics(ctx context.Context, metrics []Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	// 转换为Influx Line Protocol格式（VictoriaMetrics原生支持）
	lines := make([]string, 0, len(metrics))
	for _, m := range metrics {
		line := v.metricToInfluxLine(m)
		lines = append(lines, line)
	}
	payload := strings.Join(lines, "\n")

	// gzip压缩
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(payload)); err != nil {
		return fmt.Errorf("gzip compress failed: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("gzip close failed: %w", err)
	}

	// 指数退避重试
	var lastErr error
	backoff := v.initialBackoff

	for attempt := 0; attempt <= v.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, "POST",
			v.endpoint+"/api/v1/import", bytes.NewReader(buf.Bytes()))
		if err != nil {
			lastErr = err
			continue
		}

		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Content-Type", "text/plain")

		resp, err := v.client.Do(req)
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

	return fmt.Errorf("failed after %d retries: %w", v.maxRetries, lastErr)
}

// WriteFlow 将Flow转换为指标并写入
func (v *VictoriaMetricsStorage) WriteFlow(ctx context.Context, flow *Flow) error {
	metrics := v.flowToMetrics(flow)
	return v.WriteMetrics(ctx, metrics)
}

// WriteFlows 批量写入Flow
func (v *VictoriaMetricsStorage) WriteFlows(ctx context.Context, flows []*Flow) error {
	allMetrics := make([]Metric, 0, len(flows)*5)
	for _, flow := range flows {
		allMetrics = append(allMetrics, v.flowToMetrics(flow)...)
	}
	return v.WriteMetrics(ctx, allMetrics)
}

// metricToInfluxLine 转换为Influx Line Protocol
func (v *VictoriaMetricsStorage) metricToInfluxLine(m Metric) string {
	// measurement,tag1=value1,tag2=value2 field=value timestamp
	var tags []string
	for k, val := range m.Labels {
		if val != "" {
			tags = append(tags, fmt.Sprintf("%s=%s", escapeInfluxTag(k), escapeInfluxTag(val)))
		}
	}

	tagStr := ""
	if len(tags) > 0 {
		tagStr = "," + strings.Join(tags, ",")
	}

	return fmt.Sprintf("%s%s value=%f %d",
		escapeInfluxMeasurement(m.Name),
		tagStr,
		m.Value,
		m.Timestamp*1000000, // 毫秒转纳秒
	)
}

// flowToMetrics Flow转换为多个指标
func (v *VictoriaMetricsStorage) flowToMetrics(flow *Flow) []Metric {
	if flow == nil {
		return nil
	}

	ts := flow.Timestamp
	labels := map[string]string{
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

	return []Metric{
		{
			Name:      "cloudflow_flow_bytes",
			Labels:    labels,
			Value:     float64(flow.Bytes),
			Timestamp: ts,
		},
		{
			Name:      "cloudflow_flow_packets",
			Labels:    labels,
			Value:     float64(flow.Packets),
			Timestamp: ts,
		},
		{
			Name:      "cloudflow_flow_duration_ms",
			Labels:    labels,
			Value:     float64(flow.DurationMs),
			Timestamp: ts,
		},
	}
}

// protocolToString 协议号转字符串
func protocolToString(proto uint8) string {
	switch proto {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 1:
		return "ICMP"
	default:
		return fmt.Sprintf("%d", proto)
	}
}

// escapeInfluxMeasurement 转义measurement
func escapeInfluxMeasurement(s string) string {
	s = strings.ReplaceAll(s, " ", "\\ ")
	s = strings.ReplaceAll(s, ",", "\\,")
	return s
}

// escapeInfluxTag 转义标签
func escapeInfluxTag(s string) string {
	s = strings.ReplaceAll(s, " ", "\\ ")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "=", "\\=")
	return s
}

// HealthCheck 健康检查
func (v *VictoriaMetricsStorage) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", v.endpoint+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("VictoriaMetrics health check failed: %d", resp.StatusCode)
	}
	return nil
}

// SampleFlowToJSON Flow转JSON示例
func (v *VictoriaMetricsStorage) SampleFlowToJSON(flow *Flow) string {
	data, _ := json.MarshalIndent(flow, "", "  ")
	return string(data)
}

// ModelMetricToVictoriaMetrics Prometheus model.Metric转换
func ModelMetricToVictoriaMetrics(m model.Metric) Metric {
	labels := make(map[string]string)
	for k, v := range m {
		labels[string(k)] = string(v)
	}

	name := string(m[model.MetricNameLabel])
	delete(labels, string(model.MetricNameLabel))

	return Metric{
		Name:      name,
		Labels:    labels,
		Value:     float64(m[model.MetricNameLabel]),
		Timestamp: int64(m.Time),
	}
}
