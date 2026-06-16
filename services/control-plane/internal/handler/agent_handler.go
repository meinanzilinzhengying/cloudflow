package handler

import (
	"encoding/json"
	"net/http"
	"strings"
)

// AgentHandler Agent管理API处理器
type AgentHandler struct{}

// NewAgentHandler 创建Agent处理器
func NewAgentHandler() *AgentHandler {
	return &AgentHandler{}
}

// ListAgents 获取Agent列表
// GET /api/control-plane/agents
func (h *AgentHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 模拟数据，实际对接gRPC和数据库
	agents := []map[string]interface{}{
		{
			"id":       "agent-001",
			"name":     "web-server-01",
			"ip":       "192.168.1.101",
			"status":   "online",
			"version":  "v1.0.0",
			"uptime":   "12h30m",
			"traffic":  "1.2GB",
			"hostname": "web-server-01",
			"os":       "Ubuntu 22.04",
			"kernel":   "5.15.0-76-generic",
		},
		{
			"id":       "agent-002",
			"name":     "db-server-01",
			"ip":       "192.168.1.102",
			"status":   "online",
			"version":  "v1.0.0",
			"uptime":   "8h15m",
			"traffic":  "512MB",
			"hostname": "db-server-01",
			"os":       "CentOS 8",
			"kernel":   "5.4.0-100-generic",
		},
		{
			"id":       "agent-003",
			"name":     "app-server-01",
			"ip":       "192.168.1.103",
			"status":   "offline",
			"version":  "v0.9.9",
			"uptime":   "0h0m",
			"traffic":  "0B",
			"hostname": "app-server-01",
			"os":       "Debian 11",
			"kernel":   "5.10.0-0.bpo.9-amd64",
		},
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": agents,
	})
}

// GetAgentStatus 获取Agent状态统计
// GET /api/control-plane/agents/status
func (h *AgentHandler) GetAgentStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 模拟数据
	json.NewEncoder(w).Encode(map[string]interface{}{
		"online":  2,
		"offline": 1,
		"total":   3,
	})
}

// GetAgent 获取单个Agent详情
// GET /api/control-plane/agents/:id
func (h *AgentHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 从URL路径中提取ID
	path := r.URL.Path
	parts := strings.Split(path, "/")
	id := parts[len(parts)-1]

	agent := map[string]interface{}{
		"id":       id,
		"name":     "agent-" + id,
		"ip":       "192.168.1.100",
		"status":   "online",
		"version":  "v1.0.0",
		"uptime":   "24h",
		"traffic":  "2.5GB",
		"hostname": "server-" + id,
		"os":       "Ubuntu 22.04",
		"kernel":   "5.15.0-76-generic",
		"interfaces": []map[string]interface{}{
			{"name": "eth0", "mac": "00:11:22:33:44:55", "rx_bytes": 1024000000, "tx_bytes": 512000000},
		},
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": agent,
	})
}

// StartAgent 启动Agent
// POST /api/control-plane/agents/:id/start
func (h *AgentHandler) StartAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "agent started successfully",
	})
}

// StopAgent 停止Agent
// POST /api/control-plane/agents/:id/stop
func (h *AgentHandler) StopAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "agent stopped successfully",
	})
}

// RestartAgent 重启Agent
// POST /api/control-plane/agents/:id/restart
func (h *AgentHandler) RestartAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "agent restarted successfully",
	})
}

// UpgradeAgent 升级Agent
// POST /api/control-plane/agents/:id/upgrade
func (h *AgentHandler) UpgradeAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "invalid request body",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "agent upgrading to " + req.Version,
	})
}

// PushConfig 下发配置给Agent
// POST /api/control-plane/agents/:id/config
func (h *AgentHandler) PushConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var config map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "invalid config",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "config pushed successfully",
	})
}

// GetAgentLogs 获取Agent日志
// GET /api/control-plane/agents/:id/logs
func (h *AgentHandler) GetAgentLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	logs := []string{
		"[INFO] 2026-06-16 10:00:00 Agent started",
		"[INFO] 2026-06-16 10:00:01 Loading eBPF programs...",
		"[INFO] 2026-06-16 10:00:02 eBPF program tc_bpf loaded successfully",
		"[INFO] 2026-06-16 10:00:03 eBPF program tcp_metrics_bpf loaded successfully",
		"[INFO] 2026-06-16 10:00:04 eBPF program http_metrics_bpf loaded successfully",
		"[INFO] 2026-06-16 10:00:05 Connecting to control plane...",
		"[INFO] 2026-06-16 10:00:06 Connected to control plane at 192.168.1.100:50051",
		"[INFO] 2026-06-16 10:00:07 Heartbeat sent, latency 5ms",
		"[INFO] 2026-06-16 10:01:00 Starting packet capture on eth0",
		"[INFO] 2026-06-16 10:01:01 Packet capture started, processing packets...",
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": logs,
	})
}

// ListEdges 获取Edge节点列表
// GET /api/control-plane/edges
func (h *AgentHandler) ListEdges(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	edges := []map[string]interface{}{
		{"id": "edge-001", "name": "edge-beijing", "status": "online", "region": "beijing", "agents": 5},
		{"id": "edge-002", "name": "edge-shanghai", "status": "online", "region": "shanghai", "agents": 3},
		{"id": "edge-003", "name": "edge-guangzhou", "status": "offline", "region": "guangzhou", "agents": 0},
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": edges,
	})
}
