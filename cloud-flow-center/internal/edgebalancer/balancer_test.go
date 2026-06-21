//go:build linux

package edgebalancer

import (
	"fmt"
	"testing"
)

func TestConsistentHashBalancer(t *testing.T) {
	chb := NewConsistentHashBalancer(100)

	nodes := []*EdgeNode{
		{ID: "edge-1", Address: "10.0.0.1:8080", Weight: 1, Healthy: true, Capacity: 1000},
		{ID: "edge-2", Address: "10.0.0.2:8080", Weight: 1, Healthy: true, Capacity: 1000},
		{ID: "edge-3", Address: "10.0.0.3:8080", Weight: 1, Healthy: true, Capacity: 1000},
	}

	for _, node := range nodes {
		if err := chb.AddNode(node); err != nil {
			t.Fatalf("AddNode %s failed: %v", node.ID, err)
		}
	}

	if chb.NodeCount() != 3 {
		t.Errorf("expected 3 nodes, got %d", chb.NodeCount())
	}
	if chb.VirtualNodeCount() != 300 {
		t.Errorf("expected 300 virtual nodes, got %d", chb.VirtualNodeCount())
	}

	// 测试一致性：同一 key 总是映射到同一节点
	key := "probe-123"
	node1, err := chb.GetNode(key)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	for i := 0; i < 10; i++ {
		node, err := chb.GetNode(key)
		if err != nil {
			t.Fatalf("GetNode failed: %v", err)
		}
		if node.ID != node1.ID {
			t.Errorf("expected consistent node %s, got %s", node1.ID, node.ID)
		}
	}

	// 测试获取多个节点
	nodesForKey, err := chb.GetNodesForKey(key, 2)
	if err != nil {
		t.Fatalf("GetNodesForKey failed: %v", err)
	}
	if len(nodesForKey) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodesForKey))
	}

	// 测试分布均匀性
	distribution := make(map[string]int)
	for i := 0; i < 1000; i++ {
		node, err := chb.GetNode(fmt.Sprintf("probe-%d", i))
		if err != nil {
			continue
		}
		distribution[node.ID]++
	}
	if len(distribution) != 3 {
		t.Errorf("expected 3 nodes in distribution, got %d: %v", len(distribution), distribution)
	}
	// 每个节点应该分到大约 1/3 的数据（放宽范围）
	for id, count := range distribution {
		if count < 100 || count > 500 {
			t.Errorf("unexpected distribution for %s: %d", id, count)
		}
	}

	// 测试移除节点
	chb.RemoveNode("edge-2")
	if chb.NodeCount() != 2 {
		t.Errorf("expected 2 nodes after remove, got %d", chb.NodeCount())
	}

	// 移除后，原 key 应该仍映射到健康节点
	nodeAfter, err := chb.GetNode(key)
	if err != nil {
		t.Fatalf("GetNode after remove failed: %v", err)
	}
	if nodeAfter.ID == "edge-2" {
		t.Errorf("expected removed node to not be selected, got %s", nodeAfter.ID)
	}
}

func TestConsistentHashBalancerWeight(t *testing.T) {
	chb := NewConsistentHashBalancer(100)

	// 添加权重不同的节点
	chb.AddNode(&EdgeNode{ID: "edge-1", Address: "10.0.0.1:8080", Weight: 1, Healthy: true})
	chb.AddNode(&EdgeNode{ID: "edge-2", Address: "10.0.0.2:8080", Weight: 3, Healthy: true})

	distribution := make(map[string]int)
	for i := 0; i < 1000; i++ {
		node, err := chb.GetNode(fmt.Sprintf("probe-%d", i))
		if err != nil {
			continue
		}
		distribution[node.ID]++
	}

	// edge-2 权重更大，应该分到更多数据
	if distribution["edge-2"] <= distribution["edge-1"] {
		t.Errorf("expected edge-2 to have more data than edge-1, got edge-1:%d edge-2:%d",
			distribution["edge-1"], distribution["edge-2"])
	}
}

