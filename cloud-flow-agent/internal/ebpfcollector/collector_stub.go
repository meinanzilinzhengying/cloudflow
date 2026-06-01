//go:build !linux

package ebpfcollector

import (
	edge "cloud-flow/proto"
)

// Collector eBPF 采集器 (stub implementation for non-CGO builds)
type Collector struct {
	stopCh    chan struct{}
	collectCh chan []*edge.MetricData
}

// New 创建 eBPF 采集器 (stub - 在非 CGO 构建中不可用)
func New() (*Collector, error) {
	return nil, nil
}

// NewWithFallback 创建一个采集器，如果 eBPF 不可用则使用回退方案 (stub)
func NewWithFallback() (*Collector, error) {
	return nil, nil
}

// IsAvailable 检查 eBPF 采集器是否可用 (stub)
func (c *Collector) IsAvailable() bool {
	return false
}

// Start 启动采集器 (stub)
func (c *Collector) Start() {
}

// Stop 停止采集器 (stub)
func (c *Collector) Stop() {
}

// Collect 采集网络流量数据 (stub)
func (c *Collector) Collect() []*edge.MetricData {
	return nil
}
