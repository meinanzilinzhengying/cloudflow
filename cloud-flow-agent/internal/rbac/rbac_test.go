//go:build linux

package rbac_test

import (
	"testing"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/rbac"
)

// ============================================================================
// 一、权限测试
// ============================================================================

func TestPermission(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		p := rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)
		if s := p.String(); s != "asset:view" {
			t.Errorf("expected asset:view, got %s", s)
		}
	})

	t.Run("Parse valid", func(t *testing.T) {
		p, err := rbac.ParsePermission("asset:edit")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Resource != rbac.ResourceAsset || p.Action != rbac.ActionEdit {
			t.Errorf("unexpected parse result: %v", p)
		}
	})

	t.Run("Parse invalid", func(t *testing.T) {
		_, err := rbac.ParsePermission("invalid")
		if err == nil {
			t.Error("expected error for invalid format")
		}
	})

	t.Run("Parse invalid resource", func(t *testing.T) {
		_, err := rbac.ParsePermission("invalid:view")
		if err == nil {
			t.Error("expected error for invalid resource")
		}
	})

	t.Run("Match wildcard", func(t *testing.T) {
		p := rbac.Permission{Resource: "*", Action: "*"}
		if !p.Match(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)) {
			t.Error("expected wildcard to match")
		}
	})

	t.Run("Match specific", func(t *testing.T) {
		p := rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)
		if !p.Match(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)) {
			t.Error("expected match")
		}
		if p.Match(rbac.NewPermission(rbac.ResourceAlert, rbac.ActionView)) {
			t.Error("expected no match for different resource")
		}
	})
}

func TestPermissionSet(t *testing.T) {
	t.Run("Contains", func(t *testing.T) {
		ps := rbac.PermissionSet{
			rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView),
			rbac.NewPermission(rbac.ResourceAsset, rbac.ActionEdit),
		}
		if !ps.Contains(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)) {
			t.Error("expected to contain asset:view")
		}
		if ps.Contains(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionDelete)) {
			t.Error("expected not to contain asset:delete")
		}
	})

	t.Run("Contains wildcard", func(t *testing.T) {
		ps := rbac.PermissionSet{rbac.WildcardPermission}
		if !ps.Contains(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionDelete)) {
			t.Error("expected wildcard to contain any permission")
		}
	})

	t.Run("Add", func(t *testing.T) {
		var ps rbac.PermissionSet
		ps.Add(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView))
		if !ps.Contains(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)) {
			t.Error("expected to contain after add")
		}
	})

	t.Run("Remove", func(t *testing.T) {
		ps := rbac.PermissionSet{
			rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView),
		}
		ps.Remove(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView))
		if ps.Contains(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)) {
			t.Error("expected not to contain after remove")
		}
	})

	t.Run("ToStrings", func(t *testing.T) {
		ps := rbac.PermissionSet{
			rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView),
		}
		strs := ps.ToStrings()
		if len(strs) != 1 || strs[0] != "asset:view" {
			t.Errorf("unexpected strings: %v", strs)
		}
	})
}

// ============================================================================
// 二、角色测试
// ============================================================================

func TestRole(t *testing.T) {
	t.Run("Super admin has all permissions", func(t *testing.T) {
		role := rbac.NewSuperAdminRole()
		if !role.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionDelete)) {
			t.Error("super admin should have delete permission")
		}
		if !role.HasPermission(rbac.NewPermission(rbac.ResourcePolicy, rbac.ActionManage)) {
			t.Error("super admin should have manage permission")
		}
	})

	t.Run("Admin cannot delete tenant", func(t *testing.T) {
		role := rbac.NewAdminRole()
		if role.HasPermission(rbac.NewPermission(rbac.ResourceTenant, rbac.ActionDelete)) {
			t.Error("admin should not have tenant delete permission")
		}
		if !role.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)) {
			t.Error("admin should have view permission")
		}
	})

	t.Run("Viewer has read only", func(t *testing.T) {
		role := rbac.NewViewerRole()
		if !role.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)) {
			t.Error("viewer should have view")
		}
		if role.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionEdit)) {
			t.Error("viewer should not have edit")
		}
	})

	t.Run("Operator has execute", func(t *testing.T) {
		role := rbac.NewOperatorRole()
		if !role.HasPermission(rbac.NewPermission(rbac.ResourceProbe, rbac.ActionExecute)) {
			t.Error("operator should have execute")
		}
		if role.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionDelete)) {
			t.Error("operator should not have delete")
		}
	})

	t.Run("Auditor has export", func(t *testing.T) {
		role := rbac.NewAuditorRole()
		if !role.HasPermission(rbac.NewPermission(rbac.ResourceLog, rbac.ActionExport)) {
			t.Error("auditor should have export")
		}
		if role.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionEdit)) {
			t.Error("auditor should not have edit")
		}
	})

	t.Run("Is builtin role", func(t *testing.T) {
		role := rbac.NewAdminRole()
		if !role.IsBuiltinRole() {
			t.Error("expected builtin role")
		}
	})

	t.Run("Add permission", func(t *testing.T) {
		role := rbac.NewViewerRole()
		role.AddPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionCreate))
		if !role.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionCreate)) {
			t.Error("expected to have added permission")
		}
	})

	t.Run("Remove permission", func(t *testing.T) {
		role := rbac.NewEditorRole()
		role.RemovePermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionEdit))
		if role.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionEdit)) {
			t.Error("expected not to have removed permission")
		}
	})
}

