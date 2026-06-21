// P6: 自然语言查询（NLQ）转 SQL 引擎
package nlq

import (
	"fmt"
"sort"
	"regexp"
	"strings"
	"sync"
)

// ============================================================================
// 核心类型定义
// ============================================================================

// NLQEngine 自然语言查询引擎
type NLQEngine struct {
	mu sync.RWMutex

	// 表结构元数据
	schemas map[string]*TableSchema

	// 查询模板
	templates []QueryTemplate

	// 实体映射
	entityMap map[string]string

	// 同义词词典
	synonyms map[string][]string
}

// TableSchema 表结构
type TableSchema struct {
	Name        string
	Alias       string
	Columns     []ColumnInfo
	Joins       []JoinInfo
	TimeColumn  string
}

// ColumnInfo 列信息
type ColumnInfo struct {
	Name        string
	Alias       string
	Type        string
	Description string
	Aggregable  bool
}

// JoinInfo 关联信息
type JoinInfo struct {
	Table     string
	Condition string
	Type      string // LEFT, INNER, RIGHT
}

// QueryTemplate 查询模板
type QueryTemplate struct {
	Name       string
	Pattern    *regexp.Regexp
	SQLBuilder func(*NLQContext) string
	Priority   int
}

// NLQContext 解析上下文
type NLQContext struct {
	OriginalQuery string
	Table         string
	Columns       []string
	Filters       []Filter
	GroupBy       []string
	OrderBy       []string
	Limit         int
	TimeRange     *TimeRange
	Aggregation   []Aggregation
}

// Filter 过滤条件
type Filter struct {
	Column   string
	Operator string
	Value    string
	Logic    string // AND, OR
}

// TimeRange 时间范围
type TimeRange struct {
	Start string
	End   string
	Raw   string
}

// Aggregation 聚合
type Aggregation struct {
	Function string
	Column   string
	Alias    string
}

// NLQResult NLQ 解析结果
type NLQResult struct {
	OriginalQuery string
	SQL           string
	Table         string
	Confidence    float64
	Explanation   string
	Parsed        *NLQContext
}

// NewNLQEngine 创建 NLQ 引擎
func NewNLQEngine() *NLQEngine {
	engine := &NLQEngine{
		schemas:   make(map[string]*TableSchema),
		entityMap: make(map[string]string),
		synonyms:  make(map[string][]string),
	}
	engine.loadBuiltinSchemas()
	engine.loadBuiltinTemplates()
	engine.loadBuiltinSynonyms()
	return engine
}

// ============================================================================
// Schema 管理
// ============================================================================

// RegisterSchema 注册表结构
func (e *NLQEngine) RegisterSchema(schema *TableSchema) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.schemas[schema.Name] = schema
	if schema.Alias != "" {
		e.schemas[schema.Alias] = schema
	}
}

// GetSchema 获取表结构
func (e *NLQEngine) GetSchema(name string) *TableSchema {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.schemas[name]
}

// ============================================================================
// 核心解析逻辑
// ============================================================================

// Convert 将自然语言转换为 SQL
func (e *NLQEngine) Convert(query string) *NLQResult {
	result := &NLQResult{
		OriginalQuery: query,
		Confidence:    0.0,
		Explanation:   "未能解析查询",
	}

	if query == "" {
		return result
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. 预处理查询
	cleanQuery := e.preprocess(query)

	// 2. 构建解析上下文
	ctx := &NLQContext{OriginalQuery: query, Limit: 100}

	// 3. 识别表
	ctx.Table = e.identifyTable(cleanQuery)
	if ctx.Table == "" {
		result.Explanation = "无法识别查询的目标表"
		return result
	}
	schema := e.schemas[ctx.Table]

	// 4. 识别时间范围
	ctx.TimeRange = e.extractTimeRange(cleanQuery)

	// 5. 识别过滤条件
	ctx.Filters = e.extractFilters(cleanQuery, schema)

	// 6. 识别聚合
	ctx.Aggregation = e.extractAggregations(cleanQuery, schema)

	// 7. 识别分组和排序
	ctx.GroupBy = e.extractGroupBy(cleanQuery, schema)
	ctx.OrderBy = e.extractOrderBy(cleanQuery, schema)

	// 8. 识别限制条数
	ctx.Limit = e.extractLimit(cleanQuery)

	// 9. 构建 SQL
	sql := e.buildSQL(ctx, schema)
	result.SQL = sql
	result.Table = ctx.Table
	result.Confidence = e.calculateConfidence(ctx, schema)
	result.Explanation = e.generateExplanation(ctx)
	result.Parsed = ctx

	return result
}

// preprocess 预处理查询
func (e *NLQEngine) preprocess(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))

	// 同义词替换
	for word, replacements := range e.synonyms {
		for _, replacement := range replacements {
			query = strings.ReplaceAll(query, word, replacement)
		}
	}

	return query
}

