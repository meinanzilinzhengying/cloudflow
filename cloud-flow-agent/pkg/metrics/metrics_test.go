package metrics

import (
	"errors"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
	if m.registry == nil {
		t.Fatal("registry is nil")
	}
}

func TestCollectFinished(t *testing.T) {
	m := New()
	
	t.Run("successful collect", func(t *testing.T) {
		start := m.CollectStarted()
		time.Sleep(1 * time.Millisecond)
		m.CollectFinished(start, nil)
		// Metrics are incremented internally, just verify no panic
	})
	
	t.Run("failed collect", func(t *testing.T) {
		start := m.CollectStarted()
		time.Sleep(1 * time.Millisecond)
		m.CollectFinished(start, errors.New("collect failed"))
		// Error counter should be incremented
	})
}

func TestSendFinished(t *testing.T) {
	m := New()
	
	t.Run("successful send", func(t *testing.T) {
		start := m.SendStarted()
		time.Sleep(1 * time.Millisecond)
		m.SendFinished(start, 1024, nil)
	})
	
	t.Run("failed send", func(t *testing.T) {
		start := m.SendStarted()
		time.Sleep(1 * time.Millisecond)
		m.SendFinished(start, 0, errors.New("send failed"))
	})
}

func TestHeartbeatSent(t *testing.T) {
	m := New()
	
	t.Run("successful heartbeat", func(t *testing.T) {
		m.HeartbeatSent(nil)
	})
	
	t.Run("failed heartbeat", func(t *testing.T) {
		m.HeartbeatSent(errors.New("heartbeat failed"))
	})
}

func TestEBPFCollect(t *testing.T) {
	m := New()
	
	t.Run("successful ebpf collect", func(t *testing.T) {
		m.EBPFCollect(nil)
	})
	
	t.Run("failed ebpf collect", func(t *testing.T) {
		m.EBPFCollect(errors.New("ebpf failed"))
	})
}

func TestStartServer(t *testing.T) {
	m := New()
	
	server, errCh := m.StartServer("127.0.0.1:0") // :0 for random port
	if server == nil {
		t.Fatal("StartServer returned nil server")
	}
	
	// Clean up
	defer server.Close()
	
	// Check that error channel was created
	if errCh == nil {
		t.Fatal("StartServer returned nil error channel")
	}
}
