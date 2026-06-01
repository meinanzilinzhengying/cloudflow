// Package vxlan 提供VXLAN隧道解封装能力
//
// 功能：
// - 解析VXLAN封装流量（UDP端口4789）
// - 提取内层五元组（源/目的IP、端口、协议）
// - 提取VNI（Virtual Network Identifier）
// - 支持华为云VXLAN格式
// - 将解封装后的流量镜像至云下TAP设备
package vxlan

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
	edge "cloud-flow/proto"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// VXLANDecapConfig VXLAN解封装配置
type VXLANDecapConfig struct {
	// 管理网卡接口
	MgmtIface string

	// 是否启用TAP镜像
	EnableTapMirror bool

	// TAP设备名称（默认 vxlan-tap0）
	TapDeviceName string

	// 是否解析内层协议
	ParseInnerProtocol bool

	// 采集间隔
	CollectInterval time.Duration
}

// InnerFlowInfo 内层流量信息（与eBPF结构对应）
type InnerFlowInfo struct {
	InnerSrcIP       uint32
	InnerDstIP       uint32
	InnerSrcPort     uint16
	InnerDstPort     uint16
	InnerProtocol    uint8
	InnerIPVersion   uint8
	VNI              uint32
	OuterPacketSize  uint64
	InnerPacketSize  uint64
	Timestamp        uint64
}

// DecapFlowKey 解封装流量键（与eBPF结构对应）
type DecapFlowKey struct {
	OuterSrcIP     uint32
	OuterDstIP     uint32
	InnerSrcIP     uint32
	InnerDstIP     uint32
	OuterSrcPort   uint16
	OuterDstPort   uint16
	InnerSrcPort   uint16
	InnerDstPort   uint16
	InnerProtocol  uint8
	InnerIPVersion uint8
	Pad            [2]byte
	VNI            uint32
}

// DecapFlowStats 解封装流量统计（与eBPF结构对应）
type DecapFlowStats struct {
	Packets    uint64
	Bytes      uint64
	InnerBytes uint64
	TsFirst    uint64
	TsLast     uint64
}

// Collector VXLAN解封装采集器
type Collector struct {
	cfg    VXLANDecapConfig
	log    *logger.Logger
	mu     sync.RWMutex

	// eBPF对象
	collection *ebpf.Collection
	prog       *ebpf.Program
	flowMap    *ebpf.Map
	eventsRing *ringbuf.Reader

	// TC链接
	tcLink link.Link

	// TAP设备
	tapDevice *os.File
	tapName   string

	// 采集控制
	stopCh chan struct{}
	wg     sync.WaitGroup

	// 数据通道
	collectCh chan []*edge.MetricData

	// 统计
	stats struct {
		totalVXLANPackets uint64
		decapSuccess      uint64
		decapFailed       uint64
		tapMirrorPackets  uint64
		tapMirrorBytes    uint64
	}
}

// NewCollector 创建VXLAN解封装采集器
func NewCollector(cfg VXLANDecapConfig, log *logger.Logger) (*Collector, error) {
	if cfg.MgmtIface == "" {
		return nil, fmt.Errorf("管理网卡接口不能为空")
	}
	if cfg.TapDeviceName == "" {
		cfg.TapDeviceName = "vxlan-tap0"
	}
	if cfg.CollectInterval <= 0 {
		cfg.CollectInterval = 5 * time.Second
	}

	return &Collector{
		cfg:       cfg,
		log:       log,
		stopCh:    make(chan struct{}),
		collectCh: make(chan []*edge.MetricData, 100),
	}, nil
}

// Start 启动采集器
func (c *Collector) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 移除内存限制
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("移除内存限制失败: %w", err)
	}

	// 加载eBPF程序
	if err := c.loadBPF(); err != nil {
		return fmt.Errorf("加载eBPF程序失败: %w", err)
	}

	// 附加TC程序
	if err := c.attachTC(); err != nil {
		c.closeBPF()
		return fmt.Errorf("附加TC程序失败: %w", err)
	}

	// 创建TAP设备（如果启用）
	if c.cfg.EnableTapMirror {
		if err := c.createTapDevice(); err != nil {
			c.log.Warnf("[VXLAN] 创建TAP设备失败: %v，禁用镜像功能", err)
		} else {
			c.log.Infof("[VXLAN] TAP设备已创建: %s", c.tapName)
		}
	}

	// 启动事件读取协程
	c.wg.Add(1)
	go c.eventLoop()

	// 启动采集循环
	c.wg.Add(1)
	go c.collectLoop()

	c.log.Infof("[VXLAN] 解封装采集器已启动: iface=%s, tap=%v",
		c.cfg.MgmtIface, c.cfg.EnableTapMirror)

	return nil
}

