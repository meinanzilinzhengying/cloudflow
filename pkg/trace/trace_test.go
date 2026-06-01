package trace

import (
	"context"
	"testing"
)

func TestTraceID_NotEmpty(t *testing.T) {
	id := TraceID()
	if id == "" {
		t.Fatal("TraceID() should not return empty string")
	}
}

func TestTraceID_Unique(t *testing.T) {
	ids := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id := TraceID()
		if ids[id] {
			t.Fatalf("duplicate TraceID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestWithTraceID_FromContext(t *testing.T) {
	ctx := context.Background()
	id := "test-trace-123"
	ctx = WithTraceID(ctx, id)
	if got := FromContext(ctx); got != id {
		t.Errorf("FromContext() = %q, want %q", got, id)
	}
}

func TestFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	if got := FromContext(ctx); got != "" {
		t.Errorf("FromContext(empty) = %q, want empty", got)
	}
}

func TestWithTraceID_Overwrite(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceID(ctx, "first")
	ctx = WithTraceID(ctx, "second")
	if got := FromContext(ctx); got != "second" {
		t.Errorf("FromContext() = %q, want %q (last write wins)", got, "second")
	}
}
