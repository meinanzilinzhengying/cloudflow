package forwarder

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"cloud-flow-edge/pkg/logger"
	"cloud-flow-edge/pkg/testutil"
	edge "cloud-flow/proto"
)

// mockClient 模拟中心服务客户端，记录转发调用
type mockClient struct {
	mu              sync.Mutex
	metricsBatches  []*edge.MetricsBatch
	tracesBatches   []*edge.TraceBatch
	profileBatches  []*edge.ProfilingBatch
	failOnMetrics   int
	metricsCallCount int
	failSequence    []bool
	sequenceIndex   int
}

func (m *mockClient) ForwardMetrics(batch *edge.MetricsBatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metricsCallCount++

	if len(m.failSequence) > 0 {
		if m.sequenceIndex < len(m.failSequence) && m.failSequence[m.sequenceIndex] {
			m.sequenceIndex++
			return fmt.Errorf("模拟转发失败")
		}
		m.sequenceIndex++
	} else if m.failOnMetrics > 0 && m.metricsCallCount == m.failOnMetrics {
		return fmt.Errorf("模拟转发失败")
	}
	m.metricsBatches = append(m.metricsBatches, batch)
	return nil
}

func (m *mockClient) ForwardTraces(batch *edge.TraceBatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tracesBatches = append(m.tracesBatches, batch)
	return nil
}

func (m *mockClient) ForwardProfiling(batch *edge.ProfilingBatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profileBatches = append(m.profileBatches, batch)
	return nil
}

func (m *mockClient) GetMetricsBatches() []*edge.MetricsBatch {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*edge.MetricsBatch, len(m.metricsBatches))
	copy(result, m.metricsBatches)
	return result
}

func (m *mockClient) GetTracesBatches() []*edge.TraceBatch {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*edge.TraceBatch, len(m.tracesBatches))
	copy(result, m.tracesBatches)
	return result
}

func (m *mockClient) GetProfilingBatches() []*edge.ProfilingBatch {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*edge.ProfilingBatch, len(m.profileBatches))
	copy(result, m.profileBatches)
	return result
}

// clearTestPersistenceData 清理测试用的持久化数据
func clearTestPersistenceData() {
	os.RemoveAll("./data/wal")
	os.RemoveAll("./data/snapshots")
	if err := os.MkdirAll("./data/wal", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "创建测试目录 ./data/wal 失败: %v\n", err)
	}
	if err := os.MkdirAll("./data/snapshots", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "创建测试目录 ./data/snapshots 失败: %v\n", err)
	}
}

func newTestMetricsBatch(probeID string, count int) *edge.MetricsBatch {
	metrics := make([]*edge.MetricData, count)
	for i := 0; i < count; i++ {
		metrics[i] = &edge.MetricData{
			Timestamp: time.Now().Unix(),
			SrcIp:     "10.0.0.1",
			DstIp:     "10.0.0.2",
			Protocol:  "tcp",
		}
	}
	return &edge.MetricsBatch{
		ProbeId: probeID,
		Metrics: metrics,
	}
}

func TestAddMetrics(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 100, 300, testutil.NewTestLogger())

	batch := newTestMetricsBatch("probe-1", 5)
	fwd.AddMetrics(batch)

	fwd.muMetrics.Lock()
	bufLen := len(fwd.metricsBuf)
	fwd.muMetrics.Unlock()

	if bufLen != 1 {
		t.Fatalf("缓冲区应有 1 条, 实际 %d", bufLen)
	}
}

func TestAddMetricsAutoFlush(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	// batchSize=3，添加 3 条应自动触发 flush
	fwd := NewForwarder(mock, 3, 300, testutil.NewTestLogger())

	for i := 0; i < 3; i++ {
		fwd.AddMetrics(newTestMetricsBatch("probe-1", 1))
	}

	// 等待 flush 完成
	time.Sleep(50 * time.Millisecond)

	fwd.muMetrics.Lock()
	bufLen := len(fwd.metricsBuf)
	fwd.muMetrics.Unlock()

	if bufLen != 0 {
		t.Fatalf("flush 后缓冲区应为 0, 实际 %d", bufLen)
	}

	batches := mock.GetMetricsBatches()
	if len(batches) != 3 {
		t.Fatalf("应转发 3 批, 实际 %d", len(batches))
	}
}

func TestAddTraces(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 100, 300, testutil.NewTestLogger())

	batch := &edge.TraceBatch{
		ProbeId: "probe-1",
		Spans: []*edge.TraceSpanData{
			{TraceId: "t1", SpanId: "s1", Service: "svc-a"},
		},
	}
	fwd.AddTraces(batch)

	fwd.muTraces.Lock()
	bufLen := len(fwd.tracesBuf)
	fwd.muTraces.Unlock()

	if bufLen != 1 {
		t.Fatalf("缓冲区应有 1 条, 实际 %d", bufLen)
	}
}

