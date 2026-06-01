// Package isolation 多维度租户隔离
//
// 隔离维度:
//   1. Namespace Isolation: 租户只能访问自己的 K8s namespace
//   2. Query Isolation: 所有查询自动注入 tenant_id 过滤条件
//   3. Storage Isolation: ClickHouse/TiDB 查询自动追加 WHERE tenant_id = ?
//   4. Alert Isolation: 告警规则和事件按租户隔离
//
// 禁止:
//   - 全局共享查询 (无 tenant_id 的查询)
//   - 跨租户数据访问
//   - 跨 namespace 数据访问 (除非有权限)
package isolation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	tenant "cloud-flow/services/shared/tenant"
)

// ---------------------------------------------------------------------------
// IsolationLevel
// ---------------------------------------------------------------------------

// IsolationLevel 定义租户隔离级别。
type IsolationLevel int

const (
	// IsolationStrict 严格隔离: 每个查询必须携带 tenant_id，不允许跨租户操作。
	IsolationStrict IsolationLevel = 1

	// IsolationPlatform 平台管理员: 可以跨租户查询，但仍需显式指定 tenant_id。
	IsolationPlatform IsolationLevel = 2
)

// String 返回隔离级别的可读名称。
func (l IsolationLevel) String() string {
	switch l {
	case IsolationStrict:
		return "strict"
	case IsolationPlatform:
		return "platform"
	default:
		return fmt.Sprintf("unknown(%d)", l)
	}
}

// ---------------------------------------------------------------------------
// QueryFilter
// ---------------------------------------------------------------------------

// QueryFilter 定义查询过滤条件，所有维度均为可选（由调用方按需填充）。
type QueryFilter struct {
	TenantID      string
	ProjectID     string
	Namespaces    []string
	StartTime     int64
	EndTime       int64
	ExtraFilters  map[string]string
}

// ---------------------------------------------------------------------------
// IsolationGuard
// ---------------------------------------------------------------------------

// IsolationGuard 是租户隔离的核心守卫，负责在查询、存储、告警等维度
// 自动注入和校验 tenant_id，防止跨租户数据泄露。
type IsolationGuard struct {
	level IsolationLevel
}

// NewIsolationGuard 创建指定隔离级别的守卫实例。
func NewIsolationGuard(level IsolationLevel) *IsolationGuard {
	return &IsolationGuard{level: level}
}

// Level 返回当前隔离级别。
func (g *IsolationGuard) Level() IsolationLevel {
	return g.level
}

// ---------------------------------------------------------------------------
// Namespace isolation
// ---------------------------------------------------------------------------

// BuildNamespaceFilter 构建包含 tenant_id 和 namespace 的 SQL WHERE 条件片段。
// 返回格式: tenant_id = 'x' AND namespace IN ('ns1','ns2')
func BuildNamespaceFilter(tenantID string, allowedNamespaces []string) string {
	escapedTenant := escapeSQLValue(tenantID)

	if len(allowedNamespaces) == 0 {
		return fmt.Sprintf("tenant_id = '%s'", escapedTenant)
	}

	escaped := make([]string, 0, len(allowedNamespaces))
	for _, ns := range allowedNamespaces {
		escaped = append(escaped, fmt.Sprintf("'%s'", escapeSQLValue(ns)))
	}

	return fmt.Sprintf("tenant_id = '%s' AND namespace IN (%s)",
		escapedTenant, strings.Join(escaped, ","))
}

// ValidateNamespaceOwnership 校验 namespace 是否属于指定租户。
//
// 命名空间归属规则:
//   - 格式为 {tenantID}-xxx 的 namespace 归属于 tenantID
//   - 格式为 shared-xxx 的 namespace 为共享命名空间，所有租户可访问
//   - 其他格式视为不合法
func ValidateNamespaceOwnership(tenantID, namespace string) error {
	if tenantID == "" {
		return fmt.Errorf("namespace ownership validation failed: tenant_id is empty")
	}
	if namespace == "" {
		return fmt.Errorf("namespace ownership validation failed: namespace is empty")
	}

	// 共享命名空间: shared-xxx
	if strings.HasPrefix(namespace, "shared-") {
		return nil
	}

	// 租户命名空间: {tenantID}-xxx
	if strings.HasPrefix(namespace, tenantID+"-") {
		return nil
	}

	return fmt.Errorf(
		"namespace ownership violation: namespace %q does not belong to tenant %q",
		namespace, tenantID)
}

// ---------------------------------------------------------------------------
// Query isolation
// ---------------------------------------------------------------------------

var (
	// reWhere 用于检测 SQL 中的 WHERE 关键字（不区分大小写，要求作为独立词出现）。
	reWhere = regexp.MustCompile(`(?i)\bWHERE\b`)

	// reTenantID 用于检测 SQL 中是否已包含 tenant_id 过滤条件。
	reTenantID = regexp.MustCompile(`(?i)\btenant_id\s*=\s*['"?]`)
)

