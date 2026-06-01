// Package dbobserver 提供数据库观测功能
// 本文件实现数据库进程 CPU/IO 指标关联
package dbobserver

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ==================== 进程指标采集器 ====================

// ProcessMetricsCollector 进程指标采集器
type ProcessMetricsCollector struct {
	config       *ObserverConfig

	// 进程指标缓存
	metrics      map[uint32]*ProcessMetrics
	metricsMu    sync.RWMutex

	// 数据库进程映射
	dbProcesses  map[uint32]*DBProcessInfo
	dbProcMu     sync.RWMutex

	// 历史指标（用于计算增量）
	lastMetrics  map[uint32]*ProcessMetrics
	lastTime     map[uint32]time.Time
	historyMu    sync.RWMutex

	// 采集间隔
	collectInterval time.Duration

	// 回调函数
	onMetricsUpdate func(pid uint32, metrics *ProcessMetrics)
}

// DBProcessInfo 数据库进程信息
type DBProcessInfo struct {
	PID          uint32       `json:"pid"`
	ProcessName  string       `json:"process_name"`
	DatabaseType DatabaseType `json:"database_type"`
	Port         int          `json:"port"`
	DataDir      string       `json:"data_dir"`
	ConfigPath   string       `json:"config_path"`
	StartTime    time.Time    `json:"start_time"`
}

// NewProcessMetricsCollector 创建进程指标采集器
func NewProcessMetricsCollector(cfg *ObserverConfig) *ProcessMetricsCollector {
	return &ProcessMetricsCollector{
		config:          cfg,
		metrics:         make(map[uint32]*ProcessMetrics),
		dbProcesses:     make(map[uint32]*DBProcessInfo),
		lastMetrics:     make(map[uint32]*ProcessMetrics),
		lastTime:        make(map[uint32]time.Time),
		collectInterval: cfg.ProcessMetricsInterval,
	}
}

// SetMetricsUpdateCallback 设置指标更新回调
func (c *ProcessMetricsCollector) SetMetricsUpdateCallback(callback func(pid uint32, metrics *ProcessMetrics)) {
	c.onMetricsUpdate = callback
}

// RegisterDBProcess 注册数据库进程
func (c *ProcessMetricsCollector) RegisterDBProcess(info *DBProcessInfo) {
	c.dbProcMu.Lock()
	defer c.dbProcMu.Unlock()
	c.dbProcesses[info.PID] = info
}

// UnregisterDBProcess 注销数据库进程
func (c *ProcessMetricsCollector) UnregisterDBProcess(pid uint32) {
	c.dbProcMu.Lock()
	defer c.dbProcMu.Unlock()
	delete(c.dbProcesses, pid)

	c.metricsMu.Lock()
	delete(c.metrics, pid)
	c.metricsMu.Unlock()

	c.historyMu.Lock()
	delete(c.lastMetrics, pid)
	delete(c.lastTime, pid)
	c.historyMu.Unlock()
}

// GetDBProcess 获取数据库进程信息
func (c *ProcessMetricsCollector) GetDBProcess(pid uint32) *DBProcessInfo {
	c.dbProcMu.RLock()
	defer c.dbProcMu.RUnlock()
	return c.dbProcesses[pid]
}

// Collect 采集进程指标
func (c *ProcessMetricsCollector) Collect() {
	c.dbProcMu.RLock()
	processes := make([]*DBProcessInfo, 0, len(c.dbProcesses))
	for _, proc := range c.dbProcesses {
		processes = append(processes, proc)
	}
	c.dbProcMu.RUnlock()

	for _, proc := range processes {
		metrics := c.collectProcessMetrics(proc.PID)
		if metrics != nil {
			metrics.DatabaseType = proc.DatabaseType
			metrics.ProcessName = proc.ProcessName

			// 存储指标
			c.metricsMu.Lock()
			c.metrics[proc.PID] = metrics
			c.metricsMu.Unlock()

			// 调用回调
			if c.onMetricsUpdate != nil {
				c.onMetricsUpdate(proc.PID, metrics)
			}
		}
	}
}

