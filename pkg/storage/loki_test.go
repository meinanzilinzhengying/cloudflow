package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLokiStorage(t *testing.T) {
	storage := NewLokiStorage("http://localhost:3100")
	assert.NotNil(t, storage)
	assert.Equal(t, "http://localhost:3100", storage.endpoint)
}

func TestLokiStorage_PushLogs_Empty(t *testing.T) {
	storage := NewLokiStorage("http://localhost:3100")
	err := storage.PushLogs(context.Background(), nil)
	assert.NoError(t, err)

	err = storage.PushLogs(context.Background(), []LogStream{})
	assert.NoError(t, err)
}

func TestLokiStorage_PushLogs_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/loki/api/v1/push", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := NewLokiStorage(server.URL)

	streams := []LogStream{
		{
			Stream: map[string]string{"job": "test"},
			Values: [][]interface{}{{"1234567890000000000", "test log line"}},
		},
	}

	err := storage.PushLogs(context.Background(), streams)
	assert.NoError(t, err)
}

func TestLokiStorage_PushLogs_Retry(t *testing.T) {
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

	storage := NewLokiStorage(server.URL)
	storage.maxRetries = 2

	streams := []LogStream{{Stream: map[string]string{"job": "test"}}}
	err := storage.PushLogs(context.Background(), streams)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, attempts, 2)
}

func TestLokiStorage_PushLogs_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	storage := NewLokiStorage(server.URL)
	storage.maxRetries = 1

	streams := []LogStream{{Stream: map[string]string{"job": "test"}}}
	err := storage.PushLogs(context.Background(), streams)
	assert.Error(t, err)
}

func TestLokiStorage_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ready", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := NewLokiStorage(server.URL)
	err := storage.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestLokiStorage_HealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	storage := NewLokiStorage(server.URL)
	err := storage.HealthCheck(context.Background())
	assert.Error(t, err)
}

func TestFlowToLogStream(t *testing.T) {
	storage := NewLokiStorage("http://localhost:3100")

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
		TenantID:  "tenant-1",
	}

	stream := storage.flowToLogStream(flow)

	assert.Equal(t, "cloudflow", stream.Stream["job"])
	assert.Equal(t, "192.168.1.1", stream.Stream["src_ip"])
	assert.Equal(t, "10.0.0.1", stream.Stream["dst_ip"])
	assert.Equal(t, "TCP", stream.Stream["protocol"])
	assert.Equal(t, "100", stream.Stream["vni"])
	assert.Equal(t, "tenant-1", stream.Stream["tenant_id"])
	assert.Equal(t, "54321", stream.Stream["src_port"])
	assert.Equal(t, "80", stream.Stream["dst_port"])

	assert.Len(t, stream.Values, 1)
}

func TestPushFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := NewLokiStorage(server.URL)

	flow := &Flow{
		Timestamp: 1234567890000,
		SrcIP:     "192.168.1.1",
		DstIP:     "10.0.0.1",
		Protocol:  6,
		Bytes:     1024,
		Packets:   10,
	}

	err := storage.PushFlow(context.Background(), flow)
	assert.NoError(t, err)
}

func TestPushFlows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := NewLokiStorage(server.URL)

	flows := []*Flow{
		{SrcIP: "1.1.1.1", DstIP: "2.2.2.2", Protocol: 6},
		{SrcIP: "3.3.3.3", DstIP: "4.4.4.4", Protocol: 17},
	}

	err := storage.PushFlows(context.Background(), flows)
	assert.NoError(t, err)
}

func TestStreamKey(t *testing.T) {
	labels1 := map[string]string{"job": "test", "src_ip": "1.1.1.1"}
	labels2 := map[string]string{"src_ip": "1.1.1.1", "job": "test"}

	key1 := streamKey(labels1)
	key2 := streamKey(labels2)

	// 相同标签不同顺序应该生成相同的key
	assert.Equal(t, key1, key2)
}

func TestFormatFlowAsText(t *testing.T) {
	storage := NewLokiStorage("http://localhost:3100")

	flow := &Flow{
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

	text := storage.FormatFlowAsText(flow)
	assert.Contains(t, text, "192.168.1.1:54321")
	assert.Contains(t, text, "10.0.0.1:80")
	assert.Contains(t, text, "TCP")
	assert.Contains(t, text, "1024 bytes")
	assert.Contains(t, text, "10 packets")
	assert.Contains(t, text, "VNI=100")
}

func TestBufferedLokiWriter(t *testing.T) {
	writer := NewBufferedLokiWriter("http://localhost:3100", 10, 1000)
	assert.NotNil(t, writer)
	assert.Equal(t, 10, writer.batchSize)
}

func TestBufferedLokiWriter_Write(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	writer := NewBufferedLokiWriter(server.URL, 2, 1000)

	flow1 := &Flow{SrcIP: "1.1.1.1"}
	err := writer.Write(context.Background(), flow1)
	assert.NoError(t, err)
	assert.Len(t, writer.buffer, 1)

	flow2 := &Flow{SrcIP: "2.2.2.2"}
	err = writer.Write(context.Background(), flow2)
	assert.NoError(t, err)
	assert.Len(t, writer.buffer, 0) // 达到batchSize后自动刷新
}

func TestBufferedLokiWriter_Flush(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	writer := NewBufferedLokiWriter(server.URL, 10, 1000)

	flow := &Flow{SrcIP: "1.1.1.1"}
	_ = writer.Write(context.Background(), flow)

	err := writer.Flush(context.Background())
	assert.NoError(t, err)
	assert.Len(t, writer.buffer, 0)
}

func TestBufferedLokiWriter_FlushEmpty(t *testing.T) {
	writer := NewBufferedLokiWriter("http://localhost:3100", 10, 1000)
	err := writer.Flush(context.Background())
	assert.NoError(t, err)
}