// identifyTable 识别目标表
func (e *NLQEngine) identifyTable(query string) string {
	// 直接匹配表名或别名
	for name, schema := range e.schemas {
		if strings.Contains(query, name) {
			return schema.Name
		}
		if schema.Alias != "" && strings.Contains(query, schema.Alias) {
			return schema.Name
		}
	}

	// 基于关键词推断表
	switch {
	case containsAny(query, []string{"流量", "flow", "packet", "byte"}):
		return "flows"
	case containsAny(query, []string{"告警", "alert", "告警"}):
		return "alerts"
	case containsAny(query, []string{"服务", "service", "拓扑"}):
		return "services"
	case containsAny(query, []string{"日志", "log", "error"}):
		return "logs"
	case containsAny(query, []string{"metric", "指标", "cpu", "memory"}):
		return "metrics"
	}

	return "flows" // 默认表
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// extractTimeRange 提取时间范围
func (e *NLQEngine) extractTimeRange(query string) *TimeRange {
	tr := &TimeRange{}

	// 匹配中文时间表达
	patterns := []struct {
		regex string
		start string
		end   string
	}{
		{`最近\s*(\d+)\s*小时`, `now() - INTERVAL \1 HOUR`, `now()`},
		{`最近\s*(\d+)\s*天`, `now() - INTERVAL \1 DAY`, `now()`},
		{`最近\s*(\d+)\s*分钟`, `now() - INTERVAL \1 MINUTE`, `now()`},
		{`今天`, `CURRENT_DATE()`, `now()`},
		{`昨天`, `CURRENT_DATE() - INTERVAL 1 DAY`, `CURRENT_DATE()`},
		{`过去\s*(\d+)\s*小时`, `now() - INTERVAL \1 HOUR`, `now()`},
		{`last\s+(\d+)\s+hours?`, `now() - INTERVAL \1 HOUR`, `now()`},
		{`last\s+(\d+)\s+days?`, `now() - INTERVAL \1 DAY`, `now()`},
		{`today`, `CURRENT_DATE()`, `now()`},
		{`yesterday`, `CURRENT_DATE() - INTERVAL 1 DAY`, `CURRENT_DATE()`},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.regex)
		if matches := re.FindStringSubmatch(query); matches != nil {
			tr.Start = re.ReplaceAllString(matches[0], p.start)
			tr.End = re.ReplaceAllString(matches[0], p.end)
			tr.Raw = matches[0]
			return tr
		}
	}

	// 默认最近 1 小时
	tr.Start = "now() - INTERVAL 1 HOUR"
	tr.End = "now()"
	return tr
}

// extractFilters 提取过滤条件
func (e *NLQEngine) extractFilters(query string, schema *TableSchema) []Filter {
	var filters []Filter

	if schema == nil {
		return filters
	}

	// 1. 提取服务名过滤
	servicePattern := regexp.MustCompile(`(?:service|服务)\s*[:=]?\s*['"]?([a-zA-Z0-9_-]+)['"]?`)
	if matches := servicePattern.FindStringSubmatch(query); matches != nil {
		filters = append(filters, Filter{
			Column:   "service_name",
			Operator: "=",
			Value:    matches[1],
			Logic:    "AND",
		})
	}

	// 2. 提取状态码过滤
	statusPattern := regexp.MustCompile(`(?:status|状态码)\s*(>=|<=|>|<|=)?\s*(\d{3})`)
	if matches := statusPattern.FindStringSubmatch(query); matches != nil {
		op := "="
		if matches[1] != "" {
			op = matches[1]
		}
		filters = append(filters, Filter{
			Column:   "status_code",
			Operator: op,
			Value:    matches[2],
			Logic:    "AND",
		})
	}

	// 3. 提取错误率过滤
	errorPattern := regexp.MustCompile(`(?:error_rate|错误率)\s*(>=|<=|>|<|大于|小于|超过|低于)\s*(\d+(?:\.\d+)?)\s*%?`)
	if matches := errorPattern.FindStringSubmatch(query); matches != nil {
		op := e.normalizeOperator(matches[1])
		filters = append(filters, Filter{
			Column:   "error_rate",
			Operator: op,
			Value:    matches[2],
			Logic:    "AND",
		})
	}

	// 4. 提取协议过滤
	protoPattern := regexp.MustCompile(`(?:protocol|协议)\s*[:=]?\s*['"]?([a-zA-Z0-9]+)['"]?`)
	if matches := protoPattern.FindStringSubmatch(query); matches != nil {
		filters = append(filters, Filter{
			Column:   "protocol",
			Operator: "=",
			Value:    matches[1],
			Logic:    "AND",
		})
	}

	// 5. 提取 IP 过滤
	ipPattern := regexp.MustCompile(`(?:ip|地址)\s*[:=]?\s*['"]?([0-9.]+)['"]?`)
	if matches := ipPattern.FindStringSubmatch(query); matches != nil {
		filters = append(filters, Filter{
			Column:   "src_ip",
			Operator: "=",
			Value:    matches[1],
			Logic:    "AND",
		})
	}

	// 6. 提取严重级别
	severityPattern := regexp.MustCompile(`(?:severity|严重级别|级别)\s*[:=]?\s*['"]?([a-zA-Z]+)['"]?`)
	if matches := severityPattern.FindStringSubmatch(query); matches != nil {
		filters = append(filters, Filter{
			Column:   "severity",
			Operator: "=",
			Value:    matches[1],
			Logic:    "AND",
		})
	}

	return filters
}

