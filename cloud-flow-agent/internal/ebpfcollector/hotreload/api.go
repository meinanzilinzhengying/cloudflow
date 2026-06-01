// Package hotreload eBPF 热更新 HTTP API 和健康检查
package hotreload

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SubsystemType eBPF 子系统类型
type SubsystemType string

const (
	SubsystemTCP    SubsystemType = "tcp"
	SubsystemHTTP   SubsystemType = "http"
	SubsystemDNS    SubsystemType = "dns"
	SubsystemMySQL  SubsystemType = "mysql"
	SubsystemTC     SubsystemType = "tc"
	SubsystemSocket SubsystemType = "socket"
	SubsystemVXLAN  SubsystemType = "vxlan"
	SubsystemDrop   SubsystemType = "drop"
	SubsystemCPU    SubsystemType = "cpu"
)

// HealthStatus 子系统健康状态
type HealthStatus struct {
	Subsystem SubsystemType `json:"subsystem"`
	Healthy   bool          `json:"healthy"`
	Message   string        `json:"message,omitempty"`
	CheckedAt time.Time     `json:"checked_at"`
}

// StatusResponse 整体状态响应
type StatusResponse struct {
	Version    string            `json:"version"`
	State      string            `json:"state"`
	Subsystems map[string]string `json:"subsystems"`
	Healthy    bool              `json:"healthy"`
}

// ReloadRequest 热更新请求
type ReloadRequest struct {
	Subsystem      string `json:"subsystem"`
	Version        string `json:"version"`
	BytecodeBase64 string `json:"bytecode_base64"`
}

// RollbackRequest 回滚请求
type RollbackRequest struct {
	Subsystem string `json:"subsystem"`
	All       bool   `json:"all"`
}

// MapsResponse pinned maps 列表响应
type MapsResponse struct {
	Maps []MapInfo `json:"maps"`
}

// MapInfo 单个 map 信息
type MapInfo struct {
	Name   string `json:"name"`
	Pinned bool   `json:"pinned"`
	Path   string `json:"path"`
}

// HealthChecker 健康检查器
type HealthChecker struct {
	manager             *ProgramManager
	checkInterval       time.Duration
	unhealthyThreshold  int
	failureCount        map[SubsystemType]int
	mu                  sync.Mutex
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(manager *ProgramManager, interval time.Duration) *HealthChecker {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &HealthChecker{
		manager:            manager,
		checkInterval:      interval,
		unhealthyThreshold: 3,
		failureCount:       make(map[SubsystemType]int),
	}
}

// Start 启动周期性健康检查
func (hc *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			results := hc.CheckAll()
			for sub, status := range results {
				if !status.Healthy {
					hc.mu.Lock()
					hc.failureCount[sub]++
					count := hc.failureCount[sub]
					hc.mu.Unlock()

					if hc.ShouldAutoRollback(sub) {
						_ = hc.manager.Rollback(sub)
						hc.mu.Lock()
						hc.failureCount[sub] = 0
						hc.mu.Unlock()
					} else {
						_ = count // 已在上方使用
					}
				} else {
					hc.mu.Lock()
					hc.failureCount[sub] = 0
					hc.mu.Unlock()
				}
			}
		}
	}
}

// CheckSubsystem 检查单个子系统的健康状态
func (hc *HealthChecker) CheckSubsystem(subsystem SubsystemType) *HealthStatus {
	status := &HealthStatus{
		Subsystem: subsystem,
		CheckedAt: time.Now(),
	}

	// 通过 manager 获取子系统的已知 map，尝试读取判断健康状态
	maps := hc.manager.GetSubsystemMaps(subsystem)
	if len(maps) == 0 {
		status.Healthy = false
		status.Message = fmt.Sprintf("subsystem %s has no registered maps", subsystem)
		return status
	}

	// 检查至少一个 map 是否可访问且有数据
	anyHealthy := false
	for _, m := range maps {
		if m != nil {
			iter := m.Iterate()
			var k, v interface{}
			if iter.Next(&k, &v) {
				anyHealthy = true
				iter.Close()
				break
			}
			iter.Close()
		}
	}

	if anyHealthy {
		status.Healthy = true
		status.Message = "subsystem is healthy, maps are active"
	} else {
		status.Healthy = false
		status.Message = "subsystem maps exist but contain no data"
	}

	return status
}

// CheckAll 检查所有已注册子系统的健康状态
func (hc *HealthChecker) CheckAll() map[SubsystemType]*HealthStatus {
	results := make(map[SubsystemType]*HealthStatus)
	subsystems := hc.manager.ListSubsystems()
	for _, sub := range subsystems {
		results[sub] = hc.CheckSubsystem(sub)
	}
	return results
}

// ShouldAutoRollback 检查是否需要自动回滚
func (hc *HealthChecker) ShouldAutoRollback(subsystem SubsystemType) bool {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	return hc.failureCount[subsystem] >= hc.unhealthyThreshold
}

