//go:build linux

// Package rbac 提供完整的 RBAC 权限管理
// - 细粒度权限控制（资源×操作）
// - 数据权限范围（全局/租户/个人/指派）
// - 角色与权限绑定
// - 策略评估引擎
package rbac

import (
	"fmt"
	"strings"
)

// ============================================================================
// 一、资源类型定义
// ============================================================================

// ResourceType 资源类型
type ResourceType string

const (
	ResourceDashboard ResourceType = "dashboard" // 仪表盘
	ResourceAsset     ResourceType = "asset"     // 资产
	ResourceAlert     ResourceType = "alert"     // 告警
	ResourceTopology  ResourceType = "topology"  // 拓扑
	ResourceTrace     ResourceType = "trace"     // 链路追踪
	ResourceProbe     ResourceType = "probe"     // 探针
	ResourceConfig    ResourceType = "config"    // 配置
	ResourceUser      ResourceType = "user"      // 用户
	ResourceTenant    ResourceType = "tenant"    // 租户
	ResourceReport    ResourceType = "report"    // 报告
	ResourceLog       ResourceType = "log"       // 日志
	ResourceMetric    ResourceType = "metric"    // 指标
	ResourcePolicy    ResourceType = "policy"    // 策略
)

// AllResourceTypes 所有资源类型
var AllResourceTypes = []ResourceType{
	ResourceDashboard, ResourceAsset, ResourceAlert, ResourceTopology,
	ResourceTrace, ResourceProbe, ResourceConfig, ResourceUser,
	ResourceTenant, ResourceReport, ResourceLog, ResourceMetric, ResourcePolicy,
}

// String 返回字符串
func (r ResourceType) String() string { return string(r) }

// Validate 验证资源类型
func (r ResourceType) Validate() bool {
	for _, rt := range AllResourceTypes {
		if rt == r {
			return true
		}
	}
	return false
}

// ============================================================================
// 二、操作类型定义
// ============================================================================

// ActionType 操作类型
type ActionType string

const (
	ActionView    ActionType = "view"    // 查看
	ActionCreate  ActionType = "create"  // 创建
	ActionEdit    ActionType = "edit"    // 编辑
	ActionDelete  ActionType = "delete"  // 删除
	ActionExport  ActionType = "export"  // 导出
	ActionManage  ActionType = "manage"  // 管理
	ActionExecute ActionType = "execute" // 执行
)

// AllActionTypes 所有操作类型
var AllActionTypes = []ActionType{
	ActionView, ActionCreate, ActionEdit, ActionDelete,
	ActionExport, ActionManage, ActionExecute,
}

// String 返回字符串
func (a ActionType) String() string { return string(a) }

// Validate 验证操作类型
func (a ActionType) Validate() bool {
	for _, at := range AllActionTypes {
		if at == a {
			return true
		}
	}
	return false
}

// CRUDActions 基本的 CRUD 操作
var CRUDActions = []ActionType{ActionView, ActionCreate, ActionEdit, ActionDelete}

// ============================================================================
// 三、权限定义
// ============================================================================

// Permission 细粒度权限（资源+操作）
type Permission struct {
	Resource ResourceType `json:"resource"`
	Action   ActionType   `json:"action"`
}

// String 返回权限字符串格式 "resource:action"
func (p Permission) String() string {
	return fmt.Sprintf("%s:%s", p.Resource, p.Action)
}

// ParsePermission 解析权限字符串
func ParsePermission(s string) (Permission, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return Permission{}, fmt.Errorf("invalid permission format: %s", s)
	}
	p := Permission{
		Resource: ResourceType(parts[0]),
		Action:   ActionType(parts[1]),
	}
	if !p.Resource.Validate() {
		return Permission{}, fmt.Errorf("invalid resource type: %s", parts[0])
	}
	if !p.Action.Validate() {
		return Permission{}, fmt.Errorf("invalid action type: %s", parts[1])
	}
	return p, nil
}

// MustParsePermission 解析权限字符串，失败 panic
func MustParsePermission(s string) Permission {
	p, err := ParsePermission(s)
	if err != nil {
		panic(err)
	}
	return p
}

// NewPermission 创建权限
func NewPermission(resource ResourceType, action ActionType) Permission {
	return Permission{Resource: resource, Action: action}
}

// Match 检查是否匹配（支持通配符）
func (p Permission) Match(other Permission) bool {
	if p.Resource != "*" && p.Resource != other.Resource {
		return false
	}
	if p.Action != "*" && p.Action != other.Action {
		return false
	}
	return true
}

// WildcardPermission 通配权限（所有资源所有操作）
var WildcardPermission = Permission{Resource: "*", Action: "*"}

// ============================================================================
// 四、预设权限组合
// ============================================================================

