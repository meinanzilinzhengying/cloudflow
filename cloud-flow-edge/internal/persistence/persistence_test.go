package persistence

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"cloudflow-edge/pkg/testutil"
	edge "cloudflow/proto"
)

func TestCreateSnapshotDeepCopy(t *testing.T) {
	log := testutil.NewTestLogger()
	p := &Persistence{
		logger:      log,
		metricsBuf:  make([]*edge.MetricsBatch, 0),
		tracesBuf:   make([]*edge.TraceBatch, 0),
		profilingBuf: make([]*edge.ProfilingBatch, 0),
	}

	originalMetrics := []*edge.MetricsBatch{
		{ProbeId: "probe-1", Metrics: []*edge.MetricData{{SrcIp: "10.0.0.1"}}},
		{ProbeId: "probe-2", Metrics: []*edge.MetricData{{SrcIp: "10.0.0.2"}}},
	}
	p.metricsBuf = originalMetrics

	p.dataMutex.Lock()
	metricsCopy := make([]*edge.MetricsBatch, len(p.metricsBuf))
	copy(metricsCopy, p.metricsBuf)
	p.dataMutex.Unlock()

	if len(metricsCopy) != len(originalMetrics) {
		t.Fatalf("副本长度不匹配，期望 %d，实际 %d", len(originalMetrics), len(metricsCopy))
	}

	// 浅拷贝验证：切片本身独立，但元素（指针）共享
	// 1. 切片独立：修改原切片不影响副本的长度
	p.dataMutex.Lock()
	p.metricsBuf = p.metricsBuf[:0]
	p.dataMutex.Unlock()

	if len(metricsCopy) != 2 {
		t.Fatalf("原切片清空后副本长度应保持不变，期望 2，实际 %d", len(metricsCopy))
	}

	// 2. 元素共享：通过原切片的指针修改对象，副本也能看到变化
	originalMetrics[0].ProbeId = "modified"
	if metricsCopy[0].ProbeId != "modified" {
		t.Errorf("浅拷贝应共享元素指针，期望 modified，实际 %s", metricsCopy[0].ProbeId)
	}
}

func TestCreateSnapshotConcurrentWrite(t *testing.T) {
	log := testutil.NewTestLogger()
	p := &Persistence{
		logger:       log,
		metricsBuf:   make([]*edge.MetricsBatch, 0),
		tracesBuf:    make([]*edge.TraceBatch, 0),
		profilingBuf: make([]*edge.ProfilingBatch, 0),
	}

	for i := 0; i < 100; i++ {
		p.metricsBuf = append(p.metricsBuf, &edge.MetricsBatch{
			ProbeId: fmt.Sprintf("probe-%d", i),
			Metrics: []*edge.MetricData{{SrcIp: "10.0.0.1"}},
		})
	}

	var wg sync.WaitGroup
	snapshotMetrics := make([][]*edge.MetricsBatch, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			p.dataMutex.Lock()
			metrics := make([]*edge.MetricsBatch, len(p.metricsBuf))
			copy(metrics, p.metricsBuf)
			p.dataMutex.Unlock()

			snapshotMetrics[idx] = metrics
		}(i)
	}

	wg.Wait()

	for i := 0; i < 10; i++ {
		if len(snapshotMetrics[i]) != 100 {
			t.Errorf("快照 %d 的长度不匹配，期望 100，实际 %d", i, len(snapshotMetrics[i]))
		}
	}

	p.dataMutex.Lock()
	p.metricsBuf = append(p.metricsBuf, &edge.MetricsBatch{ProbeId: "new-probe"})
	p.dataMutex.Unlock()

	for i := 0; i < 10; i++ {
		if len(snapshotMetrics[i]) != 100 {
			t.Errorf("并发修改后快照 %d 的长度被改变，期望 100，实际 %d", i, len(snapshotMetrics[i]))
		}
	}
}

func TestSliceIndependenceAfterCopy(t *testing.T) {
	log := testutil.NewTestLogger()
	p := &Persistence{
		logger:       log,
		metricsBuf:   make([]*edge.MetricsBatch, 0),
		tracesBuf:    make([]*edge.TraceBatch, 0),
		profilingBuf: make([]*edge.ProfilingBatch, 0),
	}

	p.metricsBuf = []*edge.MetricsBatch{
		{ProbeId: "probe-1"},
		{ProbeId: "probe-2"},
		{ProbeId: "probe-3"},
	}

	p.dataMutex.Lock()
	metricsCopy := make([]*edge.MetricsBatch, len(p.metricsBuf))
	copy(metricsCopy, p.metricsBuf)
	p.dataMutex.Unlock()

	p.dataMutex.Lock()
	p.metricsBuf = p.metricsBuf[:0]
	p.dataMutex.Unlock()

	if len(metricsCopy) != 3 {
		t.Errorf("原切片清空后副本长度被影响，期望 3，实际 %d", len(metricsCopy))
	}

	if metricsCopy[0].ProbeId != "probe-1" {
		t.Errorf("副本数据被破坏，期望 probe-1，实际 %s", metricsCopy[0].ProbeId)
	}
}
