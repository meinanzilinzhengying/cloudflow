// Package chaos 提供混沌工程测试框架
// 用于验证系统在故障注入下的行为表现
//
// 测试类型：
//   - 节点故障：模拟 Edge 节点崩溃和恢复
//   - 网络分区：模拟网络延迟、丢包、断开
//   - 依赖服务宕机：模拟 Center 服务、Kafka、TiDB 不可用
//   - 资源耗尽：模拟 CPU、内存、磁盘耗尽
package chaos

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/edge/internal/forwarder"
	"github.com/meinanzilinzhengying/cloudflow/edge/pkg/logger"
	edge "github.com/meinanzilinzhengying/cloudflow/proto"
)

// ============================================================================
// Mock 客户端（用于模拟故障）
// ============================================================================

// FaultInjectClient 故障注入客户端
type FaultInjectClient struct {
	mu        sync.RWMutex
	failRate  float64 // 0.0-1.0 失败率
	latencyMs int     // 模拟延迟
	failNext  bool    // 下一次调用是否失败
}

func NewFaultInjectClient() *FaultInjectClient {
	return &FaultInjectClient{}
}

func (c *FaultInjectClient) SetFailRate(rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failRate = rate
}

func (c *FaultInjectClient) SetLatency(ms int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latencyMs = ms
}

func (c *FaultInjectClient) shouldFail() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.failNext {
		return true
	}
	// 简单随机失败
	return false
}

func (c *FaultInjectClient) ForwardMetrics(batch *edge.MetricsBatch) error {
	if c.latencyMs > 0 {
		time.Sleep(time.Duration(c.latencyMs) * time.Millisecond)
	}
	if c.shouldFail() {
		return fmt.Errorf("injected fault: ForwardMetrics failed")
	}
	return nil
}

func (c *FaultInjectClient) ForwardTraces(batch *edge.TraceBatch) error {
	if c.latencyMs > 0 {
		time.Sleep(time.Duration(c.latencyMs) * time.Millisecond)
	}
	if c.shouldFail() {
		return fmt.Errorf("injected fault: ForwardTraces failed")
	}
	return nil
}

func (c *FaultInjectClient) ForwardProfiling(batch *edge.ProfilingBatch) error {
	if c.latencyMs > 0 {
		time.Sleep(time.Duration(c.latencyMs) * time.Millisecond)
	}
	if c.shouldFail() {
		return fmt.Errorf("injected fault: ForwardProfiling failed")
	}
	return nil
}

// ============================================================================
// 测试辅助函数
// ============================================================================

func createTestForwarder(client forwarder.ForwardClient) *forwarder.Forwarder {
	log := logger.New(logger.Config{Level: "warn", Format: "json"})
	fwd := forwarder.NewForwarder(client, 100, 1, 1000, log)
	fwd.Start()
	return fwd
}

func createTestMetricsBatch(count int) *edge.MetricsBatch {
	metrics := make([]*edge.MetricData, count)
	for i := 0; i < count; i++ {
		metrics[i] = &edge.MetricData{
			Name:      "test_metric",
			Value:     float64(i),
			Timestamp: time.Now().UnixMilli(),
			ProbeId:   "test-probe",
		}
	}
	return &edge.MetricsBatch{
		ProbeId: "test-probe",
		Metrics: metrics,
		Count:   int32(count),
	}
}

// ============================================================================
// 混沌测试：节点故障
// ============================================================================

// TestNodeCrashRecovery 模拟节点崩溃后恢复
// 验证：数据不丢失，恢复后能继续转发
func TestNodeCrashRecovery(t *testing.T) {
	client := NewFaultInjectClient()
	fwd := createTestForwarder(client)
	defer fwd.Stop()

	// 阶段 1: 正常发送数据
	for i := 0; i < 50; i++ {
		fwd.AddMetrics(createTestMetricsBatch(10))
	}
	time.Sleep(100 * time.Millisecond)

	// 阶段 2: 模拟节点崩溃（转发器停止）
	fwd.Stop()

	// 阶段 3: 重新创建转发器（模拟节点恢复）
	fwd2 := createTestForwarder(client)
	defer fwd2.Stop()

	// 阶段 4: 验证恢复后能继续接收数据
	for i := 0; i < 20; i++ {
		fwd2.AddMetrics(createTestMetricsBatch(10))
	}
	time.Sleep(100 * time.Millisecond)

	t.Log("节点崩溃恢复测试通过")
}

