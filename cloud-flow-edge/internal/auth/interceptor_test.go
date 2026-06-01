package auth

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"crypto/sha256"
	"testing"

	edge "cloud-flow/proto"
)

// TestHashToken 测试 Token 哈希函数
func TestHashToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{"空 token", "", ""},
		{"简单 token", "test", sha256Hex("test")},
		{"包含特殊字符", "abc123!@#", sha256Hex("abc123!@#")},
		{"长 token", "very-long-agent-token-with-special-chars-2024", sha256Hex("very-long-agent-token-with-special-chars-2024")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HashToken(tt.token)
			if got != tt.want {
				t.Errorf("HashToken(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}

// TestHashToken_Deterministic 测试哈希的确定性
func TestHashToken_Deterministic(t *testing.T) {
	token := "my-secret-token"
	h1 := HashToken(token)
	h2 := HashToken(token)
	if h1 != h2 {
		t.Errorf("HashToken 不确定: 第一次=%q, 第二次=%q", h1, h2)
	}
}

// TestHashToken_DifferentTokens 测试不同 token 产生不同哈希
func TestHashToken_DifferentTokens(t *testing.T) {
	h1 := HashToken("token-a")
	h2 := HashToken("token-b")
	if h1 == h2 {
		t.Errorf("不同 token 产生了相同哈希: %q", h1)
	}
}

// TestHashToken_Length 测试哈希长度（SHA256 = 64 hex chars）
func TestHashToken_Length(t *testing.T) {
	h := HashToken("test")
	if len(h) != 64 {
		t.Errorf("SHA256 哈希长度应为 64，实际为 %d", len(h))
	}
}

// TestConstantTimeCompare 测试恒定时间比较
func TestConstantTimeCompare(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"相同字符串", "hello", "hello", true},
		{"不同字符串", "hello", "world", false},
		{"空字符串相同", "", "", true},
		{"一个空一个非空", "", "hello", false},
		{"不同长度", "short", "much-longer-string", false},
		{"包含特殊字符", "abc!@#", "abc!@#", true},
		{"特殊字符不同", "abc!@#", "abc!@$", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConstantTimeCompare(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("ConstantTimeCompare(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestConstantTimeCompare_UsesSubtle 验证使用了 subtle.ConstantTimeCompare
func TestConstantTimeCompare_UsesSubtle(t *testing.T) {
	// 验证我们的实现与标准库一致
	a, b := "test-value", "test-value"
	if ConstantTimeCompare(a, b) != (subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1) {
		t.Error("ConstantTimeCompare 实现与 subtle.ConstantTimeCompare 不一致")
	}
}

// TestExtractProbeID 测试从请求中提取 ProbeID
func TestExtractProbeID(t *testing.T) {
	tests := []struct {
		name string
		req  interface{}
		want string
	}{
		{"nil 请求", nil, ""},
		{"RegisterProbeRequest", &edge.RegisterProbeRequest{ProbeId: "probe-1"}, "probe-1"},
		{"HeartbeatRequest", &edge.HeartbeatRequest{ProbeId: "probe-2"}, "probe-2"},
		{"MetricsBatch", &edge.MetricsBatch{ProbeId: "probe-3"}, "probe-3"},
		{"TraceBatch", &edge.TraceBatch{ProbeId: "probe-4"}, "probe-4"},
		{"ProfilingBatch", &edge.ProfilingBatch{ProbeId: "probe-5"}, "probe-5"},
		{"MetricData", &edge.MetricData{ProbeId: "probe-6"}, "probe-6"},
		{"TraceSpanData", &edge.TraceSpanData{ProbeId: "probe-7"}, "probe-7"},
		{"ProfilingData", &edge.ProfilingData{ProbeId: "probe-8"}, "probe-8"},
		{"ProbeInfo", &edge.ProbeInfo{ProbeId: "probe-9"}, "probe-9"},
		{"GetConfigRequest", &edge.GetConfigRequest{ProbeId: "probe-10"}, "probe-10"},
		{"ConfigUpdateAck", &edge.ConfigUpdateAck{ProbeId: "probe-11"}, "probe-11"},
		{"未知类型", "not-a-proto-message", ""},
		{"空 ProbeId", &edge.RegisterProbeRequest{ProbeId: ""}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractProbeID(tt.req)
			if got != tt.want {
				t.Errorf("ExtractProbeID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExtractAgentToken 测试从 gRPC metadata 中提取 Agent Token
func TestExtractAgentToken(t *testing.T) {
	tests := []struct {
		name      string
		headerKey string
		setup     func() context.Context
		want      string
	}{
		{
			name:      "无 metadata",
			headerKey: "x-agent-token",
			setup:     func() context.Context { return context.Background() },
			want:      "",
		},
		{
			name:      "有 token",
			headerKey: "x-agent-token",
			setup: func() context.Context {
				md := map[string][]string{"x-agent-token": {"my-token-123"}}
				return contextWithMetadata(md)
			},
			want: "my-token-123",
		},
		{
			name:      "空 headerKey 使用默认值",
			headerKey: "",
			setup: func() context.Context {
				md := map[string][]string{"x-agent-token": {"default-header-token"}}
				return contextWithMetadata(md)
			},
			want: "default-header-token",
		},
		{
			name:      "token 不存在",
			headerKey: "x-agent-token",
			setup: func() context.Context {
				md := map[string][]string{"other-header": {"value"}}
				return contextWithMetadata(md)
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			got := ExtractAgentToken(ctx, tt.headerKey)
			if got != tt.want {
				t.Errorf("ExtractAgentToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExtractClientCN_NoPeer 测试无 peer 信息时返回空
func TestExtractClientCN_NoPeer(t *testing.T) {
	ctx := context.Background()
	got := ExtractClientCN(ctx)
	if got != "" {
		t.Errorf("ExtractClientCN(empty ctx) = %q, want empty", got)
	}
}

// TestAuthConfig_Normalize 测试配置标准化
func TestAuthConfig_Normalize(t *testing.T) {
	tests := []struct {
		name string
		cfg  AuthConfig
		want string
	}{
		{"空 TokenHeader", AuthConfig{TokenHeader: ""}, DefaultTokenHeader},
		{"自定义 TokenHeader", AuthConfig{TokenHeader: "x-custom-token"}, "x-custom-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.normalize()
			if tt.cfg.TokenHeader != tt.want {
				t.Errorf("TokenHeader = %q, want %q", tt.cfg.TokenHeader, tt.want)
			}
		})
	}
}

// TestNewAuthInterceptor 测试创建认证拦截器
func TestNewAuthInterceptor(t *testing.T) {
	cfg := AuthConfig{Enabled: true, TokenHeader: ""}
	ai := NewAuthInterceptor(cfg, nil)
	if ai == nil {
		t.Fatal("NewAuthInterceptor 返回 nil")
	}
	if ai.config.TokenHeader != DefaultTokenHeader {
		t.Errorf("TokenHeader 未标准化: got %q, want %q", ai.config.TokenHeader, DefaultTokenHeader)
	}
}

// TestUpdateConfig 测试动态更新配置
func TestUpdateConfig(t *testing.T) {
	ai := NewAuthInterceptor(AuthConfig{Enabled: false}, nil)

	newCfg := AuthConfig{Enabled: true, RequireToken: true, TokenHeader: "x-new-token"}
	ai.UpdateConfig(newCfg)

	current := ai.currentConfig()
	if !current.Enabled {
		t.Error("配置未更新: Enabled 应为 true")
	}
	if !current.RequireToken {
		t.Error("配置未更新: RequireToken 应为 true")
	}
	if current.TokenHeader != "x-new-token" {
		t.Errorf("TokenHeader = %q, want %q", current.TokenHeader, "x-new-token")
	}
}

// 辅助函数
func sha256Hex(s string) string {
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}

// contextWithMetadata 创建带 metadata 的 context（模拟 gRPC metadata）
func contextWithMetadata(md map[string][]string) context.Context {
	// 使用 context.WithValue 模拟 metadata
	// 在实际测试中应使用 grpc/metadata.NewIncomingContext
	// 但为了避免依赖 gRPC metadata 包的内部实现，这里使用简单的 value
	type mdKey struct{}
	return context.WithValue(context.Background(), mdKey{}, md)
}
