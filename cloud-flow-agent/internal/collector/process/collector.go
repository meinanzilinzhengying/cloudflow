// Package process 提供进程事件采集功能
// 通过 netlink connector 监听进程事件（exec/fork/exit），
// 当 netlink 不可用时自动降级到 /proc 扫描模式
package process

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"cloud-flow-agent/pkg/logger"
	edge "cloud-flow/proto"
)

// ProcessEventType 进程事件类型
type ProcessEventType string

const (
	EventFork ProcessEventType = "fork"
	EventExec ProcessEventType = "exec"
	EventExit ProcessEventType = "exit"
)

// ProcessEvent 进程事件
type ProcessEvent struct {
	Type      ProcessEventType // 事件类型
	PID       uint32           // 进程 ID
	PPID      uint32           // 父进程 ID
	Timestamp int64            // 事件时间戳（Unix 秒）
	Comm      string           // 进程名
	ExitCode  int              // 退出码（仅 exit 事件有效）
}

// CollectMode 采集模式
type CollectMode int

const (
	ModeNetlink CollectMode = iota // netlink connector 模式
	ModeProcScan                   // /proc 扫描模式（降级）
)

// Config 进程采集器配置
type Config struct {
	// ScanInterval /proc 扫描间隔（降级模式使用）
	ScanInterval time.Duration
	// BufferSize 事件缓冲区大小
	BufferSize int
	// MaxProcs 最大监控进程数
	MaxProcs int
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		ScanInterval: 5 * time.Second,
		BufferSize:   1024,
		MaxProcs:     10000,
	}
}

// Collector 进程事件采集器
type Collector struct {
	cfg       Config
	log       *logger.Logger
	mode      CollectMode
	eventCh   chan *ProcessEvent
	stopCh    chan struct{}
	mu        sync.Mutex
	started   bool
	// 已知进程缓存，用于 /proc 扫描模式下检测进程变化
	knownPIDs map[uint32]procInfo
}

// procInfo 进程基本信息
type procInfo struct {
	pid   uint32
	ppid  uint32
	comm  string
	state string
}

// NewCollector 创建进程事件采集器
func NewCollector(log *logger.Logger, cfg Config) *Collector {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1024
	}
	if cfg.MaxProcs <= 0 {
		cfg.MaxProcs = 10000
	}
	if cfg.ScanInterval <= 0 {
		cfg.ScanInterval = 5 * time.Second
	}

	return &Collector{
		cfg:       cfg,
		log:       log,
		eventCh:   make(chan *ProcessEvent, cfg.BufferSize),
		stopCh:    make(chan struct{}),
		knownPIDs: make(map[uint32]procInfo),
	}
}

// Start 启动进程事件采集
func (c *Collector) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("采集器已经启动")
	}

	// 尝试使用 netlink connector 模式
	if err := c.startNetlink(); err != nil {
		c.log.Warnf("netlink connector 模式不可用: %v，降级到 /proc 扫描模式", err)
		c.mode = ModeProcScan
		go c.procScanLoop()
	} else {
		c.mode = ModeNetlink
		c.log.Info("进程事件采集器已启动 (netlink connector 模式)")
	}

	c.started = true
	return nil
}

// Stop 停止进程事件采集
func (c *Collector) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return
	}

	close(c.stopCh)
	c.stopCh = make(chan struct{})
	c.started = false
	c.log.Info("进程事件采集器已停止")
}

// Collect 采集进程指标数据，返回 edge.MetricData 格式
func (c *Collector) Collect() []*edge.MetricData {
	now := time.Now().Unix()
	var metrics []*edge.MetricData

	// 获取当前进程总数和运行状态
	totalProcs, runningProcs, sleepingProcs := c.getProcessStats()

	// 进程总数指标
	metrics = append(metrics, &edge.MetricData{
		Timestamp: now,
		SrcIp:     "localhost",
		DstIp:     "process",
		Protocol:  "process",
		Bytes:     int64(totalProcs),
		Tags: map[string]string{
			"type":    "process_total",
			"mode":    c.modeString(),
			"running": strconv.Itoa(runningProcs),
			"sleeping": strconv.Itoa(sleepingProcs),
		},
	})

	// 从事件通道中收集事件并转换为指标
	c.collectEvents(&metrics)

	return metrics
}

// Mode 返回当前采集模式
func (c *Collector) Mode() CollectMode {
	return c.mode
}

// Events 返回事件通道（用于外部消费原始事件）
func (c *Collector) Events() <-chan *ProcessEvent {
	return c.eventCh
}

