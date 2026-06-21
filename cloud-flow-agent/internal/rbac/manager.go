//go:build linux

package rbac

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 一、RBAC 管理器
// ============================================================================

// Manager RBAC 管理器
type Manager struct {
	mu sync.RWMutex

	roles       map[string]*Role
	bindings    map[string]*RoleBinding
	policies    map[string]*DataPolicy
	userPerms   map[string]*UserPermissions

	// 缓存
	permCache   map[string]*UserPermissions
	cacheMu     sync.RWMutex
}

// NewManager 创建 RBAC 管理器
func NewManager() *Manager {
	m := &Manager{
		roles:     make(map[string]*Role),
		bindings:  make(map[string]*RoleBinding),
		policies:  make(map[string]*DataPolicy),
		userPerms: make(map[string]*UserPermissions),
		permCache: make(map[string]*UserPermissions),
	}
	// 初始化内置角色
	m.initBuiltinRoles()
	return m
}

// initBuiltinRoles 初始化内置角色
func (m *Manager) initBuiltinRoles() {
	for _, br := range AllBuiltinRoles {
		role, err := CreateBuiltinRole(br)
		if err == nil {
			m.roles[role.ID] = role
		}
	}
}

// ============================================================================
// 二、角色管理
// ============================================================================

// CreateRole 创建角色
func (m *Manager) CreateRole(role *Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if role.ID == "" {
		return fmt.Errorf("role ID cannot be empty")
	}

	if _, exists := m.roles[role.ID]; exists {
		return fmt.Errorf("role already exists: %s", role.ID)
	}

	if role.IsBuiltinRole() {
		return fmt.Errorf("cannot create builtin role: %s", role.Builtin)
	}

	role.CreatedAt = time.Now()
	role.UpdatedAt = time.Now()
	m.roles[role.ID] = role

	return nil
}

// GetRole 获取角色
func (m *Manager) GetRole(roleID string) *Role {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.roles[roleID]
}

// UpdateRole 更新角色
func (m *Manager) UpdateRole(role *Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.roles[role.ID]
	if !exists {
		return fmt.Errorf("role not found: %s", role.ID)
	}

	if existing.IsBuiltinRole() {
		return fmt.Errorf("cannot modify builtin role: %s", role.ID)
	}

	role.UpdatedAt = time.Now()
	m.roles[role.ID] = role

	// 清除相关缓存
	m.invalidateCacheByRole(role.ID)

	return nil
}

// DeleteRole 删除角色
func (m *Manager) DeleteRole(roleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	role, exists := m.roles[roleID]
	if !exists {
		return fmt.Errorf("role not found: %s", roleID)
	}

	if role.IsBuiltinRole() {
		return fmt.Errorf("cannot delete builtin role: %s", roleID)
	}

	// 检查是否还有绑定
	for _, binding := range m.bindings {
		if binding.RoleID == roleID {
			return fmt.Errorf("role is still bound to users")
		}
	}

	delete(m.roles, roleID)
	m.invalidateCacheByRole(roleID)

	return nil
}

// ListRoles 列出所有角色
func (m *Manager) ListRoles(tenantID string) []*Role {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Role
	for _, role := range m.roles {
		// 全局角色（tenantID 为空）或本租户角色
		if role.TenantID == "" || role.TenantID == tenantID {
			result = append(result, role)
		}
	}
	return result
}

// ============================================================================
// 三、角色绑定管理
// ============================================================================

// BindRole 绑定用户到角色
func (m *Manager) BindRole(userID, roleID, tenantID string, scope DataScope, createdBy string) (*RoleBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.roles[roleID]
	if !exists {
		return nil, fmt.Errorf("role not found: %s", roleID)
	}

	binding := &RoleBinding{
		ID:        fmt.Sprintf("binding-%s-%s", userID, roleID),
		UserID:    userID,
		RoleID:    roleID,
		TenantID:  tenantID,
		Scope:     scope,
		CreatedAt: time.Now(),
		CreatedBy: createdBy,
	}

	m.bindings[binding.ID] = binding
	m.invalidateCache(userID)

	return binding, nil
}

// UnbindRole 解除用户角色绑定
func (m *Manager) UnbindRole(bindingID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	binding, exists := m.bindings[bindingID]
	if !exists {
		return fmt.Errorf("binding not found: %s", bindingID)
	}

	delete(m.bindings, bindingID)
	m.invalidateCache(binding.UserID)

	return nil
}

// UnbindUserRole 解除用户的指定角色
func (m *Manager) UnbindUserRole(userID, roleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, binding := range m.bindings {
		if binding.UserID == userID && binding.RoleID == roleID {
			delete(m.bindings, id)
			m.invalidateCache(userID)
			return nil
		}
	}
	return fmt.Errorf("binding not found for user %s and role %s", userID, roleID)
}

