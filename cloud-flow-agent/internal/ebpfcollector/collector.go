//go:build linux

// Package ebpfcollector 提供基于 eBPF 的网络流量采集功能
package ebpfcollector

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"

	"cloudflow-agent/internal/ebpfcollector/bpf"
	"cloudflow-agent/internal/ebpfcollector/parser"
	edge "cloudflow/proto"
)

// 注意：eBPF 程序需要先编译，使用 `make ebpf` 命令

// Collector eBPF 采集器
type Collector struct {
	objs            *bpf.Objects
	tcpMetricsObjs  *bpf.TCPMetricsObjects
	httpMetricsObjs *bpf.HTTPMetricsObjects
	httpFullObjs    *bpf.HTTPFullObjects
	dnsFullObjs     *bpf.DNSFullObjects
	mysqlFullObjs   *bpf.MySQLFullObjects
	links           []link.Link
	tcpLinks        []link.Link
	httpLinks       []link.Link
	httpFullLinks   []link.Link
	dnsFullLinks    []link.Link
	mysqlFullLinks  []link.Link
	stopCh          chan struct{}
	collectCh       chan []*edge.MetricData
	enableTCPMetrics  bool
	enableHTTPMetrics bool
	enableHTTPFull    bool
	enableDNSFull     bool
	enableMySQLFull   bool
	mgmtIface       string // 管理网卡接口
}

// CollectorOptions 采集器配置选项
type CollectorOptions struct {
	EnableTCPMetrics  bool   // 启用TCP深度指标采集
	EnableHTTPMetrics bool   // 启用HTTP请求指标采集
	EnableHTTPFull    bool   // 启用HTTP全字段解析
	EnableDNSFull     bool   // 启用DNS全字段解析
	EnableMySQLFull   bool   // 启用MySQL全字段解析
	MgmtIface        string // 管理网卡接口名称
}

// New 创建 eBPF 采集器
func New() (*Collector, error) {
	return NewWithOptions(&CollectorOptions{
		EnableTCPMetrics:  true,
		EnableHTTPMetrics: true,
		EnableHTTPFull:    false,
		EnableDNSFull:     false,
		EnableMySQLFull:   false,
	})
}

// NewWithOptions 使用选项创建 eBPF 采集器
func NewWithOptions(opts *CollectorOptions) (*Collector, error) {
	if opts == nil {
		opts = &CollectorOptions{
			EnableTCPMetrics:  true,
			EnableHTTPMetrics: true,
			EnableHTTPFull:    false,
			EnableDNSFull:     false,
			EnableMySQLFull:   false,
		}
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("移除内存限制失败: %w", err)
	}

	objs, err := loadBPFObjects()
	if err != nil {
		return nil, fmt.Errorf("加载 eBPF 对象失败: %w", err)
	}

	links, err := attachProgram(objs.TCProg, opts.MgmtIface)
	if err != nil {
		objs.Close()
		return nil, fmt.Errorf("附加 eBPF 程序失败: %w", err)
	}

	collector := &Collector{
		objs:             objs,
		links:            links,
		stopCh:           make(chan struct{}),
		collectCh:        make(chan []*edge.MetricData, 10),
		enableTCPMetrics:  opts.EnableTCPMetrics,
		enableHTTPMetrics: opts.EnableHTTPMetrics,
		enableHTTPFull:    opts.EnableHTTPFull,
		enableDNSFull:     opts.EnableDNSFull,
		enableMySQLFull:   opts.EnableMySQLFull,
		mgmtIface:        opts.MgmtIface,
	}

	// 加载TCP指标eBPF程序
	if opts.EnableTCPMetrics {
		tcpObjs, tcpLinks, err := loadTCPMetricsObjects()
		if err != nil {
			log.Printf("警告: 加载TCP指标eBPF程序失败: %v，将继续使用基础流量采集", err)
		} else {
			collector.tcpMetricsObjs = tcpObjs
			collector.tcpLinks = tcpLinks
			log.Printf("成功加载TCP指标eBPF程序")
		}
	}

	// 加载HTTP指标eBPF程序
	if opts.EnableHTTPMetrics {
		httpObjs, httpLinks, err := loadHTTPMetricsObjects()
		if err != nil {
			log.Printf("警告: 加载HTTP指标eBPF程序失败: %v，将继续使用基础流量采集", err)
		} else {
			collector.httpMetricsObjs = httpObjs
			collector.httpLinks = httpLinks
			log.Printf("成功加载HTTP指标eBPF程序")
		}
	}

	// 加载HTTP全字段解析eBPF程序
	if opts.EnableHTTPFull {
		httpFullObjs, httpFullLinks, err := loadHTTPFullObjects()
		if err != nil {
			log.Printf("警告: 加载HTTP全字段解析eBPF程序失败: %v，将继续使用基础流量采集", err)
		} else {
			collector.httpFullObjs = httpFullObjs
			collector.httpFullLinks = httpFullLinks
			log.Printf("成功加载HTTP全字段解析eBPF程序")
		}
	}

	// 加载DNS全字段解析eBPF程序
	if opts.EnableDNSFull {
		dnsFullObjs, dnsFullLinks, err := loadDNSFullObjects()
		if err != nil {
			log.Printf("警告: 加载DNS全字段解析eBPF程序失败: %v，将继续使用基础流量采集", err)
		} else {
			collector.dnsFullObjs = dnsFullObjs
			collector.dnsFullLinks = dnsFullLinks
			log.Printf("成功加载DNS全字段解析eBPF程序")
		}
	}

	// 加载MySQL全字段解析eBPF程序
	if opts.EnableMySQLFull {
		mysqlFullObjs, mysqlFullLinks, err := loadMySQLFullObjects()
		if err != nil {
			log.Printf("警告: 加载MySQL全字段解析eBPF程序失败: %v，将继续使用基础流量采集", err)
		} else {
			collector.mysqlFullObjs = mysqlFullObjs
			collector.mysqlFullLinks = mysqlFullLinks
			log.Printf("成功加载MySQL全字段解析eBPF程序")
		}
	}

	// 降权为普通用户运行
	if err := dropPrivileges(); err != nil {
		log.Printf("警告: 无法降权: %v，将继续以当前权限运行", err)
	}

	return collector, nil
}

