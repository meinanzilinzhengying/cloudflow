package cgroup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	cgroupV2Root = "/sys/fs/cgroup"
	cgroupName   = "cloud-flow-agent"
)

// Manager cgroup v2 资源管理器
type Manager struct {
	cgroupRoot string
	cgroupPath string
	pid        int
	mu         sync.RWMutex
	enabled    bool
}

// Config 资源配置
type Config struct {
	MaxCPUCores float64 // 最大CPU核心数 (如 1.0)
	MaxMemoryMB int64   // 最大内存(MB) (如 1024)
	MaxPids     int64   // 最大进程数 (可选)
}

// NewManager 创建cgroup管理器
func NewManager(cfg *Config) (*Manager, error) {
	// 检测cgroup v2是否可用
	if !detectCgroupV2() {
		return nil, fmt.Errorf("cgroup v2 未挂载或不可用")
	}

	// 检查是否有权限写入cgroup
	if err := checkCgroupWritable(); err != nil {
		return nil, fmt.Errorf("cgroup 不可写: %w", err)
	}

	pid := os.Getpid()
	cgroupPath := filepath.Join(cgroupV2Root, cgroupName)

	m := &Manager{
		cgroupRoot: cgroupV2Root,
		cgroupPath: cgroupPath,
		pid:        pid,
		enabled:    true,
	}

	// 创建cgroup目录
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return nil, fmt.Errorf("创建cgroup目录失败: %w", err)
	}

	// 配置CPU限制
	if cfg.MaxCPUCores > 0 {
		cpuLimit := parseCPULimit(cfg.MaxCPUCores)
		cpuMaxPath := filepath.Join(cgroupPath, "cpu.max")
		if err := writeCgroupFile(cpuMaxPath, cpuLimit); err != nil {
			// 清理已创建的目录
			_ = os.RemoveAll(cgroupPath)
			return nil, fmt.Errorf("写入cpu.max失败: %w", err)
		}
	}

	// 配置内存限制
	if cfg.MaxMemoryMB > 0 {
		memoryLimit := parseMemoryLimit(cfg.MaxMemoryMB)
		memoryMaxPath := filepath.Join(cgroupPath, "memory.max")
		if err := writeCgroupFile(memoryMaxPath, memoryLimit); err != nil {
			// 清理已创建的目录
			_ = os.RemoveAll(cgroupPath)
			return nil, fmt.Errorf("写入memory.max失败: %w", err)
		}
	}

	// 配置进程数限制（可选）
	if cfg.MaxPids > 0 {
		pidsMaxPath := filepath.Join(cgroupPath, "pids.max")
		if err := writeCgroupFile(pidsMaxPath, strconv.FormatInt(cfg.MaxPids, 10)); err != nil {
			// 进程数限制失败不是致命错误，仅记录
			fmt.Fprintf(os.Stderr, "[Cgroup] 写入pids.max失败: %v\n", err)
		}
	}

	return m, nil
}

// ApplyToCurrentProcess 将当前进程加入cgroup
func (m *Manager) ApplyToCurrentProcess() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled {
		return fmt.Errorf("cgroup管理器未启用")
	}

	// 写入cgroup.procs文件将当前进程加入cgroup
	procsPath := filepath.Join(m.cgroupPath, "cgroup.procs")
	pidStr := strconv.Itoa(m.pid)

	if err := writeCgroupFile(procsPath, pidStr); err != nil {
		return fmt.Errorf("将进程加入cgroup失败: %w", err)
	}

	return nil
}

// Close 清理cgroup
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled {
		return nil
	}

	// 将进程移回根cgroup
	procsPath := filepath.Join(cgroupV2Root, "cgroup.procs")
	pidStr := strconv.Itoa(m.pid)
	_ = writeCgroupFile(procsPath, pidStr)

	// 删除cgroup目录
	// 注意：只有当cgroup为空时才能删除
	if err := os.Remove(m.cgroupPath); err != nil {
		// 如果删除失败，可能是cgroup中还有子进程
		// 这不是致命错误，系统重启后会自动清理
		return fmt.Errorf("清理cgroup目录失败: %w", err)
	}

	m.enabled = false
	return nil
}