// Stop 停止采集器
func (c *Collector) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	close(c.stopCh)
	c.wg.Wait()

	// 分离TC程序
	if c.tcLink != nil {
		c.tcLink.Close()
		c.tcLink = nil
	}

	// 关闭TAP设备
	if c.tapDevice != nil {
		c.tapDevice.Close()
		c.tapDevice = nil
		// 删除TAP设备
		if c.tapName != "" {
			c.deleteTapDevice()
		}
	}

	// 关闭eBPF对象
	c.closeBPF()

	c.log.Info("[VXLAN] 解封装采集器已停止")
	return nil
}

// Collect 采集数据
func (c *Collector) Collect() []*edge.MetricData {
	// 从内部通道读取
	return nil // 实际实现中从collectCh读取
}

// loadBPF 加载eBPF程序
func (c *Collector) loadBPF() error {
	// 创建eBPF集合规范
	spec := &ebpf.CollectionSpec{
		Programs: map[string]*ebpf.ProgramSpec{
			"vxlan_decap": {
				Name:    "vxlan_decap",
				Type:    ebpf.SchedCLS,
				License: "GPL",
			},
		},
		Maps: map[string]*ebpf.MapSpec{
			"vxlan_flow_map": {
				Name:       "vxlan_flow_map",
				Type:       ebpf.Hash,
				KeySize:    36, // sizeof(DecapFlowKey)
				ValueSize:  40, // sizeof(DecapFlowStats)
				MaxEntries: 100000,
			},
			"inner_flow_events": {
				Name:       "inner_flow_events",
				Type:       ebpf.RingBuf,
				MaxEntries: 1 << 24, // 16MB
			},
		},
	}

	// 加载集合
	collection, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("创建eBPF集合失败: %w", err)
	}

	c.collection = collection
	c.prog = collection.Programs["vxlan_decap"]
	c.flowMap = collection.Maps["vxlan_flow_map"]

	// 打开ring buffer
	eventsMap := collection.Maps["inner_flow_events"]
	reader, err := ringbuf.NewReader(eventsMap)
	if err != nil {
		collection.Close()
		return fmt.Errorf("创建ring buffer reader失败: %w", err)
	}
	c.eventsRing = reader

	return nil
}

// closeBPF 关闭eBPF对象
func (c *Collector) closeBPF() {
	if c.eventsRing != nil {
		c.eventsRing.Close()
		c.eventsRing = nil
	}
	if c.collection != nil {
		c.collection.Close()
		c.collection = nil
	}
	c.prog = nil
	c.flowMap = nil
}

// attachTC 附加TC程序到网卡
func (c *Collector) attachTC() error {
	if c.prog == nil {
		return fmt.Errorf("eBPF程序未加载")
	}

	// 获取网卡
	iface, err := net.InterfaceByName(c.cfg.MgmtIface)
	if err != nil {
		return fmt.Errorf("获取网卡信息失败: %w", err)
	}

	// 使用cilium/ebpf的link.AttachTC
	l, err := link.AttachTC(link.TCOptions{
		Program:   c.prog,
		Interface: iface.Index,
		Attach:    ebpf.AttachTCIngress,
	})
	if err != nil {
		return fmt.Errorf("附加TC程序失败: %w", err)
	}

	c.tcLink = l
	return nil
}

// createTapDevice 创建TAP设备
func (c *Collector) createTapDevice() error {
	if c.cfg.TapDeviceName == "" {
		c.cfg.TapDeviceName = "vxlan-tap0"
	}

	// 使用netlink创建TAP设备
	tap := &netlink.Tuntap{
		LinkAttrs: netlink.LinkAttrs{
			Name:  c.cfg.TapDeviceName,
			Flags: net.FlagUp,
		},
		Mode:  netlink.TUNTAP_MODE_TAP,
		Flags: netlink.TUNTAP_DEFAULTS,
	}

	// 检查是否已存在
	existing, err := netlink.LinkByName(c.cfg.TapDeviceName)
	if err == nil {
		// 已存在，使用现有设备
		c.tapName = c.cfg.TapDeviceName
		c.log.Infof("[VXLAN] TAP设备已存在: %s", c.cfg.TapDeviceName)
		return nil
	}

	// 创建新设备
	if err := netlink.LinkAdd(tap); err != nil {
		return fmt.Errorf("创建TAP设备失败: %w", err)
	}

	// 启用设备
	if err := netlink.LinkSetUp(tap); err != nil {
		netlink.LinkDel(tap)
		return fmt.Errorf("启用TAP设备失败: %w", err)
	}

	c.tapName = c.cfg.TapDeviceName
	return nil
}

