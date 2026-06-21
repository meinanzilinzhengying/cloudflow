package ratelimit

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MiddlewareConfig 限流中间件配置
type MiddlewareConfig struct {
	GlobalQPS      float64
	GlobalBurst    int
	IPQPS          float64
	IPBurst        int
	UserQPS        float64
	UserBurst      int
	AuthQPS        float64
	AuthBurst      int
	PenaltySeconds int
	RedisLimiter   *RedisLimiter
	WhitelistIPs   []string
	StatusCode     int
}

func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		GlobalQPS:      10000,
		GlobalBurst:    15000,
		IPQPS:          100,
		IPBurst:        150,
		UserQPS:        300,
		UserBurst:      500,
		AuthQPS:        5,
		AuthBurst:      10,
		PenaltySeconds: 60,
		StatusCode:     http.StatusTooManyRequests,
	}
}

// Middleware HTTP 限流中间件
type Middleware struct {
	config        *MiddlewareConfig
	globalLimiter *TokenBucket
	ipLimiter     *TokenBucket
	userLimiter   *TokenBucket
	authLimiter   *TokenBucket
	penalties     map[string]time.Time
	penaltiesMu   sync.RWMutex
	whitelist     map[string]bool
}

func NewMiddleware(config *MiddlewareConfig) *Middleware {
	if config == nil {
		config = DefaultMiddlewareConfig()
	}
	m := &Middleware{
		config:        config,
		globalLimiter: NewTokenBucket(config.GlobalQPS, config.GlobalBurst),
		ipLimiter:     NewTokenBucket(config.IPQPS, config.IPBurst),
		userLimiter:   NewTokenBucket(config.UserQPS, config.UserBurst),
		authLimiter:   NewTokenBucket(config.AuthQPS, config.AuthBurst),
		penalties:     make(map[string]time.Time),
		whitelist:     make(map[string]bool),
	}
	for _, ip := range config.WhitelistIPs {
		m.whitelist[ip] = true
	}
	return m
}

func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := ExtractClientIP(r)

		if m.whitelist[clientIP] {
			next.ServeHTTP(w, r)
			return
		}

		if m.isPenalized(clientIP) {
			m.writeRateLimitResponse(w, fmt.Sprintf("IP %s temporarily blocked due to rate limit", clientIP))
			return
		}

		if !m.globalLimiter.Allow("global") {
			m.writeRateLimitResponse(w, "global rate limit exceeded")
			return
		}

		if !m.ipLimiter.Allow(clientIP) {
			m.penalize(clientIP)
			m.writeRateLimitResponse(w, fmt.Sprintf("rate limit exceeded for IP %s", clientIP))
			return
		}

		if m.config.RedisLimiter != nil {
			if !m.config.RedisLimiter.Allow(clientIP) {
				m.penalize(clientIP)
				m.writeRateLimitResponse(w, "distributed rate limit exceeded")
				return
			}
		}

		userID := m.extractUserID(r)
		if userID != "" {
			if !m.userLimiter.Allow(userID) {
				m.writeRateLimitResponse(w, "user rate limit exceeded")
				return
			}
		}

		if m.isAuthPath(r.URL.Path) {
			if !m.authLimiter.Allow(clientIP) {
				m.penalize(clientIP)
				m.writeRateLimitResponse(w, "authentication rate limit exceeded")
				return
			}
		}

		w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(m.config.IPQPS, 'f', 0, 64))
		w.Header().Set("X-RateLimit-Window", "1s")

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) isAuthPath(path string) bool {
	authPaths := []string{"/login", "/verify", "/refresh", "/apikeys", "/users/create"}
	for _, p := range authPaths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func (m *Middleware) extractUserID(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth != "" && len(auth) > 7 && auth[:7] == "Bearer " {
		return "authenticated_user"
	}
	if r.Header.Get("X-API-Key") != "" {
		return "api_key_user"
	}
	return ""
}

func (m *Middleware) isPenalized(ip string) bool {
	m.penaltiesMu.RLock()
	defer m.penaltiesMu.RUnlock()
	until, ok := m.penalties[ip]
	return ok && time.Now().Before(until)
}

func (m *Middleware) penalize(ip string) {
	m.penaltiesMu.Lock()
	defer m.penaltiesMu.Unlock()
	m.penalties[ip] = time.Now().Add(time.Duration(m.config.PenaltySeconds) * time.Second)
}

func (m *Middleware) writeRateLimitResponse(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(m.config.PenaltySeconds))
	w.WriteHeader(m.config.StatusCode)
	fmt.Fprintf(w, `{"error":"rate_limit_exceeded","message":"%s","retry_after":%d}`, message, m.config.PenaltySeconds)
}

// ConnectionLimiter 并发连接限制器
type ConnectionLimiter struct {
	maxConns int64
	curr     int64
}

func NewConnectionLimiter(maxConns int64) *ConnectionLimiter {
	return &ConnectionLimiter{maxConns: maxConns}
}

func (cl *ConnectionLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&cl.curr, 1) > cl.maxConns {
			atomic.AddInt64(&cl.curr, -1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"server_overloaded","message":"too many concurrent connections"}`))
			return
		}
		defer atomic.AddInt64(&cl.curr, -1)
		next.ServeHTTP(w, r)
	})
}