// IsEnabled 返回cgroup管理器是否启用
func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// GetCgroupPath 返回cgroup路径
func (m *Manager) GetCgroupPath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cgroupPath
}

// detectCgroupV2 检测cgroup v2是否挂载
func detectCgroupV2() bool {
	// 检查cgroup v2根目录是否存在
	info, err := os.Stat(cgroupV2Root)
	if err != nil || !info.IsDir() {
		return false
	}

	// 检查cgroup.controllers文件是否存在（cgroup v2特有）
	controllersPath := filepath.Join(cgroupV2Root, "cgroup.controllers")
	if _, err := os.Stat(controllersPath); err != nil {
		return false
	}

	// 检查cgroup.subtree_control是否存在
	subtreeControlPath := filepath.Join(cgroupV2Root, "cgroup.subtree_control")
	if _, err := os.Stat(subtreeControlPath); err != nil {
		return false
	}

	return true
}

// checkCgroupWritable 检查cgroup是否可写
func checkCgroupWritable() error {
	// 尝试读取cgroup.procs检查权限
	procsPath := filepath.Join(cgroupV2Root, "cgroup.procs")
	if _, err := os.Stat(procsPath); err != nil {
		return fmt.Errorf("无法访问cgroup.procs: %w", err)
	}

	// 尝试读取cgroup.subtree_control检查是否可以启用控制器
	subtreeControlPath := filepath.Join(cgroupV2Root, "cgroup.subtree_control")
	data, err := os.ReadFile(subtreeControlPath)
	if err != nil {
		// 可能是权限问题，尝试检查是否以root运行
		if os.Geteuid() != 0 {
			return fmt.Errorf("需要root权限才能管理cgroup")
		}
		return fmt.Errorf("无法读取cgroup.subtree_control: %w", err)
	}

	// 检查必要的控制器是否可用
	controllers := strings.Fields(string(data))
	hasCPU := false
	hasMemory := false
	for _, c := range controllers {
		if c == "cpu" || c == "+cpu" {
			hasCPU = true
		}
		if c == "memory" || c == "+memory" {
			hasMemory = true
		}
	}

	if !hasCPU {
		// 尝试启用cpu控制器
		if err := writeCgroupFile(subtreeControlPath, "+cpu"); err != nil {
			fmt.Fprintf(os.Stderr, "[Cgroup] 警告: cpu控制器不可用: %v\n", err)
		}
	}

	if !hasMemory {
		// 尝试启用memory控制器
		if err := writeCgroupFile(subtreeControlPath, "+memory"); err != nil {
			fmt.Fprintf(os.Stderr, "[Cgroup] 警告: memory控制器不可用: %v\n", err)
		}
	}

	return nil
}

// writeCgroupFile 写入cgroup文件
func writeCgroupFile(path, value string) error {
	// 确保值以换行符结尾（cgroup文件要求）
	if !strings.HasSuffix(value, "\n") {
		value += "\n"
	}

	return os.WriteFile(path, []byte(value), 0644)
}

// parseMemoryLimit 将MB转换为bytes
func parseMemoryLimit(mb int64) string {
	bytes := mb * 1024 * 1024
	return strconv.FormatInt(bytes, 10)
}

// parseCPULimit 将核心数转换为cpu.max格式
// 格式: "quota period" (如 "100000 100000" 表示 1 核)
// 1 核 = 100000us / 100000us
// 0.5 核 = 50000us / 100000us
func parseCPULimit(cores float64) string {
	const period = 100000 // 默认周期为100ms (100000us)

	if cores <= 0 {
		// 无限制
		return "max " + strconv.Itoa(period)
	}

	quota := int64(cores * period)
	return strconv.FormatInt(quota, 10) + " " + strconv.Itoa(period)
}