// collectProcessMetrics 采集单个进程的指标
func (c *ProcessMetricsCollector) collectProcessMetrics(pid uint32) *ProcessMetrics {
	metrics := &ProcessMetrics{
		PID:       pid,
		Timestamp: time.Now(),
	}

	// 读取 /proc/[pid]/stat 获取 CPU 信息
	if err := c.readProcessStat(pid, metrics); err != nil {
		return nil
	}

	// 读取 /proc/[pid]/statm 获取内存信息
	c.readProcessStatm(pid, metrics)

	// 读取 /proc/[pid]/io 获取 IO 信息
	c.readProcessIO(pid, metrics)

	// 读取 /proc/[pid]/fd 获取连接数
	c.readProcessFD(pid, metrics)

	// 计算增量指标
	c.calculateDeltaMetrics(pid, metrics)

	return metrics
}

// readProcessStat 读取进程状态信息
func (c *ProcessMetricsCollector) readProcessStat(pid uint32, metrics *ProcessMetrics) error {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return err
	}

	// /proc/[pid]/stat 格式复杂，需要正确解析
	// 格式: pid (comm) state ppid pgrp session tty_nr tpgid flags ...
	fields := strings.Fields(string(data))
	if len(fields) < 17 {
		return fmt.Errorf("stat 格式错误")
	}

	// 找到进程名（可能在括号中包含空格）
	commStart := strings.Index(string(data), "(")
	commEnd := strings.LastIndex(string(data), ")")
	if commStart == -1 || commEnd == -1 {
		return fmt.Errorf("无法解析进程名")
	}

	// 重新解析字段（从 ) 之后开始）
	afterComm := string(data)[commEnd+2:]
	fields = strings.Fields(afterComm)
	if len(fields) < 15 {
		return fmt.Errorf("stat 字段不足")
	}

	// 解析 CPU 时间
	// fields[11] = utime, fields[12] = stime (单位: jiffies)
	utime, _ := strconv.ParseInt(fields[11], 10, 64)
	stime, _ := strconv.ParseInt(fields[12], 10, 64)

	metrics.UserCPUTime = utime
	metrics.SysCPUTime = stime
	metrics.CPUTime = utime + stime

	// 解析线程数
	// fields[17] = num_threads (从原始位置计算)
	if len(fields) >= 18 {
		threads, _ := strconv.Atoi(fields[17])
		metrics.ThreadCount = threads
	}

	return nil
}

// readProcessStatm 读取进程内存信息
func (c *ProcessMetricsCollector) readProcessStatm(pid uint32, metrics *ProcessMetrics) {
	statmPath := fmt.Sprintf("/proc/%d/statm", pid)
	data, err := os.ReadFile(statmPath)
	if err != nil {
		return
	}

	fields := strings.Fields(string(data))
	if len(fields) < 7 {
		return
	}

	// 字段说明（单位：页）
	// size, resident, shared, text, lib, data, dt
	pageSize := int64(os.Getpagesize())

	resident, _ := strconv.ParseInt(fields[1], 10, 64)
	size, _ := strconv.ParseInt(fields[0], 10, 64)

	metrics.MemoryRSS = resident * pageSize
	metrics.MemoryVMS = size * pageSize
	metrics.MemoryUsage = metrics.MemoryRSS
}

// readProcessIO 读取进程 IO 信息
func (c *ProcessMetricsCollector) readProcessIO(pid uint32, metrics *ProcessMetrics) {
	ioPath := fmt.Sprintf("/proc/%d/io", pid)
	file, err := os.Open(ioPath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)

		switch key {
		case "read_bytes":
			metrics.IOReadBytes = value
		case "write_bytes":
			metrics.IOWriteBytes = value
		case "rchar":
			// 读取的总字符数（包括缓存）
		case "wchar":
			// 写入的总字符数（包括缓存）
		case "syscr":
			metrics.IOReadOps = value
		case "syscw":
			metrics.IOWriteOps = value
		}
	}
}

// readProcessFD 读取进程文件描述符信息
func (c *ProcessMetricsCollector) readProcessFD(pid uint32, metrics *ProcessMetrics) {
	fdPath := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdPath)
	if err != nil {
		return
	}

	connectionCount := 0
	for _, entry := range entries {
		linkPath := filepath.Join(fdPath, entry.Name())
		link, err := os.Readlink(linkPath)
		if err != nil {
			continue
		}

		// 检查是否是 socket
		if strings.HasPrefix(link, "socket:") {
			connectionCount++
		}
	}

	metrics.ConnectionCount = connectionCount
}

