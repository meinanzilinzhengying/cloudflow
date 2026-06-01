// Package portal 提供 Web 前端展示服务
package portal

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud-flow-center/internal/alerting"
	"cloud-flow-center/internal/config"
	"cloud-flow-center/internal/security"
	"cloud-flow-center/internal/storage"
	"cloud-flow-center/pkg/audit"
	"cloud-flow-center/pkg/auth"
	"cloud-flow-center/pkg/logger"
	"cloud-flow/pkg/ratelimit"
	"cloud-flow/pkg/safety"
)

//go:embed static/*
var staticFiles embed.FS

// errTooLarge is a sentinel error for request entity too large.
// Replaces the removed http.ErrTooLarge (Go 1.24+).
var errTooLarge = errors.New("http: request entity too large")

// Server Portal HTTP 服务
type Server struct {
	store              storage.StorageEngine
	logger             *logger.Logger
	jwtSecret          string
	jwtManager         *auth.JWTManager
	auditLogger        *audit.Logger
	alertManager       *alerting.AlertManager
	rateLimiter        *ratelimit.MultiLevelRateLimiter // 多级限流器
	rateLimitEnabled   bool
	securityMiddleware *security.SecurityMiddleware
	configMgr          *config.ConfigManager // 配置管理器
	csrfTokens         map[string]time.Time
	csrfMutex          sync.Mutex
	allowedOrigins     []string
	loginFailures      map[string]loginFailureInfo
	loginFailuresMutex sync.Mutex
	secureCookie       bool // 是否使用安全Cookie（HTTPS环境）
	tokenDuration      time.Duration // JWT 令牌有效期
	redisStore         *RedisStore // Redis 外部存储（可选，nil 时使用内存存储）
	centerConfig       *config.Config // 中心服务配置（用于系统配置查询）
}

// RateLimitLevel 限流级别
const (
	RateLimitLevelLogin = "login"
	RateLimitLevelQuery = "query"
	RateLimitLevelAPI   = "api"
)

type loginFailureInfo struct {
	count int
	lastFailure time.Time
}

// NewServer 创建 Portal 服务
func NewServer(store storage.StorageEngine, jwtSecret string, auditLogger *audit.Logger, alertManager *alerting.AlertManager, log *logger.Logger, secureCookie bool, rateLimitCfg config.RateLimitConfig, tokenDuration time.Duration, redisAddr string, centerCfg *config.Config, configMgr *config.ConfigManager) *Server {
	jwtManager := auth.NewJWTManager(jwtSecret)
	
	// 初始化多级速率限制器
	rateLimiter := ratelimit.NewMultiLevelRateLimiter()
	if rateLimitCfg.Enabled {
		rateLimiter.RegisterLevel(RateLimitLevelLogin, rateLimitCfg.Login.BucketSize, rateLimitCfg.Login.RefillRate, rateLimitCfg.Login.CleanupInterval)
		rateLimiter.RegisterLevel(RateLimitLevelQuery, rateLimitCfg.Query.BucketSize, rateLimitCfg.Query.RefillRate, rateLimitCfg.Query.CleanupInterval)
		rateLimiter.RegisterLevel(RateLimitLevelAPI, rateLimitCfg.API.BucketSize, rateLimitCfg.API.RefillRate, rateLimitCfg.API.CleanupInterval)
		log.Infof("速率限制已启用: Login(bucket=%d, refill=%d), Query(bucket=%d, refill=%d), API(bucket=%d, refill=%d)",
			rateLimitCfg.Login.BucketSize, rateLimitCfg.Login.RefillRate,
			rateLimitCfg.Query.BucketSize, rateLimitCfg.Query.RefillRate,
			rateLimitCfg.API.BucketSize, rateLimitCfg.API.RefillRate,
		)
	}
	
	// 初始化安全中间件
	securityMiddleware := security.NewSecurityMiddleware(security.SecurityConfig{
		EnableParamValidation: true,
		EnableAuditLog:        true,
		MaxRequestBodySize:    10 * 1024 * 1024, // 10MB
		AllowedContentTypes:   []string{"application/json", "application/x-www-form-urlencoded", "multipart/form-data"},
	}, log.Warnf)
	
	// 初始化 CSRF 令牌映射
	csrfTokens := make(map[string]time.Time)
	// 初始化登录失败计数器
	loginFailures := make(map[string]loginFailureInfo)
	// 从环境变量或配置读取允许的 CORS 来源
	allowedOrigins := []string{"http://localhost:3000", "http://localhost:8080"} // 默认值
	if origins := os.Getenv("CLOUD_FLOW_CORS_ALLOWED_ORIGINS"); origins != "" {
		allowedOrigins = strings.Split(origins, ",")
		log.Infof("从环境变量读取 CORS 允许的来源: %v", allowedOrigins)
	}
	// 设置默认 token 有效期
	if tokenDuration <= 0 {
		tokenDuration = 24 * time.Hour
	}

	// 尝试初始化 Redis 存储（可选）
	var redisStore *RedisStore
	if redisAddr != "" {
		rs, err := NewRedisStore(redisAddr)
		if err != nil {
			log.Warnf("连接 Redis 失败: %v，将回退到内存存储 CSRF token 和登录失败计数", err)
		} else {
			redisStore = rs
			log.Infof("Redis 存储已启用，地址: %s", redisAddr)
		}
	}

	log.Infof("CSRF Cookie Secure 属性设置为: %v", secureCookie)
	return &Server{
		store:              store,
		jwtSecret:          jwtSecret,
		jwtManager:        jwtManager,
		auditLogger:        auditLogger,
		alertManager:       alertManager,
		logger:             log,
		rateLimiter:        rateLimiter,
		rateLimitEnabled:   rateLimitCfg.Enabled,
		securityMiddleware:  securityMiddleware,
		configMgr:          configMgr,
		csrfTokens:         csrfTokens,
		allowedOrigins:     allowedOrigins,
		loginFailures:     loginFailures,
		secureCookie:       secureCookie,
		tokenDuration:      tokenDuration,
		redisStore:         redisStore,
		centerConfig:       centerCfg,
	}
}

// maxBytesMiddleware 全局请求体大小限制中间件，限制为 1MB
func (s *Server) maxBytesMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 仅对需要读取请求体的方法进行限制
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB 限制
		}
		next.ServeHTTP(w, r)
	})
}

// getClientIP 获取客户端真实 IP
func (s *Server) getClientIP(r *http.Request) string {
	// 检查 X-Forwarded-For 头部（处理反向代理的情况）
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For 可能包含多个 IP，取第一个
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	
	// 检查 X-Real-IP 头部
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// 从 RemoteAddr 获取
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitMiddleware 速率限制中间件（通用 API 接口）
func (s *Server) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return s.rateLimitMiddlewareWithLevel(next, RateLimitLevelAPI)
}

// rateLimitMiddlewareWithLevel 速率限制中间件（指定级别）
func (s *Server) rateLimitMiddlewareWithLevel(next http.HandlerFunc, level string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimitEnabled {
			ip := s.getClientIP(r)
			if !s.rateLimiter.Allow(level, ip) {
				s.logger.Warnf("Rate limit exceeded for %s: IP=%s, Level=%s", r.URL.Path, ip, level)
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}
		next(w, r)
	}
}

// loginRateLimitMiddleware 登录接口专用速率限制中间件
func (s *Server) loginRateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return s.rateLimitMiddlewareWithLevel(next, RateLimitLevelLogin)
}

// queryRateLimitMiddleware 查询接口专用速率限制中间件
func (s *Server) queryRateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return s.rateLimitMiddlewareWithLevel(next, RateLimitLevelQuery)
}

// generateCSRFToken 生成 CSRF 令牌
func (s *Server) generateCSRFToken() string {
	randomStr, err := auth.GenerateRandomString(32)
	if err != nil {
		s.logger.Errorf("生成随机字符串失败: %v", err)
		return "" // 生成失败返回空，由调用方处理
	}
	token := fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomStr)

	// 优先使用 Redis 存储
	if s.redisStore != nil {
		if err := s.redisStore.SetCSRFToken(token, 15*time.Minute); err != nil {
			s.logger.Errorf("Redis 设置 CSRF token 失败，回退到内存存储: %v", err)
		} else {
			return token
		}
	}

	// 回退到内存存储
	s.csrfMutex.Lock()
	// 清理过期的令牌
	now := time.Now()
	for t, expiry := range s.csrfTokens {
		if expiry.Before(now) {
			delete(s.csrfTokens, t)
		}
	}
	// 防止内存无限增长：超过 10000 个 token 时拒绝生成
	if len(s.csrfTokens) >= 10000 {
		s.csrfMutex.Unlock()
		s.logger.Warn("CSRF token 数量已达上限 (10000)，跳过生成")
		return ""
	}
	s.csrfTokens[token] = time.Now().Add(15 * time.Minute) // 15分钟过期
	s.csrfMutex.Unlock()
	return token
}

// validateCSRFToken 验证 CSRF 令牌
func (s *Server) validateCSRFToken(token string) bool {
	// 优先使用 Redis 验证
	if s.redisStore != nil {
		if s.redisStore.ValidateCSRFToken(token) {
			return true
		}
		// Redis 未命中，继续尝试内存存储（兼容回退场景）
	}

	s.csrfMutex.Lock()
	defer s.csrfMutex.Unlock()
	
	// 清理过期的令牌
	now := time.Now()
	for t, expiry := range s.csrfTokens {
		if expiry.Before(now) {
			delete(s.csrfTokens, t)
		}
	}
	
	// 验证令牌
	if expiry, ok := s.csrfTokens[token]; ok {
		if expiry.After(now) {
			// 令牌有效，使用后删除
			delete(s.csrfTokens, token)
			return true
		}
	}
	return false
}

