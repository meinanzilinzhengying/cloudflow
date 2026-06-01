// Package whitelist 提供 Agent 白名单管理功能
//
// 功能：
// 1. 内存白名单存储，支持多维度查询（ProbeID/CN/TokenHash）
// 2. 过期自动清理
// 3. 从 Center 远程同步白名单
package whitelist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"cloud-flow-edge/pkg/logger"
)

// ============================================================================
// 数据结构
// ============================================================================

// AgentIdentity Agent身份信息
type AgentIdentity struct {
	ProbeID   string            `json:"probe_id"`
	HostIP    string            `json:"host_ip"`
	Hostname  string            `json:"hostname"`
	CN        string            `json:"cn"`          // 证书Common Name
	TokenHash string            `json:"token_hash"`   // Token的SHA256哈希
	AddedAt   time.Time         `json:"added_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	Tags      map[string]string `json:"tags"`
}

// IsExpired 检查是否过期
func (a *AgentIdentity) IsExpired() bool {
	if a.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(a.ExpiresAt)
}

// SyncRequest 白名单同步请求
type SyncRequest struct {
	EdgeNodeID string `json:"edge_node_id"`
	Version    int64  `json:"version"`
}

// SyncResponse 白名单同步响应
type SyncResponse struct {
	Version   int64           `json:"version"`
	Agents    []AgentIdentity `json:"agents"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Stats 白名单统计
type Stats struct {
	TotalCount   int   `json:"total_count"`
	ExpiredCount int   `json:"expired_count"`
	ActiveCount  int   `json:"active_count"`
}

// ============================================================================
// Manager 白名单管理器
// ============================================================================

// Manager Agent白名单管理器
type Manager struct {
	mu sync.RWMutex

	// 主索引
	whitelist map[string]*AgentIdentity // ProbeID -> Identity

	// 二级索引
	cnIndex        map[string]string // CN -> ProbeID
	tokenHashIndex map[string]string // TokenHash -> ProbeID

	// 版本控制
	version int64

	// 配置
	logger        *logger.Logger
	cleanupInterval time.Duration

	// 控制
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewManager 创建白名单管理器
func NewManager(log *logger.Logger) *Manager {
	return &Manager{
		whitelist:       make(map[string]*AgentIdentity),
		cnIndex:         make(map[string]string),
		tokenHashIndex:  make(map[string]string),
		logger:          log,
		cleanupInterval: 5 * time.Minute,
		stopCh:          make(chan struct{}),
	}
}

// Start 启动白名单管理器
func (m *Manager) Start() {
	m.wg.Add(1)
	go m.cleanupLoop()
	m.logger.Info("[whitelist] 白名单管理器已启动")
}

// Stop 停止白名单管理器
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	m.logger.Info("[whitelist] 白名单管理器已停止")
}

// cleanupLoop 定期清理过期条目
func (m *Manager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			removed := m.RemoveExpired()
			if removed > 0 {
				m.logger.Infof("[whitelist] 清理 %d 个过期条目", removed)
			}
		case <-m.stopCh:
			return
		}
	}
}

// Add 添加Agent到白名单
func (m *Manager) Add(agent AgentIdentity) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent.AddedAt.IsZero() {
		agent.AddedAt = time.Now()
	}

	// 删除旧索引
	if old, exists := m.whitelist[agent.ProbeID]; exists {
		delete(m.cnIndex, old.CN)
		delete(m.tokenHashIndex, old.TokenHash)
	}

	// 添加新条目
	m.whitelist[agent.ProbeID] = &agent

	// 维护二级索引
	if agent.CN != "" {
		m.cnIndex[agent.CN] = agent.ProbeID
	}
	if agent.TokenHash != "" {
		m.tokenHashIndex[agent.TokenHash] = agent.ProbeID
	}
}

// Remove 从白名单移除Agent
func (m *Manager) Remove(probeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent, exists := m.whitelist[probeID]; exists {
		delete(m.cnIndex, agent.CN)
		delete(m.tokenHashIndex, agent.TokenHash)
		delete(m.whitelist, probeID)
	}
}

// GetByProbeID 按ProbeID查询
func (m *Manager) GetByProbeID(probeID string) *AgentIdentity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.whitelist[probeID]
}

// GetByCN 按证书CN查询
func (m *Manager) GetByCN(cn string) *AgentIdentity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probeID, exists := m.cnIndex[cn]
	if !exists {
		return nil
	}
	return m.whitelist[probeID]
}

// GetByTokenHash 按Token哈希查询
func (m *Manager) GetByTokenHash(tokenHash string) *AgentIdentity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probeID, exists := m.tokenHashIndex[tokenHash]
	if !exists {
		return nil
	}
	return m.whitelist[probeID]
}

// IsAllowed 检查ProbeID是否在白名单中（含过期检查）
func (m *Manager) IsAllowed(probeID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, exists := m.whitelist[probeID]
	if !exists {
		return false
	}
	return !agent.IsExpired()
}

