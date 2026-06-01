// Package tenant 租户上下文全链路透传
//
// 设计目标:
//   - tenant_id 从认证层注入，贯穿整个请求链
//   - gRPC metadata 自动提取/注入
//   - HTTP header 自动提取/注入
//   - Context 中间件支持
//   - 禁止无租户查询
//
// 透传链路:
//
//	HTTP Request (Authorization header)
//	  → JWT 解析 → TenantContext 注入 context.Context
//	    → gRPC metadata (x-tenant-id)
//	      → Storage query (WHERE tenant_id = ?)
//
// 禁止:
//   - 无租户 ID 的查询
//   - 跨租户数据访问
//   - 全局共享查询
package tenant

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"cloud-flow/services/shared/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ---------------------------------------------------------------------------
// Context key (unexported)
// ---------------------------------------------------------------------------

// tenantCtxKey 是 context.Value 的键类型，未导出以防止外部包直接操作。
type tenantCtxKey struct{}

// contextKey 将 *TenantContext 注入到 context.Context 中。
func contextKey(ctx context.Context, tc *TenantContext) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tc)
}

// ---------------------------------------------------------------------------
// TenantContext
// ---------------------------------------------------------------------------

// TenantContext 携带当前请求的租户身份信息，从认证层注入后贯穿整个请求链。
type TenantContext struct {
	// TenantID 租户唯一标识，必填。
	TenantID string

	// UserID 用户唯一标识。
	UserID string

	// Username 用户名。
	Username string

	// Role 用户角色：admin / editor / viewer。
	Role string

	// ProjectID 当前操作所属项目（可选）。
	ProjectID string

	// Namespaces 该用户被授权访问的命名空间列表。
	Namespaces []string

	// IsPlatformAdmin 是否为平台级管理员（可访问所有租户）。
	IsPlatformAdmin bool

	// AuthMethod 认证方式：jwt / oidc / apikey。
	AuthMethod string
}

// ---------------------------------------------------------------------------
// Context methods
// ---------------------------------------------------------------------------

// FromContext 从 context.Context 中提取 TenantContext。
// 第二个返回值表示是否成功提取。
func FromContext(ctx context.Context) (*TenantContext, bool) {
	tc, ok := ctx.Value(tenantCtxKey{}).(*TenantContext)
	return tc, ok
}

// MustFromContext 从 context.Context 中提取 TenantContext。
// 若上下文中不存在租户信息则 panic，仅用于已由中间件强制注入的路径。
func MustFromContext(ctx context.Context) *TenantContext {
	tc, ok := FromContext(ctx)
	if !ok {
		panic("tenant: context does not contain TenantContext")
	}
	return tc
}

// NewContext 将 *TenantContext 注入到 context.Context 中并返回新上下文。
func NewContext(ctx context.Context, tc *TenantContext) context.Context {
	return contextKey(ctx, tc)
}

// WithTenantID 是便捷方法，仅设置 TenantID 后注入上下文。
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	tc := &TenantContext{TenantID: tenantID}
	return NewContext(ctx, tc)
}

// ---------------------------------------------------------------------------
// gRPC metadata 常量
// ---------------------------------------------------------------------------

const (
	// MetadataKeyTenantID gRPC metadata 中租户 ID 的键。
	MetadataKeyTenantID = "x-tenant-id"
	// MetadataKeyUserID gRPC metadata 中用户 ID 的键。
	MetadataKeyUserID = "x-user-id"
	// MetadataKeyRole gRPC metadata 中用户角色的键。
	MetadataKeyRole = "x-user-role"
	// MetadataKeyProjectID gRPC metadata 中项目 ID 的键。
	MetadataKeyProjectID = "x-project-id"
	// MetadataKeyNamespaces gRPC metadata 中命名空间列表的键。
	MetadataKeyNamespaces = "x-namespaces"
)

// ---------------------------------------------------------------------------
// gRPC metadata helpers
// ---------------------------------------------------------------------------

