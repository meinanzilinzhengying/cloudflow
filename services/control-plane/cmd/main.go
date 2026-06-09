// Package main Control Plane 入口
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/services/control-plane"
)

func main() {
	cfg := controlplane.DefaultConfig()

	// 命令行参数
	flag.StringVar(&cfg.GrpcAddr, "grpc-addr", cfg.GrpcAddr, "gRPC listen address")
	flag.StringVar(&cfg.HttpAddr, "http-addr", cfg.HttpAddr, "HTTP listen address")
	flag.Parse()

	// 从环境变量读取配置（Docker Compose 部署时使用）
	if v := os.Getenv("SERVICE_NAME"); v != "" {
		cfg.ServiceName = v
	}
	if v := os.Getenv("ETCD_ENDPOINTS"); v != "" {
		cfg.EtcdEndpoints = strings.Split(v, ",")
	}
	if v := os.Getenv("ETCD_PREFIX"); v != "" {
		cfg.EtcdPrefix = v
	}
	if v := os.Getenv("DATA_PLANE_ADDR"); v != "" {
		cfg.DataPlaneAddr = v
	}
	if v := os.Getenv("AUTH_ADDR"); v != "" {
		cfg.AuthAddr = v
	}
	if v := os.Getenv("TENANT_ADDR"); v != "" {
		cfg.TenantAddr = v
	}
	if v := os.Getenv("AGENT_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.AgentTTL = d
		}
	}
	if v := os.Getenv("HEARTBEAT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.HeartbeatTimeout = d
		}
	}
	if v := os.Getenv("TLS_ENABLED"); v == "true" || v == "1" {
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
	if v := os.Getenv("TLS_CLIENT_AUTH"); v == "true" || v == "1" {
		cfg.TLSClientAuth = true
	}
	if v := os.Getenv("TLS_INSECURE_SKIP"); v == "true" || v == "1" {
		cfg.TLSInsecureSkip = true
	}

	svc, err := controlplane.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create service: %v\n", err)
		os.Exit(1)
	}

	if err := svc.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start service: %v\n", err)
		os.Exit(1)
	}

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("Shutting down...")
	svc.Stop()
}