// calculateDeltaMetrics 计算增量指标
func (c *ProcessMetricsCollector) calculateDeltaMetrics(pid uint32, current *ProcessMetrics) {
	c.historyMu.Lock()
	defer c.historyMu.Unlock()

	last, hasLast := c.lastMetrics[pid]
	lastTime, hasTime := c.lastTime[pid]

	if hasLast && hasTime {
		// 计算时间间隔
		duration := current.Timestamp.Sub(lastTime).Seconds()
		if duration > 0 {
			// 计算 CPU 使用率
			cpuDelta := float64(current.CPUTime - last.CPUTime)
			// CPU 使用率 = CPU 时间增量 / (时间间隔 * 100)
			// 注意：需要考虑 CPU 核心数
			cpuCores := getCPUCount()
			current.CPUUsage = (cpuDelta / (duration * 100.0)) / float64(cpuCores) * 100.0
			if current.CPUUsage > 100.0 {
				current.CPUUsage = 100.0
			}
		}
	}

	// 更新历史记录
	c.lastMetrics[pid] = current
	c.lastTime[pid] = current.Timestamp
}

// GetMetrics 获取进程指标
func (c *ProcessMetricsCollector) GetMetrics(pid uint32) *ProcessMetrics {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()
	return c.metrics[pid]
}

// GetAllMetrics 获取所有进程指标
func (c *ProcessMetricsCollector) GetAllMetrics() map[uint32]*ProcessMetrics {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	result := make(map[uint32]*ProcessMetrics)
	for k, v := range c.metrics {
		result[k] = v
	}
	return result
}

// Close 关闭采集器
func (c *ProcessMetricsCollector) Close() {
	c.metricsMu.Lock()
	c.metrics = make(map[uint32]*ProcessMetrics)
	c.metricsMu.Unlock()

	c.historyMu.Lock()
	c.lastMetrics = make(map[uint32]*ProcessMetrics)
	c.lastTime = make(map[uint32]time.Time)
	c.historyMu.Unlock()
}

// ==================== 数据库进程发现 ====================

// DBProcessDiscoverer 数据库进程发现器
type DBProcessDiscoverer struct {
	// 数据库进程名映射
	processNames map[DatabaseType][]string
}

// NewDBProcessDiscoverer 创建数据库进程发现器
func NewDBProcessDiscoverer() *DBProcessDiscoverer {
	return &DBProcessDiscoverer{
		processNames: map[DatabaseType][]string{
			DatabaseTypeMySQL:      {"mysqld", "mysql"},
			DatabaseTypeOracle:     {"oracle", "ora_", "tnslsnr"},
			DatabaseTypePostgreSQL: {"postgres", "postmaster"},
			DatabaseTypeDaMeng:     {"dmserver", "dmagent", "dmap"},
			DatabaseTypeGaussDB:    {"gaussdb", "postgres", "gs_ctl"},
		},
	}
}

// Discover 发现数据库进程
func (d *DBProcessDiscoverer) Discover() []*DBProcessInfo {
	processes := make([]*DBProcessInfo, 0)

	// 读取 /proc 目录
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return processes
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// 检查是否是进程目录（数字）
		pid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue
		}

		// 获取进程信息
		info := d.getProcessInfo(uint32(pid))
		if info != nil {
			processes = append(processes, info)
		}
	}

	return processes
}

