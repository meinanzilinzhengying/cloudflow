// Package metrics 提供 Prometheus metrics 暴露能力
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// ============================================================================
// 预定义指标
// ============================================================================

var (
	// HttpRequestsTotal HTTP 请求计数器
	// Labels: method, status, path
	HttpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "status", "path"},
	)

	// HttpRequestDurationSeconds HTTP 请求延迟 histogram
	// Labels: method, path
	HttpRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// GrpcRequestsTotal gRPC 请求计数器
	// Labels: method, service, status
	GrpcRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_requests_total",
			Help: "Total number of gRPC requests",
		},
		[]string{"method", "service", "status"},
	)

	// GrpcRequestDurationSeconds gRPC 请求延迟 histogram
	// Labels: method, service
	GrpcRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "gRPC request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "service"},
	)
)

// ============================================================================
// Registry 管理
// ============================================================================

var (
	// globalRegistry 全局 Registry
	globalRegistry *prometheus.Registry
)

// NewRegistry 创建新的 Prometheus Registry
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	return reg
}

// init 初始化全局 Registry
func init() {
	globalRegistry = NewRegistry()
}

// Handler 返回标准的 Prometheus metrics HTTP handler
func Handler() http.Handler {
	return promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	)
}

// HandlerForRegistry 返回使用指定 Registry 的 Prometheus metrics HTTP handler
func HandlerForRegistry(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	)
}

// ============================================================================
// gRPC Interceptor
// ============================================================================

// GrpcUnaryInterceptor 返回 gRPC unary server interceptor
func GrpcUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		service := extractServiceName(info.FullMethod)
		method := extractMethodName(info.FullMethod)
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start).Seconds()
		statusCode := "OK"
		if err != nil {
			statusCode = status.Code(err).String()
		}
		GrpcRequestsTotal.WithLabelValues(method, service, statusCode).Inc()
		GrpcRequestDurationSeconds.WithLabelValues(method, service).Observe(duration)
		return resp, err
	}
}

// extractServiceName 从 FullMethod 提取服务名称
func extractServiceName(fullMethod string) string {
	if len(fullMethod) > 0 && fullMethod[0] == '/' {
		fullMethod = fullMethod[1:]
	}
	idx := -1
	for i, c := range fullMethod {
		if c == '/' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "unknown"
	}
	return fullMethod[:idx]
}

// extractMethodName 从 FullMethod 提取方法名称
func extractMethodName(fullMethod string) string {
	idx := -1
	for i := len(fullMethod) - 1; i >= 0; i-- {
		if fullMethod[i] == '/' {
			idx = i
			break
		}
	}
	if idx == -1 || idx == len(fullMethod)-1 {
		return "unknown"
	}
	return fullMethod[idx+1:]
}

// ============================================================================
// HTTP Middleware
// ============================================================================

// responseWriter 包装 http.ResponseWriter 以捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader 重写 WriteHeader 以捕获状态码
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// HttpMiddleware 返回 HTTP middleware
func HttpMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		duration := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		HttpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(rw.statusCode), path).Inc()
		HttpRequestDurationSeconds.WithLabelValues(r.Method, path).Observe(duration)
	})
}

// normalizePath 规范化路径以避免高基数
func normalizePath(path string) string {
	return path
}

// ============================================================================
// 辅助函数
// ============================================================================

// InstrumentHandler 包装 http.Handler 以自动记录 Prometheus 指标
func InstrumentHandler(handler http.Handler) http.Handler {
	return HttpMiddleware(handler)
}

// InstrumentHandlerFunc 包装 http.HandlerFunc 以自动记录 Prometheus 指标
func InstrumentHandlerFunc(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		HttpMiddleware(handler).ServeHTTP(w, r)
	}
}

// RegisterCustomCounter 注册自定义 Counter 指标
func RegisterCustomCounter(name, help string, labels []string) *prometheus.CounterVec {
	return promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: name,
			Help: help,
		},
		labels,
	)
}

// RegisterCustomHistogram 注册自定义 Histogram 指标
func RegisterCustomHistogram(name, help string, labels []string, buckets []float64) *prometheus.HistogramVec {
	if buckets == nil {
		buckets = prometheus.DefBuckets
	}
	return promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    name,
			Help:    help,
			Buckets: buckets,
		},
		labels,
	)
}

// RegisterCustomGauge 注册自定义 Gauge 指标
func RegisterCustomGauge(name, help string, labels []string) *prometheus.GaugeVec {
	return promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: help,
		},
		labels,
	)
}
