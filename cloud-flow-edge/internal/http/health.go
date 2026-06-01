// Package http 提供 HTTP 服务功能
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"cloud-flow-edge/internal/probemgr"
	"cloud-flow-edge/pkg/logger"
)

// Version 版本号，可在编译时通过 -ldflags "-X cloud-flow-edge/internal/http.Version=x.y.z" 注入
var Version = "dev"

type HealthHandler struct {
	manager   *probemgr.Manager
	logger    *logger.Logger
	startTime time.Time
}

func NewHealthHandler(manager *probemgr.Manager, log *logger.Logger) *HealthHandler {
	return &HealthHandler{
		manager:   manager,
		logger:    log,
		startTime: time.Now(),
	}
}

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
	Probes    int       `json:"probes"`
	Version   string    `json:"version"`
}

func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Uptime:    time.Since(h.startTime).String(),
		Probes:    h.manager.GetProbeCount(),
		Version:   Version,
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
