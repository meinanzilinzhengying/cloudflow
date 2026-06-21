//go:build linux

package optimizer

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDefaultOptimizerConfig(t *testing.T) {
	cfg := DefaultOptimizerConfig()
	if cfg.DefaultPartitionBy != "toYYYYMMDD(timestamp)" {
		t.Errorf("unexpected partitionBy: %s", cfg.DefaultPartitionBy)
	}
	if cfg.DefaultHotDays != 7 {
		t.Errorf("unexpected hotDays: %d", cfg.DefaultHotDays)
	}
	if cfg.DefaultIndexGranularity != 8192 {
		t.Errorf("unexpected granularity: %d", cfg.DefaultIndexGranularity)
	}
}

func TestAnalyzeTable(t *testing.T) {
	opt := NewTableOptimizer(nil)
	columns := []ColumnInfo{
		{Name: "timestamp", Type: "DateTime", HasIndex: true},
		{Name: "tenant_id", Type: "String", HasIndex: false, IsLowCardinality: true},
		{Name: "flow_id", Type: "UInt32", HasIndex: false},
	}
	settings := TableSettings{
		PartitionBy:         "",
		TTLDays:             0,
		HotDays:             0,
		HasMaterializedView: false,
		HasProjection:       false,
	}

	suggestions := opt.AnalyzeTable("flows", columns, settings)
	if len(suggestions) == 0 {
		t.Fatal("expected optimization suggestions")
	}

	var hasPartition, hasTTL, hasTier, hasIndex bool
	for _, s := range suggestions {
		switch s.Category {
		case "partition":
			hasPartition = true
			if s.Severity != "critical" {
				t.Errorf("expected critical severity for partition, got %s", s.Severity)
			}
		case "ttl":
			hasTTL = true
			if s.Severity != "critical" {
				t.Errorf("expected critical severity for ttl, got %s", s.Severity)
			}
		case "tier":
			hasTier = true
		case "index":
			hasIndex = true
		}
	}
	if !hasPartition {
		t.Error("expected partition suggestion")
	}
	if !hasTTL {
		t.Error("expected TTL suggestion")
	}
	if !hasTier {
		t.Error("expected tier suggestion")
	}
	if !hasIndex {
		t.Error("expected index suggestion")
	}
}

func TestAnalyzeTableOptimized(t *testing.T) {
	opt := NewTableOptimizer(nil)
	columns := []ColumnInfo{
		{Name: "timestamp", Type: "DateTime", HasIndex: true},
		{Name: "tenant_id", Type: "String", HasIndex: true, IsLowCardinality: true},
	}
	settings := TableSettings{
		PartitionBy:         "toYYYYMMDD(timestamp)",
		TTLDays:             30,
		HotDays:             7,
		HasMaterializedView: true,
		HasProjection:       true,
	}

	suggestions := opt.AnalyzeTable("flows", columns, settings)
	if len(suggestions) > 0 {
		t.Errorf("expected no suggestions for optimized table, got %d: %v", len(suggestions), suggestions)
	}
}

func TestGenerateOptimizedTableDDL(t *testing.T) {
	opt := NewTableOptimizer(nil)
	columns := []ColumnInfo{
		{Name: "timestamp", Type: "DateTime"},
		{Name: "tenant_id", Type: "String", IsLowCardinality: true},
		{Name: "flow_id", Type: "UInt32"},
	}
	ddl := opt.GenerateOptimizedTableDDL("flows", columns, "toYYYYMMDD(timestamp)", 30)
	if ddl == "" {
		t.Fatal("expected non-empty DDL")
	}
	if !contains(ddl, "CREATE TABLE IF NOT EXISTS flows") {
		t.Error("expected CREATE TABLE statement")
	}
	if !contains(ddl, "PARTITION BY toYYYYMMDD(timestamp)") {
		t.Error("expected PARTITION BY clause")
	}
	if !contains(ddl, "LowCardinality(String)") {
		t.Error("expected LowCardinality for String")
	}
	if !contains(ddl, "TTL") {
		t.Error("expected TTL clause")
	}
}

