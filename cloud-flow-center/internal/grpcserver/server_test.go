package grpcserver

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"cloud-flow-center/pkg/logger"
	edge "cloud-flow/proto"
)

// mockStorage 模拟存储引擎，记录调用并支持错误注入
type mockStorage struct {
	mu sync.Mutex

	// 调用追踪
	saveMetricsCalled bool
	lastProbeID       string
	saveTracesCalled  bool
	saveProfilingCalled bool
	saveProbeInfoCalled bool
	forwardMetricsCalled bool
	queryMetricsCalled   bool

	// 错误注入
	forceError bool
	errMsg     string
}

func (m *mockStorage) enableError(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forceError = true
	m.errMsg = msg
}

func (m *mockStorage) getErrorLocked() error {
	if m.forceError {
		return fmt.Errorf(m.errMsg)
	}
	return nil
}

func (m *mockStorage) SaveMetrics(probeID string, metrics interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveMetricsCalled = true
	m.lastProbeID = probeID
	return m.getErrorLocked()
}

func (m *mockStorage) SaveTraces(probeID string, spans interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveTracesCalled = true
	return m.getErrorLocked()
}

func (m *mockStorage) SaveProfiling(probeID string, profiles interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveProfilingCalled = true
	return m.getErrorLocked()
}

func (m *mockStorage) SaveProbeInfo(edgeNodeID string, data interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveProbeInfoCalled = true
	return m.getErrorLocked()
}

func (m *mockStorage) QueryMetrics(day string, probeID string, limit int) ([]map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queryMetricsCalled = true
	return []map[string]interface{}{}, m.getErrorLocked()
}

func (m *mockStorage) QueryMetricsByAlert(day string, metricName string, operator string, threshold float64, limit int) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

func (m *mockStorage) QueryTraces(day string, probeID string, limit int) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

func (m *mockStorage) GetRecentMetrics(metricType string, limit int, timeWindow time.Duration) ([]*edge.MetricData, error) {
	return []*edge.MetricData{}, nil
}

func (m *mockStorage) GetOverview() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (m *mockStorage) GetNodes() ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

func (m *mockStorage) CreateUser(username, password, role string) error { return nil }
func (m *mockStorage) GetUser(username string) (map[string]interface{}, error) { return map[string]interface{}{}, nil }
func (m *mockStorage) UpdateUser(username, password, role string) error { return nil }
func (m *mockStorage) DeleteUser(username string) error { return nil }
func (m *mockStorage) UpdateUserRole(username, role string) error { return nil }
func (m *mockStorage) ListUsers() ([]map[string]interface{}, error) { return []map[string]interface{}{}, nil }
func (m *mockStorage) VerifyUser(username, password string) (bool, string, error) { return true, "admin", nil }
func (m *mockStorage) ChangePassword(username, oldPassword, newPassword string) error { return nil }
func (m *mockStorage) SaveUserPreferences(username string, prefs map[string]interface{}) error { return nil }
func (m *mockStorage) GetUserPreferences(username string) (map[string]interface{}, error) { return map[string]interface{}{}, nil }
func (m *mockStorage) ListBusiness(page, pageSize int) ([]map[string]interface{}, int, error) { return []map[string]interface{}{}, 0, nil }
func (m *mockStorage) CreateBusiness(data map[string]interface{}) error { return nil }
func (m *mockStorage) GetBusiness(id string) (map[string]interface{}, error) { return map[string]interface{}{}, nil }
func (m *mockStorage) UpdateBusiness(id string, data map[string]interface{}) error { return nil }
func (m *mockStorage) DeleteBusiness(id string) error { return nil }
func (m *mockStorage) ListService(page, pageSize int) ([]map[string]interface{}, int, error) { return []map[string]interface{}{}, 0, nil }
func (m *mockStorage) CreateService(data map[string]interface{}) error { return nil }
func (m *mockStorage) GetService(id string) (map[string]interface{}, error) { return map[string]interface{}{}, nil }
func (m *mockStorage) UpdateService(id string, data map[string]interface{}) error { return nil }
func (m *mockStorage) DeleteService(id string) error { return nil }
func (m *mockStorage) ListCollector(page, pageSize int) ([]map[string]interface{}, int, error) { return []map[string]interface{}{}, 0, nil }
func (m *mockStorage) CreateCollector(data map[string]interface{}) error { return nil }
func (m *mockStorage) GetCollector(id string) (map[string]interface{}, error) { return map[string]interface{}{}, nil }
func (m *mockStorage) UpdateCollector(id string, data map[string]interface{}) error { return nil }
func (m *mockStorage) DeleteCollector(id string) error { return nil }
func (m *mockStorage) ListDataNode(page, pageSize int) ([]map[string]interface{}, int, error) { return []map[string]interface{}{}, 0, nil }
func (m *mockStorage) CreateDataNode(data map[string]interface{}) error { return nil }
func (m *mockStorage) GetDataNode(id string) (map[string]interface{}, error) { return map[string]interface{}{}, nil }
func (m *mockStorage) UpdateDataNode(id string, data map[string]interface{}) error { return nil }
func (m *mockStorage) DeleteDataNode(id string) error { return nil }
func (m *mockStorage) StartCleanup() {}
func (m *mockStorage) Stop() {}
func (m *mockStorage) DB() interface{} { return nil }

