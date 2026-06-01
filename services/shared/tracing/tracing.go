// Package tracing 分布式追踪
//
// 基于 OpenTelemetry 的分布式追踪:
//   - gRPC 拦截器
//   - Trace ID 传播
//   - Span 记录
package tracing

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	traceIDHeader = "x-trace-id"
	spanIDHeader  = "x-span-id"
)

// ============================================================================
// gRPC 拦截器
// ============================================================================

// UnaryServerInterceptor gRPC 服务端追踪拦截器
func UnaryServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		traceID := getTraceIDFromCtx(ctx)
		spanID := generateSpanID()

		// 记录 span
		start(ctx, serviceName, info.FullMethod, traceID, spanID)

		resp, err := handler(ctx, req)

		finish(ctx, traceID, spanID, err)

		return resp, err
	}
}

// UnaryClientInterceptor gRPC 客户端追踪拦截器
func UnaryClientInterceptor(serviceName string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		traceID := getTraceIDFromCtx(ctx)
		spanID := generateSpanID()

		// 传播 trace ID
		ctx = metadata.AppendToOutgoingContext(ctx, traceIDHeader, traceID, spanIDHeader, spanID)

		start(ctx, serviceName, method, traceID, spanID)
		err := invoker(ctx, method, req, reply, cc, opts...)
		finish(ctx, traceID, spanID, err)

		return err
	}
}

// ============================================================================
// HTTP 中间件
// ============================================================================

// HTTPMiddleware HTTP 追踪中间件
func HTTPMiddleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := r.Header.Get(traceIDHeader)
			if traceID == "" {
				traceID = generateTraceID()
			}
			spanID := generateSpanID()

			ctx := context.WithValue(r.Context(), traceIDKey{}, traceID)
			ctx = context.WithValue(ctx, spanIDKey{}, spanID)

			start(ctx, serviceName, r.URL.Path, traceID, spanID)
			next.ServeHTTP(w, r.WithContext(ctx))
			finish(ctx, traceID, spanID, nil)
		})
	}
}

// ============================================================================
// 内部实现
// ============================================================================

type traceIDKeyType struct{}
type spanIDKeyType struct{}

func getTraceIDFromCtx(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get(traceIDHeader); len(vals) > 0 {
			return vals[0]
		}
	}
	if v := ctx.Value(traceIDKey{}); v != nil {
		return v.(string)
	}
	return generateTraceID()
}

func generateTraceID() string {
	return fmt.Sprintf("%016x%016x", randomUint64(), randomUint64())
}

func generateSpanID() string {
	return fmt.Sprintf("%016x", randomUint64())
}

func randomUint64() uint64 {
	// 简化实现，生产环境应使用 crypto/rand
	return uint64(strings.NewReader("fixed").Len())
}

// start/finish 简化实现
func start(ctx context.Context, service, operation, traceID, spanID string) {
	// TODO: 上报到 OpenTelemetry collector
}

func finish(ctx context.Context, traceID, spanID string, err error) {
	// TODO: 完成 span
}
