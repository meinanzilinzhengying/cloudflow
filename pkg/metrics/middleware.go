// P24: 业务指标中间件 — 在 HTTP/gRPC 请求链中自动注入业务埋点
//
// 功能：
//   - 从请求中提取 tenant_id / user_id 并注入 context
//   - 自动记录租户级别的 API 调用指标
//   - 自动记录用户行为指标
//
package metrics

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ============================================================================
// HTTP 中间件
// ============================================================================

// BusinessHTTPMiddleware 返回业务指标 HTTP 中间件
// 自动从 HTTP Header 中提取 tenant_id 和 user_id，并记录租户 API 调用
func BusinessHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 从 Header 提取租户 ID 和用户 ID
		tenantID := r.Header.Get("X-Tenant-ID")
		userID := r.Header.Get("X-User-ID")
		if tenantID == "" {
			// 尝试从 URL 参数提取（用于某些前端调用场景）
			tenantID = r.URL.Query().Get("tenant_id")
		}
		if userID == "" {
			userID = r.URL.Query().Get("user_id")
		}

		// 注入 context
		ctx := r.Context()
		if tenantID != "" {
			ctx = WithTenantID(ctx, tenantID)
		}
		if userID != "" {
			ctx = WithUserID(ctx, userID)
		}
		r = r.WithContext(ctx)

		// 记录租户 API 调用（规范化 endpoint 避免高基数）
		endpoint := normalizeBusinessEndpoint(r.URL.Path)
		if tenantID != "" {
			RecordTenantAPICall(tenantID, r.Method, endpoint)
		}

		// 记录用户行为（将操作归类）
		if userID != "" && tenantID != "" {
			actionType := classifyAction(r.Method, endpoint)
			RecordUserAction(tenantID, userID, actionType)
		}

		next.ServeHTTP(w, r)
	})
}

// BusinessHTTPMiddlewareFunc 包装 http.HandlerFunc
func BusinessHTTPMiddlewareFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		BusinessHTTPMiddleware(next).ServeHTTP(w, r)
	}
}

// ============================================================================
// gRPC 拦截器
// ============================================================================

// BusinessGRPCUnaryInterceptor 返回 gRPC unary 业务指标拦截器
// 从 gRPC metadata 中提取 tenant_id 和 user_id
func BusinessGRPCUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 从 metadata 提取租户 ID 和用户 ID
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			if vals := md.Get("x-tenant-id"); len(vals) > 0 && vals[0] != "" {
				ctx = WithTenantID(ctx, vals[0])
			}
			if vals := md.Get("x-user-id"); len(vals) > 0 && vals[0] != "" {
				ctx = WithUserID(ctx, vals[0])
			}
		}

		// 记录租户 API 调用（gRPC method 作为 endpoint）
		if tenantID := GetTenantID(ctx); tenantID != "" {
			method := extractMethodName(info.FullMethod)
			RecordTenantAPICall(tenantID, "gRPC", method)
		}

		return handler(ctx, req)
	}
}

// BusinessGRPCStreamInterceptor 返回 gRPC stream 业务指标拦截器
func BusinessGRPCStreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := stream.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			if vals := md.Get("x-tenant-id"); len(vals) > 0 && vals[0] != "" {
				ctx = WithTenantID(ctx, vals[0])
			}
			if vals := md.Get("x-user-id"); len(vals) > 0 && vals[0] != "" {
				ctx = WithUserID(ctx, vals[0])
			}
		}

		if tenantID := GetTenantID(ctx); tenantID != "" {
			method := extractMethodName(info.FullMethod)
			RecordTenantAPICall(tenantID, "gRPC-STREAM", method)
		}

		return handler(srv, stream)
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// normalizeBusinessEndpoint 规范化业务 endpoint 避免高基数
// 将路径中的 ID 参数替换为通配符
func normalizeBusinessEndpoint(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if isIDLike(p) {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

// isIDLike 判断字符串是否像 ID（UUID 或纯数字）
func isIDLike(s string) bool {
	if len(s) == 0 {
		return false
	}
	// UUID 格式
	if len(s) == 36 && strings.Count(s, "-") == 4 {
		return true
	}
	// 纯数字
	allDigit := true
	for _, c := range s {
		if c < '0' || c > '9' {
			allDigit = false
			break
		}
	}
	if allDigit && len(s) > 4 {
		return true
	}
	return false
}

// classifyAction 根据 HTTP 方法和 endpoint 分类用户操作类型
func classifyAction(method, endpoint string) string {
	// 根据 endpoint 路径判断操作类型
	lowerEndpoint := strings.ToLower(endpoint)

	if strings.Contains(lowerEndpoint, "export") || strings.Contains(lowerEndpoint, "download") {
		return "export"
	}
	if strings.Contains(lowerEndpoint, "query") || strings.Contains(lowerEndpoint, "search") || strings.Contains(lowerEndpoint, "list") || strings.Contains(lowerEndpoint, "get") {
		return "query"
	}
	if strings.Contains(lowerEndpoint, "create") || strings.Contains(lowerEndpoint, "add") || strings.Contains(lowerEndpoint, "post") {
		return "create"
	}
	if strings.Contains(lowerEndpoint, "update") || strings.Contains(lowerEndpoint, "put") || strings.Contains(lowerEndpoint, "patch") {
		return "update"
	}
	if strings.Contains(lowerEndpoint, "delete") || strings.Contains(lowerEndpoint, "remove") {
		return "delete"
	}

	// 根据 HTTP 方法判断
	switch method {
	case "GET":
		return "query"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "other"
	}
}

// InjectBusinessContext 从 HTTP 请求注入业务上下文到标准 context
// 用于非中间件场景（如直接调用 handler 时）
func InjectBusinessContext(ctx context.Context, tenantID, userID string) context.Context {
	if tenantID != "" {
		ctx = WithTenantID(ctx, tenantID)
	}
	if userID != "" {
		ctx = WithUserID(ctx, userID)
	}
	return ctx
}

// ExtractBusinessContextFromGRPC 从 gRPC metadata 提取业务上下文
func ExtractBusinessContextFromGRPC(ctx context.Context) (tenantID, userID string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ""
	}
	if vals := md.Get("x-tenant-id"); len(vals) > 0 {
		tenantID = vals[0]
	}
	if vals := md.Get("x-user-id"); len(vals) > 0 {
		userID = vals[0]
	}
	return
}