// modeString 返回采集模式的字符串表示
func (c *Collector) modeString() string {
	switch c.mode {
	case ModeNetlink:
		return "netlink"
	case ModeProcScan:
		return "proc_scan"
	default:
		return "unknown"
	}
}

// ============================================================================
// netlink connector 实现
// ============================================================================

const (
	// NETLINK_KOBJECT_UEVENT = 15, 但进程事件使用 NETLINK_CONNECTOR
	NETLINK_CONNECTOR = 11

	// CN_IDX_PROC = 0x1
	CN_IDX_PROC = 0x1

	// PROC_CN_MCAST_LISTEN = 1
	PROC_CN_MCAST_LISTEN = 1

	// PROC_EVENT_NONE = 0x00000000
	PROC_EVENT_NONE = 0x00000000
	// PROC_EVENT_FORK = 0x00000001
	PROC_EVENT_FORK = 0x00000001
	// PROC_EVENT_EXEC = 0x00000002
	PROC_EVENT_EXEC = 0x00000002
	// PROC_EVENT_EXIT = 0x00000004
	PROC_EVENT_EXIT = 0x00000004
)

// cnMsg 结构体对应 Linux 内核的 cn_msg
type cnMsg struct {
	ID    [2]uint32 // cb_id: val, seq
	Seq   uint32
	Ack   uint32
	Len   uint16
	Flags uint16
}

// procEventHeader 进程事件头部
type procEventHeader struct {
	What uint32
	CPU  uint32
	Timestamp [2]uint64 // nsec, sec
}

// procEventFork fork 事件数据
type procEventFork struct {
	ParentPID  uint32
	ParentTGID uint32
	ChildPID   uint32
	ChildTGID  uint32
}

// procEventExec exec 事件数据
type procEventExec struct {
	ProcessPID  uint32
	ProcessTGID uint32
}

// procEventExit exit 事件数据
type procEventExit struct {
	ProcessPID  uint32
	ProcessTGID uint32
	ExitCode    uint32
	ExitSignal  uint32
}

// startNetlink 启动 netlink connector 模式
func (c *Collector) startNetlink() error {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM, NETLINK_CONNECTOR)
	if err != nil {
		return fmt.Errorf("创建 netlink socket 失败: %w", err)
	}
	defer func() {
		if err != nil {
			syscall.Close(fd)
		}
	}()

	// 绑定 socket
	addr := &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: CN_IDX_PROC,
		Pid:    uint32(os.Getpid()),
	}
	if err := syscall.Bind(fd, addr); err != nil {
		return fmt.Errorf("绑定 netlink socket 失败: %w", err)
	}

	// 发送订阅消息
	if err := c.sendNetlinkSubscribe(fd); err != nil {
		return fmt.Errorf("发送 netlink 订阅失败: %w", err)
	}

	// 启动监听协程
	go c.netlinkListenLoop(fd)

	return nil
}

// sendNetlinkSubscribe 发送 netlink connector 订阅消息
func (c *Collector) sendNetlinkSubscribe(fd int) error {
	// 构建 nlmsghdr + cn_msg + proc_cn_mcast_op
	// nlmsghdr: 16 bytes
	// cn_msg: 16 bytes
	// proc_cn_mcast_op: 4 bytes (uint32)
	totalLen := 16 + 16 + 4

	buf := make([]byte, totalLen)

	// nlmsghdr
	binary.NativeEndian.PutUint32(buf[0:4], uint32(totalLen)) // nlmsg_len
	binary.NativeEndian.PutUint16(buf[4:6], 16)              // nlmsg_type = NLMSG_DONE
	binary.NativeEndian.PutUint16(buf[6:8], 0)               // nlmsg_flags
	binary.NativeEndian.PutUint32(buf[8:12], 0)              // nlmsg_seq
	binary.NativeEndian.PutUint32(buf[12:16], 0)             // nlmsg_pid

	// cn_msg
	binary.NativeEndian.PutUint32(buf[16:20], CN_IDX_PROC)          // id.val
	binary.NativeEndian.PutUint32(buf[20:24], CN_IDX_PROC)          // id.seq
	binary.NativeEndian.PutUint32(buf[24:28], 1)                    // seq
	binary.NativeEndian.PutUint32(buf[28:32], 0)                    // ack
	binary.NativeEndian.PutUint16(buf[32:34], 4)                    // len (proc_cn_mcast_op 大小)
	binary.NativeEndian.PutUint16(buf[34:36], 0)                    // flags

	// proc_cn_mcast_op
	binary.NativeEndian.PutUint32(buf[36:40], PROC_CN_MCAST_LISTEN)

	// 发送消息
	addr := &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Pid:    0, // 内核
		Groups: 0,
	}
	return syscall.Sendto(fd, buf, 0, addr)
}

