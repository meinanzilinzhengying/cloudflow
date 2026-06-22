//go:build linux

package rbac

import (
	"fmt"
	"time"
)

// ============================================================================
// 一、内置角色定义
// ============================================================================

// BuiltinRole 内置角色
type BuiltinRole string

const (
	RoleSuperAdmin BuiltinRole = "super_admin" // 超级管理员：所有权限
	RoleAdmin      BuiltinRole = "admin"       // 管理员：租户管理权限
	RoleEditor     BuiltinRole = "editor"      // 编辑者：读写权限
	RoleViewer     BuiltinRole = "viewer"      // 查看者：只读权限
	RoleOperator   BuiltinRole = "operator"      // 运维人员：查看+执行
	RoleAuditor    BuiltinRole = "auditor"       // 审计人员：查看+导出
)

// String 返回角色名称
func (r BuiltinRole) String() string { return string(r) }

// AllBuiltinRoles 所有内置角色
var AllBuiltinRoles = []BuiltinRole{
	RoleSuperAdmin, RoleAdmin, RoleEditor, RoleViewer, RoleOperator, RoleAuditor,
}

// IsBuiltin 检查是否为内置角色
func (r BuiltinRole) IsBuiltin() bool {
	for _, br := range AllBuiltinRoles {
		if br == r {
			return true
		}
	}
	return false
}

// ============================================================================
// 二、角色定义
// ============================================================================

// Role 角色定义
type Role struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Builtin     BuiltinRole   `json:"builtin,omitempty"` // 内置角色标识

	// 权限
	Permissions PermissionSet `json:"permissions"`

	// 数据范围
	DataScope DataScope `json:"data_scope"`

	// 元数据
	TenantID  string            `json:"tenant_id,omitempty"` // 空表示全局角色
	Labels    map[string]string `json:"labels,omitempty"`

	// 时间戳
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsBuiltinRole 是否为内置角色
func (r *Role) IsBuiltinRole() bool {
	return r.Builtin != ""
}

// HasPermission 检查是否有权限
func (r *Role) HasPermission(p Permission) bool {
	return r.Permissions.Contains(p)
}

// HasPermissionString 检查是否有权限（字符串格式）
func (r *Role) HasPermissionString(s string) bool {
	return r.Permissions.ContainsString(s)
}

// AddPermission 添加权限
func (r *Role) AddPermission(p Permission) {
	r.Permissions.Add(p)
	r.UpdatedAt = time.Now()
}

// RemovePermission 移除权限
func (r *Role) RemovePermission(p Permission) {
	r.Permissions.Remove(p)
	r.UpdatedAt = time.Now()
}

// ============================================================================
// 三、内置角色工厂函数
// ============================================================================