// loadTCPMetricsObjects 加载TCP指标eBPF对象
func loadTCPMetricsObjects() (*bpf.TCPMetricsObjects, []link.Link, error) {
	opts := &ebpf.CollectionOptions{}

	tcpObjs, err := bpf.LoadTCPMetrics(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("加载TCP指标eBPF对象失败: %w", err)
	}

	tcpLinks, err := bpf.AttachTCPMetricsProbes(tcpObjs)
	if err != nil {
		tcpObjs.Close()
		return nil, nil, fmt.Errorf("附加TCP指标kprobe失败: %w", err)
	}

	return tcpObjs, tcpLinks, nil
}

// loadHTTPMetricsObjects 加载HTTP指标eBPF对象
func loadHTTPMetricsObjects() (*bpf.HTTPMetricsObjects, []link.Link, error) {
	opts := &ebpf.CollectionOptions{}

	httpObjs, err := bpf.LoadHTTPMetrics(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("加载HTTP指标eBPF对象失败: %w", err)
	}

	httpLinks, err := bpf.AttachHTTPMetricsProbes(httpObjs)
	if err != nil {
		httpObjs.Close()
		return nil, nil, fmt.Errorf("附加HTTP指标kprobe失败: %w", err)
	}

	return httpObjs, httpLinks, nil
}

// loadHTTPFullObjects 加载HTTP全字段解析eBPF对象
func loadHTTPFullObjects() (*bpf.HTTPFullObjects, []link.Link, error) {
	opts := &ebpf.CollectionOptions{}

	httpFullObjs, err := bpf.LoadHTTPFull(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("加载HTTP全字段解析eBPF对象失败: %w", err)
	}

	httpFullLinks, err := bpf.AttachHTTPFullProbes(httpFullObjs)
	if err != nil {
		httpFullObjs.Close()
		return nil, nil, fmt.Errorf("附加HTTP全字段解析kprobe失败: %w", err)
	}

	return httpFullObjs, httpFullLinks, nil
}

// loadDNSFullObjects 加载DNS全字段解析eBPF对象
func loadDNSFullObjects() (*bpf.DNSFullObjects, []link.Link, error) {
	opts := &ebpf.CollectionOptions{}

	dnsFullObjs, err := bpf.LoadDNSFull(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("加载DNS全字段解析eBPF对象失败: %w", err)
	}

	dnsFullLinks, err := bpf.AttachDNSFullProbes(dnsFullObjs)
	if err != nil {
		dnsFullObjs.Close()
		return nil, nil, fmt.Errorf("附加DNS全字段解析kprobe失败: %w", err)
	}

	return dnsFullObjs, dnsFullLinks, nil
}

// loadMySQLFullObjects 加载MySQL全字段解析eBPF对象
func loadMySQLFullObjects() (*bpf.MySQLFullObjects, []link.Link, error) {
	opts := &ebpf.CollectionOptions{}

	mysqlFullObjs, err := bpf.LoadMySQLFull(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("加载MySQL全字段解析eBPF对象失败: %w", err)
	}

	mysqlFullLinks, err := bpf.AttachMySQLFullProbes(mysqlFullObjs)
	if err != nil {
		mysqlFullObjs.Close()
		return nil, nil, fmt.Errorf("附加MySQL全字段解析kprobe失败: %w", err)
	}

	return mysqlFullObjs, mysqlFullLinks, nil
}

// dropPrivileges 降权为普通用户
func dropPrivileges() error {
	// 尝试使用 "cloud-flow" 用户，如果不存在则使用 "nobody"
	targetUsers := []string{"cloud-flow", "nobody"}
	var pwd syscall.Passwd
	bufSize := 4096
	buf := make([]byte, bufSize)
	var targetUser string
	var err error
	var ptr *syscall.Passwd

	// 尝试获取用户信息
	for _, user := range targetUsers {
		ptr, err = syscall.Getpwnam_r(user, &pwd, buf, int32(len(buf)))
		if err == syscall.ERANGE {
			// 缓冲区不够大，增大后重试
			bufSize *= 2
			buf = make([]byte, bufSize)
			ptr, err = syscall.Getpwnam_r(user, &pwd, buf, int32(len(buf)))
		}
		if err == nil && ptr != nil {
			targetUser = user
			break
		}
	}

	if ptr == nil {
		log.Printf("警告: 目标用户 %v 均不存在，eBPF 采集器将以当前用户运行", targetUsers)
		return nil
	}

	// 先设置组 ID
	if err := syscall.Setgid(int(pwd.Gid)); err != nil {
		return fmt.Errorf("设置组 ID 失败: %w", err)
	}

	// 再设置用户 ID
	if err := syscall.Setuid(int(pwd.Uid)); err != nil {
		return fmt.Errorf("设置用户 ID 失败: %w", err)
	}

	// 验证权限是否已降
	if syscall.Getuid() != int(pwd.Uid) || syscall.Getgid() != int(pwd.Gid) {
		return fmt.Errorf("权限降权验证失败")
	}

	// 验证是否能够读取 /proc 目录
	if _, err := os.ReadDir("/proc"); err != nil {
		log.Printf("警告: 降权后无法读取 /proc 目录: %v，某些功能可能受限", err)
	}

	log.Printf("成功降权为用户 %s (UID: %d, GID: %d)", targetUser, pwd.Uid, pwd.Gid)
	return nil
}