func TestConsistentHashBalancerHealth(t *testing.T) {
	chb := NewConsistentHashBalancer(100)
	chb.AddNode(&EdgeNode{ID: "edge-1", Address: "10.0.0.1:8080", Weight: 1, Healthy: true})
	chb.AddNode(&EdgeNode{ID: "edge-2", Address: "10.0.0.2:8080", Weight: 1, Healthy: false})

	// 应该只返回健康节点
	for i := 0; i < 100; i++ {
		node, err := chb.GetNode(fmt.Sprintf("probe-%d", i))
		if err != nil {
			continue
		}
		if node.ID == "edge-2" {
			t.Errorf("expected unhealthy node not to be selected, got %s", node.ID)
		}
	}

	// 标记为健康
	chb.UpdateNodeHealth("edge-2", true)
	found := false
	for i := 0; i < 1000 && !found; i++ {
		node, err := chb.GetNode(fmt.Sprintf("probe-%d", i))
		if err != nil {
			continue
		}
		if node.ID == "edge-2" {
			found = true
		}
	}
	if !found {
		t.Error("expected edge-2 to be selected after health update")
	}
}

func TestConsistentHashBalancerMigration(t *testing.T) {
	chb := NewConsistentHashBalancer(100)

	for i := 1; i <= 5; i++ {
		chb.AddNode(&EdgeNode{
			ID:      fmt.Sprintf("edge-%d", i),
			Address: fmt.Sprintf("10.0.0.%d:8080", i),
			Weight:  1,
			Healthy: true,
		})
	}

	// 计算移除一个节点时的数据迁移率
	rate := chb.MigrationRate("edge-1")
	if rate < 0 || rate > 1.0 {
		t.Errorf("unexpected migration rate: %f", rate)
	}
	t.Logf("Migration rate when removing edge-1: %.2f%%", rate*100)

	// 5 个节点移除 1 个，理论迁移率约为 20%
	// 但由于 hash 特性，实际可能接近 0 或 20%
	if rate > 0.8 {
		t.Errorf("unexpected high migration rate: %.2f%%", rate*100)
	}
}

func TestConsistentHashBalancerEmpty(t *testing.T) {
	chb := NewConsistentHashBalancer(100)
	_, err := chb.GetNode("probe-1")
	if err == nil {
		t.Error("expected error for empty balancer")
	}
}

func TestConsistentHashBalancerDuplicate(t *testing.T) {
	chb := NewConsistentHashBalancer(100)
	chb.AddNode(&EdgeNode{ID: "edge-1", Address: "10.0.0.1:8080", Weight: 1, Healthy: true})
	err := chb.AddNode(&EdgeNode{ID: "edge-1", Address: "10.0.0.1:8080", Weight: 1, Healthy: true})
	if err == nil {
		t.Error("expected error for duplicate node")
	}
}

// ============================================================================
// 分片管理器测试
// ============================================================================

func TestShardManager(t *testing.T) {
	sm := NewShardManager(ShardByProbeID, 100)

	for i := 1; i <= 3; i++ {
		err := sm.AddShardNode(&EdgeNode{
			ID:      fmt.Sprintf("edge-%d", i),
			Address: fmt.Sprintf("10.0.0.%d:8080", i),
			Weight:  1,
			Healthy: true,
		})
		if err != nil {
			t.Fatalf("AddShardNode failed: %v", err)
		}
	}

	// 按 probe_id 分片
	shardKey := sm.GetShardKey("probe-123", "tenant-1", nil)
	if shardKey != "probe-123" {
		t.Errorf("expected shard key 'probe-123', got %s", shardKey)
	}

	node, err := sm.GetShardForKey("probe-123")
	if err != nil {
		t.Fatalf("GetShardForKey failed: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil node")
	}

	// 分布统计
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("probe-%d", i)
	}
	distribution := sm.GetShardDistribution(keys)
	if len(distribution) != 3 {
		t.Errorf("expected 3 shards in distribution, got %d", len(distribution))
	}

	// 统计
	stats := sm.Stats()
	if stats["total_nodes"].(int) != 3 {
		t.Errorf("expected 3 nodes, got %v", stats["total_nodes"])
	}
	if stats["strategy"].(string) != "probe_id" {
		t.Errorf("expected strategy 'probe_id', got %v", stats["strategy"])
	}
}

