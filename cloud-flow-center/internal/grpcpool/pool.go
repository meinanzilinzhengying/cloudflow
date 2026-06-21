//go:build linux

package grpcpool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"
)

// ============================================================================
// gRPC 连接池
// 解决 P6: gRPC 长连接数量大时资源消耗高
// 策略：连接复用、健康检查、自动回收、负载均衡
// ============================================================================

// PoolConfig 连接池配置
type PoolConfig struct {
	MaxConns        int           // 最大连接数
	MaxIdleConns    int           // 最大空闲连接数
	ConnTimeout     time.Duration // 连接超时
	IdleTimeout     time.Duration // 空闲超时
	HealthCheckInterval time.Duration // 健康检查间隔
	KeepAliveTime   time.Duration // keepalive 时间
	KeepAliveTimeout time.Duration // keepalive 超时
}

// DefaultPoolConfig 返回默认配置
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxConns:            100,
		MaxIdleConns:        20,
		ConnTimeout:         5 * time.Second,
		IdleTimeout:         30 * time.Minute,
		HealthCheckInterval: 10 * time.Second,
		KeepAliveTime:       30 * time.Second,
		KeepAliveTimeout:    10 * time.Second,
	}
}

// PooledConn 连接池中的连接
type PooledConn struct {
	conn       *grpc.ClientConn
	addr       string
	createdAt  time.Time
	lastUsedAt time.Time
	inUse      bool
	mu         sync.Mutex
}

// IsHealthy 检查连接是否健康
func (pc *PooledConn) IsHealthy() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.conn == nil {
		return false
	}
	state := pc.conn.GetState()
	return state == connectivity.Ready || state == connectivity.Idle
}

// Close 关闭连接
func (pc *PooledConn) Close() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.conn != nil {
		return pc.conn.Close()
	}
	return nil
}

// ConnManager 连接管理器
type ConnManager struct {
	config     *PoolConfig
	mu         sync.RWMutex
	pools      map[string]*connPool // addr -> pool
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// connPool 单个地址的连接池
type connPool struct {
	addr       string
	conns      []*PooledConn
	maxConns   int
	maxIdle    int
	mu         sync.Mutex
}

// NewConnManager 创建连接管理器
func NewConnManager(config *PoolConfig) *ConnManager {
	if config == nil {
		config = DefaultPoolConfig()
	}
	cm := &ConnManager{
		config: config,
		pools:  make(map[string]*connPool),
		stopCh: make(chan struct{}),
	}
	cm.startHealthCheck()
	cm.startCleanup()
	return cm
}

// GetConn 获取连接
func (cm *ConnManager) GetConn(addr string) (*grpc.ClientConn, error) {
	pool := cm.getOrCreatePool(addr)

	pool.mu.Lock()
	defer pool.mu.Unlock()

	// 查找空闲健康连接
	for _, pc := range pool.conns {
		if !pc.inUse && pc.IsHealthy() {
			pc.inUse = true
			pc.lastUsedAt = time.Now()
			return pc.conn, nil
		}
	}

	// 创建新连接
	if len(pool.conns) < pool.maxConns {
		conn, err := cm.createConn(addr)
		if err != nil {
			return nil, err
		}
		pc := &PooledConn{
			conn:       conn,
			addr:       addr,
			createdAt:  time.Now(),
			lastUsedAt: time.Now(),
			inUse:      true,
		}
		pool.conns = append(pool.conns, pc)
		return conn, nil
	}

	// 等待其他连接释放
	return nil, fmt.Errorf("connection pool exhausted for %s", addr)
}

// ReleaseConn 释放连接
func (cm *ConnManager) ReleaseConn(addr string, conn *grpc.ClientConn) {
	pool := cm.getPool(addr)
	if pool == nil {
		return
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	for _, pc := range pool.conns {
		if pc.conn == conn {
			pc.inUse = false
			pc.lastUsedAt = time.Now()
			return
		}
	}
}

// getOrCreatePool 获取或创建连接池
func (cm *ConnManager) getOrCreatePool(addr string) *connPool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	pool, ok := cm.pools[addr]
	if !ok {
		pool = &connPool{
			addr:     addr,
			maxConns: cm.config.MaxConns,
			maxIdle:  cm.config.MaxIdleConns,
			conns:    make([]*PooledConn, 0),
		}
		cm.pools[addr] = pool
	}
	return pool
}

// getPool 获取连接池
func (cm *ConnManager) getPool(addr string) *connPool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.pools[addr]
}