// csrfMiddleware CSRF 保护中间件
func (s *Server) csrfMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 对于 GET 请求，生成并设置 CSRF 令牌
		if r.Method == http.MethodGet {
			token := s.generateCSRFToken()
			if token == "" {
				http.Error(w, "Failed to generate CSRF token", http.StatusInternalServerError)
				return
			}
			// 设置 HttpOnly cookie（用于 SameSite 保护，防止跨站请求携带）
			http.SetCookie(w, &http.Cookie{
				Name:     "csrf_token",
				Value:    token,
				Path:     "/",
				HttpOnly: true, // 禁止前端 JS 读取，防止 XSS 窃取
				Secure:   s.secureCookie, // 根据环境动态设置：HTTPS环境为true，HTTP环境为false
				SameSite: http.SameSiteLaxMode,
				MaxAge:   15 * 60, // 15分钟
			})
			// 同时通过响应头返回 token，让前端 JS 读取并在后续请求中通过 X-CSRF-Token 头提交
			w.Header().Set("X-CSRF-Token", token)
		} else if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			// 对于非 GET 请求，验证 CSRF 令牌
			// 只从请求头或表单获取 token，不再从 cookie 读取（防止绕过）
			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				// 尝试从表单中获取
				token = r.FormValue("csrf_token")
			}
			if !s.validateCSRFToken(token) {
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

type contextKey string

const (
	userContextKey contextKey = "user"
	roleContextKey contextKey = "role"
)

// authMiddleware 认证中间件（支持 Basic Auth 和 JWT）
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var userID, role string

		// 检查 JWT 令牌
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString := authHeader[7:]
			claims, err := s.validateJWT(tokenString)
			if err == nil && claims != nil {
				// JWT 验证成功
				// 安全地获取 user_id
				var ok bool
				userID, ok = claims["user_id"].(string)
				if !ok {
					userID = ""
				}
				role, ok = claims["role"].(string)
				if !ok {
					role = ""
				}
				if userID == "" {
					http.Error(w, "invalid token", http.StatusUnauthorized)
					return
				}
				// 记录审计日志
				if s.auditLogger != nil {
					safety.CheckAndWarn(s.logger, s.auditLogger.Log(
						audit.ActionLogin,
						userID,
						"portal",
						"authenticate",
						"success",
						"JWT authentication successful",
						claims,
						r.RemoteAddr,
						r.UserAgent(),
					), "记录审计日志失败")
				}
				// 将用户信息存入 context
				ctx := r.Context()
				ctx = context.WithValue(ctx, userContextKey, userID)
				ctx = context.WithValue(ctx, roleContextKey, role)
				next(w, r.WithContext(ctx))
				return
			}
		}

		// 回退到 Basic Auth
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Cloud Flow Portal"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 使用用户管理系统验证
		var valid bool
		var err error

		if s.store == nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="Cloud Flow Portal"`)
			http.Error(w, "Storage service unavailable", http.StatusServiceUnavailable)
			return
		}

		valid, role, err = s.store.VerifyUser(user, pass)
		userID = user

		if err != nil || !valid {
			// 记录审计日志
			if s.auditLogger != nil {
				safety.CheckAndWarn(s.logger, s.auditLogger.Log(
					audit.ActionLogin,
					user,
					"portal",
					"authenticate",
					"failure",
					"Basic Auth authentication failed",
					nil,
					r.RemoteAddr,
					r.UserAgent(),
				), "记录审计日志失败")
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="Cloud Flow Portal"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 记录审计日志
		if s.auditLogger != nil {
			safety.CheckAndWarn(s.logger, s.auditLogger.Log(
				audit.ActionLogin,
				userID,
				"portal",
				"authenticate",
				"success",
				"Basic Auth authentication successful",
				map[string]interface{}{"role": role},
				r.RemoteAddr,
				r.UserAgent(),
			), "记录审计日志失败")
		}

		// 将用户信息存入 context
		ctx := r.Context()
		ctx = context.WithValue(ctx, userContextKey, userID)
		ctx = context.WithValue(ctx, roleContextKey, role)
		next(w, r.WithContext(ctx))
	}
}

// validateJWT 验证 JWT 令牌
func (s *Server) validateJWT(tokenString string) (map[string]interface{}, error) {
	// 检查 JWT 管理器是否初始化
	if s.jwtManager == nil {
		return nil, fmt.Errorf("JWT manager not initialized")
	}

	// 使用 JWT 管理器验证令牌
	claims, err := s.jwtManager.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	// 将 claims 转换为 map[string]interface{}
	result := map[string]interface{}{
		"user_id": claims.UserID,
		"role":    claims.Role,
		"exp":     claims.ExpiresAt,
		"iat":     claims.IssuedAt,
		"sub":     claims.Subject,
	}

	return result, nil
}

// methodHandler 限制 HTTP 方法，不匹配时返回 405
func methodHandler(method string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}

// Handler 返回 HTTP handler
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 为所有 API 端点添加速率限制
	// 受保护的 API 端点（需要认证）
	mux.HandleFunc("/api/overview", s.rateLimitMiddleware(s.authMiddleware(s.handleOverview)))
	mux.HandleFunc("/api/nodes", s.rateLimitMiddleware(s.authMiddleware(s.handleNodes)))
	mux.HandleFunc("/api/metrics", s.rateLimitMiddleware(s.authMiddleware(s.handleMetrics)))
	mux.HandleFunc("/api/traces", s.rateLimitMiddleware(s.authMiddleware(s.handleTraces)))
	// /api/tracing 作为 /api/traces 的别名
	mux.HandleFunc("/api/tracing", s.rateLimitMiddleware(s.authMiddleware(s.handleTraces)))
	mux.HandleFunc("/api/traffic", s.rateLimitMiddleware(s.authMiddleware(s.handleTraffic)))
	mux.HandleFunc("/api/protocol", s.rateLimitMiddleware(s.authMiddleware(s.handleProtocol)))
	mux.HandleFunc("/api/cpu", s.rateLimitMiddleware(s.authMiddleware(s.handleCPU)))
	mux.HandleFunc("/api/memory", s.rateLimitMiddleware(s.authMiddleware(s.handleMemory)))
	mux.HandleFunc("/api/alerts", s.rateLimitMiddleware(s.authMiddleware(s.handleAlerts)))
	mux.HandleFunc("/api/rules", s.rateLimitMiddleware(s.authMiddleware(s.handleRules)))
	
	// 导出功能
	mux.HandleFunc("/api/export/metrics/json", s.rateLimitMiddleware(s.authMiddleware(s.handleExportMetricsJSON)))
	mux.HandleFunc("/api/export/metrics/csv", s.rateLimitMiddleware(s.authMiddleware(s.handleExportMetricsCSV)))
	mux.HandleFunc("/api/export/traces/json", s.rateLimitMiddleware(s.authMiddleware(s.handleExportTracesJSON)))
	mux.HandleFunc("/api/export/traces/csv", s.rateLimitMiddleware(s.authMiddleware(s.handleExportTracesCSV)))
	
	// 用户管理（需要 CSRF 保护）
	mux.HandleFunc("/api/users", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleListUsers))))
	mux.HandleFunc("/api/users/create", s.rateLimitMiddleware(s.authMiddleware(s.csrfMiddleware(methodHandler(http.MethodPost, s.handleCreateUser)))))
	mux.HandleFunc("/api/users/update", s.rateLimitMiddleware(s.authMiddleware(s.csrfMiddleware(methodHandler(http.MethodPut, s.handleUpdateUser)))))
	mux.HandleFunc("/api/users/delete", s.rateLimitMiddleware(s.authMiddleware(s.csrfMiddleware(methodHandler(http.MethodDelete, s.handleDeleteUser)))))

	// RESTful 风格别名（与 action 风格并存）
	mux.HandleFunc("/api/users/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// 根据 method 和路径分发
		path := strings.TrimPrefix(r.URL.Path, "/api/users/")
		if path == "" || path == "/" {
			switch r.Method {
			case http.MethodGet:
				s.handleListUsers(w, r)
			case http.MethodPost:
				s.handleCreateUser(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		} else {
			// /api/users/{id} - 从路径提取 ID，注入到 query 参数中
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/")
			// 将路径中的 ID 注入到 query 参数，供现有 handler 使用
			q := r.URL.Query()
			q.Set("id", id)
			r.URL.RawQuery = q.Encode()
			switch r.Method {
			case http.MethodPut:
				s.handleUpdateUser(w, r)
			case http.MethodDelete:
				s.handleDeleteUser(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}
	})))
	mux.HandleFunc("/api/users/login", s.rateLimitMiddleware(methodHandler(http.MethodPost, s.handleLogin)))
	mux.HandleFunc("/api/users/verify", s.rateLimitMiddleware(methodHandler(http.MethodGet, s.handleVerify)))

	// CSRF token 获取端点
	mux.HandleFunc("/api/csrf-token", s.csrfMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// csrfMiddleware 的 GET 分支会自动设置 csrf_token cookie
		s.writeJSONWithStatus(w, r, http.StatusOK, map[string]interface{}{"success": true})
	}))
	
	// 用户信息
	mux.HandleFunc("/api/users/info", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleUserInfo(w, r)
		case http.MethodPut:
			s.csrfMiddleware(s.handleUpdateUserInfo)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.HandleFunc("/api/users/password", s.rateLimitMiddleware(s.authMiddleware(s.csrfMiddleware(methodHandler(http.MethodPut, s.handleChangePassword)))))
	
	// 指标高级查询
	mux.HandleFunc("/api/metrics/list", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleMetricList))))
	mux.HandleFunc("/api/metrics/trend", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleMetricTrend))))
	mux.HandleFunc("/api/metrics/aggregation", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleMetricAggregation))))
	
	// 告警管理
	mux.HandleFunc("/api/alert/list", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleAlertList))))
	mux.HandleFunc("/api/alert/rules", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleAlertRules(w, r)
		case http.MethodPost:
			s.csrfMiddleware(s.handleCreateAlertRule)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.HandleFunc("/api/alert/rules/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			s.csrfMiddleware(s.handleUpdateAlertRule)(w, r)
		case http.MethodDelete:
			s.csrfMiddleware(s.handleDeleteAlertRule)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.HandleFunc("/api/alert/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/handle") {
			s.csrfMiddleware(methodHandler(http.MethodPut, s.handleAlertHandle))(w, r)
		} else {
			methodHandler(http.MethodGet, s.handleAlertDetail)(w, r)
		}
	})))
	
	// 资产管理
	mux.HandleFunc("/api/asset/change-events", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleAssetChangeEvents))))
	mux.HandleFunc("/api/asset/resource-pools", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleResourcePools))))
	mux.HandleFunc("/api/asset/cloud-platforms", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleCloudPlatforms))))
	mux.HandleFunc("/api/asset/regions", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleRegions))))
	mux.HandleFunc("/api/asset/availability-zones", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleAvailabilityZones))))
	mux.HandleFunc("/api/asset/servers", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleServers))))
	mux.HandleFunc("/api/asset/hosts", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleHosts))))
	mux.HandleFunc("/api/asset/vpcs", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleVpcs))))
	mux.HandleFunc("/api/asset/subnets", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleSubnets))))
	mux.HandleFunc("/api/asset/routers", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleRouters))))
	mux.HandleFunc("/api/asset/dhcp-servers", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleDhcpServers))))
	mux.HandleFunc("/api/asset/ip-addresses", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleIpAddresses))))
	
	// 网络分析（/api/tracing 已在上方作为 /api/traces 别名注册）
	mux.HandleFunc("/api/topology", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleTopology))))
	mux.HandleFunc("/api/network/resource-analysis", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleNetworkResourceAnalysis))))
	mux.HandleFunc("/api/network/path-analysis", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleNetworkPathAnalysis))))
	mux.HandleFunc("/api/network/topology-analysis", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleNetworkTopologyAnalysis))))
	mux.HandleFunc("/api/network/flow-logs", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleFlowLogs))))
	
	// 业务观测 - Business CRUD
	mux.HandleFunc("/api/business", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleBusinessList(w, r)
		case http.MethodPost:
			s.csrfMiddleware(s.handleBusinessCreate)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.HandleFunc("/api/business/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleBusinessDetail(w, r)
		case http.MethodPut:
			s.csrfMiddleware(s.handleBusinessUpdate)(w, r)
		case http.MethodDelete:
			s.csrfMiddleware(s.handleBusinessDelete)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	
	// 业务观测 - Service CRUD
	mux.HandleFunc("/api/service", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleServiceList(w, r)
		case http.MethodPost:
			s.csrfMiddleware(s.handleServiceCreate)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.HandleFunc("/api/service/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleServiceDetail(w, r)
		case http.MethodPut:
			s.csrfMiddleware(s.handleServiceUpdate)(w, r)
		case http.MethodDelete:
			s.csrfMiddleware(s.handleServiceDelete)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	
	// 系统管理 - Collectors CRUD
	mux.HandleFunc("/api/system/collectors", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleCollectorList(w, r)
		case http.MethodPost:
			s.csrfMiddleware(s.handleCollectorCreate)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.HandleFunc("/api/system/collectors/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleCollectorDetail(w, r)
		case http.MethodPut:
			s.csrfMiddleware(s.handleCollectorUpdate)(w, r)
		case http.MethodDelete:
			s.csrfMiddleware(s.handleCollectorDelete)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	
	// 配置管理
	mux.HandleFunc("/api/system/config/reload", s.rateLimitMiddleware(s.authMiddleware(s.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		s.handleConfigReload(w, r)
	}))))
	
	// 系统管理 - Data Nodes CRUD
	mux.HandleFunc("/api/system/data-nodes", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleDataNodeList(w, r)
		case http.MethodPost:
			s.csrfMiddleware(s.handleDataNodeCreate)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.HandleFunc("/api/system/data-nodes/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleDataNodeDetail(w, r)
		case http.MethodPut:
			s.csrfMiddleware(s.handleDataNodeUpdate)(w, r)
		case http.MethodDelete:
			s.csrfMiddleware(s.handleDataNodeDelete)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	
	// 系统管理 - Config
	mux.HandleFunc("/api/system/config", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleGetSystemConfig(w, r)
		case http.MethodPut:
			s.csrfMiddleware(s.handleUpdateSystemConfig)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	
	// 系统管理 - Logs
	mux.HandleFunc("/api/system/logs", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleSystemLogs))))
	
	// 报表管理
	mux.HandleFunc("/api/report/list", s.rateLimitMiddleware(s.authMiddleware(methodHandler(http.MethodGet, s.handleReportList))))
	mux.HandleFunc("/api/report/generate", s.rateLimitMiddleware(s.authMiddleware(s.csrfMiddleware(methodHandler(http.MethodPost, s.handleReportGenerate)))))
	mux.HandleFunc("/api/report/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			methodHandler(http.MethodGet, s.handleReportDownload)(w, r)
		} else {
			methodHandler(http.MethodGet, s.handleReportDetail)(w, r)
		}
	})))
	
	// Swagger 文档（带认证）
	RegisterSwaggerRoutes(mux, s.authMiddleware)
	
	mux.HandleFunc("/api/healthz", s.rateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))

	// Alertmanager webhook 端点（无需认证，由 Alertmanager 调用）
	mux.HandleFunc("/api/v1/alerts/webhook", s.handleAlertmanagerWebhook)

	// 静态文件（带认证）
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		s.logger.Errorf("创建静态文件子目录失败: %v", err)
		mux.HandleFunc("/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		})))
	} else {
		mux.HandleFunc("/", s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			http.FileServer(http.FS(sub)).ServeHTTP(w, r)
		})))
	}

	// 使用全局请求体大小限制中间件包装 mux
	return s.maxBytesMiddleware(mux)
}

// handleAlertmanagerWebhook 接收 Alertmanager 发送的告警通知
func (s *Server) handleAlertmanagerWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload []struct {
		Status   string            `json:"status"`
		Labels   map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		StartsAt time.Time         `json:"startsAt"`
		EndsAt   time.Time         `json:"endsAt"`
		GeneratorURL string        `json:"generatorURL"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.logger.Warnf("Alertmanager webhook 解析失败: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	for _, alert := range payload {
		s.logger.Infof("收到告警: status=%s alertname=%s instance=%s",
			alert.Status,
			alert.Labels["alertname"],
			alert.Labels["instance"])
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	overview, err := s.store.GetOverview()
	if err != nil {
		s.logger.Errorf("获取概览数据失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
		return
	}
	if overview == nil {
		overview = map[string]interface{}{}
	}
	s.writeJSON(w, r, overview)
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	nodes, err := s.store.GetNodes()
	if err != nil {
		s.logger.Errorf("获取节点数据失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
		return
	}
	if nodes == nil {
		nodes = []map[string]interface{}{}
	}
	s.writeJSON(w, r, nodes)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")

	// 支持分页
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	// 设置上限，防止内存溢出
	if limit > 10000 {
		limit = 10000
	}

	results, err := s.store.QueryMetrics(day, probeID, limit)
	if err != nil {
		s.logger.Errorf("查询指标数据失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
		return
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	s.writeJSON(w, r, results)
}

func (s *Server) handleTraces(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")

	// 支持分页
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	// 设置上限，防止内存溢出
	if limit > 10000 {
		limit = 10000
	}

	results, err := s.store.QueryTraces(day, probeID, limit)
	if err != nil {
		s.logger.Errorf("查询链路追踪数据失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
		return
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	s.writeJSON(w, r, results)
}

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	// 获取查询参数
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")
	metricType := r.URL.Query().Get("type")

	// 从存储引擎中查询流量数据
	metrics, err := s.store.QueryMetrics(day, probeID, 1000)
	if err != nil {
		s.logger.Errorf("查询流量数据失败: %v", err)
		// 失败时返回空数据
		data := map[string]interface{}{
			"labels":   []string{},
			"inbound":  []int64{},
			"outbound": []int64{},
		}
		s.writeJSON(w, r, data)
		return
	}

	// 按 type 过滤数据（如果指定了 type 参数）
	if metricType != "" {
		filtered := make([]map[string]interface{}, 0, len(metrics))
		for _, metric := range metrics {
			if t, ok := metric["protocol"].(string); ok && t == metricType {
				filtered = append(filtered, metric)
			}
		}
		metrics = filtered
	}

	// 按小时聚合数据
	hourlyData := make(map[int]map[string]int64)
	for _, metric := range metrics {
		// 解析时间戳，兼容 int64 和 float64 类型
		var ts int64
		switch v := metric["timestamp"].(type) {
		case int64:
			ts = v
		case float64:
			ts = int64(v)
		case int:
			ts = int64(v)
		default:
			s.logger.Warnf("跳过未知类型的字段 timestamp: 值类型为 %T", metric["timestamp"])
			continue
		}
		t := time.Unix(ts, 0)
		hour := t.Hour()

		if _, ok := hourlyData[hour]; !ok {
			hourlyData[hour] = map[string]int64{"inbound": 0, "outbound": 0}
		}

		// 根据 IP 地址判断 inbound/outbound
		// LIMITATION: 当前流量方向判断逻辑基于简化的启发式规则，存在以下局限性：
		// 1. 仅通过 localhost/127.0.0.1 判断本地地址，未考虑本机其他 IP（如内网 IP、容器 IP）
		// 2. 外部流量方向判断依赖硬编码的常见服务器端口列表，无法覆盖自定义端口
		// 3. 无法判断的流量默认各分配一半，可能导致统计偏差
		// 4. 在 NAT/代理/负载均衡场景下，src_ip/dst_ip 可能不是真实通信端点
		// 如需更精确的流量方向判断，建议在探针侧添加流量方向标记字段
		if bytes, ok := metric["bytes"]; ok {
			var bytesVal int64
			switch v := bytes.(type) {
			case int64:
				bytesVal = v
			case float64:
				bytesVal = int64(v)
			case int:
				bytesVal = int64(v)
			default:
				s.logger.Warnf("跳过未知类型的字段 bytes: 值类型为 %T", metric["bytes"])
				continue
			}
			// 尝试从 src_ip 和 dst_ip 字段判断流量方向
			// 简单规则：如果 src_ip 是 localhost 或 127.0.0.1，则认为是 outbound
			// 如果 dst_ip 是 localhost 或 127.0.0.1，则认为是 inbound
			// 否则，根据协议和端口进行简单判断
			srcIP := ""
			dstIP := ""
			if ip, ok := metric["src_ip"].(string); ok {
				srcIP = ip
			}
			if ip, ok := metric["dst_ip"].(string); ok {
				dstIP = ip
			}

			// 检查是否为本地流量
			isLocalSrc := isPrivateIP(srcIP)
			isLocalDst := isPrivateIP(dstIP)

			if isLocalSrc && !isLocalDst {
				// 本地发送到外部，是 outbound
				hourlyData[hour]["outbound"] += bytesVal
			} else if !isLocalSrc && isLocalDst {
				// 外部发送到本地，是 inbound
				hourlyData[hour]["inbound"] += bytesVal
			} else if isLocalSrc && isLocalDst {
				// 本地内部流量，各分配一半
				hourlyData[hour]["inbound"] += bytesVal / 2
				hourlyData[hour]["outbound"] += bytesVal / 2
			} else {
				// 外部之间的流量，根据端口判断
				srcPort := 0
				dstPort := 0
				if port, ok := metric["src_port"].(int64); ok {
					srcPort = int(port)
				}
				if port, ok := metric["dst_port"].(int64); ok {
					dstPort = int(port)
				}

				// 常见的服务器端口，认为是 inbound
				commonServerPorts := map[int]bool{80: true, 443: true, 22: true, 3306: true, 5432: true, 6379: true, 8080: true, 8443: true, 27017: true, 11211: true, 9092: true, 1521: true, 1433: true}
				if commonServerPorts[dstPort] {
					hourlyData[hour]["inbound"] += bytesVal
				} else if commonServerPorts[srcPort] {
					hourlyData[hour]["outbound"] += bytesVal
				} else {
					// 无法判断，根据端口号范围启发式判断：
					// 高端口（>1024）通常是客户端临时端口，低端口（<=1024）通常是服务端口
					if dstPort <= 1024 && dstPort > 0 {
						hourlyData[hour]["inbound"] += bytesVal
					} else if srcPort <= 1024 && srcPort > 0 {
						hourlyData[hour]["outbound"] += bytesVal
					} else {
						// 仍无法判断，各分配一半
						hourlyData[hour]["inbound"] += bytesVal / 2
						hourlyData[hour]["outbound"] += bytesVal / 2
					}
				}
			}
		}
	}

	// 构建返回数据
	labels := []string{}
	inbound := []int64{}
	outbound := []int64{}

	for i := 0; i < 24; i += 3 { // 每 3 小时一个点
		labels = append(labels, fmt.Sprintf("%02d:00", i))
		hourData, ok := hourlyData[i]
		if ok {
			inbound = append(inbound, hourData["inbound"])
			outbound = append(outbound, hourData["outbound"])
		} else {
			inbound = append(inbound, 0)
			outbound = append(outbound, 0)
		}
	}

	data := map[string]interface{}{
		"labels":   labels,
		"inbound":  inbound,
		"outbound": outbound,
	}
	s.writeJSON(w, r, data)
}

func (s *Server) handleProtocol(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	// 获取查询参数
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")
	metricType := r.URL.Query().Get("type")

	// 从存储引擎中查询流量数据
	metrics, err := s.store.QueryMetrics(day, probeID, 1000)
	if err != nil {
		s.logger.Errorf("查询协议数据失败: %v", err)
		// 失败时返回空数据
		data := map[string]interface{}{
			"data": []map[string]interface{}{},
		}
		s.writeJSON(w, r, data)
		return
	}

	// 按 type 过滤数据（如果指定了 type 参数）
	if metricType != "" {
		filtered := make([]map[string]interface{}, 0, len(metrics))
		for _, metric := range metrics {
			if t, ok := metric["protocol"].(string); ok && t == metricType {
				filtered = append(filtered, metric)
			}
		}
		metrics = filtered
	}

	// 统计协议分布
	protocolCount := make(map[string]int64)
	for _, metric := range metrics {
		if protocol, ok := metric["protocol"].(string); ok {
			protocolCount[protocol]++
		}
	}

	// 构建返回数据
	protocolData := []map[string]interface{}{}
	for protocol, count := range protocolCount {
		protocolData = append(protocolData, map[string]interface{}{
			"name":  protocol,
			"value": count,
		})
	}

	data := map[string]interface{}{
		"data": protocolData,
	}
	s.writeJSON(w, r, data)
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	case uint:
		return float64(val)
	case uint64:
		return float64(val)
	default:
		return 0
	}
}

func (s *Server) handleCPU(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	// 获取查询参数
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")

	// 从存储引擎中查询流量数据
	metrics, err := s.store.QueryMetrics(day, probeID, 1000)
	if err != nil {
		s.logger.Errorf("查询CPU数据失败: %v", err)
		// 失败时返回空数据
		data := map[string]interface{}{
			"labels": []string{},
			"values": []float64{},
		}
		s.writeJSON(w, r, data)
		return
	}

	// 按小时聚合数据
	hourlyData := make(map[int]float64)
	hourlyCount := make(map[int]int)
	
	for _, metric := range metrics {
		// 解析时间戳，兼容 int64/float64/int 类型
		var ts int64
		switch v := metric["timestamp"].(type) {
		case int64:
			ts = v
		case float64:
			ts = int64(v)
		case int:
			ts = int64(v)
		default:
			s.logger.Warnf("跳过未知类型的字段 timestamp: 值类型为 %T", metric["timestamp"])
			continue
		}
		t := time.Unix(ts, 0)
		hour := t.Hour()
		
		// 检查是否为 CPU 指标（通过 cpu_usage 字段判断）
		if cpuUsage, ok := metric["cpu_usage"]; ok {
			cpu := toFloat64(cpuUsage)
			if cpu > 0 {
				hourlyData[hour] += cpu
				hourlyCount[hour]++
			}
		}
	}

	// 构建返回数据
	labels := []string{}
	values := []float64{}
	
	for i := 0; i < 24; i += 3 { // 每 3 小时一个点
		labels = append(labels, fmt.Sprintf("%02d:00", i))
		if count := hourlyCount[i]; count > 0 {
			avgCPU := hourlyData[i] / float64(count)
			values = append(values, avgCPU)
		} else {
			values = append(values, 0)
		}
	}

	data := map[string]interface{}{
		"labels": labels,
		"values": values,
	}
	s.writeJSON(w, r, data)
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	// 获取查询参数
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")

	// 从存储引擎中查询流量数据
	metrics, err := s.store.QueryMetrics(day, probeID, 1000)
	if err != nil {
		s.logger.Errorf("查询内存数据失败: %v", err)
		// 失败时返回空数据
		data := map[string]interface{}{
			"labels": []string{},
			"values": []float64{},
		}
		s.writeJSON(w, r, data)
		return
	}

	// 按小时聚合数据
	hourlyData := make(map[int]float64)
	hourlyCount := make(map[int]int)
	
	for _, metric := range metrics {
		// 解析时间戳，兼容 int64/float64/int 类型
		var ts int64
		switch v := metric["timestamp"].(type) {
		case int64:
			ts = v
		case float64:
			ts = int64(v)
		case int:
			ts = int64(v)
		default:
			s.logger.Warnf("跳过未知类型的字段 timestamp: 值类型为 %T", metric["timestamp"])
			continue
		}
		t := time.Unix(ts, 0)
		hour := t.Hour()
		
		// 检查是否为内存指标（通过 memory_usage 字段判断）
		if memUsage, ok := metric["memory_usage"]; ok {
			mem := toFloat64(memUsage)
			if mem > 0 {
				hourlyData[hour] += mem
				hourlyCount[hour]++
			}
		}
	}

	// 构建返回数据
	labels := []string{}
	values := []float64{}
	
	for i := 0; i < 24; i += 3 { // 每 3 小时一个点
		labels = append(labels, fmt.Sprintf("%02d:00", i))
		if count := hourlyCount[i]; count > 0 {
			avgMemory := hourlyData[i] / float64(count)
			values = append(values, avgMemory)
		} else {
			values = append(values, 0)
		}
	}

	data := map[string]interface{}{
		"labels": labels,
		"values": values,
	}
	s.writeJSON(w, r, data)
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if s.alertManager == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "告警服务不可用"})
		return
	}
	alerts := s.alertManager.GetActiveAlerts()
	s.writeJSON(w, r, alerts)
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	if s.alertManager == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "告警服务不可用"})
		return
	}
	rules := s.alertManager.GetRuleManager().GetRules()
	s.writeJSON(w, r, rules)
}

