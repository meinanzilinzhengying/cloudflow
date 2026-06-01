// Package ebpfcollector 提供非侵入式 eBPF 网络采集功能
//
// 特点：
// - 使用 BPF_PROG_TYPE_SOCKET_FILTER 替代 kprobe
// - 不修改内核函数，不注入代码
// - 部署/卸载不影响业务进程
// - 零拷贝网络包采集
package ebpfcollector

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
	edge "cloud-flow/proto"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"
)

// NonInvasiveCollector 非侵入式 eBPF 采集器
type NonInvasiveCollector struct {
	mu     sync.RWMutex
	log    *logger.Logger
	config NonInvasiveConfig

	// eBPF 对象
	collection *ebpf.Collection
	prog       *ebpf.Program
	flowMap    *ebpf.Map
	statsMap   *ebpf.Map

	// Socket 链接
	socketLink link.Link
	rawSocket  int

	// 采集控制
	stopCh chan struct{}
	wg     sync.WaitGroup

	// 数据通道
	collectCh chan []*edge.MetricData
}

// NonInvasiveConfig 非侵入式采集器配置
type NonInvasiveConfig struct {
	// 管理网卡接口
	MgmtIface string

	// 采集模式
	Mode NonInvasiveMode

	// 采样率 (0.0-1.0)
	SampleRate float64

	// 采集间隔
	CollectInterval time.Duration

	// 是否启用五元组解析
	EnableFlowParse bool

	// 是否启用轻量级模式（仅计数，不解析五元组）
	LightMode bool
}

// NonInvasiveMode 采集模式
type NonInvasiveMode int

const (
	// ModeSocketFilter Socket Filter 模式（推荐）
	ModeSocketFilter NonInvasiveMode = iota

	// ModeTC Traffic Control 模式（备用）
	ModeTC
)

// NewNonInvasiveCollector 创建非侵入式采集器
func NewNonInvasiveCollector(cfg NonInvasiveConfig, log *logger.Logger) (*NonInvasiveCollector, error) {
	if cfg.MgmtIface == "" {
		return nil, fmt.Errorf("管理网卡接口不能为空")
	}
	if cfg.SampleRate <= 0 || cfg.SampleRate > 1 {
		cfg.SampleRate = 1.0
	}
	if cfg.CollectInterval <= 0 {
		cfg.CollectInterval = 5 * time.Second
	}

	return &NonInvasiveCollector{
		config:    cfg,
		log:       log,
		stopCh:    make(chan struct{}),
		collectCh: make(chan []*edge.MetricData, 100),
	}, nil
}

// Start 启动采集器
func (c *NonInvasiveCollector) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 移除内存限制
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("移除内存限制失败: %w", err)
	}

	// 加载 eBPF 程序
	if err := c.loadBPF(); err != nil {
		return fmt.Errorf("加载 eBPF 程序失败: %w", err)
	}

	// 附加到 raw socket
	if err := c.attach(); err != nil {
		c.closeBPF()
		return fmt.Errorf("附加 eBPF 程序失败: %w", err)
	}

	// 启动采集循环
	c.wg.Add(1)
	go c.collectLoop()

	c.log.Infof("[非侵入式eBPF] 采集器已启动: iface=%s, mode=%v, sample_rate=%.2f",
		c.config.MgmtIface, c.config.Mode, c.config.SampleRate)

	return nil
}

// Stop 停止采集器
func (c *NonInvasiveCollector) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	close(c.stopCh)
	c.wg.Wait()

	// 分离 eBPF 程序
	if err := c.detach(); err != nil {
		c.log.Warnf("[非侵入式eBPF] 分离程序警告: %v", err)
	}

	// 关闭 eBPF 对象
	c.closeBPF()

	c.log.Info("[非侵入式eBPF] 采集器已停止")
	return nil
}

// Collect 采集数据
func (c *NonInvasiveCollector) Collect() []*edge.MetricData {
	select {
	case data := <-c.collectCh:
		return data
	default:
		return nil
	}
}

