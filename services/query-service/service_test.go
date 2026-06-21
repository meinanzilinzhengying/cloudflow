package queryservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meinanzilinzhengying/cloudflow/pkg/storage"
	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
)

// ============================================================================
// Manual Mocks for storage interfaces
// ============================================================================

// mockRows implements storage.Rows

type mockRows struct {
	columns    []string
	data       [][]interface{}
	currentRow int
	scanErr    error
	nextErr    error
	closeErr   error
	colsErr    error
}

func newMockRows(columns []string, data [][]interface{}) *mockRows {
	return &mockRows{
		columns:    columns,
		data:       data,
		currentRow: -1,
	}
}

func (m *mockRows) Next() bool {
	if m.nextErr != nil {
		return false
	}
	m.currentRow++
	return m.currentRow < len(m.data)
}

func (m *mockRows) Scan(dest ...interface{}) error {
	if m.scanErr != nil {
		return m.scanErr
	}
	if m.currentRow < 0 || m.currentRow >= len(m.data) {
		return errors.New("no current row")
	}
	row := m.data[m.currentRow]
	if len(dest) != len(row) {
		return fmt.Errorf("scan argument count mismatch: expected %d, got %d", len(row), len(dest))
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *interface{}:
			*ptr = row[i]
		case *string:
			if v, ok := row[i].(string); ok {
				*ptr = v
			}
		case *int64:
			if v, ok := row[i].(int64); ok {
				*ptr = v
			}
		case *uint64:
			if v, ok := row[i].(uint64); ok {
				*ptr = v
			}
		case *float64:
			if v, ok := row[i].(float64); ok {
				*ptr = v
			}
		case *time.Time:
			if v, ok := row[i].(time.Time); ok {
				*ptr = v
			}
		default:
			// Try to assign via json marshal/unmarshal for interface types
			// or just copy if types match
		}
	}
	return nil
}

func (m *mockRows) Close() error {
	return m.closeErr
}

func (m *mockRows) Err() error {
	return nil
}

func (m *mockRows) Columns() ([]string, error) {
	if m.colsErr != nil {
		return nil, m.colsErr
	}
	return m.columns, nil
}

// mockRow implements storage.Row

type mockRow struct {
	scanErr error
	values  []interface{}
}

func (m *mockRow) Scan(dest ...interface{}) error {
	if m.scanErr != nil {
		return m.scanErr
	}
	for i, d := range dest {
		if i >= len(m.values) {
			return errors.New("not enough values")
		}
		switch ptr := d.(type) {
		case *interface{}:
			*ptr = m.values[i]
		case *string:
			if v, ok := m.values[i].(string); ok {
				*ptr = v
			}
		case *uint64:
			if v, ok := m.values[i].(uint64); ok {
				*ptr = v
			}
		case *float64:
			if v, ok := m.values[i].(float64); ok {
				*ptr = v
			}
		case *int64:
			if v, ok := m.values[i].(int64); ok {
				*ptr = v
			}
		default:
		}
	}
	return nil
}

// mockResult implements storage.Result

type mockResult struct{}

func (m *mockResult) LastInsertId() (int64, error) { return 0, nil }
func (m *mockResult) RowsAffected() (int64, error) { return 0, nil }

// mockTimeSeriesStorage implements storage.TimeSeriesStorage

type mockTimeSeriesStorage struct {
	queryFunc     func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error)
	queryRowFunc  func(ctx context.Context, sql string, args ...interface{}) storage.Row
	execFunc      func(ctx context.Context, sql string, args ...interface{}) (storage.Result, error)
	pingErr       error
	dbType        storage.DatabaseType
	queryCalls    []queryCall
	queryRowCalls []queryRowCall
}

type queryCall struct {
	sql  string
	args []interface{}
}

type queryRowCall struct {
	sql  string
	args []interface{}
}