// PermissionSet 权限集合
type PermissionSet []Permission

// Contains 检查是否包含某个权限
func (ps PermissionSet) Contains(p Permission) bool {
	for _, perm := range ps {
		if perm.Match(p) || perm == WildcardPermission {
			return true
		}
	}
	return false
}

// ContainsString 检查是否包含权限字符串
func (ps PermissionSet) ContainsString(s string) bool {
	p, err := ParsePermission(s)
	if err != nil {
		return false
	}
	return ps.Contains(p)
}

// Add 添加权限
func (ps *PermissionSet) Add(p Permission) {
	if !ps.Contains(p) {
		*ps = append(*ps, p)
	}
}

// Remove 移除权限
func (ps *PermissionSet) Remove(p Permission) {
	var result PermissionSet
	for _, perm := range *ps {
		if perm.Resource != p.Resource || perm.Action != p.Action {
			result = append(result, perm)
		}
	}
	*ps = result
}

// ToStrings 转换为字符串列表
func (ps PermissionSet) ToStrings() []string {
	var result []string
	for _, p := range ps {
		result = append(result, p.String())
	}
	return result
}

// ============================================================================
// 五、常用权限快捷方式
// ============================================================================

// FullPermissions 所有权限
func FullPermissions() PermissionSet {
	var ps PermissionSet
	for _, r := range AllResourceTypes {
		for _, a := range AllActionTypes {
			ps.Add(NewPermission(r, a))
		}
	}
	return ps
}

// ReadOnlyPermissions 只读权限
func ReadOnlyPermissions() PermissionSet {
	var ps PermissionSet
	for _, r := range AllResourceTypes {
		ps.Add(NewPermission(r, ActionView))
		ps.Add(NewPermission(r, ActionExport))
	}
	return ps
}

// AssetManagementPermissions 资产管理权限
func AssetManagementPermissions() PermissionSet {
	return PermissionSet{
		NewPermission(ResourceAsset, ActionView),
		NewPermission(ResourceAsset, ActionCreate),
		NewPermission(ResourceAsset, ActionEdit),
		NewPermission(ResourceAsset, ActionDelete),
		NewPermission(ResourceAsset, ActionExport),
		NewPermission(ResourceTopology, ActionView),
		NewPermission(ResourceTrace, ActionView),
		NewPermission(ResourceMetric, ActionView),
	}
}

// AlertManagementPermissions 告警管理权限
func AlertManagementPermissions() PermissionSet {
	return PermissionSet{
		NewPermission(ResourceAlert, ActionView),
		NewPermission(ResourceAlert, ActionCreate),
		NewPermission(ResourceAlert, ActionEdit),
		NewPermission(ResourceAlert, ActionDelete),
		NewPermission(ResourceAlert, ActionExecute),
		NewPermission(ResourceAsset, ActionView),
		NewPermission(ResourceMetric, ActionView),
	}
}

// OperatorPermissions 运维权限
func OperatorPermissions() PermissionSet {
	return PermissionSet{
		NewPermission(ResourceDashboard, ActionView),
		NewPermission(ResourceAsset, ActionView),
		NewPermission(ResourceAlert, ActionView),
		NewPermission(ResourceAlert, ActionEdit),
		NewPermission(ResourceAlert, ActionExecute),
		NewPermission(ResourceTopology, ActionView),
		NewPermission(ResourceTrace, ActionView),
		NewPermission(ResourceProbe, ActionView),
		NewPermission(ResourceProbe, ActionExecute),
		NewPermission(ResourceConfig, ActionView),
		NewPermission(ResourceLog, ActionView),
		NewPermission(ResourceMetric, ActionView),
		NewPermission(ResourceReport, ActionView),
		NewPermission(ResourceReport, ActionExport),
	}
}

// AuditorPermissions 审计权限
func AuditorPermissions() PermissionSet {
	return PermissionSet{
		NewPermission(ResourceDashboard, ActionView),
		NewPermission(ResourceAsset, ActionView),
		NewPermission(ResourceAsset, ActionExport),
		NewPermission(ResourceAlert, ActionView),
		NewPermission(ResourceAlert, ActionExport),
		NewPermission(ResourceTopology, ActionView),
		NewPermission(ResourceTrace, ActionView),
		NewPermission(ResourceTrace, ActionExport),
		NewPermission(ResourceLog, ActionView),
		NewPermission(ResourceLog, ActionExport),
		NewPermission(ResourceReport, ActionView),
		NewPermission(ResourceReport, ActionExport),
		NewPermission(ResourceUser, ActionView),
		NewPermission(ResourceTenant, ActionView),
		NewPermission(ResourcePolicy, ActionView),
		NewPermission(ResourcePolicy, ActionExport),
	}
}