// NewWithFallback 创建一个采集器，如果 eBPF 不可用则使用回退方案
func NewWithFallback() (*Collector, error) {
	collector, err := New()
	if err != nil {
		log.Printf("eBPF 采集器初始化失败: %v，将使用传统采集器作为回退", err)
		return nil, err
	}
	return collector, nil
}

// IsAvailable 检查 eBPF 采集器是否可用
func (c *Collector) IsAvailable() bool {
	return c != nil && c.objs != nil && c.objs.NetworkMap != nil
}

// IsTCPMetricsAvailable 检查TCP指标采集是否可用
func (c *Collector) IsTCPMetricsAvailable() bool {
	return c != nil && c.tcpMetricsObjs != nil
}

// IsHTTPMetricsAvailable 检查HTTP指标采集是否可用
func (c *Collector) IsHTTPMetricsAvailable() bool {
	return c != nil && c.httpMetricsObjs != nil
}

// IsHTTPFullAvailable 检查HTTP全字段解析是否可用
func (c *Collector) IsHTTPFullAvailable() bool {
	return c != nil && c.httpFullObjs != nil
}

// IsDNSFullAvailable 检查DNS全字段解析是否可用
func (c *Collector) IsDNSFullAvailable() bool {
	return c != nil && c.dnsFullObjs != nil
}

// IsMySQLFullAvailable 检查MySQL全字段解析是否可用
func (c *Collector) IsMySQLFullAvailable() bool {
	return c != nil && c.mysqlFullObjs != nil
}

// Start 启动采集器
func (c *Collector) Start() {
	go c.collectLoop()
}

// Stop 停止采集器
func (c *Collector) Stop() {
	close(c.stopCh)
	for _, l := range c.links {
		l.Close()
	}
	for _, l := range c.tcpLinks {
		l.Close()
	}
	for _, l := range c.httpLinks {
		l.Close()
	}
	for _, l := range c.httpFullLinks {
		l.Close()
	}
	for _, l := range c.dnsFullLinks {
		l.Close()
	}
	for _, l := range c.mysqlFullLinks {
		l.Close()
	}
	if c.tcpMetricsObjs != nil {
		c.tcpMetricsObjs.Close()
	}
	if c.httpMetricsObjs != nil {
		c.httpMetricsObjs.Close()
	}
	if c.httpFullObjs != nil {
		c.httpFullObjs.Close()
	}
	if c.dnsFullObjs != nil {
		c.dnsFullObjs.Close()
	}
	if c.mysqlFullObjs != nil {
		c.mysqlFullObjs.Close()
	}
	if c.objs != nil {
		c.objs.Close()
	}
}

// Collect 采集网络流量数据
func (c *Collector) Collect() []*edge.MetricData {
	select {
	case metrics := <-c.collectCh:
		return metrics
	case <-time.After(1 * time.Second):
		return nil
	}
}

// EBPFCollectorInterface eBPF 采集器统一接口
// 支持多种后端实现（libbpf / cilium-ebpf）
type EBPFCollectorInterface interface {
	Start()
	Stop()
	Collect() []*edge.MetricData
	IsAvailable() bool
	IsTCPMetricsAvailable() bool
	IsHTTPMetricsAvailable() bool
}

// 确保 Collector 实现接口
var _ EBPFCollectorInterface = (*Collector)(nil)

// 确保 LibbpfCollector 实现接口（编译时检查）
var _ EBPFCollectorInterface = (*LibbpfCollector)(nil)

// BackendType eBPF 后端类型
type BackendType string

const (
	BackendLibbpf   BackendType = "libbpf"   // libbpf C 后端（推荐，跨架构兼容）
	BackendCilium   BackendType = "cilium"   // cilium/ebpf Go 后端（原有实现）
	BackendAuto     BackendType = "auto"     // 自动选择
)

// NewCollector 创建 eBPF 采集器（统一入口）
// 根据环境变量 CLOUD_FLOW_BPF_BACKEND 或自动检测结果选择后端：
//   - "libbpf": 使用 libbpf C 后端（推荐，支持鲲鹏920/海光C86）
//   - "cilium": 使用 cilium/ebpf Go 后端（原有实现）
//   - "auto" 或未设置: 自动选择（优先 libbpf）
func NewCollector(opts *CollectorOptions) (EBPFCollectorInterface, error) {
	backend := BackendType(os.Getenv("CLOUD_FLOW_BPF_BACKEND"))
	if backend == "" {
		backend = BackendAuto
	}

	switch backend {
	case BackendLibbpf:
		return NewLibbpfCollector(opts)
	case BackendCilium:
		return NewWithOptions(opts)
	case BackendAuto:
		// 优先尝试 libbpf 后端
		coll, err := NewLibbpfCollector(opts)
		if err != nil {
			log.Printf("[eBPF] libbpf 后端初始化失败: %v，回退到 cilium/ebpf 后端", err)
			return NewWithOptions(opts)
		}
		return coll, nil
	default:
		return nil, fmt.Errorf("未知的 eBPF 后端类型: %s（可选: libbpf, cilium, auto）", backend)
	}
}

// collectLoop 采集循环
func (c *Collector) collectLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := c.collectData()
			if len(metrics) > 0 {
				select {
				case c.collectCh <- metrics:
				default:
				}
			}
		case <-c.stopCh:
			return
		}
	}
}