func TestBuiltinRoleTemplates(t *testing.T) {
	for _, br := range rbac.AllBuiltinRoles {
		role, err := rbac.CreateBuiltinRole(br)
		if err != nil {
			t.Errorf("failed to create builtin role %s: %v", br, err)
			continue
		}
		if role == nil {
			t.Errorf("builtin role %s is nil", br)
		}
	}
}

// ============================================================================
// 三、数据范围测试
// ============================================================================

func TestDataScope(t *testing.T) {
	t.Run("Priority", func(t *testing.T) {
		if rbac.DataScopeGlobal.Priority() <= rbac.DataScopeTenant.Priority() {
			t.Error("global should have higher priority than tenant")
		}
		if rbac.DataScopeTenant.Priority() <= rbac.DataScopeSelf.Priority() {
			t.Error("tenant should have higher priority than self")
		}
	})

	t.Run("WiderThan", func(t *testing.T) {
		if !rbac.DataScopeGlobal.WiderThan(rbac.DataScopeTenant) {
			t.Error("global should be wider than tenant")
		}
		if rbac.DataScopeSelf.WiderThan(rbac.DataScopeTenant) {
			t.Error("self should not be wider than tenant")
		}
	})

	t.Run("Validate", func(t *testing.T) {
		if !rbac.DataScopeGlobal.Validate() {
			t.Error("global should be valid")
		}
		if rbac.DataScope("invalid").Validate() {
			t.Error("invalid should not be valid")
		}
	})
}

func TestDataAuthContext(t *testing.T) {
	t.Run("Global access", func(t *testing.T) {
		ctx := rbac.NewDataAuthContext("user-1", "tenant-1", rbac.DataScopeGlobal)
		if !ctx.CanAccess("tenant-2", "user-2", "res-1") {
			t.Error("global should access any resource")
		}
	})

	t.Run("Tenant access", func(t *testing.T) {
		ctx := rbac.NewDataAuthContext("user-1", "tenant-1", rbac.DataScopeTenant)
		if !ctx.CanAccess("tenant-1", "user-2", "res-1") {
			t.Error("should access same tenant")
		}
		if ctx.CanAccess("tenant-2", "user-2", "res-1") {
			t.Error("should not access different tenant")
		}
	})

	t.Run("Self access", func(t *testing.T) {
		ctx := rbac.NewDataAuthContext("user-1", "tenant-1", rbac.DataScopeSelf)
		if !ctx.CanAccess("tenant-1", "user-1", "res-1") {
			t.Error("should access own resource")
		}
		if ctx.CanAccess("tenant-1", "user-2", "res-1") {
			t.Error("should not access other's resource")
		}
	})

	t.Run("Assigned access", func(t *testing.T) {
		ctx := rbac.NewDataAuthContext("user-1", "tenant-1", rbac.DataScopeAssigned)
		ctx.AssignedIDs = []string{"res-1", "res-2"}
		if !ctx.CanAccess("tenant-1", "user-2", "res-1") {
			t.Error("should access assigned resource")
		}
		if ctx.CanAccess("tenant-1", "user-2", "res-3") {
			t.Error("should not access unassigned resource")
		}
	})

	t.Run("GetFilter global", func(t *testing.T) {
		ctx := rbac.NewDataAuthContext("user-1", "tenant-1", rbac.DataScopeGlobal)
		filter := ctx.GetFilter()
		if len(filter) != 0 {
			t.Errorf("expected empty filter, got %v", filter)
		}
	})

	t.Run("GetFilter tenant", func(t *testing.T) {
		ctx := rbac.NewDataAuthContext("user-1", "tenant-1", rbac.DataScopeTenant)
		filter := ctx.GetFilter()
		if filter["tenant_id"] != "tenant-1" {
			t.Errorf("expected tenant filter, got %v", filter)
		}
	})
}