func TestAddProfiling(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 100, 300, testutil.NewTestLogger())

	batch := &edge.ProfilingBatch{
		ProbeId: "probe-1",
		Profiles: []*edge.ProfilingData{
			{Type: "cpu", Stack: "main->foo", Count: 10},
		},
	}
	fwd.AddProfiling(batch)

	fwd.muProfiling.Lock()
	bufLen := len(fwd.profilingBuf)
	fwd.muProfiling.Unlock()

	if bufLen != 1 {
		t.Fatalf("缓冲区应有 1 条, 实际 %d", bufLen)
	}
}

func TestTimedFlush(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	// 100ms 定时 flush
	fwd := NewForwarder(mock, 1000, 1, testutil.NewTestLogger())
	fwd.flushInterval = 100 * time.Millisecond
	fwd.Start()
	defer fwd.Stop()

	fwd.AddMetrics(newTestMetricsBatch("probe-1", 1))
	fwd.AddMetrics(newTestMetricsBatch("probe-1", 1))

	// 等待定时 flush
	time.Sleep(200 * time.Millisecond)

	batches := mock.GetMetricsBatches()
	if len(batches) != 2 {
		t.Fatalf("定时 flush 应转发 2 批, 实际 %d", len(batches))
	}
}

func TestStopFlushesRemaining(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 1000, 300, testutil.NewTestLogger())
	fwd.Start()

	fwd.AddMetrics(newTestMetricsBatch("probe-1", 1))
	fwd.AddTraces(&edge.TraceBatch{ProbeId: "probe-1", Spans: []*edge.TraceSpanData{{TraceId: "t1"}}})
	fwd.AddProfiling(&edge.ProfilingBatch{ProbeId: "probe-1", Profiles: []*edge.ProfilingData{{Type: "cpu"}}})

	fwd.Stop()

	mBatches := mock.GetMetricsBatches()
	if len(mBatches) != 1 {
		t.Fatalf("Stop 应刷新剩余 metrics, 期望 1, 实际 %d", len(mBatches))
	}

	tBatches := mock.GetTracesBatches()
	if len(tBatches) != 1 {
		t.Fatalf("Stop 应刷新剩余 traces, 期望 1, 实际 %d", len(tBatches))
	}

	pBatches := mock.GetProfilingBatches()
	if len(pBatches) != 1 {
		t.Fatalf("Stop 应刷新剩余 profiles, 期望 1, 实际 %d", len(pBatches))
	}
}

func TestDefaultBatchSizeAndInterval(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 0, 0, testutil.NewTestLogger())

	if fwd.batchSize != 100 {
		t.Fatalf("默认 batchSize 应为 100, 实际 %d", fwd.batchSize)
	}
	if fwd.flushInterval != 5*time.Second {
		t.Fatalf("默认 flushInterval 应为 5s, 实际 %s", fwd.flushInterval)
	}
}

func TestConcurrentAdd(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	// batchSize 设大，避免自动 flush
	fwd := NewForwarder(mock, 5000, 300, testutil.NewTestLogger())

	// 并发添加，不应 panic
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fwd.AddMetrics(newTestMetricsBatch("probe-1", 1))
			fwd.AddTraces(&edge.TraceBatch{ProbeId: "probe-1", Spans: []*edge.TraceSpanData{{TraceId: "t1"}}})
			fwd.AddProfiling(&edge.ProfilingBatch{ProbeId: "probe-1", Profiles: []*edge.ProfilingData{{Type: "cpu"}}})
		}()
	}
	wg.Wait()

	fwd.muMetrics.Lock()
	mLen := len(fwd.metricsBuf)
	fwd.muMetrics.Unlock()

	fwd.muTraces.Lock()
	tLen := len(fwd.tracesBuf)
	fwd.muTraces.Unlock()

	fwd.muProfiling.Lock()
	pLen := len(fwd.profilingBuf)
	fwd.muProfiling.Unlock()

	if mLen != 100 || tLen != 100 || pLen != 100 {
		t.Fatalf("并发添加后缓冲区大小异常: metrics=%d, traces=%d, profiles=%d", mLen, tLen, pLen)
	}
}