// SetSampleRate 设置采样率（运行时调整）
func (c *NonInvasiveCollector) SetSampleRate(rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.SampleRate = rate
	c.log.Infof("[非侵入式eBPF] 采样率已调整为: %.2f", rate)
}

// loadBPF 加载 eBPF 程序
func (c *NonInvasiveCollector) loadBPF() error {
	// 这里使用嵌入的 eBPF 字节码
	// 实际项目中应该通过 go:embed 嵌入编译后的 .o 文件
	// 或者使用 cilium/ebpf 的 CollectionSpec 从 ELF 加载

	// 示例：创建简单的 eBPF 程序规范
	spec := &ebpf.CollectionSpec{
		Programs: map[string]*ebpf.ProgramSpec{
			"socket_filter_prog": {
				Name:    "socket_filter_prog",
				Type:    ebpf.SocketFilter,
				License: "GPL",
				// 字节码应该来自编译后的 .o 文件
				// 这里简化处理，实际应该使用 go:embed
			},
		},
		Maps: map[string]*ebpf.MapSpec{
			"flow_stats_map": {
				Name:       "flow_stats_map",
				Type:       ebpf.Hash,
				KeySize:    16, // sizeof(struct flow_key)
				ValueSize:  32, // sizeof(struct flow_stats)
				MaxEntries: 100000,
			},
			"global_stats_map": {
				Name:       "global_stats_map",
				Type:       ebpf.Array,
				KeySize:    4,
				ValueSize:  8,
				MaxEntries: 1,
			},
		},
	}

	// 加载集合
	collection, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("创建 eBPF 集合失败: %w", err)
	}

	c.collection = collection
	c.prog = collection.Programs["socket_filter_prog"]
	c.flowMap = collection.Maps["flow_stats_map"]
	c.statsMap = collection.Maps["global_stats_map"]

	return nil
}

// closeBPF 关闭 eBPF 对象
func (c *NonInvasiveCollector) closeBPF() {
	if c.collection != nil {
		c.collection.Close()
		c.collection = nil
	}
	c.prog = nil
	c.flowMap = nil
	c.statsMap = nil
}

// attach 附加 eBPF 程序到 raw socket
func (c *NonInvasiveCollector) attach() error {
	if c.prog == nil {
		return fmt.Errorf("eBPF 程序未加载")
	}

	// 创建 raw socket
	// ETH_P_ALL 接收所有协议包
	sock, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(unix.ETH_P_ALL))
	if err != nil {
		return fmt.Errorf("创建 raw socket 失败: %w", err)
	}

	// 绑定到指定网卡
	iface, err := net.InterfaceByName(c.config.MgmtIface)
	if err != nil {
		unix.Close(sock)
		return fmt.Errorf("获取网卡信息失败: %w", err)
	}

	addr := unix.SockaddrLinklayer{
		Protocol: unix.ETH_P_ALL,
		Ifindex:  iface.Index,
	}

	if err := unix.Bind(sock, &addr); err != nil {
		unix.Close(sock)
		return fmt.Errorf("绑定 socket 失败: %w", err)
	}

	c.rawSocket = sock

	// 使用 link.AttachSocketFilter 附加 eBPF 程序
	// 这是非侵入式的，仅作为 socket 过滤器运行
	l, err := link.AttachSocketFilter(link.SocketFilterOptions{
		Program: c.prog,
		Socket:  sock,
	})
	if err != nil {
		unix.Close(sock)
		return fmt.Errorf("附加 socket filter 失败: %w", err)
	}

	c.socketLink = l

	c.log.Infof("[非侵入式eBPF] 已附加到网卡: %s", c.config.MgmtIface)
	return nil
}

