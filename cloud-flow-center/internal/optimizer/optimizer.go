//go:build linux

package optimizer

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// ClickHouse 存储优化器
// 提供表结构优化建议、DDL 生成和性能分析
// ============================================================================

// TableOptimizer ClickHouse 表优化器
type TableOptimizer struct {
	config *OptimizerConfig
}

// OptimizerConfig 优化器配置
type OptimizerConfig struct {
	DefaultPartitionBy string // 默认分区表达式
	DefaultHotDays     int    // 热数据保留天数
	DefaultColdVolume  string // 冷存储卷名
	DefaultIndexGranularity int // 索引粒度
}

// DefaultOptimizerConfig 返回默认配置
func DefaultOptimizerConfig() *OptimizerConfig {
	return &OptimizerConfig{
		DefaultPartitionBy:      "toYYYYMMDD(timestamp)",
		DefaultHotDays:          7,
		DefaultColdVolume:       "cold",
		DefaultIndexGranularity: 8192,
	}
}

// NewTableOptimizer 创建表优化器
func NewTableOptimizer(cfg *OptimizerConfig) *TableOptimizer {
	if cfg == nil {
		cfg = DefaultOptimizerConfig()
	}
	return &TableOptimizer{config: cfg}
}

// ============================================================================
// 优化建议生成
// ============================================================================

// OptimizationSuggestion 优化建议
type OptimizationSuggestion struct {
	Category    string `json:"category"`
	Severity    string `json:"severity"` // critical/warning/info
	Table       string `json:"table"`
	Description string `json:"description"`
	SQL         string `json:"sql,omitempty"` // 修复 SQL
}

// AnalyzeTable 分析表结构并给出优化建议
func (to *TableOptimizer) AnalyzeTable(tableName string, columns []ColumnInfo, currentSettings TableSettings) []OptimizationSuggestion {
	var suggestions []OptimizationSuggestion

	// 1. 分区检查
	if currentSettings.PartitionBy == "" {
		suggestions = append(suggestions, OptimizationSuggestion{
			Category:    "partition",
			Severity:    "critical",
			Table:       tableName,
			Description: "表缺少分区，建议按时间分区",
			SQL:         fmt.Sprintf("PARTITION BY %s", to.config.DefaultPartitionBy),
		})
	} else if !isRecommendedPartition(currentSettings.PartitionBy) {
		suggestions = append(suggestions, OptimizationSuggestion{
			Category:    "partition",
			Severity:    "warning",
			Table:       tableName,
			Description: fmt.Sprintf("当前分区 %s 可能不够优化，建议按天分区", currentSettings.PartitionBy),
			SQL:         fmt.Sprintf("PARTITION BY %s", to.config.DefaultPartitionBy),
		})
	}

	// 2. TTL 检查
	if currentSettings.TTLDays <= 0 {
		suggestions = append(suggestions, OptimizationSuggestion{
			Category:    "ttl",
			Severity:    "critical",
			Table:       tableName,
			Description: "表缺少 TTL 策略，数据将永久保留",
			SQL:         fmt.Sprintf("TTL timestamp_dt + INTERVAL 30 DAY DELETE"),
		})
	}

	// 3. 冷热分离检查
	if currentSettings.HotDays <= 0 {
		suggestions = append(suggestions, OptimizationSuggestion{
			Category:    "tier",
			Severity:    "warning",
			Table:       tableName,
			Description: "缺少冷热分离策略，建议将冷数据迁移到低成本存储",
			SQL:         fmt.Sprintf("TTL timestamp_dt + INTERVAL %d DAY TO VOLUME '%s', timestamp_dt + INTERVAL 30 DAY DELETE",
				to.config.DefaultHotDays, to.config.DefaultColdVolume),
		})
	}

	// 4. 索引检查
	hasTimestampIndex := false
	hasTenantIndex := false
	for _, col := range columns {
		if col.Name == "timestamp" && col.HasIndex {
			hasTimestampIndex = true
		}
		if col.Name == "tenant_id" && col.HasIndex {
			hasTenantIndex = true
		}
	}
	if !hasTimestampIndex {
		suggestions = append(suggestions, OptimizationSuggestion{
			Category:    "index",
			Severity:    "warning",
			Table:       tableName,
			Description: "timestamp 字段缺少索引，影响时间范围查询性能",
		})
	}
	if !hasTenantIndex {
		suggestions = append(suggestions, OptimizationSuggestion{
			Category:    "index",
			Severity:    "warning",
			Table:       tableName,
			Description: "tenant_id 字段缺少索引，影响多租户查询性能",
		})
	}

	// 5. 物化视图检查
	if !currentSettings.HasMaterializedView {
		suggestions = append(suggestions, OptimizationSuggestion{
			Category:    "materialized_view",
			Severity:    "info",
			Table:       tableName,
			Description: "建议创建物化视图预聚合高频查询",
		})
	}

	// 6. Projection 检查
	if !currentSettings.HasProjection {
		suggestions = append(suggestions, OptimizationSuggestion{
			Category:    "projection",
			Severity:    "info",
			Table:       tableName,
			Description: "建议创建 Projection 优化特定查询模式",
		})
	}

	return suggestions
}