// EnforceQueryFilter 对原始 SQL 注入 tenant_id 过滤条件。
//
// 规则:
//   - 若 context 中无 tenant_id → 返回错误
//   - 若 SQL 无 WHERE 子句 → 追加 WHERE tenant_id = 'xxx'
//   - 若 SQL 有 WHERE 子句但无 tenant_id → 追加 AND tenant_id = 'xxx'
//   - 若 SQL 已有 tenant_id → 校验是否与 context 中的 tenant_id 一致
//
// 注意: 此实现使用字符串匹配而非完整 SQL 解析器，适用于结构化 SQL 场景。
func (g *IsolationGuard) EnforceQueryFilter(ctx context.Context, originalSQL string) (string, error) {
	return g.enforceQuery(ctx, originalSQL)
}

// EnforceClickHouseQuery 对 ClickHouse 查询注入 tenant_id 过滤条件。
// 逻辑与 EnforceQueryFilter 相同，保留独立方法以便未来针对 ClickHouse 方言做特殊处理。
func (g *IsolationGuard) EnforceClickHouseQuery(ctx context.Context, query string) (string, error) {
	return g.enforceQuery(ctx, query)
}

// EnforceTiDBQuery 对 TiDB 查询注入 tenant_id 过滤条件。
// 逻辑与 EnforceQueryFilter 相同，保留独立方法以便未来针对 TiDB 方言做特殊处理。
func (g *IsolationGuard) EnforceTiDBQuery(ctx context.Context, query string) (string, error) {
	return g.enforceQuery(ctx, query)
}

// enforceQuery 是查询过滤的核心实现。
func (g *IsolationGuard) enforceQuery(ctx context.Context, sql string) (string, error) {
	// 1. 从 context 提取 tenant_id
	tc, ok := tenant.FromContext(ctx)
	if !ok || tc == nil || tc.TenantID == "" {
		return "", fmt.Errorf("tenant isolation violation: no tenant_id in context")
	}

	// 2. 校验 tenant_id 格式
	if err := validateTenantID(tc.TenantID); err != nil {
		return "", fmt.Errorf("tenant isolation violation: invalid tenant_id: %w", err)
	}

	// 3. 检查 SQL 中是否已包含 tenant_id 过滤
	if reTenantID.MatchString(sql) {
		// 已有 tenant_id 过滤，校验是否匹配当前租户
		// 对于 IsolationPlatform 级别，允许查询中携带不同的 tenant_id（显式跨租户）
		if g.level == IsolationStrict {
			if err := g.validateExistingTenantID(sql, tc.TenantID); err != nil {
				return "", err
			}
		}
		return sql, nil
	}

	// 4. 注入 tenant_id 过滤条件
	escapedTenant := escapeSQLValue(tc.TenantID)
	filter := fmt.Sprintf("tenant_id = '%s'", escapedTenant)

	trimmedSQL := strings.TrimSpace(sql)
	upperSQL := strings.ToUpper(trimmedSQL)

	if reWhere.MatchString(trimmedSQL) {
		// 已有 WHERE 子句 → 追加 AND
		return g.appendAndCondition(trimmedSQL, upperSQL, filter), nil
	}

	// 无 WHERE 子句 → 追加 WHERE
	return g.appendWhereClause(trimmedSQL, upperSQL, filter), nil
}

// validateExistingTenantID 校验 SQL 中已有的 tenant_id 值是否与当前租户匹配。
// 使用简单的字符串匹配，检查 tenant_id = 'xxx' 或 tenant_id = "xxx" 格式。
func (g *IsolationGuard) validateExistingTenantID(sql, expectedTenantID string) error {
	// 匹配 tenant_id = 'value' 或 tenant_id = "value"
	re := regexp.MustCompile(`(?i)\btenant_id\s*=\s*['"]([^'"]+)['"]`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) >= 2 {
		if matches[1] != expectedTenantID {
			return fmt.Errorf(
				"tenant isolation violation: query tenant_id %q does not match context tenant_id %q",
				matches[1], expectedTenantID)
		}
	}
	return nil
}

// appendAndCondition 在已有 WHERE 子句的 SQL 中追加 AND 条件。
func (g *IsolationGuard) appendAndCondition(sql, upperSQL, filter string) string {
	// 查找 WHERE 关键字的位置
	whereIdx := reWhere.FindStringIndex(upperSQL)
	if whereIdx == nil {
		return sql
	}

	// 从 WHERE 之后找到条件开始的位置
	afterWhere := sql[whereIdx[1]:]
	afterWhere = strings.TrimSpace(afterWhere)

	return sql[:whereIdx[1]] + " " + afterWhere + " AND " + filter
}

