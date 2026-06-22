package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/meinanzilinzhengying/cloudflow/services/query-service"
	"github.com/meinanzilinzhengying/cloudflow/pkg/storage"
)

func main() {
	cfg := queryservice.DefaultConfig()
	flag.StringVar(&cfg.GrpcAddr, "grpc-addr", cfg.GrpcAddr, "gRPC listen address")
	flag.StringVar(&cfg.HttpAddr, "http-addr", cfg.HttpAddr, "HTTP listen address")
	flag.Parse()

	// 从环境变量读取 ClickHouse/TSDB 配置（兼容两套命名）
	if addr := os.Getenv("CLICKHOUSE_ADDR"); addr != "" {
		cfg.TimeSeriesDBHost = addr
	}
	if addr := os.Getenv("TSDB_ADDR"); addr != "" {
		cfg.TimeSeriesDBHost = addr
	}
	if user := os.Getenv("CLICKHOUSE_USER"); user != "" {
		cfg.TimeSeriesDBUser = user
	}
	if user := os.Getenv("TSDB_USER"); user != "" {
		cfg.TimeSeriesDBUser = user
	}
	if password := os.Getenv("CLICKHOUSE_PASSWORD"); password != "" {
		cfg.TimeSeriesDBPassword = password
	}
	if password := os.Getenv("TSDB_PASSWORD"); password != "" {
		cfg.TimeSeriesDBPassword = password
	}
	if db := os.Getenv("CLICKHOUSE_DATABASE"); db != "" {
		cfg.TimeSeriesDBDatabase = db
	}
	if db := os.Getenv("TSDB_DATABASE"); db != "" {
		cfg.TimeSeriesDBDatabase = db
	}
	if port := os.Getenv("CLICKHOUSE_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.TimeSeriesDBPort = p
		}
	}
	if port := os.Getenv("TSDB_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.TimeSeriesDBPort = p
		}
	}
	if tsdbType := os.Getenv("TSDB_TYPE"); tsdbType != "" {
		if tsdbType == "clickhouse" {
			cfg.TimeSeriesDBType = storage.DatabaseClickHouse
		}
	}

	// Auth 配置
	if addr := os.Getenv("AUTH_ADDR"); addr != "" {
		cfg.AuthAddr = addr
	} else {
		cfg.AuthAddr = "auth-service:9006"
	}

	// P0-2 TLS 配置
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

	svc, err := queryservice.New(cfg)
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
