package hashring

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestNewRing(t *testing.T) {
	ring := NewRing(100)
	if ring == nil {
		t.Fatal("NewRing 返回 nil")
	}
	if ring.replicas != 100 {
		t.Errorf("replicas = %d, want 100", ring.replicas)
	}
}

func TestNewRing_DefaultReplicas(t *testing.T) {
	ring := NewRing(0)
	if ring.replicas != 150 {
		t.Errorf("默认 replicas = %d, want 150", ring.replicas)
	}
}

func TestAddNode(t *testing.T) {
	ring := NewRing(10)
	ring.AddNode("edge-01")

	if ring.GetNodeCount() != 1 {
		t.Errorf("节点数 = %d, want 1", ring.GetNodeCount())
	}

	// 重复添加不应增加
	ring.AddNode("edge-01")
	if ring.GetNodeCount() != 1 {
		t.Errorf("重复添加后节点数 = %d, want 1", ring.GetNodeCount())
	}
}

func TestRemoveNode(t *testing.T) {
	ring := NewRing(10)
	ring.AddNode("edge-01")
	ring.AddNode("edge-02")
	ring.RemoveNode("edge-01")

	if ring.GetNodeCount() != 1 {
		t.Errorf("移除后节点数 = %d, want 1", ring.GetNodeCount())
	}

	// 移除不存在的节点不应 panic
	ring.RemoveNode("edge-99")
	if ring.GetNodeCount() != 1 {
		t.Errorf("移除不存在的节点后节点数 = %d, want 1", ring.GetNodeCount())
	}
}

func TestGetNode(t *testing.T) {
	ring := NewRing(100)
	ring.AddNode("edge-01")

	node, ok := ring.GetNode("probe-001")
	if !ok {
		t.Error("GetNode 返回 false")
	}
	if node != "edge-01" {
		t.Errorf("GetNode = %q, want edge-01", node)
	}
}

func TestGetNode_EmptyRing(t *testing.T) {
	ring := NewRing(10)
	_, ok := ring.GetNode("probe-001")
	if ok {
		t.Error("空环应返回 false")
	}
}

func TestGetNode_Distribution(t *testing.T) {
	ring := NewRing(150)
	ring.AddNode("edge-01")
	ring.AddNode("edge-02")
	ring.AddNode("edge-03")

	// 分配 10000 个 key，检查分布
	counts := map[string]int{}
	total := 10000
	for i := 0; i < total; i++ {
		key := fmt.Sprintf("probe-%d", i)
		node, _ := ring.GetNode(key)
		counts[node]++
	}

	// 每个节点应分配约 33% 的 key
	for node, count := range counts {
		ratio := float64(count) / float64(total)
		if ratio < 0.20 || ratio > 0.50 {
			t.Errorf("节点 %s 分配比例 %.2f，期望 0.20~0.50", node, ratio)
		}
	}
}

func TestGetNodes(t *testing.T) {
	ring := NewRing(100)
	ring.AddNode("edge-01")
	ring.AddNode("edge-02")
	ring.AddNode("edge-03")

	nodes := ring.GetNodes("probe-001", 2)
	if len(nodes) != 2 {
		t.Fatalf("GetNodes 返回 %d 个节点, want 2", len(nodes))
	}
	if nodes[0] == nodes[1] {
		t.Errorf("返回了重复节点: %v", nodes)
	}
}

func TestGetNodes_MoreThanAvailable(t *testing.T) {
	ring := NewRing(100)
	ring.AddNode("edge-01")
	ring.AddNode("edge-02")

	nodes := ring.GetNodes("probe-001", 5)
	if len(nodes) != 2 {
		t.Errorf("GetNodes 返回 %d 个节点, want 2", len(nodes))
	}
}

func TestConsistency_AfterAddNode(t *testing.T) {
	ring := NewRing(150)
	ring.AddNode("edge-01")
	ring.AddNode("edge-02")

	// 记录初始分配
	assignments := map[string]string{}
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("probe-%d", i)
		node, _ := ring.GetNode(key)
		assignments[key] = node
	}

	// 添加新节点
	ring.AddNode("edge-03")

	// 检查迁移率
	migrated := 0
	for key, oldNode := range assignments {
		newNode, _ := ring.GetNode(key)
		if newNode != oldNode {
			migrated++
		}
	}

	ratio := float64(migrated) / float64(len(assignments))
	if ratio > 0.5 {
		t.Errorf("迁移率 %.2f 过高，期望 < 0.5", ratio)
	}
	t.Logf("添加节点后迁移率: %.4f (%d/1000)", ratio, migrated)
}

