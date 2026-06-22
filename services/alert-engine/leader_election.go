// leader_election.go 内嵌到 alert-engine 的基于数据库的 Leader 选举
// P0-7 修复：防止多实例重复告警
package alertengine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/pkg/storage"
)

// LeaderElection 基于数据库的 Leader 选举器
type LeaderElection struct {
	db       storage.RelationalStorage
	key      string
	nodeID   string
	ttl      time.Duration
	interval time.Duration

	isLeader bool
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewLeaderElection 创建 Leader 选举器
func NewLeaderElection(db storage.RelationalStorage, key, nodeID string) *LeaderElection {
	if key == "" {
		key = "alert-engine:leader"
	}
	if nodeID == "" {
		nodeID = fmt.Sprintf("node-%d", time.Now().UnixNano())
	}
	return &LeaderElection{
		db:       db,
		key:      key,
		nodeID:   nodeID,
		ttl:      30 * time.Second,
		interval: 10 * time.Second,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动选举循环
func (l *LeaderElection) Start(ctx context.Context) {
	// 确保表存在
	l.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS leader_election (
			service_name VARCHAR(100) PRIMARY KEY,
			node_id VARCHAR(100) NOT NULL,
			acquired_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`)
	go l.electionLoop(ctx)
}

// Stop 停止选举
func (l *LeaderElection) Stop() {
	close(l.stopCh)
}

// IsLeader 检查当前是否是 Leader
func (l *LeaderElection) IsLeader() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isLeader
}

// electionLoop 选举主循环
func (l *LeaderElection) electionLoop(ctx context.Context) {
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.tryAcquire(ctx)
		}
	}
}

// tryAcquire 尝试获取/续期领导权
func (l *LeaderElection) tryAcquire(ctx context.Context) {
	now := time.Now()
	expires := now.Add(l.ttl)

	// 1. 查询当前 leader
	var currentNodeID string
	var expiresAt time.Time
	err := l.db.QueryRow(ctx, "SELECT node_id, expires_at FROM leader_election WHERE service_name = ?", l.key).Scan(&currentNodeID, &expiresAt)

	if err != nil && err.Error() != "sql: no rows in result set" {
		l.mu.Lock()
		l.isLeader = false
		l.mu.Unlock()
		return
	}

	if err != nil || expiresAt.Before(now) {
		// 没有 leader 或已过期，尝试获取
		_, dbErr := l.db.Exec(ctx, `
			INSERT INTO leader_election (service_name, node_id, acquired_at, expires_at)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
			node_id = VALUES(node_id),
			acquired_at = VALUES(acquired_at),
			expires_at = VALUES(expires_at)
		`, l.key, l.nodeID, now, expires)
		if dbErr != nil {
			l.mu.Lock()
			l.isLeader = false
			l.mu.Unlock()
			return
		}
		// 确认
		var newNodeID string
		l.db.QueryRow(ctx, "SELECT node_id FROM leader_election WHERE service_name = ?", l.key).Scan(&newNodeID)
		l.mu.Lock()
		l.isLeader = (newNodeID == l.nodeID)
		l.mu.Unlock()
		return
	}

	if currentNodeID == l.nodeID {
		// 当前就是 leader，续期
		l.db.Exec(ctx, "UPDATE leader_election SET expires_at = ? WHERE service_name = ? AND node_id = ?", expires, l.key, l.nodeID)
		l.mu.Lock()
		l.isLeader = true
		l.mu.Unlock()
		return
	}

	// 别人是 leader
	l.mu.Lock()
	l.isLeader = false
	l.mu.Unlock()
}