// collectData 从 eBPF map 中采集数据
func (c *Collector) collectData() []*edge.MetricData {
	var metrics []*edge.MetricData
	now := time.Now().Unix()

	// 采集基础流量数据
	iter := c.objs.NetworkMap.Iterate()
	var key, value []byte
	for iter.Next(&key, &value) {
		flow := parseNetworkData(key, value)
		if flow == nil {
			continue
		}

		parsedMetric := parser.ParseNetworkData(
			flow.SrcIP,
			flow.DstIP,
			flow.SrcPort,
			flow.DstPort,
			flow.Protocol,
			value,
		)

		parsedMetric.Timestamp = now
		parsedMetric.Bytes = flow.Bytes
		parsedMetric.Packets = flow.Packets

		metrics = append(metrics, parsedMetric)

		c.objs.NetworkMap.Delete(key)
	}

	// 采集TCP深度指标
	if c.tcpMetricsObjs != nil {
		tcpMetrics := c.collectTCPMetrics(now)
		metrics = append(metrics, tcpMetrics...)
	}

	// 采集HTTP请求指标
	if c.httpMetricsObjs != nil {
		httpMetrics := c.collectHTTPMetrics(now)
		metrics = append(metrics, httpMetrics...)
	}

	// 采集HTTP全字段解析数据
	if c.httpFullObjs != nil {
		httpFullMetrics := c.collectHTTPFullMetrics(now)
		metrics = append(metrics, httpFullMetrics...)
	}

	// 采集DNS全字段解析数据
	if c.dnsFullObjs != nil {
		dnsFullMetrics := c.collectDNSFullMetrics(now)
		metrics = append(metrics, dnsFullMetrics...)
	}

	// 采集MySQL全字段解析数据
	if c.mysqlFullObjs != nil {
		mysqlFullMetrics := c.collectMySQLFullMetrics(now)
		metrics = append(metrics, mysqlFullMetrics...)
	}

	return metrics
}

// collectTCPMetrics 采集TCP深度指标
func (c *Collector) collectTCPMetrics(now int64) []*edge.MetricData {
	var metrics []*edge.MetricData

	// 阶段 1：遍历 tcp_flow_stats_map，为每个条目生成一条 MetricData
	flowIter := c.tcpMetricsObjs.IterateTcpFlowStats()
	var flowKey bpf.TcpConnKey
	var flowStats bpf.TcpFlowStats
	for flowIter.Next(&flowKey, &flowStats) {
		metric := &edge.MetricData{
			Timestamp: now,
			SrcIp:     intToIP(flowKey.Saddr).String(),
			DstIp:     intToIP(flowKey.Daddr).String(),
			SrcPort:   int32(flowKey.Sport),
			DstPort:   int32(flowKey.Dport),
			Protocol:  "tcp",
			Bytes:     int64(flowStats.BytesSent),
			Packets:   int64(flowStats.PacketsSent),
			Latency:   int64(flowStats.ConnectLatencyNs),
			Tags: map[string]string{
				"metric_type":         "tcp_flow_stats",
				"pid":                 strconv.FormatUint(uint64(flowKey.Pid), 10),
				"comm":                string(bytes.TrimRight(flowKey.Comm[:], "\x00")),
				"retrans_count":       strconv.FormatUint(flowStats.RetransCount, 10),
				"zero_window_count":   strconv.FormatUint(flowStats.ZeroWindowCount, 10),
				"queue_overflow_count": strconv.FormatUint(flowStats.QueueOverflowCount, 10),
				"conn_fail_count":     strconv.FormatUint(flowStats.ConnFailCount, 10),
				"bytes_recv":          strconv.FormatUint(flowStats.BytesRecv, 10),
				"packets_recv":        strconv.FormatUint(flowStats.PacketsRecv, 10),
				"connect_complete":    strconv.FormatUint(uint64(flowStats.ConnectComplete), 10),
			},
		}
		metrics = append(metrics, metric)

		// 遍历后删除已读取的条目，避免重复采集
		_ = c.tcpMetricsObjs.TcpFlowStatsMap.Delete(&flowKey)
	}

	// 阶段 2：读取 global_tcp_metrics_map 全局汇总
	globalMetrics, err := c.tcpMetricsObjs.GetGlobalTCPMetrics()
	if err == nil && globalMetrics != nil {
		metric := &edge.MetricData{
			Timestamp: now,
			Protocol:  "tcp_summary",
			Tags: map[string]string{
				"metric_type":           "global_tcp_metrics",
				"total_connections":     fmt.Sprintf("%d", globalMetrics.TotalConnections),
				"failed_connections":    fmt.Sprintf("%d", globalMetrics.FailedConnections),
				"total_retrans":         fmt.Sprintf("%d", globalMetrics.TotalRetrans),
				"zero_window_events":    fmt.Sprintf("%d", globalMetrics.ZeroWindowEvents),
				"queue_overflow_events": fmt.Sprintf("%d", globalMetrics.QueueOverflowEvents),
				"avg_latency_ns":        fmt.Sprintf("%d", globalMetrics.AvgLatencyNs),
				"max_latency_ns":        fmt.Sprintf("%d", globalMetrics.MaxLatencyNs),
				"min_latency_ns":        fmt.Sprintf("%d", globalMetrics.MinLatencyNs),
				"latency_samples":       fmt.Sprintf("%d", globalMetrics.LatencySamples),
			},
		}
		metrics = append(metrics, metric)
	}

	return metrics
}