func TestConsistency_AfterRemoveNode(t *testing.T) {
	ring := NewRing(150)
	ring.AddNode("edge-01")
	ring.AddNode("edge-02")
	ring.AddNode("edge-03")

	assignments := map[string]string{}
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("probe-%d", i)
		node, _ := ring.GetNode(key)
		assignments[key] = node
	}

	ring.RemoveNode("edge-02")

	migrated := 0
	for key, oldNode := range assignments {
		newNode, _ := ring.GetNode(key)
		if newNode != oldNode {
			migrated++
		}
	}

	ratio := float64(migrated) / float64(len(assignments))
	if ratio > 0.5 {
		t.Errorf("迁移率 %.2f 过高，期望 < 0.5", ratio)
	}
	t.Logf("移除节点后迁移率: %.4f (%d/1000)", ratio, migrated)
}

func TestMigrationRate(t *testing.T) {
	oldRing := NewRing(150)
	oldRing.AddNode("edge-01")
	oldRing.AddNode("edge-02")

	newRing := NewRing(150)
	newRing.AddNode("edge-01")
	newRing.AddNode("edge-02")
	newRing.AddNode("edge-03")

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("probe-%d", i)
	}

	rate := newRing.MigrationRate(keys, oldRing)
	if rate > 0.5 {
		t.Errorf("迁移率 %.2f 过高", rate)
	}
	t.Logf("迁移率: %.4f", rate)
}

func TestDeterministic(t *testing.T) {
	ring := NewRing(100)
	ring.AddNode("edge-01")

	node1, _ := ring.GetNode("probe-001")
	node2, _ := ring.GetNode("probe-001")
	node3, _ := ring.GetNode("probe-001")

	if node1 != node2 || node2 != node3 {
		t.Error("同一 key 多次查询结果不一致")
	}
}

func TestLargeScale(t *testing.T) {
	// 模拟 100 个 Edge 节点，10000 个 Agent
	ring := NewRing(150)
	for i := 0; i < 100; i++ {
		ring.AddNode(fmt.Sprintf("edge-%03d", i))
	}

	counts := map[string]int{}
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("probe-%d", i)
		node, _ := ring.GetNode(key)
		counts[node]++
	}

	// 每个节点应分配约 100 个 agent
	for node, count := range counts {
		if count < 50 || count > 200 {
			t.Errorf("节点 %s 分配 %d 个 agent，期望 50~200", node, count)
		}
	}
	t.Logf("100 个 Edge 节点分配 10000 个 Agent: 最少=%d, 最多=%d",
		minCount(counts), maxCount(counts))
}

func TestParseNodeID(t *testing.T) {
	tests := []struct {
		vKey string
		want string
	}{
		{"edge-01#160", "edge-01"},
		{"edge-02#0", "edge-02"},
		{"edge-03#999", "edge-03"},
		{"no-hash", "no-hash"},
	}

	for _, tt := range tests {
		got := ParseNodeID(tt.vKey)
		if got != tt.want {
			t.Errorf("ParseNodeID(%q) = %q, want %q", tt.vKey, got, tt.want)
		}
	}
}

func TestGetNodesList(t *testing.T) {
	ring := NewRing(10)
	ring.AddNode("edge-01")
	ring.AddNode("edge-02")
	ring.AddNode("edge-03")

	nodes := ring.GetNodesList()
	if len(nodes) != 3 {
		t.Errorf("GetNodesList 返回 %d 个节点, want 3", len(nodes))
	}
}

func TestConcurrentAccess(t *testing.T) {
	ring := NewRing(100)
	ring.AddNode("edge-01")
	ring.AddNode("edge-02")

	done := make(chan struct{})
	const goroutines = 50
	const opsPerGoroutine = 100

	// 并发读
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < opsPerGoroutine; j++ {
				ring.GetNode(fmt.Sprintf("probe-%d-%d", id, j))
			}
			done <- struct{}{}
		}(i)
	}

	// 并发写
	for i := 0; i < 5; i++ {
		go func(id int) {
			nodeID := fmt.Sprintf("edge-new-%d", id)
			ring.AddNode(nodeID)
			ring.RemoveNode(nodeID)
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < goroutines+5; i++ {
		<-done
	}
}

func minCount(m map[string]int) int {
	min := int(^uint(0) >> 1)
	for _, v := range m {
		if v < min {
			min = v
		}
	}
	return min
}

func maxCount(m map[string]int) int {
	max := 0
	for _, v := range m {
		if v > max {
			max = v
		}
	}
	return max
}

func BenchmarkGetNode(b *testing.B) {
	ring := NewRing(150)
	for i := 0; i < 10; i++ {
		ring.AddNode(fmt.Sprintf("edge-%02d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ring.GetNode(fmt.Sprintf("probe-%d", rand.Intn(100000)))
	}
}
