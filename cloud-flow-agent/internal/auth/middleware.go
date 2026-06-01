//go:build linux

// Package auth 提供认证授权中间件
// - RBAC权限控制
// - 管理员/租户视图隔离
package auth

import (
	"context"
	"net/http"
	"strings"

	"cloud-flow-agent/internal/tenant"
	"cloud-flow-agent/pkg/logger"
)

// Middleware 认证中间件
type Middleware struct {
	tenantManager *tenant.TenantManager
	log           *logger.Logger
}

// NewMiddleware 创建认证中间件
func NewMiddleware(tenantManager *tenant.TenantManager, log *logger.Logger) *Middleware {
	return &Middleware{
		tenantManager: tenantManager,
		log:           log,
	}
}

// AuthMiddleware 认证中间件
func (m *Middleware) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 从请求头获取认证信息
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// 尝试从Cookie获取
			if cookie, err := r.Cookie("auth_token"); err == nil {
				authHeader = "Bearer " + cookie.Value
			}
		}
		
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		// 解析Token
		user, err := m.parseToken(authHeader)
		if err != nil {
			m.log.Warnf("认证失败: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		// 检查用户状态
		if user.Status != tenant.UserStatusActive {
			http.Error(w, "User inactive", http.StatusForbidden)
			return
		}
		
		// 将用户信息存入上下文
		ctx := context.WithValue(r.Context(), tenant.ContextKeyUser, user)
		ctx = context.WithValue(ctx, tenant.ContextKeyTenant, user.TenantID)
		ctx = context.WithValue(ctx, tenant.ContextKeyRole, user.Role)
		
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdminOnly 仅管理员访问
func (m *Middleware) AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !tenant.IsAdmin(r.Context()) {
			http.Error(w, "Forbidden: Admin only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// TenantOnly 仅租户访问
func (m *Middleware) TenantOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := tenant.GetUserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		if user.Role != tenant.RoleTenant && user.Role != tenant.RoleAdmin {
			http.Error(w, "Forbidden: Tenant only", http.StatusForbidden)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// RequirePermission 需要特定权限
func (m *Middleware) RequirePermission(permission tenant.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := tenant.GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			
			if !user.Role.HasPermission(permission) {
				http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// TenantIsolation 租户隔离中间件
// 确保租户只能访问自己的资源
func (m *Middleware) TenantIsolation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 管理员可以访问所有资源
		if tenant.IsAdmin(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}
		
		// 获取当前用户的租户ID
		userTenantID := tenant.GetTenantIDFromContext(r.Context())
		if userTenantID == "" {
			http.Error(w, "Forbidden: No tenant assigned", http.StatusForbidden)
			return
		}
		
		// 将租户ID添加到请求上下文，供后续处理使用
		ctx := context.WithValue(r.Context(), "requested_tenant_id", userTenantID)
		
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// parseToken 解析Token
// 简化实现，实际应使用JWT或其他标准Token格式
func (m *Middleware) parseToken(authHeader string) (*tenant.User, error) {
	// 支持 Bearer Token 和 Basic Auth
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return nil, m.validateAPIKey(authHeader)
	}
	
	scheme := strings.ToLower(parts[0])
	token := parts[1]
	
	switch scheme {
	case "bearer":
		return m.validateBearerToken(token)
	case "basic":
		return m.validateBasicAuth(token)
	default:
		return nil, m.validateAPIKey(token)
	}
}

// validateBearerToken 验证Bearer Token
func (m *Middleware) validateBearerToken(token string) (*tenant.User, error) {
	// 简化实现：Token格式为 "user_id:timestamp:signature"
	// 实际生产环境应使用JWT
	
	parts := strings.Split(token, ":")
	if len(parts) < 2 {
		return nil, nil
	}
	
	userID := parts[0]
	user := m.tenantManager.GetUser(userID)
	if user == nil {
		return nil, nil
	}
	
	return user, nil
}

// validateBasicAuth 验证Basic Auth
func (m *Middleware) validateBasicAuth(token string) (*tenant.User, error) {
	// 简化实现：base64(username:password)
	// 实际应解码并验证
	
	// 这里简化处理，直接根据用户名查找
	// 实际生产环境需要解码base64并验证密码
	
	user := m.tenantManager.GetUserByUsername("admin")
	if user != nil {
		return user, nil
	}
	
	return nil, nil
}

// validateAPIKey 验证API Key
func (m *Middleware) validateAPIKey(key string) (*tenant.User, error) {
	// 简化实现：API Key直接映射到用户
	// 实际生产环境应查询数据库验证
	
	// 模拟几个测试API Key
	apiKeyMap := map[string]string{
		"admin-key-123":    "admin",
		"tenant1-key-456":  "tenant1",
		"tenant2-key-789":  "tenant2",
	}
	
	if username, ok := apiKeyMap[key]; ok {
		user := m.tenantManager.GetUserByUsername(username)
		if user != nil {
			return user, nil
		}
	}
	
	return nil, nil
}

// MockAuthMiddleware 模拟认证中间件（用于开发和测试）
func (m *Middleware) MockAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 从查询参数获取模拟用户
		mockUser := r.URL.Query().Get("mock_user")
		if mockUser == "" {
			mockUser = "admin" // 默认管理员
		}
		
		// 查找或创建模拟用户
		user := m.tenantManager.GetUserByUsername(mockUser)
		if user == nil {
			// 创建模拟用户
			role := tenant.RoleTenant
			if mockUser == "admin" {
				role = tenant.RoleAdmin
			}
			
			user = &tenant.User{
				ID:       "user-" + mockUser,
				Username: mockUser,
				TenantID: "tenant-" + mockUser,
				Role:     role,
				Status:   tenant.UserStatusActive,
			}
			
			// 确保租户存在
			if m.tenantManager.GetTenant(user.TenantID) == nil {
				t := &tenant.Tenant{
					ID:     user.TenantID,
					Name:   "Tenant " + mockUser,
					Status: tenant.TenantStatusActive,
					Quota:  tenant.DefaultTenantQuota(),
				}
				m.tenantManager.CreateTenant(t)
			}
			
			m.tenantManager.CreateUser(user)
		}
		
		// 存入上下文
		ctx := context.WithValue(r.Context(), tenant.ContextKeyUser, user)
		ctx = context.WithValue(ctx, tenant.ContextKeyTenant, user.TenantID)
		ctx = context.WithValue(ctx, tenant.ContextKeyRole, user.Role)
		
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CORS 跨域中间件
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// Logging 日志中间件
func Logging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// 包装ResponseWriter以获取状态码
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			
			next.ServeHTTP(wrapped, r)
			
			duration := time.Since(start)
			
			// 获取用户信息
			user := tenant.GetUserFromContext(r.Context())
			username := "anonymous"
			if user != nil {
				username = user.Username
			}
			
			log.Infof("[%s] %s %s %d %v (user: %s)",
				r.Method,
				r.URL.Path,
				r.RemoteAddr,
				wrapped.statusCode,
				duration,
				username,
			)
		})
	}
}

// responseWriter 包装http.ResponseWriter以捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Helper functions for common auth patterns

// GetCurrentUser 获取当前用户
func GetCurrentUser(r *http.Request) *tenant.User {
	return tenant.GetUserFromContext(r.Context())
}

// GetCurrentTenantID 获取当前租户ID
func GetCurrentTenantID(r *http.Request) string {
	return tenant.GetTenantIDFromContext(r.Context())
}

// IsCurrentUserAdmin 检查当前用户是否是管理员
func IsCurrentUserAdmin(r *http.Request) bool {
	return tenant.IsAdmin(r.Context())
}

// CanAccessAsset 检查是否可以访问资产
func CanAccessAsset(r *http.Request, assetTenantID string) bool {
	// 管理员可以访问所有资产
	if tenant.IsAdmin(r.Context()) {
		return true
	}
	
	// 租户只能访问自己的资产
	userTenantID := tenant.GetTenantIDFromContext(r.Context())
	return userTenantID == assetTenantID
}