func TestPolicyCondition(t *testing.T) {
	t.Run("EQ condition", func(t *testing.T) {
		cond := rbac.PolicyCondition{Field: "status", Operator: "eq", Value: "active"}
		res := map[string]interface{}{"status": "active"}
		ok, err := cond.Evaluate(res)
		if err != nil || !ok {
			t.Errorf("expected match, got ok=%v, err=%v", ok, err)
		}
	})

	t.Run("NE condition", func(t *testing.T) {
		cond := rbac.PolicyCondition{Field: "status", Operator: "ne", Value: "deleted"}
		res := map[string]interface{}{"status": "active"}
		ok, err := cond.Evaluate(res)
		if err != nil || !ok {
			t.Errorf("expected match, got ok=%v, err=%v", ok, err)
		}
	})

	t.Run("IN condition", func(t *testing.T) {
		cond := rbac.PolicyCondition{Field: "role", Operator: "in", Value: []string{"admin", "editor"}}
		res := map[string]interface{}{"role": "admin"}
		ok, err := cond.Evaluate(res)
		if err != nil || !ok {
			t.Errorf("expected match, got ok=%v, err=%v", ok, err)
		}
	})

	t.Run("Missing field", func(t *testing.T) {
		cond := rbac.PolicyCondition{Field: "missing", Operator: "eq", Value: "x"}
		res := map[string]interface{}{"status": "active"}
		ok, err := cond.Evaluate(res)
		if err != nil || ok {
			t.Errorf("expected no match for missing field, got ok=%v", ok)
		}
	})
}

// ============================================================================
// 四、操作矩阵测试
// ============================================================================

func TestOperationMatrix(t *testing.T) {
	om := rbac.NewOperationMatrix(rbac.ResourceAsset)
	om.SetAction(rbac.ActionView, true)
	om.SetAction(rbac.ActionCreate, true)
	om.SetAction(rbac.ActionDelete, false)

	if !om.IsAllowed(rbac.ActionView) {
		t.Error("expected view allowed")
	}
	if om.IsAllowed(rbac.ActionDelete) {
		t.Error("expected delete not allowed")
	}
	if om.IsAllowed(rbac.ActionEdit) {
		t.Error("expected edit not set (default false)")
	}

	allowed := om.GetAllowedActions()
	if len(allowed) != 2 {
		t.Errorf("expected 2 allowed actions, got %d", len(allowed))
	}
}

// ============================================================================
// 五、RBAC 管理器测试
// ============================================================================

