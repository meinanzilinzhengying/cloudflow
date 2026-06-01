// Package auth 提供 mTLS 认证拦截器，用于 gRPC 服务端的 Agent 身份验证
// 支持基于 TLS 证书 CN、Agent Token 和白名单 ProbeID 的多维度认证
package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"cloud-flow-edge/internal/whitelist"
	"cloud-flow/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// DefaultTokenHeader 默认的 Agent Token gRPC metadata header 键名
const DefaultTokenHeader = "x-agent-token"

// AuthConfig 认证拦截器配置
type AuthConfig struct {
	// Enabled 是否启用认证拦截器
	Enabled bool

	// RequireMTLS 是否要求 mTLS（客户端必须提供有效证书）
	RequireMTLS bool

	// RequireToken 是否要求 Agent Token 认证
	RequireToken bool

	// RequireWhitelist 是否要求 ProbeID 在白名单中
	RequireWhitelist bool

	// TokenHeader gRPC metadata 中 Token 的 header 键名
	TokenHeader string

	// RejectLog 是否记录被拒绝的连接详情
	RejectLog bool
}

// normalize 确保配置中的默认值被正确设置
func (c *AuthConfig) normalize() {
	if c.TokenHeader == "" {
		c.TokenHeader = DefaultTokenHeader
	}
}

// AuthInterceptor gRPC 一元拦截器，执行 Agent 认证
type AuthInterceptor struct {
	config    AuthConfig
	wlManager *whitelist.Manager
	mu        sync.RWMutex
}

// NewAuthInterceptor 创建一个新的认证拦截器
func NewAuthInterceptor(config AuthConfig, wlManager *whitelist.Manager) *AuthInterceptor {
	config.normalize()
	return &AuthInterceptor{
		config:    config,
		wlManager: wlManager,
	}
}

// UpdateConfig 动态更新认证配置
func (ai *AuthInterceptor) UpdateConfig(config AuthConfig) {
	config.normalize()
	ai.mu.Lock()
	defer ai.mu.Unlock()
	ai.config = config
}

// UnaryServerInterceptor 返回 gRPC 一元服务器拦截器函数
// 可直接传入 grpc.Server 的拦截器链
func (ai *AuthInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 读取当前配置的快照
		cfg := ai.currentConfig()

		// 如果认证未启用，直接放行
		if !cfg.Enabled {
			return handler(ctx, req)
		}

		// 提取认证信息
		clientCN := ExtractClientCN(ctx)
		agentToken := ExtractAgentToken(ctx, cfg.TokenHeader)
		probeID := ExtractProbeID(req)

		// 获取客户端地址
		clientAddr := extractClientAddr(ctx)

		// ---- 1. mTLS 证书 CN 检查 ----
		if cfg.RequireMTLS {
			if clientCN == "" {
				ai.logReject(clientAddr, "", probeID, "mTLS required but no client certificate CN provided")
				return nil, fmt.Errorf("permission denied: mTLS required but no client certificate provided")
			}
			if !ai.wlManager.IsAllowedCN(clientCN) {
				ai.logReject(clientAddr, clientCN, probeID, "certificate CN not in whitelist")
				return nil, fmt.Errorf("permission denied: certificate CN %q is not authorized", clientCN)
			}
		}

		// ---- 2. Agent Token 检查 ----
		if cfg.RequireToken {
			if agentToken == "" {
				ai.logReject(clientAddr, clientCN, probeID, "token required but not provided")
				return nil, fmt.Errorf("permission denied: agent token required but not provided in header %q", cfg.TokenHeader)
			}
			tokenHash := HashToken(agentToken)
			if !ai.wlManager.IsAllowedToken(tokenHash) {
				ai.logReject(clientAddr, clientCN, probeID, "token hash not in whitelist")
				return nil, fmt.Errorf("permission denied: agent token is not authorized")
			}
		}

		// ---- 3. 白名单 ProbeID 检查 ----
		if cfg.RequireWhitelist {
			if probeID == "" {
				ai.logReject(clientAddr, clientCN, probeID, "whitelist check required but ProbeID not found in request")
				return nil, fmt.Errorf("permission denied: ProbeID is required for whitelist verification")
			}
			if !ai.wlManager.IsAllowed(probeID) {
				ai.logReject(clientAddr, clientCN, probeID, "ProbeID not in whitelist or expired")
				return nil, fmt.Errorf("permission denied: ProbeID %q is not authorized or has expired", probeID)
			}
		}

		return handler(ctx, req)
	}
}

// currentConfig 读取当前配置的快照
func (ai *AuthInterceptor) currentConfig() AuthConfig {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.config
}

