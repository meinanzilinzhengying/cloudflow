package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"cloud-flow/services/data-plane"
)

func main() {
	cfg := dataplane.DefaultConfig()
	flag.StringVar(&cfg.GrpcAddr, "grpc-addr", cfg.GrpcAddr, "gRPC listen address")
	flag.StringVar(&cfg.MetricsAddr, "metrics-addr", cfg.MetricsAddr, "Metrics HTTP listen address")
	flag.Parse()

	// 从环境变量读取配置
	if addr := os.Getenv("CLICKHOUSE_ADDR"); addr != "" {
		cfg.ClickHouseAddr = addr
	}
	if user := os.Getenv("CLICKHOUSE_USER"); user != "" {
		cfg.ClickHouseUser = user
	}
	if password := os.Getenv("CLICKHOUSE_PASSWORD"); password != "" {
		cfg.ClickHousePassword = password
	}
	if db := os.Getenv("CLICKHOUSE_DATABASE"); db != "" {
		cfg.ClickHouseDatabase = db
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

	svc, err := dataplane.New(cfg)
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