// collectHTTPMetrics 采集HTTP请求指标
func (c *Collector) collectHTTPMetrics(now int64) []*edge.MetricData {
	var metrics []*edge.MetricData

	// 1. 采集全局HTTP指标(请求成功率、响应时延、异常比例)
	globalMetrics, err := c.httpMetricsObjs.GetGlobalHTTPMetrics()
	if err == nil && globalMetrics != nil {
		// 计算成功率
		successRate := float64(0)
		if globalMetrics.TotalResponses > 0 {
			successRate = float64(globalMetrics.SuccessResponses) / float64(globalMetrics.TotalResponses) * 100
		}

		// 计算异常比例
		errorRate := float64(0)
		if globalMetrics.TotalResponses > 0 {
			errorRate = float64(globalMetrics.ErrorResponses) / float64(globalMetrics.TotalResponses) * 100
		}

		metric := &edge.MetricData{
			Timestamp: now,
			Protocol:  "http",
			Tags: map[string]string{
				"metric_type":            "global_http_metrics",
				"total_requests":          fmt.Sprintf("%d", globalMetrics.TotalRequests),
				"total_responses":        fmt.Sprintf("%d", globalMetrics.TotalResponses),
				"success_count":          fmt.Sprintf("%d", globalMetrics.SuccessResponses),
				"error_count":            fmt.Sprintf("%d", globalMetrics.ErrorResponses),
				"success_rate":           fmt.Sprintf("%.2f", successRate),
				"error_rate":             fmt.Sprintf("%.2f", errorRate),
				"avg_latency_ns":         fmt.Sprintf("%d", globalMetrics.AvgLatencyNs),
				"avg_latency_us":         fmt.Sprintf("%.2f", float64(globalMetrics.AvgLatencyNs)/1000),
				"max_latency_ns":         fmt.Sprintf("%d", globalMetrics.MaxLatencyNs),
				"max_latency_us":         fmt.Sprintf("%.2f", float64(globalMetrics.MaxLatencyNs)/1000),
				"min_latency_ns":         fmt.Sprintf("%d", globalMetrics.MinLatencyNs),
				"min_latency_us":         fmt.Sprintf("%.2f", float64(globalMetrics.MinLatencyNs)/1000),
				"latency_samples":        fmt.Sprintf("%d", globalMetrics.LatencySamples),
			},
		}
		metrics = append(metrics, metric)
	}

	// 2. 采集HTTP统计指标(按连接维度)
	statsIter := c.httpMetricsObjs.IterateHttpStats()
	var statsKey bpf.HttpFlowKey
	var statsValue bpf.HttpStats
	for statsIter.Next(&statsKey, &statsValue) {
		// 计算成功率
		successRate := float64(0)
		if statsValue.ResponseCount > 0 {
			successRate = float64(statsValue.SuccessCount) / float64(statsValue.ResponseCount) * 100
		}

		// 计算异常比例
		errorRate := float64(0)
		if statsValue.ResponseCount > 0 {
			errorRate = float64(statsValue.ErrorCount) / float64(statsValue.ResponseCount) * 100
		}

		metric := &edge.MetricData{
			Timestamp: now,
			SrcIp:     intToIP(statsKey.Saddr).String(),
			DstIp:     intToIP(statsKey.Daddr).String(),
			SrcPort:   int32(statsKey.Sport),
			DstPort:   int32(statsKey.Dport),
			Protocol:  "http",
			Tags: map[string]string{
				"metric_type":   "http_connection_stats",
				"request_count":  fmt.Sprintf("%d", statsValue.RequestCount),
				"response_count": fmt.Sprintf("%d", statsValue.ResponseCount),
				"success_count":  fmt.Sprintf("%d", statsValue.SuccessCount),
				"error_count":    fmt.Sprintf("%d", statsValue.ErrorCount),
				"success_rate":   fmt.Sprintf("%.2f", successRate),
				"error_rate":     fmt.Sprintf("%.2f", errorRate),
				"avg_latency_ns": fmt.Sprintf("%d", statsValue.AvgLatencyNs),
				"avg_latency_us": fmt.Sprintf("%.2f", float64(statsValue.AvgLatencyNs)/1000),
				"max_latency_ns": fmt.Sprintf("%d", statsValue.MaxLatencyNs),
				"max_latency_us": fmt.Sprintf("%.2f", float64(statsValue.MaxLatencyNs)/1000),
				"min_latency_ns": fmt.Sprintf("%d", statsValue.MinLatencyNs),
				"min_latency_us": fmt.Sprintf("%.2f", float64(statsValue.MinLatencyNs)/1000),
				"total_request_bytes":  fmt.Sprintf("%d", statsValue.TotalRequestBytes),
				"total_response_bytes": fmt.Sprintf("%d", statsValue.TotalResponseBytes),
				"pid":           fmt.Sprintf("%d", statsKey.Pid),
			},
		}
		metrics = append(metrics, metric)
	}

	// 3. 采集异常状态码统计
	errorIter := c.httpMetricsObjs.IterateErrorStats()
	var statusCode uint16
	var errorCount uint64
	for errorIter.Next(&statusCode, &errorCount) {
		metric := &edge.MetricData{
			Timestamp: now,
			Protocol:  "http",
			Tags: map[string]string{
				"metric_type": "http_error_stats",
				"status_code": fmt.Sprintf("%d", statusCode),
				"error_count": fmt.Sprintf("%d", errorCount),
			},
		}
		metrics = append(metrics, metric)
	}

	return metrics
}

