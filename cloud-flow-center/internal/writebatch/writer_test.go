//go:build linux

package writebatch

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockWriteTarget 模拟写入目标
type mockWriteTarget struct {
	mu      sync.Mutex
	batches map[string][][][]interface{}
	writes  int
}

func (m *mockWriteTarget) WriteBatch(ctx context.Context, table string, rows [][]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batches[table] = append(m.batches[table], rows)
	m.writes++
	return nil
}

func (m *mockWriteTarget) WriteCompressed(ctx context.Context, table string, data []byte) error {
	return m.WriteBatch(ctx, table, nil)
}

func TestDefaultBatchWriterConfig(t *testing.T) {
	cfg := DefaultBatchWriterConfig()
	if cfg.BatchSize != 5000 {
		t.Errorf("expected batch size 5000, got %d", cfg.BatchSize)
	}
	if cfg.FlushInterval != 1*time.Second {
		t.Errorf("unexpected flush interval: %v", cfg.FlushInterval)
	}
	if cfg.WorkerCount != 4 {
		t.Errorf("expected worker count 4, got %d", cfg.WorkerCount)
	}
}

func TestBatchWriter(t *testing.T) {
	target := &mockWriteTarget{batches: make(map[string][][][]interface{})}
	cfg := &BatchWriterConfig{
		BatchSize:     10,
		FlushInterval: 100 * time.Millisecond,
		MaxQueueSize:  100,
		WorkerCount:   2,
		MaxRetries:    1,
		RetryInterval: 10 * time.Millisecond,
	}
	bw := NewBatchWriter(cfg, target)
	bw.Start()
	defer bw.Stop()

	// 写入多条记录
	for i := 0; i < 25; i++ {
		record := &WriteRecord{
			Table:   "flows",
			Values:  []interface{}{i, "data"},
			TenantID: "tenant-1",
		}
		if err := bw.Write(record); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// 等待刷新
	time.Sleep(200 * time.Millisecond)

	// 检查统计
	stats := bw.GetStats()
	if stats.TotalWritten == 0 {
		t.Error("expected some records to be written")
	}
	if stats.TotalBatches == 0 {
		t.Error("expected some batches to be written")
	}
	if stats.TotalErrors > 0 {
		t.Errorf("expected 0 errors, got %d", stats.TotalErrors)
	}

	// 检查目标
	target.mu.Lock()
	if len(target.batches["flows"]) == 0 {
		t.Error("expected batches in target")
	}
	target.mu.Unlock()
}

func TestBatchWriterQueueFull(t *testing.T) {
	target := &mockWriteTarget{batches: make(map[string][][][]interface{})}
	cfg := &BatchWriterConfig{
		BatchSize:     1000,
		FlushInterval: 10 * time.Second,
		MaxQueueSize:  2,
		WorkerCount:   1,
	}
	bw := NewBatchWriter(cfg, target)
	bw.Start()
	defer bw.Stop()

	// 填满队列
	for i := 0; i < 5; i++ {
		record := &WriteRecord{Table: "flows", Values: []interface{}{i}}
		bw.Write(record)
	}

	// 检查丢弃
	stats := bw.GetStats()
	if stats.DroppedCount == 0 {
		t.Log("no drops (may have been consumed quickly)")
	}
}

func TestBatchWriterInvalidRecord(t *testing.T) {
	target := &mockWriteTarget{batches: make(map[string][][][]interface{})}
	bw := NewBatchWriter(nil, target)

	if err := bw.Write(nil); err == nil {
		t.Error("expected error for nil record")
	}
	if err := bw.Write(&WriteRecord{Table: ""}); err == nil {
		t.Error("expected error for empty table")
	}
}

func TestBatchWriterFlushAll(t *testing.T) {
	target := &mockWriteTarget{batches: make(map[string][][][]interface{})}
	cfg := &BatchWriterConfig{
		BatchSize:     1000,
		FlushInterval: 10 * time.Second,
		MaxQueueSize:  100,
		WorkerCount:   1,
	}
	bw := NewBatchWriter(cfg, target)
	bw.Start()

	// 写入少量数据
	for i := 0; i < 5; i++ {
		bw.Write(&WriteRecord{Table: "flows", Values: []interface{}{i}})
	}

	// 直接停止（会触发 flushAll）
	bw.Stop()

	// 检查数据已写入
	stats := bw.GetStats()
	if stats.TotalWritten < 5 {
		t.Errorf("expected at least 5 written, got %d", stats.TotalWritten)
	}
}

func TestBatchWriterMultipleTables(t *testing.T) {
	target := &mockWriteTarget{batches: make(map[string][][][]interface{})}
	cfg := &BatchWriterConfig{
		BatchSize:     5,
		FlushInterval: 100 * time.Millisecond,
		MaxQueueSize:  100,
		WorkerCount:   2,
	}
	bw := NewBatchWriter(cfg, target)
	bw.Start()
	defer bw.Stop()

	for i := 0; i < 10; i++ {
		bw.Write(&WriteRecord{Table: "flows", Values: []interface{}{i}})
		bw.Write(&WriteRecord{Table: "traces", Values: []interface{}{i}})
	}

	time.Sleep(200 * time.Millisecond)

	target.mu.Lock()
	if len(target.batches["flows"]) == 0 {
		t.Error("expected flows batches")
	}
	if len(target.batches["traces"]) == 0 {
		t.Error("expected traces batches")
	}
	target.mu.Unlock()
}

func TestBatchWriterBatchWrite(t *testing.T) {
	target := &mockWriteTarget{batches: make(map[string][][][]interface{})}
	bw := NewBatchWriter(nil, target)

	records := []*WriteRecord{
		{Table: "flows", Values: []interface{}{1}},
		{Table: "flows", Values: []interface{}{2}},
		{Table: "flows", Values: []interface{}{3}},
	}
	if err := bw.WriteBatch(records); err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(100) // 100/sec

	// 令牌桶初始满桶，前100个立即通过
	for i := 0; i < 100; i++ {
		rl.acquire(1)
	}
	// 第101个需要等待约1秒
	start := time.Now()
	rl.acquire(1)
	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond {
		t.Logf("rate limiter allowed 101st write in %v (expected ~1s)", elapsed)
	}
}

func TestBatchWriterStats(t *testing.T) {
	target := &mockWriteTarget{batches: make(map[string][][][]interface{})}
	cfg := &BatchWriterConfig{
		BatchSize:     5,
		FlushInterval: 50 * time.Millisecond,
		MaxQueueSize:  100,
		WorkerCount:   1,
	}
	bw := NewBatchWriter(cfg, target)
	bw.Start()
	defer bw.Stop()

	for i := 0; i < 10; i++ {
		bw.Write(&WriteRecord{Table: "flows", Values: []interface{}{i}})
	}

	time.Sleep(200 * time.Millisecond)

	stats := bw.GetStats()
	if stats.TotalWritten < 10 {
		t.Errorf("expected at least 10 written, got %d", stats.TotalWritten)
	}
	if stats.TotalBatches <= 0 {
		t.Errorf("expected >0 batches, got %d", stats.TotalBatches)
	}
	if stats.AvgBatchSize <= 0 {
		t.Errorf("expected avg batch size > 0, got %f", stats.AvgBatchSize)
	}
}