func (e *NLQEngine) normalizeOperator(op string) string {
	switch op {
	case "大于", "超过", ">":
		return ">"
	case "小于", "低于", "<":
		return "<"
	case "大于等于", ">=":
		return ">="
	case "小于等于", "<=":
		return "<="
	case "等于", "==":
		return "="
	default:
		return ">="
	}
}

// extractAggregations 提取聚合
func (e *NLQEngine) extractAggregations(query string, schema *TableSchema) []Aggregation {
	var aggs []Aggregation

	aggPatterns := []struct {
		regex    string
		function string
		column   string
	}{
		{`(?:总|sum|总计|合计|总量)\s*(?:流量|字节|bytes)?`, "SUM", "bytes"},
		{`(?:平均|avg|平均值)\s*(?:延迟|latency|响应时间)?`, "AVG", "latency"},
		{`(?:最大|max|最大值)\s*(?:延迟|latency)?`, "MAX", "latency"},
		{`(?:最小|min|最小值)\s*(?:延迟|latency)?`, "MIN", "latency"},
		{`(?:数量|count|条数|次数)\s*(?:请求|告警|日志)?`, "COUNT", "*"},
		{`(?:总|sum|总计)\s*(?:包|packets|报文)?`, "SUM", "packets"},
	}

	for _, p := range aggPatterns {
		re := regexp.MustCompile(p.regex)
		if re.MatchString(query) {
			aggs = append(aggs, Aggregation{
				Function: p.function,
				Column:   p.column,
				Alias:    fmt.Sprintf("%s_%s", strings.ToLower(p.function), p.column),
			})
		}
	}

	return aggs
}

// extractGroupBy 提取分组
func (e *NLQEngine) extractGroupBy(query string, schema *TableSchema) []string {
	// 按服务分组
	if strings.Contains(query, "按服务") || strings.Contains(query, "by service") {
		return []string{"service_name"}
	}
	// 按协议分组
	if strings.Contains(query, "按协议") || strings.Contains(query, "by protocol") {
		return []string{"protocol"}
	}
	// 按状态码分组
	if strings.Contains(query, "按状态码") || strings.Contains(query, "by status") {
		return []string{"status_code"}
	}
	// 按小时分组
	if strings.Contains(query, "按小时") || strings.Contains(query, "by hour") {
		return []string{"DATE_FORMAT(timestamp, '%H')"}
	}
	// 按天分组
	if strings.Contains(query, "按天") || strings.Contains(query, "by day") {
		return []string{"DATE(timestamp)"}
	}
	return []string{}
}

// extractOrderBy 提取排序
func (e *NLQEngine) extractOrderBy(query string, schema *TableSchema) []string {
	var orderBy []string

	if strings.Contains(query, "降序") || strings.Contains(query, "desc") || strings.Contains(query, "最多") || strings.Contains(query, "最大") {
		orderBy = append(orderBy, "DESC")
	} else if strings.Contains(query, "升序") || strings.Contains(query, "asc") || strings.Contains(query, "最少") || strings.Contains(query, "最小") {
		orderBy = append(orderBy, "ASC")
	} else {
		orderBy = append(orderBy, "DESC") // 默认降序
	}

	return orderBy
}