func TestManager(t *testing.T) {
	m := rbac.NewManager()

	t.Run("Init builtin roles", func(t *testing.T) {
		roles := m.ListRoles("")
		if len(roles) < len(rbac.AllBuiltinRoles) {
			t.Errorf("expected at least %d builtin roles, got %d", len(rbac.AllBuiltinRoles), len(roles))
		}
	})

	t.Run("Create custom role", func(t *testing.T) {
		role := &rbac.Role{
			ID:          "role-custom-1",
			Name:        "自定义角色",
			Description: "测试角色",
			Permissions: rbac.PermissionSet{
				rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView),
			},
			DataScope: rbac.DataScopeTenant,
			TenantID:  "tenant-1",
		}
		if err := m.CreateRole(role); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := m.GetRole("role-custom-1")
		if got == nil {
			t.Fatal("expected role to exist")
		}
		if got.Name != "自定义角色" {
			t.Errorf("unexpected name: %s", got.Name)
		}
	})

	t.Run("Create role duplicate", func(t *testing.T) {
		role := &rbac.Role{
			ID: "role-custom-1",
		}
		if err := m.CreateRole(role); err == nil {
			t.Error("expected error for duplicate role")
		}
	})

	t.Run("Create builtin role rejected", func(t *testing.T) {
		role := &rbac.Role{
			ID:      "role-custom-builtin",
			Builtin: rbac.RoleAdmin,
		}
		if err := m.CreateRole(role); err == nil {
			t.Error("expected error for creating builtin role")
		}
	})

	t.Run("Delete role", func(t *testing.T) {
		if err := m.DeleteRole("role-custom-1"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m.GetRole("role-custom-1") != nil {
			t.Error("expected role to be deleted")
		}
	})

	t.Run("Delete builtin role rejected", func(t *testing.T) {
		if err := m.DeleteRole("role-super-admin"); err == nil {
			t.Error("expected error for deleting builtin role")
		}
	})

	t.Run("List roles by tenant", func(t *testing.T) {
		// 创建租户角色
		role := &rbac.Role{
			ID:        "role-tenant-1",
			Name:      "租户角色",
			TenantID:  "tenant-1",
			DataScope: rbac.DataScopeTenant,
		}
		m.CreateRole(role)

		roles := m.ListRoles("tenant-1")
		found := false
		for _, r := range roles {
			if r.ID == "role-tenant-1" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find tenant role")
		}
	})
}

func TestManagerPermissions(t *testing.T) {
	m := rbac.NewManager()

	// 创建用户-角色绑定
	m.BindRole("user-1", "role-viewer", "tenant-1", rbac.DataScopeTenant, "admin")
	m.BindRole("user-1", "role-operator", "tenant-1", rbac.DataScopeTenant, "admin")

	t.Run("Check permission", func(t *testing.T) {
		allowed, scope := m.CheckPermission("user-1", rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView))
		if !allowed {
			t.Error("expected user-1 to have view permission")
		}
		if scope != rbac.DataScopeTenant {
			t.Errorf("expected tenant scope, got %s", scope)
		}
	})

	t.Run("Check permission string", func(t *testing.T) {
		allowed, _ := m.CheckPermissionString("user-1", "asset:view")
		if !allowed {
			t.Error("expected user-1 to have asset:view")
		}
	})

	t.Run("Check permission not found", func(t *testing.T) {
		allowed, _ := m.CheckPermission("user-1", rbac.NewPermission(rbac.ResourcePolicy, rbac.ActionManage))
		if allowed {
			t.Error("expected no policy manage permission")
		}
	})

	t.Run("Check permission no user", func(t *testing.T) {
		allowed, _ := m.CheckPermission("user-nonexist", rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView))
		if allowed {
			t.Error("expected no permission for non-existent user")
		}
	})

	t.Run("Get user permissions", func(t *testing.T) {
		perms := m.GetUserPermissions("user-1")
		if perms == nil {
			t.Fatal("expected permissions for user-1")
		}
		if len(perms.RoleIDs) != 2 {
			t.Errorf("expected 2 roles, got %d", len(perms.RoleIDs))
		}
		if !perms.HasPermission(rbac.NewPermission(rbac.ResourceAlert, rbac.ActionExecute)) {
			t.Error("expected operator execute permission")
		}
	})

	t.Run("User permissions can access tenant", func(t *testing.T) {
		perms := m.GetUserPermissions("user-1")
		if !perms.CanAccessTenant("tenant-1") {
			t.Error("expected to access tenant-1")
		}
	})

	t.Run("User permissions cannot access other tenant", func(t *testing.T) {
		perms := m.GetUserPermissions("user-1")
		if perms.CanAccessTenant("tenant-2") {
			t.Error("expected not to access tenant-2")
		}
	})
}