// NewSuperAdminRole 创建超级管理员角色
func NewSuperAdminRole() *Role {
	return &Role{
		ID:          "role-super-admin",
		Name:        "超级管理员",
		Description: "系统超级管理员，拥有所有权限",
		Builtin:     RoleSuperAdmin,
		Permissions: FullPermissions(),
		DataScope:   DataScopeGlobal,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// NewAdminRole 创建管理员角色
func NewAdminRole() *Role {
	perms := FullPermissions()
	// 移除超级管理员专属权限（用户/租户管理中的删除）
	perms.Remove(NewPermission(ResourceUser, ActionDelete))
	perms.Remove(NewPermission(ResourceTenant, ActionDelete))
	perms.Remove(NewPermission(ResourcePolicy, ActionDelete))

	return &Role{
		ID:          "role-admin",
		Name:        "管理员",
		Description: "租户管理员，管理租户内所有资源",
		Builtin:     RoleAdmin,
		Permissions: perms,
		DataScope:   DataScopeTenant,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// NewEditorRole 创建编辑者角色
func NewEditorRole() *Role {
	return &Role{
		ID:          "role-editor",
		Name:        "编辑者",
		Description: "可以查看和编辑资源，但不能删除",
		Builtin:     RoleEditor,
		Permissions: PermissionSet{
			NewPermission(ResourceDashboard, ActionView),
			NewPermission(ResourceDashboard, ActionCreate),
			NewPermission(ResourceDashboard, ActionEdit),
			NewPermission(ResourceAsset, ActionView),
			NewPermission(ResourceAsset, ActionCreate),
			NewPermission(ResourceAsset, ActionEdit),
			NewPermission(ResourceAsset, ActionExport),
			NewPermission(ResourceAlert, ActionView),
			NewPermission(ResourceAlert, ActionCreate),
			NewPermission(ResourceAlert, ActionEdit),
			NewPermission(ResourceTopology, ActionView),
			NewPermission(ResourceTrace, ActionView),
			NewPermission(ResourceTrace, ActionExport),
			NewPermission(ResourceMetric, ActionView),
			NewPermission(ResourceMetric, ActionExport),
			NewPermission(ResourceReport, ActionView),
			NewPermission(ResourceReport, ActionCreate),
			NewPermission(ResourceReport, ActionEdit),
			NewPermission(ResourceReport, ActionExport),
			NewPermission(ResourceConfig, ActionView),
		},
		DataScope: DataScopeTenant,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// NewViewerRole 创建查看者角色
func NewViewerRole() *Role {
	return &Role{
		ID:          "role-viewer",
		Name:        "查看者",
		Description: "只读访问所有资源",
		Builtin:     RoleViewer,
		Permissions: ReadOnlyPermissions(),
		DataScope:   DataScopeTenant,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// NewOperatorRole 创建运维角色
func NewOperatorRole() *Role {
	return &Role{
		ID:          "role-operator",
		Name:        "运维人员",
		Description: "负责日常运维操作和告警处理",
		Builtin:     RoleOperator,
		Permissions: OperatorPermissions(),
		DataScope:   DataScopeTenant,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// NewAuditorRole 创建审计角色
func NewAuditorRole() *Role {
	return &Role{
		ID:          "role-auditor",
		Name:        "审计人员",
		Description: "负责安全审计和合规检查",
		Builtin:     RoleAuditor,
		Permissions: AuditorPermissions(),
		DataScope:   DataScopeGlobal,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// ============================================================================
// 四、角色模板
// ============================================================================

// RoleTemplate 角色模板
type RoleTemplate struct {
	BuiltinRole BuiltinRole
	Factory     func() *Role
}

// BuiltinRoleTemplates 内置角色模板
var BuiltinRoleTemplates = map[BuiltinRole]RoleTemplate{
	RoleSuperAdmin: {RoleSuperAdmin, NewSuperAdminRole},
	RoleAdmin:      {RoleAdmin, NewAdminRole},
	RoleEditor:     {RoleEditor, NewEditorRole},
	RoleViewer:     {RoleViewer, NewViewerRole},
	RoleOperator:   {RoleOperator, NewOperatorRole},
	RoleAuditor:    {RoleAuditor, NewAuditorRole},
}

// CreateBuiltinRole 创建内置角色实例
func CreateBuiltinRole(br BuiltinRole) (*Role, error) {
	template, ok := BuiltinRoleTemplates[br]
	if !ok {
		return nil, fmt.Errorf("unknown builtin role: %s", br)
	}
	return template.Factory(), nil
}

// ============================================================================
// 五、角色绑定
// ============================================================================

// RoleBinding 用户-角色绑定
type RoleBinding struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	RoleID    string    `json:"role_id"`
	TenantID  string    `json:"tenant_id,omitempty"`
	Scope     DataScope `json:"scope"` // 可覆盖角色的数据范围
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

// ============================================================================
// 六、用户权限聚合
// ============================================================================

// UserPermissions 用户聚合权限
type UserPermissions struct {
	UserID      string        `json:"user_id"`
	TenantID    string        `json:"tenant_id"`
	Permissions PermissionSet   `json:"permissions"`
	DataScope   DataScope     `json:"data_scope"`
	RoleIDs     []string      `json:"role_ids"`
}

// HasPermission 检查是否有权限
func (up *UserPermissions) HasPermission(p Permission) bool {
	return up.Permissions.Contains(p)
}

// CanAccessTenant 是否能访问指定租户数据
func (up *UserPermissions) CanAccessTenant(tenantID string) bool {
	if up.DataScope == DataScopeGlobal {
		return true
	}
	return up.TenantID == tenantID
}

// CanAccessResource 是否能访问资源（数据权限）
func (up *UserPermissions) CanAccessResource(resourceTenantID string, ownerID string) bool {
	switch up.DataScope {
	case DataScopeGlobal:
		return true
	case DataScopeTenant:
		return up.TenantID == resourceTenantID
	case DataScopeSelf:
		return up.UserID == ownerID
	case DataScopeAssigned:
		// 指派资源需额外检查
		return up.TenantID == resourceTenantID // 简化：先检查租户
	default:
		return false
	}
}
