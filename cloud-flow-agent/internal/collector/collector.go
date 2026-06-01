// Package collector 从 /proc 文件系统采集系统指标
// 无外部依赖，适用于 Linux 环境
package collector

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	edge "cloud-flow/proto"
)

// nvmePartitionRegex 预编译 NVMe 分区正则表达式，避免每次调用 isPartition 时重复编译
var nvmePartitionRegex = regexp.MustCompile(`^nvme\d+n\d+p\d+$`)

type CollectConfig struct {
	CPU     bool
	Memory  bool
	Network bool
	Disk    bool
}

type Collector struct {
	cfg             CollectConfig
	lastCPUUser     uint64
	lastCPUSystem   uint64
	lastCPUIdle     uint64
	lastCPURead     bool
	lastNetRxBytes  uint64
	lastNetTxBytes  uint64
	lastNetRead     bool
	lastDiskReads   uint64
	lastDiskWrites  uint64
	lastDiskRead    bool
	lastCollectTime time.Time
	mu              sync.Mutex
	log             Logger // 可选的日志接口，用于记录警告信息
}

// Logger 日志接口，避免对具体日志库的依赖
type Logger interface {
	Warnf(format string, args ...interface{})
}

func New(cfg CollectConfig) *Collector {
	return &Collector{cfg: cfg}
}

// SetLogger 设置日志记录器
func (c *Collector) SetLogger(log Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = log
}

// Collect 采集当前系统指标
func (c *Collector) Collect() ([]*edge.MetricData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()
	var metrics []*edge.MetricData

	if c.cfg.CPU {
		if m := c.collectCPU(now); m != nil {
			metrics = append(metrics, m...)
		}
	}
	if c.cfg.Memory {
		if m := c.collectMemory(now); m != nil {
			metrics = append(metrics, m...)
		}
	}
	if c.cfg.Network {
		if m, err := c.collectNetwork(now); err != nil {
			return nil, fmt.Errorf("采集网络指标失败: %w", err)
		} else if m != nil {
			metrics = append(metrics, m...)
		}
	}
	if c.cfg.Disk {
		if m := c.collectDisk(now); m != nil {
			metrics = append(metrics, m...)
		}
	}

	c.lastCollectTime = time.Now()
	return metrics, nil
}

// collectCPU 从 /proc/stat 读取 CPU 使用率
func (c *Collector) collectCPU(now int64) []*edge.MetricData {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return nil
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return nil
	}

	user, _ := strconv.ParseUint(fields[1], 10, 64)
	system, _ := strconv.ParseUint(fields[3], 10, 64)
	idle, _ := strconv.ParseUint(fields[4], 10, 64)

	var cpuPercent int64
	if c.lastCPURead {
		dUser := user - c.lastCPUUser
		dSystem := system - c.lastCPUSystem
		dIdle := idle - c.lastCPUIdle
		total := dUser + dSystem + dIdle
		if total > 0 {
			cpuPercent = int64((dUser + dSystem) * 10000 / total) // 放大100倍存整数
		}
	}

	c.lastCPUUser = user
	c.lastCPUSystem = system
	c.lastCPUIdle = idle
	c.lastCPURead = true

	return []*edge.MetricData{{
		Timestamp: now,
		SrcIp:     "localhost",
		DstIp:     "cpu",
		Protocol:  "cpu",
		Bytes:     cpuPercent,
		Tags:      map[string]string{"type": "cpu_percent", "unit": "percent_x100", "cpu_usage": fmt.Sprintf("%.2f", float64(cpuPercent)/100.0)},
	}}
}

// collectMemory 从 /proc/meminfo 读取内存信息
func (c *Collector) collectMemory(now int64) []*edge.MetricData {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil
	}
	defer f.Close()

	var totalKB, availableKB uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			totalKB = parseMemLine(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			availableKB = parseMemLine(line)
		}
	}

	usedKB := totalKB - availableKB
	var usedPercent int64
	if totalKB > 0 {
		usedPercent = int64(usedKB * 10000 / totalKB)
	}

	return []*edge.MetricData{{
		Timestamp: now,
		SrcIp:     "localhost",
		DstIp:     "memory",
		Protocol:  "memory",
		Bytes:     int64(usedKB * 1024),
		Packets:   int64(totalKB * 1024),
		Latency:   usedPercent,
		Tags:      map[string]string{"type": "memory", "available_kb": fmt.Sprintf("%d", availableKB), "memory_usage": fmt.Sprintf("%.2f", float64(usedPercent)/100.0)},
	}}
}