// GetUserBindings 获取用户的角色绑定
func (m *Manager) GetUserBindings(userID string) []*RoleBinding {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*RoleBinding
	for _, binding := range m.bindings {
		if binding.UserID == userID {
			result = append(result, binding)
		}
	}
	return result
}

// GetUserRoles 获取用户的角色列表
func (m *Manager) GetUserRoles(userID string) []*Role {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Role
	seen := make(map[string]bool)
	for _, binding := range m.bindings {
		if binding.UserID == userID {
			if role, ok := m.roles[binding.RoleID]; ok && !seen[role.ID] {
				result = append(result, role)
				seen[role.ID] = true
			}
		}
	}
	return result
}

// ============================================================================
// 四、权限评估
// ============================================================================

// CheckPermission 检查用户是否有权限
func (m *Manager) CheckPermission(userID string, permission Permission) (bool, DataScope) {
	perms := m.GetUserPermissions(userID)
	if perms == nil {
		return false, DataScopeSelf
	}
	return perms.HasPermission(permission), perms.DataScope
}

// CheckPermissionString 检查用户是否有权限（字符串格式）
func (m *Manager) CheckPermissionString(userID string, permissionStr string) (bool, DataScope) {
	p, err := ParsePermission(permissionStr)
	if err != nil {
		return false, DataScopeSelf
	}
	return m.CheckPermission(userID, p)
}

// CheckResourceAccess 检查资源访问权限（数据权限 + 操作权限）
func (m *Manager) CheckResourceAccess(userID string, resourceType ResourceType, action ActionType, resourceTenantID string, ownerID string) (bool, *AccessCheckResult) {
	perms := m.GetUserPermissions(userID)
	if perms == nil {
		return false, &AccessCheckResult{
			Allowed: false,
			Reason:  "user permissions not found",
		}
	}

	// 1. 检查操作权限
	perm := NewPermission(resourceType, action)
	if !perms.HasPermission(perm) {
		return false, &AccessCheckResult{
			Allowed:   false,
			Reason:    fmt.Sprintf("user does not have %s permission", perm),
			DataScope: perms.DataScope,
		}
	}

	// 2. 检查数据权限（租户隔离）
	if !perms.CanAccessResource(resourceTenantID, ownerID) {
		return false, &AccessCheckResult{
			Allowed:   false,
			Reason:    "data scope restriction: cannot access this tenant/resource",
			DataScope: perms.DataScope,
		}
	}

	return true, &AccessCheckResult{
		Allowed:   true,
		DataScope: perms.DataScope,
	}
}

// GetUserPermissions 获取用户的聚合权限（带缓存）
func (m *Manager) GetUserPermissions(userID string) *UserPermissions {
	// 尝试从缓存读取
	m.cacheMu.RLock()
	if cached, ok := m.permCache[userID]; ok {
		m.cacheMu.RUnlock()
		return cached
	}
	m.cacheMu.RUnlock()

	// 计算权限
	m.mu.RLock()
	bindings := make([]*RoleBinding, 0)
	for _, b := range m.bindings {
		if b.UserID == userID {
			bindings = append(bindings, b)
		}
	}
	m.mu.RUnlock()

	if len(bindings) == 0 {
		return nil
	}

	perms := &UserPermissions{
		UserID:      userID,
		Permissions: PermissionSet{},
		RoleIDs:     make([]string, 0, len(bindings)),
		DataScope:   DataScopeSelf,
	}

	// 聚合权限（取最宽的数据范围）
	m.mu.RLock()
	for _, binding := range bindings {
		role, ok := m.roles[binding.RoleID]
		if !ok {
			continue
		}
		perms.RoleIDs = append(perms.RoleIDs, role.ID)

		// 设置租户ID（从 binding 中读取）
		if perms.TenantID == "" && binding.TenantID != "" {
			perms.TenantID = binding.TenantID
		}

		// 合并权限
		for _, p := range role.Permissions {
			perms.Permissions.Add(p)
		}

		// 取最宽的数据范围
		scope := binding.Scope
		if scope == "" {
			scope = role.DataScope
		}
		if scope.Priority() > perms.DataScope.Priority() {
			perms.DataScope = scope
		}
	}
	m.mu.RUnlock()

	// 写入缓存
	m.cacheMu.Lock()
	m.permCache[userID] = perms
	m.cacheMu.Unlock()

	return perms
}

// ============================================================================
// 五、数据策略管理
// ============================================================================

// CreateDataPolicy 创建数据策略
func (m *Manager) CreateDataPolicy(policy *DataPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if policy.ID == "" {
		return fmt.Errorf("policy ID cannot be empty")
	}

	if _, exists := m.policies[policy.ID]; exists {
		return fmt.Errorf("policy already exists: %s", policy.ID)
	}

	m.policies[policy.ID] = policy
	return nil
}