// ColumnInfo 列信息
type ColumnInfo struct {
	Name     string
	Type     string
	HasIndex bool
	IsLowCardinality bool
}

// TableSettings 表设置
type TableSettings struct {
	PartitionBy          string
	TTLDays              int
	HotDays              int
	HasMaterializedView  bool
	HasProjection        bool
	IndexGranularity     int
}

func isRecommendedPartition(partitionBy string) bool {
	recommended := []string{
		"toYYYYMMDD(timestamp)",
		"toYYYYMM(timestamp)",
		"toMonday(timestamp)",
	}
	for _, p := range recommended {
		if strings.Contains(partitionBy, p) {
			return true
		}
	}
	return false
}

// ============================================================================
// DDL 生成器
// ============================================================================

// GenerateOptimizedTableDDL 生成优化的表 DDL
func (to *TableOptimizer) GenerateOptimizedTableDDL(tableName string, columns []ColumnInfo, partitionBy string, ttlDays int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n", tableName))

	for i, col := range columns {
		colType := col.Type
		if col.IsLowCardinality && isStringType(colType) {
			colType = "LowCardinality(" + colType + ")"
		}
		if i < len(columns)-1 {
			sb.WriteString(fmt.Sprintf("    %s %s,\n", col.Name, colType))
		} else {
			sb.WriteString(fmt.Sprintf("    %s %s\n", col.Name, colType))
		}
	}

	sb.WriteString(")\n")
	sb.WriteString(fmt.Sprintf("ENGINE = MergeTree()\n"))
	sb.WriteString(fmt.Sprintf("PARTITION BY %s\n", partitionBy))
	sb.WriteString(fmt.Sprintf("ORDER BY (tenant_id, timestamp, flow_id)\n"))
	sb.WriteString(fmt.Sprintf("SETTINGS index_granularity = %d\n", to.config.DefaultIndexGranularity))

	if ttlDays > 0 {
		ttlSQL := to.GenerateTTL(ttlDays, to.config.DefaultHotDays, to.config.DefaultColdVolume)
		sb.WriteString(ttlSQL + "\n")
	}

	return sb.String()
}

// GenerateTTL 生成 TTL 语句
func (to *TableOptimizer) GenerateTTL(totalDays, hotDays int, coldVolume string) string {
	if totalDays <= 0 {
		return ""
	}
	var parts []string
	if hotDays > 0 && hotDays < totalDays && coldVolume != "" && coldVolume != "default" {
		parts = append(parts, fmt.Sprintf("timestamp_dt + INTERVAL %d DAY TO VOLUME '%s'", hotDays, coldVolume))
	}
	parts = append(parts, fmt.Sprintf("timestamp_dt + INTERVAL %d DAY DELETE", totalDays))
	return fmt.Sprintf("TTL %s", strings.Join(parts, ", "))
}