func TestManagerBindings(t *testing.T) {
	m := rbac.NewManager()

	t.Run("Bind role", func(t *testing.T) {
		binding, err := m.BindRole("user-1", "role-viewer", "tenant-1", rbac.DataScopeTenant, "admin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if binding.UserID != "user-1" || binding.RoleID != "role-viewer" {
			t.Errorf("unexpected binding: %v", binding)
		}
	})

	t.Run("Bind non-existent role", func(t *testing.T) {
	_, err := m.BindRole("user-1", "role-nonexist", "tenant-1", rbac.DataScopeTenant, "admin")
		if err == nil {
			t.Error("expected error for non-existent role")
		}
	})

	t.Run("Get user bindings", func(t *testing.T) {
		bindings := m.GetUserBindings("user-1")
		if len(bindings) != 1 {
			t.Errorf("expected 1 binding, got %d", len(bindings))
		}
	})

	t.Run("Get user roles", func(t *testing.T) {
		roles := m.GetUserRoles("user-1")
		if len(roles) != 1 || roles[0].ID != "role-viewer" {
			t.Errorf("unexpected roles: %v", roles)
		}
	})

	t.Run("Unbind role", func(t *testing.T) {
		bindings := m.GetUserBindings("user-1")
		if len(bindings) == 0 {
			t.Fatal("no bindings to unbind")
		}
		if err := m.UnbindRole(bindings[0].ID); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(m.GetUserBindings("user-1")) != 0 {
			t.Error("expected no bindings after unbind")
		}
	})

	t.Run("Unbind user role", func(t *testing.T) {
		m.BindRole("user-2", "role-viewer", "tenant-1", rbac.DataScopeTenant, "admin")
		if err := m.UnbindUserRole("user-2", "role-viewer"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(m.GetUserBindings("user-2")) != 0 {
			t.Error("expected no bindings after unbind")
		}
	})
}

func TestManagerResourceAccess(t *testing.T) {
	m := rbac.NewManager()
	m.BindRole("user-1", "role-admin", "tenant-1", rbac.DataScopeTenant, "admin")

	t.Run("Access own tenant resource", func(t *testing.T) {
		allowed, result := m.CheckResourceAccess("user-1", rbac.ResourceAsset, rbac.ActionView, "tenant-1", "user-1")
		if !allowed {
			t.Errorf("expected allowed, got reason: %s", result.Reason)
		}
	})

	t.Run("Access other tenant resource denied", func(t *testing.T) {
		allowed, result := m.CheckResourceAccess("user-1", rbac.ResourceAsset, rbac.ActionView, "tenant-2", "user-2")
		if allowed {
			t.Error("expected denied for different tenant")
		}
		if result.Reason == "" {
			t.Error("expected reason for denial")
		}
	})

	t.Run("Action denied", func(t *testing.T) {
		// admin role does not have tenant delete
		allowed, result := m.CheckResourceAccess("user-1", rbac.ResourceTenant, rbac.ActionDelete, "tenant-1", "user-1")
		if allowed {
			t.Error("expected denied for tenant delete")
		}
		if result.Reason == "" {
			t.Error("expected reason for denial")
		}
	})
}

func TestManagerDataPolicy(t *testing.T) {
	m := rbac.NewManager()
	m.BindRole("user-1", "role-viewer", "tenant-1", rbac.DataScopeTenant, "admin")

	t.Run("Create data policy", func(t *testing.T) {
		policy := &rbac.DataPolicy{
			ID:           "policy-1",
			Name:         "测试策略",
			ResourceType: rbac.ResourceAsset,
			DataScope:    rbac.DataScopeTenant,
			RoleIDs:      []string{"role-viewer"},
		}
		if err := m.CreateDataPolicy(policy); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.GetDataPolicy("policy-1") == nil {
			t.Error("expected policy to exist")
		}
	})

	t.Run("Evaluate policy allowed", func(t *testing.T) {
		resource := map[string]interface{}{
			"tenant_id": "tenant-1",
			"owner_id":  "user-1",
			"status":    "active",
		}
		result, err := m.EvaluatePolicy("user-1", rbac.ResourceAsset, resource)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Errorf("expected allowed, got reason: %s", result.Reason)
		}
	})

	t.Run("Evaluate policy tenant denied", func(t *testing.T) {
		resource := map[string]interface{}{
			"tenant_id": "tenant-2",
			"owner_id":  "user-2",
		}
		result, err := m.EvaluatePolicy("user-1", rbac.ResourceAsset, resource)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("expected denied for different tenant")
		}
	})

	t.Run("Delete data policy", func(t *testing.T) {
		if err := m.DeleteDataPolicy("policy-1"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m.GetDataPolicy("policy-1") != nil {
			t.Error("expected policy to be deleted")
		}
	})
}