// deleteTapDevice 删除TAP设备
func (c *Collector) deleteTapDevice() error {
	link, err := netlink.LinkByName(c.tapName)
	if err != nil {
		return nil // 设备不存在
	}
	return netlink.LinkDel(link)
}

// eventLoop 事件读取循环
func (c *Collector) eventLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		default:
			// 读取ring buffer事件
			record, err := c.eventsRing.Read()
			if err != nil {
				if err == ringbuf.ErrClosed {
					return
				}
				continue
			}

			// 解析内层流量信息
			if len(record.RawSample) >= 48 { // sizeof(InnerFlowInfo)
				info := c.parseInnerFlowInfo(record.RawSample)

				// 镜像到TAP设备
				if c.cfg.EnableTapMirror && c.tapDevice != nil {
					c.mirrorToTap(info, record.RawSample)
				}
			}
		}
	}
}

// collectLoop 采集循环
func (c *Collector) collectLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.cfg.CollectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.collectFlows()
		case <-c.stopCh:
			return
		}
	}
}

// collectFlows 采集流量数据
func (c *Collector) collectFlows() {
	if c.flowMap == nil {
		return
	}

	now := time.Now().Unix()
	var metrics []*edge.MetricData

	// 遍历flow map
	var key DecapFlowKey
	var stats DecapFlowStats

	entries := c.flowMap.Iterate()
	for entries.Next(&key, &stats) {
		metric := c.flowToMetric(&key, &stats, now)
		if metric != nil {
			metrics = append(metrics, metric)
		}
	}

	// TODO: 发送metrics到通道
}

// parseInnerFlowInfo 解析内层流量信息
func (c *Collector) parseInnerFlowInfo(data []byte) *InnerFlowInfo {
	if len(data) < 48 {
		return nil
	}

	return &InnerFlowInfo{
		InnerSrcIP:      binary.LittleEndian.Uint32(data[0:4]),
		InnerDstIP:      binary.LittleEndian.Uint32(data[4:8]),
		InnerSrcPort:    binary.LittleEndian.Uint16(data[8:10]),
		InnerDstPort:    binary.LittleEndian.Uint16(data[10:12]),
		InnerProtocol:   data[12],
		InnerIPVersion:  data[13],
		VNI:             binary.LittleEndian.Uint32(data[16:20]),
		OuterPacketSize: binary.LittleEndian.Uint64(data[24:32]),
		InnerPacketSize: binary.LittleEndian.Uint64(data[32:40]),
		Timestamp:       binary.LittleEndian.Uint64(data[40:48]),
	}
}

// mirrorToTap 镜像流量到TAP设备
func (c *Collector) mirrorToTap(info *InnerFlowInfo, rawPacket []byte) {
	// TODO: 实现TAP镜像
	// 需要重构内层以太网帧并写入TAP设备
}

// flowToMetric 将流量数据转换为指标
func (c *Collector) flowToMetric(key *DecapFlowKey, stats *DecapFlowStats, now int64) *edge.MetricData {
	innerSrcIP := make(net.IP, 4)
	binary.LittleEndian.PutUint32(innerSrcIP, key.InnerSrcIP)

	innerDstIP := make(net.IP, 4)
	binary.LittleEndian.PutUint32(innerDstIP, key.InnerDstIP)

	protocol := "unknown"
	if key.InnerProtocol == 6 {
		protocol = "tcp"
	} else if key.InnerProtocol == 17 {
		protocol = "udp"
	}

	return &edge.MetricData{
		Timestamp: now,
		SrcIp:     innerSrcIP.String(),
		DstIp:     innerDstIP.String(),
		SrcPort:   int32(key.InnerSrcPort),
		DstPort:   int32(key.InnerDstPort),
		Protocol:  protocol,
		Bytes:     int64(stats.Bytes),
		Packets:   int64(stats.Packets),
		Tags: map[string]string{
			"source":          "vxlan_decap",
			"vni":             fmt.Sprintf("%d", key.VNI),
			"inner_ip_version": fmt.Sprintf("%d", key.InnerIPVersion),
		},
	}
}

// Stats 返回统计信息
func (c *Collector) Stats() map[string]interface{} {
	return map[string]interface{}{
		"tap_enabled": c.cfg.EnableTapMirror,
		"tap_device":  c.tapName,
	}
}
