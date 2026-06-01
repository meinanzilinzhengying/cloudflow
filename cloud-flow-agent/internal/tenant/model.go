//go:build linux

// Package tenant 提供多租户管理功能
// - 管理员/租户视图
// - RBAC权限控制
package tenant

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// Role 用户角色
type Role string

const (
	RoleAdmin   Role = "admin"   // 管理员
	RoleTenant  Role = "tenant"  // 租户
	RoleViewer  Role = "viewer"  // 只读用户
	RoleOperator Role = "operator" // 运维人员
)

// String 返回角色名称
func (r Role) String() string {
	return string(r)
}

// HasPermission 检查是否有权限
func (r Role) HasPermission(permission Permission) bool {
	switch r {
	case RoleAdmin:
		return true
	case RoleTenant:
		return permission == PermissionView || permission == PermissionManage || permission == PermissionAlert
	case RoleOperator:
		return permission == PermissionView || permission == PermissionAlert
	case RoleViewer:
		return permission == PermissionView
	default:
		return false
	}
}

// Permission 权限类型
type Permission string

const (
	PermissionView   Permission = "view"   // 查看
	PermissionManage Permission = "manage" // 管理
	PermissionAlert  Permission = "alert"  // 告警操作
	PermissionAdmin  Permission = "admin"  // 管理员
)

// Tenant 租户
type Tenant struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      TenantStatus      `json:"status"`
	
	// 配额
	Quota       TenantQuota       `json:"quota"`
	
	// 元数据
	Metadata    map[string]string `json:"metadata"`
	Labels      map[string]string `json:"labels"`
	
	// 时间戳
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

// TenantStatus 租户状态
type TenantStatus string

const (
	TenantStatusActive   TenantStatus = "active"
	TenantStatusInactive TenantStatus = "inactive"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusExpired  TenantStatus = "expired"
)

// TenantQuota 租户配额
type TenantQuota struct {
	MaxAssets      int `json:"max_assets"`      // 最大资产数
	MaxUsers       int `json:"max_users"`       // 最大用户数
	MaxAlerts      int `json:"max_alerts"`      // 最大告警数
	MaxStorageGB   int `json:"max_storage_gb"`  // 最大存储(GB)
	MaxQueryQPS    int `json:"max_query_qps"`   // 最大查询QPS
}

// DefaultTenantQuota 默认配额
func DefaultTenantQuota() TenantQuota {
	return TenantQuota{
		MaxAssets:    100,
		MaxUsers:     10,
		MaxAlerts:    1000,
		MaxStorageGB: 10,
		MaxQueryQPS:  100,
	}
}

// User 用户
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone,omitempty"`
	
	// 归属
	TenantID string `json:"tenant_id"`
	Role     Role   `json:"role"`
	
	// 状态
	Status   UserStatus `json:"status"`
	LastLogin time.Time `json:"last_login,omitempty"`
	
	// 偏好
	Preferences UserPreferences `json:"preferences"`
	
	// 时间戳
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserStatus 用户状态
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusInactive  UserStatus = "inactive"
	UserStatusLocked    UserStatus = "locked"
	UserStatusSuspended UserStatus = "suspended"
)

// UserPreferences 用户偏好
type UserPreferences struct {
	Language       string `json:"language"`        // 语言
	Timezone       string `json:"timezone"`        // 时区
	DefaultView    string `json:"default_view"`    // 默认视图
	PageSize       int    `json:"page_size"`       // 分页大小
	EmailNotify    bool   `json:"email_notify"`    // 邮件通知
	SMSNotify      bool   `json:"sms_notify"`      // 短信通知
}

// DefaultUserPreferences 默认偏好
func DefaultUserPreferences() UserPreferences {
	return UserPreferences{
		Language:    "zh-CN",
		Timezone:    "Asia/Shanghai",
		DefaultView: "dashboard",
		PageSize:    20,
		EmailNotify: true,
		SMSNotify:   false,
	}
}

// TenantManager 租户管理器
type TenantManager struct {
	mu       sync.RWMutex
	tenants  map[string]*Tenant
	users    map[string]*User
	log      *logger.Logger
}

// NewTenantManager 创建租户管理器
func NewTenantManager(log *logger.Logger) *TenantManager {
	return &TenantManager{
		tenants: make(map[string]*Tenant),
		users:   make(map[string]*User),
		log:     log,
	}
}

// CreateTenant 创建租户
func (m *TenantManager) CreateTenant(tenant *Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if tenant.ID == "" {
		return fmt.Errorf("租户ID不能为空")
	}
	
	if _, exists := m.tenants[tenant.ID]; exists {
		return fmt.Errorf("租户已存在: %s", tenant.ID)
	}
	
	if tenant.Quota.MaxAssets == 0 {
		tenant.Quota = DefaultTenantQuota()
	}
	
	tenant.Status = TenantStatusActive
	tenant.CreatedAt = time.Now()
	tenant.UpdatedAt = time.Now()
	
	m.tenants[tenant.ID] = tenant
	m.log.Infof("创建租户: %s (%s)", tenant.Name, tenant.ID)
	
	return nil
}

// GetTenant 获取租户
func (m *TenantManager) GetTenant(tenantID string) *Tenant {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tenants[tenantID]
}