// GetDataPolicy 获取数据策略
func (m *Manager) GetDataPolicy(policyID string) *DataPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.policies[policyID]
}

// DeleteDataPolicy 删除数据策略
func (m *Manager) DeleteDataPolicy(policyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.policies[policyID]; !exists {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	delete(m.policies, policyID)
	return nil
}

// EvaluatePolicy 评估策略
func (m *Manager) EvaluatePolicy(userID string, resourceType ResourceType, resource map[string]interface{}) (*AccessCheckResult, error) {
	perms := m.GetUserPermissions(userID)
	if perms == nil {
		return &AccessCheckResult{Allowed: false, Reason: "no permissions"}, nil
	}

	// 检查数据范围
	resourceTenantID := ""
	if tid, ok := resource["tenant_id"].(string); ok {
		resourceTenantID = tid
	}
	ownerID := ""
	if oid, ok := resource["owner_id"].(string); ok {
		ownerID = oid
	}

	if !perms.CanAccessResource(resourceTenantID, ownerID) {
		return &AccessCheckResult{
			Allowed:   false,
			Reason:    "data scope restriction",
			DataScope: perms.DataScope,
		}, nil
	}

	// 检查策略条件
	m.mu.RLock()
	for _, policy := range m.policies {
		if policy.ResourceType != resourceType {
			continue
		}
		if policy.TenantID != "" && policy.TenantID != resourceTenantID {
			continue
		}

		// 检查用户是否适用
		applicable := false
		for _, uid := range policy.UserIDs {
			if uid == userID {
				applicable = true
				break
			}
		}
		if !applicable {
			for _, rid := range policy.RoleIDs {
				for _, userRole := range perms.RoleIDs {
					if rid == userRole {
						applicable = true
						break
					}
				}
				if applicable {
					break
				}
			}
		}
		if !applicable {
			continue
		}

		// 评估条件
		allMatch := true
		for _, cond := range policy.Conditions {
			match, err := cond.Evaluate(resource)
			if err != nil || !match {
				allMatch = false
				break
			}
		}
		if !allMatch {
			return &AccessCheckResult{
				Allowed:    false,
				Reason:     "policy condition not satisfied",
				DataScope:  policy.DataScope,
				Conditions: policy.Conditions,
			}, nil
		}
	}
	m.mu.RUnlock()

	return &AccessCheckResult{
		Allowed:   true,
		DataScope: perms.DataScope,
	}, nil
}

// ============================================================================
// 六、缓存管理
// ============================================================================

// invalidateCache 清除用户缓存
func (m *Manager) invalidateCache(userID string) {
	m.cacheMu.Lock()
	delete(m.permCache, userID)
	m.cacheMu.Unlock()
}

// invalidateCacheByRole 清除角色的所有缓存
func (m *Manager) invalidateCacheByRole(roleID string) {
	m.cacheMu.Lock()
	for _, binding := range m.bindings {
		if binding.RoleID == roleID {
			delete(m.permCache, binding.UserID)
		}
	}
	m.cacheMu.Unlock()
}

// ClearCache 清除所有缓存
func (m *Manager) ClearCache() {
	m.cacheMu.Lock()
	m.permCache = make(map[string]*UserPermissions)
	m.cacheMu.Unlock()
}

// ============================================================================
// 七、统计与查询
// ============================================================================

// GetStats 获取统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	m.cacheMu.RLock()
	defer m.mu.RUnlock()
	defer m.cacheMu.RUnlock()

	return map[string]interface{}{
		"role_count":      len(m.roles),
		"binding_count":   len(m.bindings),
		"policy_count":    len(m.policies),
		"cache_size":      len(m.permCache),
		"builtin_roles":   len(AllBuiltinRoles),
	}
}

// GetRoleBindings 获取角色的所有绑定
func (m *Manager) GetRoleBindings(roleID string) []*RoleBinding {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*RoleBinding
	for _, binding := range m.bindings {
		if binding.RoleID == roleID {
			result = append(result, binding)
		}
	}
	return result
}

// ============================================================================
// 八、上下文集成
// ============================================================================

// ManagerKey 上下文键
type ManagerKey struct{}

// WithManager 将 RBAC 管理器注入上下文
func WithManager(ctx context.Context, manager *Manager) context.Context {
	return context.WithValue(ctx, ManagerKey{}, manager)
}

// ManagerFromContext 从上下文获取 RBAC 管理器
func ManagerFromContext(ctx context.Context) *Manager {
	if m, ok := ctx.Value(ManagerKey{}).(*Manager); ok {
		return m
	}
	return nil
}
