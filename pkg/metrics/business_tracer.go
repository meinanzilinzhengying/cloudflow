// P24: 业务流程追踪器 — 提供 Span/Trace 风格的业务追踪能力
//
// 使用方式：
//   tracer := metrics.NewBusinessTracer("flow_ingest", tenantID)
//   span := tracer.Start("validation")
//   // ... 执行业务逻辑 ...
//   span.End(metrics.StatusSuccess)
//
//   span2 := tracer.Start("storage")
//   // ... 存储数据 ...
//   span2.End(metrics.StatusFailure)
//   tracer.Close()
//
package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SpanStatus 表示业务流程阶段的状态
type SpanStatus string

const (
	StatusSuccess SpanStatus = "success"
	StatusFailure SpanStatus = "failure"
	StatusSkipped SpanStatus = "skipped"
	StatusTimeout SpanStatus = "timeout"
)

// BusinessSpan 表示一个业务追踪阶段
type BusinessSpan struct {
	tracer    *BusinessTracer
	name      string
	stage     string
	startTime time.Time
	endTime   *time.Time
	status    SpanStatus
	attrs     map[string]string
}

// End 结束当前 Span 并记录指标
func (s *BusinessSpan) End(status SpanStatus) {
	if s.endTime != nil {
		return // 已结束，幂等
	}
	now := time.Now()
	s.endTime = &now
	s.status = status
	latency := now.Sub(s.startTime)

	// 记录 Prometheus 指标
	RecordPipelineStage(s.tracer.tenantID, s.tracer.pipeline, s.stage, string(status), latency)

	// 记录到 tracer 的 stages
	s.tracer.mu.Lock()
	s.tracer.stages = append(s.tracer.stages, StageRecord{
		Name:      s.stage,
		StartTime: s.startTime,
		EndTime:   now,
		Latency:   latency,
		Status:    status,
	})
	s.tracer.mu.Unlock()
}

// SetAttr 设置 Span 属性（用于调试和日志，不直接写入 Prometheus）
func (s *BusinessSpan) SetAttr(key, value string) {
	s.attrs[key] = value
}

// StageRecord 记录一个阶段的执行结果
type StageRecord struct {
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Latency   time.Duration
	Status    SpanStatus
}

// BusinessTracer 业务流程追踪器
type BusinessTracer struct {
	pipeline  string
	tenantID  string
	startTime time.Time
	endTime   *time.Time
	stages    []StageRecord
	mu        sync.Mutex
	closed    bool
}

// NewBusinessTracer 创建新的业务流程追踪器
// pipeline: 业务流程名称，如 "flow_ingest", "alert_evaluate", "query_execute"
// tenantID: 租户 ID
func NewBusinessTracer(pipeline, tenantID string) *BusinessTracer {
	if tenantID == "" {
		tenantID = "unknown"
	}
	return &BusinessTracer{
		pipeline:  pipeline,
		tenantID:  tenantID,
		startTime: time.Now(),
		stages:    make([]StageRecord, 0, 8),
	}
}

// Start 开始一个新的业务阶段
func (t *BusinessTracer) Start(stage string) *BusinessSpan {
	return &BusinessSpan{
		tracer:    t,
		name:      fmt.Sprintf("%s/%s", t.pipeline, stage),
		stage:     stage,
		startTime: time.Now(),
		attrs:     make(map[string]string),
	}
}

// Close 结束整个业务流程追踪，记录总耗时
func (t *BusinessTracer) Close(status SpanStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	now := time.Now()
	t.endTime = &now
	totalDuration := now.Sub(t.startTime)

	// 记录总耗时
	RecordBusinessOperationDuration(t.tenantID, t.pipeline, "total", totalDuration)
	RecordBusinessOperation(t.tenantID, t.pipeline, string(status))

	// 如果任一阶段失败，则整体失败
	finalStatus := status
	if status == StatusSuccess {
		for _, s := range t.stages {
			if s.Status != StatusSuccess && s.Status != StatusSkipped {
				finalStatus = StatusFailure
				break
			}
		}
	}

	// 记录整体操作结果
	RecordBusinessOperation(t.tenantID, t.pipeline, string(finalStatus))
}

// GetStages 获取所有阶段记录（用于调试/日志）
func (t *BusinessTracer) GetStages() []StageRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]StageRecord, len(t.stages))
	copy(result, t.stages)
	return result
}

// GetTotalDuration 获取整个流程总耗时
func (t *BusinessTracer) GetTotalDuration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.endTime != nil {
		return t.endTime.Sub(t.startTime)
	}
	return time.Since(t.startTime)
}

// IsClosed 检查追踪器是否已关闭
func (t *BusinessTracer) IsClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// ============================================================================
// 便捷函数：一次性追踪完整业务操作
// ============================================================================

// TraceBusinessOperation 追踪一个完整的业务操作
// 用法：
//   metrics.TraceBusinessOperation(ctx, "query_flows", func() error { ... })
func TraceBusinessOperation(ctx context.Context, operation string, fn func() error) error {
	tenantID := GetTenantID(ctx)
	tracer := NewBusinessTracer(operation, tenantID)
	defer func() {
		if r := recover(); r != nil {
			tracer.Close(StatusFailure)
			panic(r)
		}
	}()

	err := fn()
	if err != nil {
		tracer.Close(StatusFailure)
	} else {
		tracer.Close(StatusSuccess)
	}
	return err
}

// TraceBusinessOperationWithStages 追踪带阶段的业务操作
// 用法：
//   metrics.TraceBusinessOperationWithStages(ctx, "flow_ingest", func(tracer *BusinessTracer) error {
//       span := tracer.Start("validation")
//       // ... validate ...
//       span.End(metrics.StatusSuccess)
//       return nil
//   })
func TraceBusinessOperationWithStages(ctx context.Context, operation string, fn func(*BusinessTracer) error) error {
	tenantID := GetTenantID(ctx)
	tracer := NewBusinessTracer(operation, tenantID)
	defer func() {
		if r := recover(); r != nil {
			tracer.Close(StatusFailure)
			panic(r)
		}
	}()

	err := fn(tracer)
	if err != nil {
		tracer.Close(StatusFailure)
	} else {
		tracer.Close(StatusSuccess)
	}
	return err
}

// ============================================================================
// 快捷追踪函数（无需创建 tracer）
// ============================================================================

// TraceFunc 包装一个函数，自动记录其执行耗时和结果
// 适用于简单的业务操作追踪
func TraceFunc(ctx context.Context, operation, stage string, fn func() error) error {
	tenantID := GetTenantID(ctx)
	start := time.Now()
	err := fn()
	latency := time.Since(start)

	status := string(StatusSuccess)
	if err != nil {
		status = string(StatusFailure)
	}
	RecordBusinessOperationDuration(tenantID, operation, stage, latency)
	RecordBusinessOperation(tenantID, operation, status)
	return err
}

// TraceFuncWithValue 包装一个返回值的函数，自动记录执行耗时
func TraceFuncWithValue[T any](ctx context.Context, operation, stage string, fn func() (T, error)) (T, error) {
	tenantID := GetTenantID(ctx)
	start := time.Now()
	result, err := fn()
	latency := time.Since(start)

	status := string(StatusSuccess)
	if err != nil {
		status = string(StatusFailure)
	}
	RecordBusinessOperationDuration(tenantID, operation, stage, latency)
	RecordBusinessOperation(tenantID, operation, status)
	return result, err
}