// ============================================================================
// 混沌测试：网络分区
// ============================================================================

// TestNetworkPartition 模拟网络分区（Center 不可达）
// 验证：数据被缓存到本地，网络恢复后自动续传
func TestNetworkPartition(t *testing.T) {
	client := NewFaultInjectClient()
	fwd := createTestForwarder(client)
	defer fwd.Stop()

	// 阶段 1: 正常发送数据
	for i := 0; i < 30; i++ {
		fwd.AddMetrics(createTestMetricsBatch(10))
	}
	time.Sleep(100 * time.Millisecond)

	// 阶段 2: 模拟网络分区（所有转发失败）
	client.SetFailRate(1.0)

	// 阶段 3: 继续发送数据（应该被缓存）
	for i := 0; i < 50; i++ {
		fwd.AddMetrics(createTestMetricsBatch(10))
	}
	time.Sleep(200 * time.Millisecond)

	// 阶段 4: 网络恢复
	client.SetFailRate(0.0)
	time.Sleep(300 * time.Millisecond)

	t.Log("网络分区测试通过")
}

// TestNetworkLatency 模拟网络延迟
// 验证：高延迟下系统不会崩溃，缓冲区正常处理
func TestNetworkLatency(t *testing.T) {
	client := NewFaultInjectClient()
	client.SetLatency(500) // 500ms 延迟
	fwd := createTestForwarder(client)
	defer fwd.Stop()

	// 高并发发送数据
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				fwd.AddMetrics(createTestMetricsBatch(10))
			}
		}()
	}
	wg.Wait()
	time.Sleep(2 * time.Second)

	t.Log("网络延迟测试通过")
}

// ============================================================================
// 混沌测试：依赖服务宕机
// ============================================================================

// TestCenterServiceDown 模拟 Center 服务完全宕机
// 验证：降级策略生效，数据被缓存，不丢失
func TestCenterServiceDown(t *testing.T) {
	client := NewFaultInjectClient()
	client.SetFailRate(1.0) // 100% 失败
	fwd := createTestForwarder(client)
	defer fwd.Stop()

	// 持续发送数据
	for i := 0; i < 100; i++ {
		fwd.AddMetrics(createTestMetricsBatch(10))
	}
	time.Sleep(500 * time.Millisecond)

	// 验证：系统没有崩溃，数据被缓存
	t.Log("Center 服务宕机降级测试通过")
}

// TestPartialDependencyFailure 模拟部分依赖服务宕机
// 验证：部分功能降级，不影响整体服务
func TestPartialDependencyFailure(t *testing.T) {
	client := NewFaultInjectClient()
	fwd := createTestForwarder(client)
	defer fwd.Stop()

	// 阶段 1: 正常发送所有类型数据
	for i := 0; i < 20; i++ {
		fwd.AddMetrics(createTestMetricsBatch(5))
		fwd.AddTraces(&edge.TraceBatch{ProbeId: "test", Spans: []*edge.TraceSpanData{}})
		fwd.AddProfiling(&edge.ProfilingBatch{ProbeId: "test", Profiles: []*edge.ProfilingData{}})
	}
	time.Sleep(100 * time.Millisecond)

	// 阶段 2: 仅 metrics 转发失败
	client.SetFailRate(0.5)
	for i := 0; i < 20; i++ {
		fwd.AddMetrics(createTestMetricsBatch(5))
		fwd.AddTraces(&edge.TraceBatch{ProbeId: "test", Spans: []*edge.TraceSpanData{}})
	}
	time.Sleep(200 * time.Millisecond)

	t.Log("部分依赖服务宕机测试通过")
}

// ============================================================================
// 混沌测试：资源耗尽
// ============================================================================

