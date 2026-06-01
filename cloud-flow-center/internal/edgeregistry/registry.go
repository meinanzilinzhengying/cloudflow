// Package edgeregistry 维护 Center 侧的 Edge 节点注册表
// Edge 通过 Heartbeat 注册/续约，Center 维护活跃 Edge 列表供 Agent 发现
package edgeregistry

import (
	"sync"
	"time"
)

// EdgeNode Edge 节点信息
type EdgeNode struct {
	ID            string            `json:"id"`
	Address       string            `json:"address"`
	Region        string            `json:"region"`
	CloudPlatform string            `json:"cloud_platform"`
	Version       string            `json:"version"`
	ProbeCount    int32             `json:"probe_count"`
	Healthy       bool              `json:"healthy"`
	Weight        int32             `json:"weight"`
	Tags          map[string]string `json:"tags"`
	RegisteredAt  time.Time         `json:"registered_at"`
	LastHeartbeat time.Time         `json:"last_heartbeat"`
}

// Registry Edge 节点注册表
type Registry struct {
	mu      sync.RWMutex
	edges   map[string]*EdgeNode
	stopCh  chan struct{}
	stopped bool
}

// NewRegistry 创建 Edge 注册表
func NewRegistry() *Registry {
	return &Registry{
		edges:  make(map[string]*EdgeNode),
		stopCh: make(chan struct{}),
	}
}

// RegisterOrUpdate 注册或更新 Edge 节点（心跳续约）
func (r *Registry) RegisterOrUpdate(node *EdgeNode) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.edges[node.ID]; ok {
		existing.Address = node.Address
		existing.Region = node.Region
		existing.CloudPlatform = node.CloudPlatform
		existing.ProbeCount = node.ProbeCount
		existing.Healthy = true
		existing.Weight = node.Weight
		existing.LastHeartbeat = time.Now()
		if node.Tags != nil {
			existing.Tags = node.Tags
		}
		if node.Version != "" {
			existing.Version = node.Version
		}
		return
	}

	node.Healthy = true
	node.RegisteredAt = time.Now()
	node.LastHeartbeat = time.Now()
	if node.Tags == nil {
		node.Tags = make(map[string]string)
	}
	if node.Weight <= 0 {
		node.Weight = 100
	}
	r.edges[node.ID] = node
}

// Deregister 注销 Edge 节点
func (r *Registry) Deregister(edgeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.edges, edgeID)
}

// Get 获取单个 Edge 节点
func (r *Registry) Get(edgeID string) (*EdgeNode, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	node, ok := r.edges[edgeID]
	if !ok {
		return nil, false
	}
	cp := *node
	return &cp, true
}

// List 列出所有 Edge 节点
func (r *Registry) List() []*EdgeNode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*EdgeNode, 0, len(r.edges))
	for _, node := range r.edges {
		cp := *node
		result = append(result, &cp)
	}
	return result
}

// ListHealthy 列出健康的 Edge 节点
func (r *Registry) ListHealthy() []*EdgeNode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*EdgeNode, 0)
	for _, node := range r.edges {
		if node.Healthy {
			cp := *node
			result = append(result, &cp)
		}
	}
	return result
}

// ListByRegion 按区域列出健康的 Edge 节点
func (r *Registry) ListByRegion(region string) []*EdgeNode {
	if region == "" {
		return r.ListHealthy()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*EdgeNode, 0)
	for _, node := range r.edges {
		if node.Healthy && (node.Region == region || node.Region == "") {
			cp := *node
			result = append(result, &cp)
		}
	}
	return result
}

// Count 获取 Edge 节点总数
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.edges)
}

// HealthyCount 获取健康 Edge 节点数
func (r *Registry) HealthyCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, node := range r.edges {
		if node.Healthy {
			count++
		}
	}
	return count
}

// StartCleanup 启动定期清理过期 Edge 节点
func (r *Registry) StartCleanup(interval, timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				removed := r.cleanupExpired(timeout)
				if removed > 0 {
					_ = removed
				}
			case <-r.stopCh:
				return
			}
		}
	}()
}

// cleanupExpired 清理心跳超时的 Edge 节点
func (r *Registry) cleanupExpired(timeout time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	removed := 0
	for id, node := range r.edges {
		if now.Sub(node.LastHeartbeat) > timeout {
			node.Healthy = false
			if now.Sub(node.LastHeartbeat) > timeout*2 {
				delete(r.edges, id)
			}
			removed++
		}
	}
	return removed
}

// Close 关闭注册表
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.stopped {
		r.stopped = true
		close(r.stopCh)
	}
}
