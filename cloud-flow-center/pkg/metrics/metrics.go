// Package metrics 为 Center 提供 Prometheus 指标
package metrics

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	// gRPC 指标
	GRPCRequestsTotal   *prometheus.CounterVec
	GRPCErrorsTotal     *prometheus.CounterVec
	GRPCLatencySeconds  *prometheus.HistogramVec

	// 存储指标
	StorageWritesTotal  *prometheus.CounterVec
	StorageErrorsTotal  *prometheus.CounterVec

	// 数据量指标
	MetricsReceivedTotal prometheus.Counter
	TracesReceivedTotal  prometheus.Counter
	ProfsReceivedTotal   prometheus.Counter

	// 节点指标
	EdgeNodesActive     prometheus.Gauge
	ProbeNodesActive    prometheus.Gauge
}

func New() *Metrics {
	return &Metrics{
		GRPCRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "center_grpc_requests_total",
			Help: "Center gRPC 请求总数",
		}, []string{"method"}),
		GRPCErrorsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "center_grpc_errors_total",
			Help: "Center gRPC 错误总数",
		}, []string{"method"}),
		GRPCLatencySeconds: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "center_grpc_latency_seconds",
			Help:    "Center gRPC 请求延迟",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),
		StorageWritesTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "center_storage_writes_total",
			Help: "存储写入总数",
		}, []string{"table"}),
		StorageErrorsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "center_storage_errors_total",
			Help: "存储写入错误总数",
		}, []string{"table"}),
		MetricsReceivedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "center_metrics_received_total",
			Help: "接收到的指标数据总数",
		}),
		TracesReceivedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "center_traces_received_total",
			Help: "接收到的链路追踪数据总数",
		}),
		ProfsReceivedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "center_profiling_received_total",
			Help: "接收到的性能分析数据总数",
		}),
		EdgeNodesActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "center_edge_nodes_active",
			Help: "当前活跃边缘节点数",
		}),
		ProbeNodesActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "center_probe_nodes_active",
			Help: "当前活跃探针数",
		}),
	}
}

// StartServer 启动 Prometheus metrics HTTP 服务
func StartServer(port int) {
	mux := http.NewServeMux()

	// 带有 Basic Auth 认证的 metrics handler
	metricsHandler := withBasicAuth(promhttp.Handler())
	mux.Handle("/metrics", metricsHandler)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	go func() {
		addr := fmt.Sprintf(":%d", port)
		if err := http.ListenAndServe(addr, mux); err != nil {
			fmt.Printf("Center metrics HTTP 服务启动失败: %v\n", err)
		}
	}()
}

// withBasicAuth 添加 Basic Auth 认证中间件
func withBasicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := os.Getenv("METRICS_USERNAME")
		password := os.Getenv("METRICS_PASSWORD")

		// 如果未配置认证信息，则不进行认证
		if username == "" || password == "" {
			next.ServeHTTP(w, r)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Metrics"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 使用 constant-time 比较防止时序攻击
		userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1
		passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1

		if userMatch && passMatch {
			next.ServeHTTP(w, r)
		} else {
			w.Header().Set("WWW-Authenticate", `Basic realm="Metrics"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}
	})
}
