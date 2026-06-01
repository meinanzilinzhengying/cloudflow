// Package main Control Plane 入口
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"cloud-flow/services/control-plane"
)

func main() {
	cfg := controlplane.DefaultConfig()

	flag.StringVar(&cfg.GrpcAddr, "grpc-addr", cfg.GrpcAddr, "gRPC listen address")
	flag.StringVar(&cfg.HttpAddr, "http-addr", cfg.HttpAddr, "HTTP listen address")
	flag.Parse()

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
