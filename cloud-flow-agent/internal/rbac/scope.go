//go:build linux

package rbac

import (
	"fmt"
)

// ============================================================================
// 一、数据范围定义
// ============================================================================

// DataScope 数据权限范围
type DataScope string

const (
	DataScopeGlobal   DataScope = "global"   // 全局：所有数据
	DataScopeTenant   DataScope = "tenant"   // 租户：本租户数据
	DataScopeSelf     DataScope = "self"     // 个人：仅自己的数据
	DataScopeAssigned DataScope = "assigned" // 指派：分配给该用户的数据
)

// String 返回字符串
func (d DataScope) String() string { return string(d) }

// AllDataScopes 所有数据范围
var AllDataScopes = []DataScope{
	DataScopeGlobal, DataScopeTenant, DataScopeSelf, DataScopeAssigned,
}

// Validate 验证数据范围
func (d DataScope) Validate() bool {
	for _, s := range AllDataScopes {
		if s == d {
			return true
		}
	}
	return false
}

// Priority 数据范围优先级（数值越大权限越广）
func (d DataScope) Priority() int {
	switch d {
	case DataScopeGlobal:
		return 4
	case DataScopeTenant:
		return 3
	case DataScopeAssigned:
		return 2
	case DataScopeSelf:
		return 1
	default:
		return 0
	}
}

// WiderThan 是否比另一个范围更宽
func (d DataScope) WiderThan(other DataScope) bool {
	return d.Priority() > other.Priority()
}

// ============================================================================
// 二、数据权限策略
// ============================================================================

// DataPolicy 数据权限策略
type DataPolicy struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`

	// 作用范围
	ResourceType ResourceType `json:"resource_type"` // 适用的资源类型
	DataScope    DataScope    `json:"data_scope"`  // 数据范围

	// 过滤条件（可选）
	Conditions []PolicyCondition `json:"conditions,omitempty"`

	// 作用对象
	RoleIDs  []string `json:"role_ids,omitempty"`  // 绑定角色
	UserIDs  []string `json:"user_ids,omitempty"`  // 绑定用户
	TenantID string   `json:"tenant_id,omitempty"` // 绑定租户
}

// PolicyCondition 策略条件
type PolicyCondition struct {
	Field    string      `json:"field"`    // 字段名
	Operator string      `json:"operator"` // 操作符：eq, ne, gt, lt, in, contains
	Value    interface{} `json:"value"`    // 值
}

// Evaluate 评估条件
func (pc PolicyCondition) Evaluate(resource map[string]interface{}) (bool, error) {
	val, ok := resource[pc.Field]
	if !ok {
		return false, nil
	}

	switch pc.Operator {
	case "eq":
		return fmt.Sprintf("%v", val) == fmt.Sprintf("%v", pc.Value), nil
	case "ne":
		return fmt.Sprintf("%v", val) != fmt.Sprintf("%v", pc.Value), nil
	case "in":
		if arr, ok := pc.Value.([]string); ok {
			strVal := fmt.Sprintf("%v", val)
			for _, item := range arr {
				if item == strVal {
					return true, nil
				}
			}
		}
		return false, nil
	case "contains":
		strVal := fmt.Sprintf("%v", val)
		strNeedle := fmt.Sprintf("%v", pc.Value)
		return contains(strVal, strNeedle), nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", pc.Operator)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// 三、资源访问控制检查
// ============================================================================

// AccessCheckResult 访问检查结果
type AccessCheckResult struct {
	Allowed     bool     `json:"allowed"`
	Reason      string   `json:"reason,omitempty"`
	DataScope   DataScope `json:"data_scope,omitempty"`
	Conditions  []PolicyCondition `json:"conditions,omitempty"`
}

// AccessChecker 访问检查器接口
type AccessChecker interface {
	CheckAccess(userID string, resourceType ResourceType, action ActionType, resource map[string]interface{}) (*AccessCheckResult, error)
}

// ============================================================================
// 四、数据权限上下文
// ============================================================================

// DataAuthContext 数据权限上下文
type DataAuthContext struct {
	UserID        string    `json:"user_id"`
	TenantID      string    `json:"tenant_id"`
	DataScope     DataScope `json:"data_scope"`
	AssignedIDs   []string  `json:"assigned_ids,omitempty"` // 指派的资源ID列表
}

// NewDataAuthContext 创建数据权限上下文
func NewDataAuthContext(userID, tenantID string, scope DataScope) *DataAuthContext {
	return &DataAuthContext{
		UserID:    userID,
		TenantID:  tenantID,
		DataScope: scope,
	}
}

// CanAccess 检查是否能访问资源
func (ctx *DataAuthContext) CanAccess(resourceTenantID, ownerID string, resourceID string) bool {
	switch ctx.DataScope {
	case DataScopeGlobal:
		return true
	case DataScopeTenant:
		return ctx.TenantID == resourceTenantID
	case DataScopeSelf:
		return ctx.UserID == ownerID
	case DataScopeAssigned:
		if ctx.TenantID != resourceTenantID {
			return false
		}
		for _, id := range ctx.AssignedIDs {
			if id == resourceID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// GetFilter 获取数据过滤条件（用于数据库查询）
func (ctx *DataAuthContext) GetFilter() map[string]interface{} {
	filter := make(map[string]interface{})
	switch ctx.DataScope {
	case DataScopeGlobal:
		// 无限制
	case DataScopeTenant:
		filter["tenant_id"] = ctx.TenantID
	case DataScopeSelf:
		filter["owner_id"] = ctx.UserID
	case DataScopeAssigned:
		filter["tenant_id"] = ctx.TenantID
		if len(ctx.AssignedIDs) > 0 {
			filter["id"] = ctx.AssignedIDs
		}
	}
	return filter
}

// ============================================================================
// 五、字段级权限
// ============================================================================

// FieldPermission 字段级权限
type FieldPermission struct {
	Resource ResourceType `json:"resource"`
	Field    string       `json:"field"`
	Action   ActionType   `json:"action"` // view, edit
}

// FieldMask 字段掩码（用于数据脱敏）
type FieldMask struct {
	Field   string `json:"field"`
	MaskType string `json:"mask_type"` // full, partial, hash
}

// MaskValue 掩码值
func (fm *FieldMask) MaskValue(value string) string {
	switch fm.MaskType {
	case "full":
		return "****"
	case "partial":
		if len(value) <= 4 {
			return "****"
		}
		return value[:2] + "****" + value[len(value)-2:]
	case "hash":
		return "[HASH]"
	default:
		return value
	}
}

// ============================================================================
// 六、操作权限矩阵（CRUD 细分）
// ============================================================================

// OperationMatrix 操作权限矩阵
type OperationMatrix struct {
	Resource ResourceType      `json:"resource"`
	Actions  map[ActionType]bool `json:"actions"`
}

// NewOperationMatrix 创建操作矩阵
func NewOperationMatrix(resource ResourceType) *OperationMatrix {
	return &OperationMatrix{
		Resource: resource,
		Actions:  make(map[ActionType]bool),
	}
}

// SetAction 设置操作权限
func (om *OperationMatrix) SetAction(action ActionType, allowed bool) {
	om.Actions[action] = allowed
}

// IsAllowed 检查操作是否允许
func (om *OperationMatrix) IsAllowed(action ActionType) bool {
	if allowed, ok := om.Actions[action]; ok {
		return allowed
	}
	return false
}

// GetAllowedActions 获取所有允许的操作
func (om *OperationMatrix) GetAllowedActions() []ActionType {
	var result []ActionType
	for action, allowed := range om.Actions {
		if allowed {
			result = append(result, action)
		}
	}
	return result
}