func (m *mockTimeSeriesStorage) InsertFlow(ctx context.Context, flow *storage.Flow) error {
	return nil
}
func (m *mockTimeSeriesStorage) InsertFlows(ctx context.Context, flows []*storage.Flow) error {
	return nil
}
func (m *mockTimeSeriesStorage) QueryFlows(ctx context.Context, query *storage.FlowQuery) ([]*storage.Flow, error) {
	return nil, nil
}
func (m *mockTimeSeriesStorage) AggregateFlows(ctx context.Context, agg *storage.FlowAggregate) ([]*storage.AggregateResult, error) {
	return nil, nil
}
func (m *mockTimeSeriesStorage) Exec(ctx context.Context, sql string, args ...interface{}) (storage.Result, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return &mockResult{}, nil
}
func (m *mockTimeSeriesStorage) Query(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
	m.queryCalls = append(m.queryCalls, queryCall{sql: sql, args: args})
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return newMockRows(nil, nil), nil
}
func (m *mockTimeSeriesStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) storage.Row {
	m.queryRowCalls = append(m.queryRowCalls, queryRowCall{sql: sql, args: args})
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return &mockRow{}
}
func (m *mockTimeSeriesStorage) Ping(ctx context.Context) error {
	return m.pingErr
}
func (m *mockTimeSeriesStorage) PingContext(ctx context.Context) error {
	return m.pingErr
}
func (m *mockTimeSeriesStorage) Close() error {
	return nil
}
func (m *mockTimeSeriesStorage) RawDB() *sql.DB {
	return nil
}
func (m *mockTimeSeriesStorage) Type() storage.DatabaseType {
	if m.dbType != "" {
		return m.dbType
	}
	return storage.DatabaseClickHouse
}

// ============================================================================
// Helper functions
// ============================================================================

func newTestService(tsDB storage.TimeSeriesStorage) *Service {
	cfg := DefaultConfig()
	s := &Service{
		config:    cfg,
		startTime: time.Now(),
	}
	if tsDB != nil {
		s.tsDB = tsDB
	}
	return s
}

func assertQueryContains(t *testing.T, calls []queryCall, expected string) {
	require.NotEmpty(t, calls, "expected at least one query call")
	assert.Contains(t, calls[0].sql, expected, "query SQL should contain %q", expected)
}

// ============================================================================
// Unit tests for QueryFlows
// ============================================================================

func TestQueryFlows_NoFilters(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "FROM ebpf_events")
			assert.Contains(t, sql, "ORDER BY timestamp DESC")
			assert.Contains(t, sql, "LIMIT 1000")
			assert.Len(t, args, 1)
			assert.Equal(t, "test-tenant", args[0])
			return newMockRows([]string{"src_ip", "dst_ip", "timestamp"}, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	resp, err := s.QueryFlows(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Records)
	assert.Equal(t, int64(0), resp.Total)
	assert.GreaterOrEqual(t, resp.TookMs, int64(0))
}

func TestQueryFlows_SrcIPFilter(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "src_ip = ?")
			assert.Len(t, args, 2)
			assert.Equal(t, "test-tenant", args[0])
			assert.Equal(t, "10.0.0.1", args[1])
			return newMockRows([]string{"src_ip", "dst_ip", "timestamp"}, [][]interface{}{
				{"10.0.0.1", "10.0.0.2", uint64(1700000000000000000)},
			}), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant", SrcIp: "10.0.0.1"}
	resp, err := s.QueryFlows(context.Background(), req)

	require.NoError(t, err)
	require.Len(t, resp.Records, 1)
	assert.Equal(t, "10.0.0.1", resp.Records[0]["src_ip"])
	assert.Equal(t, int64(1), resp.Total)
}

func TestQueryFlows_DstIPFilter(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "dst_ip = ?")
			assert.Len(t, args, 2)
			assert.Equal(t, "test-tenant", args[0])
			assert.Equal(t, "192.168.1.1", args[1])
			return newMockRows([]string{"src_ip", "dst_ip", "timestamp"}, [][]interface{}{
				{"10.0.0.1", "192.168.1.1", uint64(1700000000000000000)},
			}), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant", DstIp: "192.168.1.1"}
	resp, err := s.QueryFlows(context.Background(), req)

	require.NoError(t, err)
	require.Len(t, resp.Records, 1)
	assert.Equal(t, "192.168.1.1", resp.Records[0]["dst_ip"])
}

func TestQueryFlows_TimeRange(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "timestamp >= ?")
			assert.Contains(t, sql, "timestamp <= ?")
			assert.Len(t, args, 3)
			assert.Equal(t, "default", args[0])
			assert.Equal(t, int64(1700000000000000000), args[1])
			assert.Equal(t, int64(1700003600000000000), args[2])
			return newMockRows([]string{"timestamp"}, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{
		StartTime: 1700000000000000000,
		EndTime:   1700003600000000000,
	}
	resp, err := s.QueryFlows(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Records)
}

