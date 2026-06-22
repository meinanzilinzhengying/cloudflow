// Package lock 提供基于 etcd 的分布式锁
// P0-8 修复：解决分布式环境下的并发修改冲突
package lock

import (
	"context"
	"fmt"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// LockManager 分布式锁管理器接口
type LockManager interface {
	// Lock 获取锁，阻塞直到成功或超时
	Lock(ctx context.Context, lockName string) (*Lock, error)
	// TryLock 尝试获取锁，非阻塞
	TryLock(ctx context.Context, lockName string) (*Lock, error)
	// Unlock 释放锁
	Unlock(lock *Lock) error
	// Close 关闭锁管理器
	Close() error
}

// Lock 分布式锁实例
type Lock struct {
	Name     string
	Mutex    *concurrency.Mutex
	Session  *concurrency.Session
}

// EtcdLockManager 基于 etcd 的分布式锁管理器
type EtcdLockManager struct {
	client  *clientv3.Client
	prefix  string
	sessions sync.Map // map[string]*concurrency.Session
}

// NewEtcdLockManager 创建 etcd 分布式锁管理器
// endpoints: etcd 节点地址列表，如 ["localhost:2379"]
// prefix: 锁key前缀，如 "/cloudflow/lock/"
func NewEtcdLockManager(endpoints []string, prefix string) (*EtcdLockManager, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("etcd endpoints cannot be empty")
	}
	if prefix == "" {
		prefix = "/cloudflow/lock/"
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd connect failed: %w", err)
	}

	return &EtcdLockManager{
		client: client,
		prefix: prefix,
	}, nil
}

// NewEtcdLockManagerWithClient 使用已有 etcd client 创建锁管理器
func NewEtcdLockManagerWithClient(client *clientv3.Client, prefix string) *EtcdLockManager {
	if prefix == "" {
		prefix = "/cloudflow/lock/"
	}
	return &EtcdLockManager{
		client: client,
		prefix: prefix,
	}
}

// Lock 获取锁（阻塞，带超时）
func (m *EtcdLockManager) Lock(ctx context.Context, lockName string) (*Lock, error) {
	if m.client == nil {
		return nil, fmt.Errorf("etcd client not initialized")
	}

	key := m.prefix + lockName

	// 创建会话（如果已有则复用）
	var session *concurrency.Session
	if s, ok := m.sessions.Load(key); ok {
		session = s.(*concurrency.Session)
	} else {
		var err error
		session, err = concurrency.NewSession(m.client, concurrency.WithTTL(10))
		if err != nil {
			return nil, fmt.Errorf("create session failed: %w", err)
		}
		m.sessions.Store(key, session)
	}

	mutex := concurrency.NewMutex(session, key)
	if err := mutex.Lock(ctx); err != nil {
		return nil, fmt.Errorf("lock failed: %w", err)
	}

	return &Lock{
		Name:    lockName,
		Mutex:   mutex,
		Session: session,
	}, nil
}

// TryLock 尝试获取锁（非阻塞）
func (m *EtcdLockManager) TryLock(ctx context.Context, lockName string) (*Lock, error) {
	if m.client == nil {
		return nil, fmt.Errorf("etcd client not initialized")
	}

	key := m.prefix + lockName

	var session *concurrency.Session
	if s, ok := m.sessions.Load(key); ok {
		session = s.(*concurrency.Session)
	} else {
		var err error
		session, err = concurrency.NewSession(m.client, concurrency.WithTTL(10))
		if err != nil {
			return nil, fmt.Errorf("create session failed: %w", err)
		}
		m.sessions.Store(key, session)
	}

	mutex := concurrency.NewMutex(session, key)
	if err := mutex.TryLock(ctx); err != nil {
		return nil, fmt.Errorf("try lock failed: %w", err)
	}

	return &Lock{
		Name:    lockName,
		Mutex:   mutex,
		Session: session,
	}, nil
}

// Unlock 释放锁
func (m *EtcdLockManager) Unlock(lock *Lock) error {
	if lock == nil || lock.Mutex == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := lock.Mutex.Unlock(ctx); err != nil {
		return fmt.Errorf("unlock failed: %w", err)
	}
	return nil
}

// Close 关闭锁管理器
func (m *EtcdLockManager) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// ============================================================================
// 内存锁（单机/测试用，不依赖 etcd）
// ============================================================================

// MemoryLockManager 基于内存的锁管理器（单机/测试用）
type MemoryLockManager struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewMemoryLockManager 创建内存锁管理器
func NewMemoryLockManager() *MemoryLockManager {
	return &MemoryLockManager{
		locks: make(map[string]*sync.Mutex),
	}
}

// Lock 获取锁
func (m *MemoryLockManager) Lock(ctx context.Context, lockName string) (*Lock, error) {
	m.mu.Lock()
	mutex, ok := m.locks[lockName]
	if !ok {
		mutex = &sync.Mutex{}
		m.locks[lockName] = mutex
	}
	m.mu.Unlock()
	mutex.Lock()
	return &Lock{Name: lockName}, nil
}

// TryLock 尝试获取锁
func (m *MemoryLockManager) TryLock(ctx context.Context, lockName string) (*Lock, error) {
	m.mu.Lock()
	mutex, ok := m.locks[lockName]
	if !ok {
		mutex = &sync.Mutex{}
		m.locks[lockName] = mutex
	}
	m.mu.Unlock()
	if !mutex.TryLock() {
		return nil, fmt.Errorf("lock is already held")
	}
	return &Lock{Name: lockName}, nil
}

// Unlock 释放锁
func (m *MemoryLockManager) Unlock(lock *Lock) error {
	m.mu.Lock()
	mutex, ok := m.locks[lock.Name]
	m.mu.Unlock()
	if ok {
		mutex.Unlock()
	}
	return nil
}

// Close 关闭
func (m *MemoryLockManager) Close() error {
	return nil
}
