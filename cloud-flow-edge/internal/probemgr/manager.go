// Package probemgr 提供探针管理功能
// 负责探针的注册、心跳、状态维护和超时清理
package probemgr

import (
	"sync"
	"time"

	"cloud-flow-edge/pkg/logger"
)

// Probe 探针信息
type Probe struct {
	ID            string    // 探针唯一标识
	HostIP        string    // 所在主机 IP
	Hostname      string    // 所在主机名
	Version       string    // 探针版本
	Status        string    // 状态: online / offline
	RegisteredAt  time.Time // 注册时间
	LastHeartbeat time.Time // 最后心跳时间
}

// Manager 探针管理器
type Manager struct {
	mu     sync.RWMutex
	probes map[string]*Probe
	logger *logger.Logger
	stopCh chan struct{}
	stopped bool
	stopMu sync.Mutex
}

// NewManager 创建探针管理器
func NewManager(log *logger.Logger) *Manager {
	return &Manager{
		probes: make(map[string]*Probe),
		logger: log,
		stopCh: make(chan struct{}),
	}
}

// Register 注册新探针
// 如果探针已存在则更新信息但保留 RegisteredAt，状态重置为 online
func (m *Manager) Register(id, hostIP, hostname, version string) *Probe {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// 已存在的探针：更新字段但保留注册时间
	if existing, ok := m.probes[id]; ok {
		existing.HostIP = hostIP
		existing.Hostname = hostname
		existing.Version = version
		existing.Status = "online"
		existing.LastHeartbeat = now
		m.logger.Infof("探针重新注册: id=%s, hostIP=%s, hostname=%s, version=%s", id, hostIP, hostname, version)
		return existing
	}

	// 新探针
	p := &Probe{
		ID:            id,
		HostIP:        hostIP,
		Hostname:      hostname,
		Version:       version,
		Status:        "online",
		RegisteredAt:  now,
		LastHeartbeat: now,
	}
	m.probes[id] = p
	m.logger.Infof("探针注册成功: id=%s, hostIP=%s, hostname=%s, version=%s", id, hostIP, hostname, version)
	return p
}

// Heartbeat 更新探针心跳
// 返回 true 表示探针存在并已更新，false 表示探针不存在
func (m *Manager) Heartbeat(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.probes[id]
	if !ok {
		m.logger.Warnf("收到未知探针心跳: id=%s", id)
		return false
	}
	p.LastHeartbeat = time.Now()
	p.Status = "online"
	return true
}

// GetProbe 获取单个探针信息
func (m *Manager) GetProbe(id string) (*Probe, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.probes[id]
	return p, ok
}

// GetAllProbes 获取所有探针信息
func (m *Manager) GetAllProbes() []*Probe {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probes := make([]*Probe, 0, len(m.probes))
	for _, p := range m.probes {
		probes = append(probes, p)
	}
	return probes
}

// RemoveOfflineProbes 移除超时探针
// timeout 为超时阈值，超过该时间未收到心跳的探针将被标记 offline 后删除
// 返回被移除的探针数量
func (m *Manager) RemoveOfflineProbes(timeout time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removed := 0
	for id, p := range m.probes {
		if now.Sub(p.LastHeartbeat) > timeout {
			// 先标记为 offline 再删除，便于外部监听/日志追踪
			p.Status = "offline"
			m.logger.Infof("移除超时探针: id=%s, lastHeartbeat=%s", id, p.LastHeartbeat.Format(time.RFC3339))
			delete(m.probes, id)
			removed++
		}
	}
	return removed
}

// StartCleanup 启动后台清理协程，周期性清理超时探针
// interval 为清理间隔，timeout 为超时阈值
func (m *Manager) StartCleanup(interval, timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				removed := m.RemoveOfflineProbes(timeout)
				if removed > 0 {
					m.logger.Infof("定时清理完成，移除 %d 个超时探针", removed)
				}
			case <-m.stopCh:
				m.logger.Info("探针清理协程已停止")
				return
			}
		}
	}()
	m.logger.Infof("探针清理协程已启动，interval=%s, timeout=%s", interval, timeout)
}

// GetProbeCount 获取当前在线探针数量
func (m *Manager) GetProbeCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, p := range m.probes {
		if p.Status == "online" {
			count++
		}
	}
	return count
}

// Stop 停止清理协程
func (m *Manager) Stop() {
	m.stopMu.Lock()
	defer m.stopMu.Unlock()
	if !m.stopped {
		close(m.stopCh)
		m.stopped = true
		m.logger.Info("探针管理器已停止")
	}
}
