package kafkaconsumer

import (
	"testing"
	"time"
)

// ============================================================================
// NoOpDedup 测试
// ============================================================================

func TestNoOpDedup(t *testing.T) {
	d := &NoOpDedup{}

	for i := 0; i < 10; i++ {
		isDup, err := d.IsDuplicate("topic1", 0, int64(i))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if isDup {
			t.Errorf("NoOpDedup should never return duplicate, got true on iteration %d", i)
		}
	}

	isDup, err := d.IsDuplicate("topic1", 0, 100)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("NoOpDedup should never return duplicate")
	}
	isDup, err = d.IsDuplicate("topic1", 0, 100)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("NoOpDedup should never return duplicate on second call")
	}
}

// ============================================================================
// MemoryDedup 测试
// ============================================================================

func TestMemoryDedup_NewMessage(t *testing.T) {
	d := NewMemoryDedup(1 * time.Hour)
	defer d.Close()

	isDup, err := d.IsDuplicate("topic1", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("first message should not be duplicate")
	}
}

func TestMemoryDedup_DuplicateMessage(t *testing.T) {
	d := NewMemoryDedup(1 * time.Hour)
	defer d.Close()

	isDup, err := d.IsDuplicate("topic1", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("first message should not be duplicate")
	}

	isDup, err = d.IsDuplicate("topic1", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isDup {
		t.Error("second message should be duplicate")
	}
}

func TestMemoryDedup_DifferentPartitions(t *testing.T) {
	d := NewMemoryDedup(1 * time.Hour)
	defer d.Close()

	isDup, err := d.IsDuplicate("topic1", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("first message should not be duplicate")
	}

	isDup, err = d.IsDuplicate("topic1", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("different partition should not be duplicate")
	}
}

func TestMemoryDedup_DifferentTopics(t *testing.T) {
	d := NewMemoryDedup(1 * time.Hour)
	defer d.Close()

	isDup, err := d.IsDuplicate("topic1", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("first message should not be duplicate")
	}

	isDup, err = d.IsDuplicate("topic2", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("different topic should not be duplicate")
	}
}

func TestMemoryDedup_TTLOperation(t *testing.T) {
	d := NewMemoryDedup(100 * time.Millisecond)
	defer d.Close()

	isDup, err := d.IsDuplicate("topic1", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("first message should not be duplicate")
	}

	time.Sleep(200 * time.Millisecond)

	isDup, err = d.IsDuplicate("topic1", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("message after TTL should not be duplicate")
	}
}

func TestMemoryDedup_Stats(t *testing.T) {
	d := NewMemoryDedup(1 * time.Hour)
	defer d.Close()

	for i := 0; i < 5; i++ {
		_, _ = d.IsDuplicate("topic1", 0, int64(i))
	}

	stats := d.Stats()
	if stats != 5 {
		t.Errorf("expected stats=5, got %d", stats)
	}
}

func TestMemoryDedup_ConcurrentAccess(t *testing.T) {
	d := NewMemoryDedup(1 * time.Hour)
	defer d.Close()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				_, _ = d.IsDuplicate("topic1", 0, int64(j))
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	stats := d.Stats()
	if stats != 100 {
		t.Errorf("expected stats=100 after concurrent access, got %d", stats)
	}
}

// ============================================================================
// RedisDedup Key 生成测试
// ============================================================================

func TestRedisDedup_Key(t *testing.T) {
	d := &RedisDedup{prefix: "cloudflow:dedup", ttl: 24 * time.Hour}
	key := d.key("topic1", 0, 100)
	expected := "cloudflow:dedup:topic1:0:100"
	if key != expected {
		t.Errorf("expected key=%s, got %s", expected, key)
	}
}

func TestRedisDedup_KeyDifferentValues(t *testing.T) {
	d := &RedisDedup{prefix: "test", ttl: 1 * time.Hour}

	tests := []struct {
		topic     string
		partition int32
		offset    int64
		expected  string
	}{
		{"flow.raw", 0, 0, "test:flow.raw:0:0"},
		{"metrics", 1, 999, "test:metrics:1:999"},
		{"logs", 5, 1000000, "test:logs:5:1000000"},
	}

	for _, tc := range tests {
		key := d.key(tc.topic, tc.partition, tc.offset)
		if key != tc.expected {
			t.Errorf("key mismatch: got %s, expected %s", key, tc.expected)
		}
	}
}

// ============================================================================
// 集成测试：Consumer 与 Dedup 协同
// ============================================================================

func TestConsumerDedupIntegration(t *testing.T) {
	dedup := NewMemoryDedup(1 * time.Hour)
	defer dedup.Close()

	messages := []struct {
		topic     string
		partition int32
		offset    int64
		isDup     bool
	}{
		{"flow.raw", 0, 100, false},
		{"flow.raw", 0, 101, false},
		{"flow.raw", 0, 100, true},
		{"flow.raw", 0, 102, false},
		{"metrics", 0, 100, false},
		{"flow.raw", 1, 100, false},
		{"flow.raw", 0, 100, true},
	}

	for _, msg := range messages {
		isDup, err := dedup.IsDuplicate(msg.topic, msg.partition, msg.offset)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if isDup != msg.isDup {
			t.Errorf("IsDuplicate(%s,%d,%d) = %v, expected %v",
				msg.topic, msg.partition, msg.offset, isDup, msg.isDup)
		}
	}
}
