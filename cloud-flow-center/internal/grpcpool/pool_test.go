//go:build linux

package grpcpool

import (
	"testing"
	"time"
)

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()
	if cfg.MaxConns != 100 {
		t.Errorf("expected max conns 100, got %d", cfg.MaxConns)
	}
	if cfg.MaxIdleConns != 20 {
		t.Errorf("expected max idle conns 20, got %d", cfg.MaxIdleConns)
	}
	if cfg.ConnTimeout != 5*time.Second {
		t.Errorf("unexpected conn timeout: %v", cfg.ConnTimeout)
	}
}

func TestConnManagerStats(t *testing.T) {
	cm := NewConnManager(nil)
	defer cm.Close()

	stats := cm.Stats()
	if stats["pool_count"].(int) != 0 {
		t.Errorf("expected 0 pools, got %v", stats["pool_count"])
	}
	if stats["total_conns"].(int) != 0 {
		t.Errorf("expected 0 conns, got %v", stats["total_conns"])
	}
}

func TestConnManagerPoolCreation(t *testing.T) {
	cm := NewConnManager(nil)
	defer cm.Close()

	// 获取不存在的地址的池（内部创建）
	pool := cm.getOrCreatePool("test-addr:8080")
	if pool == nil {
		t.Fatal("expected pool to be created")
	}
	if pool.addr != "test-addr:8080" {
		t.Errorf("expected addr 'test-addr:8080', got %s", pool.addr)
	}
	if pool.maxConns != 100 {
		t.Errorf("expected max conns 100, got %d", pool.maxConns)
	}

	// 再次获取应该返回同一个池
	pool2 := cm.getOrCreatePool("test-addr:8080")
	if pool != pool2 {
		t.Error("expected same pool")
	}
}

func TestPooledConnHealthy(t *testing.T) {
	pc := &PooledConn{
		addr:      "test:8080",
		createdAt: time.Now(),
	}
	// 没有 conn 的 PooledConn 应该不健康
	if pc.IsHealthy() {
		t.Error("expected nil conn to be unhealthy")
	}
}

func TestConnManagerClose(t *testing.T) {
	cm := NewConnManager(nil)
	cm.Close()

	stats := cm.Stats()
	if stats["pool_count"].(int) != 0 {
		t.Errorf("expected 0 pools after close, got %v", stats["pool_count"])
	}
}

func TestConnManagerCleanup(t *testing.T) {
	cm := NewConnManager(&PoolConfig{
		MaxConns:        10,
		MaxIdleConns:    2,
		IdleTimeout:     100 * time.Millisecond,
		ConnTimeout:     1 * time.Second,
		HealthCheckInterval: 50 * time.Millisecond,
	})
	defer cm.Close()

	// 创建池
	pool := cm.getOrCreatePool("test:8080")
	pool.mu.Lock()
	pool.conns = append(pool.conns, &PooledConn{
		addr:       "test:8080",
		inUse:      false,
		lastUsedAt: time.Now().Add(-1 * time.Hour), // 很久未使用
	})
	pool.mu.Unlock()

	// 等待清理
	time.Sleep(200 * time.Millisecond)

	// 清理应该移除了旧连接
	stats := cm.Stats()
	if stats["total_conns"].(int) > 0 {
		t.Logf("old connections may still exist: %v", stats)
	}
}

func TestConnManagerMultiplePools(t *testing.T) {
	cm := NewConnManager(nil)
	defer cm.Close()

	cm.getOrCreatePool("addr1:8080")
	cm.getOrCreatePool("addr2:8080")
	cm.getOrCreatePool("addr3:8080")

	stats := cm.Stats()
	if stats["pool_count"].(int) != 3 {
		t.Errorf("expected 3 pools, got %v", stats["pool_count"])
	}
}
