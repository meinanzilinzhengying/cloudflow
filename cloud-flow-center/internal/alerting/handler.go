// Package alerting 提供告警规则引擎和通知功能
package alerting

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"cloudflow-center/pkg/logger"
)

type UserVerifier interface {
	VerifyUser(username, password string) (bool, string, error)
}

// Handler 告警 HTTP 处理器
type Handler struct {
	alertManager   *AlertManager
	logger         *logger.Logger
	verifier       UserVerifier
	allowedOrigins []string
}

// NewHandler 创建告警 HTTP 处理器
func NewHandler(alertManager *AlertManager, log *logger.Logger, verifier UserVerifier, allowedOrigins []string) *Handler {
	return &Handler{
		alertManager:   alertManager,
		logger:         log,
		verifier:       verifier,
		allowedOrigins: allowedOrigins,
	}
}

func (h *Handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Cloud Flow Alerting"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if h.verifier == nil {
			h.logger.Warnf("用户认证服务未配置，拒绝请求")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		valid, _, err := h.verifier.VerifyUser(user, pass)
		if err != nil || !valid {
			h.logger.Warnf("用户认证失败: %s", user)
			w.Header().Set("WWW-Authenticate", `Basic realm="Cloud Flow Alerting"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func (h *Handler) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && h.isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		next(w, r)
	}
}

func (h *Handler) isOriginAllowed(origin string) bool {
	for _, allowed := range h.allowedOrigins {
		if allowed == origin {
			return true
		}
	}
	return false
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	origin := r.Header.Get("Origin")
	if origin != "" && h.isOriginAllowed(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Errorf("JSON Encode 失败: %v", err)
	}
}

func (h *Handler) writeJSONWithStatus(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	origin := r.Header.Get("Origin")
	if origin != "" && h.isOriginAllowed(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
	}

	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Errorf("JSON Encode 失败: %v", err)
	}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/alerts", h.corsMiddleware(h.authMiddleware(h.handleAlerts)))
	mux.HandleFunc("/api/alerts/history", h.corsMiddleware(h.authMiddleware(h.handleAlertHistory)))
	mux.HandleFunc("/api/alerts/resolve", h.corsMiddleware(h.authMiddleware(h.handleResolveAlert)))
	mux.HandleFunc("/api/rules", h.corsMiddleware(h.authMiddleware(h.handleRules)))
	mux.HandleFunc("/api/rules/create", h.corsMiddleware(h.authMiddleware(h.handleCreateRule)))
	mux.HandleFunc("/api/rules/update", h.corsMiddleware(h.authMiddleware(h.handleUpdateRule)))
	mux.HandleFunc("/api/rules/delete", h.corsMiddleware(h.authMiddleware(h.handleDeleteRule)))
}

func (h *Handler) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		http.Error(w, `{"error":"告警服务不可用"}`, http.StatusServiceUnavailable)
		return
	}
	alerts := h.alertManager.GetActiveAlerts()
	h.writeJSON(w, r, map[string]interface{}{
		"status":  "success",
		"alerts":  alerts,
		"count":   len(alerts),
		"message": "获取活跃告警成功",
	})
}

func (h *Handler) handleAlertHistory(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		http.Error(w, `{"error":"告警服务不可用"}`, http.StatusServiceUnavailable)
		return
	}
	history := h.alertManager.GetAlertHistory()
	h.writeJSON(w, r, map[string]interface{}{
		"status":  "success",
		"history": history,
		"count":   len(history),
		"message": "获取告警历史成功",
	})
}

func (h *Handler) handleResolveAlert(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		http.Error(w, `{"error":"告警服务不可用"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		h.writeJSONWithStatus(w, r, http.StatusMethodNotAllowed, map[string]interface{}{
			"status":  "error",
			"message": "只支持 POST 方法",
		})
		return
	}

	var req struct {
		AlertID string `json:"alert_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "请求体格式错误",
		})
		return
	}

	if req.AlertID == "" {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "告警 ID 不能为空",
		})
		return
	}

	if err := h.alertManager.ResolveAlert(req.AlertID); err != nil {
		h.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{
			"status":  "error",
			"message": "解决告警失败",
		})
		return
	}

	h.writeJSON(w, r, map[string]interface{}{
		"status":  "success",
		"message": "告警已解决",
	})
}

func (h *Handler) handleRules(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		http.Error(w, `{"error":"告警服务不可用"}`, http.StatusServiceUnavailable)
		return
	}
	rules := h.alertManager.GetRuleManager().GetRules()
	h.writeJSON(w, r, map[string]interface{}{
		"status":  "success",
		"rules":   rules,
		"count":   len(rules),
		"message": "获取规则成功",
	})
}

func (h *Handler) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		http.Error(w, `{"error":"告警服务不可用"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		h.writeJSONWithStatus(w, r, http.StatusMethodNotAllowed, map[string]interface{}{
			"status":  "error",
			"message": "只支持 POST 方法",
		})
		return
	}

	var rule Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "请求体格式错误",
		})
		return
	}

	if rule.Name == "" {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "规则名称不能为空",
		})
		return
	}

	validSeverities := map[string]bool{"info": true, "warning": true, "critical": true}
	if rule.Severity == "" {
		rule.Severity = "warning"
	}
	if !validSeverities[rule.Severity] {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "无效的严重程度，允许的值: info, warning, critical",
		})
		return
	}

	if rule.Duration.Duration <= 0 {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "持续时间必须大于 0",
		})
		return
	}
	if rule.Condition.Threshold == 0 {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "告警阈值不能为 0",
		})
		return
	}

	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}

	if err := h.alertManager.GetRuleManager().SaveRule(&rule); err != nil {
		h.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{
			"status":  "error",
			"message": "创建规则失败",
		})
		return
	}

	h.writeJSON(w, r, map[string]interface{}{
		"status":  "success",
		"rule":    rule,
		"message": "规则创建成功",
	})
}

