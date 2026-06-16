package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVictoriaMetricsStorage(t *testing.T) {
	storage := NewVictoriaMetricsStorage("http://localhost:8428")
	assert.NotNil(t, storage)
	assert.Equal(t, "http://localhost:8428", storage.endpoint)
}

func TestVictoriaMetricsStorage_WriteMetrics_Empty(t *testing.T) {
	storage := NewVictoriaMetricsStorage("http://localhost:8428")
	err := storage.WriteMetrics(context.Background(), nil)
	assert.NoError(t, err)

	err = storage.WriteMetrics(context.Background(), []Metric{})
	assert.NoError(t, err)
}

func TestVictoriaMetricsStorage_WriteMetrics_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/import", r.URL.Path)
		assert.Equal(t, "gzip", r.Header.Get("Content-Encoding"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := NewVictoriaMetricsStorage(server.URL)

	metrics := []Metric{
		{
			Name:      "test_metric",
			Labels:    map[string]string{"foo": "bar"},
			Value:     123.45,
			Timestamp: 1234567890000,
		},
	}

	err := storage.WriteMetrics(context.Background(), metrics)
	assert.NoError(t, err)
}

func TestVictoriaMetricsStorage_WriteMetrics_Retry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := NewVictoriaMetricsStorage(server.URL)
	storage.maxRetries = 2

	metrics := []Metric{{Name: "test"}}
	err := storage.WriteMetrics(context.Background(), metrics)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, attempts, 2)
}

func TestVictoriaMetricsStorage_WriteMetrics_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	storage := NewVictoriaMetricsStorage(server.URL)
	storage.maxRetries = 1

	metrics := []Metric{{Name: "test"}}
	err := storage.WriteMetrics(context.Background(), metrics)
	assert.Error(t, err)
}

func TestVictoriaMetricsStorage_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := NewVictoriaMetricsStorage(server.URL)
	err := storage.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestVictoriaMetricsStorage_HealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	storage := NewVictoriaMetricsStorage(server.URL)
	err := storage.HealthCheck(context.Background())
	assert.Error(t, err)
}

func TestFlowToMetrics(t *testing.T) {
	storage := NewVictoriaMetricsStorage("http://localhost:8428")

	flow := &Flow{
		Timestamp: 1234567890000,
		SrcIP:     "192.168.1.1",
		DstIP:     "10.0.0.1",
		SrcPort:   54321,
		DstPort:   80,
		Protocol:  6,
		Bytes:     1024,
		Packets:   10,
		DurationMs: 1500,
		VNI:       100,
	}

	metrics := storage.flowToMetrics(flow)
	assert.Len(t, metrics, 3) // bytes, packets, duration

	names := make(map[string]bool)
	for _, m := range metrics {
		names[m.Name] = true
		assert.Equal(t, "192.168.1.1", m.Labels["src_ip"])
		assert.Equal(t, "10.0.0.1", m.Labels["dst_ip"])
		assert.Equal(t, "TCP", m.Labels["protocol"])
		assert.Equal(t, "100", m.Labels["vni"])
	}

	assert.True(t, names["cloudflow_flow_bytes"])
	assert.True(t, names["cloudflow_flow_packets"])
	assert.True(t, names["cloudflow_flow_duration_ms"])
}

func TestMetricToInfluxLine(t *testing.T) {
	storage := NewVictoriaMetricsStorage("http://localhost:8428")

	metric := Metric{
		Name:      "test_metric",
		Labels:    map[string]string{"tag1": "value1", "tag2": "value2"},
		Value:     123.45,
		Timestamp: 1234567890000,
	}

	line := storage.metricToInfluxLine(metric)
	assert.Contains(t, line, "test_metric")
	assert.Contains(t, line, "tag1=value1")
	assert.Contains(t, line, "tag2=value2")
	assert.Contains(t, line, "value=123.45")
}

func TestProtocolToString(t *testing.T) {
	tests := []struct {
		proto    uint8
		expected string
	}{
		{6, "TCP"},
		{17, "UDP"},
		{1, "ICMP"},
		{99, "99"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, protocolToString(tt.proto))
		})
	}
}

func TestEscapeInflux(t *testing.T) {
	assert.Equal(t, "hello\\ world", escapeInfluxMeasurement("hello world"))
	assert.Equal(t, "hello\\,world", escapeInfluxMeasurement("hello,world"))
	assert.Equal(t, "key\\=value", escapeInfluxTag("key=value"))
}

func TestWriteFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := NewVictoriaMetricsStorage(server.URL)

	flow := &Flow{
		Timestamp: 1234567890000,
		SrcIP:     "192.168.1.1",
		DstIP:     "10.0.0.1",
		Protocol:  6,
		Bytes:     1024,
		Packets:   10,
	}

	err := storage.WriteFlow(context.Background(), flow)
	assert.NoError(t, err)
}

func TestWriteFlows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := NewVictoriaMetricsStorage(server.URL)

	flows := []*Flow{
		{SrcIP: "1.1.1.1", DstIP: "2.2.2.2"},
		{SrcIP: "3.3.3.3", DstIP: "4.4.4.4"},
	}

	err := storage.WriteFlows(context.Background(), flows)
	assert.NoError(t, err)
}