// detach 分离 eBPF 程序
func (c *NonInvasiveCollector) detach() error {
	if c.socketLink != nil {
		if err := c.socketLink.Close(); err != nil {
			return err
		}
		c.socketLink = nil
	}

	if c.rawSocket > 0 {
		unix.Close(c.rawSocket)
		c.rawSocket = 0
	}

	return nil
}

// collectLoop 采集循环
func (c *NonInvasiveCollector) collectLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.CollectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := c.collectData()
			if len(metrics) > 0 {
				select {
				case c.collectCh <- metrics:
				default:
					c.log.Warn("[非侵入式eBPF] 采集通道已满，丢弃数据")
				}
			}
		case <-c.stopCh:
			return
		}
	}
}

// collectData 采集数据
func (c *NonInvasiveCollector) collectData() []*edge.MetricData {
	c.mu.RLock()
	flowMap := c.flowMap
	statsMap := c.statsMap
	sampleRate := c.config.SampleRate
	c.mu.RUnlock()

	if flowMap == nil {
		return nil
	}

	now := time.Now().Unix()
	var metrics []*edge.MetricData

	// 读取全局统计
	if statsMap != nil {
		var totalPackets uint64
		if err := statsMap.Lookup(uint32(0), &totalPackets); err == nil {
			metrics = append(metrics, &edge.MetricData{
				Timestamp: now,
				Name:      "noninvasive_total_packets",
				Value:     float64(totalPackets),
				Tags: map[string]string{
					"source": "noninvasive_ebpf",
					"iface":  c.config.MgmtIface,
				},
			})
		}
	}

	// 读取流统计（如果启用五元组解析）
	if c.config.EnableFlowParse && flowMap != nil {
		// 采样处理
		if sampleRate < 1.0 && !c.shouldSample() {
			return metrics
		}

		// 遍历 flow map
		// 注意：实际实现中应该使用 batch lookup 提高效率
		var key flowKey
		var value flowStats

		entries := flowMap.Iterate()
		for entries.Next(&key, &value) {
			metric := c.flowToMetric(&key, &value, now)
			if metric != nil {
				metrics = append(metrics, metric)
			}
		}

		if err := entries.Err(); err != nil {
			c.log.Warnf("[非侵入式eBPF] 遍历 flow map 错误: %v", err)
		}
	}

	return metrics
}

// shouldSample 采样判断
func (c *NonInvasiveCollector) shouldSample() bool {
	// 简单随机采样
	// 实际应该使用更精确的计数器采样
	return true // 简化处理
}

// flowToMetric 将流数据转换为指标
func (c *NonInvasiveCollector) flowToMetric(key *flowKey, stats *flowStats, now int64) *edge.MetricData {
	srcIP := net.IP(key.SrcIP[:])
	dstIP := net.IP(key.DstIP[:])

	protocol := "unknown"
	if key.Protocol == 6 {
		protocol = "tcp"
	} else if key.Protocol == 17 {
		protocol = "udp"
	}

	return &edge.MetricData{
		Timestamp: now,
		SrcIp:     srcIP.String(),
		DstIp:     dstIP.String(),
		SrcPort:   int32(key.SrcPort),
		DstPort:   int32(key.DstPort),
		Protocol:  protocol,
		Bytes:     int64(stats.Bytes),
		Packets:   int64(stats.Packets),
		Tags: map[string]string{
			"source":    "noninvasive_ebpf",
			"iface":     c.config.MgmtIface,
			"flow_type": "socket_filter",
		},
	}
}

// flowKey 五元组键（与 eBPF 结构对应）
type flowKey struct {
	SrcIP   [4]byte
	DstIP   [4]byte
	SrcPort uint16
	DstPort uint16
	Protocol uint8
	Pad     [3]byte
}

// flowStats 流统计（与 eBPF 结构对应）
type flowStats struct {
	Bytes     uint64
	Packets   uint64
	TsStart   uint64
	TsLast    uint64
}

// Ensure NonInvasiveCollector implements EBPFCollectorInterface
var _ EBPFCollectorInterface = (*NonInvasiveCollector)(nil)
