// Package election 预留 Edge Leader Election 能力
// 当前为接口定义和空实现，后续可基于 Redis 或 etcd 实现
package election

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	leaderKeyPrefix = "cloudflow:leader:"
	leaderLockKey   = "cloudflow:leader:lock"
)

// State Leader 状态
type State string

const (
	StateFollower State = "follower" // 跟随者
	StateCandidate State = "candidate" // 候选者
	StateLeader    State = "leader"    // 领导者
)

// LeaderElection Leader 选举接口
type LeaderElection interface {
	// Campaign 发起选举
	Campaign(ctx context.Context) error
	// Resign 辞去 Leader
	Resign(ctx context.Context) error
	// IsLeader 是否是 Leader
	IsLeader() bool
	// GetState 获取当前状态
	GetState() State
	// GetLeaderID 获取当前 Leader ID
	GetLeaderID() string
	// OnLeaderChange 注册 Leader 变更回调
	OnLeaderChange(callback func(leaderID string))
	// Close 关闭选举
	Close() error
}

// NoopElection 空实现（单节点模式或禁用选举时使用）
type NoopElection struct {
	mu        sync.RWMutex
	nodeID    string
	state     State
	callbacks []func(string)
	stopCh    chan struct{}
	stopped   bool
}

// NewNoopElection 创建空选举实现
func NewNoopElection(nodeID string) *NoopElection {
	return &NoopElection{
		nodeID: nodeID,
		state:  StateLeader, // 单节点模式下自己是 Leader
		stopCh: make(chan struct{}),
	}
}

func (n *NoopElection) Campaign(ctx context.Context) error {
	n.mu.Lock()
	n.state = StateLeader
	n.mu.Unlock()
	n.notifyCallbacks(n.nodeID)
	return nil
}

func (n *NoopElection) Resign(ctx context.Context) error {
	n.mu.Lock()
	n.state = StateFollower
	n.mu.Unlock()
	return nil
}

func (n *NoopElection) IsLeader() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state == StateLeader
}

func (n *NoopElection) GetState() State {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state
}

func (n *NoopElection) GetLeaderID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.state == StateLeader {
		return n.nodeID
	}
	return ""
}

func (n *NoopElection) OnLeaderChange(callback func(string)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.callbacks = append(n.callbacks, callback)
}

func (n *NoopElection) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.stopped {
		n.stopped = true
		close(n.stopCh)
	}
	return nil
}

func (n *NoopElection) notifyCallbacks(leaderID string) {
	for _, cb := range n.callbacks {
		go cb(leaderID)
	}
}

// RedisElection 基于 Redis SETNX + TTL 的 Leader 选举
type RedisElection struct {
	mu        sync.RWMutex
	client    *redis.Client
	nodeID    string
	electionKey string
	leaderID  string
	state     State
	ttl       time.Duration
	renewalInterval time.Duration
	callbacks []func(string)
	stopCh       chan struct{}
	stopped      bool
	cancelRenewal context.CancelFunc
}

// NewRedisElection 创建基于 Redis 的选举
func NewRedisElection(client *redis.Client, nodeID string, ttl time.Duration) *RedisElection {
	if ttl <= 0 {
		ttl = 10 * time.Second
	}
	return &RedisElection{
		client:    client,
		nodeID:    nodeID,
		electionKey: leaderKeyPrefix + "default",
		ttl:       ttl,
		renewalInterval: ttl / 3,
		stopCh:    make(chan struct{}),
		callbacks: make([]func(string), 0),
	}
}