func TestQueryFlows_EmptyResults(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return newMockRows([]string{"src_ip", "dst_ip"}, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "tenant-1"}
	resp, err := s.QueryFlows(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Records)
	assert.Equal(t, int64(0), resp.Total)
	assert.Equal(t, int64(0), resp.TookMs)
}

func TestQueryFlows_DatabaseError(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return nil, errors.New("connection refused")
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	_, err := s.QueryFlows(context.Background(), req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "query flows failed")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestQueryFlows_NoDBConnection(t *testing.T) {
	s := newTestService(nil)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	resp, err := s.QueryFlows(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Records)
	assert.Equal(t, int64(0), resp.Total)
	assert.GreaterOrEqual(t, resp.TookMs, int64(0))
}

// ============================================================================
// Unit tests for QueryMetrics
// ============================================================================

func TestQueryMetrics_BasicQuery(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "FROM ebpf_events")
			assert.Contains(t, sql, "LIMIT 1000")
			return newMockRows([]string{"probe_id", "bytes"}, [][]interface{}{
				{"probe-1", uint64(1024)},
				{"probe-2", uint64(2048)},
			}), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{Service: "myservice"}
	resp, err := s.QueryMetrics(context.Background(), req)

	require.NoError(t, err)
	require.Len(t, resp.Records, 2)
	assert.Equal(t, int64(2), resp.Total)
	assert.Equal(t, "probe-1", resp.Records[0]["probe_id"])
}

func TestQueryMetrics_EmptyResults(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return newMockRows([]string{"probe_id"}, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "empty-tenant"}
	resp, err := s.QueryMetrics(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Records)
	assert.Equal(t, int64(0), resp.Total)
}

func TestQueryMetrics_DatabaseError(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return nil, errors.New("clickhouse timeout")
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	_, err := s.QueryMetrics(context.Background(), req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "query metrics failed")
	assert.Contains(t, err.Error(), "clickhouse timeout")
}

func TestQueryMetrics_NoDBConnection(t *testing.T) {
	s := newTestService(nil)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	resp, err := s.QueryMetrics(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Records)
	assert.Equal(t, int64(0), resp.Total)
}

// ============================================================================
// Unit tests for QueryDashboard
// ============================================================================

func TestQueryDashboard_AggregateQueries(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "FROM ebpf_events")
			// Return different mock data based on query name pattern
			if strings.Contains(sql, "count() as count") {
				return newMockRows([]string{"count", "date"}, [][]interface{}{
					{uint64(1000), time.Now()},
				}), nil
			}
			if strings.Contains(sql, "top_talkers") || (strings.Contains(sql, "src_ip") && strings.Contains(sql, "dst_ip")) {
				return newMockRows([]string{"src_ip", "dst_ip", "total_bytes", "flow_count"}, [][]interface{}{
					{"10.0.0.1", "10.0.0.2", uint64(1024000), uint64(100)},
				}), nil
			}
			if strings.Contains(sql, "error_rate") {
				return newMockRows([]string{"service", "error_count", "total_count", "error_rate"}, [][]interface{}{
					{"api-gateway", uint64(10), uint64(100), float64(10.0)},
				}), nil
			}
			if strings.Contains(sql, "quantile(0.95)") {
				return newMockRows([]string{"service", "p95_latency"}, [][]interface{}{
					{"api-gateway", uint64(1500000)},
				}), nil
			}
			return newMockRows(nil, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "tenant-1"}
	resp, err := s.QueryDashboard(context.Background(), req)

	require.NoError(t, err)
	require.Len(t, resp.Records, 1)
	require.Contains(t, resp.Records[0], "dashboard")

	dashboard := resp.Records[0]["dashboard"].(map[string]interface{})
	require.Contains(t, dashboard, "flow_count")
	require.Contains(t, dashboard, "top_talkers")
	require.Contains(t, dashboard, "error_rate")
	require.Contains(t, dashboard, "latency_p95")
}