// collectHTTPFullMetrics 采集HTTP全字段解析数据
func (c *Collector) collectHTTPFullMetrics(now int64) []*edge.MetricData {
	var metrics []*edge.MetricData

	// 1. 采集HTTP统计信息
	httpStats, err := c.httpFullObjs.GetHTTPStats()
	if err == nil && httpStats != nil {
		metric := &edge.MetricData{
			Timestamp: now,
			Protocol:  "http_full",
			Tags: map[string]string{
				"metric_type":     "http_full_stats",
				"total_requests":  fmt.Sprintf("%d", httpStats.TotalRequests),
				"total_responses": fmt.Sprintf("%d", httpStats.TotalResponses),
				"success_count":   fmt.Sprintf("%d", httpStats.SuccessCount),
				"error_count":     fmt.Sprintf("%d", httpStats.ErrorCount),
				"avg_latency_ns":  fmt.Sprintf("%d", httpStats.AvgLatencyNs),
				"max_latency_ns":  fmt.Sprintf("%d", httpStats.MaxLatencyNs),
				"min_latency_ns":  fmt.Sprintf("%d", httpStats.MinLatencyNs),
			},
		}
		metrics = append(metrics, metric)
	}

	// 2. 采集HTTP事务数据
	txnIter := c.httpFullObjs.IterateHTTPTransactions()
	var txnKey bpf.HTTPConnKey
	var txnValue bpf.HTTPTransaction
	for txnIter.Next(&txnKey, &txnValue) {
		if txnValue.Complete == 0 {
			continue
		}

		req := &txnValue.Request
		resp := &txnValue.Response

		metric := &edge.MetricData{
			Timestamp: now,
			SrcIp:     intToIP(txnKey.Saddr).String(),
			DstIp:     intToIP(txnKey.Daddr).String(),
			SrcPort:   int32(txnKey.Sport),
			DstPort:   int32(txnKey.Dport),
			Protocol:  "http_full",
			Latency:   int64(resp.LatencyNs),
			Tags: map[string]string{
				"metric_type":       "http_full_transaction",
				"request_id":        fmt.Sprintf("%d", req.RequestId),
				"method":            req.Method.GetMethodName(),
				"path":              req.GetPath(),
				"host":              req.GetHost(),
				"user_agent":        req.GetUserAgent(),
				"referer":           req.GetReferer(),
				"content_type":      req.GetContentType(),
				"content_length":    fmt.Sprintf("%d", req.ContentLength),
				"http_version":      req.GetHttpVersion(),
				"is_https":          fmt.Sprintf("%v", req.IsHttps == 1),
				"status_code":       fmt.Sprintf("%d", resp.StatusCode),
				"status_text":       resp.GetStatusText(),
				"response_content_type": resp.GetResponseContentType(),
				"response_content_length": fmt.Sprintf("%d", resp.ContentLength),
				"server":            resp.GetServer(),
				"is_chunked":        fmt.Sprintf("%v", resp.IsChunked == 1),
			"is_gzipped":        fmt.Sprintf("%v", resp.IsGzipped == 1),
			"is_cached":         fmt.Sprintf("%v", resp.IsCached == 1),
			"latency_us":        fmt.Sprintf("%d", resp.LatencyNs/1000),
			"pid":               fmt.Sprintf("%d", txnKey.Pid),
			"x_forwarded_for":   req.GetXForwardedFor(),
			"x_real_ip":         req.GetXRealIP(),
		},
	}
		metrics = append(metrics, metric)

		// 从map中删除已处理的事务
		c.httpFullObjs.HttpTransactionsMap.Delete(txnKey)
	}

	return metrics
}

// collectDNSFullMetrics 采集DNS全字段解析数据
func (c *Collector) collectDNSFullMetrics(now int64) []*edge.MetricData {
	var metrics []*edge.MetricData

	// 1. 采集DNS统计信息
	// 查询总数
	queryCount, _ := c.dnsFullObjs.GetDNSStats(0)
	metric := &edge.MetricData{
		Timestamp: now,
		Protocol:  "dns_full",
		Tags: map[string]string{
			"metric_type":  "dns_full_stats",
			"query_count":  fmt.Sprintf("%d", queryCount),
		},
	}
	metrics = append(metrics, metric)

	// 2. 采集各响应码统计
	for rcode := uint8(0); rcode <= 5; rcode++ {
		count, err := c.dnsFullObjs.GetDNSStats(uint32(rcode + 1))
		if err == nil && count > 0 {
			metric := &edge.MetricData{
				Timestamp: now,
				Protocol:  "dns_full",
				Tags: map[string]string{
					"metric_type": "dns_rcode_stats",
					"rcode":       bpf.GetRcodeName(rcode),
					"rcode_value": fmt.Sprintf("%d", rcode),
					"count":       fmt.Sprintf("%d", count),
				},
			}
			metrics = append(metrics, metric)
		}
	}

	// 3. 采集DNS查询数据
	queryIter := c.dnsFullObjs.IterateDNSQueries()
	var queryKey bpf.DNSConnKey
	var queryValue bpf.DNSRequestFull
	for queryIter.Next(&queryKey, &queryValue) {
		// 获取主查询问题
		primaryQuestion := queryValue.GetPrimaryQuestion()
		queryName := ""
		queryType := ""
		if primaryQuestion != nil {
			queryName = primaryQuestion.GetQName()
			queryType = bpf.GetRecordTypeName(primaryQuestion.Qtype)
		}

		metric := &edge.MetricData{
			Timestamp: now,
			SrcIp:     intToIP(queryKey.Saddr).String(),
			DstIp:     intToIP(queryKey.Daddr).String(),
			SrcPort:   int32(queryKey.Sport),
			DstPort:   int32(queryKey.Dport),
			Protocol:  "dns_full",
			Tags: map[string]string{
				"metric_type":       "dns_query",
				"transaction_id":    fmt.Sprintf("%d", queryValue.TransactionId),
				"query_name":        queryName,
				"query_type":        queryType,
				"query_class":       fmt.Sprintf("%d", primaryQuestion.Qclass),
				"opcode":            fmt.Sprintf("%d", queryValue.Opcode),
				"recursion_desired": fmt.Sprintf("%v", queryValue.RecursionDesired == 1),
				"question_count":    fmt.Sprintf("%d", queryValue.QuestionCount),
				"pid":               fmt.Sprintf("%d", queryKey.Pid),
			},
		}
		metrics = append(metrics, metric)
	}

	return metrics
}

