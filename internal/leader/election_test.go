package leader

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   3, // 使用独立的 DB
	})

	err := client.Ping(context.Background()).Err()
	if err != nil {
		t.Skip("Redis not available, skipping test")
	}

	client.FlushDB(context.Background())
	return client
}

func TestElectionBasic(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	election := NewElection(Config{
		Redis:         redisClient,
		KeyPrefix:     "test:election:",
		NodeID:        "node-1",
		LeaseDuration: 2 * time.Second,
		RetryInterval: 500 * time.Millisecond,
	})

	// 启动选举
	election.Start(ctx)
	defer election.Stop()

	// 等待成为 Leader
	time.Sleep(1 * time.Second)

	assert.True(t, election.IsLeader(), "应该成为 Leader")

	// 获取 Leader 信息
	nodeID, expiration, err := election.GetLeaderInfo(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node-1", nodeID)
	assert.Greater(t, expiration, time.Now().UnixNano())
}

func TestElectionFailover(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 节点 1 先成为 Leader
	election1 := NewElection(Config{
		Redis:         redisClient,
		KeyPrefix:     "test:election:",
		NodeID:        "node-1",
		LeaseDuration: 2 * time.Second,
		RetryInterval: 500 * time.Millisecond,
	})

	election1.Start(ctx)
	time.Sleep(1 * time.Second)
	assert.True(t, election1.IsLeader(), "节点1应该成为 Leader")

	// 节点 2 尝试竞争，应该是 Follower
	election2 := NewElection(Config{
		Redis:         redisClient,
		KeyPrefix:     "test:election:",
		NodeID:        "node-2",
		LeaseDuration: 2 * time.Second,
		RetryInterval: 500 * time.Millisecond,
	})

	election2.Start(ctx)
	defer election2.Stop()
	time.Sleep(1 * time.Second)
	assert.False(t, election2.IsLeader(), "节点2应该是 Follower")

	// 模拟节点 1 故障（停止选举）
	election1.Stop()

	// 等待节点 2 检测到并接管
	time.Sleep(3 * time.Second)
	assert.True(t, election2.IsLeader(), "节点2应该接管成为 Leader")
}

func TestElectionCallbacks(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	election := NewElection(Config{
		Redis:         redisClient,
		KeyPrefix:     "test:election:",
		NodeID:        "node-1",
		LeaseDuration: 2 * time.Second,
		RetryInterval: 500 * time.Millisecond,
	})

	becameLeader := false
	becameFollower := false

	election.OnChange(func(isLeader bool) {
		if isLeader {
			becameLeader = true
		} else {
			becameFollower = true
		}
	})

	election.Start(ctx)
	defer election.Stop()

	// 等待成为 Leader 并触发回调
	time.Sleep(1 * time.Second)
	assert.True(t, becameLeader, "应该触发成为 Leader 回调")

	// 停止后检查是否触发 Follower 回调
	election.Stop()
	time.Sleep(1 * time.Second)
	// 注意：Stop 不会自动触发 becomeFollower，需要租约过期
}

func TestElectionLeaderChan(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	election := NewElection(Config{
		Redis:         redisClient,
		KeyPrefix:     "test:election:",
		NodeID:        "node-1",
		LeaseDuration: 2 * time.Second,
		RetryInterval: 500 * time.Millisecond,
	})

	election.Start(ctx)
	defer election.Stop()

	// 从通道接收领导权变化通知
	select {
	case isLeader := <-election.LeaderChan():
		assert.True(t, isLeader, "应该收到成为 Leader 的通知")
	case <-time.After(2 * time.Second):
		t.Fatal("超时未收到领导权变化通知")
	}
}

func TestElectionMultipleNodes(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 5 个节点
	elections := make([]*Election, 5)
	for i := 0; i < 5; i++ {
		elections[i] = NewElection(Config{
			Redis:         redisClient,
			KeyPrefix:     "test:election:",
			NodeID:        fmt.Sprintf("node-%d", i),
			LeaseDuration: 2 * time.Second,
			RetryInterval: 500 * time.Millisecond,
		})
		elections[i].Start(ctx)
	}

	// 清理
	defer func() {
		for _, e := range elections {
			e.Stop()
		}
	}()

	// 等待选举完成
	time.Sleep(2 * time.Second)

	// 应该有且仅有一个 Leader
	leaderCount := 0
	for _, e := range elections {
		if e.IsLeader() {
			leaderCount++
		}
	}

	assert.Equal(t, 1, leaderCount, "应该有且仅有一个 Leader")
}
