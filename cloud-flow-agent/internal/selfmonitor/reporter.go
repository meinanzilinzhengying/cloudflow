// Package selfmonitor 提供自监控指标上报功能
//
// 每10秒将自监控指标上报至Center
package selfmonitor

import (
	"context"
	"fmt"
	"time"

	"cloud-flow-agent/pkg/logger"
	edge "cloud-flow/proto"
)

// MetricsSender 指标发送接口（适配grpcclient.Client）
type MetricsSender interface {
	SendMetrics(ctx context.Context, batch *edge.MetricsBatch) error
}

// Reporter 自监控指标上报器
type Reporter struct {
	cfg       Config
	log       *logger.Logger
	collector *Collector

	// gRPC客户端
	sender MetricsSender

	// 生命周期
	stopCh chan struct{}

	// 探针标识
	probeID string
}

// NewReporter 创建自监控上报器
func NewReporter(cfg Config, collector *Collector, sender MetricsSender, probeID string, log *logger.Logger) *Reporter {
	if cfg.ReportInterval <= 0 {
		cfg.ReportInterval = 10 * time.Second
	}

	return &Reporter{
		cfg:       cfg,
		log:       log,
		collector: collector,
		sender:    sender,
		probeID:   probeID,
		stopCh:    make(chan struct{}),
	}
}

// Start 启动上报器
func (r *Reporter) Start() {
	go r.reportLoop()
	r.log.Info("[自监控] 上报器已启动，上报间隔: %v", r.cfg.ReportInterval)
}

// Stop 停止上报器
func (r *Reporter) Stop() {
	close(r.stopCh)
	r.log.Info("[自监控] 上报器已停止")
}

// reportLoop 上报循环
func (r *Reporter) reportLoop() {
	ticker := time.NewTicker(r.cfg.ReportInterval)
	defer ticker.Stop()

	// 立即执行一次上报
	r.report()

	for {
		select {
		case <-ticker.C:
			r.report()
		case <-r.stopCh:
			return
		}
	}
}

// report 执行一次上报
func (r *Reporter) report() {
	if r.sender == nil {
		r.log.Warn("[自监控] gRPC客户端未初始化，跳过上报")
		return
	}

	// 获取最新指标快照
	snapshot := r.collector.GetSnapshot()

	// 转换为MetricData
	metrics := r.snapshotToMetrics(snapshot)
	if len(metrics) == 0 {
		return
	}

	// 打包成批次
	batch := &edge.MetricsBatch{
		ProbeId:   r.probeID,
		Timestamp: time.Now().Unix(),
		Metrics:   metrics,
	}

	// 上报
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.sender.SendMetrics(ctx, batch); err != nil {
		r.log.Warnf("[自监控] 上报失败: %v", err)
	} else {
		r.log.Debugf("[自监控] 上报成功: %d条指标", len(metrics))
	}
}

// snapshotToMetrics 将指标快照转换为MetricData列表
func (r *Reporter) snapshotToMetrics(s MetricsSnapshot) []*edge.MetricData {
	now := time.Now().Unix()
	baseTags := map[string]string{
		"probe_id": r.probeID,
		"source":   "self_monitor",
	}

	metrics := []*edge.MetricData{
		{
			Timestamp: now,
			ProbeId:   r.probeID,
			MetricId:  "self_heartbeat_status",
			Name:      "agent_heartbeat_status",
			Value:     float64(s.HeartbeatStatus),
			Tags:      mergeTags(baseTags, map[string]string{"metric_type": "status"}),
		},
		{
			Timestamp: now,
			ProbeId:   r.probeID,
			MetricId:  "self_heartbeat_latency_ms",
			Name:      "agent_heartbeat_latency",
			Value:     s.HeartbeatLatencyMs,
			Tags:      mergeTags(baseTags, map[string]string{"metric_type": "latency", "unit": "ms"}),
		},
		{
			Timestamp: now,
			ProbeId:   r.probeID,
			MetricId:  "self_cpu_percent",
			Name:      "agent_cpu_usage",
			Value:     s.CPUPercent,
			Tags:      mergeTags(baseTags, map[string]string{"metric_type": "usage", "unit": "percent"}),
		},
		{
			Timestamp: now,
			ProbeId:   r.probeID,
			MetricId:  "self_memory_percent",
			Name:      "agent_memory_usage",
			Value:     s.MemoryPercent,
			Tags:      mergeTags(baseTags, map[string]string{"metric_type": "usage", "unit": "percent"}),
		},
		{
			Timestamp: now,
			ProbeId:   r.probeID,
			MetricId:  "self_packet_drop_rate",
			Name:      "agent_packet_drop_rate",
			Value:     s.PacketDropRate,
			Tags:      mergeTags(baseTags, map[string]string{"metric_type": "rate", "unit": "percent"}),
		},
		{
			Timestamp: now,
			ProbeId:   r.probeID,
			MetricId:  "self_report_success_rate",
			Name:      "agent_report_success_rate",
			Value:     s.ReportSuccessRate,
			Tags:      mergeTags(baseTags, map[string]string{"metric_type": "rate", "unit": "percent"}),
		},
	}

	return metrics
}

// mergeTags 合并标签
func mergeTags(base, extra map[string]string) map[string]string {
	result := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}

// ReportNow 立即上报一次（用于测试或强制上报）
func (r *Reporter) ReportNow() error {
	if r.sender == nil {
		return fmt.Errorf("gRPC客户端未初始化")
	}

	snapshot := r.collector.GetSnapshot()
	metrics := r.snapshotToMetrics(snapshot)

	batch := &edge.MetricsBatch{
		ProbeId:   r.probeID,
		Timestamp: time.Now().Unix(),
		Metrics:   metrics,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return r.sender.SendMetrics(ctx, batch)
}