// createConn 创建新连接
func (cm *ConnManager) createConn(addr string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cm.config.ConnTimeout)
	defer cancel()

	keepaliveParams := keepalive.ClientParameters{
		Time:                cm.config.KeepAliveTime,
		Timeout:             cm.config.KeepAliveTimeout,
		PermitWithoutStream: true,
	}

	return grpc.DialContext(ctx, addr,
		grpc.WithKeepaliveParams(keepaliveParams),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
}

// startHealthCheck 启动健康检查
func (cm *ConnManager) startHealthCheck() {
	cm.wg.Add(1)
	go func() {
		defer cm.wg.Done()
		ticker := time.NewTicker(cm.config.HealthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-cm.stopCh:
				return
			case <-ticker.C:
				cm.checkHealth()
			}
		}
	}()
}

// checkHealth 健康检查
func (cm *ConnManager) checkHealth() {
	cm.mu.RLock()
	pools := make([]*connPool, 0, len(cm.pools))
	for _, pool := range cm.pools {
		pools = append(pools, pool)
	}
	cm.mu.RUnlock()

	for _, pool := range pools {
		pool.mu.Lock()
		var healthy []*PooledConn
		for _, pc := range pool.conns {
			if pc.IsHealthy() || pc.inUse {
				healthy = append(healthy, pc)
			} else {
				pc.Close()
			}
		}
		pool.conns = healthy
		pool.mu.Unlock()
	}
}

// startCleanup 启动清理循环
func (cm *ConnManager) startCleanup() {
	cm.wg.Add(1)
	go func() {
		defer cm.wg.Done()
		ticker := time.NewTicker(cm.config.IdleTimeout / 2)
		defer ticker.Stop()

		for {
			select {
			case <-cm.stopCh:
				return
			case <-ticker.C:
				cm.cleanupIdle()
			}
		}
	}()
}

// cleanupIdle 清理空闲连接
func (cm *ConnManager) cleanupIdle() {
	cm.mu.RLock()
	pools := make([]*connPool, 0, len(cm.pools))
	for _, pool := range cm.pools {
		pools = append(pools, pool)
	}
	cm.mu.RUnlock()

	for _, pool := range pools {
		pool.mu.Lock()
		var active []*PooledConn
		idleCount := 0
		for _, pc := range pool.conns {
			if pc.inUse {
				active = append(active, pc)
			} else if time.Since(pc.lastUsedAt) > cm.config.IdleTimeout {
				if idleCount < pool.maxIdle {
					active = append(active, pc)
					idleCount++
				} else {
					pc.Close()
				}
			} else {
				active = append(active, pc)
			}
		}
		pool.conns = active
		pool.mu.Unlock()
	}
}

// Close 关闭所有连接
func (cm *ConnManager) Close() {
	close(cm.stopCh)
	cm.wg.Wait()

	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, pool := range cm.pools {
		pool.mu.Lock()
		for _, pc := range pool.conns {
			pc.Close()
		}
		pool.conns = nil
		pool.mu.Unlock()
	}
	cm.pools = make(map[string]*connPool)
}

// Stats 获取统计
func (cm *ConnManager) Stats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	totalConns := 0
	totalIdle := 0
	addrStats := make(map[string]map[string]int)

	for addr, pool := range cm.pools {
		pool.mu.Lock()
		idle := 0
		for _, pc := range pool.conns {
			if !pc.inUse {
				idle++
			}
		}
		addrStats[addr] = map[string]int{
			"total": len(pool.conns),
			"idle":  idle,
			"max":   pool.maxConns,
		}
		totalConns += len(pool.conns)
		totalIdle += idle
		pool.mu.Unlock()
	}

	return map[string]interface{}{
		"total_conns":  totalConns,
		"total_idle":   totalIdle,
		"pool_count":   len(cm.pools),
		"addr_stats":   addrStats,
	}
}
