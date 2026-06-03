package clickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/pkg/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStorageBatchWrite 测试批量写入功能
func TestStorageBatchWrite(t *testing.T) {
	// Skip if no ClickHouse available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN:        "clickhouse://localhost:9000/cloudflow",
		MaxOpenConns: 10,
		MaxIdleConns: 5,
		BatchSize:    100,
		FlushInterval: 5 * time.Second,
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	// 创建测试数据
	flows := make([]*flow.UnifiedFlow, 10)
	for i := 0; i < 10; i++ {
		flows[i] = &flow.UnifiedFlow{
			Timestamp: time.Now().UnixNano(),
			SrcIP:     flow.IP{192, 168, 1, byte(i)},
			DstIP:     flow.IP{10, 0, 0, byte(i)},
			SrcPort:   uint16(8000 + i),
			DstPort:   80,
			Protocol:  flow.ProtoTCP,
		}
	}

	// 批量写入
	ctx := context.Background()
	err = storage.BatchWriteFlows(ctx, flows)
	assert.NoError(t, err)

	// 验证写入成功（查询刚写入的数据）
	count, err := storage.CountFlows(ctx, time.Now().Add(-1*time.Minute), time.Now())
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(10))
}

// TestStorageQueryFlows 测试流量查询
func TestStorageQueryFlows(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN:        "clickhouse://localhost:9000/cloudflow",
		MaxOpenConns: 10,
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	now := time.Now()

	// 测试时间范围查询
	flows, err := storage.QueryFlows(ctx, QueryOptions{
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now,
		Limit:     100,
	})
	assert.NoError(t, err)
	assert.NotNil(t, flows)

	// 测试过滤条件
	flows, err = storage.QueryFlows(ctx, QueryOptions{
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now,
		SrcIP:     "192.168.1.1",
		Limit:     100,
	})
	assert.NoError(t, err)

	// 测试排序
	flows, err = storage.QueryFlows(ctx, QueryOptions{
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now,
		OrderBy:   "timestamp",
		OrderDesc: true,
		Limit:     100,
	})
	assert.NoError(t, err)
}

// TestStorageFailover 测试故障转移
func TestStorageFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN:        "clickhouse://localhost:9000/cloudflow",
		AlternateDSNs: []string{
			"clickhouse://backup-host:9000/cloudflow",
		},
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	// 模拟主连接失败
	storage.db.Close()

	// 测试故障转移
	ctx := context.Background()
	success := storage.Failover()
	
	// 如果备用连接可用，应该成功
	if len(cfg.AlternateDSNs) > 0 {
		// 注意：实际环境中需要备用服务器运行
		t.Logf("Failover attempted, success: %v", success)
	}
}

// TestStorageInvalidOrderBy 测试 SQL 注入防护
func TestStorageInvalidOrderBy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN: "clickhouse://localhost:9000/cloudflow",
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	now := time.Now()

	// 测试无效的 ORDER BY（应该被拒绝）
	_, err = storage.QueryFlows(ctx, QueryOptions{
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now,
		OrderBy:   "timestamp; DROP TABLE flows;--", // SQL 注入尝试
		Limit:     100,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid order by field")
}

// TestStorageConnectionPool 测试连接池配置
func TestStorageConnectionPool(t *testing.T) {
	cfg := Config{
		DSN:          "clickhouse://localhost:9000/cloudflow",
		MaxOpenConns: 10,
		MaxIdleConns: 5,
		ConnMaxLifetime: 30 * time.Minute,
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	// 验证连接池统计
	stats := storage.DB().Stats()
	assert.Equal(t, 10, stats.MaxOpenConnections)
}

// TestStorageHealthCheck 测试健康检查
func TestStorageHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN: "clickhouse://localhost:9000/cloudflow",
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	
	// 测试健康检查
	err = storage.Ping(ctx)
	assert.NoError(t, err)
}

// TestStorageWriteTrace 测试 Trace 写入
func TestStorageWriteTrace(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN: "clickhouse://localhost:9000/cloudflow",
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()

	// 创建测试 trace
	trace := &flow.Trace{
		TraceID:   "test-trace-id-123",
		SpanID:    "span-456",
		Timestamp: time.Now().UnixNano(),
		ServiceName: "test-service",
		Operation:   "HTTP GET",
		Duration:  100 * time.Millisecond,
	}

	err = storage.WriteTrace(ctx, trace)
	assert.NoError(t, err)
}

// TestStorageAggregation 测试聚合查询
func TestStorageAggregation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN: "clickhouse://localhost:9000/cloudflow",
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	now := time.Now()

	// 测试按源 IP 聚合
	result, err := storage.AggregateFlows(ctx, AggregationOptions{
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now,
		GroupBy:   "src_ip",
		Metrics:   []string{"count", "sum_bytes"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, result)
}
