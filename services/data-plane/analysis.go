package dataplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// categoryTables maps table name to whether timestamp is UInt64 (true) or DateTime (false)
var categoryTables = map[string]bool{
	"file_events":     false,
	"protocol_events": false,
	"process_events":  false,
	"host_metrics":    false,
	"network_events":  true, // UInt64 nanoseconds
	"security_events": true, // UInt64 nanoseconds
	"syscall_events":  false,
}

func buildTimeFilter(table string, isUInt64 bool) string {
	if isUInt64 {
		return fmt.Sprintf("toDateTime(intDiv(timestamp, 1000000000)) >= now() - INTERVAL 24 HOUR")
	}
	return fmt.Sprintf("timestamp >= now() - INTERVAL 24 HOUR")
}

func (ae *AnalysisEngine) runAnalysis(ctx context.Context) {
	if ae.chDB == nil {
		return
	}
	result := &AnalysisResult{Timestamp: time.Now(), EventTypes: make(map[string]uint64), Categories: make(map[string]uint64), Probes: make(map[string]uint64), TimeWindow: "last_24h"}

	var totalEvents uint64
	for table, isUInt64 := range categoryTables {
		filter := buildTimeFilter(table, isUInt64)
		q := fmt.Sprintf("SELECT count() FROM cloudflow.%s WHERE %s", table, filter)
		var cnt sql.NullInt64
		if err := ae.chDB.QueryRowContext(ctx, q).Scan(&cnt); err == nil && cnt.Valid {
			totalEvents += uint64(cnt.Int64)
			result.Categories[table] = uint64(cnt.Int64)
		}
	}
	result.TotalEvents = totalEvents

	// Event types across all tables
	for table, isUInt64 := range categoryTables {
		filter := buildTimeFilter(table, isUInt64)
		q := fmt.Sprintf("SELECT event_type, count() FROM cloudflow.%s WHERE %s GROUP BY event_type", table, filter)
		if rows, err := ae.chDB.QueryContext(ctx, q); err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var et string
					var c sql.NullInt64
					if rows.Scan(&et, &c) == nil && c.Valid {
						result.EventTypes[et] += uint64(c.Int64)
					}
				}
			}()
		}
	}

	// Probes across all tables
	for table, isUInt64 := range categoryTables {
		filter := buildTimeFilter(table, isUInt64)
		q := fmt.Sprintf("SELECT probe_id, count() FROM cloudflow.%s WHERE %s GROUP BY probe_id", table, filter)
		if rows, err := ae.chDB.QueryContext(ctx, q); err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var pid string
					var c sql.NullInt64
					if rows.Scan(&pid, &c) == nil && c.Valid {
						result.Probes[pid] += uint64(c.Int64)
					}
				}
			}()
		}
	}

	ae.mu.Lock()
	ae.lastResult = result
	ae.mu.Unlock()
	fmt.Printf("[AnalysisEngine] Complete: %d events across %d categories\n", result.TotalEvents, len(result.Categories))
}

func (s *Service) analysisHandler(w http.ResponseWriter, r *http.Request) {
	if s.analysisEngine == nil {
		http.Error(w, "not ready", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.analysisEngine.GetResult())
}

func (s *Service) analysisEventsHandler(w http.ResponseWriter, r *http.Request) {
	if s.clickHouseDB == nil {
		http.Error(w, "not ready", 503)
		return
	}
	type Trend struct {
		Timestamp string `json:"timestamp"`
		Count     uint64 `json:"count"`
	}
	var trends []Trend

	// Union all tables, normalize timestamps to DateTime
	unionParts := []string{}
	for table, isUInt64 := range categoryTables {
		filter := buildTimeFilter(table, isUInt64)
		var tsExpr string
		if isUInt64 {
			tsExpr = fmt.Sprintf("toStartOfMinute(toDateTime(intDiv(timestamp, 1000000000)))")
		} else {
			tsExpr = fmt.Sprintf("toStartOfMinute(timestamp)")
		}
		unionParts = append(unionParts, fmt.Sprintf("SELECT %s as ts, count() as cnt FROM cloudflow.%s WHERE %s GROUP BY ts", tsExpr, table, filter))
	}

	unionSQL := strings.Join(unionParts, " UNION ALL ")
	q := fmt.Sprintf("SELECT ts, sum(cnt) FROM (%s) GROUP BY ts ORDER BY ts", unionSQL)

	rows, err := s.clickHouseDB.QueryContext(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var ts time.Time
		var c uint64
		if rows.Scan(&ts, &c) == nil {
			trends = append(trends, Trend{Timestamp: ts.Format("2006-01-02 15:04:05"), Count: c})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trends)
}

func (s *Service) analysisTopHandler(w http.ResponseWriter, r *http.Request) {
	if s.clickHouseDB == nil {
		http.Error(w, "not ready", 503)
		return
	}
	cat := r.URL.Query().Get("category")
	if cat == "" {
		cat = "process"
	}

	// Map category to table name
	tableMap := map[string]string{
		"file":     "file_events",
		"protocol": "protocol_events",
		"process":  "process_events",
		"host":     "host_metrics",
		"network":  "network_events",
		"security": "security_events",
		"syscall":  "syscall_events",
	}
	table, ok := tableMap[cat]
	if !ok {
		table = "process_events"
	}

	isUInt64 := categoryTables[table]
	filter := buildTimeFilter(table, isUInt64)

	type Item struct {
		Name  string `json:"name"`
		Count uint64 `json:"count"`
	}
	var items []Item
	q := fmt.Sprintf("SELECT details, count() FROM cloudflow.%s WHERE %s GROUP BY details ORDER BY count() DESC LIMIT 20", table, filter)
	rows, err := s.clickHouseDB.QueryContext(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var c uint64
		if rows.Scan(&d, &c) == nil {
			items = append(items, Item{Name: d, Count: c})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}