// collectMySQLFullMetrics 采集MySQL全字段解析数据
func (c *Collector) collectMySQLFullMetrics(now int64) []*edge.MetricData {
	var metrics []*edge.MetricData

	// 1. 采集MySQL命令统计
	for cmd := uint8(0); cmd < 29; cmd++ {
		count, err := c.mysqlFullObjs.GetCommandStats(uint32(cmd))
		if err == nil && count > 0 {
			metric := &edge.MetricData{
				Timestamp: now,
				Protocol:  "mysql_full",
				Tags: map[string]string{
					"metric_type":  "mysql_cmd_stats",
					"command":      bpf.GetCommandName(cmd),
					"command_code": fmt.Sprintf("%d", cmd),
					"count":        fmt.Sprintf("%d", count),
				},
			}
			metrics = append(metrics, metric)
		}
	}

	// 2. 采集MySQL错误统计
	errIter := c.mysqlFullObjs.IterateErrorStats()
	var errCode uint16
	var errCount uint64
	for errIter.Next(&errCode, &errCount) {
		metric := &edge.MetricData{
			Timestamp: now,
			Protocol:  "mysql_full",
			Tags: map[string]string{
				"metric_type": "mysql_error_stats",
				"error_code":  fmt.Sprintf("%d", errCode),
				"error_name":  bpf.GetErrorName(errCode),
				"count":       fmt.Sprintf("%d", errCount),
			},
		}
		metrics = append(metrics, metric)
	}

	// 3. 采集MySQL连接数据
	connIter := c.mysqlFullObjs.IterateMySQLConnections()
	var connKey bpf.MySQLConnKey
	var connValue bpf.MySQLTransaction
	for connIter.Next(&connKey, &connValue) {
		if connValue.Complete == 0 {
			continue
		}

		cmd := &connValue.Command
		resp := &connValue.Response
		auth := &connValue.Auth

		metric := &edge.MetricData{
			Timestamp: now,
			SrcIp:     intToIP(connKey.Saddr).String(),
			DstIp:     intToIP(connKey.Daddr).String(),
			SrcPort:   int32(connKey.Sport),
			DstPort:   int32(connKey.Dport),
			Protocol:  "mysql_full",
			Latency:   int64(resp.LatencyNs),
			Tags: map[string]string{
				"metric_type":      "mysql_transaction",
				"command":          bpf.GetCommandName(cmd.Command),
				"sql_type":         cmd.GetSQLType(),
				"sql_preview":      cmd.GetSQL(),
				"database":         auth.GetDatabase(),
				"username":         auth.GetUsername(),
				"server_version":   connValue.Handshake.GetServerVersion(),
				"packet_type":      bpf.GetPacketTypeName(resp.PacketType),
				"is_error":         fmt.Sprintf("%v", resp.IsErrorResponse()),
				"error_code":       fmt.Sprintf("%d", resp.ErrorCode),
				"error_message":    resp.GetErrorMessage(),
				"affected_rows":    fmt.Sprintf("%d", resp.AffectedRows),
				"last_insert_id":   fmt.Sprintf("%d", resp.LastInsertId),
				"field_count":      fmt.Sprintf("%d", resp.FieldCount),
				"row_count":        fmt.Sprintf("%d", resp.RowCount),
				"warnings":         fmt.Sprintf("%d", resp.Warnings),
				"latency_us":       fmt.Sprintf("%d", resp.LatencyNs/1000),
				"pid":              fmt.Sprintf("%d", connKey.Pid),
			},
		}
		metrics = append(metrics, metric)

		// 从map中删除已处理的连接
		c.mysqlFullObjs.MySQLConnectionsMap.Delete(connKey)
	}

	return metrics
}

// intToIP 将uint32转换为net.IP
func intToIP(ipInt uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, ipInt)
	return ip
}

// NetworkFlow 网络流量数据
type NetworkFlow struct {
	SrcIP     string
	DstIP     string
	SrcPort   uint16
	DstPort   uint16
	Protocol  string
	Bytes     int64
	Packets   int64
	Timestamp int64
}

// BPF 数据长度常量
const (
	bpfKeySize   = 12
	bpfValueSize = 31
)

// parseNetworkData 解析网络流量数据
func parseNetworkData(key, value []byte) *NetworkFlow {
	// BPF 实际数据结构:
	// key: 12 字节 (4-byte src_ip + 4-byte dst_ip + 2-byte src_port + 2-byte dst_port)
	// value: 31 字节 (4-byte dst_ip + 2-byte dst_port + 1-byte protocol + 8-byte bytes + 8-byte packets + 8-byte timestamp)

	var srcIP net.IP
	var srcPort uint16
	var dstIP net.IP
	var dstPort uint16
	var protocol string
	var bytes, packets, timestamp int64

	// 检查 key 长度（使用常量，便于版本兼容）
	if len(key) != bpfKeySize {
		log.Printf("警告: 无效的 key 长度: %d (期望 %d)，可能是 BPF 程序版本不匹配", len(key), bpfKeySize)
		return nil
	}

	// 解析源 IP 和端口
	// key 格式: src_ip(4) + dst_ip(4) + src_port(2) + dst_port(2)
	srcIP = net.IP(key[:4])
	srcPort = binary.BigEndian.Uint16(key[8:10])

	// 检查 value 长度（使用常量，便于版本兼容）
	if len(value) != bpfValueSize {
		log.Printf("警告: 无效的 value 长度: %d (期望 %d)，可能是 BPF 程序版本不匹配", len(value), bpfValueSize)
		return nil
	}

	// 解析目标 IP、端口、协议和统计数据
	dstIP = net.IP(value[:4])
	dstPort = binary.BigEndian.Uint16(value[4:6])

	// 解析协议
	protocol = "unknown"
	switch value[6] {
	case 6:
		protocol = "tcp"
	case 17:
		protocol = "udp"
	case 1:
		protocol = "icmp"
	}

	// 解析统计数据
	bytes = int64(binary.BigEndian.Uint64(value[7:15]))
	packets = int64(binary.BigEndian.Uint64(value[15:23]))
	timestamp = int64(binary.BigEndian.Uint64(value[23:31]))

	return &NetworkFlow{
		SrcIP:     srcIP.String(),
		DstIP:     dstIP.String(),
		SrcPort:   srcPort,
		DstPort:   dstPort,
		Protocol:  protocol,
		Bytes:     bytes,
		Packets:   packets,
		Timestamp: timestamp,
	}
}

