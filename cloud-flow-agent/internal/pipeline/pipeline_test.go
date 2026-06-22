//go:build linux

package pipeline_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/pipeline"
)

// ============================================================================
// 一、数据记录测试
// ============================================================================

func TestDataRecord(t *testing.T) {
	record := &pipeline.DataRecord{
		ID:        "rec-1",
		Type:      pipeline.DataTypeFlow,
		Timestamp: time.Now().UnixNano(),
		ProbeID:   "probe-1",
		TenantID:  "tenant-1",
		Payload:   []byte("test data"),
		Meta: &pipeline.DataMeta{
			SrcIP:     "10.0.0.1",
			DstIP:     "10.0.0.2",
			SrcPort:   12345,
			DstPort:   80,
			Protocol:  6,
			Namespace: "default",
			Service:   "web",
		},
	}

	if record.Size() <= 0 {
		t.Error("expected non-zero size")
	}
	if record.String() == "" {
		t.Error("expected non-empty string")
	}
}

// ============================================================================
// 二、数据批次测试
// ============================================================================

func TestDataBatch(t *testing.T) {
	batch := pipeline.NewDataBatch(pipeline.DataTypeFlow)

	if !batch.IsEmpty() {
		t.Error("expected empty batch")
	}

	for i := 0; i < 5; i++ {
		batch.Add(&pipeline.DataRecord{
			ID:   fmt.Sprintf("rec-%d", i),
			Type: pipeline.DataTypeFlow,
		})
	}

	if batch.IsEmpty() {
		t.Error("expected non-empty batch")
	}
	if batch.RecordCount != 5 {
		t.Errorf("expected 5 records, got %d", batch.RecordCount)
	}
	if len(batch.ValidRecords()) != 5 {
		t.Errorf("expected 5 valid records, got %d", len(batch.ValidRecords()))
	}

	// 添加被丢弃的记录
	batch.Add(&pipeline.DataRecord{ID: "dropped", Dropped: true})
	if batch.DroppedCount() != 1 {
		t.Errorf("expected 1 dropped, got %d", batch.DroppedCount())
	}
	if len(batch.ValidRecords()) != 5 {
		t.Errorf("expected 5 valid after drop, got %d", len(batch.ValidRecords()))
	}
}

// ============================================================================
// 三、配置测试
// ============================================================================

func TestPipelineConfig(t *testing.T) {
	cfg := pipeline.DefaultPipelineConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if cfg.BatchSize != 1000 {
		t.Errorf("expected batch_size=1000, got %d", cfg.BatchSize)
	}

	// 无效配置
	cfg2 := &pipeline.PipelineConfig{BatchSize: 0}
	if err := cfg2.Validate(); err == nil {
		t.Error("expected error for batch_size=0")
	}

	cfg3 := &pipeline.PipelineConfig{BatchSize: 100, SampleRate: 2.0}
	if err := cfg3.Validate(); err == nil {
		t.Error("expected error for sample_rate > 1")
	}
}

// ============================================================================
// 四、过滤预处理器测试
// ============================================================================

func TestFilterPreprocessor(t *testing.T) {
	rules := []*pipeline.FilterRule{
		{Field: "namespace", Operator: "eq", Value: "kube-system"},
		{Field: "src_ip", Operator: "contains", Value: "127.0.0"},
	}
	fp := pipeline.NewFilterPreprocessor(rules)

	if fp.Name() != "filter" {
		t.Errorf("expected name='filter', got %s", fp.Name())
	}
	if fp.Stage() != pipeline.StageFilter {
		t.Errorf("expected stage=filter, got %s", fp.Stage())
	}

	// 匹配规则，应被丢弃
	record1 := &pipeline.DataRecord{
		Meta: &pipeline.DataMeta{Namespace: "kube-system"},
	}
	if err := fp.Process(record1); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !record1.Dropped {
		t.Error("expected record to be dropped")
	}

	// 不匹配，应保留
	record2 := &pipeline.DataRecord{
		Meta: &pipeline.DataMeta{Namespace: "default"},
	}
	if err := fp.Process(record2); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if record2.Dropped {
		t.Error("expected record to be kept")
	}

	// 空元数据，应丢弃
	record3 := &pipeline.DataRecord{Meta: nil}
	if err := fp.Process(record3); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !record3.Dropped {
		t.Error("expected empty meta record to be dropped")
	}
}