func TestGenerateTTL(t *testing.T) {
	opt := NewTableOptimizer(nil)

	tests := []struct {
		totalDays int
		hotDays   int
		coldVol   string
		want      string
	}{
		{totalDays: 0, hotDays: 0, coldVol: "", want: ""},
		{totalDays: 30, hotDays: 0, coldVol: "", want: "TTL timestamp_dt + INTERVAL 30 DAY DELETE"},
		{totalDays: 30, hotDays: 7, coldVol: "cold", want: "TTL timestamp_dt + INTERVAL 7 DAY TO VOLUME 'cold', timestamp_dt + INTERVAL 30 DAY DELETE"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("ttl_%d_%d", tt.totalDays, tt.hotDays), func(t *testing.T) {
			got := opt.GenerateTTL(tt.totalDays, tt.hotDays, tt.coldVol)
			if got != tt.want {
				t.Errorf("GenerateTTL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateProjectionDDL(t *testing.T) {
	opt := NewTableOptimizer(nil)
	ddl := opt.GenerateProjectionDDL("flows", "proj_tenant", []string{"tenant_id", "count()"}, []string{"tenant_id"})
	if !contains(ddl, "ADD PROJECTION") {
		t.Error("expected ADD PROJECTION")
	}
	if !contains(ddl, "proj_tenant") {
		t.Error("expected projection name")
	}
}

func TestGenerateMaterializedViewDDL(t *testing.T) {
	opt := NewTableOptimizer(nil)
	ddl := opt.GenerateMaterializedViewDDL("mv_flows_1m", "flows", "flows_1m", "SELECT tenant_id, toStartOfMinute(timestamp) as minute, count()", "tenant_id, minute")
	if !contains(ddl, "CREATE MATERIALIZED VIEW") {
		t.Error("expected CREATE MATERIALIZED VIEW")
	}
	if !contains(ddl, "mv_flows_1m") {
		t.Error("expected materialized view name")
	}
}

func TestAnalyzeQuery(t *testing.T) {
	opt := NewTableOptimizer(nil)
	plan := opt.AnalyzeQuery("flows", "tenant_id = ? AND timestamp > ?", 1*time.Hour)
	if plan.Table != "flows" {
		t.Errorf("expected table flows, got %s", plan.Table)
	}
	if !plan.PartitionPruned {
		t.Error("expected partition pruned for 1h range")
	}
	if plan.ReadRows >= plan.EstimatedRows {
		t.Errorf("expected read rows < estimated rows after pruning")
	}
}

func TestGetOptimizationReport(t *testing.T) {
	opt := NewTableOptimizer(nil)
	tables := []string{"flows", "traces"}
	settings := map[string]TableSettings{
		"flows": {
			PartitionBy: "toYYYYMMDD(timestamp)",
			TTLDays:     30,
			HotDays:     7,
		},
		"traces": {
			PartitionBy: "",
			TTLDays:     0,
			HotDays:     0,
		},
	}
	columns := map[string][]ColumnInfo{
		"flows": {
			{Name: "timestamp", Type: "DateTime", HasIndex: true},
			{Name: "tenant_id", Type: "String", HasIndex: true},
		},
		"traces": {
			{Name: "timestamp", Type: "DateTime", HasIndex: false},
			{Name: "tenant_id", Type: "String", HasIndex: false},
		},
	}

	report := opt.GetOptimizationReport(tables, settings, columns)
	if report["tables_analyzed"] != 2 {
		t.Errorf("expected 2 tables analyzed, got %v", report["tables_analyzed"])
	}
	if report["total_issues"].(int) <= 0 {
		t.Errorf("expected issues, got %v", report["total_issues"])
	}
	if report["critical"].(int) <= 0 {
		t.Errorf("expected critical issues, got %v", report["critical"])
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