func newTestLogger() *logger.Logger {
	return logger.New(logger.Config{Level: "error", Format: "console"})
}

func TestServerReportProbes(t *testing.T) {
	log := newTestLogger()
	store := &mockStorage{}
	srv := NewServer(store, log, "test-api-key")

	req := &edge.ReportProbesRequest{
		EdgeNodeId:    "test-edge",
		CloudPlatform: "aws",
		Region:        "us-west-2",
		Probes: []*edge.ProbeInfo{
			{ProbeId: "probe-1", HostIp: "10.0.0.1", Hostname: "host-1", Status: "online", Version: "1.0.0", LastHeartbeat: time.Now().Unix()},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := srv.ReportProbes(ctx, req)
	if err != nil {
		t.Errorf("ReportProbes failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("ReportProbes failed, resp: %v", resp)
	}
	// 验证存储层被调用
	store.mu.Lock()
	if !store.saveProbeInfoCalled {
		t.Error("SaveProbeInfo should have been called")
	}
	store.mu.Unlock()
}

func TestServerForwardMetrics(t *testing.T) {
	log := newTestLogger()
	store := &mockStorage{}
	srv := NewServer(store, log, "test-api-key")

	batch := &edge.MetricsBatch{
		ProbeId: "test-probe",
		Metrics: []*edge.MetricData{{Timestamp: time.Now().Unix(), SrcIp: "10.0.0.1", DstIp: "10.0.0.2", Protocol: "tcp", Bytes: 1024, Packets: 10}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := srv.ForwardMetrics(ctx, batch)
	if err != nil {
		t.Errorf("ForwardMetrics failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("ForwardMetrics failed, resp: %v", resp)
	}

	store.mu.Lock()
	if !store.saveMetricsCalled {
		t.Error("SaveMetrics should have been called")
	}
	if store.lastProbeID != "test-probe" {
		t.Errorf("lastProbeID = %q, want %q", store.lastProbeID, "test-probe")
	}
	store.mu.Unlock()
}

func TestServerForwardMetrics_StorageError(t *testing.T) {
	log := newTestLogger()
	store := &mockStorage{}
	store.enableError("storage unavailable")
	srv := NewServer(store, log, "test-api-key")

	batch := &edge.MetricsBatch{
		ProbeId: "test-probe",
		Metrics: []*edge.MetricData{{Timestamp: time.Now().Unix()}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := srv.ForwardMetrics(ctx, batch)
	if err == nil {
		t.Error("ForwardMetrics should return error when storage fails")
	}
}

func TestServerHeartbeat(t *testing.T) {
	log := newTestLogger()
	store := &mockStorage{}
	srv := NewServer(store, log, "test-api-key")

	req := &edge.EdgeHeartbeatRequest{
		EdgeNodeId: "test-edge", CloudPlatform: "aws", Region: "us-west-2",
		Timestamp: time.Now().Unix(), ProbeCount: 1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := srv.Heartbeat(ctx, req)
	if err != nil {
		t.Errorf("Heartbeat failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("Heartbeat failed, resp: %v", resp)
	}
}