func TestFilterRuleMatch(t *testing.T) {
	rule := &pipeline.FilterRule{Field: "service", Operator: "eq", Value: "test-svc"}
	record := &pipeline.DataRecord{Meta: &pipeline.DataMeta{Service: "test-svc"}}
	if !rule.Match(record) {
		t.Error("expected match")
	}

	record2 := &pipeline.DataRecord{Meta: &pipeline.DataMeta{Service: "other-svc"}}
	if rule.Match(record2) {
		t.Error("expected no match")
	}
}

// ============================================================================
// 五、采样预处理器测试
// ============================================================================

func TestSamplePreprocessor(t *testing.T) {
	t.Run("Random sample full", func(t *testing.T) {
		sp := pipeline.NewSamplePreprocessor(1.0, pipeline.SampleStrategyRandom)
		record := &pipeline.DataRecord{ID: "test"}
		if err := sp.Process(record); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if record.Dropped {
			t.Error("expected full rate to keep all")
		}
	})

	t.Run("Random sample zero", func(t *testing.T) {
		sp := pipeline.NewSamplePreprocessor(0.0, pipeline.SampleStrategyRandom)
		record := &pipeline.DataRecord{ID: "test"}
		if err := sp.Process(record); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !record.Dropped {
			t.Error("expected zero rate to drop all")
		}
	})

	t.Run("Consistent sample", func(t *testing.T) {
		sp := pipeline.NewSamplePreprocessor(0.5, pipeline.SampleStrategyConsistent)
		record1 := &pipeline.DataRecord{ID: "same-id"}
		record2 := &pipeline.DataRecord{ID: "same-id"}
		sp.Process(record1)
		sp.Process(record2)
		// 相同 ID 应该有相同结果
		if record1.Dropped != record2.Dropped {
			t.Error("expected consistent result for same ID")
		}
	})

	t.Run("Count sample", func(t *testing.T) {
		sp := pipeline.NewSamplePreprocessor(0.25, pipeline.SampleStrategyCount) // 每4取1
		kept := 0
		for i := 0; i < 100; i++ {
			r := &pipeline.DataRecord{ID: fmt.Sprintf("rec-%d", i)}
			sp.Process(r)
			if !r.Dropped {
				kept++
			}
		}
		if kept < 20 || kept > 30 {
			t.Errorf("expected ~25 kept, got %d", kept)
		}
	})
}

// ============================================================================
// 六、富化预处理器测试
// ============================================================================

func TestEnrichPreprocessor(t *testing.T) {
	ep := pipeline.NewEnrichPreprocessor()
	ep.AddEnricher(pipeline.AddTimeEnricher())
	ep.AddEnricher(pipeline.AddGeoEnricher())

	record := &pipeline.DataRecord{
		ID:   "test",
		Meta: &pipeline.DataMeta{SrcIP: "10.0.0.1"},
	}
	if err := ep.Process(record); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !record.Processed {
		t.Error("expected record to be marked processed")
	}
	if record.Meta.Tags == nil {
		t.Fatal("expected tags to be set")
	}
	if record.Meta.Tags["hour"] == "" {
		t.Error("expected hour tag")
	}
	if record.Meta.Tags["src_geo"] != "private" {
		t.Errorf("expected private geo, got %s", record.Meta.Tags["src_geo"])
	}
}

func TestGeoEnricherPublic(t *testing.T) {
	ep := pipeline.NewEnrichPreprocessor()
	ep.AddEnricher(pipeline.AddGeoEnricher())

	record := &pipeline.DataRecord{
		Meta: &pipeline.DataMeta{SrcIP: "8.8.8.8"},
	}
	ep.Process(record)
	if record.Meta.Tags["src_geo"] != "public" {
		t.Errorf("expected public geo, got %s", record.Meta.Tags["src_geo"])
	}
}

// ============================================================================
// 七、预处理管道测试
// ============================================================================

func TestPreprocessPipeline(t *testing.T) {
	pp := pipeline.NewPreprocessPipeline()

	fp := pipeline.NewFilterPreprocessor([]*pipeline.FilterRule{
		{Field: "namespace", Operator: "eq", Value: "drop-me"},
	})
	sp := pipeline.NewSamplePreprocessor(1.0, pipeline.SampleStrategyRandom)

	pp.Add(fp)
	pp.Add(sp)

	if stats := pp.GetStats(); stats["processor_count"] != 2 {
		t.Errorf("expected 2 processors, got %v", stats["processor_count"])
	}

	// 被过滤
	record1 := &pipeline.DataRecord{Meta: &pipeline.DataMeta{Namespace: "drop-me"}}
	if err := pp.Process(record1); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !record1.Dropped {
		t.Error("expected filtered record to be dropped")
	}

	// 通过
	record2 := &pipeline.DataRecord{Meta: &pipeline.DataMeta{Namespace: "default"}}
	if err := pp.Process(record2); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if record2.Dropped {
		t.Error("expected record to pass")
	}

	// 批次处理
	batch := pipeline.NewDataBatch(pipeline.DataTypeFlow)
	batch.Add(&pipeline.DataRecord{Meta: &pipeline.DataMeta{Namespace: "drop-me"}})
	batch.Add(&pipeline.DataRecord{Meta: &pipeline.DataMeta{Namespace: "default"}})
	if err := pp.ProcessBatch(batch); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(batch.ValidRecords()) != 1 {
		t.Errorf("expected 1 valid record, got %d", len(batch.ValidRecords()))
	}
}