// isPrivateIP 判断 IP 是否为私有/内网地址
func isPrivateIP(ip string) bool {
	if ip == "localhost" || ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	// 解析 IP
	netIP := net.ParseIP(ip)
	if netIP == nil {
		return false
	}
	// RFC 1918 私有地址
	privateRanges := []struct {
		network *net.IPNet
	}{
		{mustParseCIDR("10.0.0.0/8")},
		{mustParseCIDR("172.16.0.0/12")},
		{mustParseCIDR("192.168.0.0/16")},
		{mustParseCIDR("169.254.0.0/16")}, // 链路本地
		{mustParseCIDR("fc00::/7")},        // IPv6 ULA
	}
	for _, r := range privateRanges {
		if r.network.Contains(netIP) {
			return true
		}
	}
	return false
}

func mustParseCIDR(s string) *net.IPNet {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return network
}

func today() string {
	return time.Now().Format("2006-01-02")
}

func (s *Server) handleExportMetricsJSON(w http.ResponseWriter, r *http.Request) {
	// 导出功能需要 editor 或 admin 角色
	role, _ := r.Context().Value(roleContextKey).(string)
	if role != "admin" && role != "editor" {
		s.writeJSONWithStatus(w, r, http.StatusForbidden, map[string]interface{}{"error": "Permission denied: editor or admin role required"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")
	
	// 导出时可以设置更大的限制
	limit := 10000
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	// 设置上限，防止内存溢出
	if limit > 100000 {
		limit = 100000
	}
	
	results, err := s.store.QueryMetrics(day, probeID, limit)
	if err != nil {
		s.logger.Errorf("导出指标数据查询失败: %v", err)
		s.setExportHeaders(w, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "Error querying metrics"})
		return
	}
	if results == nil {
		results = []map[string]interface{}{}
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=metrics_%s.json", sanitizeFilename(day)))

	// NOTE: 导出 handler 的 CORS 设置是独立的，不通过 writeJSON 统一设置，
	// 因为导出接口的 Content-Type 为 application/json（附件下载），
	// 与普通 API 的 Content-Type 不同，且浏览器对下载请求的 CORS 行为有差异。
	// 统一 CORS 配置：使用服务实例的 allowedOrigins 配置
	s.setExportHeaders(w, r)
	
	if err := json.NewEncoder(w).Encode(results); err != nil {
		s.logger.Errorf("JSON Encode 失败: %v", err)
	}
}

func (s *Server) handleExportMetricsCSV(w http.ResponseWriter, r *http.Request) {
	// 导出功能需要 editor 或 admin 角色
	role, _ := r.Context().Value(roleContextKey).(string)
	if role != "admin" && role != "editor" {
		s.writeJSONWithStatus(w, r, http.StatusForbidden, map[string]interface{}{"error": "Permission denied: editor or admin role required"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")
	
	// 导出时可以设置更大的限制
	limit := 10000
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	// 设置上限，防止内存溢出
	if limit > 100000 {
		limit = 100000
	}
	
	results, err := s.store.QueryMetrics(day, probeID, limit)
	if err != nil {
		s.setExportHeaders(w, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "Error querying metrics"})
		return
	}
	if results == nil {
		results = []map[string]interface{}{}
	}

	// 设置响应头
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=metrics_%s.csv", sanitizeFilename(day)))
	
	// 创建 CSV writer
	writer := csv.NewWriter(w)
	defer writer.Flush()
	
	// 写入表头
	headers := []string{"ID", "ProbeID", "Timestamp", "SrcIP", "DstIP", "SrcPort", "DstPort", "Protocol", "Bytes", "Packets", "Latency"}
	writer.Write(headers)
	
	// 写入数据
	for _, metric := range results {
		row := []string{
			fmt.Sprintf("%v", metric["id"]),
			fmt.Sprintf("%v", metric["probe_id"]),
			fmt.Sprintf("%v", metric["timestamp"]),
			fmt.Sprintf("%v", metric["src_ip"]),
			fmt.Sprintf("%v", metric["dst_ip"]),
			fmt.Sprintf("%v", metric["src_port"]),
			fmt.Sprintf("%v", metric["dst_port"]),
			fmt.Sprintf("%v", metric["protocol"]),
			fmt.Sprintf("%v", metric["bytes"]),
			fmt.Sprintf("%v", metric["packets"]),
			fmt.Sprintf("%v", metric["latency"]),
		}
		writer.Write(row)
	}
}

func (s *Server) handleExportTracesJSON(w http.ResponseWriter, r *http.Request) {
	// 导出功能需要 editor 或 admin 角色
	role, _ := r.Context().Value(roleContextKey).(string)
	if role != "admin" && role != "editor" {
		s.writeJSONWithStatus(w, r, http.StatusForbidden, map[string]interface{}{"error": "Permission denied: editor or admin role required"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")
	
	// 导出时可以设置更大的限制
	limit := 10000
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	// 设置上限，防止内存溢出
	if limit > 100000 {
		limit = 100000
	}
	
	results, err := s.store.QueryTraces(day, probeID, limit)
	if err != nil {
		s.logger.Errorf("导出链路追踪数据查询失败: %v", err)
		s.setExportHeaders(w, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "Error querying traces"})
		return
	}
	if results == nil {
		results = []map[string]interface{}{}
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=traces_%s.json", sanitizeFilename(day)))
	
	// 统一 CORS 配置：使用服务实例的 allowedOrigins 配置
	s.setExportHeaders(w, r)
	
	if err := json.NewEncoder(w).Encode(results); err != nil {
		s.logger.Errorf("JSON Encode 失败: %v", err)
	}
}

func (s *Server) handleExportTracesCSV(w http.ResponseWriter, r *http.Request) {
	// 导出功能需要 editor 或 admin 角色
	role, _ := r.Context().Value(roleContextKey).(string)
	if role != "admin" && role != "editor" {
		s.writeJSONWithStatus(w, r, http.StatusForbidden, map[string]interface{}{"error": "Permission denied: editor or admin role required"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")
	
	// 导出时可以设置更大的限制
	limit := 10000
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	// 设置上限，防止内存溢出
	if limit > 100000 {
		limit = 100000
	}
	
	results, err := s.store.QueryTraces(day, probeID, limit)
	if err != nil {
		s.setExportHeaders(w, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "Error querying traces"})
		return
	}
	if results == nil {
		results = []map[string]interface{}{}
	}

	// 设置响应头
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=traces_%s.csv", sanitizeFilename(day)))
	
	// 创建 CSV writer
	writer := csv.NewWriter(w)
	defer writer.Flush()
	
	// 写入表头
	headers := []string{"ID", "ProbeID", "Timestamp", "TraceID", "SpanID", "ParentID", "Operation", "Duration", "Service", "Tags"}
	writer.Write(headers)
	
	// 写入数据
	for _, trace := range results {
		row := []string{
			fmt.Sprintf("%v", trace["id"]),
			fmt.Sprintf("%v", trace["probe_id"]),
			fmt.Sprintf("%v", trace["timestamp"]),
			fmt.Sprintf("%v", trace["trace_id"]),
			fmt.Sprintf("%v", trace["span_id"]),
			fmt.Sprintf("%v", trace["parent_id"]),
			fmt.Sprintf("%v", trace["operation"]),
			fmt.Sprintf("%v", trace["duration"]),
			fmt.Sprintf("%v", trace["service"]),
			fmt.Sprintf("%v", trace["tags"]),
		}
		writer.Write(row)
	}
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	// 检查 userID 是否存在于 context 中
	userID, ok := r.Context().Value(userContextKey).(string)
	if !ok || userID == "" {
		s.logger.Warnf("用户身份信息缺失，拒绝访问用户列表")
		s.writeJSONWithStatus(w, r, http.StatusUnauthorized, map[string]interface{}{"error": "Unauthorized: user identity missing"})
		return
	}

	// 检查用户权限：只有 admin 角色可以访问用户列表
	role, ok := r.Context().Value(roleContextKey).(string)
	if !ok || role != "admin" {
		s.logger.Warnf("非管理员用户尝试访问用户列表")
		s.writeJSONWithStatus(w, r, http.StatusForbidden, map[string]interface{}{"error": "Permission denied: admin role required"})
		return
	}

	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}

	users, err := s.store.ListUsers()
	if err != nil {
		s.logger.Errorf("获取用户列表失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
		return
	}
	if users == nil {
		users = []map[string]interface{}{}
	}
	s.writeJSON(w, r, users)
}

// validateUsername 验证用户名格式：字母数字下划线，3-32字符
func validateUsername(username string) (string, bool) {
	if len(username) < 3 || len(username) > 32 {
		return "Username must be 3-32 characters", false
	}
	for _, c := range username {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return "Username can only contain letters, numbers and underscore", false
		}
	}
	return "", true
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	// 检查用户权限：只有 admin 角色可以创建用户
	role, ok := r.Context().Value(roleContextKey).(string)
	if !ok || role != "admin" {
		s.logger.Warnf("非管理员用户尝试创建用户")
		s.writeJSONWithStatus(w, r, http.StatusForbidden, map[string]interface{}{"error": "Permission denied: admin role required"})
		return
	}

	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}

	var userData struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&userData); err != nil {
		if err == errTooLarge {
			s.logger.Warnf("请求体超过大小限制: %v", err)
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
		return
	}

	if userData.Username == "" || userData.Password == "" || userData.Role == "" {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Username, password and role are required"})
		return
	}

	// 用户名强度验证
	if errMsg, valid := validateUsername(userData.Username); !valid {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": errMsg})
		return
	}

	// 密码强度验证：至少8字符，包含大小写字母和数字
	if len(userData.Password) < 8 {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Password must be at least 8 characters"})
		return
	}
	var hasUpper, hasLower, hasDigit bool
	for _, c := range userData.Password {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c >= 'a' && c <= 'z' {
			hasLower = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Password must contain uppercase, lowercase and digit"})
		return
	}

	// 角色白名单验证
	allowedRoles := map[string]bool{"admin": true, "viewer": true, "editor": true}
	if !allowedRoles[userData.Role] {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Invalid role. Allowed roles: admin, viewer, editor"})
		return
	}

	// 检查用户是否已存在
	existingUser, err := s.store.GetUser(userData.Username)
	if err != nil {
		s.logger.Errorf("检查用户是否存在失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
		return
	}
	if existingUser != nil {
		s.writeJSONWithStatus(w, r, http.StatusConflict, map[string]interface{}{"error": "User already exists"})
		return
	}

	err = s.store.CreateUser(userData.Username, userData.Password, userData.Role)
	if err != nil {
		s.logger.Errorf("创建用户失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
		return
	}

	s.writeJSON(w, r, map[string]interface{}{"success": true, "message": "User created successfully"})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	// 检查用户权限：只有 admin 角色可以更新用户
	role, ok := r.Context().Value(roleContextKey).(string)
	if !ok || role != "admin" {
		s.logger.Warnf("非管理员用户尝试更新用户")
		s.writeJSONWithStatus(w, r, http.StatusForbidden, map[string]interface{}{"error": "Permission denied: admin role required"})
		return
	}

	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}

	var userData struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&userData); err != nil {
		if err == errTooLarge {
			s.logger.Warnf("更新用户请求体超过大小限制: %v", err)
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
		return
	}

	if userData.Username == "" || userData.Role == "" {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Username and role are required"})
		return
	}

	// 角色白名单验证
	allowedRoles := map[string]bool{"admin": true, "viewer": true, "editor": true}
	if !allowedRoles[userData.Role] {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Invalid role. Allowed roles: admin, viewer, editor"})
		return
	}

	// 空密码时跳过密码更新，只更新角色
	if userData.Password == "" {
		err := s.store.UpdateUserRole(userData.Username, userData.Role)
		if err != nil {
			s.logger.Errorf("更新用户角色失败: %v", err)
			s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
			return
		}
		s.writeJSON(w, r, map[string]interface{}{"success": true, "message": "User role updated successfully"})
		return
	}

	// 密码强度验证：至少8字符，包含大小写字母和数字（与 handleCreateUser 一致）
	if len(userData.Password) < 8 {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Password must be at least 8 characters"})
		return
	}
	var hasUpper, hasLower, hasDigit bool
	for _, c := range userData.Password {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c >= 'a' && c <= 'z' {
			hasLower = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Password must contain uppercase, lowercase and digit"})
		return
	}

	err := s.store.UpdateUser(userData.Username, userData.Password, userData.Role)
	if err != nil {
		s.logger.Errorf("更新用户失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
		return
	}

	s.writeJSON(w, r, map[string]interface{}{"success": true, "message": "User updated successfully"})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	// 检查用户权限：只有 admin 角色可以删除用户
	role, ok := r.Context().Value(roleContextKey).(string)
	if !ok || role != "admin" {
		s.logger.Warnf("非管理员用户尝试删除用户")
		s.writeJSONWithStatus(w, r, http.StatusForbidden, map[string]interface{}{"error": "Permission denied: admin role required"})
		return
	}

	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}

	var userData struct {
		Username string `json:"username"`
	}

	if err := json.NewDecoder(r.Body).Decode(&userData); err != nil {
		if err == errTooLarge {
			s.logger.Warnf("删除用户请求体超过大小限制: %v", err)
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
		return
	}

	if userData.Username == "" {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Username is required"})
		return
	}

	err := s.store.DeleteUser(userData.Username)
	if err != nil {
		s.logger.Errorf("删除用户失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "internal server error"})
		return
	}

	s.writeJSON(w, r, map[string]interface{}{"success": true, "message": "User deleted successfully"})
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	// 检查 JWT 令牌
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		s.writeJSONWithStatus(w, r, http.StatusUnauthorized, map[string]interface{}{"success": false, "error": "Missing or invalid Authorization header"})
		return
	}

	tokenString := authHeader[7:]
	claims, err := s.validateJWT(tokenString)
	if err != nil || claims == nil {
		s.writeJSONWithStatus(w, r, http.StatusUnauthorized, map[string]interface{}{"success": false, "error": "Invalid or expired token"})
		return
	}

	// 验证 user_id 字段
	_, ok := claims["user_id"].(string)
	if !ok {
		s.writeJSONWithStatus(w, r, http.StatusUnauthorized, map[string]interface{}{"success": false, "error": "Invalid token claims"})
		return
	}

	s.writeJSON(w, r, map[string]interface{}{"success": true, "message": "Token is valid"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// 登录端点豁免 CSRF，但必须验证 Content-Type
	if r.Header.Get("Content-Type") != "application/json" {
		s.writeJSONWithStatus(w, r, http.StatusUnsupportedMediaType, map[string]interface{}{"success": false, "message": "仅支持 application/json"})
		return
	}

	var loginData struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {
		if err == errTooLarge {
			s.logger.Warnf("登录请求体超过大小限制: %v", err)
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
		return
	}

	// 检查登录失败次数，防止暴力破解
	// 优先使用 Redis 存储
	if s.redisStore != nil {
		failCount, err := s.redisStore.GetLoginFailureCount(loginData.Username)
		if err != nil {
			s.logger.Warnf("Redis 获取登录失败计数失败: %v", err)
		} else if failCount >= 5 {
			s.writeJSONWithStatus(w, r, http.StatusTooManyRequests, map[string]interface{}{"error": "Login temporarily blocked due to too many failed attempts. Please try again later."})
			return
		}
	} else {
		// 回退到内存存储：在同一个锁区间内完成检查，避免竞态条件。
		// 先检查是否已锁定，不释放锁；验证完成后再决定是否增加计数。
		s.loginFailuresMutex.Lock()
		// 以约 1% 的概率执行全量清理过期记录
		if time.Now().UnixNano()%100 == 0 {
			cutoff := time.Now().Add(-30 * time.Minute)
			for user, info := range s.loginFailures {
				if info.lastFailure.Before(cutoff) {
					delete(s.loginFailures, user)
				}
			}
		}
		failureInfo, exists := s.loginFailures[loginData.Username]
		if exists && time.Since(failureInfo.lastFailure) > 30*time.Minute {
			delete(s.loginFailures, loginData.Username)
			exists = false
		}
		if exists && failureInfo.count >= 5 {
			s.loginFailuresMutex.Unlock()
			s.writeJSONWithStatus(w, r, http.StatusTooManyRequests, map[string]interface{}{"error": "Login temporarily blocked due to too many failed attempts. Please try again later."})
			return
		}
		// 记录当前检查时的失败计数，用于后续 CAS 式更新
		checkCount := 0
		if exists {
			checkCount = failureInfo.count
		}
		s.loginFailuresMutex.Unlock()
	}

	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"error": "存储服务不可用"})
		return
	}

	var valid bool
	var role string
	var err error

	valid, role, err = s.store.VerifyUser(loginData.Username, loginData.Password)
	
	if err != nil || !valid {
		// 增加登录失败计数
		if s.redisStore != nil {
			if _, err := s.redisStore.IncrLoginFailure(loginData.Username); err != nil {
				s.logger.Warnf("Redis 增加登录失败计数失败: %v", err)
			}
		} else {
			// CAS 式更新：只在计数未变化时才更新，防止竞态条件
			s.loginFailuresMutex.Lock()
			info, ok := s.loginFailures[loginData.Username]
			if ok && time.Since(info.lastFailure) <= 30*time.Minute && info.count == checkCount {
				info.count++
				info.lastFailure = time.Now()
			} else if !ok || time.Since(info.lastFailure) > 30*time.Minute {
				info = loginFailureInfo{count: 1, lastFailure: time.Now()}
			}
			s.loginFailures[loginData.Username] = info
			s.loginFailuresMutex.Unlock()
		}

		// 记录审计日志
		if s.auditLogger != nil {
			_ = s.auditLogger.Log(
				audit.ActionLogin,
				loginData.Username,
				"portal",
				"login",
				"failure",
				"Login failed: invalid username or password",
				nil,
				r.RemoteAddr,
				r.UserAgent(),
			)
		}

		s.writeJSONWithStatus(w, r, http.StatusUnauthorized, map[string]interface{}{"error": "Invalid username or password"})
		return
	}

	// 登录成功，清除失败记录
	if s.redisStore != nil {
		if err := s.redisStore.ResetLoginFailure(loginData.Username); err != nil {
			s.logger.Warnf("Redis 重置登录失败计数失败: %v", err)
		}
	} else {
		s.loginFailuresMutex.Lock()
		delete(s.loginFailures, loginData.Username)
		s.loginFailuresMutex.Unlock()
	}

	// 生成 JWT 令牌
	token, err := s.jwtManager.GenerateToken(loginData.Username, role, s.tokenDuration)
	if err != nil {
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to generate token"})
		return
	}

	s.writeJSON(w, r, map[string]interface{}{
		"success": true,
		"token":   token,
		"user": map[string]interface{}{
			"username": loginData.Username,
			"role":     role,
		},
	})
}

// setExportHeaders 设置导出 handler 的 CORS 响应头
func (s *Server) setExportHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" && contains(s.allowedOrigins, origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// 仅在 OPTIONS 预检请求中设置 Allow-Methods 和 Allow-Headers
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
	}
}

// contains 检查字符串是否在切片中
// NOTE: 当前使用线性遍历 O(n)，allowedOrigins 列表通常较短（<20），
// 性能影响可忽略。如果列表规模增大，建议改为 map[string]bool 查找 O(1)。
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// sanitizeFilename 清理文件名，移除路径分隔符和特殊字符
func sanitizeFilename(name string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, name)
	return safe
}

func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	// 获取 Origin 头
	origin := r.Header.Get("Origin")

	// 仅在有 Origin 头且在允许列表中时设置 CORS 相关头
	if origin != "" && contains(s.allowedOrigins, origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// 仅在 OPTIONS 预检请求中设置 Allow-Methods 和 Allow-Headers
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Errorf("JSON Encode 失败: %v", err)
	}
}

// writeJSONWithStatus 写入带指定 HTTP 状态码的 JSON 响应
func (s *Server) writeJSONWithStatus(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	// 获取 Origin 头
	origin := r.Header.Get("Origin")

	// 仅在有 Origin 头且在允许列表中时设置 CORS 相关头
	if origin != "" && contains(s.allowedOrigins, origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// 仅在 OPTIONS 预检请求中设置 Allow-Methods 和 Allow-Headers
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
	}

	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Errorf("JSON Encode 失败: %v", err)
	}
}

// requireAdmin 中间件：仅允许管理员访问
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role, ok := r.Context().Value(roleContextKey).(string)
		if !ok || role != "admin" {
			s.logger.Warnf("非管理员用户尝试访问受限接口: %s", r.URL.Path)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// handleConfigReload 处理配置重新加载请求
func (s *Server) handleConfigReload(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "配置管理器未初始化"})
		return
	}

	// 只允许 POST 方法
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.logger.Infof("用户手动触发配置重新加载")
	err := s.configMgr.Reload()
	if err != nil {
		s.logger.Errorf("配置重新加载失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"error": "配置重新加载失败", "message": err.Error()})
		return
	}

	newConfig := s.configMgr.GetConfig()
	s.writeJSON(w, r, map[string]interface{}{
		"success": true,
		"message": "配置已重新加载",
		"config":  newConfig.Summary(),
	})
}

// ==================== 新增 API Handler ====================

// --- 辅助函数 ---

// parsePagination 从 query 参数中解析分页参数
func parsePagination(r *http.Request) (page, pageSize int) {
	page = 1
	pageSize = 20
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
			pageSize = v
		}
	}
	return
}

// extractPathID 从 URL 路径中提取 ID 参数
// 例如: /api/alert/123 -> "123"
func extractPathID(path, prefix string) string {
	id := strings.TrimPrefix(path, prefix)
	// 去除前导斜杠
	id = strings.TrimPrefix(id, "/")
	// 去除尾部斜杠和多余路径段
	if idx := strings.Index(id, "/"); idx != -1 {
		id = id[:idx]
	}
	return id
}

// successResponse 构建统一的成功响应
func successResponse(data interface{}, message string) map[string]interface{} {
	return map[string]interface{}{
		"success": true,
		"data":    data,
		"message": message,
	}
}

// paginatedResponse 构建分页响应
func paginatedResponse(items interface{}, total, page, pageSize int) map[string]interface{} {
	return map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	}
}

// ==================== 2. 用户信息 ====================

// handleUserInfo 获取当前登录用户信息（GET /api/users/info）
func (s *Server) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userContextKey).(string)
	if !ok || userID == "" {
		s.writeJSONWithStatus(w, r, http.StatusUnauthorized, map[string]interface{}{"success": false, "message": "未授权"})
		return
	}
	role, _ := r.Context().Value(roleContextKey).(string)

	// 从数据库获取用户偏好设置
	prefs := map[string]interface{}{
		"theme":            "light",
		"language":         "zh-CN",
		"page_size":        20,
		"refresh_interval": 30,
	}
	if s.store != nil {
		if p, err := s.store.GetUserPreferences(userID); err == nil {
			prefs = p
		}
	}

	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"user_id":          userID,
		"username":         userID,
		"role":             role,
		"theme":            prefs["theme"],
		"language":         prefs["language"],
		"page_size":        prefs["page_size"],
		"refresh_interval": prefs["refresh_interval"],
	}, "获取用户信息成功"))
}