// Campaign 发起选举
func (r *RedisElection) Campaign(ctx context.Context) error {
	r.mu.Lock()
	r.state = StateCandidate
	r.mu.Unlock()

	// 取消旧的续约协程
	if r.cancelRenewal != nil {
		r.cancelRenewal()
		r.cancelRenewal = nil
	}

	// 1. SET leader_key {nodeID} NX EX {ttl}
	ok, err := r.client.SetNX(ctx, r.electionKey, r.nodeID, r.ttl).Result()
	if err != nil {
		r.mu.Lock()
		r.state = StateFollower
		r.mu.Unlock()
		return fmt.Errorf("Redis SETNX 失败: %w", err)
	}

	if ok {
		// 竞选成功，成为 Leader
		r.mu.Lock()
		r.state = StateLeader
		r.leaderID = r.nodeID
		r.mu.Unlock()
		r.notifyCallbacks(r.nodeID)

		// 启动续约协程
		renewCtx, cancel := context.WithCancel(ctx)
		r.cancelRenewal = cancel
		go r.renewLeadership(renewCtx)
		return nil
	}

	// 竞选失败，获取当前 Leader
	currentLeader, err := r.client.Get(ctx, r.electionKey).Result()
	if err == nil && currentLeader != "" {
		r.mu.Lock()
		r.leaderID = currentLeader
		r.state = StateFollower
		r.mu.Unlock()
		r.notifyCallbacks(currentLeader)
	}

	// 启动监听协程，等待 Leader 变更
	go r.watchLeaderChange(ctx)
	return nil
}

// renewLeadership 续约 Leader 身份
func (r *RedisElection) renewLeadership(ctx context.Context) {
	ticker := time.NewTicker(r.renewalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ok, err := r.client.SetNX(ctx, r.electionKey, r.nodeID, r.ttl).Result()
			if err != nil || !ok {
				// 续约失败，不再是 Leader
				r.mu.Lock()
				r.state = StateFollower
				r.leaderID = ""
				r.mu.Unlock()
				r.notifyCallbacks("")
				return
			}
		case <-r.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// watchLeaderChange 监听 Leader 变更（Follower 模式）
func (r *RedisElection) watchLeaderChange(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			currentLeader, err := r.client.Get(ctx, r.electionKey).Result()
			if err != nil {
				continue // Redis 暂时不可用
			}

			r.mu.Lock()
			oldLeader := r.leaderID
			r.leaderID = currentLeader
			r.mu.Unlock()

			if currentLeader != oldLeader {
				r.notifyCallbacks(currentLeader)
			}

			// 如果 Leader 已过期，尝试竞选
			if currentLeader == "" {
				r.mu.Lock()
				r.state = StateCandidate
				r.mu.Unlock()

				// 取消旧的续约协程
				if r.cancelRenewal != nil {
					r.cancelRenewal()
					r.cancelRenewal = nil
				}

				// 直接在循环内竞选，不再递归
				ok, err := r.client.SetNX(ctx, r.electionKey, r.nodeID, r.ttl).Result()
				if err != nil {
					r.mu.Lock()
					r.state = StateFollower
					r.mu.Unlock()
					continue
				}

				if ok {
					r.mu.Lock()
					r.state = StateLeader
					r.leaderID = r.nodeID
					r.mu.Unlock()
					r.notifyCallbacks(r.nodeID)

					renewCtx, cancel := context.WithCancel(ctx)
					r.cancelRenewal = cancel
					go r.renewLeadership(renewCtx)
				} else {
					currentLeader, err := r.client.Get(ctx, r.electionKey).Result()
					if err == nil && currentLeader != "" {
						r.mu.Lock()
						r.leaderID = currentLeader
						r.state = StateFollower
						r.mu.Unlock()
						r.notifyCallbacks(currentLeader)
					}
				}
			}
		case <-r.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Resign 辞去 Leader
func (r *RedisElection) Resign(ctx context.Context) error {
	r.mu.Lock()
	if r.state != StateLeader {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	// 只有当前 Leader 才能删除 key
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)
	_, err := script.Run(ctx, r.client, []string{r.electionKey}, r.nodeID).Result()
	if err != nil {
		return fmt.Errorf("辞去 Leader 失败: %w", err)
	}

	r.mu.Lock()
	r.state = StateFollower
	r.leaderID = ""
	r.mu.Unlock()
	r.notifyCallbacks("")
	return nil
}

func (r *RedisElection) IsLeader() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state == StateLeader
}

func (r *RedisElection) GetState() State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

func (r *RedisElection) GetLeaderID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.leaderID
}

func (r *RedisElection) OnLeaderChange(callback func(string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, callback)
}

func (r *RedisElection) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.stopped {
		r.stopped = true
		if r.cancelRenewal != nil {
			r.cancelRenewal()
		}
		close(r.stopCh)
	}
	return nil
}

func (r *RedisElection) notifyCallbacks(leaderID string) {
	for _, cb := range r.callbacks {
		go cb(leaderID)
	}
}