func TestQueryDashboard_ErrorRateCalculation(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			if strings.Contains(sql, "error_rate") {
				assert.Contains(t, sql, "category = 'security'")
				assert.Contains(t, sql, "error_rate")
				return newMockRows([]string{"service", "error_count", "total_count", "error_rate"}, [][]interface{}{
					{"web-service", uint64(5), uint64(50), float64(10.0)},
				}), nil
			}
			return newMockRows(nil, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	resp, err := s.QueryDashboard(context.Background(), req)

	require.NoError(t, err)
	dashboard := resp.Records[0]["dashboard"].(map[string]interface{})
	errorRateRecords := dashboard["error_rate"].([]map[string]interface{})
	require.Len(t, errorRateRecords, 1)
	assert.Equal(t, "web-service", errorRateRecords[0]["service"])
	assert.Equal(t, uint64(5), errorRateRecords[0]["error_count"])
	assert.Equal(t, uint64(50), errorRateRecords[0]["total_count"])
	assert.Equal(t, float64(10.0), errorRateRecords[0]["error_rate"])
}

func TestQueryDashboard_LatencyP95(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			if strings.Contains(sql, "quantile(0.95)") {
				assert.Contains(t, sql, "latency_ns")
				return newMockRows([]string{"service", "p95_latency"}, [][]interface{}{
					{"backend", uint64(2500000)},
				}), nil
			}
			return newMockRows(nil, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	resp, err := s.QueryDashboard(context.Background(), req)

	require.NoError(t, err)
	dashboard := resp.Records[0]["dashboard"].(map[string]interface{})
	latencyRecords := dashboard["latency_p95"].([]map[string]interface{})
	require.Len(t, latencyRecords, 1)
	assert.Equal(t, "backend", latencyRecords[0]["service"])
	assert.Equal(t, uint64(2500000), latencyRecords[0]["p95_latency"])
}

func TestQueryDashboard_NoDBConnection(t *testing.T) {
	s := newTestService(nil)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	resp, err := s.QueryDashboard(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Records)
	assert.Equal(t, int64(0), resp.Total)
}

// ============================================================================
// Unit tests for overviewHandler
// ============================================================================

func TestOverviewHandler_ResponseStructure(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryRowFunc: func(ctx context.Context, sql string, args ...interface{}) storage.Row {
			if strings.Contains(sql, "count()") {
				return &mockRow{values: []interface{}{uint64(1500)}}
			}
			if strings.Contains(sql, "count(DISTINCT probe_id)") {
				return &mockRow{values: []interface{}{uint64(5)}}
			}
			if strings.Contains(sql, "count(DISTINCT dst_ip)") {
				return &mockRow{values: []interface{}{uint64(10)}}
			}
			if strings.Contains(sql, "JSONExtractFloat") {
				if strings.Contains(sql, "cpu_percent") {
					return &mockRow{values: []interface{}{45.5}}
				}
				if strings.Contains(sql, "memory_percent") {
					return &mockRow{values: []interface{}{62.3}}
				}
				if strings.Contains(sql, "net_rx_bytes") {
					return &mockRow{values: []interface{}{float64(15000000)}}
				}
				if strings.Contains(sql, "disk_percent") {
					return &mockRow{values: []interface{}{30.1}}
				}
			}
			if strings.Contains(sql, "avg(latency_ms)") {
				return &mockRow{values: []interface{}{12.5}}
			}
			return &mockRow{values: []interface{}{uint64(0)}}
		},
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			if strings.Contains(sql, "toStartOfMinute") {
				return newMockRows([]string{"t", "total"}, [][]interface{}{
					{time.Now(), int64(1000)},
				}), nil
			}
			if strings.Contains(sql, "dst_ip, count()") {
				return newMockRows([]string{"dst_ip", "cnt"}, [][]interface{}{
					{"svc-1", uint64(500)},
					{"svc-2", uint64(300)},
				}), nil
			}
			return newMockRows(nil, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := httptest.NewRequest(http.MethodGet, "/overview", nil)
	w := httptest.NewRecorder()

	s.overviewHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, float64(1500), result["totalFlows"])
	assert.Equal(t, float64(5), result["activeAgents"])
	assert.Equal(t, float64(10), result["activeServices"])

	// Check traffic data
	assert.NotEmpty(t, result["trafficLabels"])
	assert.NotEmpty(t, result["trafficInbound"])
	assert.Equal(t, []interface{}{}, result["trafficOutbound"])

	// Check top services
	assert.NotEmpty(t, result["topServices"])
	topServices := result["topServices"].([]interface{})
	assert.Len(t, topServices, 2)

	// Check avgLatency
	assert.Equal(t, 12.5, result["avgLatency"])
}

func TestOverviewHandler_ResourcesExtraction(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryRowFunc: func(ctx context.Context, sql string, args ...interface{}) storage.Row {
			if strings.Contains(sql, "cpu_percent") {
				return &mockRow{values: []interface{}{75.0}}
			}
			if strings.Contains(sql, "memory_percent") {
				return &mockRow{values: []interface{}{80.0}}
			}
			if strings.Contains(sql, "net_rx_bytes") {
				return &mockRow{values: []interface{}{float64(5000000)}}
			}
			if strings.Contains(sql, "disk_percent") {
				return &mockRow{values: []interface{}{45.0}}
			}
			return &mockRow{values: []interface{}{uint64(0)}}
		},
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return newMockRows(nil, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := httptest.NewRequest(http.MethodGet, "/overview", nil)
	w := httptest.NewRecorder()

	s.overviewHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	resources := result["resources"].([]interface{})
	require.Len(t, resources, 4)

	cpu := resources[0].(map[string]interface{})
	assert.Equal(t, "CPU", cpu["name"])
	assert.Equal(t, 75.0, cpu["percentage"])
	assert.Equal(t, "stroke-primary-500", cpu["color"])

	mem := resources[1].(map[string]interface{})
	assert.Equal(t, "Memory", mem["name"])
	assert.Equal(t, 80.0, mem["percentage"])

	netio := resources[2].(map[string]interface{})
	assert.Equal(t, "NetworkIO", netio["name"])
	assert.Equal(t, 5.0, netio["percentage"]) // 5000000 / 1000000.0

	disk := resources[3].(map[string]interface{})
	assert.Equal(t, "Disk", disk["name"])
	assert.Equal(t, 45.0, disk["percentage"])
}

func TestOverviewHandler_MissingClickHouseConnection(t *testing.T) {
	s := newTestService(nil)
	req := httptest.NewRequest(http.MethodGet, "/overview", nil)
	w := httptest.NewRecorder()

	s.overviewHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, float64(0), result["totalFlows"])
	assert.Equal(t, float64(0), result["activeAgents"])
	assert.Equal(t, float64(0), result["activeServices"])
	assert.Equal(t, []interface{}{}, result["trafficLabels"])
	assert.Equal(t, []interface{}{}, result["topServices"])
	assert.Equal(t, []interface{}{}, result["resources"])
	assert.Equal(t, float64(0), result["avgLatency"])
}

// ============================================================================
// Unit tests for QueryTraces
// ============================================================================

func TestQueryTraces_BasicQuery(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "FROM traces")
			assert.Contains(t, sql, "LIMIT 100")
			return newMockRows([]string{"trace_id", "service"}, [][]interface{}{
				{"trace-1", "service-a"},
			}), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{Service: "service-a"}
	resp, err := s.QueryTraces(context.Background(), req)

	require.NoError(t, err)
	require.Len(t, resp.Records, 1)
	assert.Equal(t, "trace-1", resp.Records[0]["trace_id"])
	assert.Equal(t, "service-a", resp.Records[0]["service"])
	assert.Equal(t, int64(1), resp.Total)
}

func TestQueryTraces_EmptyResults(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return newMockRows([]string{"trace_id"}, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	resp, err := s.QueryTraces(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Records)
	assert.Equal(t, int64(0), resp.Total)
}

func TestQueryTraces_DatabaseError(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return nil, errors.New("traces table not found")
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	_, err := s.QueryTraces(context.Background(), req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "query traces failed")
	assert.Contains(t, err.Error(), "traces table not found")
}

func TestQueryTraces_NoDBConnection(t *testing.T) {
	s := newTestService(nil)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	resp, err := s.QueryTraces(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Records)
	assert.Equal(t, int64(0), resp.Total)
}

// ============================================================================
// Unit tests for HTTP handlers (flows, metrics, traces)
// ============================================================================

func TestFlowsHandler(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return newMockRows([]string{"src_ip"}, [][]interface{}{
				{"10.0.0.1"},
			}), nil
		},
	}

	s := newTestService(mockDB)
	req := httptest.NewRequest(http.MethodGet, "/flows?src_ip=10.0.0.1&limit=10", nil)
	w := httptest.NewRecorder()

	s.flowsHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var result svcproto.QueryFlowResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Len(t, result.Records, 1)
}