// extractLimit 提取限制条数
func (e *NLQEngine) extractLimit(query string) int {
	patterns := []struct {
		regex string
	}{
		{`(?:top|前)\s*(\d+)`},
		{`limit\s+(\d+)`},
		{`(\d+)\s*条`},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.regex)
		if matches := re.FindStringSubmatch(query); matches != nil {
			var limit int
			fmt.Sscanf(matches[1], "%d", &limit)
			if limit > 0 && limit <= 1000 {
				return limit
			}
		}
	}

	return 100 // 默认
}

// buildSQL 构建 SQL
func (e *NLQEngine) buildSQL(ctx *NLQContext, schema *TableSchema) string {
	var sb strings.Builder

	// SELECT
	sb.WriteString("SELECT ")
	if len(ctx.Aggregation) > 0 {
		for i, agg := range ctx.Aggregation {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s(%s) AS %s", agg.Function, agg.Column, agg.Alias))
		}
		if len(ctx.GroupBy) > 0 {
			sb.WriteString(", ")
			sb.WriteString(strings.Join(ctx.GroupBy, ", "))
		}
	} else {
		if schema != nil && len(schema.Columns) > 0 {
			cols := make([]string, 0, len(schema.Columns))
			for _, col := range schema.Columns {
				cols = append(cols, col.Name)
			}
			sb.WriteString(strings.Join(cols, ", "))
		} else {
			sb.WriteString("*")
		}
	}

	// FROM
	sb.WriteString(fmt.Sprintf(" FROM %s", ctx.Table))

	// WHERE
	var conditions []string

	// 时间范围
	if ctx.TimeRange != nil && ctx.TimeRange.Start != "" {
		timeCol := "timestamp"
		if schema != nil && schema.TimeColumn != "" {
			timeCol = schema.TimeColumn
		}
		conditions = append(conditions, fmt.Sprintf("%s >= %s", timeCol, ctx.TimeRange.Start))
		if ctx.TimeRange.End != "" {
			conditions = append(conditions, fmt.Sprintf("%s <= %s", timeCol, ctx.TimeRange.End))
		}
	}

	// 过滤条件
	for _, f := range ctx.Filters {
		val := f.Value
		if f.Operator == "=" {
			val = fmt.Sprintf("'%s'", f.Value)
		}
		conditions = append(conditions, fmt.Sprintf("%s %s %s", f.Column, f.Operator, val))
	}

	if len(conditions) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conditions, " AND "))
	}

	// GROUP BY
	if len(ctx.GroupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		sb.WriteString(strings.Join(ctx.GroupBy, ", "))
	}

	// ORDER BY
	if len(ctx.Aggregation) > 0 {
		sb.WriteString(fmt.Sprintf(" ORDER BY %s ", ctx.Aggregation[0].Alias))
		if len(ctx.OrderBy) > 0 {
			sb.WriteString(ctx.OrderBy[0])
		} else {
			sb.WriteString("DESC")
		}
	}

	// LIMIT
	if ctx.Limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", ctx.Limit))
	}

	return sb.String()
}

// calculateConfidence 计算置信度
func (e *NLQEngine) calculateConfidence(ctx *NLQContext, schema *TableSchema) float64 {
	score := 0.5 // 基础分

	if schema != nil {
		score += 0.2
	}
	if ctx.Table != "" {
		score += 0.1
	}
	if len(ctx.Filters) > 0 {
		score += 0.1
	}
	if ctx.TimeRange != nil && ctx.TimeRange.Raw != "" {
		score += 0.1
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// generateExplanation 生成解释
func (e *NLQEngine) generateExplanation(ctx *NLQContext) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("从表 %s 查询", ctx.Table))

	if ctx.TimeRange != nil && ctx.TimeRange.Raw != "" {
		parts = append(parts, fmt.Sprintf("时间范围: %s", ctx.TimeRange.Raw))
	}

	if len(ctx.Filters) > 0 {
		parts = append(parts, fmt.Sprintf("包含 %d 个过滤条件", len(ctx.Filters)))
	}

	if len(ctx.Aggregation) > 0 {
		funcs := make([]string, len(ctx.Aggregation))
		for i, a := range ctx.Aggregation {
			funcs[i] = a.Function
		}
		parts = append(parts, fmt.Sprintf("使用聚合: %s", strings.Join(funcs, ", ")))
	}

	if len(ctx.GroupBy) > 0 {
		parts = append(parts, fmt.Sprintf("按 %s 分组", strings.Join(ctx.GroupBy, ", ")))
	}

	parts = append(parts, fmt.Sprintf("限制返回 %d 条", ctx.Limit))

	return strings.Join(parts, "，")
}

// ============================================================================
// 内置 Schema / 模板 / 同义词
// ============================================================================

