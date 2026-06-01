// Package tlsutil 提供 TLS 相关工具函数
// 包括证书加载、生成自签名证书等功能
package tlsutil

import (
	"crypto/x509"
	"fmt"
	"os"
)

// LoadCA 加载 CA 证书
func LoadCA(caPath string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("读取 CA 证书失败: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("解析 CA 证书失败")
	}

	return certPool, nil
}