// loadBPFObjects 加载 eBPF 对象
func loadBPFObjects() (*bpf.Objects, error) {
	// 1. 首先尝试从文件系统加载（优先使用本地编译的版本）
	ebpfFile := findEBPFFile()
	if ebpfFile != "" {
		spec, err := ebpf.LoadCollectionSpec(ebpfFile)
		if err != nil {
			log.Printf("从文件 %s 加载 eBPF spec 失败: %v，尝试使用嵌入版本", ebpfFile, err)
		} else {
			objs := &bpf.Objects{}
			if err := spec.LoadAndAssign(objs, nil); err != nil {
				log.Printf("从文件 %s 加载 eBPF 对象失败: %v，尝试使用嵌入版本", ebpfFile, err)
			} else {
				log.Printf("成功从文件 %s 加载 eBPF 对象", ebpfFile)
				return objs, nil
			}
		}
	}

	// 2. 检查嵌入的 BPF 程序是否可用
	if err := bpf.CheckRequiredGoFiles(); err != nil {
		return nil, fmt.Errorf("eBPF 程序未嵌入，请运行 'make ebpf' 命令编译 tc.bpf.c: %w", err)
	}

	// 3. 尝试加载嵌入的 BPF 对象
	objs := &bpf.Objects{}
	if err := objs.Load(nil); err != nil {
		// 检查是否是内核兼容性问题
		if strings.Contains(err.Error(), "invalid bpf program") ||
			strings.Contains(err.Error(), "kernel version") ||
			strings.Contains(err.Error(), "BTF") {
			return nil, fmt.Errorf("eBPF 程序与当前内核不兼容: %w\n"+
				"请在目标系统上运行 'make ebpf' 重新编译 BPF 程序，确保与当前内核版本匹配", err)
		}
		return nil, fmt.Errorf("加载 eBPF 对象失败: %w", err)
	}
	log.Printf("成功加载嵌入的 eBPF 对象")
	return objs, nil
}

// findEBPFFile 查找 eBPF 目标文件
func findEBPFFile() string {
	searchPaths := []string{
		"bpf/tc.bpf.o",
		"internal/ebpfcollector/bpf/tc.bpf.o",
		"/etc/cloudflow-agent/bpf/tc.bpf.o",
		"/usr/share/cloudflow-agent/bpf/tc.bpf.o",
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	execPath, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(execPath)
		ebpfFile := filepath.Join(dir, "bpf", "tc.bpf.o")
		if _, err := os.Stat(ebpfFile); err == nil {
			return ebpfFile
		}
	}

	return ""
}

// attachProgram 附加 eBPF 程序到网络设备
func attachProgram(prog *ebpf.Program, mgmtIface string) ([]link.Link, error) {
	devices, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("获取网络设备失败: %w", err)
	}

	var links []link.Link
	for _, dev := range devices {
		// 跳过回环接口
		if dev.Name == "lo" {
			continue
		}

		// 如果指定了管理网卡,只附加到管理网卡
		if mgmtIface != "" && dev.Name != mgmtIface {
			log.Printf("跳过非管理网卡 %s", dev.Name)
			continue
		}

		// 检查设备是否有有效的索引
		if dev.Index <= 0 {
			log.Printf("设备 %s 索引无效: %d", dev.Name, dev.Index)
			continue
		}

		// 尝试附加到设备的入站方向
		l, err := link.AttachTC(link.TCOptions{
			Interface: dev.Index,
			Direction: link.TCIngress,
			Program:   prog,
		})
		if err != nil {
			log.Printf("附加到设备 %s 入站方向失败: %v", dev.Name, err)
			continue
		}
		links = append(links, l)

		// 尝试附加到设备的出站方向
		l, err = link.AttachTC(link.TCOptions{
			Interface: dev.Index,
			Direction: link.TCEgress,
			Program:   prog,
		})
		if err != nil {
			log.Printf("附加到设备 %s 出站方向失败: %v", dev.Name, err)
			continue
		}
		links = append(links, l)
	}

	if len(links) == 0 {
		log.Printf("警告: 未能附加到任何网络设备")
	}

	return links, nil
}

// GetMap 获取指定的 eBPF map
func (c *Collector) GetMap(name string) *ebpf.Map {
	switch name {
	case "network_map":
		return c.objs.NetworkMap
	case "tcp_latency_map":
		if c.tcpMetricsObjs != nil {
			return c.tcpMetricsObjs.TcpLatencyMap
		}
	case "tcp_stats_map":
		if c.tcpMetricsObjs != nil {
			return c.tcpMetricsObjs.TcpStatsMap
		}
	case "global_tcp_metrics_map":
		if c.tcpMetricsObjs != nil {
			return c.tcpMetricsObjs.GlobalTcpMetricsMap
		}
	case "http_stats_map":
		if c.httpMetricsObjs != nil {
			return c.httpMetricsObjs.HttpStatsMap
		}
	case "global_http_metrics_map":
		if c.httpMetricsObjs != nil {
			return c.httpMetricsObjs.GlobalHttpMetricsMap
		}
	}
	return nil
}

// GetTCPMetrics 获取TCP指标采集器对象
func (c *Collector) GetTCPMetrics() *bpf.TCPMetricsObjects {
	return c.tcpMetricsObjs
}

// GetHTTPMetrics 获取HTTP指标采集器对象
func (c *Collector) GetHTTPMetrics() *bpf.HTTPMetricsObjects {
	return c.httpMetricsObjs
}
