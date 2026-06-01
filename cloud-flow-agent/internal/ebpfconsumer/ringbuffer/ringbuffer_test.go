// Package ringbuffer 测试
package ringbuffer

import (
	"sync"
	"testing"
	"time"

	"cloud-flow-agent/internal/ebpfconsumer/pool"
)

func TestRingBuffer_Basic(t *testing.T) {
	rb := New(1024)
	event := &pool.RawEvent{Len: 10, Type: pool.EventTypeTCP}

	// Push
	if !rb.TryPush(event) {
		t.Fatal("Push failed")
	}

	// Pop
	got, ok := rb.TryPop()
	if !ok {
		t.Fatal("Pop failed")
	}
	if got.Len != 10 {
		t.Fatalf("Expected Len=10, got %d", got.Len)
	}
}

func TestRingBuffer_Batch(t *testing.T) {
	rb := New(1024)
	events := make([]*pool.RawEvent, 100)
	for i := range events {
		events[i] = &pool.RawEvent{Len: uint16(i)}
	}

	// Batch push
	n := rb.TryPushBatch(events)
	if n != 100 {
		t.Fatalf("Expected 100, got %d", n)
	}

	// Batch pop
	got := make([]*pool.RawEvent, 100)
	m := rb.TryPopBatch(got)
	if m != 100 {
		t.Fatalf("Expected 100, got %d", m)
	}
}

func BenchmarkRingBuffer_SingleProducerSingleConsumer(b *testing.B) {
	rb := New(65536)
	event := &pool.RawEvent{Len: 10}

	go func() {
		for i := 0; i < b.N; i++ {
			for !rb.TryPush(event) {
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for {
			if _, ok := rb.TryPop(); ok {
				break
			}
		}
	}
}

func BenchmarkRingBuffer_MultiProducerMultiConsumer(b *testing.B) {
	rb := New(65536)
	producers := 4
	consumers := 4

	var wg sync.WaitGroup
	wg.Add(producers + consumers)

	// Producers
	for p := 0; p < producers; p++ {
		go func() {
			defer wg.Done()
			event := &pool.RawEvent{Len: 10}
			for i := 0; i < b.N/producers; i++ {
				for !rb.TryPush(event) {
				}
			}
		}()
	}

	b.ResetTimer()

	// Consumers
	for c := 0; c < consumers; c++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/consumers; i++ {
				for {
					if _, ok := rb.TryPop(); ok {
						break
					}
				}
			}
		}()
	}

	wg.Wait()
}

func BenchmarkRingBuffer_Batch(b *testing.B) {
	rb := New(65536)
	batchSize := 64
	events := make([]*pool.RawEvent, batchSize)
	for i := range events {
		events[i] = &pool.RawEvent{Len: uint16(i)}
	}
	got := make([]*pool.RawEvent, batchSize)

	b.ResetTimer()
	for i := 0; i < b.N/batchSize; i++ {
		rb.TryPushBatch(events)
		rb.TryPopBatch(got)
	}
}

func TestRingBuffer_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	rb := New(65536)
	producers := 4
	consumers := 4
	iterations := 1000000

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(producers + consumers)

	// Producers
	for p := 0; p < producers; p++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations/producers; i++ {
				event := &pool.RawEvent{Seq: uint64(i), CPU: uint8(id)}
				for !rb.TryPush(event) {
				}
			}
		}(p)
	}

	// Consumers
	var total uint64
	for c := 0; c < consumers; c++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations/consumers; i++ {
				for {
					if _, ok := rb.TryPop(); ok {
						break
					}
				}
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)
	eventsPerSec := float64(iterations) / elapsed.Seconds()

	t.Logf("Events: %d, Time: %v, Throughput: %.2f events/s", iterations, elapsed, eventsPerSec)

	if eventsPerSec < 500000 {
		t.Errorf("Throughput too low: %.2f events/s (expected > 500k)", eventsPerSec)
	}
}
