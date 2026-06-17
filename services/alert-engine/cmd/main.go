package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/meinanzilinzhengying/cloudflow/services/alert-engine"
)

func main() {
	cfg := alertengine.DefaultConfig()
	flag.StringVar(&cfg.GrpcAddr, "grpc-addr", cfg.GrpcAddr, "gRPC listen address")
	flag.StringVar(&cfg.HttpAddr, "http-addr", cfg.HttpAddr, "HTTP listen address")
	flag.Parse()

	// 从环境变量读取服务地址配置
	if v := os.Getenv("GRPC_ADDR"); v != "" {
		cfg.GrpcAddr = v
	}
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		cfg.HttpAddr = v
	}

	// 从环境变量读取 TiDB 配置
	if v := os.Getenv("TIDB_ADDR"); v != "" {
		cfg.TiDBAddr = v
	}
	if v := os.Getenv("TIDB_USER"); v != "" {
		cfg.TiDBUser = v
	}
	if v := os.Getenv("TIDB_PASSWORD"); v != "" {
		cfg.TiDBPassword = v
	}
	if v := os.Getenv("TIDB_DATABASE"); v != "" {
		cfg.TiDBDatabase = v
	}

	// 从环境变量读取其他服务地址
	if v, ok := os.LookupEnv("AUTH_ADDR"); ok {
		cfg.AuthAddr = v
	}
	if v := os.Getenv("DATA_PLANE_ADDR"); v != "" {
		cfg.DataPlaneAddr = v
	}
	if v := os.Getenv("TENANT_ADDR"); v != "" {
		cfg.TenantAddr = v
	}

	// P0-2 修复: 从环境变量读取 TLS 配置
	if v := os.Getenv("TLS_ENABLED"); v == "true" {
		cfg.TLSEnabled = true
	}
	if v := os.Getenv("TLS_CA_FILE"); v != "" {
		cfg.TLSCAFile = v
	}
	if v := os.Getenv("TLS_CERT_FILE"); v != "" {
		cfg.TLSCertFile = v
	}
	if v := os.Getenv("TLS_KEY_FILE"); v != "" {
		cfg.TLSKeyFile = v
	}
	if v := os.Getenv("TLS_CLIENT_AUTH"); v == "true" {
		cfg.TLSClientAuth = true
	}
	if v := os.Getenv("TLS_INSECURE_SKIP"); v == "true" {
		cfg.TLSInsecureSkip = true
	}

	// 生产环境指标数据源配置
	// MockMetricsEnabled: true 使用模拟数据（仅开发测试），false 使用真实数据源 (VM + ClickHouse)
	if v := os.Getenv("MOCK_METRICS_ENABLED"); v == "true" {
		cfg.MockMetricsEnabled = true
	}

	svc, err := alertengine.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
		os.Exit(1)
	}
	if err := svc.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	svc.Stop()
}