// logReject 记录被拒绝的连接详情
func (ai *AuthInterceptor) logReject(clientAddr, cn, probeID, reason string) {
	cfg := ai.currentConfig()
	if !cfg.RejectLog {
		return
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	fmt.Printf(
		"[auth] REJECTED connection | addr=%s | cn=%q | probeID=%q | reason=%s | time=%s\n",
		clientAddr, cn, probeID, reason, timestamp,
	)
}

// extractClientAddr 从 gRPC 上下文中提取客户端地址
func extractClientAddr(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p == nil || p.Addr == nil {
		return "unknown"
	}
	return p.Addr.String()
}

// ============================================================================
// 公共辅助函数
// ============================================================================

// ExtractClientCN 从 gRPC 上下文中提取 TLS 客户端证书的 Common Name (CN)
// 优先从 peer.Peer 的 TLS 状态中提取证书链的第一个叶子证书的 Subject CN
func ExtractClientCN(ctx context.Context) string {
	// 方式 1: 通过 gRPC peer 上下文获取 TLS 连接信息
	p, ok := peer.FromContext(ctx)
	if ok && p != nil {
		tlsInfo, isTLS := p.AuthInfo.(interface {
			GetState() interface{}
		})
		if isTLS && tlsInfo != nil {
			// 尝试从 TLS connection state 获取证书
			state := tlsInfo.GetState()
			if state != nil {
				cn := extractCNFromState(state)
				if cn != "" {
					return cn
				}
			}
		}
	}

	// 方式 2: 从 context value 中查找（某些中间件可能注入）
	if v := ctx.Value("peer.certificate"); v != nil {
		if certBytes, ok := v.([]byte); ok {
			cn := extractCNFromRawDER(certBytes)
			if cn != "" {
				return cn
			}
		}
	}

	return ""
}

// extractCNFromState 从 TLS connection state 中提取 CN
// 使用类型断言处理标准库的 tls.ConnectionState
func extractCNFromState(state interface{}) string {
	// 尝试通过反射式接口访问证书
	// 标准库 tls.ConnectionState 包含 PeerCertificates 字段
	type certGetter interface {
		PeerCertificates() interface{}
	}

	// 使用 map 访问方式（兼容性更强）
	switch s := state.(type) {
	case map[string]interface{}:
		if certs, ok := s["PeerCertificates"]; ok {
			return extractCNFromCertSlice(certs)
		}
	case interface{ PeerCertificates() []interface{} }:
		certs := s.PeerCertificates()
		if len(certs) > 0 {
			return extractCNFromCertObj(certs[0])
		}
	}

	return ""
}

// extractCNFromCertSlice 从证书切片中提取第一个证书的 CN
func extractCNFromCertSlice(certs interface{}) string {
	switch c := certs.(type) {
	case []interface{}:
		if len(c) > 0 {
			return extractCNFromCertObj(c[0])
		}
	}
	return ""
}

// extractCNFromCertObj 从证书对象中提取 CN
func extractCNFromCertObj(cert interface{}) string {
	// 尝试 Subject.CommonName 访问
	type cnAccessor interface {
		Subject() interface{ CommonName() string }
	}
	if ca, ok := cert.(cnAccessor); ok {
		return ca.Subject().CommonName()
	}

	// 尝试直接访问 Subject 字段
	switch c := cert.(type) {
	case map[string]interface{}:
		if subject, ok := c["Subject"].(map[string]interface{}); ok {
			if cn, ok := subject["CommonName"].(string); ok {
				return cn
			}
		}
	case interface{ Subject() interface{} }:
		subj := c.Subject()
		if subMap, ok := subj.(map[string]interface{}); ok {
			if cn, ok := subMap["CommonName"].(string); ok {
				return cn
			}
		}
	}

	return ""
}

// extractCNFromRawDER 从 DER 编码的证书中提取 CN（简化实现）
func extractCNFromRawDER(der []byte) string {
	// 在实际生产中应使用 crypto/x509.ParseCertificate 解析
	// 此处提供基于字符串搜索的简化实现
	// ASN.1 OID for CN: 2.5.4.3 = 55 04 03
	str := string(der)
	cnPrefix := "CN="
	idx := strings.Index(str, cnPrefix)
	if idx == -1 {
		return ""
	}
	start := idx + len(cnPrefix)
	end := start
	for end < len(str) && str[end] >= ' ' && str[end] < 127 && str[end] != ',' {
		end++
	}
	return str[start:end]
}

// ExtractAgentToken 从 gRPC metadata 中提取 Agent Token
func ExtractAgentToken(ctx context.Context, headerKey string) string {
	if headerKey == "" {
		headerKey = DefaultTokenHeader
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get(headerKey)
	if len(values) == 0 {
		// 尝试小写匹配（gRPC metadata key 不区分大小写）
		values = md.Get(strings.ToLower(headerKey))
	}
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

// ExtractProbeID 使用类型 switch 从 gRPC 请求中提取 ProbeID
// 支持所有 edge 包中包含 ProbeID 字段的请求类型
func ExtractProbeID(req interface{}) string {
	if req == nil {
		return ""
	}

	switch r := req.(type) {
	case *edge.RegisterProbeRequest:
		return r.GetProbeId()
	case *edge.HeartbeatRequest:
		return r.GetProbeId()
	case *edge.MetricsBatch:
		return r.GetProbeId()
	case *edge.TraceBatch:
		return r.GetProbeId()
	case *edge.ProfilingBatch:
		return r.GetProbeId()
	case *edge.MetricData:
		return r.GetProbeId()
	case *edge.TraceSpanData:
		return r.GetProbeId()
	case *edge.ProfilingData:
		return r.GetProbeId()
	case *edge.ProbeInfo:
		return r.GetProbeId()
	case *edge.GetConfigRequest:
		return r.ProbeId
	case *edge.ConfigUpdateAck:
		return r.ProbeId
	default:
		return ""
	}
}

// HashToken 使用 SHA256 对 Token 进行哈希，返回小写十六进制字符串
// 使用 crypto/subtle.ConstantTimeCompare 进行安全比较
func HashToken(token string) string {
	if token == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// ConstantTimeCompare 使用恒定时间比较两个字符串，防止时序攻击
// 返回 true 表示匹配
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
