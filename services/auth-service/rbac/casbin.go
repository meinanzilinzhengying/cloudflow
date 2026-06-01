// Package rbac Casbin RBAC 权限引擎
//
// 权限模型:
//
//	tenant → project → role → policy
//
// Casbin Model (RBAC with domains):
//
//	[request_definition]
//	r = sub, dom, obj, act
//
//	[policy_definition]
//	p = sub, dom, obj, act, eft
//
//	[role_definition]
//	g = _, _, _
//
//	[policy_effect]
//	e = some(where (p.eft == allow)) && !some(where (p.eft == deny))
//
//	[matchers]
//	m = g(r.sub, p.sub, r.dom) && r.dom == p.dom && r.obj == p.obj && r.act == p.act
//
// domain 格式: "tenant_id:project_id"
// sub 格式: "user_id" 或 "role_name"
package rbac

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
)

// ---------------------------------------------------------------------------
// Permission constants
// ---------------------------------------------------------------------------

const (
	PermFlowRead       = "flow:read"
	PermFlowWrite      = "flow:write"
	PermAlertRead      = "alert:read"
	PermAlertWrite     = "alert:write"
	PermTopologyRead   = "topology:read"
	PermTopologyWrite  = "topology:write"
	PermAgentManage    = "agent:manage"
	PermConfigManage   = "config:manage"
	PermTenantManage   = "tenant:manage"
	PermUserManage     = "user:manage"
	PermProjectManage  = "project:manage"
	PermRoleManage     = "role:manage"
	PermPolicyManage   = "policy:manage"
)

// ---------------------------------------------------------------------------
// Casbin model definition
// ---------------------------------------------------------------------------

// rbacModel 是 Casbin 的 RBAC with domains 模型定义。
const rbacModel = `
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act, eft

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = g(r.sub, p.sub, r.dom) && r.dom == p.dom && r.obj == p.obj && r.act == p.act
`

// ---------------------------------------------------------------------------
// Data structures
// ---------------------------------------------------------------------------

// PolicyRule 表示一条 Casbin 策略规则。
type PolicyRule struct {
	Type string // "p" for policy, "g" for grouping
	V0   string
	V1   string
	V2   string
	V3   string
	V4   string
}

// RoleMapping 表示用户-角色-域的映射关系。
type RoleMapping struct {
	User   string
	Role   string
	Domain string
}

// ---------------------------------------------------------------------------
// RBACConfig
// ---------------------------------------------------------------------------

// RBACConfig 是 RBAC 引擎的配置。
type RBACConfig struct {
	// DefaultPoliciesEnabled 是否在启动时加载默认策略
	DefaultPoliciesEnabled bool
	// SuperAdminRole 超级管理员角色名称
	SuperAdminRole string
	// BuiltinRoles 内置角色及其权限列表 (role → permissions)
	BuiltinRoles map[string][]string
}

