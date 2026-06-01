// Package middleware 提供全链路 traceId 透传、请求日志和审计日志中间件
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"cloudflow/services/shared/logger"
)

// ==============================
// gRPC 中间件
// ==============================

const (
	GrpcTraceIDKey = "x-trace-id"
	GrpcUserIDKey  = "x-user-id"
	GrpcTenantIDKey= "x-tenant-id"
)

// GrpcUnaryServerInterceptor gRPC 一元服务拦截器 - 处理 traceId 透传和请求日志
func GrpcUnaryServerInterceptor(l *logger.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// 从 incoming metadata 中提取 traceId，不存在则生成
		ctx = extractAndSetTraceID(ctx)

		// 提取用户和租户信息
		ctx = extractAndSetUserTenant(ctx)

		// 注入 logger 到 context
		logCtx := l.WithContext(ctx)

		// 记录请求开始
		logCtx.Info("grpc_request_start",
			"method", info.FullMethod,
			"request", req,
		)

		// 调用 handler
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		// 记录响应
		if err != nil {
			logCtx.ErrorWithStack(err, "grpc_request_failed",
				"method", info.FullMethod,
				"duration_ms", duration.Milliseconds(),
			)
		} else {
			logCtx.Info("grpc_request_complete",
				"method", info.FullMethod,
				"duration_ms", duration.Milliseconds(),
			)
		}

		return resp, err
	}
}

// GrpcStreamServerInterceptor gRPC 流服务拦截器
func GrpcStreamServerInterceptor(l *logger.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		// 提取 traceId
		ctx := extractAndSetTraceID(ss.Context())
		ctx = extractAndSetUserTenant(ctx)

		// 注入 logger
		logCtx := l.WithContext(ctx)

		logCtx.Info("grpc_stream_request_start",
			"method", info.FullMethod,
			"is_client_stream", info.IsClientStream,
			"is_server_stream", info.IsServerStream,
		)

		// 包装 stream
		wrapped := &wrappedStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		err := handler(srv, wrapped)
		duration := time.Since(start)

		if err != nil {
			logCtx.ErrorWithStack(err, "grpc_stream_request_failed",
				"method", info.FullMethod,
				"duration_ms", duration.Milliseconds(),
			)
		} else {
			logCtx.Info("grpc_stream_request_complete",
				"method", info.FullMethod,
				"duration_ms", duration.Milliseconds(),
			)
		}

		return err
	}
}

// GrpcUnaryClientInterceptor gRPC 客户端拦截器 - 传递 traceId
func GrpcUnaryClientInterceptor(l *logger.Logger) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()

		// 确保有 traceId
		ctx = ensureTraceID(ctx)

		// 将 traceId 注入 outgoing metadata
		ctx = injectTraceIDToMetadata(ctx)

		logCtx := l.WithContext(ctx)
		logCtx.Debug("grpc_client_request_start",
			"method", method,
		)

		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		if err != nil {
			logCtx.ErrorWithStack(err, "grpc_client_request_failed",
				"method", method,
				"duration_ms", duration.Milliseconds(),
			)
		} else {
			logCtx.Debug("grpc_client_request_complete",
				"method", method,
				"duration_ms", duration.Milliseconds(),
			)
		}

		return err
	}
}

// ==============================
// HTTP 中间件
// ==============================

const (
	HttpTraceIDHeader = "X-Trace-ID"
	HttpUserIDHeader  = "X-User-ID"
	HttpTenantIDHeader= "X-Tenant-ID"
)

// HTTPMiddleware HTTP 中间件
type HTTPMiddleware struct {
	logger *logger.Logger
}

func NewHTTPMiddleware(l *logger.Logger) *HTTPMiddleware {
	return &HTTPMiddleware{logger: l}
}

func (m *HTTPMiddleware) TraceIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 从请求头获取或生成 traceId
		ctx := r.Context()
		traceID := r.Header.Get(HttpTraceIDHeader)
		if traceID == "" {
			traceID = logger.TraceIDFromContext(ctx)
			if traceID == "" {
				ctx = logger.WithTraceID(ctx)
				traceID = logger.TraceIDFromContext(ctx)
			}
		} else {
			ctx = logger.WithTraceIDValue(ctx, traceID)
		}

		// 提取用户和租户信息
		userID := r.Header.Get(HttpUserIDHeader)
		if userID != "" {
			ctx = logger.WithUserID(ctx, userID)
		}
		tenantID := r.Header.Get(HttpTenantIDHeader)
		if tenantID != "" {
			ctx = logger.WithTenantID(ctx, tenantID)
		}

		// 注入 traceId 到响应头
		w.Header().Set(HttpTraceIDHeader, traceID)

		// 创建带上下文的 logger
		logCtx := m.logger.WithContext(ctx)

		// 记录请求
		logCtx.Info("http_request_start",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)

		// 包装 ResponseWriter 以捕获状态码
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// 调用下一个处理器
		next.ServeHTTP(rw, r.WithContext(ctx))

		duration := time.Since(start)

		// 记录响应
		logCtx.Info("http_request_complete",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// AuditMiddleware 审计中间件 - 记录关键操作
func (m *HTTPMiddleware) AuditMiddleware(resource string, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			start := time.Now()

			// 包装 ResponseWriter
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// 调用下一个处理器
			next.ServeHTTP(rw, r.WithContext(ctx))

			duration := time.Since(start)

			// 记录审计日志
			result := "success"
			if rw.statusCode >= 400 {
				result = "failed"
			}

			m.logger.AuditWithDuration(
				ctx,
				logger.AuditTypeAccess,
				resource,
				action,
				result,
				duration,
				map[string]interface{}{
					"method": r.Method,
					"path": r.URL.Path,
					"status": rw.statusCode,
					"ip": r.RemoteAddr,
					"user_agent": r.UserAgent(),
				},
			)
		})
	}
}

