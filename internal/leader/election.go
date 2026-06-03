// Package leader 提供基于 Redis 的分布式 Leader 选举
package leader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// Config Leader 选举配置
type Config struct {
	// Redis 客户端
	Redis *redis.Client

	// Key 前缀
	KeyPrefix string

	// 租约时长（Leader 持有锁的时间）
	LeaseDuration time.Duration

	// 重试间隔
	RetryInterval time.Duration

	// 节点 ID（唯一标识）
	NodeID string
}

// Election Leader 选举器
type Election struct {
	config     Config
	isLeader   bool
	mu         sync.RWMutex
	stopCh     chan struct{}
	leaderCh   chan bool // 通知领导权变化
	callbacks  []func(bool) // 领导权变化回调
}

// NewElection 创建 Leader 选举器
func NewElection(config Config) *Election {
	if config.KeyPrefix == "" {
		config.KeyPrefix = "leader:election:"
	}
	if config.LeaseDuration == 0 {
		config.LeaseDuration = 10 * time.Second // 默认 10 秒租约
	}
	if config.RetryInterval == 0 {
		config.RetryInterval = 2 * time.Second // 默认 2 秒重试
	}
	if config.NodeID == "" {
		config.NodeID = fmt.Sprintf("node-%d", time.Now().UnixNano())
	}

	return &Election{
		config:   config,
		isLeader: false,
		stopCh:   make(chan struct{}),
		leaderCh: make(chan bool, 1),
	}
}

// Start 启动选举循环
func (e *Election) Start(ctx context.Context) {
	go e.electionLoop(ctx)
}

// Stop 停止选举
func (e *Election) Stop() {
	close(e.stopCh)
}

// IsLeader 检查当前节点是否是 Leader
func (e *Election) IsLeader() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isLeader
}

// LeaderChan 返回领导权变化通知通道
func (e *Election) LeaderChan() <-chan bool {
	return e.leaderCh
}

// OnChange 注册领导权变化回调
func (e *Election) OnChange(callback func(isLeader bool)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.callbacks = append(e.callbacks, callback)
}

// electionLoop 选举主循环
func (e *Election) electionLoop(ctx context.Context) {
	ticker := time.NewTicker(e.config.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.tryAcquireLeadership(ctx)
		}
	}
}

// tryAcquireLeadership 尝试获取领导权
func (e *Election) tryAcquireLeadership(ctx context.Context) {
	key := e.config.KeyPrefix + "lock"
	now := time.Now().UnixNano()
	expiration := now + int64(e.config.LeaseDuration)

	// 使用 SET NX EX 原子操作尝试获取锁
	acquired, err := e.config.Redis.SetNX(ctx, key, fmt.Sprintf("%s:%d", e.config.NodeID, expiration), e.config.LeaseDuration).Result()
	if err != nil {
		// Redis 错误，记录但不改变状态
		return
	}

	if acquired {
		// 成功获取领导权
		e.becomeLeader()
	} else {
		// 检查当前 Leader 是否过期
		currentValue, err := e.config.Redis.Get(ctx, key).Result()
		if err != nil {
			return
		}

		// 解析过期时间
		var currentExpiration int64
		fmt.Sscanf(currentValue, "%*[^:]:%d", &currentExpiration)

		if now > currentExpiration {
			// Leader 已过期，尝试抢占
			acquired, err := e.config.Redis.SetNX(ctx, key, fmt.Sprintf("%s:%d", e.config.NodeID, expiration), e.config.LeaseDuration).Result()
			if err == nil && acquired {
				e.becomeLeader()
			}
		} else {
			// 不是 Leader
			e.becomeFollower()
		}
	}
}

// becomeLeader 成为 Leader
func (e *Election) becomeLeader() {
	e.mu.Lock()
	wasLeader := e.isLeader
	e.isLeader = true
	callbacks := make([]func(bool), len(e.callbacks))
	copy(callbacks, e.callbacks)
	e.mu.Unlock()

	if !wasLeader {
		// 领导权变化，发送通知
		select {
		case e.leaderCh <- true:
		default:
		}

		// 执行回调
		for _, cb := range callbacks {
			cb(true)
		}
	}

	// 定期续期租约
	go e.renewLease()
}

// becomeFollower 成为 Follower
func (e *Election) becomeFollower() {
	e.mu.Lock()
	wasLeader := e.isLeader
	e.isLeader = false
	callbacks := make([]func(bool), len(e.callbacks))
	copy(callbacks, e.callbacks)
	e.mu.Unlock()

	if wasLeader {
		// 领导权变化，发送通知
		select {
		case e.leaderCh <- false:
		default:
		}

		// 执行回调
		for _, cb := range callbacks {
			cb(false)
		}
	}
}

// renewLease 续期租约（仅 Leader 调用）
func (e *Election) renewLease() {
	key := e.config.KeyPrefix + "lock"
	ticker := time.NewTicker(e.config.LeaseDuration / 3) // 每 1/3 租约时间续期
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.mu.RLock()
			isLeader := e.isLeader
			e.mu.RUnlock()

			if !isLeader {
				return
			}

			// 续期
			now := time.Now().UnixNano()
			expiration := now + int64(e.config.LeaseDuration)
			err := e.config.Redis.Set(context.Background(), key, fmt.Sprintf("%s:%d", e.config.NodeID, expiration), e.config.LeaseDuration).Err()
			if err != nil {
				// 续期失败，可能失去领导权
				return
			}
		}
	}
}

// GetLeaderInfo 获取当前 Leader 信息
func (e *Election) GetLeaderInfo(ctx context.Context) (nodeID string, expiration int64, err error) {
	key := e.config.KeyPrefix + "lock"
	value, err := e.config.Redis.Get(ctx, key).Result()
	if err != nil {
		return "", 0, err
	}

	var exp int64
	fmt.Sscanf(value, "%[^:]:%d", &nodeID, &exp)
	return nodeID, exp, nil
}