// getProcessInfo 获取进程信息
func (d *DBProcessDiscoverer) getProcessInfo(pid uint32) *DBProcessInfo {
	// 读取 /proc/[pid]/comm 获取进程名
	commPath := fmt.Sprintf("/proc/%d/comm", pid)
	commData, err := os.ReadFile(commPath)
	if err != nil {
		return nil
	}
	comm := strings.TrimSpace(string(commData))

	// 检查是否是数据库进程
	var dbType DatabaseType
	var found bool

	for dt, names := range d.processNames {
		for _, name := range names {
			if strings.HasPrefix(comm, name) || comm == name {
				dbType = dt
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return nil
	}

	// 获取进程详细信息
	info := &DBProcessInfo{
		PID:          pid,
		ProcessName:  comm,
		DatabaseType: dbType,
	}

	// 读取命令行参数获取端口等信息
	d.readProcessCmdline(pid, info)

	// 获取进程启动时间
	d.getProcessStartTime(pid, info)

	return info
}

// readProcessCmdline 读取进程命令行参数
func (d *DBProcessDiscoverer) readProcessCmdline(pid uint32, info *DBProcessInfo) {
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return
	}

	// cmdline 使用 null 字节分隔参数
	args := strings.Split(string(data), "\x00")

	for i, arg := range args {
		// 查找端口参数
		if arg == "-p" && i+1 < len(args) {
			port, _ := strconv.Atoi(args[i+1])
			info.Port = port
		}
		// 查找数据目录
		if (arg == "--datadir" || arg == "-D") && i+1 < len(args) {
			info.DataDir = args[i+1]
		}
		// 查找配置文件
		if arg == "--defaults-file" && i+1 < len(args) {
			info.ConfigPath = args[i+1]
		}
	}
}

// getProcessStartTime 获取进程启动时间
func (d *DBProcessDiscoverer) getProcessStartTime(pid uint32, info *DBProcessInfo) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return
	}

	fields := strings.Fields(string(data))
	if len(fields) < 22 {
		return
	}

	// 字段 22 是进程启动时间（jiffies since boot）
	startTime, _ := strconv.ParseInt(fields[21], 10, 64)

	// 转换为时间（简化处理）
	// 实际需要获取系统启动时间并计算
	info.StartTime = time.Unix(startTime/100, 0)
}

// ==================== 网络指标采集 ====================

// NetworkMetricsCollector 网络指标采集器
type NetworkMetricsCollector struct {
	// 上次采集的指标
	lastNetStats map[string]*NetDevStats
	lastTime     time.Time
	mu           sync.RWMutex
}

// NetDevStat 网络设备统计
type NetDevStat struct {
	Interface string
	RxBytes   uint64
	RxPackets uint64
	RxErrors  uint64
	TxDropped uint64
	TxBytes   uint64
	TxPackets uint64
	TxErrors  uint64
	TxDropped uint64
}

// NetDevStat 网络设备统计（用于计算）
type NetDevStats struct {
	Interface  string
	RxBytes    uint64
	RxPackets  uint64
	RxErrors   uint64
	RxDropped  uint64
	TxBytes    uint64
	TxPackets  uint64
	TxErrors   uint64
	TxDropped  uint64
}

// NewNetworkMetricsCollector 创建网络指标采集器
func NewNetworkMetricsCollector() *NetworkMetricsCollector {
	return &NetworkMetricsCollector{
		lastNetStats: make(map[string]*NetDevStat),
	}
}

// Collect 采集网络指标
func (c *NetworkMetricsCollector) Collect() map[string]*NetDevStat {
	// 读取 /proc/net/dev
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil
	}
	defer file.Close()

	stats := make(map[string]*NetDevStat)
	scanner := bufio.NewScanner(file)

	// 跳过前两行标题
	scanner.Scan()
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 17 {
			continue
		}

		// 解析网络设备统计
		iface := strings.TrimSuffix(fields[0], ":")
		stat := &NetDevStat{
			Interface: iface,
		}

		// 接收统计
		stat.RxBytes, _ = strconv.ParseUint(fields[1], 10, 64)
		stat.RxPackets, _ = strconv.ParseUint(fields[2], 10, 64)
		stat.RxErrors, _ = strconv.ParseUint(fields[3], 10, 64)
		// fields[4] = rx dropped, fields[5] = rx fifo, fields[6] = rx frame, fields[7] = rx compressed, fields[8] = rx multicast

		// 发送统计
		stat.TxBytes, _ = strconv.ParseUint(fields[9], 10, 64)
		stat.TxPackets, _ = strconv.ParseUint(fields[10], 10, 64)
		stat.TxErrors, _ = strconv.ParseUint(fields[11], 10, 64)
		// fields[12] = tx dropped, ...

		stats[iface] = stat
	}

	c.mu.Lock()
	c.lastNetStats = make(map[string]*NetDevStat)
	for k, v := range stats {
		c.lastNetStats[k] = v
	}
	c.lastTime = time.Now()
	c.mu.Unlock()

	return stats
}

// GetDelta 获取增量指标
func (c *NetworkMetricsCollector) GetDelta(iface string) *NetDevStat {
	c.mu.RLock()
	defer c.mu.RUnlock()

	current := c.lastNetStats[iface]
	if current == nil {
		return nil
	}

	return current
}

// ==================== 磁盘 IO 指标采集 ====================

