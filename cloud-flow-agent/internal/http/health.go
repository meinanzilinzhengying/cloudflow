// Package http 提供 HTTP 服务功能
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"google.golang.org/grpc/connectivity"

	"cloudflow-agent/internal/collector"
	"cloudflow-agent/internal/ebpfcollector"
	"cloudflow-agent/internal/grpcclient"
	"cloudflow-agent/internal/sqlaggregator"
	"cloudflow-agent/pkg/logger"
)

var Version = "dev"

type ClientGetter interface {
	Get() *grpcclient.Client
}

type HealthHandler struct {
	clientGetter  ClientGetter
	collector     *collector.Collector
	ebpfCollector *ebpfcollector.Collector
	cpuProfiler   interface{ IsEnabled() bool }
	sqlAggregator *sqlaggregator.SQLAggregator
	logger        *logger.Logger
	startTime     time.Time
}

func NewHealthHandler(clientGetter ClientGetter, collector *collector.Collector, ebpfCollector *ebpfcollector.Collector, cpuProfiler interface{ IsEnabled() bool }, sqlAggregator *sqlaggregator.SQLAggregator, log *logger.Logger) *HealthHandler {
	return &HealthHandler{
		clientGetter:  clientGetter,
		collector:     collector,
		ebpfCollector: ebpfCollector,
		cpuProfiler:   cpuProfiler,
		sqlAggregator: sqlAggregator,
		logger:        log,
		startTime:     time.Now(),
	}
}

type HealthResponse struct {
	Status             string       `json:"status"`
	Timestamp          time.Time    `json:"timestamp"`
	Uptime             string       `json:"uptime"`
	EdgeConnected      bool         `json:"edge_connected"`
	EBPFAvailable      bool         `json:"ebpf_available"`
	TCPMetricsEnabled  bool         `json:"tcp_metrics_enabled"`
	HTTPMetricsEnabled bool         `json:"http_metrics_enabled"`
	HTTPFullEnabled    bool         `json:"http_full_enabled"`
	DNSFullEnabled     bool         `json:"dns_full_enabled"`
	MySQLFullEnabled   bool         `json:"mysql_full_enabled"`
	SQLAggEnabled      bool         `json:"sql_agg_enabled"`
	SQLAggStats        *SQLAggStats `json:"sql_agg_stats,omitempty"`
	CPUProfilerEnabled bool         `json:"cpu_profiler_enabled"`
	Version            string       `json:"version"`
}

type SQLAggStats struct {
	TotalRequests uint64  `json:"total_requests"`
	SuccessRate   float64 `json:"success_rate"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	SlowQueries   uint64  `json:"slow_queries"`
	QueriesPerSec uint64  `json:"queries_per_sec"`
	ProcessCount  int     `json:"process_count"`
}

func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	edgeConnected := false
	if h.clientGetter != nil {
		client := h.clientGetter.Get()
		if client != nil {
			state := client.GetState()
			edgeConnected = state == connectivity.Ready
		}
	}
	ebpfAvailable := h.ebpfCollector != nil
	tcpMetricsEnabled := false
	if h.ebpfCollector != nil {
		tcpMetricsEnabled = h.ebpfCollector.IsTCPMetricsAvailable()
	}
	httpMetricsEnabled := false
	if h.ebpfCollector != nil {
		httpMetricsEnabled = h.ebpfCollector.IsHTTPMetricsAvailable()
	}
	httpFullEnabled := false
	dnsFullEnabled := false
	mysqlFullEnabled := false
	if h.ebpfCollector != nil {
		httpFullEnabled = h.ebpfCollector.IsHTTPFullAvailable()
		dnsFullEnabled = h.ebpfCollector.IsDNSFullAvailable()
		mysqlFullEnabled = h.ebpfCollector.IsMySQLFullAvailable()
	}
	cpuProfilerEnabled := h.cpuProfiler != nil
	sqlAggEnabled := h.sqlAggregator != nil
	var sqlAggStats *SQLAggStats
	if sqlAggEnabled {
		stats := h.sqlAggregator.GetStats()
		if enabled, ok := stats["enabled"].(bool); ok && enabled {
			globalStats := h.sqlAggregator.GetGlobalStats()
			sqlAggStats = &SQLAggStats{
				TotalRequests: globalStats.TotalRequests,
				SuccessRate:   globalStats.SuccessRate(),
				AvgLatencyMs:  globalStats.AvgLatencyMs(),
				SlowQueries:   globalStats.SlowQueries,
				QueriesPerSec: globalStats.Queries1s,
				ProcessCount:  len(h.sqlAggregator.GetDBProcessStats()),
			}
		}
	}
	response := HealthResponse{
		Status:             "healthy",
		Timestamp:          time.Now(),
		Uptime:             time.Since(h.startTime).String(),
		EdgeConnected:      edgeConnected,
		EBPFAvailable:      ebpfAvailable,
		TCPMetricsEnabled:  tcpMetricsEnabled,
		HTTPMetricsEnabled: httpMetricsEnabled,
		HTTPFullEnabled:    httpFullEnabled,
		DNSFullEnabled:     dnsFullEnabled,
		MySQLFullEnabled:   mysqlFullEnabled,
		SQLAggEnabled:      sqlAggEnabled,
		SQLAggStats:        sqlAggStats,
		CPUProfilerEnabled: cpuProfilerEnabled,
		Version:            Version,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Warnf("发送健康检查响应失败: %v", err)
	}
}

type Server struct {
	server *http.Server
}

func StartHealthServer(addr string, handler *HealthHandler) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.HandleHealth)
	mux.HandleFunc("/ready", handler.HandleHealth)
	mux.HandleFunc("/live", handler.HandleHealth)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	s := &Server{server: server}
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			handler.logger.Warnf("健康检查 HTTP 服务器错误: %v", err)
		}
	}()
	return s
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