// ExtractFromGRPCMetadata 从 gRPC metadata 中提取 TenantContext。
// 若 metadata 中缺少 tenant_id，则返回的 TenantContext 的 TenantID 为空。
func ExtractFromGRPCMetadata(md metadata.MD) *TenantContext {
	tc := &TenantContext{}

	if vals := md.Get(MetadataKeyTenantID); len(vals) > 0 {
		tc.TenantID = vals[0]
	}
	if vals := md.Get(MetadataKeyUserID); len(vals) > 0 {
		tc.UserID = vals[0]
	}
	if vals := md.Get(MetadataKeyRole); len(vals) > 0 {
		tc.Role = vals[0]
	}
	if vals := md.Get(MetadataKeyProjectID); len(vals) > 0 {
		tc.ProjectID = vals[0]
	}
	if vals := md.Get(MetadataKeyNamespaces); len(vals) > 0 && vals[0] != "" {
		tc.Namespaces = strings.Split(vals[0], ",")
	}

	return tc
}

// InjectToGRPCMetadata 根据 TenantContext 创建 gRPC metadata。
func InjectToGRPCMetadata(tc *TenantContext) metadata.MD {
	md := metadata.MD{}

	if tc.TenantID != "" {
		md.Set(MetadataKeyTenantID, tc.TenantID)
	}
	if tc.UserID != "" {
		md.Set(MetadataKeyUserID, tc.UserID)
	}
	if tc.Role != "" {
		md.Set(MetadataKeyRole, tc.Role)
	}
	if tc.ProjectID != "" {
		md.Set(MetadataKeyProjectID, tc.ProjectID)
	}
	if len(tc.Namespaces) > 0 {
		md.Set(MetadataKeyNamespaces, strings.Join(tc.Namespaces, ","))
	}

	return md
}

// GRPCUnaryServerInterceptor 返回 gRPC 服务端一元拦截器。
// 从 metadata 中提取租户信息并注入到 context 中。
// 若未找到 tenant_id，则返回 codes.PermissionDenied。
func GRPCUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.PermissionDenied, "tenant: missing gRPC metadata")
		}

		tc := ExtractFromGRPCMetadata(md)
		if tc.TenantID == "" {
			return nil, status.Error(codes.PermissionDenied, "tenant: missing tenant_id in metadata")
		}

		ctx = NewContext(ctx, tc)
		return handler(ctx, req)
	}
}