// APIHandler HTTP API 处理器
type APIHandler struct {
	manager *ProgramManager
	checker *HealthChecker
}

// NewAPIHandler 创建 HTTP API 处理器
func NewAPIHandler(manager *ProgramManager, checker *HealthChecker) *APIHandler {
	return &APIHandler{
		manager: manager,
		checker: checker,
	}
}

// RegisterRoutes 注册所有 HTTP 路由
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/ebpf/status", h.handleStatus)
	mux.HandleFunc("/api/ebpf/health", h.handleHealth)
	mux.HandleFunc("/api/ebpf/health/", h.handleHealthSubsystem)
	mux.HandleFunc("/api/ebpf/reload", h.handleReload)
	mux.HandleFunc("/api/ebpf/rollback", h.handleRollback)
	mux.HandleFunc("/api/ebpf/version", h.handleVersion)
	mux.HandleFunc("/api/ebpf/maps", h.handleMaps)
}

// handleStatus GET /api/ebpf/status - 整体状态
func (h *APIHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subsystems := h.manager.ListSubsystems()
	subsystemStates := make(map[string]string, len(subsystems))
	allHealthy := true

	for _, sub := range subsystems {
		state := h.manager.GetSubsystemState(sub)
		subsystemStates[string(sub)] = state
		if state != "running" && state != "loaded" {
			allHealthy = false
		}
	}

	resp := StatusResponse{
		Version:    h.manager.GetCurrentVersion(),
		State:      h.manager.GetState(),
		Subsystems: subsystemStates,
		Healthy:    allHealthy,
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleHealth GET /api/ebpf/health - 所有子系统健康检查
func (h *APIHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	results := h.checker.CheckAll()
	writeJSON(w, http.StatusOK, results)
}

// handleHealthSubsystem GET /api/ebpf/health/{subsystem} - 单个子系统健康检查
func (h *APIHandler) handleHealthSubsystem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从路径中提取子系统名称: /api/ebpf/health/{subsystem}
	subsystemStr := strings.TrimPrefix(r.URL.Path, "/api/ebpf/health/")
	subsystemStr = strings.TrimSuffix(subsystemStr, "/")
	if subsystemStr == "" {
		http.Error(w, "subsystem name is required", http.StatusBadRequest)
		return
	}

	subsystem := SubsystemType(subsystemStr)
	status := h.checker.CheckSubsystem(subsystem)
	writeJSON(w, http.StatusOK, status)
}

// handleReload POST /api/ebpf/reload - 触发子系统热更新
func (h *APIHandler) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req ReloadRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.Subsystem == "" {
		http.Error(w, "subsystem is required", http.StatusBadRequest)
		return
	}
	if req.Version == "" {
		http.Error(w, "version is required", http.StatusBadRequest)
		return
	}
	if req.BytecodeBase64 == "" {
		http.Error(w, "bytecode_base64 is required", http.StatusBadRequest)
		return
	}

	bytecode, err := base64.StdEncoding.DecodeString(req.BytecodeBase64)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid base64 bytecode: %v", err), http.StatusBadRequest)
		return
	}

	subsystem := SubsystemType(req.Subsystem)
	if err := h.manager.Reload(subsystem, req.Version, bytecode); err != nil {
		http.Error(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"subsystem": req.Subsystem,
		"version":   req.Version,
		"message":   "reload successful",
	})
}

// handleRollback POST /api/ebpf/rollback - 回滚到上一版本
func (h *APIHandler) handleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req RollbackRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if !req.All && req.Subsystem == "" {
		http.Error(w, "either subsystem or all must be specified", http.StatusBadRequest)
		return
	}

	if req.All {
		if err := h.manager.RollbackAll(); err != nil {
			http.Error(w, fmt.Sprintf("rollback all failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "all subsystems rolled back successfully",
		})
		return
	}

	subsystem := SubsystemType(req.Subsystem)
	if err := h.manager.Rollback(subsystem); err != nil {
		http.Error(w, fmt.Sprintf("rollback failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"subsystem": req.Subsystem,
		"message":   "rollback successful",
	})
}

// handleVersion GET /api/ebpf/version - 当前版本信息
func (h *APIHandler) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	versionInfo := h.manager.GetVersionInfo()
	writeJSON(w, http.StatusOK, versionInfo)
}

// handleMaps GET /api/ebpf/maps - 列出所有 pinned maps
func (h *APIHandler) handleMaps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mapsInfo := h.manager.ListPinnedMaps()
	resp := MapsResponse{
		Maps: make([]MapInfo, 0, len(mapsInfo)),
	}

	for _, m := range mapsInfo {
		pinnedPath := m.PinPath
		resp.Maps = append(resp.Maps, MapInfo{
			Name:   filepath.Base(m.PinPath),
			Pinned: pinnedPath != "",
			Path:   pinnedPath,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// writeJSON 写入 JSON 响应
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// 如果写入响应体失败，记录日志（此处简单处理）
		http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
	}
}