// appendWhereClause 在没有 WHERE 子句的 SQL 中追加 WHERE 条件。
// 需要正确处理 GROUP BY、ORDER BY、LIMIT 等尾部子句。
func (g *IsolationGuard) appendWhereClause(sql, upperSQL, filter string) string {
	// 查找可能存在的尾部子句关键字
	clauseKeywords := []struct {
		keyword string
	}{
		{"GROUP BY"},
		{"ORDER BY"},
		{"HAVING"},
		{"LIMIT"},
		{"OFFSET"},
		{"UNION"},
		{"INTERSECT"},
		{"EXCEPT"},
	}

	insertIdx := len(sql)

	for _, ck := range clauseKeywords {
		idx := strings.Index(upperSQL, ck.keyword)
		if idx != -1 && idx < insertIdx {
			insertIdx = idx
		}
	}

	if insertIdx == len(sql) {
		// 没有尾部子句，直接追加
		return sql + " WHERE " + filter
	}

	// 在尾部子句之前插入 WHERE
	return sql[:insertIdx] + " WHERE " + filter + " " + sql[insertIdx:]
}

// ---------------------------------------------------------------------------
// Storage isolation
// ---------------------------------------------------------------------------

// BuildStoragePath 构建对象存储路径，确保租户数据物理隔离。
// 返回格式: {tenantID}/{dataType}/
func (g *IsolationGuard) BuildStoragePath(tenantID, dataType string) string {
	if err := validateTenantID(tenantID); err != nil {
		// 不应发生，调用方应确保 tenant_id 合法
		return fmt.Sprintf("_invalid_/%s/", dataType)
	}
	// 清理 dataType，移除路径分隔符防止目录穿越
	cleanDataType := strings.ReplaceAll(dataType, "/", "_")
	cleanDataType = strings.ReplaceAll(cleanDataType, "\\", "_")
	return fmt.Sprintf("%s/%s/", tenantID, cleanDataType)
}

// BuildClickHouseTable 构建每租户 ClickHouse 表名。
// 返回格式: flow_{tenant_hash}
//
// 注意: 此方法返回基于租户 ID 哈希的表名，适用于每租户独立表的隔离策略。
// 也可以使用共享表 + tenant_id 列的方式，此时无需调用此方法。
func (g *IsolationGuard) BuildClickHouseTable(tenantID string) string {
	if err := validateTenantID(tenantID); err != nil {
		return "flow_invalid"
	}
	hash := sha256.Sum256([]byte(tenantID))
	// 取前 16 个字符作为哈希后缀，足够避免冲突
	return fmt.Sprintf("flow_%x", hash[:8])
}

// ---------------------------------------------------------------------------
// Alert isolation
// ---------------------------------------------------------------------------

// AlertRule 告警规则。
type AlertRule struct {
	Id        string
	TenantID  string
	Name      string
	Enabled   bool
	CreatedAt int64
}

// AlertEvent 告警事件。
type AlertEvent struct {
	Id        string
	TenantID  string
	RuleID    string
	Severity  string
	Message   string
	Timestamp int64
}

// EnforceAlertFilter 过滤告警规则列表，仅返回属于当前租户的规则。
// 若 context 中无 tenant_id，返回空列表。
func (g *IsolationGuard) EnforceAlertFilter(ctx context.Context, rules []*AlertRule) []*AlertRule {
	tc, ok := tenant.FromContext(ctx)
	if !ok || tc == nil || tc.TenantID == "" {
		return nil
	}

	filtered := make([]*AlertRule, 0, len(rules))
	for _, rule := range rules {
		if rule != nil && rule.TenantID == tc.TenantID {
			filtered = append(filtered, rule)
		}
	}
	return filtered
}

// EnforceAlertEventFilter 过滤告警事件列表，仅返回属于当前租户的事件。
// 若 context 中无 tenant_id，返回空列表。
func (g *IsolationGuard) EnforceAlertEventFilter(ctx context.Context, events []*AlertEvent) []*AlertEvent {
	tc, ok := tenant.FromContext(ctx)
	if !ok || tc == nil || tc.TenantID == "" {
		return nil
	}

	filtered := make([]*AlertEvent, 0, len(events))
	for _, event := range events {
		if event != nil && event.TenantID == tc.TenantID {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// ---------------------------------------------------------------------------
// SQL injection prevention
// ---------------------------------------------------------------------------

// escapeSQLValue 对字符串中的单引号进行转义，防止 SQL 注入。
// 将 ' 替换为 ''（SQL 标准转义方式）。
func escapeSQLValue(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// validateTenantID 校验 tenant_id 格式。
//
// 合法格式: 字母、数字、下划线、连字符，长度 1-64 字符。
func validateTenantID(tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("tenant_id must not be empty")
	}
	if len(tenantID) > 64 {
		return fmt.Errorf("tenant_id must not exceed 64 characters, got %d", len(tenantID))
	}

	// 仅允许字母、数字、下划线、连字符
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validPattern.MatchString(tenantID) {
		return fmt.Errorf(
			"tenant_id contains invalid characters: only alphanumeric, underscore, and hyphen are allowed")
	}
	return nil
}