// DiskIOMetricsCollector 磁盘 IO 指标采集器
type DiskIOMetricsCollector struct {
	lastStats map[string]*DiskIOStat
	lastTime  time.Time
	mu        sync.RWMutex
}

// DiskIOStat 磁盘 IO 统计
type DiskIOStat struct {
	Device       string
	Reads        uint64  // 读次数
	Writes       uint64  // 写次数
	ReadBytes    uint64  // 读字节数
	WriteBytes   uint64  // 写字节数
	ReadTime     uint64  // 读时间（毫秒）
	WriteTime    uint64  // 写时间（毫秒）
	IOPSTotal    uint64  // IOPS 总数
	Throughput   uint64  // 吞吐量（字节/秒）
	AvgLatency   float64 // 平均延迟（毫秒）
}

// NewDiskIOMetricsCollector 创建磁盘 IO 指标采集器
func NewDiskIOMetricsCollector() *DiskIOMetricsCollector {
	return &DiskIOMetricsCollector{
		lastStats: make(map[string]*DiskIOStat),
	}
}

// Collect 采集磁盘 IO 指标
func (c *DiskIOMetricsCollector) Collect() map[string]*DiskIOStat {
	// 读取 /proc/diskstats
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil
	}
	defer file.Close()

	stats := make(map[string]*DiskIOStat)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}

		// 解析磁盘统计
		// 格式: major minor name reads_completed reads_merged sectors_read ms_reading writes_completed writes_merged sectors_written ms_writing io_pending ms_io weighted_ms_io
		device := fields[2]
		stat := &DiskIOStat{
			Device: device,
		}

		// 读统计
		stat.Reads, _ = strconv.ParseUint(fields[3], 10, 64)
		// fields[4] = reads merged
		sectorsRead, _ := strconv.ParseUint(fields[5], 10, 64)
		stat.ReadBytes = sectorsRead * 512 // 每扇区 512 字节
		stat.ReadTime, _ = strconv.ParseUint(fields[6], 10, 64)

		// 写统计
		stat.Writes, _ = strconv.ParseUint(fields[7], 10, 64)
		// fields[8] = writes merged
		sectorsWritten, _ := strconv.ParseUint(fields[9], 10, 64)
		stat.WriteBytes = sectorsWritten * 512
		stat.WriteTime, _ = strconv.ParseUint(fields[10], 10, 64)

		// 计算汇总指标
		stat.IOPSTotal = stat.Reads + stat.Writes

		stats[device] = stat
	}

	c.mu.Lock()
	c.lastStats = stats
	c.lastTime = time.Now()
	c.mu.Unlock()

	return stats
}

// GetStats 获取磁盘统计
func (c *DiskIOMetricsCollector) GetStats(device string) *DiskIOStat {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastStats[device]
}

// ==================== CPU 核心数缓存 ====================

var cpuCount int
var cpuCountOnce sync.Once

func getCPUCount() int {
	cpuCountOnce.Do(func() {
		cpuCount = 1
		if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			cpuCount = strings.Count(string(data), "processor")
			if cpuCount < 1 {
				cpuCount = 1
			}
		}
	})
	return cpuCount
}

// ==================== 进程指标聚合 ====================

// ProcessMetricsAggregator 进程指标聚合器
type ProcessMetricsAggregator struct {
	collector   *ProcessMetricsCollector
	aggregation map[DatabaseType]*AggregatedMetrics
	mu          sync.RWMutex
}

// AggregatedMetrics 聚合指标
type AggregatedMetrics struct {
	DatabaseType    DatabaseType
	ProcessCount    int
	TotalCPU        float64
	TotalMemory     int64
	TotalIORead     int64
	TotalIOWrite    int64
	TotalConnCount  int
	AvgCPU          float64
	AvgMemory       int64
	MaxCPU          float64
	MaxMemory       int64
	UpdateTime      time.Time
}

// NewProcessMetricsAggregator 创建进程指标聚合器
func NewProcessMetricsAggregator(collector *ProcessMetricsCollector) *ProcessMetricsAggregator {
	return &ProcessMetricsAggregator{
		collector:   collector,
		aggregation: make(map[DatabaseType]*AggregatedMetrics),
	}
}