// ============================================================================
// 八、管道管理器测试
// ============================================================================

func TestPipelineManager(t *testing.T) {
	cfg := &pipeline.PipelineConfig{
		BatchSize:    3,
		BatchTimeout: 100 * time.Millisecond,
		MaxBatchSize: 1024 * 1024,
		BufferSize:   100,
		OutputBuffer: 100,
		WorkerCount:  2,
		Format:       pipeline.FormatProtobuf,
	}

	pm, err := pipeline.NewPipelineManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 添加测试写入器
	writer := pipeline.NewMockWriter()
	uw := pipeline.NewUnifiedWriter(pm)
	uw.AddWriter(writer)

	if err := pm.Start(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	uw.Start()

	// 提交记录
	for i := 0; i < 10; i++ {
		record := &pipeline.DataRecord{
			ID:        fmt.Sprintf("rec-%d", i),
			Type:      pipeline.DataTypeFlow,
			Timestamp: time.Now().UnixNano(),
			Meta: &pipeline.DataMeta{
				Namespace: "default",
				Service:   fmt.Sprintf("svc-%d", i%3),
			},
		}
		if err := pm.Submit(record); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}

	// 等待批次超时
	time.Sleep(200 * time.Millisecond)

	stats := pm.GetStats()
	if stats.RecordsReceived != 10 {
		t.Errorf("expected 10 received, got %d", stats.RecordsReceived)
	}

	uw.Stop()
	pm.Stop()

	// 验证写入器
	if writer.GetRecordCount() < 5 {
		t.Errorf("expected at least 5 records written, got %d", writer.GetRecordCount())
	}
}

func TestPipelineManagerWithFilter(t *testing.T) {
	cfg := &pipeline.PipelineConfig{
		BatchSize:    5,
		BatchTimeout: 200 * time.Millisecond,
		BufferSize:   100,
		OutputBuffer: 100,
		WorkerCount:  1,
		EnableFilter: true,
		Format:       pipeline.FormatProtobuf,
	}

	pm, err := pipeline.NewPipelineManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 添加过滤规则
	pm.AddPreprocessor(pipeline.NewFilterPreprocessor([]*pipeline.FilterRule{
		{Field: "namespace", Operator: "eq", Value: "kube-system"},
	}))

	writer := pipeline.NewMockWriter()
	uw := pipeline.NewUnifiedWriter(pm)
	uw.AddWriter(writer)

	pm.Start()
	uw.Start()

	// 提交混合记录
	for i := 0; i < 10; i++ {
		ns := "default"
		if i%2 == 0 {
			ns = "kube-system"
		}
		pm.Submit(&pipeline.DataRecord{
			ID:   fmt.Sprintf("rec-%d", i),
			Meta: &pipeline.DataMeta{Namespace: ns},
		})
	}

	time.Sleep(300 * time.Millisecond)

	uw.Stop()
	pm.Stop()

	stats := pm.GetStats()
	if stats.RecordsDropped == 0 {
		t.Error("expected some records to be dropped")
	}
	if stats.RecordsPassed == 0 {
		t.Error("expected some records to pass")
	}
}

func TestPipelineManagerWithSample(t *testing.T) {
	cfg := &pipeline.PipelineConfig{
		BatchSize:    100,
		BatchTimeout: 200 * time.Millisecond,
		BufferSize:   1000,
		OutputBuffer: 1000,
		WorkerCount:  1,
		EnableSample: true,
		SampleRate:   0.5,
		Format:       pipeline.FormatProtobuf,
	}

	pm, err := pipeline.NewPipelineManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	writer := pipeline.NewMockWriter()
	uw := pipeline.NewUnifiedWriter(pm)
	uw.AddWriter(writer)

	pm.Start()
	uw.Start()

	// 提交大量记录
	for i := 0; i < 100; i++ {
		pm.Submit(&pipeline.DataRecord{
			ID:   fmt.Sprintf("rec-%d", i),
			Meta: &pipeline.DataMeta{Namespace: "default"},
		})
	}

	time.Sleep(300 * time.Millisecond)

	uw.Stop()
	pm.Stop()

	stats := pm.GetStats()
	if stats.RecordsReceived != 100 {
		t.Errorf("expected 100 received, got %d", stats.RecordsReceived)
	}
	if stats.RecordsPassed == 0 {
		t.Error("expected some records to pass")
	}
	if stats.RecordsPassed == 100 {
		t.Error("expected some records to be dropped with 50% sample rate")
	}
}

func TestPipelineManagerBufferFull(t *testing.T) {
	cfg := &pipeline.PipelineConfig{
		BatchSize:    1000,
		BatchTimeout: 10 * time.Second, // 很长，不会自动 flush
		BufferSize:   2,
		OutputBuffer: 2,
		WorkerCount:  1,
		Format:       pipeline.FormatProtobuf,
	}

	pm, err := pipeline.NewPipelineManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pm.Start()

	// 填满缓冲区
	pm.Submit(&pipeline.DataRecord{ID: "1"})
	pm.Submit(&pipeline.DataRecord{ID: "2"})

	// 第三个应该失败
	if err := pm.Submit(&pipeline.DataRecord{ID: "3"}); err == nil {
		t.Error("expected error for full buffer")
	}

	pm.Stop()
}

func TestPipelineManagerStats(t *testing.T) {
	cfg := &pipeline.PipelineConfig{
		BatchSize:    5,
		BatchTimeout: 100 * time.Millisecond,
		BufferSize:   100,
		OutputBuffer: 100,
		WorkerCount:  1,
		Format:       pipeline.FormatProtobuf,
	}

	pm, err := pipeline.NewPipelineManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pm.Start()
	pm.Submit(&pipeline.DataRecord{ID: "1"})
	pm.Stop()

	stats := pm.GetStats()
	if stats.RecordsReceived != 1 {
		t.Errorf("expected 1 received, got %d", stats.RecordsReceived)
	}

	pm.ResetStats()
	stats = pm.GetStats()
	if stats.RecordsReceived != 0 {
		t.Errorf("expected 0 after reset, got %d", stats.RecordsReceived)
	}
}

// ============================================================================
// 九、模拟写入器测试
// ============================================================================

func TestMockWriter(t *testing.T) {
	mw := pipeline.NewMockWriter()

	batch1 := pipeline.NewDataBatch(pipeline.DataTypeFlow)
	batch1.Add(&pipeline.DataRecord{ID: "1"})
	batch1.Add(&pipeline.DataRecord{ID: "2"})

	if err := mw.Write(batch1); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mw.GetRecordCount() != 2 {
		t.Errorf("expected 2 records, got %d", mw.GetRecordCount())
	}

	if len(mw.GetBatches()) != 1 {
		t.Errorf("expected 1 batch, got %d", len(mw.GetBatches()))
	}

	mw.Close()
	if err := mw.Write(batch1); err == nil {
		t.Error("expected error for closed writer")
	}
}

// ============================================================================
// 十、统一写入器测试
// ============================================================================

func TestUnifiedWriter(t *testing.T) {
	cfg := &pipeline.PipelineConfig{
		BatchSize:    3,
		BatchTimeout: 100 * time.Millisecond,
		BufferSize:   100,
		OutputBuffer: 100,
		WorkerCount:  1,
	}

	pm, _ := pipeline.NewPipelineManager(cfg)
	writer1 := pipeline.NewMockWriter()
	writer2 := pipeline.NewMockWriter()

	uw := pipeline.NewUnifiedWriter(pm)
	uw.AddWriter(writer1)
	uw.AddWriter(writer2)

	pm.Start()
	uw.Start()

	pm.Submit(&pipeline.DataRecord{ID: "1"})
	pm.Submit(&pipeline.DataRecord{ID: "2"})
	pm.Submit(&pipeline.DataRecord{ID: "3"})

	time.Sleep(200 * time.Millisecond)

	uw.Stop()
	pm.Stop()

	// 两个写入器都应该收到数据
	if writer1.GetRecordCount() == 0 {
		t.Error("expected writer1 to receive data")
	}
	if writer2.GetRecordCount() == 0 {
		t.Error("expected writer2 to receive data")
	}
	if writer1.GetRecordCount() != writer2.GetRecordCount() {
		t.Errorf("expected same count: w1=%d, w2=%d", writer1.GetRecordCount(), writer2.GetRecordCount())
	}
}

// ============================================================================
// 辅助函数
// ============================================================================