func (h *Handler) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		http.Error(w, `{"error":"告警服务不可用"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPut {
		h.writeJSONWithStatus(w, r, http.StatusMethodNotAllowed, map[string]interface{}{
			"status":  "error",
			"message": "只支持 PUT 方法",
		})
		return
	}

	var rule Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "请求体格式错误",
		})
		return
	}

	if rule.ID == "" {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "规则 ID 不能为空",
		})
		return
	}

	if rule.Name == "" {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "规则名称不能为空",
		})
		return
	}

	validSeverities := map[string]bool{"info": true, "warning": true, "critical": true}
	if rule.Severity != "" && !validSeverities[rule.Severity] {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "无效的严重程度，允许的值: info, warning, critical",
		})
		return
	}

	if rule.Duration.Duration <= 0 {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "持续时间必须大于 0",
		})
		return
	}

	if err := h.alertManager.GetRuleManager().SaveRule(&rule); err != nil {
		h.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{
			"status":  "error",
			"message": "更新规则失败",
		})
		return
	}

	h.writeJSON(w, r, map[string]interface{}{
		"status":  "success",
		"rule":    rule,
		"message": "规则更新成功",
	})
}

func (h *Handler) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		http.Error(w, `{"error":"告警服务不可用"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodDelete {
		h.writeJSONWithStatus(w, r, http.StatusMethodNotAllowed, map[string]interface{}{
			"status":  "error",
			"message": "只支持 DELETE 方法",
		})
		return
	}

	var req struct {
		RuleID string `json:"rule_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "请求体格式错误",
		})
		return
	}

	if req.RuleID == "" {
		h.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{
			"status":  "error",
			"message": "规则 ID 不能为空",
		})
		return
	}

	if err := h.alertManager.GetRuleManager().DeleteRule(req.RuleID); err != nil {
		h.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{
			"status":  "error",
			"message": "删除规则失败",
		})
		return
	}

	h.writeJSON(w, r, map[string]interface{}{
		"status":  "success",
		"message": "规则删除成功",
	})
}