// IsAllowedCN 检查CN是否在白名单中
func (m *Manager) IsAllowedCN(cn string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probeID, exists := m.cnIndex[cn]
	if !exists {
		return false
	}
	agent := m.whitelist[probeID]
	return agent != nil && !agent.IsExpired()
}

// IsAllowedToken 检查Token哈希是否在白名单中
func (m *Manager) IsAllowedToken(tokenHash string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probeID, exists := m.tokenHashIndex[tokenHash]
	if !exists {
		return false
	}
	agent := m.whitelist[probeID]
	return agent != nil && !agent.IsExpired()
}

// LoadFromCenter 从Center批量加载白名单（全量替换）
func (m *Manager) LoadFromCenter(agents []AgentIdentity, version int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清空旧数据
	m.whitelist = make(map[string]*AgentIdentity)
	m.cnIndex = make(map[string]string)
	m.tokenHashIndex = make(map[string]string)

	// 加载新数据
	for i := range agents {
		agent := agents[i]
		m.whitelist[agent.ProbeID] = &agent

		if agent.CN != "" {
			m.cnIndex[agent.CN] = agent.ProbeID
		}
		if agent.TokenHash != "" {
			m.tokenHashIndex[agent.TokenHash] = agent.ProbeID
		}
	}

	m.version = version
	m.logger.Infof("[whitelist] 从Center同步白名单: %d 条, version=%d", len(agents), version)
}

// RemoveExpired 移除过期条目
func (m *Manager) RemoveExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for probeID, agent := range m.whitelist {
		if agent.IsExpired() {
			delete(m.cnIndex, agent.CN)
			delete(m.tokenHashIndex, agent.TokenHash)
			delete(m.whitelist, probeID)
			removed++
		}
	}
	return removed
}

// GetAll 获取所有白名单条目
func (m *Manager) GetAll() []AgentIdentity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]AgentIdentity, 0, len(m.whitelist))
	for _, agent := range m.whitelist {
		result = append(result, *agent)
	}
	return result
}

// GetStats 获取统计信息
func (m *Manager) GetStats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := Stats{
		TotalCount: len(m.whitelist),
	}
	for _, agent := range m.whitelist {
		if agent.IsExpired() {
			stats.ExpiredCount++
		} else {
			stats.ActiveCount++
		}
	}
	return stats
}

// GetVersion 获取当前白名单版本
func (m *Manager) GetVersion() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

// Count 获取白名单总数
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.whitelist)
}

// ============================================================================
// Center 同步器
// ============================================================================

// CenterSyncer Center白名单同步器
type CenterSyncer struct {
	manager       *Manager
	centerAddr    string
	edgeNodeID    string
	syncInterval  time.Duration
	apiKey        string
	logger        *logger.Logger
	httpClient    *http.Client

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewCenterSyncer 创建Center同步器
func NewCenterSyncer(manager *Manager, centerAddr, edgeNodeID, apiKey string, syncInterval time.Duration, log *logger.Logger) *CenterSyncer {
	if syncInterval == 0 {
		syncInterval = 30 * time.Second
	}

	return &CenterSyncer{
		manager:      manager,
		centerAddr:   centerAddr,
		edgeNodeID:   edgeNodeID,
		syncInterval: syncInterval,
		apiKey:       apiKey,
		logger:       log,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		stopCh:       make(chan struct{}),
	}
}

// Start 启动同步器
func (s *CenterSyncer) Start() {
	s.wg.Add(1)
	go s.syncLoop()
	s.logger.Infof("[whitelist] Center同步器已启动: 同步间隔=%v", s.syncInterval)
}

// Stop 停止同步器
func (s *CenterSyncer) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("[whitelist] Center同步器已停止")
}

// syncLoop 定期同步
func (s *CenterSyncer) syncLoop() {
	defer s.wg.Done()

	// 首次立即同步
	s.syncOnce()

	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.syncOnce()
		case <-s.stopCh:
			return
		}
	}
}

// syncOnce 执行一次同步
func (s *CenterSyncer) syncOnce() {
	url := fmt.Sprintf("%s/api/v1/whitelist?edge_node_id=%s&version=%d",
		s.centerAddr, s.edgeNodeID, s.manager.GetVersion())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		s.logger.Warnf("[whitelist] 创建同步请求失败: %v", err)
		return
	}

	// 设置认证头
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Warnf("[whitelist] 同步白名单失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Warnf("[whitelist] 同步白名单失败: status=%d, body=%s", resp.StatusCode, string(body))
		return
	}

	var syncResp SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		s.logger.Warnf("[whitelist] 解析同步响应失败: %v", err)
		return
	}

	// 检查版本，如果没有更新则跳过
	if syncResp.Version <= s.manager.GetVersion() && s.manager.Count() > 0 {
		s.logger.Debugf("[whitelist] 白名单已是最新: version=%d", syncResp.Version)
		return
	}

	// 更新白名单
	s.manager.LoadFromCenter(syncResp.Agents, syncResp.Version)
}

// ForceSync 强制立即同步
func (s *CenterSyncer) ForceSync() {
	s.syncOnce()
}
