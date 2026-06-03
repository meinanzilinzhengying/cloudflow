//go:build linux && cgo

// Package ebpfcollector 提供基于 eBPF 的网络流量采集功能
//
// 本文件实现了基于 libbpf C 加载器的 eBPF 采集器 (LibbpfCollector)，
// 作为 cilium/ebpf Go 实现的替代方案。通过 CGo 直接调用 libbpf C API
// 加载和管理 BPF 程序，适用于需要更细粒度控制或国产芯片适配的场景。
package ebpfcollector

/*
#cgo CFLAGS: -I./bpf
#cgo LDFLAGS: -lbpf -lelf -lz

#include "bpf/libbpf_loader.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"cloudflow-agent/internal/ebpfcollector/parser"
	"cloudflow-agent/internal/kernel"
	edge "cloudflow/proto"
)

// LibbpfCollector 基于 libbpf 的 eBPF 采集器
//
// 通过 CGo 调用 libbpf C API 实现 BPF 程序的加载、挂载和数据采集，
// 提供与 Collector 相同的接口，确保上层调用代码无需修改。
type LibbpfCollector struct {
	ctx             *C.bpf_loader_ctx_t
	stopCh          chan struct{}
	collectCh       chan []*edge.MetricData
	mu              sync.Mutex
	stopped         bool

	// 配置选项
	enableTCPMetrics  bool
	enableHTTPMetrics bool
	enableHTTPFull    bool
	enableDNSFull     bool
	enableMySQLFull   bool
	mgmtIface         string

	// 内核与芯片信息
	arch            kernel.Arch
	vendor          kernel.Vendor
	kernelVersion   string
	kernelMajor     int
	kernelMinor     int
	kernelPatch     int
}

// NewLibbpfCollector 创建基于 libbpf 的 eBPF 采集器
//
// 执行流程：
//  1. 调用 kernel.Detect() 检测内核版本和芯片信息
//  2. 根据架构和芯片类型进行内核版本校验
//  3. 构造 bpf_loader_config_t 并调用 bpf_loader_init() 加载 BPF 程序
//  4. 挂载 TC 和 kprobe 探针
func NewLibbpfCollector(opts *CollectorOptions) (*LibbpfCollector, error) {
	if opts == nil {
		opts = &CollectorOptions{
			EnableTCPMetrics:  true,
			EnableHTTPMetrics: true,
			EnableHTTPFull:    false,
			EnableDNSFull:     false,
			EnableMySQLFull:   false,
		}
	}

	// ---- 1. 内核能力检测 ----
	detector := kernel.NewDetector(nil)
	cap, err := detector.Detect()
	if err != nil {
		return nil, fmt.Errorf("内核能力检测失败: %w", err)
	}

	log.Printf("[libbpf] 内核检测结果: %s", cap.Summary())

	// ---- 2. 内核版本校验 ----
	if err := checkKernelRequirements(cap); err != nil {
		return nil, err
	}

	// ---- 3. 芯片识别日志 ----
	logChipInfo(cap)

	// ---- 4. 构造 libbpf 加载器配置 ----
	cConfig := buildLoaderConfig(opts)

	// ---- 5. 分配 C 上下文并初始化 ----
	ctx := (*C.bpf_loader_ctx_t)(C.malloc(C.sizeof_bpf_loader_ctx_t))
	if ctx == nil {
		return nil, fmt.Errorf("分配 bpf_loader_ctx_t 内存失败")
	}
	// 确保在初始化失败时释放内存
	defer func() {
		if ctx != nil {
			C.free(unsafe.Pointer(ctx))
		}
	}()

	ret := C.bpf_loader_init(ctx, &cConfig)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(ctx))
		return nil, fmt.Errorf("BPF 加载器初始化失败 (ret=%d): %s", ret, errMsg)
	}

	log.Printf("[libbpf] BPF 加载器初始化成功, 已加载 %d 个子系统",
		int(C.bpf_loader_get_subsys_count(ctx)))

	// ---- 6. 挂载 TC 程序 ----
	iface := C.CString(opts.MgmtIface)
	defer C.free(unsafe.Pointer(iface))

	// 挂载 ingress 方向
	ret = C.bpf_loaderattach_tc(ctx, iface, 1)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(ctx))
		log.Printf("[libbpf] 警告: TC ingress 挂载失败 (ret=%d): %s", ret, errMsg)
	} else {
		log.Printf("[libbpf] TC ingress 挂载成功")
	}

	// 挂载 egress 方向
	ret = C.bpf_loaderattach_tc(ctx, iface, 0)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(ctx))
		log.Printf("[libbpf] 警告: TC egress 挂载失败 (ret=%d): %s", ret, errMsg)
	} else {
		log.Printf("[libbpf] TC egress 挂载成功")
	}

	// ---- 7. 挂载 kprobe 探针 ----
	ret = C.bpf_loader_attach_kprobes(ctx)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(ctx))
		log.Printf("[libbpf] 警告: kprobe 挂载失败 (ret=%d): %s", ret, errMsg)
	} else {
		log.Printf("[libbpf] kprobe 挂载成功")
	}

	// ---- 8. 构造采集器实例 ----
	// 初始化成功，将 ctx 的所有权转移给 LibbpfCollector
	// 取消 defer 中的 free，由 LibbpfCollector 管理 ctx 生命周期
	collector := &LibbpfCollector{
		ctx:               ctx,
		stopCh:            make(chan struct{}),
		collectCh:         make(chan []*edge.MetricData, 10),
		enableTCPMetrics:  opts.EnableTCPMetrics,
		enableHTTPMetrics: opts.EnableHTTPMetrics,
		enableHTTPFull:    opts.EnableHTTPFull,
		enableDNSFull:     opts.EnableDNSFull,
		enableMySQLFull:   opts.EnableMySQLFull,
		mgmtIface:         opts.MgmtIface,
		arch:              cap.Arch,
		vendor:            cap.Vendor,
		kernelVersion:     cap.KernelVersion,
		kernelMajor:       cap.KernelMajor,
		kernelMinor:       cap.KernelMinor,
		kernelPatch:       cap.KernelPatch,
	}
	// 阻止 defer 释放 ctx
	ctx = nil

	// 设置 finalizer，防止 Go GC 时泄漏 C 内存
	runtime.SetFinalizer(collector, (*LibbpfCollector).finalizerCleanup)

	return collector, nil
}

// Start 启动采集器
//
// 启动后台采集协程，定期从 BPF Map 中读取数据并通过 collectCh 传递。
func (lc *LibbpfCollector) Start() {
	log.Printf("[libbpf] 采集器启动")
	go lc.collectLoop()
}

// Stop 停止采集器
//
// 停止后台采集协程，卸载所有 BPF 程序并释放 C 资源。
func (lc *LibbpfCollector) Stop() {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.stopped {
		return
	}
	lc.stopped = true

	log.Printf("[libbpf] 正在停止采集器...")

	// 停止采集协程
	close(lc.stopCh)

	// 清理 C 端资源
	if lc.ctx != nil {
		ret := C.bpf_loader_cleanup(lc.ctx)
		if ret != C.BPF_LOADER_OK {
			errMsg := C.GoString(C.bpf_loader_get_last_error(lc.ctx))
			log.Printf("[libbpf] 警告: BPF 资源清理失败 (ret=%d): %s", ret, errMsg)
		}
		C.free(unsafe.Pointer(lc.ctx))
		lc.ctx = nil
	}

	// 清除 finalizer，避免重复释放
	runtime.SetFinalizer(lc, nil)

	log.Printf("[libbpf] 采集器已停止")
}

// Collect 采集网络流量数据
//
// 从 collectCh 中获取最新一批采集数据，超时返回 nil。
// 接口与 Collector.Collect() 保持一致。
func (lc *LibbpfCollector) Collect() []*edge.MetricData {
	select {
	case metrics := <-lc.collectCh:
		return metrics
	case <-time.After(1 * time.Second):
		return nil
	}
}

// IsAvailable 检查采集器是否可用
func (lc *LibbpfCollector) IsAvailable() bool {
	if lc == nil || lc.ctx == nil {
		return false
	}
	return C.bpf_loader_is_subsys_loaded(lc.ctx, C.BPF_SUBSYS_TC) == 1
}

// IsTCPMetricsAvailable 检查 TCP 指标采集是否可用
func (lc *LibbpfCollector) IsTCPMetricsAvailable() bool {
	if lc == nil || lc.ctx == nil {
		return false
	}
	return C.bpf_loader_is_subsys_loaded(lc.ctx, C.BPF_SUBSYS_TCP_METRICS) == 1
}

// IsHTTPMetricsAvailable 检查 HTTP 指标采集是否可用
func (lc *LibbpfCollector) IsHTTPMetricsAvailable() bool {
	if lc == nil || lc.ctx == nil {
		return false
	}
	return C.bpf_loader_is_subsys_loaded(lc.ctx, C.BPF_SUBSYS_HTTP_METRICS) == 1
}

// IsHTTPFullAvailable 检查 HTTP 全字段解析是否可用
func (lc *LibbpfCollector) IsHTTPFullAvailable() bool {
	if lc == nil || lc.ctx == nil {
		return false
	}
	return C.bpf_loader_is_subsys_loaded(lc.ctx, C.BPF_SUBSYS_HTTP_FULL) == 1
}

// IsDNSFullAvailable 检查 DNS 全字段解析是否可用
func (lc *LibbpfCollector) IsDNSFullAvailable() bool {
	if lc == nil || lc.ctx == nil {
		return false
	}
	return C.bpf_loader_is_subsys_loaded(lc.ctx, C.BPF_SUBSYS_DNS_FULL) == 1
}

// IsMySQLFullAvailable 检查 MySQL 全字段解析是否可用
func (lc *LibbpfCollector) IsMySQLFullAvailable() bool {
	if lc == nil || lc.ctx == nil {
		return false
	}
	return C.bpf_loader_is_subsys_loaded(lc.ctx, C.BPF_SUBSYS_MYSQL_FULL) == 1
}

// GetArch 获取系统架构
func (lc *LibbpfCollector) GetArch() kernel.Arch {
	return lc.arch
}

// GetVendor 获取芯片厂商
func (lc *LibbpfCollector) GetVendor() kernel.Vendor {
	return lc.vendor
}

// GetKernelVersion 获取内核版本字符串
func (lc *LibbpfCollector) GetKernelVersion() string {
	return lc.kernelVersion
}

// ---- 内部方法 ----

// collectLoop 采集循环
//
// 每 5 秒从 BPF Map 中读取一次数据，与 Collector.collectLoop() 保持一致。
func (lc *LibbpfCollector) collectLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := lc.collectData()
			if len(metrics) > 0 {
				select {
				case lc.collectCh <- metrics:
				default:
					log.Printf("[libbpf] 警告: collectCh 已满，丢弃本批数据 (%d 条)", len(metrics))
				}
			}
		case <-lc.stopCh:
			return
		}
	}
}

// collectData 从 BPF Map 中采集数据
//
// 采集流程与 Collector.collectData() 保持一致：
//  1. 采集基础网络流量数据 (network_map)
//  2. 采集 TCP 深度指标
//  3. 采集 HTTP 请求指标
//  4. 采集 HTTP 全字段解析数据
//  5. 采集 DNS 全字段解析数据
//  6. 采集 MySQL 全字段解析数据
func (lc *LibbpfCollector) collectData() []*edge.MetricData {
	var metrics []*edge.MetricData
	now := time.Now().Unix()

	// 1. 采集基础流量数据
	networkMetrics := lc.collectNetworkData(now)
	metrics = append(metrics, networkMetrics...)

	// 2. 采集 TCP 深度指标
	if lc.enableTCPMetrics && lc.IsTCPMetricsAvailable() {
		tcpMetrics := lc.collectTCPMetrics(now)
		metrics = append(metrics, tcpMetrics...)
	}

	// 3. 采集 HTTP 请求指标
	if lc.enableHTTPMetrics && lc.IsHTTPMetricsAvailable() {
		httpMetrics := lc.collectHTTPMetrics(now)
		metrics = append(metrics, httpMetrics...)
	}

	// 4. 采集 HTTP 全字段解析数据
	if lc.enableHTTPFull && lc.IsHTTPFullAvailable() {
		httpFullMetrics := lc.collectHTTPFullMetrics(now)
		metrics = append(metrics, httpFullMetrics...)
	}

	// 5. 采集 DNS 全字段解析数据
	if lc.enableDNSFull && lc.IsDNSFullAvailable() {
		dnsFullMetrics := lc.collectDNSFullMetrics(now)
		metrics = append(metrics, dnsFullMetrics...)
	}

	// 6. 采集 MySQL 全字段解析数据
	if lc.enableMySQLFull && lc.IsMySQLFullAvailable() {
		mysqlFullMetrics := lc.collectMySQLFullMetrics(now)
		metrics = append(metrics, mysqlFullMetrics...)
	}

	return metrics
}

// collectNetworkData 采集基础网络流量数据
//
// 通过 C 回调函数遍历 network_map，将每条流记录转换为 MetricData。
func (lc *LibbpfCollector) collectNetworkData(now int64) []*edge.MetricData {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.ctx == nil || lc.stopped {
		return nil
	}

	var metrics []*edge.MetricData
	collector := &networkCallbackCtx{
		metrics: &metrics,
		now:     now,
	}

	// 使用 C 回调函数遍历 network_map
	// 注意：C 回调函数中不能直接调用 Go 代码，
	// 因此通过 exportGoCallback 机制在 C 和 Go 之间桥接
	ret := C.bpf_loader_collect_network(
		lc.ctx,
		(C.map_iterate_callback)(C.network_map_callback),
		unsafe.Pointer(collector),
	)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(lc.ctx))
		log.Printf("[libbpf] 警告: 采集网络数据失败 (ret=%d): %s", ret, errMsg)
	}

	return metrics
}

// collectTCPMetrics 采集 TCP 深度指标
func (lc *LibbpfCollector) collectTCPMetrics(now int64) []*edge.MetricData {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.ctx == nil || lc.stopped {
		return nil
	}

	var metrics []*edge.MetricData

	// 1. 采集全局 TCP 指标
	var globalMetrics C.global_tcp_metrics_t
	ret := C.bpf_loader_lookup_global_metrics(lc.ctx, &globalMetrics)
	if ret == C.BPF_LOADER_OK {
		metric := &edge.MetricData{
			Timestamp: now,
			Protocol:  "tcp_summary",
			Tags: map[string]string{
				"metric_type":           "global_tcp_metrics",
				"total_connections":     fmt.Sprintf("%d", uint64(globalMetrics.total_connections)),
				"failed_connections":    fmt.Sprintf("%d", uint64(globalMetrics.failed_connections)),
				"total_retrans":         fmt.Sprintf("%d", uint64(globalMetrics.total_retrans)),
				"zero_window_events":    fmt.Sprintf("%d", uint64(globalMetrics.zero_window_events)),
				"queue_overflow_events": fmt.Sprintf("%d", uint64(globalMetrics.queue_overflow_events)),
				"avg_latency_ns":        fmt.Sprintf("%d", uint64(globalMetrics.avg_latency_ns)),
				"max_latency_ns":        fmt.Sprintf("%d", uint64(globalMetrics.max_latency_ns)),
				"min_latency_ns":        fmt.Sprintf("%d", uint64(globalMetrics.min_latency_ns)),
				"latency_samples":       fmt.Sprintf("%d", uint64(globalMetrics.latency_samples)),
			},
		}
		metrics = append(metrics, metric)

		// 清零全局指标
		C.bpf_loader_clear_global_metrics(lc.ctx)
	}

	// 2. 采集 TCP 连接级统计
	tcpCollector := &tcpStatsCallbackCtx{
		metrics: &metrics,
		now:     now,
	}
	ret = C.bpf_loader_collect_tcp_metrics(
		lc.ctx,
		(C.map_iterate_callback)(C.tcp_stats_callback),
		unsafe.Pointer(tcpCollector),
	)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(lc.ctx))
		log.Printf("[libbpf] 警告: 采集 TCP 统计数据失败 (ret=%d): %s", ret, errMsg)
	}

	return metrics
}

// collectHTTPMetrics 采集 HTTP 请求指标
func (lc *LibbpfCollector) collectHTTPMetrics(now int64) []*edge.MetricData {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.ctx == nil || lc.stopped {
		return nil
	}

	var metrics []*edge.MetricData

	// 通过通用 Map 遍历接口采集 HTTP 统计
	httpCollector := &genericCallbackCtx{
		metrics:   &metrics,
		now:       now,
		mapName:   "http_stats_map",
		subsys:    C.BPF_SUBSYS_HTTP_METRICS,
		protoType: "http",
	}

	ret := C.bpf_loader_iterate_map(
		lc.ctx,
		C.BPF_SUBSYS_HTTP_METRICS,
		C.CString("http_stats_map"),
		(C.map_iterate_callback)(C.generic_map_callback),
		unsafe.Pointer(httpCollector),
	)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(lc.ctx))
		log.Printf("[libbpf] 警告: 采集 HTTP 指标数据失败 (ret=%d): %s", ret, errMsg)
	}

	return metrics
}

// collectHTTPFullMetrics 采集 HTTP 全字段解析数据
func (lc *LibbpfCollector) collectHTTPFullMetrics(now int64) []*edge.MetricData {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.ctx == nil || lc.stopped {
		return nil
	}

	var metrics []*edge.MetricData

	httpFullCollector := &genericCallbackCtx{
		metrics:   &metrics,
		now:       now,
		mapName:   "http_transactions_map",
		subsys:    C.BPF_SUBSYS_HTTP_FULL,
		protoType: "http_full",
	}

	ret := C.bpf_loader_iterate_map(
		lc.ctx,
		C.BPF_SUBSYS_HTTP_FULL,
		C.CString("http_transactions_map"),
		(C.map_iterate_callback)(C.generic_map_callback),
		unsafe.Pointer(httpFullCollector),
	)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(lc.ctx))
		log.Printf("[libbpf] 警告: 采集 HTTP 全字段数据失败 (ret=%d): %s", ret, errMsg)
	}

	return metrics
}

// collectDNSFullMetrics 采集 DNS 全字段解析数据
func (lc *LibbpfCollector) collectDNSFullMetrics(now int64) []*edge.MetricData {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.ctx == nil || lc.stopped {
		return nil
	}

	var metrics []*edge.MetricData

	dnsCollector := &genericCallbackCtx{
		metrics:   &metrics,
		now:       now,
		mapName:   "dns_queries_map",
		subsys:    C.BPF_SUBSYS_DNS_FULL,
		protoType: "dns_full",
	}

	ret := C.bpf_loader_iterate_map(
		lc.ctx,
		C.BPF_SUBSYS_DNS_FULL,
		C.CString("dns_queries_map"),
		(C.map_iterate_callback)(C.generic_map_callback),
		unsafe.Pointer(dnsCollector),
	)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(lc.ctx))
		log.Printf("[libbpf] 警告: 采集 DNS 全字段数据失败 (ret=%d): %s", ret, errMsg)
	}

	return metrics
}

// collectMySQLFullMetrics 采集 MySQL 全字段解析数据
func (lc *LibbpfCollector) collectMySQLFullMetrics(now int64) []*edge.MetricData {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.ctx == nil || lc.stopped {
		return nil
	}

	var metrics []*edge.MetricData

	mysqlCollector := &genericCallbackCtx{
		metrics:   &metrics,
		now:       now,
		mapName:   "mysql_connections_map",
		subsys:    C.BPF_SUBSYS_MYSQL_FULL,
		protoType: "mysql_full",
	}

	ret := C.bpf_loader_iterate_map(
		lc.ctx,
		C.BPF_SUBSYS_MYSQL_FULL,
		C.CString("mysql_connections_map"),
		(C.map_iterate_callback)(C.generic_map_callback),
		unsafe.Pointer(mysqlCollector),
	)
	if ret != C.BPF_LOADER_OK {
		errMsg := C.GoString(C.bpf_loader_get_last_error(lc.ctx))
		log.Printf("[libbpf] 警告: 采集 MySQL 全字段数据失败 (ret=%d): %s", ret, errMsg)
	}

	return metrics
}

// finalizerCleanup GC finalizer，防止 C 内存泄漏
func (lc *LibbpfCollector) finalizerCleanup() {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.ctx != nil {
		C.bpf_loader_cleanup(lc.ctx)
		C.free(unsafe.Pointer(lc.ctx))
		lc.ctx = nil
	}
}

// ---- 回调上下文结构体 ----

// networkCallbackCtx 网络数据采集回调上下文
type networkCallbackCtx struct {
	metrics *[]*edge.MetricData
	now     int64
}

// tcpStatsCallbackCtx TCP 统计数据采集回调上下文
type tcpStatsCallbackCtx struct {
	metrics *[]*edge.MetricData
	now     int64
}

// genericCallbackCtx 通用 Map 遍历回调上下文
type genericCallbackCtx struct {
	metrics   *[]*edge.MetricData
	now       int64
	mapName   string
	subsys    C.uint32_t
	protoType string
}

// ---- 辅助函数 ----

// checkKernelRequirements 检查内核版本是否满足要求
//
// 根据系统架构和芯片类型执行不同的版本校验策略：
//   - ARM64 (鲲鹏 920): 内核 >= 4.19.90-24
//   - x86_64 (海光 C86): 内核 >= 3.10
//   - x86_64 (其他):     内核 >= 3.10
//   - ARM64 (其他):      内核 >= 4.10
func checkKernelRequirements(cap *kernel.KernelCapability) error {
	switch cap.Arch {
	case kernel.ArchAARCH64:
		if cap.Vendor == kernel.VendorKunpeng {
			// 鲲鹏 920 (ARM64) 要求内核 >= 4.19
			if cap.KernelMajor < 4 || (cap.KernelMajor == 4 && cap.KernelMinor < 19) {
				return fmt.Errorf(
					"鲲鹏 920 (ARM64) 要求内核版本 >= 4.19.90-24，当前内核版本: %s\n"+
						"请升级内核至 4.19.90-24 或更高版本。\n"+
						"升级命令参考:\n"+
						"  CentOS: yum update kernel -y && reboot\n"+
						"  Ubuntu: apt-get install linux-image-$(uname -m) -y && reboot",
					cap.KernelVersion,
				)
			}
			log.Printf("[libbpf] 鲲鹏 920 内核版本校验通过: %s >= 4.19", cap.KernelVersion)
		} else {
			// 其他 ARM64 芯片要求内核 >= 4.10
			if cap.KernelMajor < 4 || (cap.KernelMajor == 4 && cap.KernelMinor < 10) {
				return fmt.Errorf(
					"ARM64 架构要求内核版本 >= 4.10，当前内核版本: %s\n"+
						"请升级内核至 4.10 或更高版本。",
					cap.KernelVersion,
				)
			}
		}

	case kernel.ArchX86_64:
		if cap.Vendor == kernel.VendorHygon {
			// 海光 C86 (x86_64) 要求内核 >= 3.10
			if cap.KernelMajor < 3 || (cap.KernelMajor == 3 && cap.KernelMinor < 10) {
				return fmt.Errorf(
					"海光 C86 (x86_64) 要求内核版本 >= 3.10，当前内核版本: %s\n"+
						"请升级内核至 3.10 或更高版本。",
					cap.KernelVersion,
				)
			}
			log.Printf("[libbpf] 海光 C86 内核版本校验通过: %s >= 3.10", cap.KernelVersion)
		} else {
			// 其他 x86_64 芯片要求内核 >= 3.10
			if cap.KernelMajor < 3 || (cap.KernelMajor == 3 && cap.KernelMinor < 10) {
				return fmt.Errorf(
					"x86_64 架构要求内核版本 >= 3.10，当前内核版本: %s\n"+
						"请升级内核至 3.10 或更高版本。",
					cap.KernelVersion,
				)
			}
		}

	default:
		return fmt.Errorf("不支持的系统架构: %s，仅支持 x86_64 和 aarch64", cap.Arch)
	}

	return nil
}

// logChipInfo 输出芯片识别结果
func logChipInfo(cap *kernel.KernelCapability) {
	switch cap.Vendor {
	case kernel.VendorKunpeng:
		log.Printf("[libbpf] 芯片识别: 鲲鹏 (Kunpeng) 920 - ARM64 架构, implementer=0x48")
	case kernel.VendorHygon:
		log.Printf("[libbpf] 芯片识别: 海光 (Hygon) C86 - x86_64 架构, vendor_id=HygonGenuine")
	case kernel.VendorIntel:
		log.Printf("[libbpf] 芯片识别: Intel - x86_64 架构")
	case kernel.VendorAMD:
		log.Printf("[libbpf] 芯片识别: AMD - x86_64 架构")
	default:
		log.Printf("[libbpf] 芯片识别: 未知厂商, 架构=%s", cap.Arch)
	}
}

// buildLoaderConfig 根据 CollectorOptions 构造 bpf_loader_config_t
func buildLoaderConfig(opts *CollectorOptions) C.bpf_loader_config_t {
	config := C.bpf_loader_get_default_config()

	// 构造子系统启用掩码
	var subsysMask C.uint32_t = C.BPF_SUBSYS_TC // TC 始终启用

	if opts.EnableTCPMetrics {
		subsysMask |= C.BPF_SUBSYS_TCP_METRICS
	}
	if opts.EnableHTTPMetrics {
		subsysMask |= C.BPF_SUBSYS_HTTP_METRICS
	}
	if opts.EnableHTTPFull {
		subsysMask |= C.BPF_SUBSYS_HTTP_FULL
	}
	if opts.EnableDNSFull {
		subsysMask |= C.BPF_SUBSYS_DNS_FULL
	}
	if opts.EnableMySQLFull {
		subsysMask |= C.BPF_SUBSYS_MYSQL_FULL
	}
	config.enabled_subsystems = subsysMask

	// 设置 TC 网卡接口
	if opts.MgmtIface != "" {
		cIface := C.CString(opts.MgmtIface)
		defer C.free(unsafe.Pointer(cIface))
		// 使用 C.strncpy 拷贝到 config 的固定大小数组
		C.strncpy(&config.tc_interface[0], cIface, C.TC_MAX_INTERFACE_NAME-1)
	}

	// 设置日志级别为 INFO
	config.log_level = 3

	// 启用自动清理
	config.auto_cleanup = 1

	return config
}

// parseNetworkEntryFromC 从 C 的 network_map_entry_t 解析 MetricData
//
// 将 C 端传递的 network_map_entry_t 转换为 Go 端的 MetricData，
// 字段映射与 Collector.collectData() 中的 parseNetworkData 保持一致。
func parseNetworkEntryFromC(entry *C.network_map_entry_t, now int64) *edge.MetricData {
	// flow_key_t 使用网络字节序（大端序）
	srcIP := intToIP(uint32(entry.key.src_ip))
	dstIP := intToIP(uint32(entry.key.dst_ip))
	srcPort := uint16(netEndianToHost16(uint16(entry.key.src_port)))
	dstPort := uint16(netEndianToHost16(uint16(entry.key.dst_port)))

	// network_data_t 中的 dst_ip 和 dst_port 也是网络字节序
	dataDstIP := intToIP(uint32(entry.value.dst_ip))
	dataDstPort := uint16(netEndianToHost16(uint16(entry.value.dst_port)))

	// 优先使用 value 中的地址信息
	if dataDstIP != nil && !dataDstIP.IsUnspecified() {
		dstIP = dataDstIP
	}
	if dataDstPort != 0 {
		dstPort = dataDstPort
	}

	// 解析协议
	protocol := "unknown"
	switch uint8(entry.value.protocol) {
	case 6:
		protocol = "tcp"
	case 17:
		protocol = "udp"
	case 1:
		protocol = "icmp"
	}

	// 使用 parser 解析网络数据（与 Collector 保持一致）
	parsedMetric := parser.ParseNetworkData(
		srcIP.String(),
		dstIP.String(),
		srcPort,
		dstPort,
		protocol,
		nil,
	)

	parsedMetric.Timestamp = now
	parsedMetric.Bytes = int64(entry.value.bytes)
	parsedMetric.Packets = int64(entry.value.packets)

	return parsedMetric
}

// parseTCPStatsEntryFromC 从 C 的 tcp_flow_stats_entry_t 解析 MetricData
// 按五元组+进程维度聚合，所有 TCP 指标统一上报
func parseTCPStatsEntryFromC(entry *C.tcp_flow_stats_entry_t, now int64) *edge.MetricData {
	srcIP := intToIP(uint32(entry.key.saddr))
	dstIP := intToIP(uint32(entry.key.daddr))

	// 提取进程名（C 字符串，以 \x00 结尾）
	commBytes := C.GoBytes(unsafe.Pointer(&entry.key.comm[0]), 16)
	comm := strings.TrimRight(string(commBytes), "\x00")

	return &edge.MetricData{
		Timestamp: now,
		SrcIp:     srcIP.String(),
		DstIp:     dstIP.String(),
		SrcPort:   int32(entry.key.sport),
		DstPort:   int32(entry.key.dport),
		Protocol:  "tcp",
		Bytes:     int64(entry.value.bytes_sent),
		Packets:   int64(entry.value.packets_sent),
		Latency:   int64(entry.value.connect_latency_ns),
		Tags: map[string]string{
			"metric_type":          "tcp_flow_stats",
			"pid":                  fmt.Sprintf("%d", uint32(entry.key.pid)),
			"comm":                 comm,
			"retrans_count":        fmt.Sprintf("%d", uint64(entry.value.retrans_count)),
			"zero_window_count":    fmt.Sprintf("%d", uint64(entry.value.zero_window_count)),
			"queue_overflow_count": fmt.Sprintf("%d", uint64(entry.value.queue_overflow_count)),
			"conn_fail_count":      fmt.Sprintf("%d", uint64(entry.value.conn_fail_count)),
			"bytes_recv":           fmt.Sprintf("%d", uint64(entry.value.bytes_recv)),
			"packets_recv":         fmt.Sprintf("%d", uint64(entry.value.packets_recv)),
			"connect_complete":     fmt.Sprintf("%d", uint64(entry.value.connect_complete)),
		},
	}
}

// netEndianToHost16 将网络字节序 16 位值转换为主机字节序
func netEndianToHost16(val uint16) uint16 {
	buf := make([]byte, 2)
	buf[0] = byte(val >> 8)
	buf[1] = byte(val)
	return binary.BigEndian.Uint16(buf)
}

// intToIP 将 uint32 转换为 net.IP
// 注意：与 Collector 中的 intToIP 保持一致，使用大端序
func intToIP(ipInt uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, ipInt)
	return ip
}