// GRPCUnaryClientInterceptor 返回 gRPC 客户端一元拦截器。
// 将 context 中的 TenantContext 注入到 outgoing gRPC metadata 中。
// 若上下文中无租户信息，则直接透传（不中断调用，由服务端拦截器拒绝）。
func GRPCUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		tc, ok := FromContext(ctx)
		if !ok || tc == nil {
			// 上下文中无租户信息，直接透传，由服务端拦截器决定是否拒绝。
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		md := InjectToGRPCMetadata(tc)
		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ---------------------------------------------------------------------------
// HTTP middleware
// ---------------------------------------------------------------------------

const (
	httpHeaderTenantID = "X-Tenant-Id"
	httpHeaderUserID   = "X-User-Id"
	httpHeaderRole     = "X-User-Role"
	httpHeaderProject  = "X-Project-Id"
	httpHeaderNS       = "X-Namespaces"
	httpHeaderAuth     = "Authorization"
)

// HTTPMiddleware 从认证上下文或 HTTP 请求头中提取租户信息并注入到 request context 中。
// 优先从认证中间件注入的 AuthContext 获取（已验证的 JWT token 信息），其次从请求头获取。
// 安全注意：tenant_id 必须来自经过验证的 JWT token，不应直接信任客户端提供的 X-Tenant-Id header。
//
// 若请求中不包含租户信息，中间件不会拒绝请求（由 RequireTenantID 强制）。
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc := &TenantContext{}

		if authCtx, ok := auth.FromContext(r.Context()); ok && authCtx != nil {
			tc.TenantID = authCtx.TenantID
			tc.UserID = authCtx.UserID
			tc.Username = authCtx.Username
			tc.Role = authCtx.Role
			tc.AuthMethod = authCtx.AuthMethod
		} else {
			if v := r.Header.Get(httpHeaderTenantID); v != "" {
				tc.TenantID = v
			}
			if v := r.Header.Get(httpHeaderUserID); v != "" {
				tc.UserID = v
			}
			if v := r.Header.Get(httpHeaderRole); v != "" {
				tc.Role = v
			}
			if v := r.Header.Get(httpHeaderProject); v != "" {
				tc.ProjectID = v
			}
			if v := r.Header.Get(httpHeaderNS); v != "" {
				tc.Namespaces = strings.Split(v, ",")
			}
			if v := r.Header.Get(httpHeaderAuth); v != "" {
				tc.AuthMethod = "jwt"
			}
		}

		ctx := NewContext(r.Context(), tc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireTenantID 拒绝未携带 tenant_id 的请求，返回 HTTP 403。
// 应在 HTTPMiddleware 之后使用。
func RequireTenantID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := FromContext(r.Context())
		if !ok || tc == nil || tc.TenantID == "" {
			http.Error(w, `{"error":"tenant_id is required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ErrTenantAccessDenied 跨租户访问错误。
var ErrTenantAccessDenied = fmt.Errorf("tenant: access denied for target tenant")

// ErrNamespaceAccessDenied 命名空间访问错误。
var ErrNamespaceAccessDenied = fmt.Errorf("tenant: namespace access denied")

// ErrProjectAccessDenied 项目访问错误。
var ErrProjectAccessDenied = fmt.Errorf("tenant: project access denied")

// ValidateTenantAccess 检查上下文中的租户是否与目标租户匹配。
// 平台管理员（IsPlatformAdmin）可绕过此检查。
func ValidateTenantAccess(ctx context.Context, targetTenantID string) error {
	tc, ok := FromContext(ctx)
	if !ok || tc == nil {
		return fmt.Errorf("tenant: no tenant context available")
	}

	// 平台管理员可访问所有租户。
	if tc.IsPlatformAdmin {
		return nil
	}

	if tc.TenantID != targetTenantID {
		return fmt.Errorf("%w: current tenant %q, target tenant %q",
			ErrTenantAccessDenied, tc.TenantID, targetTenantID)
	}
	return nil
}

// ValidateNamespaceAccess 检查请求的命名空间是否在用户允许的列表中。
// 平台管理员或未设置 Namespaces 限制时放行。
func ValidateNamespaceAccess(ctx context.Context, namespace string) error {
	tc, ok := FromContext(ctx)
	if !ok || tc == nil {
		return fmt.Errorf("tenant: no tenant context available")
	}

	// 平台管理员可访问所有命名空间。
	if tc.IsPlatformAdmin {
		return nil
	}

	// 未设置命名空间限制时放行。
	if len(tc.Namespaces) == 0 {
		return nil
	}

	for _, ns := range tc.Namespaces {
		if ns == namespace {
			return nil
		}
	}

	return fmt.Errorf("%w: namespace %q is not in allowed list %v",
		ErrNamespaceAccessDenied, namespace, tc.Namespaces)
}

// ValidateProjectAccess 检查用户是否有权访问指定项目。
// 若上下文中的 ProjectID 与目标匹配则放行；平台管理员可绕过。
func ValidateProjectAccess(ctx context.Context, projectID string) error {
	tc, ok := FromContext(ctx)
	if !ok || tc == nil {
		return fmt.Errorf("tenant: no tenant context available")
	}

	// 平台管理员可访问所有项目。
	if tc.IsPlatformAdmin {
		return nil
	}

	// 若上下文中未设置 ProjectID，则无法判断，拒绝访问。
	if tc.ProjectID == "" {
		return fmt.Errorf("%w: no project context available", ErrProjectAccessDenied)
	}

	if tc.ProjectID != projectID {
		return fmt.Errorf("%w: current project %q, target project %q",
			ErrProjectAccessDenied, tc.ProjectID, projectID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Storage query helpers
// ---------------------------------------------------------------------------

// TenantFilter 返回用于 SQL WHERE 子句的租户过滤条件。
// 对 tenantID 进行单引号转义以防止 SQL 注入。
//
// 使用示例:
//
//	query := "SELECT * FROM resources WHERE " + tenant.TenantFilter(tenantID)
func TenantFilter(tenantID string) string {
	escaped := strings.ReplaceAll(tenantID, "'", "''")
	return fmt.Sprintf("tenant_id = '%s'", escaped)
}

// MustHaveTenant 从 context 中提取 tenant_id，若不存在则 panic。
// 用于已由中间件强制保证租户存在的存储查询路径。
func MustHaveTenant(ctx context.Context) string {
	tc := MustFromContext(ctx)
	if tc.TenantID == "" {
		panic("tenant: tenant_id is empty in context")
	}
	return tc.TenantID
}