func (e *NLQEngine) loadBuiltinSchemas() {
	schemas := []*TableSchema{
		{
			Name:       "flows",
			Alias:      "流量",
			TimeColumn: "timestamp",
			Columns: []ColumnInfo{
				{Name: "timestamp", Type: "timestamp", Description: "时间戳"},
				{Name: "src_ip", Type: "string", Description: "源IP"},
				{Name: "dst_ip", Type: "string", Description: "目标IP"},
				{Name: "src_port", Type: "int", Description: "源端口"},
				{Name: "dst_port", Type: "int", Description: "目标端口"},
				{Name: "protocol", Type: "string", Description: "协议"},
				{Name: "bytes", Type: "bigint", Description: "字节数", Aggregable: true},
				{Name: "packets", Type: "int", Description: "包数", Aggregable: true},
				{Name: "service_name", Type: "string", Description: "服务名"},
			},
		},
		{
			Name:       "alerts",
			Alias:      "告警",
			TimeColumn: "created_at",
			Columns: []ColumnInfo{
				{Name: "created_at", Type: "timestamp", Description: "创建时间"},
				{Name: "service_name", Type: "string", Description: "服务名"},
				{Name: "severity", Type: "string", Description: "严重级别"},
				{Name: "message", Type: "string", Description: "告警消息"},
				{Name: "status", Type: "string", Description: "状态"},
			},
		},
		{
			Name:       "metrics",
			Alias:      "指标",
			TimeColumn: "timestamp",
			Columns: []ColumnInfo{
				{Name: "timestamp", Type: "timestamp", Description: "时间戳"},
				{Name: "service_name", Type: "string", Description: "服务名"},
				{Name: "metric_name", Type: "string", Description: "指标名"},
				{Name: "value", Type: "double", Description: "值", Aggregable: true},
			},
		},
		{
			Name:       "logs",
			Alias:      "日志",
			TimeColumn: "timestamp",
			Columns: []ColumnInfo{
				{Name: "timestamp", Type: "timestamp", Description: "时间戳"},
				{Name: "service_name", Type: "string", Description: "服务名"},
				{Name: "level", Type: "string", Description: "日志级别"},
				{Name: "message", Type: "string", Description: "日志内容"},
			},
		},
		{
			Name:       "services",
			Alias:      "服务",
			TimeColumn: "updated_at",
			Columns: []ColumnInfo{
				{Name: "service_name", Type: "string", Description: "服务名"},
				{Name: "status", Type: "string", Description: "状态"},
				{Name: "latency", Type: "double", Description: "延迟", Aggregable: true},
				{Name: "error_rate", Type: "double", Description: "错误率", Aggregable: true},
				{Name: "updated_at", Type: "timestamp", Description: "更新时间"},
			},
		},
	}

	for _, s := range schemas {
		e.RegisterSchema(s)
	}
}

func (e *NLQEngine) loadBuiltinTemplates() {
	// 模板通过正则匹配和 SQL 构建函数定义
	e.templates = []QueryTemplate{
		{
			Name:     "traffic_summary",
			Pattern:  regexp.MustCompile(`(?:流量|流量统计|总流量)`),
			Priority: 1,
		},
		{
			Name:     "error_analysis",
			Pattern:  regexp.MustCompile(`(?:错误|error|失败|异常)`),
			Priority: 2,
		},
		{
			Name:     "latency_analysis",
			Pattern:  regexp.MustCompile(`(?:延迟|latency|响应时间|慢)`),
			Priority: 2,
		},
		{
			Name:     "service_overview",
			Pattern:  regexp.MustCompile(`(?:服务|service).*?(?:状态|概览|overview)`),
			Priority: 1,
		},
	}
}

func (e *NLQEngine) loadBuiltinSynonyms() {
	e.synonyms = map[string][]string{
		"响应时间": {"latency", "延迟"},
		"错误率":   {"error_rate", "error rate"},
		"流量":    {"flow", "traffic"},
		"服务":    {"service", "svc"},
		"告警":    {"alert", "warning"},
		"日志":    {"log", "logs"},
	}
}

// ============================================================================
// 辅助方法
// ============================================================================

// ListSchemas 列出所有表
func (e *NLQEngine) ListSchemas() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	seen := make(map[string]bool)
	var names []string
	for name, schema := range e.schemas {
		if !seen[schema.Name] {
			seen[schema.Name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// AddSynonym 添加同义词
func (e *NLQEngine) AddSynonym(word string, replacement string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.synonyms[word] = append(e.synonyms[word], replacement)
}