func TestManagerCache(t *testing.T) {
	m := rbac.NewManager()
	m.BindRole("user-1", "role-viewer", "tenant-1", rbac.DataScopeTenant, "admin")

	// 首次查询，缓存
	perms1 := m.GetUserPermissions("user-1")
	perms2 := m.GetUserPermissions("user-1")

	if perms1 == nil || perms2 == nil {
		t.Fatal("expected permissions")
	}
	// 缓存应返回相同对象（简化验证）
	if perms1.DataScope != perms2.DataScope {
		t.Error("expected same data scope from cache")
	}

	// 清除缓存
	m.ClearCache()
	perms3 := m.GetUserPermissions("user-1")
	if perms3 == nil {
		t.Fatal("expected permissions after cache clear")
	}
}

func TestManagerStats(t *testing.T) {
	m := rbac.NewManager()
	stats := m.GetStats()
	if stats["role_count"] == nil {
		t.Error("expected role_count in stats")
	}
	if stats["builtin_roles"] != len(rbac.AllBuiltinRoles) {
		t.Errorf("expected %d builtin roles, got %v", len(rbac.AllBuiltinRoles), stats["builtin_roles"])
	}
}

// ============================================================================
// 六、角色绑定测试
// ============================================================================

func TestRoleBinding(t *testing.T) {
	binding := &rbac.RoleBinding{
		ID:     "binding-1",
		UserID: "user-1",
		RoleID: "role-1",
		Scope:  rbac.DataScopeTenant,
	}
	if binding.UserID != "user-1" {
		t.Errorf("unexpected user ID: %s", binding.UserID)
	}
	if binding.Scope != rbac.DataScopeTenant {
		t.Errorf("unexpected scope: %s", binding.Scope)
	}
}

// ============================================================================
// 七、用户权限聚合测试
// ============================================================================

func TestUserPermissions(t *testing.T) {
	up := &rbac.UserPermissions{
		UserID:   "user-1",
		TenantID: "tenant-1",
		DataScope: rbac.DataScopeTenant,
		Permissions: rbac.PermissionSet{
			rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView),
			rbac.NewPermission(rbac.ResourceAsset, rbac.ActionEdit),
		},
		RoleIDs: []string{"role-1"},
	}

	if !up.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionView)) {
		t.Error("expected to have view permission")
	}
	if up.HasPermission(rbac.NewPermission(rbac.ResourceAsset, rbac.ActionDelete)) {
		t.Error("expected not to have delete permission")
	}
	if !up.CanAccessTenant("tenant-1") {
		t.Error("expected to access tenant-1")
	}
	if up.CanAccessTenant("tenant-2") {
		t.Error("expected not to access tenant-2")
	}
	if !up.CanAccessResource("tenant-1", "user-2") {
		t.Error("expected to access same tenant resource")
	}
}

// ============================================================================
// 八、字段掩码测试
// ============================================================================

func TestFieldMask(t *testing.T) {
	t.Run("Full mask", func(t *testing.T) {
		fm := &rbac.FieldMask{Field: "password", MaskType: "full"}
		if fm.MaskValue("secret123") != "****" {
			t.Error("expected full mask")
		}
	})

	t.Run("Partial mask", func(t *testing.T) {
		fm := &rbac.FieldMask{Field: "phone", MaskType: "partial"}
		masked := fm.MaskValue("13812345678")
		if masked != "13****78" {
			t.Errorf("expected partial mask, got %s", masked)
		}
	})

	t.Run("Hash mask", func(t *testing.T) {
		fm := &rbac.FieldMask{Field: "token", MaskType: "hash"}
		if fm.MaskValue("abc123") != "[HASH]" {
			t.Error("expected hash mask")
		}
	})
}

// ============================================================================
// 九、快捷权限集合测试
// ============================================================================

func TestPermissionShortcuts(t *testing.T) {
	t.Run("Full permissions", func(t *testing.T) {
		ps := rbac.FullPermissions()
		if len(ps) != len(rbac.AllResourceTypes)*len(rbac.AllActionTypes) {
			t.Errorf("expected %d permissions, got %d", len(rbac.AllResourceTypes)*len(rbac.AllActionTypes), len(ps))
		}
	})

	t.Run("Read only permissions", func(t *testing.T) {
		ps := rbac.ReadOnlyPermissions()
		for _, p := range ps {
			if p.Action != rbac.ActionView && p.Action != rbac.ActionExport {
				t.Errorf("expected only view/export, got %s", p.Action)
			}
		}
	})
}
