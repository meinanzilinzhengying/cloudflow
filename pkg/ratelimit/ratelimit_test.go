package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestNewTokenBucket(t *testing.T) {
	tests := []struct {
		name       string
		bucketSize int
		refillRate int
		wantTokens int
	}{
		{"normal", 10, 1, 10},
		{"zero size", 0, 1, 0},
		{"zero refill", 5, 0, 5},
		{"both zero", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := NewTokenBucket(tt.bucketSize, tt.refillRate)
			tb.mu.Lock()
			got := tb.tokens
			tb.mu.Unlock()
			if got != tt.wantTokens {
				t.Errorf("tokens = %d, want %d", got, tt.wantTokens)
			}
		})
	}
}

func TestAllow_BasicConsumption(t *testing.T) {
	tb := NewTokenBucket(5, 0) // 不补充
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Fatalf("Allow() #%d should return true", i+1)
		}
	}
	if tb.Allow() {
		t.Fatal("Allow() should return false when tokens exhausted")
	}
}

func TestAllow_NoExceedBucketSize(t *testing.T) {
	tb := NewTokenBucket(3, 100) // 高补充率
	// 消耗所有令牌
	for i := 0; i < 3; i++ {
		tb.Allow()
	}
	// 等待补充
	time.Sleep(10 * time.Millisecond)
	tb.Allow()
	tb.mu.Lock()
	if tb.tokens > 3 {
		t.Errorf("tokens should not exceed bucketSize: got %d, want <= 3", tb.tokens)
	}
	tb.mu.Unlock()
}

func TestAllow_ConcurrentSafety(t *testing.T) {
	tb := NewTokenBucket(1000, 0)
	var wg sync.WaitGroup
	var allowed int64
	var mu sync.Mutex
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tb.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 1000 {
		t.Errorf("concurrent Allow() count = %d, want 1000", allowed)
	}
	// 所有令牌耗尽后应拒绝
	if tb.Allow() {
		t.Fatal("should reject after all tokens consumed")
	}
}

func TestAllow_ZeroRefillRate(t *testing.T) {
	tb := NewTokenBucket(1, 0) // 1个令牌，不补充
	tb.Allow()                 // 消耗
	if tb.Allow() {
		t.Fatal("should reject immediately after consumption")
	}
	time.Sleep(50 * time.Millisecond)
	if tb.Allow() {
		t.Fatal("should reject when refillRate is 0")
	}
}

func TestAllow_RefillOverTime(t *testing.T) {
	tb := NewTokenBucket(5, 1000) // 每秒补充1000个
	for i := 0; i < 5; i++ {
		tb.Allow()
	}
	time.Sleep(20 * time.Millisecond) // 应补充约20个
	if !tb.Allow() {
		t.Fatal("should allow after refill")
	}
}

func TestAllow_ZeroBucketSize(t *testing.T) {
	tb := NewTokenBucket(0, 100)
	if tb.Allow() {
		t.Fatal("should always reject when bucketSize is 0")
	}
}
