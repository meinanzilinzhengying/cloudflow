package output

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"math/rand"
	"time"
)

type EdgeClient struct {
	addr       string
	batch      []*Event
	mu         sync.Mutex
	ticker     *time.Ticker
	stopCh     chan struct{}
	client     *http.Client
	retryCount int
	maxRetries int
	baseInterval time.Duration
	closed     bool
}

// 按类别采样率：在探针端直接过滤，减少网络传输和Redis压力
var categorySampling = map[string]float64{
	"file_events":    0.01,  // 1%
	"network_events": 0.10,  // 10%
	"process_events": 0.10,  // 10%
	"security_events": 1.00, // 100%
	"syscall":         0.001, // 0.1% - syscall量极大，极低采样
}

func shouldSample(category string) bool {
	rate, ok := categorySampling[category]
	if !ok {
		rate = 1.0
	}
	if rate >= 1.0 {
		return true
	}
	return rand.Float64() < rate
}

func NewEdgeClient(addr string) (*EdgeClient, error) {
	e := &EdgeClient{
		addr:       addr,
		batch:      make([]*Event, 0, 2000),
		ticker:     time.NewTicker(5 * time.Second),
		stopCh:     make(chan struct{}),
		client:     &http.Client{Timeout: 30 * time.Second},
		retryCount: 0,
		maxRetries: 100,
		baseInterval: 5 * time.Second,
	}
	go e.flushLoop()
	return e, nil
}

func (e *EdgeClient) WriteEvent(ev *Event) error {
	if !shouldSample(ev.Category) {
		return nil
	}
	e.mu.Lock()
	e.batch = append(e.batch, ev)
	shouldFlush := len(e.batch) >= 2000
	e.mu.Unlock()
	if shouldFlush {
		e.flush()
	}
	return nil
}

func (e *EdgeClient) WriteBatch(events []*Event) error {
	filtered := make([]*Event, 0, len(events))
	for _, ev := range events {
		if shouldSample(ev.Category) {
			filtered = append(filtered, ev)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	e.mu.Lock()
	e.batch = append(e.batch, filtered...)
	shouldFlush := len(e.batch) >= 2000
	e.mu.Unlock()
	if shouldFlush {
		e.flush()
	}
	return nil
}

func (e *EdgeClient) WriteMetrics(probeID string, cpu, mem, disk float64, netRx, netTx, diskRead, diskWrite uint64) error {
	details, _ := json.Marshal(map[string]interface{}{
		"cpu_percent":      cpu,
		"memory_percent":   mem,
		"disk_percent":     disk,
		"net_rx_bytes":     netRx,
		"net_tx_bytes":     netTx,
		"disk_read_bytes":  diskRead,
		"disk_write_bytes": diskWrite,
	})
	ev := &Event{Timestamp: time.Now(), ProbeID: probeID, Category: "metrics", EventType: "host_metrics", Details: string(details)}
	return e.WriteEvent(ev)
}

func (e *EdgeClient) WriteProcessEvent(ts time.Time, probeID string, pid, ppid uint32, comm, exe, args, eventType string) error {
	details, _ := json.Marshal(map[string]interface{}{
		"pid":  pid,
		"ppid": ppid,
		"comm": comm,
		"exe":  exe,
		"args": args,
	})
	ev := &Event{Timestamp: ts, ProbeID: probeID, Category: "process", EventType: eventType, Details: string(details)}
	return e.WriteEvent(ev)
}

func (e *EdgeClient) WriteFileEvent(ts time.Time, probeID string, pid uint32, comm, filename, operation string, result int32) error {
	details, _ := json.Marshal(map[string]interface{}{
		"pid":      pid,
		"comm":     comm,
		"filename": filename,
		"result":   result,
	})
	ev := &Event{Timestamp: ts, ProbeID: probeID, Category: "file", EventType: operation, Details: string(details)}
	return e.WriteEvent(ev)
}

func (e *EdgeClient) WriteSyscallEvent(ts time.Time, probeID string, pid uint32, comm string, syscallNr, latencyNs, count uint64) error {
	details, _ := json.Marshal(map[string]interface{}{
		"pid":        pid,
		"comm":       comm,
		"syscall_nr": syscallNr,
		"latency_ns": latencyNs,
		"count":      count,
	})
	ev := &Event{Timestamp: ts, ProbeID: probeID, Category: "syscall", EventType: "syscall", Details: string(details)}
	return e.WriteEvent(ev)
}

func (e *EdgeClient) flushLoop() {
	for {
		select {
		case <-e.ticker.C:
			e.flush()
		case <-e.stopCh:
			return
		}
	}
}

func (e *EdgeClient) flush() {
	e.mu.Lock()
	if len(e.batch) == 0 {
		e.mu.Unlock()
		return
	}
	events := make([]*Event, len(e.batch))
	copy(events, e.batch)
	e.batch = e.batch[:0]
	e.mu.Unlock()

	data, _ := json.Marshal(events)
	resp, err := e.client.Post("http://"+e.addr+"/api/v1/ingest", "application/json", bytes.NewReader(data))
	if err != nil {
		e.mu.Lock()
		e.retryCount++
		if e.retryCount > e.maxRetries {
			log.Printf("[EDGE] max retries (%d) reached, dropping %d events", e.maxRetries, len(events))
			e.retryCount = 0
			e.mu.Unlock()
			return
		}
		e.batch = append(events, e.batch...)
		if len(e.batch) > 10000 {
			e.batch = e.batch[:10000]
		}
		backoff := e.baseInterval * time.Duration(1<<uint(e.retryCount-1))
		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
		e.ticker.Reset(backoff)
		e.mu.Unlock()
		log.Printf("[EDGE] flush failed: %v, retry %d/%d, backoff %v", err, e.retryCount, e.maxRetries, backoff)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		e.mu.Lock()
		e.retryCount++
		if e.retryCount > e.maxRetries {
			log.Printf("[EDGE] max retries (%d) reached, dropping %d events", e.maxRetries, len(events))
			e.retryCount = 0
			e.mu.Unlock()
			return
		}
		e.batch = append(events, e.batch...)
		if len(e.batch) > 10000 {
			e.batch = e.batch[:10000]
		}
		backoff := e.baseInterval * time.Duration(1<<uint(e.retryCount-1))
		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
		e.ticker.Reset(backoff)
		e.mu.Unlock()
		log.Printf("[EDGE] server returned %d, retry %d/%d, backoff %v", resp.StatusCode, e.retryCount, e.maxRetries, backoff)
		return
	}

	// 只有 200 才认为成功
	e.mu.Lock()
	e.retryCount = 0
	e.ticker.Reset(e.baseInterval)
	e.mu.Unlock()
	io.Copy(io.Discard, resp.Body)
	log.Printf("[EDGE] flushed %d events", len(events))
}

func (e *EdgeClient) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.mu.Unlock()
	close(e.stopCh)
	e.ticker.Stop()
	e.flush()
	return nil
}

func (e *EdgeClient) Flush() {
	e.flush()
}
