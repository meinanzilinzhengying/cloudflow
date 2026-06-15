package dataplane

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	edge "github.com/meinanzilinzhengying/cloudflow/proto"
	"google.golang.org/grpc"
)

type probeGRPC struct {
	edge.UnimplementedProbeServiceServer
	svc *Service
}

func RegisterProbeService(s *grpc.Server, svc *Service) {
	edge.RegisterProbeServiceServer(s, &probeGRPC{svc: svc})
}

func resolveTimestamp(ms int64) time.Time {
	if ms == 0 { return time.Now() }
	if ms > 1e15 { return time.Unix(0, ms) }
	if ms > 1e11 { return time.UnixMilli(ms) }
	return time.Unix(ms, 0)
}

func extractMetricValue(m *edge.MetricData) float64 {
	if m.Value != 0 { return m.Value }
	if m.Tags != nil {
		for _, key := range []string{"cpu_usage", "memory_usage", "metric_value", "value"} {
			if v, ok := m.Tags[key]; ok {
				if f, err := strconv.ParseFloat(v, 64); err == nil { return f }
			}
		}
	}
	if m.Latency != 0 { return float64(m.Latency) / 100.0 }
	if m.Bytes != 0 {
		if unit, hasUnit := m.Tags["unit"]; hasUnit && strings.Contains(unit, "percent") { return float64(m.Bytes) / 100.0 }
		return float64(m.Bytes)
	}
	return 0
}

func extractMetricName(m *edge.MetricData) string {
	if m.Name != "" { return m.Name }
	if m.Tags != nil { if t, ok := m.Tags["type"]; ok && t != "" { return t } }
	if m.DstIp != "" && m.DstIp != "localhost" { return m.DstIp }
	if m.Protocol != "" { return string(m.Protocol) }
	return "unknown"
}

func buildLabels(m *edge.MetricData) string {
	labels := make(map[string]string)
	for k, v := range m.Tags { labels[k] = v }
	if m.SrcIp != "" { labels["src_ip"] = m.SrcIp }
	proto := string(m.Protocol)
	if proto != "" && proto != "tcp" { labels["protocol"] = proto }
	if m.Service != "" { labels["service"] = m.Service }
	if m.Endpoint != "" { labels["endpoint"] = m.Endpoint }
	if m.Bytes != 0 { labels["bytes"] = fmt.Sprintf("%d", m.Bytes) }
	if m.Packets != 0 { labels["packets"] = fmt.Sprintf("%d", m.Packets) }
	if m.Latency != 0 { labels["latency"] = fmt.Sprintf("%d", m.Latency) }
	b, _ := json.Marshal(labels)
	return string(b)
}

func (g *probeGRPC) SendMetrics(ctx context.Context, batch *edge.MetricsBatch) (*edge.SendResponse, error) {
	fmt.Printf("[ProbeService] SendMetrics called, batch has %d metrics\n", len(batch.Metrics))
	if g.svc == nil || g.svc.clickHouseDB == nil {
		return &edge.SendResponse{Success: false, Message: "ClickHouse not ready"}, nil
	}
	if len(batch.Metrics) == 0 {
		return &edge.SendResponse{Success: true, Message: "no metrics"}, nil
	}

	tx, err := g.svc.clickHouseDB.Begin()
	if err != nil { return nil, fmt.Errorf("tx begin: %w", err) }
	defer tx.Rollback()

	// 写入 cloudflow.metrics (transaction)
	metricsStmt, err := tx.PrepareContext(ctx, "INSERT INTO cloudflow.metrics (timestamp, probe_id, metric_name, metric_value, labels)")
	if err != nil { return nil, fmt.Errorf("prepare metrics: %w", err) }
	defer metricsStmt.Close()
	for _, m := range batch.Metrics {
		ts := resolveTimestamp(m.Timestamp)
		if _, err := metricsStmt.ExecContext(ctx, ts, m.ProbeId, extractMetricName(m), extractMetricValue(m), buildLabels(m)); err != nil {
			return nil, fmt.Errorf("insert metric: %w", err)
		}
	}

	if err := tx.Commit(); err != nil { return nil, fmt.Errorf("commit: %w", err) }

	// 写入 cloudflow.flows (独立连接，transaction 不支持 Exec)
	for _, m := range batch.Metrics {
		ts := resolveTimestamp(m.Timestamp)
		proto := string(m.Protocol)
		dstIP := m.DstIp
		if dstIP == "" { dstIP = extractMetricName(m) }
		if _, err := g.svc.clickHouseDB.ExecContext(ctx,
			"INSERT INTO cloudflow.flows (timestamp, probe_id, src_ip, dst_ip, protocol, bytes, packets, latency_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			ts, m.ProbeId, m.SrcIp, dstIP, proto,
			uint64(m.Bytes), uint64(m.Packets),
			float64(m.Latency)/100.0); err != nil {
			fmt.Printf("[ProbeService] flows insert error (non-fatal): %v\n", err)
		}
	}

	g.svc.addMetricsIngested(uint64(len(batch.Metrics)))
	fmt.Printf("[ProbeService] inserted %d metrics + %d flows\n", len(batch.Metrics), len(batch.Metrics))
	return &edge.SendResponse{Success: true, Message: "ok"}, nil
}

func (g *probeGRPC) RegisterProbe(ctx context.Context, req *edge.RegisterProbeRequest) (*edge.RegisterProbeResponse, error) {
	return &edge.RegisterProbeResponse{Success: true, AssignedEdgeId: "edge-01"}, nil
}
func (g *probeGRPC) Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) (*edge.HeartbeatResponse, error) {
	return &edge.HeartbeatResponse{Success: true, ServerTime: time.Now().UnixMilli()}, nil
}
func (g *probeGRPC) SendTraces(ctx context.Context, batch *edge.TraceBatch) (*edge.SendResponse, error) { return &edge.SendResponse{Success: true}, nil }
func (g *probeGRPC) SendProfiling(ctx context.Context, batch *edge.ProfilingBatch) (*edge.SendResponse, error) { return &edge.SendResponse{Success: true}, nil }
func (g *probeGRPC) SendLogs(ctx context.Context, batch *edge.LogBatch) (*edge.SendResponse, error) { return &edge.SendResponse{Success: true}, nil }
func (g *probeGRPC) GetConfig(ctx context.Context, req *edge.GetConfigRequest) (*edge.GetConfigResponse, error) {
	// TODO: #15 - 实现完整的配置版本管理 + SHA256校验
	// 当前返回基础默认配置
	return &edge.GetConfigResponse{
		Success:    true,
		HasUpdate:  true,
		ServerTime: time.Now().Unix(),
		Config: &edge.CollectionConfig{
			Enabled:        true,
			SamplingRate:   100,
			FlushInterval:  1000,
			BatchSize:      100,
			QueueSize:      10000,
			HeartbeatInterval: 10,
		},
	}, nil
}
func (g *probeGRPC) DiscoverEdges(ctx context.Context, req *edge.DiscoverEdgesRequest) (*edge.DiscoverEdgesResponse, error) { return &edge.DiscoverEdgesResponse{Success: true}, nil }
func (g *probeGRPC) StreamData(stream edge.ProbeService_StreamDataServer) error { return fmt.Errorf("not implemented") }