// handleUpdateUserInfo 更新用户偏好设置（PUT /api/users/info）
func (s *Server) handleUpdateUserInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userContextKey).(string)
	if !ok || userID == "" {
		s.writeJSONWithStatus(w, r, http.StatusUnauthorized, map[string]interface{}{"success": false, "message": "未授权"})
		return
	}

	var prefs struct {
		Theme           string `json:"theme"`
		Language        string `json:"language"`
		PageSize        int    `json:"page_size"`
		RefreshInterval int    `json:"refresh_interval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}

	// 持久化到数据库
	if s.store != nil {
		if err := s.store.SaveUserPreferences(userID, map[string]interface{}{
			"theme":            prefs.Theme,
			"language":         prefs.Language,
			"page_size":        prefs.PageSize,
			"refresh_interval": prefs.RefreshInterval,
		}); err != nil {
			s.logger.Errorf("保存用户偏好设置失败: %v", err)
			s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "保存失败"})
			return
		}
	}

	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"user_id":          userID,
		"theme":            prefs.Theme,
		"language":         prefs.Language,
		"page_size":        prefs.PageSize,
		"refresh_interval": prefs.RefreshInterval,
	}, "用户偏好设置更新成功"))
}

// handleChangePassword 修改密码（PUT /api/users/password）
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userContextKey).(string)
	if !ok || userID == "" {
		s.writeJSONWithStatus(w, r, http.StatusUnauthorized, map[string]interface{}{"success": false, "message": "未授权"})
		return
	}

	var pwdData struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&pwdData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	if pwdData.OldPassword == "" || pwdData.NewPassword == "" {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "旧密码和新密码不能为空"})
		return
	}
	if len(pwdData.NewPassword) < 8 {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "新密码至少8个字符"})
		return
	}

	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}

	if err := s.store.ChangePassword(userID, pwdData.OldPassword, pwdData.NewPassword); err != nil {
		s.logger.Errorf("修改密码失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}

	s.writeJSON(w, r, successResponse(nil, "密码修改成功"))
}

// ==================== 3. 指标高级查询 ====================

// handleMetricList 分页指标列表（GET /api/metrics/list）
func (s *Server) handleMetricList(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	day := r.URL.Query().Get("date")
	if day == "" {
		day = today()
	}
	probeID := r.URL.Query().Get("probe_id")
	metricName := r.URL.Query().Get("metric_name")

	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}

	results, err := s.store.QueryMetrics(day, probeID, page*pageSize)
	if err != nil {
		s.logger.Errorf("查询指标列表失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}

	// 按 metric_name 过滤
	if metricName != "" {
		filtered := make([]map[string]interface{}, 0)
		for _, m := range results {
			if name, ok := m["metric_name"].(string); ok && name == metricName {
				filtered = append(filtered, m)
			}
		}
		results = filtered
	}

	// 分页截取
	total := len(results)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	items := results[start:end]
	if items == nil {
		items = []map[string]interface{}{}
	}

	s.writeJSON(w, r, paginatedResponse(items, total, page, pageSize))
}

// handleMetricTrend 指标趋势数据（GET /api/metrics/trend）
func (s *Server) handleMetricTrend(w http.ResponseWriter, r *http.Request) {
	metricName := r.URL.Query().Get("metric_name")
	timeRange := r.URL.Query().Get("time_range")
	if timeRange == "" {
		timeRange = "1h"
	}
	probeID := r.URL.Query().Get("probe_id")

	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}

	day := today()
	results, err := s.store.QueryMetrics(day, probeID, 1000)
	if err != nil {
		s.logger.Errorf("查询指标趋势失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}

	// 按时间排序并构建趋势数据
	labels := []string{}
	values := []float64{}
	for _, m := range results {
		if metricName != "" {
			if name, ok := m["metric_name"].(string); ok && name != metricName {
				continue
			}
		}
		if ts, ok := m["timestamp"]; ok {
			var t time.Time
			switch v := ts.(type) {
			case int64:
				t = time.Unix(v, 0)
			case float64:
				t = time.Unix(int64(v), 0)
			}
			labels = append(labels, t.Format("15:04:05"))
			if val, ok := m["value"]; ok {
				values = append(values, toFloat64(val))
			}
		}
	}

	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"metric_name": metricName,
		"time_range":  timeRange,
		"labels":      labels,
		"values":      values,
	}, "获取指标趋势成功"))
}

// handleMetricAggregation 指标聚合查询（GET /api/metrics/aggregation）
func (s *Server) handleMetricAggregation(w http.ResponseWriter, r *http.Request) {
	metricName := r.URL.Query().Get("metric_name")
	aggType := r.URL.Query().Get("aggregation")
	if aggType == "" {
		aggType = "avg"
	}
	timeRange := r.URL.Query().Get("time_range")
	probeID := r.URL.Query().Get("probe_id")

	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}

	day := today()
	results, err := s.store.QueryMetrics(day, probeID, 1000)
	if err != nil {
		s.logger.Errorf("查询指标聚合失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}

	var aggValue float64
	count := 0
	for _, m := range results {
		if metricName != "" {
			if name, ok := m["metric_name"].(string); ok && name != metricName {
				continue
			}
		}
		if val, ok := m["value"]; ok {
			v := toFloat64(val)
			switch aggType {
			case "avg":
				aggValue += v
				count++
			case "sum":
				aggValue += v
			case "max":
				if count == 0 || v > aggValue {
					aggValue = v
				}
				count++
			case "min":
				if count == 0 || v < aggValue {
					aggValue = v
				}
				count++
			}
		}
	}
	if aggType == "avg" && count > 0 {
		aggValue /= float64(count)
	}

	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"metric_name": metricName,
		"aggregation": aggType,
		"time_range":  timeRange,
		"value":       aggValue,
		"count":       count,
	}, "获取指标聚合成功"))
}

// ==================== 4. 告警管理 ====================

// handleAlertList 分页告警列表（GET /api/alert/list）
func (s *Server) handleAlertList(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	severity := r.URL.Query().Get("severity")
	status := r.URL.Query().Get("status")

	if s.alertManager == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "告警服务不可用"})
		return
	}

	alerts := s.alertManager.GetActiveAlerts()
	// 如果请求已解决的告警，也包含历史
	if status == "resolved" {
		alerts = s.alertManager.GetAlertHistory()
	} else if status == "all" {
		history := s.alertManager.GetAlertHistory()
		alerts = append(alerts, history...)
	}

	// 按严重程度过滤
	if severity != "" {
		filtered := make([]*alerting.Alert, 0)
		for _, a := range alerts {
			if a.Severity == severity {
				filtered = append(filtered, a)
			}
		}
		alerts = filtered
	}

	total := len(alerts)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	items := alerts[start:end]
	if items == nil {
		items = []*alerting.Alert{}
	}

	s.writeJSON(w, r, paginatedResponse(items, total, page, pageSize))
}

// handleAlertDetail 告警详情（GET /api/alert/{id}）
func (s *Server) handleAlertDetail(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/alert/")

	if s.alertManager == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "告警服务不可用"})
		return
	}

	// 先在活跃告警中查找
	activeAlerts := s.alertManager.GetActiveAlerts()
	for _, a := range activeAlerts {
		if a.ID == id {
			s.writeJSON(w, r, successResponse(a, "获取告警详情成功"))
			return
		}
	}

	// 再在历史告警中查找
	historyAlerts := s.alertManager.GetAlertHistory()
	for _, a := range historyAlerts {
		if a.ID == id {
			s.writeJSON(w, r, successResponse(a, "获取告警详情成功"))
			return
		}
	}

	s.writeJSONWithStatus(w, r, http.StatusNotFound, map[string]interface{}{"success": false, "message": "告警不存在"})
}

// handleAlertHandle 处理告警（PUT /api/alert/{id}/handle）
func (s *Server) handleAlertHandle(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/alert/")

	if s.alertManager == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "告警服务不可用"})
		return
	}

	var handleData struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&handleData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}

	if handleData.Action == "resolve" {
		if err := s.alertManager.ResolveAlert(id); err != nil {
			s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
	}

	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"alert_id": id,
		"action":   handleData.Action,
		"comment":  handleData.Comment,
	}, "告警处理成功"))
}

// handleAlertRules 告警规则列表（GET /api/alert/rules）
func (s *Server) handleAlertRules(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)

	if s.alertManager == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "告警服务不可用"})
		return
	}

	rules := s.alertManager.GetRuleManager().GetRules()
	total := len(rules)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	items := rules[start:end]
	if items == nil {
		items = []*alerting.Rule{}
	}

	s.writeJSON(w, r, paginatedResponse(items, total, page, pageSize))
}

// handleCreateAlertRule 创建告警规则（POST /api/alert/rules）
func (s *Server) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	if s.alertManager == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "告警服务不可用"})
		return
	}

	var ruleData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&ruleData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}

	// 将 map 转换为 Rule 结构
	ruleBytes, _ := json.Marshal(ruleData)
	var rule alerting.Rule
	if err := json.Unmarshal(ruleBytes, &rule); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的规则数据"})
		return
	}

	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	if err := s.alertManager.GetRuleManager().SaveRule(&rule); err != nil {
		s.logger.Errorf("保存告警规则失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "保存规则失败"})
		return
	}

	s.writeJSON(w, r, successResponse(rule, "告警规则创建成功"))
}

// handleUpdateAlertRule 更新告警规则（PUT /api/alert/rules/{id}）
func (s *Server) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/alert/rules/")

	if s.alertManager == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "告警服务不可用"})
		return
	}

	var ruleData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&ruleData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}

	// 查找现有规则
	existing := s.alertManager.GetRuleManager().GetRuleByID(id)
	if existing == nil {
		s.writeJSONWithStatus(w, r, http.StatusNotFound, map[string]interface{}{"success": false, "message": "规则不存在"})
		return
	}

	// 更新字段
	ruleBytes, _ := json.Marshal(ruleData)
	var updatedRule alerting.Rule
	if err := json.Unmarshal(ruleBytes, &updatedRule); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的规则数据"})
		return
	}
	updatedRule.ID = id
	updatedRule.CreatedAt = existing.CreatedAt
	updatedRule.UpdatedAt = time.Now()

	if err := s.alertManager.GetRuleManager().SaveRule(&updatedRule); err != nil {
		s.logger.Errorf("更新告警规则失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "更新规则失败"})
		return
	}

	s.writeJSON(w, r, successResponse(updatedRule, "告警规则更新成功"))
}

// handleDeleteAlertRule 删除告警规则（DELETE /api/alert/rules/{id}）
func (s *Server) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/alert/rules/")

	if s.alertManager == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "告警服务不可用"})
		return
	}

	if err := s.alertManager.GetRuleManager().DeleteRule(id); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}

	s.writeJSON(w, r, successResponse(nil, "告警规则删除成功"))
}

// ==================== 5. 资产管理 ====================

// handleAssetChangeEvents 资产变更事件（GET /api/asset/change-events）
func (s *Server) handleAssetChangeEvents(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleResourcePools 资源池列表（GET /api/asset/resource-pools）
func (s *Server) handleResourcePools(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleCloudPlatforms 云平台列表（GET /api/asset/cloud-platforms）
func (s *Server) handleCloudPlatforms(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleRegions 区域列表（GET /api/asset/regions）
func (s *Server) handleRegions(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleAvailabilityZones 可用区列表（GET /api/asset/availability-zones）
func (s *Server) handleAvailabilityZones(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleServers 服务器列表（GET /api/asset/servers）
func (s *Server) handleServers(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleHosts 主机列表（GET /api/asset/hosts）
func (s *Server) handleHosts(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleVpcs VPC 列表（GET /api/asset/vpcs）
func (s *Server) handleVpcs(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleSubnets 子网列表（GET /api/asset/subnets）
func (s *Server) handleSubnets(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleRouters 路由器列表（GET /api/asset/routers）
func (s *Server) handleRouters(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleDhcpServers DHCP 服务器列表（GET /api/asset/dhcp-servers）
func (s *Server) handleDhcpServers(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleIpAddresses IP 地址列表（GET /api/asset/ip-addresses）
func (s *Server) handleIpAddresses(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// ==================== 6. 网络分析 ====================

// handleTopology 拓扑图数据（GET /api/topology）
func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"nodes": []map[string]interface{}{},
		"edges": []map[string]interface{}{},
	}, "获取拓扑数据成功"))
}

// handleNetworkResourceAnalysis 网络资源分析（GET /api/network/resource-analysis）
func (s *Server) handleNetworkResourceAnalysis(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"total_bandwidth":  0,
		"used_bandwidth":   0,
		"total_connections": 0,
		"active_connections": 0,
	}, "获取网络资源分析成功"))
}

// handleNetworkPathAnalysis 网络路径分析（GET /api/network/path-analysis）
func (s *Server) handleNetworkPathAnalysis(w http.ResponseWriter, r *http.Request) {
	srcIP := r.URL.Query().Get("src_ip")
	dstIP := r.URL.Query().Get("dst_ip")

	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"src_ip":  srcIP,
		"dst_ip":  dstIP,
		"hops":    []map[string]interface{}{},
		"latency": 0,
		"status":  "completed",
	}, "获取路径分析成功"))
}

// handleNetworkTopologyAnalysis 网络拓扑分析（GET /api/network/topology-analysis）
func (s *Server) handleNetworkTopologyAnalysis(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"nodes":      []map[string]interface{}{},
		"links":      []map[string]interface{}{},
		"statistics": map[string]interface{}{},
	}, "获取网络拓扑分析成功"))
}

// handleFlowLogs 流日志查询（GET /api/network/flow-logs）
func (s *Server) handleFlowLogs(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// ==================== 7. 业务观测 ====================

// handleBusinessList 业务列表（GET /api/business）
func (s *Server) handleBusinessList(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	items, total, err := s.store.ListBusiness(page, pageSize)
	if err != nil {
		s.logger.Errorf("查询业务列表失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}
	s.writeJSON(w, r, paginatedResponse(items, total, page, pageSize))
}

// handleBusinessCreate 创建业务（POST /api/business）
func (s *Server) handleBusinessCreate(w http.ResponseWriter, r *http.Request) {
	var bizData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&bizData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	bizData["id"] = fmt.Sprintf("biz-%d", time.Now().UnixNano())
	bizData["created_at"] = time.Now().Format(time.RFC3339)
	if err := s.store.CreateBusiness(bizData); err != nil {
		s.logger.Errorf("创建业务失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "创建失败"})
		return
	}
	s.writeJSON(w, r, successResponse(bizData, "业务创建成功"))
}

// handleBusinessDetail 业务详情（GET /api/business/{id}）
func (s *Server) handleBusinessDetail(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/business/")
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	biz, err := s.store.GetBusiness(id)
	if err != nil {
		s.logger.Errorf("查询业务详情失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}
	if biz == nil {
		s.writeJSONWithStatus(w, r, http.StatusNotFound, map[string]interface{}{"success": false, "message": "业务不存在"})
		return
	}
	s.writeJSON(w, r, successResponse(biz, "获取业务详情成功"))
}

// handleBusinessUpdate 更新业务（PUT /api/business/{id}）
func (s *Server) handleBusinessUpdate(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/business/")
	var bizData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&bizData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	if err := s.store.UpdateBusiness(id, bizData); err != nil {
		s.logger.Errorf("更新业务失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "更新失败"})
		return
	}
	bizData["id"] = id
	s.writeJSON(w, r, successResponse(bizData, "业务更新成功"))
}

// handleBusinessDelete 删除业务（DELETE /api/business/{id}）
func (s *Server) handleBusinessDelete(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/business/")
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	if err := s.store.DeleteBusiness(id); err != nil {
		s.logger.Errorf("删除业务失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	s.writeJSON(w, r, successResponse(map[string]interface{}{"id": id}, "业务删除成功"))
}

// handleServiceList 服务列表（GET /api/service）
func (s *Server) handleServiceList(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	items, total, err := s.store.ListService(page, pageSize)
	if err != nil {
		s.logger.Errorf("查询服务列表失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}
	s.writeJSON(w, r, paginatedResponse(items, total, page, pageSize))
}

// handleServiceCreate 创建服务（POST /api/service）
func (s *Server) handleServiceCreate(w http.ResponseWriter, r *http.Request) {
	var svcData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&svcData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	svcData["id"] = fmt.Sprintf("svc-%d", time.Now().UnixNano())
	svcData["created_at"] = time.Now().Format(time.RFC3339)
	if err := s.store.CreateService(svcData); err != nil {
		s.logger.Errorf("创建服务失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "创建失败"})
		return
	}
	s.writeJSON(w, r, successResponse(svcData, "服务创建成功"))
}

// handleServiceDetail 服务详情（GET /api/service/{id}）
func (s *Server) handleServiceDetail(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/service/")
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	svc, err := s.store.GetService(id)
	if err != nil {
		s.logger.Errorf("查询服务详情失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}
	if svc == nil {
		s.writeJSONWithStatus(w, r, http.StatusNotFound, map[string]interface{}{"success": false, "message": "服务不存在"})
		return
	}
	s.writeJSON(w, r, successResponse(svc, "获取服务详情成功"))
}

// handleServiceUpdate 更新服务（PUT /api/service/{id}）
func (s *Server) handleServiceUpdate(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/service/")
	var svcData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&svcData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	if err := s.store.UpdateService(id, svcData); err != nil {
		s.logger.Errorf("更新服务失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "更新失败"})
		return
	}
	svcData["id"] = id
	s.writeJSON(w, r, successResponse(svcData, "服务更新成功"))
}

// handleServiceDelete 删除服务（DELETE /api/service/{id}）
func (s *Server) handleServiceDelete(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/service/")
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	if err := s.store.DeleteService(id); err != nil {
		s.logger.Errorf("删除服务失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	s.writeJSON(w, r, successResponse(map[string]interface{}{"id": id}, "服务删除成功"))
}

// ==================== 8. 系统管理 ====================

// handleCollectorList 采集器列表（GET /api/system/collectors）
func (s *Server) handleCollectorList(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	items, total, err := s.store.ListCollector(page, pageSize)
	if err != nil {
		s.logger.Errorf("查询采集器列表失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}
	s.writeJSON(w, r, paginatedResponse(items, total, page, pageSize))
}

// handleCollectorDetail 采集器详情（GET /api/system/collectors/{id}）
func (s *Server) handleCollectorDetail(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/system/collectors/")
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	col, err := s.store.GetCollector(id)
	if err != nil {
		s.logger.Errorf("查询采集器详情失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}
	if col == nil {
		s.writeJSONWithStatus(w, r, http.StatusNotFound, map[string]interface{}{"success": false, "message": "采集器不存在"})
		return
	}
	s.writeJSON(w, r, successResponse(col, "获取采集器详情成功"))
}

// handleCollectorCreate 创建采集器（POST /api/system/collectors）
func (s *Server) handleCollectorCreate(w http.ResponseWriter, r *http.Request) {
	var colData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&colData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	colData["id"] = fmt.Sprintf("collector-%d", time.Now().UnixNano())
	colData["status"] = "running"
	if err := s.store.CreateCollector(colData); err != nil {
		s.logger.Errorf("创建采集器失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "创建失败"})
		return
	}
	s.writeJSON(w, r, successResponse(colData, "采集器创建成功"))
}

// handleCollectorUpdate 更新采集器（PUT /api/system/collectors/{id}）
func (s *Server) handleCollectorUpdate(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/system/collectors/")
	var colData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&colData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	if err := s.store.UpdateCollector(id, colData); err != nil {
		s.logger.Errorf("更新采集器失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "更新失败"})
		return
	}
	colData["id"] = id
	s.writeJSON(w, r, successResponse(colData, "采集器更新成功"))
}

// handleCollectorDelete 删除采集器（DELETE /api/system/collectors/{id}）
func (s *Server) handleCollectorDelete(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/system/collectors/")
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	if err := s.store.DeleteCollector(id); err != nil {
		s.logger.Errorf("删除采集器失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	s.writeJSON(w, r, successResponse(map[string]interface{}{"id": id}, "采集器删除成功"))
}

// handleDataNodeList 数据节点列表（GET /api/system/data-nodes）
func (s *Server) handleDataNodeList(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	items, total, err := s.store.ListDataNode(page, pageSize)
	if err != nil {
		s.logger.Errorf("查询数据节点列表失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}
	s.writeJSON(w, r, paginatedResponse(items, total, page, pageSize))
}

// handleDataNodeDetail 数据节点详情（GET /api/system/data-nodes/{id}）
func (s *Server) handleDataNodeDetail(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/system/data-nodes/")
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	node, err := s.store.GetDataNode(id)
	if err != nil {
		s.logger.Errorf("查询数据节点详情失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "查询失败"})
		return
	}
	if node == nil {
		s.writeJSONWithStatus(w, r, http.StatusNotFound, map[string]interface{}{"success": false, "message": "数据节点不存在"})
		return
	}
	s.writeJSON(w, r, successResponse(node, "获取数据节点详情成功"))
}

// handleDataNodeCreate 创建数据节点（POST /api/system/data-nodes）
func (s *Server) handleDataNodeCreate(w http.ResponseWriter, r *http.Request) {
	var nodeData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&nodeData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	nodeData["id"] = fmt.Sprintf("node-%d", time.Now().UnixNano())
	nodeData["status"] = "online"
	if err := s.store.CreateDataNode(nodeData); err != nil {
		s.logger.Errorf("创建数据节点失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "创建失败"})
		return
	}
	s.writeJSON(w, r, successResponse(nodeData, "数据节点创建成功"))
}

// handleDataNodeUpdate 更新数据节点（PUT /api/system/data-nodes/{id}）
func (s *Server) handleDataNodeUpdate(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/system/data-nodes/")
	var nodeData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&nodeData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	if err := s.store.UpdateDataNode(id, nodeData); err != nil {
		s.logger.Errorf("更新数据节点失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "更新失败"})
		return
	}
	nodeData["id"] = id
	s.writeJSON(w, r, successResponse(nodeData, "数据节点更新成功"))
}

// handleDataNodeDelete 删除数据节点（DELETE /api/system/data-nodes/{id}）
func (s *Server) handleDataNodeDelete(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/system/data-nodes/")
	if s.store == nil {
		s.writeJSONWithStatus(w, r, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "存储服务不可用"})
		return
	}
	if err := s.store.DeleteDataNode(id); err != nil {
		s.logger.Errorf("删除数据节点失败: %v", err)
		s.writeJSONWithStatus(w, r, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	s.writeJSON(w, r, successResponse(map[string]interface{}{"id": id}, "数据节点删除成功"))
}

// handleGetSystemConfig 获取系统配置（GET /api/system/config）
func (s *Server) handleGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	cfg := map[string]interface{}{
		"probe_interval":    "10s",
		"data_retention":    "7d",
		"alert_check_interval": "10s",
		"max_connections":   1000,
		"log_level":         "info",
		"tls_enabled":       false,
		"rate_limit_enabled": false,
	}

	if s.centerConfig != nil {
		cfg["data_retention"] = fmt.Sprintf("%dd", s.centerConfig.RetentionDays)
		cfg["log_level"] = s.centerConfig.Log.Level
		cfg["tls_enabled"] = s.centerConfig.TLS.Enabled
		cfg["rate_limit_enabled"] = s.centerConfig.RateLimit.Enabled
		if s.centerConfig.RateLimit.Enabled {
			cfg["rate_limit_bucket_size"] = s.centerConfig.RateLimit.BucketSize
			cfg["rate_limit_refill_rate"] = s.centerConfig.RateLimit.RefillRate
		}
		if s.centerConfig.Alerting.CheckInterval > 0 {
			cfg["alert_check_interval"] = fmt.Sprintf("%ds", s.centerConfig.Alerting.CheckInterval)
		}
	}

	s.writeJSON(w, r, successResponse(cfg, "获取系统配置成功"))
}

// handleUpdateSystemConfig 更新系统配置（PUT /api/system/config）
// 注意：当前配置通过 config.yaml / 环境变量管理，运行时修改仅影响内存中的值，服务重启后恢复。
// 如需持久化，建议后续实现配置写入到数据库或配置文件。
func (s *Server) handleUpdateSystemConfig(w http.ResponseWriter, r *http.Request) {
	// 仅管理员可修改系统配置
	role, _ := r.Context().Value(roleContextKey).(string)
	if role != "admin" {
		s.writeJSONWithStatus(w, r, http.StatusForbidden, map[string]interface{}{"success": false, "message": "仅管理员可修改系统配置"})
		return
	}
	var configData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&configData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	s.writeJSON(w, r, successResponse(configData, "系统配置更新成功（注：当前仅更新内存配置，重启后恢复）"))
}

// handleSystemLogs 系统日志查询（GET /api/system/logs）
func (s *Server) handleSystemLogs(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	level := r.URL.Query().Get("level")
	_ = level // 预留过滤参数

	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// ==================== 9. 报表 ====================

// handleReportList 报表列表（GET /api/report/list）
func (s *Server) handleReportList(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	s.writeJSON(w, r, paginatedResponse([]map[string]interface{}{}, 0, page, pageSize))
}

// handleReportDetail 报表详情（GET /api/report/{id}）
func (s *Server) handleReportDetail(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/report/")
	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"id":          id,
		"name":        "",
		"type":        "",
		"status":      "completed",
		"created_at":  time.Now().Format(time.RFC3339),
		"download_url": "",
	}, "获取报表详情成功"))
}

// handleReportGenerate 生成报表（POST /api/report/generate）
func (s *Server) handleReportGenerate(w http.ResponseWriter, r *http.Request) {
	var reportData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&reportData); err != nil {
		s.writeJSONWithStatus(w, r, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效的请求体"})
		return
	}
	reportData["id"] = fmt.Sprintf("report-%d", time.Now().UnixNano())
	reportData["status"] = "generating"
	reportData["created_at"] = time.Now().Format(time.RFC3339)
	s.writeJSON(w, r, successResponse(reportData, "报表生成任务已提交"))
}

// handleReportDownload 下载报表（GET /api/report/{id}/download）
func (s *Server) handleReportDownload(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/report/")
	s.writeJSON(w, r, successResponse(map[string]interface{}{
		"report_id":    id,
		"download_url": "",
		"status":       "ready",
	}, "报表下载准备就绪"))
}