// Aggregate 聚合指标
func (a *ProcessMetricsAggregator) Aggregate() map[DatabaseType]*AggregatedMetrics {
	allMetrics := a.collector.GetAllMetrics()

	a.mu.Lock()
	defer a.mu.Unlock()

	// 按数据库类型分组聚合
	grouped := make(map[DatabaseType][]*ProcessMetrics)
	for _, m := range allMetrics {
		grouped[m.DatabaseType] = append(grouped[m.DatabaseType], m)
	}

	// 计算聚合指标
	for dbType, metrics := range grouped {
		agg := &AggregatedMetrics{
			DatabaseType: dbType,
			ProcessCount: len(metrics),
			UpdateTime:   time.Now(),
		}

		for _, m := range metrics {
			agg.TotalCPU += m.CPUUsage
			agg.TotalMemory += m.MemoryUsage
			agg.TotalIORead += m.IOReadBytes
			agg.TotalIOWrite += m.IOWriteBytes
			agg.TotalConnCount += m.ConnectionCount

			if m.CPUUsage > agg.MaxCPU {
				agg.MaxCPU = m.CPUUsage
			}
			if m.MemoryUsage > agg.MaxMemory {
				agg.MaxMemory = m.MemoryUsage
			}
		}

		if len(metrics) > 0 {
			agg.AvgCPU = agg.TotalCPU / float64(len(metrics))
			agg.AvgMemory = agg.TotalMemory / int64(len(metrics))
		}

		a.aggregation[dbType] = agg
	}

	return a.aggregation
}

// GetAggregated 获取聚合指标
func (a *ProcessMetricsAggregator) GetAggregated(dbType DatabaseType) *AggregatedMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.aggregation[dbType]
}

// ==================== 指标关联器 ====================

// MetricsCorrelator 指标关联器
type MetricsCorrelator struct {
	processCollector  *ProcessMetricsCollector
	networkCollector  *NetworkMetricsCollector
	diskCollector     *DiskIOMetricsCollector
	aggregator        *ProcessMetricsAggregator

	// 关联规则
	correlations map[string]*CorrelationRule
	mu           sync.RWMutex
}

// CorrelationRule 关联规则
type CorrelationRule struct {
	Name        string
	Condition   func(*SQLEvent, *ProcessMetrics) bool
	Correlate   func(*SQLEvent, *ProcessMetrics) *CorrelatedEvent
}

// CorrelatedEvent 关联事件
type CorrelatedEvent struct {
	SQLEvent      *SQLEvent
	ProcessMetrics *ProcessMetrics
	NetworkMetrics *NetDevStat
	DiskMetrics   *DiskIOStat
	CorrelationScore float64
	CorrelationReason string
}

// NewMetricsCorrelator 创建指标关联器
func NewMetricsCorrelator() *MetricsCorrelator {
	collector := NewProcessMetricsCollector(DefaultObserverConfig())
	return &MetricsCorrelator{
		processCollector: collector,
		networkCollector: NewNetworkMetricsCollector(),
		diskCollector:    NewDiskIOMetricsCollector(),
		aggregator:       NewProcessMetricsAggregator(collector),
		correlations:     make(map[string]*CorrelationRule),
	}
}

// Correlate 关联 SQL 事件与进程指标
func (c *MetricsCorrelator) Correlate(event *SQLEvent) *CorrelatedEvent {
	correlated := &CorrelatedEvent{
		SQLEvent: event,
	}

	// 获取进程指标
	if event.PID > 0 {
		correlated.ProcessMetrics = c.processCollector.GetMetrics(event.PID)
	}

	// 应用关联规则
	c.mu.RLock()
	defer c.mu.RUnlock()

	for name, rule := range c.correlations {
		if rule.Condition(event, correlated.ProcessMetrics) {
			result := rule.Correlate(event, correlated.ProcessMetrics)
			if result != nil {
				correlated.CorrelationScore = result.CorrelationScore
				correlated.CorrelationReason = name + ": " + result.CorrelationReason
			}
		}
	}

	return correlated
}

// AddCorrelationRule 添加关联规则
func (c *MetricsCorrelator) AddCorrelationRule(rule *CorrelationRule) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.correlations[rule.Name] = rule
}

// CollectAll 采集所有指标
func (c *MetricsCorrelator) CollectAll() {
	c.processCollector.Collect()
	c.networkCollector.Collect()
	c.diskCollector.Collect()
	c.aggregator.Aggregate()
}

// Close 关闭关联器
func (c *MetricsCorrelator) Close() {
	c.processCollector.Close()
}