func TestMetricsHandler(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return newMockRows([]string{"probe_id"}, [][]interface{}{
				{"probe-1"},
			}), nil
		},
	}

	s := newTestService(mockDB)
	req := httptest.NewRequest(http.MethodGet, "/metrics-data?service=test&limit=5", nil)
	w := httptest.NewRecorder()

	s.metricsHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var result svcproto.QueryFlowResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Len(t, result.Records, 1)
}

func TestTracesHandler(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return newMockRows([]string{"trace_id"}, [][]interface{}{
				{"trace-abc"},
			}), nil
		},
	}

	s := newTestService(mockDB)
	req := httptest.NewRequest(http.MethodGet, "/traces?service=test&limit=5", nil)
	w := httptest.NewRecorder()

	s.tracesHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var result svcproto.QueryFlowResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Len(t, result.Records, 1)
}

// ============================================================================
// Unit tests for Stats and edge cases
// ============================================================================

func TestQueryFlows_MultipleFilters(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "tenant_id = ?")
			assert.Contains(t, sql, "timestamp >= ?")
			assert.Contains(t, sql, "timestamp <= ?")
			assert.Contains(t, sql, "src_ip = ?")
			assert.Contains(t, sql, "dst_ip = ?")
			assert.Contains(t, sql, "service = ?")
			assert.Len(t, args, 6)
			return newMockRows([]string{"tenant_id", "src_ip", "dst_ip"}, [][]interface{}{
				{"tenant-1", "10.0.0.1", "10.0.0.2"},
			}), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{
		TenantId:  "tenant-1",
		StartTime: 1700000000000000000,
		EndTime:   1700003600000000000,
		SrcIp:     "10.0.0.1",
		DstIp:     "10.0.0.2",
		Service:   "my-service",
	}
	resp, err := s.QueryFlows(context.Background(), req)

	require.NoError(t, err)
	require.Len(t, resp.Records, 1)
	assert.Equal(t, "tenant-1", resp.Records[0]["tenant_id"])
}

