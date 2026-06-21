package http

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// Event 探针上报的事件
type IngestEvent struct {
	Timestamp time.Time `json:"timestamp"`
	ProbeID   string    `json:"probe_id"`
	Category  string    `json:"category"`
	EventType string    `json:"event_type"`
	SrcIP     string    `json:"src_ip"`
	DstIP     string    `json:"dst_ip"`
	SrcPort   uint16    `json:"src_port"`
	DstPort   uint16    `json:"dst_port"`
	Protocol  string    `json:"protocol"`
	Bytes     uint64    `json:"bytes"`
	Packets   uint64    `json:"packets"`
	LatencyMs float64   `json:"latency_ms"`
	Service   string    `json:"service"`
	Details   string    `json:"details"`
	Tags      string    `json:"tags"`
}

// IngestHandler 数据接收处理器
type IngestHandler struct {
	chDB       *sql.DB
	batch      []*IngestEvent
	mu         sync.Mutex
	ticker     *time.Ticker
	stopCh     chan struct{}
	batchSize  int
	flushIntvl time.Duration
}

// NewIngestHandler 创建数据接收处理器
func NewIngestHandler(chDB *sql.DB) *IngestHandler {
	// P0-15: 增大 batchSize 和减小 flush 间隔，提高吞吐量
	h := &IngestHandler{
		chDB:       chDB,
		batch:      make([]*IngestEvent, 0, 10000),
		ticker:     time.NewTicker(1 * time.Second),
		stopCh:     make(chan struct{}),
		batchSize:  10000,
		flushIntvl: 1 * time.Second,
	}
	go h.flushLoop()
	return h
}

// Stop 停止处理器
func (h *IngestHandler) Stop() {
	close(h.stopCh)
	h.ticker.Stop()
	h.flush()
}

// HandleIngest 处理数据上报
func (h *IngestHandler) HandleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var events []*IngestEvent
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		http.Error(w, fmt.Sprintf("decode error: %v", err), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	h.batch = append(h.batch, events...)
	shouldFlush := len(h.batch) >= h.batchSize
	h.mu.Unlock()

	if shouldFlush {
		go h.flush()
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (h *IngestHandler) flushLoop() {
	for {
		select {
		case <-h.ticker.C:
			h.flush()
		case <-h.stopCh:
			return
		}
	}
}

func (h *IngestHandler) flush() {
	h.mu.Lock()
	if len(h.batch) == 0 {
		h.mu.Unlock()
		return
	}
	batch := make([]*IngestEvent, len(h.batch))
	copy(batch, h.batch)
	h.batch = h.batch[:0]
	h.mu.Unlock()

	if h.chDB == nil {
		log.Printf("[INGEST] ClickHouse not available, dropped %d events", len(batch))
		return
	}

	// 批量写入 ClickHouse
	tx, err := h.chDB.Begin()
	if err != nil {
		log.Printf("[INGEST] begin tx failed: %v", err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO cloudflow.ebpf_events (timestamp, probe_id, category, event_type, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency_ms, service, details, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		log.Printf("[INGEST] prepare failed: %v", err)
		return
	}
	defer stmt.Close()

	for _, ev := range batch {
		_, err := stmt.Exec(
			ev.Timestamp, ev.ProbeID, ev.Category, ev.EventType,
			ev.SrcIP, ev.DstIP, ev.SrcPort, ev.DstPort, ev.Protocol,
			ev.Bytes, ev.Packets, ev.LatencyMs, ev.Service, ev.Details, ev.Tags,
		)
		if err != nil {
			log.Printf("[INGEST] exec failed: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[INGEST] commit failed: %v", err)
	} else {
		log.Printf("[INGEST] flushed %d events", len(batch))
	}
}