// UpdateTenant 更新租户
func (m *TenantManager) UpdateTenant(tenant *Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.tenants[tenant.ID]; !exists {
		return fmt.Errorf("租户不存在: %s", tenant.ID)
	}
	
	tenant.UpdatedAt = time.Now()
	m.tenants[tenant.ID] = tenant
	
	return nil
}

// DeleteTenant 删除租户
func (m *TenantManager) DeleteTenant(tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 检查是否还有用户
	for _, user := range m.users {
		if user.TenantID == tenantID {
			return fmt.Errorf("租户下还有用户，无法删除")
		}
	}
	
	delete(m.tenants, tenantID)
	m.log.Infof("删除租户: %s", tenantID)
	
	return nil
}

// ListTenants 列出所有租户
func (m *TenantManager) ListTenants() []*Tenant {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	tenants := make([]*Tenant, 0, len(m.tenants))
	for _, tenant := range m.tenants {
		tenants = append(tenants, tenant)
	}
	return tenants
}

// CreateUser 创建用户
func (m *TenantManager) CreateUser(user *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if user.ID == "" {
		return fmt.Errorf("用户ID不能为空")
	}
	
	if _, exists := m.users[user.ID]; exists {
		return fmt.Errorf("用户已存在: %s", user.ID)
	}
	
	// 检查租户是否存在
	if user.TenantID != "" {
		if _, exists := m.tenants[user.TenantID]; !exists {
			return fmt.Errorf("租户不存在: %s", user.TenantID)
		}
		
		// 检查租户用户配额
		tenant := m.tenants[user.TenantID]
		userCount := 0
		for _, u := range m.users {
			if u.TenantID == user.TenantID {
				userCount++
			}
		}
		if userCount >= tenant.Quota.MaxUsers {
			return fmt.Errorf("租户用户数量已达到配额限制")
		}
	}
	
	user.Status = UserStatusActive
	user.Preferences = DefaultUserPreferences()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	
	m.users[user.ID] = user
	m.log.Infof("创建用户: %s (%s), 租户: %s", user.Username, user.ID, user.TenantID)
	
	return nil
}

// GetUser 获取用户
func (m *TenantManager) GetUser(userID string) *User {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.users[userID]
}

// GetUserByUsername 根据用户名获取用户
func (m *TenantManager) GetUserByUsername(username string) *User {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, user := range m.users {
		if user.Username == username {
			return user
		}
	}
	return nil
}

// UpdateUser 更新用户
func (m *TenantManager) UpdateUser(user *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.users[user.ID]; !exists {
		return fmt.Errorf("用户不存在: %s", user.ID)
	}
	
	user.UpdatedAt = time.Now()
	m.users[user.ID] = user
	
	return nil
}

// DeleteUser 删除用户
func (m *TenantManager) DeleteUser(userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	delete(m.users, userID)
	m.log.Infof("删除用户: %s", userID)
	
	return nil
}

// ListUsers 列出用户
func (m *TenantManager) ListUsers(tenantID string) []*User {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var users []*User
	for _, user := range m.users {
		if tenantID == "" || user.TenantID == tenantID {
			users = append(users, user)
		}
	}
	return users
}

// ListUsersByTenant 列出租户下的所有用户
func (m *TenantManager) ListUsersByTenant(tenantID string) []*User {
	return m.ListUsers(tenantID)
}

// CheckPermission 检查权限
func (m *TenantManager) CheckPermission(userID string, permission Permission) bool {
	user := m.GetUser(userID)
	if user == nil {
		return false
	}
	
	return user.Role.HasPermission(permission)
}

// GetTenantStats 获取租户统计
func (m *TenantManager) GetTenantStats(tenantID string) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	stats := map[string]interface{}{
		"user_count":  0,
		"asset_count": 0, // 需要资产模块提供
		"alert_count": 0, // 需要告警模块提供
	}
	
	for _, user := range m.users {
		if user.TenantID == tenantID {
			stats["user_count"] = stats["user_count"].(int) + 1
		}
	}
	
	return stats
}

// ContextKey 上下文键
type ContextKey string

const (
	ContextKeyUser    ContextKey = "user"
	ContextKeyTenant  ContextKey = "tenant"
	ContextKeyRole    ContextKey = "role"
)

// GetUserFromContext 从上下文获取用户
func GetUserFromContext(ctx context.Context) *User {
	if user, ok := ctx.Value(ContextKeyUser).(*User); ok {
		return user
	}
	return nil
}

// GetTenantIDFromContext 从上下文获取租户ID
func GetTenantIDFromContext(ctx context.Context) string {
	if user := GetUserFromContext(ctx); user != nil {
		return user.TenantID
	}
	if tenantID, ok := ctx.Value(ContextKeyTenant).(string); ok {
		return tenantID
	}
	return ""
}

// IsAdmin 检查是否是管理员
func IsAdmin(ctx context.Context) bool {
	if user := GetUserFromContext(ctx); user != nil {
		return user.Role == RoleAdmin
	}
	return false
}

// IsTenantUser 检查是否是租户用户
func IsTenantUser(ctx context.Context) bool {
	if user := GetUserFromContext(ctx); user != nil {
		return user.Role == RoleTenant
	}
	return false
}