// netlinkListenLoop netlink 监听循环
func (c *Collector) netlinkListenLoop(fd int) {
	defer syscall.Close(fd)

	buf := make([]byte, 4096)
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		// 设置读取超时，以便定期检查 stopCh
		tv := syscall.NsecToTimeval(int64(2 * time.Second))
		syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)

		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			// 超时是正常的，继续循环
			if isTimeout(err) {
				continue
			}
			c.log.Warnf("netlink 接收失败: %v", err)
			return
		}

		if n < 16 {
			continue
		}

		// 解析 netlink 消息头
		nlmsgLen := binary.NativeEndian.Uint32(buf[0:4])
		if uint32(n) < nlmsgLen || nlmsgLen < 32 {
			continue
		}

		// 解析 cn_msg
		cnLen := binary.NativeEndian.Uint16(buf[32:34])
		if cnLen == 0 {
			continue
		}

		// 解析进程事件
		event := c.parseProcEvent(buf[36:])
		if event != nil {
			select {
			case c.eventCh <- event:
			default:
				c.log.Warnf("进程事件缓冲区已满，丢弃事件: %s pid=%d", event.Type, event.PID)
			}
		}
	}
}

// parseProcEvent 解析进程事件
func (c *Collector) parseProcEvent(data []byte) *ProcessEvent {
	if len(data) < 16 {
		return nil
	}

	// 读取事件类型
	what := binary.NativeEndian.Uint32(data[0:4])
	now := time.Now().Unix()

	switch what {
	case PROC_EVENT_FORK:
		if len(data) < 16+16 {
			return nil
		}
		forkData := data[16 : 16+16]
		parentPID := binary.NativeEndian.Uint32(forkData[0:4])
		childPID := binary.NativeEndian.Uint32(forkData[8:12])
		comm := c.getProcessComm(int(childPID))
		return &ProcessEvent{
			Type:      EventFork,
			PID:       childPID,
			PPID:      parentPID,
			Timestamp: now,
			Comm:      comm,
		}

	case PROC_EVENT_EXEC:
		if len(data) < 16+8 {
			return nil
		}
		execData := data[16 : 16+8]
		pid := binary.NativeEndian.Uint32(execData[0:4])
		comm := c.getProcessComm(int(pid))
		return &ProcessEvent{
			Type:      EventExec,
			PID:       pid,
			Timestamp: now,
			Comm:      comm,
		}

	case PROC_EVENT_EXIT:
		if len(data) < 16+16 {
			return nil
		}
		exitData := data[16 : 16+16]
		pid := binary.NativeEndian.Uint32(exitData[0:4])
		exitCode := int(binary.NativeEndian.Uint32(exitData[8:12]))
		return &ProcessEvent{
			Type:      EventExit,
			PID:       pid,
			Timestamp: now,
			ExitCode:  exitCode,
		}
	}

	return nil
}

// ============================================================================
// /proc 扫描模式实现
// ============================================================================

// procScanLoop /proc 扫描循环
func (c *Collector) procScanLoop() {
	ticker := time.NewTicker(c.cfg.ScanInterval)
	defer ticker.Stop()

	// 初始扫描，填充已知进程列表
	c.scanProc()

	for {
		select {
		case <-ticker.C:
			c.scanProc()
		case <-c.stopCh:
			return
		}
	}
}

// scanProc 扫描 /proc 目录，检测进程变化
func (c *Collector) scanProc() {
	currentPIDs := make(map[uint32]procInfo)

	procDir, err := os.Open("/proc")
	if err != nil {
		c.log.Warnf("打开 /proc 目录失败: %v", err)
		return
	}
	defer procDir.Close()

	entries, err := procDir.Readdirnames(-1)
	if err != nil {
		c.log.Warnf("读取 /proc 目录失败: %v", err)
		return
	}

	now := time.Now().Unix()
	newProcCount := 0
	exitProcCount := 0

	for _, entry := range entries {
		pid, err := strconv.ParseUint(entry, 10, 32)
		if err != nil {
			continue
		}

		info, err := c.readProcInfo(uint32(pid))
		if err != nil {
			continue
		}

		currentPIDs[uint32(pid)] = info

		// 检测新进程
		if _, exists := c.knownPIDs[uint32(pid)]; !exists {
			newProcCount++
			event := &ProcessEvent{
				Type:      EventFork,
				PID:       info.pid,
				PPID:      info.ppid,
				Timestamp: now,
				Comm:      info.comm,
			}
			select {
			case c.eventCh <- event:
			default:
			}
		}
	}

	// 检测退出的进程
	for pid := range c.knownPIDs {
		if _, exists := currentPIDs[pid]; !exists {
			exitProcCount++
			event := &ProcessEvent{
				Type:      EventExit,
				PID:       pid,
				Timestamp: now,
				Comm:      c.knownPIDs[pid].comm,
			}
			select {
			case c.eventCh <- event:
			default:
			}
		}
	}

	if newProcCount > 0 || exitProcCount > 0 {
		c.log.Debugf("/proc 扫描: 新增 %d 个进程, 退出 %d 个进程", newProcCount, exitProcCount)
	}

	// 更新已知进程列表
	c.knownPIDs = currentPIDs
}