func TestRetryCountResetOnSuccess(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 1000, 300, testutil.NewTestLogger())

	batch1 := newTestMetricsBatch("probe-1", 1)
	batch2 := newTestMetricsBatch("probe-1", 1)
	batch3 := newTestMetricsBatch("probe-1", 1)
	batch4 := newTestMetricsBatch("probe-1", 1)
	batch5 := newTestMetricsBatch("probe-1", 1)

	mock.failSequence = []bool{true, true, false, true, true}
	fwd.AddMetrics(batch1)
	fwd.AddMetrics(batch2)
	fwd.AddMetrics(batch3)
	fwd.AddMetrics(batch4)
	fwd.AddMetrics(batch5)

	fwd.flushMetrics(false)

	batches := mock.GetMetricsBatches()
	if len(batches) != 5 {
		t.Fatalf("分散失败场景应转发全部5批, 实际 %d 批", len(batches))
	}
}

func TestConsecutiveFailureTriggersDrop(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 1000, 300, testutil.NewTestLogger())

	batch1 := newTestMetricsBatch("probe-1", 1)
	batch2 := newTestMetricsBatch("probe-1", 1)
	batch3 := newTestMetricsBatch("probe-1", 1)
	batch4 := newTestMetricsBatch("probe-1", 1)

	mock.failSequence = []bool{true, true, true, true}
	fwd.AddMetrics(batch1)
	fwd.AddMetrics(batch2)
	fwd.AddMetrics(batch3)
	fwd.AddMetrics(batch4)

	fwd.flushMetrics(false)

	batches := mock.GetMetricsBatches()
	if len(batches) != 0 {
		t.Fatalf("连续3次失败应触发丢弃, 应转发0批, 实际 %d 批", len(batches))
	}

	fwd.muMetrics.Lock()
	bufLen := len(fwd.metricsBuf)
	fwd.muMetrics.Unlock()
	if bufLen != 4 {
		t.Fatalf("连续失败后批次放回缓冲区, metricsBuf应为4, 实际 %d", bufLen)
	}
}

func TestTwoFailuresThenSuccessThenThreeFails(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 1000, 300, testutil.NewTestLogger())

	batch1 := newTestMetricsBatch("probe-1", 1)
	batch2 := newTestMetricsBatch("probe-1", 1)
	batch3 := newTestMetricsBatch("probe-1", 1)
	batch4 := newTestMetricsBatch("probe-1", 1)
	batch5 := newTestMetricsBatch("probe-1", 1)
	batch6 := newTestMetricsBatch("probe-1", 1)

	mock.failSequence = []bool{true, true, false, true, true, true}
	fwd.AddMetrics(batch1)
	fwd.AddMetrics(batch2)
	fwd.AddMetrics(batch3)
	fwd.AddMetrics(batch4)
	fwd.AddMetrics(batch5)
	fwd.AddMetrics(batch6)

	fwd.flushMetrics(false)

	batches := mock.GetMetricsBatches()
	if len(batches) != 1 {
		t.Fatalf("失败2次后成功1次再失败3次，仅batch3成功转发，实际 %d 批", len(batches))
	}

	fwd.muMetrics.Lock()
	bufLen := len(fwd.metricsBuf)
	fwd.muMetrics.Unlock()
	if bufLen != 3 {
		t.Fatalf("失败批次放回缓冲区, metricsBuf应为3, 实际 %d", bufLen)
	}
}

func TestSingleFailureThenSuccess(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 1000, 300, testutil.NewTestLogger())

	batch1 := newTestMetricsBatch("probe-1", 1)
	batch2 := newTestMetricsBatch("probe-1", 1)
	batch3 := newTestMetricsBatch("probe-1", 1)

	mock.failSequence = []bool{true, false, false}
	fwd.AddMetrics(batch1)
	fwd.AddMetrics(batch2)
	fwd.AddMetrics(batch3)

	fwd.flushMetrics(false)

	batches := mock.GetMetricsBatches()
	if len(batches) != 3 {
		t.Fatalf("单次失败后成功应转发全部3批, 实际 %d 批", len(batches))
	}
}

func TestRetryCountWithFiveBatches(t *testing.T) {
	clearTestPersistenceData()
	mock := &mockClient{}
	fwd := NewForwarder(mock, 1000, 300, testutil.NewTestLogger())

	for i := 0; i < 5; i++ {
		fwd.AddMetrics(newTestMetricsBatch(fmt.Sprintf("probe-%d", i), 1))
	}

	mock.failSequence = []bool{true, true, false, true, false}
	fwd.flushMetrics(false)

	batches := mock.GetMetricsBatches()
	if len(batches) != 5 {
		t.Fatalf("5批次场景应转发全部5批, 实际 %d 批", len(batches))
	}
}