func TestQueryFlows_CustomLimit(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "LIMIT 500")
			return newMockRows([]string{"src_ip"}, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{Limit: 500}
	resp, err := s.QueryFlows(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.Total)
}

func TestQueryDashboard_WithTimeFilter(t *testing.T) {
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			assert.Contains(t, sql, "timestamp >= ?")
			assert.Contains(t, sql, "timestamp <= ?")
			assert.Len(t, args, 3) // tenant_id + start + end
			assert.Equal(t, "tenant-1", args[0])
			return newMockRows(nil, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{
		TenantId:  "tenant-1",
		StartTime: 1700000000000000000,
		EndTime:   1700003600000000000,
	}
	resp, err := s.QueryDashboard(context.Background(), req)

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Records)
}

func TestQueryDashboard_SkipsErrors(t *testing.T) {
	callCount := 0
	mockDB := &mockTimeSeriesStorage{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			callCount++
			if callCount == 2 {
				return nil, errors.New("query failed")
			}
			return newMockRows([]string{"count"}, [][]interface{}{
				{uint64(42)},
			}), nil
		},
	}

	s := newTestService(mockDB)
	req := &svcproto.QueryFlowRequest{TenantId: "test-tenant"}
	resp, err := s.QueryDashboard(context.Background(), req)

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Records)
	// Should have 4 queries attempted, but one failed and was skipped
	assert.Equal(t, 4, callCount)
}

func TestOverviewHandler_JSONExtractFloatCalls(t *testing.T) {
	jsonCalls := []string{}
	mockDB := &mockTimeSeriesStorage{
		queryRowFunc: func(ctx context.Context, sql string, args ...interface{}) storage.Row {
			if strings.Contains(sql, "JSONExtractFloat") {
				jsonCalls = append(jsonCalls, sql)
			}
			return &mockRow{values: []interface{}{0.0}}
		},
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (storage.Rows, error) {
			return newMockRows(nil, nil), nil
		},
	}

	s := newTestService(mockDB)
	req := httptest.NewRequest(http.MethodGet, "/overview", nil)
	w := httptest.NewRecorder()

	s.overviewHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, jsonCalls, 4)
	assert.True(t, strings.Contains(jsonCalls[0], "cpu_percent"))
	assert.True(t, strings.Contains(jsonCalls[1], "memory_percent"))
	assert.True(t, strings.Contains(jsonCalls[2], "net_rx_bytes"))
	assert.True(t, strings.Contains(jsonCalls[3], "disk_percent"))
}
