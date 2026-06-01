package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"cloud-flow/services/auth-service"
)

func main() {
	cfg := authservice.DefaultConfig()
	flag.StringVar(&cfg.GrpcAddr, "grpc-addr", cfg.GrpcAddr, "gRPC listen address")
	flag.Parse()

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

	svc, err := authservice.New(cfg)
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
