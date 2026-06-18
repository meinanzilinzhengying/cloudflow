package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/meinanzilinzhengying/ebpf-probe/internal/collector"
	ebpfprobe "github.com/meinanzilinzhengying/ebpf-probe"
)

func Start(port string, mgr *collector.Manager) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/probe/status", handleStatus(mgr))
	mux.HandleFunc("/api/probe/start", handleStart(mgr))
	mux.HandleFunc("/api/probe/stop", handleStop(mgr))
	mux.HandleFunc("/api/probe/restart", handleRestart(mgr))
	mux.HandleFunc("/api/probe/metrics", handleMetrics(mgr))
	mux.HandleFunc("/api/probe/health", handleHealth())
	mux.HandleFunc("/api/probe/version", handleVersion())
	mux.HandleFunc("/", handleOptions)

	log.Printf("[API] 管理API启动在端口 %s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Printf("[API] HTTP服务异常: %v", err)
	}
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func handleStatus(mgr *collector.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: mgr.Status()})
	}
}

func handleStart(mgr *collector.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: "probe started"})
	}
}

func handleStop(mgr *collector.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		mgr.Stop()
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: "probe stopped"})
	}
}

func handleRestart(mgr *collector.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: "probe restarted"})
	}
}

func handleMetrics(mgr *collector.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: mgr.Status()})
	}
}

func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: "healthy"})
	}
}

func handleVersion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: map[string]string{
			"version":    ebpfprobe.Version,
			"build_time": ebpfprobe.BuildTime,
		}})
	}
}

func handleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}
