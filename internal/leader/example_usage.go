// Package example 演示如何在实际服务中使用 Leader 选举
package example

import (
	"context"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/meinanzilinzhengying/cloudflow/internal/leader"
)

// ExampleUsage 演示 Leader 选举的使用
func ExampleUsage() {
	// 1. 创建 Redis 客户端
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})

	// 2. 创建 Leader 选举器
	election := leader.NewElection(leader.Config{
		Redis:         redisClient,
		KeyPrefix:     "cloudflow:center:",
		NodeID:        "center-node-1", // 从环境变量或配置文件读取
		LeaseDuration: 10 * time.Second,
		RetryInterval: 2 * time.Second,
	})

	// 3. 注册领导权变化回调
	election.OnChange(func(isLeader bool) {
		if isLeader {
			log.Println("🎉 成为 Leader，启动定时任务...")
			startScheduledTasks()
		} else {
			log.Println("⚠️  失去 Leader，停止定时任务...")
			stopScheduledTasks()
		}
	})

	// 4. 启动选举
	ctx := context.Background()
	election.Start(ctx)
	defer election.Stop()

	// 5. 主循环 - 只有 Leader 执行特定任务
	for {
		if election.IsLeader() {
			// 执行 Leader 专属任务
			performLeaderTasks()
		} else {
			// Follower 可以执行其他任务（如只读查询）
			performFollowerTasks()
		}

		time.Sleep(1 * time.Second)
	}
}

var taskCtx context.Context
var taskCancel context.CancelFunc

func startScheduledTasks() {
	taskCtx, taskCancel = context.WithCancel(context.Background())

	// 启动数据清理任务
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-taskCtx.Done():
				return
			case <-ticker.C:
				cleanupOldData()
			}
		}
	}()

	// 启动备份任务
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-taskCtx.Done():
				return
			case <-ticker.C:
				performBackup()
			}
		}
	}()
}

func stopScheduledTasks() {
	if taskCancel != nil {
		taskCancel()
	}
}

func performLeaderTasks() {
	// Leader 专属任务：
	// - 数据聚合
	// - 报表生成
	// - 告警处理
	// - 配置同步
	log.Println("执行 Leader 任务...")
}

func performFollowerTasks() {
	// Follower 任务：
	// - 处理查询请求
	// - 缓存更新
	// - 健康检查
	log.Println("执行 Follower 任务...")
}

func cleanupOldData() {
	log.Println("清理过期数据...")
}

func performBackup() {
	log.Println("执行数据备份...")
}

// ExampleWithGracefulShutdown 演示优雅关闭
func ExampleWithGracefulShutdown() {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	election := leader.NewElection(leader.Config{
		Redis:         redisClient,
		KeyPrefix:     "cloudflow:center:",
		NodeID:        "center-node-1",
		LeaseDuration: 10 * time.Second,
		RetryInterval: 2 * time.Second,
	})

	// 监听领导权变化
	go func() {
		for isLeader := range election.LeaderChan() {
			if isLeader {
				log.Println("节点成为 Leader")
			} else {
				log.Println("节点失去 Leader")
			}
		}
	}()

	ctx := context.Background()
	election.Start(ctx)

	// 模拟服务运行
	time.Sleep(30 * time.Second)

	// 优雅关闭
	election.Stop()
	redisClient.Close()
	log.Println("服务已关闭")
}