// GenerateProjectionDDL 生成 Projection DDL
func (to *TableOptimizer) GenerateProjectionDDL(tableName, projectionName string, selectColumns []string, groupBy []string) string {
	return fmt.Sprintf(`
ALTER TABLE %s
ADD PROJECTION IF NOT EXISTS %s
(
    SELECT %s
    GROUP BY %s
);
`, tableName, projectionName, strings.Join(selectColumns, ", "), strings.Join(groupBy, ", "))
}

// GenerateMaterializedViewDDL 生成物化视图 DDL
func (to *TableOptimizer) GenerateMaterializedViewDDL(mvName, sourceTable, targetTable, selectSQL, groupBySQL string) string {
	return fmt.Sprintf(`
CREATE MATERIALIZED VIEW IF NOT EXISTS %s
TO %s
AS %s
GROUP BY %s;
`, mvName, targetTable, selectSQL, groupBySQL)
}

// GenerateIndexDDL 生成索引 DDL
func (to *TableOptimizer) GenerateIndexDDL(tableName, indexName, columnName, indexType string) string {
	return fmt.Sprintf("ALTER TABLE %s ADD INDEX IF NOT EXISTS %s %s TYPE %s;", tableName, indexName, columnName, indexType)
}

// ============================================================================
// 性能分析
// ============================================================================

// QueryPlan 查询计划
type QueryPlan struct {
	Table          string `json:"table"`
	PartitionPruned bool   `json:"partition_pruned"`
	IndexUsed      bool   `json:"index_used"`
	ProjectionUsed bool   `json:"projection_used"`
	EstimatedRows  int64  `json:"estimated_rows"`
	ReadRows       int64  `json:"read_rows"`
}

// AnalyzeQuery 分析查询性能
func (to *TableOptimizer) AnalyzeQuery(tableName string, queryPattern string, timeRange time.Duration) QueryPlan {
	plan := QueryPlan{
		Table: tableName,
	}

	// 时间范围查询可以触发分区裁剪
	if timeRange < 24*time.Hour {
		plan.PartitionPruned = true
	}

	// 简单查询模式分析
	if strings.Contains(queryPattern, "tenant_id") && strings.Contains(queryPattern, "timestamp") {
		plan.IndexUsed = true
	}

	// 估计读取行数（基于时间范围）
	plan.EstimatedRows = int64(timeRange.Hours() * 1000) // 假设每小时1000行
	plan.ReadRows = plan.EstimatedRows

	if plan.PartitionPruned {
		plan.ReadRows = plan.ReadRows / 10 // 分区裁剪减少90%读取
	}

	return plan
}

// GetOptimizationReport 生成优化报告
func (to *TableOptimizer) GetOptimizationReport(tables []string, tableSettings map[string]TableSettings, columns map[string][]ColumnInfo) map[string]interface{} {
	report := make(map[string]interface{})
	allSuggestions := make([]OptimizationSuggestion, 0)

	for _, table := range tables {
		settings := tableSettings[table]
		cols := columns[table]
		suggestions := to.AnalyzeTable(table, cols, settings)
		allSuggestions = append(allSuggestions, suggestions...)
		report[table] = suggestions
	}

	criticalCount := 0
	warningCount := 0
	infoCount := 0
	for _, s := range allSuggestions {
		switch s.Severity {
		case "critical":
			criticalCount++
		case "warning":
			warningCount++
		case "info":
			infoCount++
		}
	}

	return map[string]interface{}{
		"tables_analyzed":  len(tables),
		"total_issues":     len(allSuggestions),
		"critical":         criticalCount,
		"warning":          warningCount,
		"info":             infoCount,
		"details":          report,
	}
}

func isStringType(t string) bool {
	return strings.Contains(t, "String") || strings.Contains(t, "string")
}
