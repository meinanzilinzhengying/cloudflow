// Package dataplane
// NOTE: Ingest logic has been moved to analysis.go. This file is kept for
// backward compatibility but ingest endpoints are no longer registered.
package dataplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ProbeEvent 探针发送的事件数据结构
type ProbeEvent struct {
	Timestamp string `json:"Timestamp"`
	ProbeID   string `json:"ProbeID"`
	Category  string `json:"Category"`
	EventType string `json:"EventType"`
	SrcIP     string `json:"SrcIP"`
	DstIP     string `json:"DstIP"`
	SrcPort   uint16 `json:"SrcPort"`
	DstPort   uint16 `json:"DstPort"`
	Protocol  string `json:"Protocol"`
	Bytes     uint64 `json:"Bytes"`
	Packets   uint64 `json:"Packets"`
	LatencyMs uint64 `json:"LatencyMs"`
	Service   string `json:"Service"`
	Details   string `json:"Details"`
	Tags      string `json:"Tags"`
}

// ingestHandler 接收探针 HTTP 数据并写入 ClickHouse
func (s *Service) ingestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var events []ProbeEvent
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if s.clickHouseDB == nil {
		http.Error(w, "ClickHouse not ready", http.StatusServiceUnavailable)
		return
	}

	// 写入 ClickHouse metrics 表
	tx, err := s.clickHouseDB.Begin()
	if err != nil {
		http.Error(w, fmt.Sprintf("DB begin: %v", err), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO cloudflow.metrics (timestamp, probe_id, metric_name, metric_value, labels)")
	if err != nil {
		http.Error(w, fmt.Sprintf("Prepare: %v", err), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	now := uint64(time.Now().UnixNano())
	for _, ev := range events {
		var ts uint64
		if t, err := time.Parse(time.RFC3339Nano, ev.Timestamp); err == nil {
			ts = uint64(t.UnixNano())
		} else {
			ts = now
		}

		metricName := ev.Category + "_" + ev.EventType
		if metricName == "_" {
			metricName = "probe_event"
		}

		labels := fmt.Sprintf(`{"probe_id":"%s","category":"%s","event_type":"%s","src_ip":"%s","dst_ip":"%s","protocol":"%s","service":"%s"}`,
			ev.ProbeID, ev.Category, ev.EventType, ev.SrcIP, ev.DstIP, ev.Protocol, ev.Service)

		metricValue := float64(ev.LatencyMs)
		if metricValue == 0 {
			metricValue = 1.0
		}

		if _, err := stmt.Exec(ts, ev.ProbeID, metricName, metricValue, labels); err != nil {
			fmt.Printf("[ingest] metric insert error: %v\n", err)
			continue
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, fmt.Sprintf("Commit: %v", err), http.StatusInternalServerError)
		return
	}

	s.addMetricsIngested(uint64(len(events)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true}`))
	fmt.Printf("[ingest] received %d events\n", len(events))
}
