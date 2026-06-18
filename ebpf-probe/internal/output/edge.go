package output

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

type EdgeClient struct {
	addr   string
	batch  []*Event
	mu     sync.Mutex
	ticker *time.Ticker
	stopCh chan struct{}
}

func NewEdgeClient(addr string) (*EdgeClient, error) {
	e := &EdgeClient{
		addr:   addr,
		batch:  make([]*Event, 0, 100),
		ticker: time.NewTicker(1 * time.Second),
		stopCh: make(chan struct{}),
	}
	go e.flushLoop()
	return e, nil
}

func (e *EdgeClient) WriteEvent(ev *Event) error {
	e.mu.Lock()
	e.batch = append(e.batch, ev)
	shouldFlush := len(e.batch) >= 100
	e.mu.Unlock()
	if shouldFlush {
		e.flush()
	}
	return nil
}

func (e *EdgeClient) WriteBatch(events []*Event) error {
	e.mu.Lock()
	e.batch = append(e.batch, events...)
	shouldFlush := len(e.batch) >= 100
	e.mu.Unlock()
	if shouldFlush {
		e.flush()
	}
	return nil
}

func (e *EdgeClient) WriteMetrics(probeID string, cpu, mem, disk float64, netRx, netTx, diskRead, diskWrite uint64) error {
	ev := &Event{Timestamp: time.Now(), ProbeID: probeID, Category: "metrics", EventType: "host_metrics"}
	return e.WriteEvent(ev)
}

func (e *EdgeClient) WriteProcessEvent(ts time.Time, probeID string, pid, ppid uint32, comm, exe, args, eventType string) error {
	ev := &Event{Timestamp: ts, ProbeID: probeID, Category: "process", EventType: eventType, Details: comm}
	return e.WriteEvent(ev)
}

func (e *EdgeClient) WriteFileEvent(ts time.Time, probeID string, pid uint32, comm, filename, operation string, result int32) error {
	ev := &Event{Timestamp: ts, ProbeID: probeID, Category: "file", EventType: operation, Details: filename}
	return e.WriteEvent(ev)
}

func (e *EdgeClient) WriteSyscallEvent(ts time.Time, probeID string, pid uint32, comm string, syscallNr, latencyNs, count uint64) error {
	ev := &Event{Timestamp: ts, ProbeID: probeID, Category: "syscall", EventType: "syscall", Details: comm}
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
	resp, err := http.Post("http://"+e.addr+"/api/v1/ingest", "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("[EDGE] flush failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[EDGE] flush status: %d", resp.StatusCode)
	}
}

func (e *EdgeClient) Close() error {
	close(e.stopCh)
	e.ticker.Stop()
	e.flush()
	return nil
}

func (e *EdgeClient) Flush() {
	e.flush()
}
