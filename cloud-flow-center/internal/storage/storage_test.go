// Package storage 存储引擎测试

package storage

import (
	"context"
	"testing"
	"time"

	"cloud-flow/cloud-flow-center/internal/storage/clickhouse"
	"cloud-flow/cloud-flow-center/internal/storage/clickhouse/schema"
)

// ============================================================================
// Router 测试
// ============================================================================

func TestRouter(t *testing.T) {
	router := NewRouter(DefaultRouterConfig())

	// 测试注册
	t.Run("Register", func(t *testing.T) {
		// 注册 mock backend
		backend := &mockBackend{dataType: DataTypeFlow}
		if err := router.Register(DataTypeFlow, backend); err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// 重复注册应该失败
		if err := router.Register(DataTypeFlow, backend); err == nil {
			t.Error("Expected error for duplicate registration")
		}
	})

	// 测试获取后端
	t.Run("GetBackend", func(t *testing.T) {
		backend, exists := router.GetBackend(DataTypeFlow)
		if !exists {
			t.Fatal("Expected backend to exist")
		}
		if backend.Name() != "mock" {
			t.Errorf("Expected name 'mock', got '%s'", backend.Name())
		}
	})

	// 测试租户管理
	t.Run("TenantManagement", func(t *testing.T) {
		tenant := &TenantConfig{
			TenantID:      "tenant-1",
			EnabledTypes:  []DataType{DataTypeFlow, DataTypeMetrics},
			RetentionDays: map[DataType]int{DataTypeFlow: 30},
		}
		router.AddTenant(tenant)

		config, exists := router.GetTenant("tenant-1")
		if !exists {
			t.Fatal("Expected tenant to exist")
		}
		if config.TenantID != "tenant-1" {
			t.Errorf("Expected tenant ID 'tenant-1', got '%s'", config.TenantID)
		}

		tenants := router.ListTenants()
		if len(tenants) != 1 {
			t.Errorf("Expected 1 tenant, got %d", len(tenants))
		}

		router.RemoveTenant("tenant-1")
		if _, exists := router.GetTenant("tenant-1"); exists {
			t.Error("Expected tenant to be removed")
		}
	})
}

// mockBackend 模拟存储后端
type mockBackend struct {
	dataType DataType
	ready    bool
}

func (m *mockBackend) Name() string                          { return "mock" }
func (m *mockBackend) Type() DataType                        { return m.dataType }
func (m *mockBackend) Ready() bool                           { return m.ready }
func (m *mockBackend) Write(ctx context.Context, data interface{}) error { return nil }
func (m *mockBackend) WriteBatch(ctx context.Context, batch []interface{}) error { return nil }
func (m *mockBackend) Query(ctx context.Context, req *QueryRequest) (*QueryResult, error) {
	return &QueryResult{}, nil
}
func (m *mockBackend) Close() error { return nil }

// ============================================================================
// ClickHouse Schema 测试
// ============================================================================

func TestClickHouseSchema(t *testing.T) {
	cfg := schema.DefaultSchemaConfig()

	t.Run("GenerateFlowsTable", func(t *testing.T) {
		ddl := schema.GenerateCreateFlowsTable(cfg)
		if ddl == "" {
			t.Error("Expected non-empty DDL")
		}
		// 检查关键元素
		if !contains(ddl, "MergeTree") {
			t.Error("Expected MergeTree engine")
		}
		if !contains(ddl, "PARTITION BY") {
			t.Error("Expected PARTITION BY clause")
		}
		if !contains(ddl, "ORDER BY") {
			t.Error("Expected ORDER BY clause")
		}
		if !contains(ddl, "TTL") {
			t.Error("Expected TTL clause")
		}
	})

	t.Run("GenerateAggregationTables", func(t *testing.T) {
		ddl := schema.GenerateCreateAggregationTables(cfg)
		if ddl == "" {
			t.Error("Expected non-empty DDL")
		}
		if !contains(ddl, "SummingMergeTree") {
			t.Error("Expected SummingMergeTree engine")
		}
	})

	t.Run("GenerateMaterializedViews", func(t *testing.T) {
		ddl := schema.GenerateCreateMaterializedViews(cfg)
		if ddl == "" {
			t.Error("Expected non-empty DDL")
		}
		if !contains(ddl, "MATERIALIZED VIEW") {
			t.Error("Expected MATERIALIZED VIEW")
		}
	})

	t.Run("GenerateIndexes", func(t *testing.T) {
		ddl := schema.GenerateCreateIndexes(cfg)
		if ddl == "" {
			t.Error("Expected non-empty DDL")
		}
		if !contains(ddl, "bloom_filter") {
			t.Error("Expected bloom_filter index")
		}
		if !contains(ddl, "set") {
			t.Error("Expected set index")
		}
	})

	t.Run("GenerateAllDDL", func(t *testing.T) {
		ddl := schema.GenerateAllDDL(cfg)
		if ddl == "" {
			t.Error("Expected non-empty DDL")
		}
		// 检查所有表都生成了
		if !contains(ddl, "CREATE TABLE") {
			t.Error("Expected CREATE TABLE statements")
		}
	})
}

// ============================================================================
// ClickHouse Storage 测试
// ============================================================================

func TestClickHouseConfig(t *testing.T) {
	cfg := clickhouse.DefaultConfig()

	if cfg.Addr == "" {
		t.Error("Expected non-empty Addr")
	}
	if cfg.Database == "" {
		t.Error("Expected non-empty Database")
	}
	if cfg.BatchSize <= 0 {
		t.Error("Expected positive BatchSize")
	}
	if cfg.WorkerCount <= 0 {
		t.Error("Expected positive WorkerCount")
	}
}

// ============================================================================
// Query 测试
// ============================================================================

func TestQueryRequest(t *testing.T) {
	req := &QueryRequest{
		DataType:  DataTypeFlow,
		TenantID:  "tenant-1",
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		SrcIP:     "10.0.0.1",
		DstIP:     "10.0.0.2",
		Namespace: "default",
		Service:   "api-server",
		Limit:     100,
	}

	if req.DataType != DataTypeFlow {
		t.Errorf("Expected DataTypeFlow, got %v", req.DataType)
	}
	if req.TenantID != "tenant-1" {
		t.Errorf("Expected tenant-1, got %s", req.TenantID)
	}
}

func TestTopologyQuery(t *testing.T) {
	req := &QueryRequest{
		DataType:  DataTypeFlow,
		TenantID:  "tenant-1",
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		Topology: &TopologyConfig{
			Type:     TopologyService,
			GroupBy:  []string{"namespace", "service"},
			MaxDepth: 3,
		},
	}

	if req.Topology == nil {
		t.Fatal("Expected Topology config")
	}
	if req.Topology.Type != TopologyService {
		t.Errorf("Expected TopologyService, got %v", req.Topology.Type)
	}
}

// ============================================================================
// 性能测试
// ============================================================================

func BenchmarkRouterRoute(b *testing.B) {
	router := NewRouter(DefaultRouterConfig())
	router.Register(DataTypeFlow, &mockBackend{dataType: DataTypeFlow, ready: true})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		router.Route(context.Background(), DataTypeFlow, "test")
	}
}

func BenchmarkSchemaGeneration(b *testing.B) {
	cfg := schema.DefaultSchemaConfig()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		schema.GenerateAllDDL(cfg)
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && 
		(s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
