package grpcutil

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestCheckAPIKey_EmptyExpected(t *testing.T) {
	// 空 expected key 应跳过验证
	ctx := context.Background()
	if !CheckAPIKey(ctx, "") {
		t.Fatal("should return true when expected key is empty")
	}
}

func TestCheckAPIKey_NoMetadata(t *testing.T) {
	ctx := context.Background()
	if CheckAPIKey(ctx, "secret") {
		t.Fatal("should return false when no metadata")
	}
}

func TestCheckAPIKey_MissingKey(t *testing.T) {
	md := metadata.Pairs("other-key", "value")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	if CheckAPIKey(ctx, "secret") {
		t.Fatal("should return false when x-api-key not in metadata")
	}
}

func TestCheckAPIKey_CorrectKey(t *testing.T) {
	md := metadata.Pairs("x-api-key", "correct-secret")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	if !CheckAPIKey(ctx, "correct-secret") {
		t.Fatal("should return true for correct key")
	}
}

func TestCheckAPIKey_WrongKey(t *testing.T) {
	md := metadata.Pairs("x-api-key", "wrong")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	if CheckAPIKey(ctx, "correct-secret") {
		t.Fatal("should return false for wrong key")
	}
}

func TestCheckAPIKey_EmptyProvided(t *testing.T) {
	md := metadata.Pairs("x-api-key", "")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	if CheckAPIKey(ctx, "secret") {
		t.Fatal("should return false when provided key is empty")
	}
}

func TestWithAuth(t *testing.T) {
	ctx := WithAuth(context.Background(), "my-key")
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("should have outgoing metadata")
	}
	vals := md.Get("x-api-key")
	if len(vals) != 1 || vals[0] != "my-key" {
		t.Fatalf("x-api-key = %v, want [my-key]", vals)
	}
}
