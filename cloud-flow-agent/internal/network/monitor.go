package network

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// Monitor 网卡监控器
type Monitor struct {
	mgmtIP        string
	available     bool
	mu            sync.RWMutex
	stopCh        chan struct{}
	checkInterval time.Duration
	logger        *logger.Logger
	edgeAddr      string // Edge节点地址，用于连通性测试
}

// NewMonitor 创建网卡监控器
func NewMonitor(mgmtIP, edgeAddr string, log *logger.Logger) *Monitor {
	if mgmtIP == "" {
		// 如果没有指定管理IP，尝试自动获取
		mgmtIP = getDefaultIP()
	}
	return &Monitor{
		mgmtIP:        mgmtIP,
		edgeAddr:      edgeAddr,
		checkInterval: 5 * time.Second,
		stopCh:        make(chan struct{}),
		logger:        log,
		available:     true, // 初始状态设为可用
	}
}

// Start 启动监控
func (m *Monitor) Start() {
	m.logger.Infof("启动网卡监控器: mgmtIP=%s, checkInterval=%s", m.mgmtIP, m.checkInterval)
	go m.monitorLoop()
}

// Stop 停止监控
func (m *Monitor) Stop() {
	close(m.stopCh)
	m.logger.Info("网卡监控器已停止")
}

// IsAvailable 检查管理网卡是否可用
func (m *Monitor) IsAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.available
}

// GetMgmtIP 获取管理IP
func (m *Monitor) GetMgmtIP() string {
	return m.mgmtIP
}

// monitorLoop 监控循环
func (m *Monitor) monitorLoop() {
	// 立即执行一次检查
	m.checkAndUpdate()

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAndUpdate()
		}
	}
}

// checkAndUpdate 执行检查并更新状态
func (m *Monitor) checkAndUpdate() {
	available := m.checkAvailability()

	m.mu.Lock()
	prevAvailable := m.available
	m.available = available
	m.mu.Unlock()

	// 状态变化时记录日志
	if prevAvailable != available {
		if available {
			m.logger.Info("管理网卡已恢复可用")
		} else {
			m.logger.Warn("管理网卡不可用，将进入缓存模式")
		}
	}
}

// checkAvailability 执行可用性检查
func (m *Monitor) checkAvailability() bool {
	// 1. 检查指定IP是否在本机网卡上
	if !m.isIPIfLocal(m.mgmtIP) {
		m.logger.Debugf("管理IP %s 不在本机网卡上", m.mgmtIP)
		return false
	}

	// 2. 检查网卡是否UP（通过尝试绑定该IP来判断）
	if !m.isInterfaceUp(m.mgmtIP) {
		m.logger.Debugf("管理IP %s 对应的网卡未UP", m.mgmtIP)
		return false
	}

	// 3. 检查是否能与Edge节点通信（如果配置了Edge地址）
	if m.edgeAddr != "" {
		if !m.canConnectToEdge() {
			m.logger.Debugf("无法连接到Edge节点 %s", m.edgeAddr)
			return false
		}
	}

	return true
}

// isIPIfLocal 检查IP是否在本机网卡上
func (m *Monitor) isIPIfLocal(ip string) bool {
	if ip == "" {
		return false
	}

	// 获取所有网卡地址
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		m.logger.Warnf("获取网卡地址失败: %v", err)
		return false
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.String() == ip {
				return true
			}
		}
	}

	return false
}

// isInterfaceUp 检查网卡是否UP
func (m *Monitor) isInterfaceUp(ip string) bool {
	// 通过尝试绑定该IP的UDP连接来判断网卡是否可用
	addr := &net.UDPAddr{IP: net.ParseIP(ip)}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// canConnectToEdge 检查是否能连接到Edge节点
func (m *Monitor) canConnectToEdge() bool {
	// 尝试建立TCP连接
	d := &net.Dialer{
		LocalAddr: &net.TCPAddr{IP: net.ParseIP(m.mgmtIP)},
		Timeout:   3 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := d.DialContext(ctx, "tcp", m.edgeAddr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// getDefaultIP 获取默认IP地址
func getDefaultIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	// 优先返回非回环的IPv4地址
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	// 如果没有IPv4，返回IPv6
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			return ipnet.IP.String()
		}
	}

	return ""
}

// SetCheckInterval 设置检查间隔（用于测试）
func (m *Monitor) SetCheckInterval(interval time.Duration) {
	m.checkInterval = interval
}

// GetState 获取当前状态信息
func (m *Monitor) GetState() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"mgmt_ip":    m.mgmtIP,
		"available":  m.available,
		"edge_addr":  m.edgeAddr,
		"check_interval": m.checkInterval.String(),
	}
}

// WaitForAvailable 等待网卡可用（带超时）
func (m *Monitor) WaitForAvailable(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.IsAvailable() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// WaitForUnavailable 等待网卡不可用（带超时）
func (m *Monitor) WaitForUnavailable(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !m.IsAvailable() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// ValidateMgmtIP 验证管理IP配置是否有效
func ValidateMgmtIP(ip string) error {
	if ip == "" {
		return nil // 空IP表示自动检测，是有效的
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("无效的管理IP地址: %s", ip)
	}

	// 检查是否为回环地址
	if parsedIP.IsLoopback() {
		return fmt.Errorf("管理IP不能是回环地址: %s", ip)
	}

	return nil
}