// collectNetwork 从 /proc/net/dev 读取网络流量（差值）
func (c *Collector) collectNetwork(now int64) ([]*edge.MetricData, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, fmt.Errorf("打开 /proc/net/dev 失败: %w", err)
	}
	defer f.Close()

	var totalRx, totalTx uint64
	scanner := bufio.NewScanner(f)
	
	// 自动跳过 header 行，直到找到包含网络接口数据的行
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// 检查第一个字段是否以冒号结尾（网络接口名称的特征）
		if strings.HasSuffix(fields[0], ":") {
			// 找到数据行，开始处理
			name := strings.TrimSuffix(fields[0], ":")
			if name == "lo" {
				continue
			}
			rx, _ := strconv.ParseUint(fields[1], 10, 64)
			tx, _ := strconv.ParseUint(fields[9], 10, 64)
			totalRx += rx
			totalTx += tx
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 /proc/net/dev 失败: %w", err)
	}

	var dRx, dTx int64
	if c.lastNetRead {
		// 检测计数器回绕：如果当前值小于上次值，说明计数器被重置
		if totalRx < c.lastNetRxBytes {
			if c.log != nil {
				c.log.Warnf("网络接收计数器回绕检测: 当前 %d < 上次 %d，差值设为 0", totalRx, c.lastNetRxBytes)
			}
			dRx = 0
		} else {
			dRx = int64(totalRx - c.lastNetRxBytes)
		}
		if totalTx < c.lastNetTxBytes {
			if c.log != nil {
				c.log.Warnf("网络发送计数器回绕检测: 当前 %d < 上次 %d，差值设为 0", totalTx, c.lastNetTxBytes)
			}
			dTx = 0
		} else {
			dTx = int64(totalTx - c.lastNetTxBytes)
		}
	}

	c.lastNetRxBytes = totalRx
	c.lastNetTxBytes = totalTx
	c.lastNetRead = true

	return []*edge.MetricData{{
		Timestamp: now,
		SrcIp:     "localhost",
		DstIp:     "network",
		Protocol:  "network",
		Bytes:     dTx,
		Packets:   dRx,
		Tags:      map[string]string{"type": "network", "unit": "bytes_delta"},
	}}, nil
}

func parseMemLine(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	val, _ := strconv.ParseUint(fields[1], 10, 64)
	return val
}

// hasDigitSuffix 检查字符串是否以数字结尾
func hasDigitSuffix(s string) bool {
	if len(s) == 0 {
		return false
	}
	lastChar := s[len(s)-1]
	return lastChar >= '0' && lastChar <= '9'
}

// collectDisk 从 /proc/diskstats 读取磁盘 IO 统计
func (c *Collector) collectDisk(now int64) []*edge.MetricData {
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil
	}
	defer f.Close()

	var totalReads, totalWrites uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}
		// 只统计物理磁盘，跳过分区和 loop 设备
		diskName := fields[2]
		if !strings.HasPrefix(diskName, "loop") && !isPartition(diskName) {
			reads, _ := strconv.ParseUint(fields[5], 10, 64)
			writes, _ := strconv.ParseUint(fields[9], 10, 64)
			totalReads += reads
			totalWrites += writes
		}
	}

	var dReads, dWrites int64
	if c.lastDiskRead {
		dReads = int64(totalReads - c.lastDiskReads)
		dWrites = int64(totalWrites - c.lastDiskWrites)
		// 防止计数器重置（如磁盘热插拔）导致负数差值
		if dReads < 0 {
			dReads = 0
		}
		if dWrites < 0 {
			dWrites = 0
		}
	}

	c.lastDiskReads = totalReads
	c.lastDiskWrites = totalWrites
	c.lastDiskRead = true

	return []*edge.MetricData{{
		Timestamp: now,
		SrcIp:     "localhost",
		DstIp:     "disk",
		Protocol:  "disk",
		Bytes:     dWrites,
		Packets:   dReads,
		Tags:      map[string]string{"type": "disk_io", "unit": "operations_delta"},
	}}
}

// isPartition 检查是否为分区
func isPartition(diskName string) bool {
	// 对于 NVMe 磁盘，使用预编译的正则表达式匹配分区格式 nvme0n1p1
	if nvmePartitionRegex.MatchString(diskName) {
		return true
	}
	// 对于其他磁盘，检查是否以数字结尾
	return hasDigitSuffix(diskName)
}
