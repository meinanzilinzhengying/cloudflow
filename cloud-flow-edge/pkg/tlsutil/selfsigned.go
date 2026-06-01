// Package tlsutil 提供 TLS 工具函数，包括自签名证书生成
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// GenSelfSignedCertFiles 在指定目录生成自签名证书和私钥文件
// 返回 (certPath, keyPath, error)
func GenSelfSignedCertFiles(dir string) (string, string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", fmt.Errorf("创建证书目录失败: %w", err)
	}

	// 生成 ECDSA P-256 密钥对
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("生成密钥对失败: %w", err)
	}

	// 生成自签名证书（有效期 10 年）
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", "", fmt.Errorf("生成序列号失败: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:  []string{"Cloud Flow"},
			Country:       []string{"CN"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              []string{"localhost", "edge", "center", "backend"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", fmt.Errorf("创建证书失败: %w", err)
	}

	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	// 写入证书
	certFile, err := os.Create(certPath)
	if err != nil {
		return "", "", fmt.Errorf("创建证书文件失败: %w", err)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return "", "", fmt.Errorf("写入证书失败: %w", err)
	}

	// 写入私钥（权限 0600，仅所有者可读写）
	keyFile, err := os.OpenFile(keyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", "", fmt.Errorf("创建私钥文件失败: %w", err)
	}
	defer keyFile.Close()
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("序列化私钥失败: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		return "", "", fmt.Errorf("写入私钥失败: %w", err)
	}

	return certPath, keyPath, nil
}

// LoadOrGenSelfSignedCert 检查证书文件是否存在，不存在则自动生成
// 返回可用的 TLS 证书配置
func LoadOrGenSelfSignedCert(certPath, keyPath, genDir string) (*tls.Certificate, error) {
	// 如果提供了路径且文件都存在，直接加载
	if certPath != "" && keyPath != "" {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("加载证书失败: %w", err)
		}
		return &cert, nil
	}

	// 自动生成自签名证书
	autoCert, autoKey, err := GenSelfSignedCertFiles(genDir)
	if err != nil {
		return nil, fmt.Errorf("生成自签名证书失败: %w", err)
	}
	// NOTE: 此处使用 log.Printf 而非结构化日志，因为 tlsutil 包不依赖外部日志库。
	// 在后续版本中应考虑注入日志接口或使用标准库的 log/slog。
	log.Printf("[TLS] 已自动生成自签名证书: cert=%s, key=%s (有效期 10 年)\n", autoCert, autoKey)

	cert, err := tls.LoadX509KeyPair(autoCert, autoKey)
	if err != nil {
		return nil, fmt.Errorf("加载自签名证书失败: %w", err)
	}
	return &cert, nil
}