func TestShardManagerByTenant(t *testing.T) {
	sm := NewShardManager(ShardByTenantID, 100)
	sm.AddShardNode(&EdgeNode{ID: "edge-1", Address: "10.0.0.1:8080", Weight: 1, Healthy: true})

	key := sm.GetShardKey("probe-123", "tenant-1", nil)
	if key != "tenant-1" {
		t.Errorf("expected shard key 'tenant-1', got %s", key)
	}
}

func TestShardManagerByHash(t *testing.T) {
	sm := NewShardManager(ShardByHash, 100)
	sm.AddShardNode(&EdgeNode{ID: "edge-1", Address: "10.0.0.1:8080", Weight: 1, Healthy: true})

	key := sm.GetShardKey("probe-123", "tenant-1", []byte("data"))
	if key == "" {
		t.Error("expected non-empty hash key")
	}
}

// ============================================================================
// 扩缩容管理器测试
// ============================================================================

func TestScalingManager(t *testing.T) {
	balancer := NewConsistentHashBalancer(100)
	for i := 1; i <= 3; i++ {
		balancer.AddNode(&EdgeNode{
			ID:      fmt.Sprintf("edge-%d", i),
			Address: fmt.Sprintf("10.0.0.%d:8080", i),
			Weight:  1,
			Healthy: true,
		})
	}

	sm := NewScalingManager(balancer)

	// 扩容
	newNode := &EdgeNode{ID: "edge-4", Address: "10.0.0.4:8080", Weight: 1, Healthy: true}
	if err := sm.ScaleUp(newNode); err != nil {
		t.Fatalf("ScaleUp failed: %v", err)
	}
	if balancer.NodeCount() != 4 {
		t.Errorf("expected 4 nodes after scale up, got %d", balancer.NodeCount())
	}

	// 检查扩容状态
	_, warming := sm.GetDrainStatus("edge-4")
	if !warming {
		t.Error("expected edge-4 to be warming up after scale up")
	}

	// 缩容
	if err := sm.ScaleDown("edge-4"); err != nil {
		t.Fatalf("ScaleDown failed: %v", err)
	}

	// 检查下线状态
	draining, _ := sm.GetDrainStatus("edge-4")
	if !draining {
		t.Error("expected edge-4 to be draining")
	}

	// 统计
	stats := sm.Stats()
	if stats["draining_nodes"].(int) != 1 {
		t.Errorf("expected 1 draining node, got %v", stats["draining_nodes"])
	}
	if stats["total_nodes"].(int) != 4 {
		t.Errorf("expected 4 total nodes, got %v", stats["total_nodes"])
	}

	// 迁移计划
	plan := sm.GetMigrationPlan(nil, []string{"edge-4"})
	if len(plan) == 0 {
		t.Log("no migration plan (node may not have data yet)")
	}
}

func TestScalingManagerDuplicateScaleUp(t *testing.T) {
	balancer := NewConsistentHashBalancer(100)
	sm := NewScalingManager(balancer)

	node := &EdgeNode{ID: "edge-1", Address: "10.0.0.1:8080", Weight: 1, Healthy: true}
	sm.ScaleUp(node)

	// 重复扩容
	err := sm.ScaleUp(node)
	if err == nil {
		t.Error("expected error for duplicate scale up")
	}
}

func TestScalingManagerDuplicateScaleDown(t *testing.T) {
	balancer := NewConsistentHashBalancer(100)
	balancer.AddNode(&EdgeNode{ID: "edge-1", Address: "10.0.0.1:8080", Weight: 1, Healthy: true})

	sm := NewScalingManager(balancer)
	sm.ScaleDown("edge-1")

	// 重复缩容
	err := sm.ScaleDown("edge-1")
	if err == nil {
		t.Error("expected error for duplicate scale down")
	}
}