// ==============================
// 辅助类型和函数
// ==============================

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func extractAndSetTraceID(ctx context.Context) context.Context {
	// 从 incoming metadata 中获取 traceId
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if traceIDs := md.Get(GrpcTraceIDKey); len(traceIDs) > 0 && traceIDs[0] != "" {
			return logger.WithTraceIDValue(ctx, traceIDs[0])
		}
	}

	// 检查 context 中是否已经有 traceId
	if logger.TraceIDFromContext(ctx) != "" {
		return ctx
	}

	// 生成新的 traceId
	return logger.WithTraceID(ctx)
}

func extractAndSetUserTenant(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if userIDs := md.Get(GrpcUserIDKey); len(userIDs) > 0 && userIDs[0] != "" {
			ctx = logger.WithUserID(ctx, userIDs[0])
		}
		if tenantIDs := md.Get(GrpcTenantIDKey); len(tenantIDs) > 0 && tenantIDs[0] != "" {
			ctx = logger.WithTenantID(ctx, tenantIDs[0])
		}
	}
	return ctx
}

func ensureTraceID(ctx context.Context) context.Context {
	if logger.TraceIDFromContext(ctx) == "" {
		return logger.WithTraceID(ctx)
	}
	return ctx
}

func injectTraceIDToMetadata(ctx context.Context) context.Context {
	traceID := logger.TraceIDFromContext(ctx)
	if traceID == "" {
		return ctx
	}

	// 注入到 outgoing metadata
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.Pairs(GrpcTraceIDKey, traceID)
	} else {
		md = md.Copy()
		md.Set(GrpcTraceIDKey, traceID)
	}

	// 注入用户和租户信息
	if userID := logger.UserIDFromContext(ctx); userID != "" {
		md.Set(GrpcUserIDKey, userID)
	}
	if tenantID := logger.TenantIDFromContext(ctx); tenantID != "" {
		md.Set(GrpcTenantIDKey, tenantID)
	}

	return metadata.NewOutgoingContext(ctx, md)
}

// ==============================
// 辅助函数 - 轻松注入中间件到 server
// ==============================

type ServerOptions struct {
	UnaryInterceptors  []grpc.UnaryServerInterceptor
	StreamInterceptors []grpc.StreamServerInterceptor
}

func WithLogger(l *logger.Logger) grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(GrpcUnaryServerInterceptor(l))
}

func WithStreamLogger(l *logger.Logger) grpc.ServerOption {
	return grpc.ChainStreamInterceptor(GrpcStreamServerInterceptor(l))
}

func WithClientLogger(l *logger.Logger) grpc.DialOption {
	return grpc.WithUnaryInterceptor(GrpcUnaryClientInterceptor(l))
}

// SetHTTPHeader 设置 traceId 到 HTTP header（用于客户端调用）
func SetHTTPHeader(req *http.Request, ctx context.Context) *http.Request {
	if traceID := logger.TraceIDFromContext(ctx); traceID != "" {
		req.Header.Set(HttpTraceIDHeader, traceID)
	}
	if userID := logger.UserIDFromContext(ctx); userID != "" {
		req.Header.Set(HttpUserIDHeader, userID)
	}
	if tenantID := logger.TenantIDFromContext(ctx); tenantID != "" {
		req.Header.Set(HttpTenantIDHeader, tenantID)
	}
	return req
}

// AuditFunc 审计函数 - 记录关键操作执行
func AuditFunc(ctx context.Context, l *logger.Logger, eventType, resource, action string, fn func() error) error {
	start := time.Now()

	l.Debug("audit_operation_start",
		"event_type", eventType,
		"resource", resource,
		"action", action,
	)

	err := fn()
	duration := time.Since(start)

	result := "success"
	if err != nil {
		result = "failed"
		l.ErrorWithStack(err, "audit_operation_failed",
			"event_type", eventType,
			"resource", resource,
			"action", action,
			"duration_ms", duration.Milliseconds(),
		)
	} else {
		l.Info("audit_operation_success",
			"event_type", eventType,
			"resource", resource,
			"action", action,
			"duration_ms", duration.Milliseconds(),
		)
	}

	l.AuditWithDuration(ctx, eventType, resource, action, result, duration, nil)

	return err
}

// AuditFuncWithDetails 带详情的审计
func AuditFuncWithDetails(ctx context.Context, l *logger.Logger, eventType, resource, action string, fn func() (map[string]interface{}, error)) error {
	start := time.Now()

	l.Debug("audit_operation_start",
		"event_type", eventType,
		"resource", resource,
		"action", action,
	)

	details, err := fn()
	duration := time.Since(start)

	result := "success"
	if err != nil {
		result = "failed"
		l.ErrorWithStack(err, "audit_operation_failed",
			"event_type", eventType,
			"resource", resource,
			"action", action,
			"duration_ms", duration.Milliseconds(),
		)
	} else {
		l.Info("audit_operation_success",
			"event_type", eventType,
			"resource", resource,
			"action", action,
			"duration_ms", duration.Milliseconds(),
		)
	}

	l.AuditWithDuration(ctx, eventType, resource, action, result, duration, details)

	return err
}
