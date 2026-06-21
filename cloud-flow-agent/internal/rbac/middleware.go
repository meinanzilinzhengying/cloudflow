//go:build linux

package rbac

import (
	"context"
	"net/http"
	"strings"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/tenant"
)

// ============================================================================
// 一、RBAC HTTP 中间件
// ============================================================================

// HTTPMiddleware RBAC HTTP 中间件
type HTTPMiddleware struct {
	manager *Manager
}

// NewHTTPMiddleware 创建中间件
func NewHTTPMiddleware(manager *Manager) *HTTPMiddleware {
	return &HTTPMiddleware{manager: manager}
}

// RequirePermission 需要特定权限
func (m *HTTPMiddleware) RequirePermission(resource ResourceType, action ActionType) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := tenant.GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			allowed, scope := m.manager.CheckPermission(user.ID, NewPermission(resource, action))
			if !allowed {
				http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
				return
			}

			// 将数据范围注入上下文
			ctx := context.WithValue(r.Context(), "data_scope", scope)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermissionString 需要特定权限（字符串格式）
func (m *HTTPMiddleware) RequirePermissionString(permissionStr string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := tenant.GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			allowed, scope := m.manager.CheckPermissionString(user.ID, permissionStr)
			if !allowed {
				http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), "data_scope", scope)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAnyPermission 需要任一权限
func (m *HTTPMiddleware) RequireAnyPermission(permissions ...Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := tenant.GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			allowed := false
			var scope DataScope
			for _, perm := range permissions {
				ok, s := m.manager.CheckPermission(user.ID, perm)
				if ok {
					allowed = true
					if s.Priority() > scope.Priority() {
						scope = s
					}
				}
			}

			if !allowed {
				http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), "data_scope", scope)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAllPermissions 需要所有权限
func (m *HTTPMiddleware) RequireAllPermissions(permissions ...Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := tenant.GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			var scope DataScope = DataScopeSelf
			for _, perm := range permissions {
				allowed, s := m.manager.CheckPermission(user.ID, perm)
				if !allowed {
					http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
					return
				}
				if s.Priority() > scope.Priority() {
					scope = s
				}
			}

			ctx := context.WithValue(r.Context(), "data_scope", scope)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ============================================================================
// 二、数据权限中间件
// ============================================================================

// DataScopeMiddleware 数据权限中间件
// 自动为请求注入数据权限上下文
func (m *HTTPMiddleware) DataScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := tenant.GetUserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		perms := m.manager.GetUserPermissions(user.ID)
		if perms == nil {
			http.Error(w, "Forbidden: No permissions", http.StatusForbidden)
			return
		}

		// 创建数据权限上下文
		authCtx := NewDataAuthContext(user.ID, user.TenantID, perms.DataScope)
		ctx := context.WithValue(r.Context(), "data_auth_context", authCtx)
		ctx = context.WithValue(ctx, "data_scope", perms.DataScope)
		ctx = context.WithValue(ctx, "user_permissions", perms)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TenantDataIsolation 租户数据隔离中间件
// 自动过滤请求中的 tenant_id 参数，确保用户只能访问自己的数据
func (m *HTTPMiddleware) TenantDataIsolation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := tenant.GetUserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		perms := m.manager.GetUserPermissions(user.ID)
		if perms == nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// 管理员跳过隔离
		if perms.DataScope == DataScopeGlobal {
			next.ServeHTTP(w, r)
			return
		}

		// 强制注入当前租户ID到查询参数和上下文
		query := r.URL.Query()
		query.Set("tenant_id", user.TenantID)
		r.URL.RawQuery = query.Encode()

		ctx := context.WithValue(r.Context(), tenant.ContextKeyTenant, user.TenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ============================================================================
// 三、资源访问控制中间件
// ============================================================================

// ResourceAccessMiddleware 资源访问控制中间件
// 检查对特定资源的访问权限
func (m *HTTPMiddleware) ResourceAccessMiddleware(resourceType ResourceType) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := tenant.GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// 从请求解析操作类型
			action := actionFromMethod(r.Method)

			// 从请求获取资源租户和所有者
			resourceTenantID := r.URL.Query().Get("tenant_id")
			if resourceTenantID == "" {
				resourceTenantID = user.TenantID
			}
			ownerID := r.URL.Query().Get("owner_id")
			if ownerID == "" {
				ownerID = user.ID
			}

			allowed, result := m.manager.CheckResourceAccess(user.ID, resourceType, action, resourceTenantID, ownerID)
			if !allowed {
				http.Error(w, "Forbidden: "+result.Reason, http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), "data_scope", result.DataScope)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// actionFromMethod 从 HTTP 方法推断操作类型
func actionFromMethod(method string) ActionType {
	switch strings.ToUpper(method) {
	case http.MethodGet:
		return ActionView
	case http.MethodPost:
		return ActionCreate
	case http.MethodPut, http.MethodPatch:
		return ActionEdit
	case http.MethodDelete:
		return ActionDelete
	default:
		return ActionView
	}
}

// ============================================================================
// 四、辅助函数
// ============================================================================

// GetDataScopeFromContext 从上下文获取数据范围
func GetDataScopeFromContext(ctx context.Context) DataScope {
	if scope, ok := ctx.Value("data_scope").(DataScope); ok {
		return scope
	}
	return DataScopeSelf
}

// GetDataAuthContext 从上下文获取数据权限上下文
func GetDataAuthContext(ctx context.Context) *DataAuthContext {
	if authCtx, ok := ctx.Value("data_auth_context").(*DataAuthContext); ok {
		return authCtx
	}
	return nil
}

// GetUserPermissionsFromContext 从上下文获取用户权限
func GetUserPermissionsFromContext(ctx context.Context) *UserPermissions {
	if perms, ok := ctx.Value("user_permissions").(*UserPermissions); ok {
		return perms
	}
	return nil
}