// defaultRBACConfig 返回默认配置。
func defaultRBACConfig() RBACConfig {
	return RBACConfig{
		DefaultPoliciesEnabled: true,
		SuperAdminRole:         "super_admin",
		BuiltinRoles: map[string][]string{
			"super_admin": {
				PermFlowRead, PermFlowWrite,
				PermAlertRead, PermAlertWrite,
				PermTopologyRead, PermTopologyWrite,
				PermAgentManage, PermConfigManage,
				PermTenantManage, PermUserManage,
				PermProjectManage, PermRoleManage, PermPolicyManage,
			},
			"admin": {
				PermFlowRead, PermFlowWrite,
				PermAlertRead, PermAlertWrite,
				PermTopologyRead, PermTopologyWrite,
				PermAgentManage, PermConfigManage,
				PermUserManage, PermProjectManage,
				PermRoleManage, PermPolicyManage,
			},
			"editor": {
				PermFlowRead, PermFlowWrite,
				PermAlertRead, PermAlertWrite,
				PermTopologyRead,
			},
			"viewer": {
				PermFlowRead,
				PermAlertRead,
				PermTopologyRead,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// MemoryAdapter
// ---------------------------------------------------------------------------

// MemoryAdapter 是一个基于内存的 Casbin 持久化适配器，
// 实现 persist.Adapter 接口。
type MemoryAdapter struct {
	policies     []PolicyRule
	roleMappings []RoleMapping
	mu           sync.RWMutex
}

// NewMemoryAdapter 创建一个新的内存适配器。
func NewMemoryAdapter() *MemoryAdapter {
	return &MemoryAdapter{
		policies:     make([]PolicyRule, 0),
		roleMappings: make([]RoleMapping, 0),
	}
}

// LoadPolicy 将所有策略加载到 Casbin model 中。
func (a *MemoryAdapter) LoadPolicy(m model.Model) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 加载策略规则 (p)
	for _, p := range a.policies {
		if p.Type == "p" {
			sec := "p"
			ptype := "p"
			rule := []string{p.V0, p.V1, p.V2, p.V3, p.V4}
			// 过滤空字符串
			rule = filterEmpty(rule)
			if len(rule) > 0 {
				m.AddPolicy(sec, ptype, rule)
			}
		}
	}

	// 加载角色映射 (g)
	for _, g := range a.roleMappings {
		sec := "g"
		ptype := "g"
		rule := []string{g.User, g.Role, g.Domain}
		rule = filterEmpty(rule)
		if len(rule) > 0 {
			m.AddPolicy(sec, ptype, rule)
		}
	}

	return nil
}

// SavePolicy 将 Casbin model 中的策略保存到内存。
func (a *MemoryAdapter) SavePolicy(m model.Model) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.policies = a.policies[:0]
	a.roleMappings = a.roleMappings[:0]

	// 保存 p 策略
	policy := m.GetPolicy("p", "p")
	for _, rule := range policy {
		pr := PolicyRule{
			Type: "p",
		}
		if len(rule) > 0 {
			pr.V0 = rule[0]
		}
		if len(rule) > 1 {
			pr.V1 = rule[1]
		}
		if len(rule) > 2 {
			pr.V2 = rule[2]
		}
		if len(rule) > 3 {
			pr.V3 = rule[3]
		}
		if len(rule) > 4 {
			pr.V4 = rule[4]
		}
		a.policies = append(a.policies, pr)
	}

	// 保存 g 策略
	grouping := m.GetPolicy("g", "g")
	for _, rule := range grouping {
		rm := RoleMapping{}
		if len(rule) > 0 {
			rm.User = rule[0]
		}
		if len(rule) > 1 {
			rm.Role = rule[1]
		}
		if len(rule) > 2 {
			rm.Domain = rule[2]
		}
		a.roleMappings = append(a.roleMappings, rm)
	}

	return nil
}

// AddPolicy 添加一条策略规则。
func (a *MemoryAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ptype == "p" {
		pr := PolicyRule{Type: "p"}
		if len(rule) > 0 {
			pr.V0 = rule[0]
		}
		if len(rule) > 1 {
			pr.V1 = rule[1]
		}
		if len(rule) > 2 {
			pr.V2 = rule[2]
		}
		if len(rule) > 3 {
			pr.V3 = rule[3]
		}
		if len(rule) > 4 {
			pr.V4 = rule[4]
		}
		a.policies = append(a.policies, pr)
	}

	return nil
}

// RemovePolicy 移除一条策略规则。
func (a *MemoryAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ptype == "p" {
		for i, p := range a.policies {
			if p.Type != "p" {
				continue
			}
			stored := []string{p.V0, p.V1, p.V2, p.V3, p.V4}
			stored = filterEmpty(stored)
			if stringSliceEqual(stored, rule) {
				a.policies = append(a.policies[:i], a.policies[i+1:]...)
				return nil
			}
		}
	}

	return nil
}

// AddGroupingPolicy 添加一条角色映射策略。
func (a *MemoryAdapter) AddGroupingPolicy(rule []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	rm := RoleMapping{}
	if len(rule) > 0 {
		rm.User = rule[0]
	}
	if len(rule) > 1 {
		rm.Role = rule[1]
	}
	if len(rule) > 2 {
		rm.Domain = rule[2]
	}
	a.roleMappings = append(a.roleMappings, rm)

	return nil
}

// RemoveGroupingPolicy 移除一条角色映射策略。
func (a *MemoryAdapter) RemoveGroupingPolicy(rule []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, g := range a.roleMappings {
		stored := []string{g.User, g.Role, g.Domain}
		stored = filterEmpty(stored)
		if stringSliceEqual(stored, rule) {
			a.roleMappings = append(a.roleMappings[:i], a.roleMappings[i+1:]...)
			return nil
		}
	}

	return nil
}

// AddPolicies 批量添加策略规则。
func (a *MemoryAdapter) AddPolicies(sec string, ptype string, rules [][]string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ptype == "p" {
		for _, rule := range rules {
			pr := PolicyRule{Type: "p"}
			if len(rule) > 0 {
				pr.V0 = rule[0]
			}
			if len(rule) > 1 {
				pr.V1 = rule[1]
			}
			if len(rule) > 2 {
				pr.V2 = rule[2]
			}
			if len(rule) > 3 {
				pr.V3 = rule[3]
			}
			if len(rule) > 4 {
				pr.V4 = rule[4]
			}
			a.policies = append(a.policies, pr)
		}
	}

	return nil
}

// RemovePolicies 批量移除策略规则。
func (a *MemoryAdapter) RemovePolicies(sec string, ptype string, rules [][]string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ptype == "p" {
		for _, rule := range rules {
			for i, p := range a.policies {
				if p.Type != "p" {
					continue
				}
				stored := []string{p.V0, p.V1, p.V2, p.V3, p.V4}
				stored = filterEmpty(stored)
				if stringSliceEqual(stored, rule) {
					a.policies = append(a.policies[:i], a.policies[i+1:]...)
					break
				}
			}
		}
	}

	return nil
}

// RemoveFilteredPolicy 移除匹配的字段策略。
func (a *MemoryAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ptype == "p" {
		newPolicies := make([]PolicyRule, 0, len(a.policies))
		for _, p := range a.policies {
			if p.Type != "p" {
				newPolicies = append(newPolicies, p)
				continue
			}
			fields := []string{p.V0, p.V1, p.V2, p.V3, p.V4}
			if !matchesFiltered(fields, fieldIndex, fieldValues) {
				newPolicies = append(newPolicies, p)
			}
		}
		a.policies = newPolicies
	}

	return nil
}

// ---------------------------------------------------------------------------
// RBACEngine
// ---------------------------------------------------------------------------

// RBACEngine 是基于 Casbin 的 RBAC 授权引擎。
type RBACEngine struct {
	enforcer *casbin.SyncedEnforcer
	adapter  persist.Adapter
	config   RBACConfig
	mu       sync.RWMutex
}

// NewRBACEngine 创建一个新的 RBAC 引擎实例（使用内存适配器）。
func NewRBACEngine(config RBACConfig) *RBACEngine {
	if config.SuperAdminRole == "" {
		config.SuperAdminRole = "super_admin"
	}
	if config.BuiltinRoles == nil {
		config.BuiltinRoles = defaultRBACConfig().BuiltinRoles
	}
	return &RBACEngine{
		adapter: NewMemoryAdapter(),
		config:  config,
	}
}

// NewRBACEngineWithAdapter 使用指定的适配器创建 RBAC 引擎实例。
// P0-02 修复: 支持持久化适配器（如 GormAdapter）
func NewRBACEngineWithAdapter(config RBACConfig, adapter persist.Adapter) *RBACEngine {
	if config.SuperAdminRole == "" {
		config.SuperAdminRole = "super_admin"
	}
	if config.BuiltinRoles == nil {
		config.BuiltinRoles = defaultRBACConfig().BuiltinRoles
	}
	return &RBACEngine{
		adapter: adapter,
		config:  config,
	}
}

// Start 初始化 Casbin enforcer，加载模型和适配器。
func (e *RBACEngine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 从字符串加载 Casbin 模型
	m, err := model.NewModelFromString(rbacModel)
	if err != nil {
		return fmt.Errorf("failed to create casbin model: %w", err)
	}

	// 创建 enforcer
	enforcer, err := casbin.NewSyncedEnforcer(m, e.adapter)
	if err != nil {
		return fmt.Errorf("failed to create casbin enforcer: %w", err)
	}

	e.enforcer = enforcer

	// 从适配器加载策略（对于持久化适配器，会从数据库加载）
	if err := e.enforcer.LoadPolicy(); err != nil {
		return fmt.Errorf("failed to load policy: %w", err)
	}

	// 检查是否需要初始化默认策略（仅当策略为空时）
	if e.config.DefaultPoliciesEnabled {
		policies := e.enforcer.GetPolicy()
		if len(policies) == 0 {
			e.AddBuiltinRoles()
			// 保存默认策略到持久化存储
			if err := e.enforcer.SavePolicy(); err != nil {
				return fmt.Errorf("failed to save default policies: %w", err)
			}
		}
	}

	return nil
}

// Stop 停止 RBAC 引擎，释放资源。
func (e *RBACEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enforcer != nil {
		e.enforcer.Stop()
	}
}

// ---------------------------------------------------------------------------
// RBACEngine - Permission check
// ---------------------------------------------------------------------------

// CheckPermission 检查用户是否拥有指定权限。
// domain 格式: "tenant_id:project_id"
func (e *RBACEngine) CheckPermission(userID, tenantID, projectID, resource, action string) (bool, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.enforcer == nil {
		return false, "RBAC engine not initialized"
	}

	domain := tenantID + ":" + projectID
	allowed, err := e.enforcer.Enforce(userID, domain, resource, action)
	if err != nil {
		return false, fmt.Sprintf("enforcement error: %v", err)
	}

	if allowed {
		return true, "permission granted"
	}

	// 尝试提供更详细的原因
	roles, _ := e.enforcer.GetRolesForUserInDomain(userID, domain)
	if len(roles) == 0 {
		return false, fmt.Sprintf("user %q has no roles in domain %q", userID, domain)
	}

	return false, fmt.Sprintf("user %q with roles %v has no permission %q on resource %q in domain %q",
		userID, roles, action, resource, domain)
}

// ---------------------------------------------------------------------------
// RBACEngine - Role management
// ---------------------------------------------------------------------------

// AddRoleForUser 为用户在指定域中添加角色。
func (e *RBACEngine) AddRoleForUser(userID, role, tenantID, projectID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enforcer == nil {
		return fmt.Errorf("RBAC engine not initialized")
	}

	domain := tenantID + ":" + projectID
	_, err := e.enforcer.AddRoleForUserInDomain(userID, role, domain)
	if err != nil {
		return fmt.Errorf("failed to add role %q for user %q in domain %q: %w", role, userID, domain, err)
	}

	return nil
}

// RemoveRoleForUser 移除用户在指定域中的角色。
func (e *RBACEngine) RemoveRoleForUser(userID, role, tenantID, projectID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enforcer == nil {
		return fmt.Errorf("RBAC engine not initialized")
	}

	domain := tenantID + ":" + projectID
	_, err := e.enforcer.DeleteRoleForUserInDomain(userID, role, domain)
	if err != nil {
		return fmt.Errorf("failed to remove role %q for user %q in domain %q: %w", role, userID, domain, err)
	}

	return nil
}

// GetRolesForUser 获取用户在指定域中的所有角色。
func (e *RBACEngine) GetRolesForUser(userID, tenantID, projectID string) ([]string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.enforcer == nil {
		return nil, fmt.Errorf("RBAC engine not initialized")
	}

	domain := tenantID + ":" + projectID
	roles, err := e.enforcer.GetRolesForUserInDomain(userID, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to get roles for user %q in domain %q: %w", userID, domain, err)
	}

	return roles, nil
}

// ---------------------------------------------------------------------------
// RBACEngine - Policy management
// ---------------------------------------------------------------------------

// AddPolicy 为角色在指定域中添加策略。
func (e *RBACEngine) AddPolicy(role, tenantID, projectID, resource, action, effect string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enforcer == nil {
		return fmt.Errorf("RBAC engine not initialized")
	}

	domain := tenantID + ":" + projectID
	_, err := e.enforcer.AddPolicy(role, domain, resource, action, effect)
	if err != nil {
		return fmt.Errorf("failed to add policy for role %q in domain %q: %w", role, domain, err)
	}

	return nil
}

// RemovePolicy 移除角色在指定域中的策略。
func (e *RBACEngine) RemovePolicy(role, tenantID, projectID, resource, action string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enforcer == nil {
		return fmt.Errorf("RBAC engine not initialized")
	}

	domain := tenantID + ":" + projectID
	_, err := e.enforcer.RemovePolicy(role, domain, resource, action)
	if err != nil {
		return fmt.Errorf("failed to remove policy for role %q in domain %q: %w", role, domain, err)
	}

	return nil
}

// GetPoliciesForRole 获取角色在指定域中的所有策略。
func (e *RBACEngine) GetPoliciesForRole(role, tenantID, projectID string) [][]string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.enforcer == nil {
		return nil
	}

	domain := tenantID + ":" + projectID
	return e.enforcer.GetFilteredPolicy(0, role, domain)
}

// ---------------------------------------------------------------------------
// RBACEngine - Built-in roles & tenant management
// ---------------------------------------------------------------------------

// AddBuiltinRoles 添加默认内置角色及其策略。
// 内置角色: super_admin, admin, editor, viewer
func (e *RBACEngine) AddBuiltinRoles() {
	if e.enforcer == nil {
		return
	}

	// 使用通配符域 "*" 表示平台级别
	platformDomain := "*"

	for roleName, permissions := range e.config.BuiltinRoles {
		for _, perm := range permissions {
			// 解析权限 "resource:action"
			parts := strings.SplitN(perm, ":", 2)
			resource := parts[0]
			action := ""
			if len(parts) > 1 {
				action = parts[1]
			}

			// super_admin 使用平台级别域 "*"
			domain := platformDomain
			if roleName != e.config.SuperAdminRole {
				// 其他内置角色也先注册到平台级别，后续通过 AddTenantDefaultPolicies 绑定到具体租户
				domain = platformDomain
			}

			_, _ = e.enforcer.AddPolicy(roleName, domain, resource, action, "allow")
		}
	}
}

// AddTenantDefaultPolicies 为新租户添加默认策略。
// 会为 admin, editor, viewer 角色在租户域下创建策略。
func (e *RBACEngine) AddTenantDefaultPolicies(tenantID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enforcer == nil {
		return
	}

	// 为每个非 super_admin 的内置角色在租户的默认项目下创建策略
	defaultProjectID := "default"
	domain := tenantID + ":" + defaultProjectID

	for roleName, permissions := range e.config.BuiltinRoles {
		if roleName == e.config.SuperAdminRole {
			continue // super_admin 使用平台级别策略
		}

		for _, perm := range permissions {
			parts := strings.SplitN(perm, ":", 2)
			resource := parts[0]
			action := ""
			if len(parts) > 1 {
				action = parts[1]
			}

			_, _ = e.enforcer.AddPolicy(roleName, domain, resource, action, "allow")
		}
	}
}

// DeleteTenant 删除租户的所有策略和角色映射。
func (e *RBACEngine) DeleteTenant(tenantID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enforcer == nil {
		return
	}

	// 删除所有匹配该租户的策略 (p 策略中 domain 在 V1 位置)
	// 遍历所有策略，移除 domain 前缀为 tenantID: 的策略
	policies := e.enforcer.GetPolicy()
	for _, policy := range policies {
		if len(policy) > 1 && strings.HasPrefix(policy[1], tenantID+":") {
			_, _ = e.enforcer.RemovePolicy(policy...)
		}
	}

	// 删除所有匹配该租户的角色映射 (g 策略中 domain 在 V2 位置)
	groupingPolicies := e.enforcer.GetGroupingPolicy()
	for _, gp := range groupingPolicies {
		if len(gp) > 2 && strings.HasPrefix(gp[2], tenantID+":") {
			_, _ = e.enforcer.RemoveGroupingPolicy(gp...)
		}
	}
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// filterEmpty 过滤字符串切片中的空字符串。
func filterEmpty(s []string) []string {
	result := make([]string, 0, len(s))
	for _, v := range s {
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

// stringSliceEqual 比较两个字符串切片是否相等。
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// matchesFiltered 检查 fields 是否匹配给定的过滤条件。
func matchesFiltered(fields []string, fieldIndex int, fieldValues []string) bool {
	for i, fv := range fieldValues {
		idx := fieldIndex + i
		if idx >= len(fields) {
			return false
		}
		if fields[idx] != fv {
			return false
		}
	}
	return true
}

// Compile-time interface compliance check.
var _ persist.Adapter = (*MemoryAdapter)(nil)
