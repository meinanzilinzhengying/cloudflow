package grpcserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// genSelfSignedCert 生成自签名证书，返回 (cert, certPath, keyPath, error)
func genSelfSignedCert(dir string) (tls.Certificate, string, string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("创建证书目录失败: %w", err)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("生成密钥对失败: %w", err)
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Cloud Flow Center"},
			Country:      []string{"CN"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              []string{"localhost", "center", "backend"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("创建证书失败: %w", err)
	}

	certPath := filepath.Join(dir, "center.crt")
	keyPath := filepath.Join(dir, "center.key")

	certFile, err := os.Create(certPath)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("创建证书文件失败: %w", err)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("写入证书失败: %w", err)
	}

	keyFile, err := os.Create(keyPath)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("创建私钥文件失败: %w", err)
	}
	defer keyFile.Close()
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("序列化私钥失败: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("写入私钥失败: %w", err)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("加载证书失败: %w", err)
	}

	return cert, certPath, keyPath, nil
}