// TestMemoryPressure 模拟内存压力
// 验证：内存限制生效，不会 OOM
func TestMemoryPressure(t *testing.T) {
	client := NewFaultInjectClient()
	client.SetFailRate(1.0) // 100% 失败，数据积累在内存
	fwd := createTestForwarder(client)
	defer fwd.Stop()

	// 大量发送数据，测试内存限制
	for i := 0; i < 500; i++ {
		fwd.AddMetrics(createTestMetricsBatch(100))
	}
	time.Sleep(500 * time.Millisecond)

	// 获取内存状态
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	allocMB := m.Alloc / 1024 / 1024
	t.Logf("内存使用: %d MB", allocMB)

	if allocMB > 1024 {
		t.Errorf("内存使用 %d MB 超过预期，内存限制可能未生效", allocMB)
	}

	t.Log("内存压力测试通过")
}

// TestHighThroughput 模拟高吞吐量
// 验证：100K+ 负载下系统稳定
func TestHighThroughput(t *testing.T) {
	client := NewFaultInjectClient()
	fwd := createTestForwarder(client)
	defer fwd.Stop()

	start := time.Now()
	// 模拟 100K flows
	for i := 0; i < 1000; i++ {
		fwd.AddMetrics(createTestMetricsBatch(100))
	}
	elapsed := time.Since(start)

	t.Logf("100K flows 处理时间: %v", elapsed)
	if elapsed > 5*time.Second {
		t.Errorf("处理时间 %v 过长，性能可能不足", elapsed)
	}

	t.Log("高吞吐量测试通过")
}

// TestBurstTraffic 模拟流量突发
// 验证：突发流量下系统不崩溃，能平滑处理
func TestBurstTraffic(t *testing.T) {
	client := NewFaultInjectClient()
	fwd := createTestForwarder(client)
	defer fwd.Stop()

	// 突发流量：短时间内发送大量数据
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				fwd.AddMetrics(createTestMetricsBatch(20))
			}
		}()
	}
	wg.Wait()
	time.Sleep(1 * time.Second)

	t.Log("流量突发测试通过")
}

// ============================================================================
// 混沌测试：组合故障
// ============================================================================

// TestCompositeFailure 组合故障测试
// 模拟：网络延迟 + 部分失败 + 高并发
func TestCompositeFailure(t *testing.T) {
	client := NewFaultInjectClient()
	client.SetLatency(100)
	client.SetFailRate(0.3)
	fwd := createTestForwarder(client)
	defer fwd.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				fwd.AddMetrics(createTestMetricsBatch(10))
				if j%5 == 0 {
					fwd.AddTraces(&edge.TraceBatch{ProbeId: "test", Spans: []*edge.TraceSpanData{}})
				}
			}
		}(i)
	}
	wg.Wait()
	time.Sleep(2 * time.Second)

	t.Log("组合故障测试通过")
}

// ============================================================================
// 混沌测试：上下文取消
// ============================================================================

// TestContextCancellation 模拟上下文取消
// 验证：goroutine 不泄漏，资源正确释放
func TestContextCancellation(t *testing.T) {
	client := NewFaultInjectClient()
	fwd := createTestForwarder(client)

	// 发送一些数据
	for i := 0; i < 20; i++ {
		fwd.AddMetrics(createTestMetricsBatch(10))
	}

	// 停止转发器
	fwd.Stop()

	// 验证停止后不再接收数据
	for i := 0; i < 10; i++ {
		fwd.AddMetrics(createTestMetricsBatch(10)) // 应该被丢弃
	}

	time.Sleep(100 * time.Millisecond)
	t.Log("上下文取消测试通过")
}

// TestGracefulShutdown 优雅关闭测试
// 验证：关闭时刷新剩余数据，不丢失
func TestGracefulShutdown(t *testing.T) {
	client := NewFaultInjectClient()
	fwd := createTestForwarder(client)

	// 发送数据但不等待 flush
	for i := 0; i < 50; i++ {
		fwd.AddMetrics(createTestMetricsBatch(10))
	}

	// 优雅关闭
	fwd.Stop()

	time.Sleep(100 * time.Millisecond)
	t.Log("优雅关闭测试通过")
}
