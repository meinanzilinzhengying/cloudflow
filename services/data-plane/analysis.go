package dataplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type AnalysisResult struct {
	Timestamp   time.Time         `json:"timestamp"`
	TotalEvents uint64            `json:"total_events"`
	EventTypes  map[string]uint64 `json:"event_types"`
	Categories  map[string]uint64 `json:"categories"`
	Probes      map[string]uint64 `json:"probes"`
	TimeWindow  string            `json:"time_window"`
}

type AnalysisEngine struct {
	chDB       *sql.DB
	lastResult *AnalysisResult
	mu         sync.RWMutex
	stopCh     chan struct{}
}

func NewAnalysisEngine(chDB *sql.DB) *AnalysisEngine {
	return &AnalysisEngine{chDB: chDB, stopCh: make(chan struct{})}
}

func (ae *AnalysisEngine) Start(ctx context.Context) {
	ae.runAnalysis(ctx)
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ae.runAnalysis(ctx)
			case <-ae.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (ae *AnalysisEngine) Stop() { close(ae.stopCh) }

func (ae *AnalysisEngine) GetResult() *AnalysisResult {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	if ae.lastResult == nil {
		return &AnalysisResult{Timestamp: time.Now(), TotalEvents: 0, EventTypes: make(map[string]uint64), Categories: make(map[string]uint64), Probes: make(map[string]uint64), TimeWindow: "0s"}
	}
	return ae.lastResult
}

func (ae *AnalysisEngine) runAnalysis(ctx context.Context) {
	if ae.chDB == nil { return }
	result := &AnalysisResult{Timestamp: time.Now(), EventTypes: make(map[string]uint64), Categories: make(map[string]uint64), Probes: make(map[string]uint64), TimeWindow: "last_5m"}
	var totalEvents sql.NullInt64
	if err := ae.chDB.QueryRowContext(ctx, "SELECT count() FROM cloudflow.ebpf_events WHERE timestamp >= toUnixTimestamp(now() - toIntervalMinute(5)) * 1000000000").Scan(&totalEvents); err == nil && totalEvents.Valid {
		result.TotalEvents = uint64(totalEvents.Int64)
	}
	if rows, err := ae.chDB.QueryContext(ctx, "SELECT event_type, count() FROM cloudflow.ebpf_events WHERE timestamp >= toUnixTimestamp(now() - toIntervalMinute(5)) * 1000000000 GROUP BY event_type"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var et string; var c sql.NullInt64
			if rows.Scan(&et, &c) == nil && c.Valid { result.EventTypes[et] = uint64(c.Int64) }
		}
	}
	if rows, err := ae.chDB.QueryContext(ctx, "SELECT category, count() FROM cloudflow.ebpf_events WHERE timestamp >= toUnixTimestamp(now() - toIntervalMinute(5)) * 1000000000 GROUP BY category"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var cat string; var c sql.NullInt64
			if rows.Scan(&cat, &c) == nil && c.Valid { result.Categories[cat] = uint64(c.Int64) }
		}
	}
	if rows, err := ae.chDB.QueryContext(ctx, "SELECT probe_id, count() FROM cloudflow.ebpf_events WHERE timestamp >= toUnixTimestamp(now() - toIntervalMinute(5)) * 1000000000 GROUP BY probe_id"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var pid string; var c sql.NullInt64
			if rows.Scan(&pid, &c) == nil && c.Valid { result.Probes[pid] = uint64(c.Int64) }
		}
	}
	ae.mu.Lock(); ae.lastResult = result; ae.mu.Unlock()
	fmt.Printf("[AnalysisEngine] Complete: %d events\n", result.TotalEvents)
}

func (s *Service) analysisHandler(w http.ResponseWriter, r *http.Request) {
	if s.analysisEngine == nil { http.Error(w, "not ready", 503); return }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.analysisEngine.GetResult())
}

func (s *Service) analysisEventsHandler(w http.ResponseWriter, r *http.Request) {
	if s.clickHouseDB == nil { http.Error(w, "not ready", 503); return }
	type Trend struct { Timestamp string `json:"timestamp"`; Count uint64 `json:"count"` }
	var trends []Trend
	rows, err := s.clickHouseDB.QueryContext(r.Context(), "SELECT toStartOfMinute(fromUnixTimestamp(intDiv(timestamp, 1000000000))) as ts, count() FROM cloudflow.ebpf_events WHERE timestamp >= toUnixTimestamp(now() - toIntervalMinute(30)) * 1000000000 GROUP BY ts ORDER BY ts")
	if err != nil { http.Error(w, err.Error(), 500); return }
	defer rows.Close()
	for rows.Next() {
		var ts time.Time; var c uint64
		if rows.Scan(&ts, &c) == nil { trends = append(trends, Trend{Timestamp: ts.Format("2006-01-02 15:04:05"), Count: c}) }
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trends)
}

func (s *Service) analysisTopHandler(w http.ResponseWriter, r *http.Request) {
	if s.clickHouseDB == nil { http.Error(w, "not ready", 503); return }
	cat := r.URL.Query().Get("category")
	if cat == "" { cat = "process" }
	type Item struct { Name string `json:"name"`; Count uint64 `json:"count"` }
	var items []Item
	q := fmt.Sprintf("SELECT details, count() FROM cloudflow.ebpf_events WHERE timestamp >= toUnixTimestamp(now() - toIntervalMinute(5)) * 1000000000 AND category = '%s' GROUP BY details ORDER BY count() DESC LIMIT 20", cat)
	rows, err := s.clickHouseDB.QueryContext(r.Context(), q)
	if err != nil { http.Error(w, err.Error(), 500); return }
	defer rows.Close()
	for rows.Next() {
		var d string; var c uint64
		if rows.Scan(&d, &c) == nil { items = append(items, Item{Name: d, Count: c}) }
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}
