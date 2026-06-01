package trace

import (
	"context"

	"github.com/google/uuid"
)

// TraceIDMDKey gRPC metadata 中 Trace ID 的 key
const TraceIDMDKey = "x-trace-id"

type traceIDKey struct{}

// TraceID 生成新的 Trace ID
func TraceID() string {
	return uuid.New().String()
}

// WithTraceID 将 Trace ID 注入 context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// FromContext 从 context 中提取 Trace ID
func FromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}
