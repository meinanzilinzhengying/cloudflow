// Package tlsutil TLS/mTLS 工具包
//
// 提供 gRPC 服务间 TLS/mTLS 通信支持
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Config TLS 配置
type Config struct {
	Enabled      bool   // 是否启用 TLS
	CAFile       string // CA 证书路径
	CertFile     string // 证书路径
	KeyFile      string // 私钥路径
	ClientAuth   bool   // 是否要求客户端证书 (mTLS)
	InsecureSkip bool   // 是否跳过证书验证 (仅用于开发)
}

// ServerCredentials 创建服务器 TLS credentials
func ServerCredentials(cfg Config) (credentials.TransportCredentials, error) {
	if !cfg.Enabled {
		return insecure.NewCredentials(), nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if cfg.ClientAuth && cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		serverCert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load server cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{serverCert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// ClientCredentials 创建客户端 TLS credentials
func ClientCredentials(cfg Config) (credentials.TransportCredentials, error) {
	if !cfg.Enabled {
		return insecure.NewCredentials(), nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if cfg.InsecureSkip {
		tlsConfig.InsecureSkipVerify = true
		return credentials.NewTLS(tlsConfig), nil
	}

	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// DialOptions 获取 gRPC dial 选项
func DialOptions(cfg Config) ([]grpc.DialOption, error) {
	creds, err := ClientCredentials(cfg)
	if err != nil {
		return nil, err
	}
	return []grpc.DialOption{grpc.WithTransportCredentials(creds)}, nil
}
