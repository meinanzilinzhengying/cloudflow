//go:build linux && ebpf
// +build linux

package ebpfcollector

import (
	"log"
	"time"
	
	edge "github.com/meinanzilinzhengying/cloudflow/proto"
)

// TraditionalCollector 传统采集器（不依赖 eBPF）
type TraditionalCollector struct {
	stopCh chan struct{}
}

// NewTraditionalCollector 创建传统采集器
func NewTraditionalCollector() *TraditionalCollector {
	return &TraditionalCollector{
		stopCh: make(chan struct{}),
	}
}

// Start 启动采集器
func (tc *TraditionalCollector) Start() {
	log.Println("[eBPF] 使用传统采集模式")
}

// Stop 停止采集器
func (tc *TraditionalCollector) Stop() {
	close(tc.stopCh)
}

// Collect 采集网络流量数据
func (tc *TraditionalCollector) Collect() []*edge.MetricData {
	// TODO: 实现传统采集逻辑
	// 可以通过以下方式：
	// 1. 使用 pcap 库
	// 2. 使用 netlink
	// 3. 解析 /proc/net/dev
	// 4. 解析 iptables 统计
	
	// 暂时返回空结果
	return nil
}

// IsAvailable 检查是否可用
func (tc *TraditionalCollector) IsAvailable() bool {
	return true
}

// IsTCPMetricsAvailable 检查 TCP 指标
func (tc *TraditionalCollector) IsTCPMetricsAvailable() bool {
	return false
}

// IsHTTPMetricsAvailable 检查 HTTP 指标
func (tc *TraditionalCollector) IsHTTPMetricsAvailable() bool {
	return false
}

// IsHTTPFullAvailable 检查 HTTP 全字段
func (tc *TraditionalCollector) IsHTTPFullAvailable() bool {
	return false
}

// IsDNSFullAvailable 检查 DNS 全字段
func (tc *TraditionalCollector) IsDNSFullAvailable() bool {
	return false
}

// IsMySQLFullAvailable 检查 MySQL 全字段
func (tc *TraditionalCollector) IsMySQLFullAvailable() bool {
	return false
}

// EnhancedFallbackCollector 增强的回退采集器
type EnhancedFallbackCollector struct {
	collector EBPFCollectorInterface
	useEBPF   bool
}

// NewEnhancedFallbackCollector 创建增强回退采集器
func NewEnhancedFallbackCollector(opts *CollectorOptions) (*EnhancedFallbackCollector, error) {
	// 先尝试 eBPF
	collector, err := NewCollector(opts)
	if err == nil && collector != nil {
		return &EnhancedFallbackCollector{
			collector: collector,
			useEBPF: true,
		}, nil
	}
	
	log.Printf("[eBPF] eBPF 不可用: %v，使用传统采集模式", err)
	
	// 降级到传统模式
	return &EnhancedFallbackCollector{
		collector: NewTraditionalCollector(),
		useEBPF: false,
	}, nil
}

// Start 启动采集器
func (efc *EnhancedFallbackCollector) Start() {
	efc.collector.Start()
}

// Stop 停止采集器
func (efc *EnhancedFallbackCollector) Stop() {
	efc.collector.Stop()
}

// Collect 采集数据
func (efc *EnhancedFallbackCollector) Collect() []*edge.MetricData {
	return efc.collector.Collect()
}

// IsAvailable 是否可用
func (efc *EnhancedFallbackCollector) IsAvailable() bool {
	return efc.collector.IsAvailable()
}

// IsTCPMetricsAvailable 检查 TCP 指标
func (efc *EnhancedFallbackCollector) IsTCPMetricsAvailable() bool {
	return efc.collector.IsTCPMetricsAvailable()
}

// IsHTTPMetricsAvailable 检查 HTTP 指标
func (efc *EnhancedFallbackCollector) IsHTTPMetricsAvailable() bool {
	return efc.collector.IsHTTPMetricsAvailable()
}

// IsHTTPFullAvailable 检查 HTTP 全字段
func (efc *EnhancedFallbackCollector) IsHTTPFullAvailable() bool {
	return efc.collector.IsHTTPFullAvailable()
}

// IsDNSFullAvailable 检查 DNS 全字段
func (efc *EnhancedFallbackCollector) IsDNSFullAvailable() bool {
	return efc.collector.IsDNSFullAvailable()
}

// IsMySQLFullAvailable 检查 MySQL 全字段
func (efc *EnhancedFallbackCollector) IsMySQLFullAvailable() bool {
	return efc.collector.IsMySQLFullAvailable()
}

// IsUsingEBPF 是否正在使用 eBPF
func (efc *EnhancedFallbackCollector) IsUsingEBPF() bool {
	return efc.useEBPF
}

// SmartCollector 智能采集器（自动选择 eBPF 或传统模式）
type SmartCollector struct {
	collector EBPFCollectorInterface
	lastError error
}

// NewSmartCollector 创建智能采集器
func NewSmartCollector(opts *CollectorOptions) *SmartCollector {
	collector, err := NewEnhancedFallbackCollector(opts)
	return &SmartCollector{
		collector: collector,
		lastError: err,
	}
}

// GetCollector 获取采集器
func (sc *SmartCollector) GetCollector() EBPFCollectorInterface {
	return sc.collector
}

// GetLastError 获取最后一次错误
func (sc *SmartCollector) GetLastError() error {
	return sc.lastError
}