// readProcInfo 读取进程信息
func (c *Collector) readProcInfo(pid uint32) (procInfo, error) {
	info := procInfo{pid: pid}

	// 读取 /proc/{pid}/stat
	statPath := filepath.Join("/proc", strconv.Itoa(int(pid)), "stat")
	data, err := os.ReadFile(statPath)
	if err != nil {
		return info, err
	}

	// /proc/{pid}/stat 格式: pid (comm) state ppid ...
	content := string(data)

	// 提取 comm（可能包含空格和括号）
	leftParen := strings.Index(content, "(")
	rightParen := strings.LastIndex(content, ")")
	if leftParen == -1 || rightParen == -1 || rightParen <= leftParen {
		return info, fmt.Errorf("无法解析 /proc/%d/stat", pid)
	}

	info.comm = content[leftParen+1 : rightParen]

	// 提取 state 和 ppid
	rest := content[rightParen+2:]
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return info, fmt.Errorf("解析 /proc/%d/stat 字段不足", pid)
	}

	info.state = fields[0]
	ppid, err := strconv.ParseUint(fields[1], 10, 32)
	if err == nil {
		info.ppid = uint32(ppid)
	}

	return info, nil
}

// ============================================================================
// 辅助方法
// ============================================================================

// getProcessComm 获取进程名称
func (c *Collector) getProcessComm(pid int) string {
	statPath := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	data, err := os.ReadFile(statPath)
	if err != nil {
		return ""
	}

	content := string(data)
	leftParen := strings.Index(content, "(")
	rightParen := strings.LastIndex(content, ")")
	if leftParen == -1 || rightParen == -1 || rightParen <= leftParen {
		return ""
	}

	return content[leftParen+1 : rightParen]
}

// getProcessStats 获取进程统计信息
func (c *Collector) getProcessStats() (total, running, sleeping int) {
	procDir, err := os.Open("/proc")
	if err != nil {
		return 0, 0, 0
	}
	defer procDir.Close()

	entries, err := procDir.Readdirnames(-1)
	if err != nil {
		return 0, 0, 0
	}

	for _, entry := range entries {
		pid, err := strconv.ParseUint(entry, 10, 32)
		if err != nil {
			continue
		}

		info, err := c.readProcInfo(uint32(pid))
		if err != nil {
			continue
		}

		total++
		switch info.state {
		case "R":
			running++
		case "S":
			sleeping++
		}
	}

	return total, running, sleeping
}

// collectEvents 从事件通道收集事件并转换为 MetricData
func (c *Collector) collectEvents(metrics *[]*edge.MetricData) {
	var forkCount, execCount, exitCount int

	for {
		select {
		case event := <-c.eventCh:
			switch event.Type {
			case EventFork:
				forkCount++
			case EventExec:
				execCount++
			case EventExit:
				exitCount++
			}
		default:
			// 通道为空，退出循环
			goto done
		}
	}

done:
	if forkCount > 0 || execCount > 0 || exitCount > 0 {
		now := time.Now().Unix()
		*metrics = append(*metrics, &edge.MetricData{
			Timestamp: now,
			SrcIp:     "localhost",
			DstIp:     "process_events",
			Protocol:  "process",
			Bytes:     int64(forkCount),
			Packets:   int64(execCount),
			Latency:   int64(exitCount),
			Tags: map[string]string{
				"type":    "process_events",
				"forks":   strconv.Itoa(forkCount),
				"execs":   strconv.Itoa(execCount),
				"exits":   strconv.Itoa(exitCount),
				"mode":    c.modeString(),
			},
		})
	}
}

// isTimeout 检查错误是否为超时
func isTimeout(err error) bool {
	if errno, ok := err.(syscall.Errno); ok {
		return errno == syscall.EAGAIN || errno == syscall.EWOULDBLOCK ||
			errno == syscall.EINTR
	}
	return strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "resource temporarily unavailable")
}
