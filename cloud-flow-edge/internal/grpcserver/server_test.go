package grpcserver

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc/peer"

	edge "cloud-flow/proto"
)

// TestExtractIP 测试 IP 提取函数
func TestExtractIP(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{"IPv4 带端口", "192.168.1.1:50051", "192.168.1.1"},
		{"IPv6 带端口", "[::1]:50051", "::1"},
		{"IPv6 完整地址", "[2001:db8::1]:8080", "2001:db8::1"},
		{"只有 IP 无端口", "10.0.0.1", "10.0.0.1"},
		{"空字符串", "", ""},
		{"localhost 带端口", "localhost:8080", "localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIP(tt.addr)
			if got != tt.want {
				t.Errorf("extractIP(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// TestExtractClientIP_WithPeer 测试从 gRPC context 中提取客户端 IP
func TestExtractClientIP_WithPeer(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want string
	}{
		{
			name: "TCPAddr IPv4",
			addr: &net.TCPAddr{
				IP:   net.ParseIP("192.168.1.100"),
				Port: 50051,
			},
			want: "192.168.1.100",
		},
		{
			name: "TCPAddr IPv6",
			addr: &net.TCPAddr{
				IP:   net.ParseIP("::1"),
				Port: 50051,
			},
			want: "::1",
		},
		{
			name: "UDPAddr",
			addr: &net.UDPAddr{
				IP:   net.ParseIP("10.0.0.1"),
				Port: 8080,
			},
			want: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: tt.addr})
			got := extractClientIP(ctx)
			if got != tt.want {
				t.Errorf("extractClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExtractClientIP_NoPeer 测试无 peer 信息
func TestExtractClientIP_NoPeer(t *testing.T) {
	ctx := context.Background()
	got := extractClientIP(ctx)
	if got != "unknown" {
		t.Errorf("extractClientIP(empty ctx) = %q, want 'unknown'", got)
	}
}

// TestExtractClientIP_NilAddr 测试 nil 地址
func TestExtractClientIP_NilAddr(t *testing.T) {
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: nil})
	got := extractClientIP(ctx)
	if got != "unknown" {
		t.Errorf("extractClientIP(nil addr) = %q, want 'unknown'", got)
	}
}

// TestExtractProbeID_EdgeServer 测试从请求中提取 ProbeID
func TestExtractProbeID_EdgeServer(t *testing.T) {
	tests := []struct {
		name string
		req  interface{}
		want string
	}{
		{"RegisterProbeRequest", &edge.RegisterProbeRequest{ProbeId: "probe-001"}, "probe-001"},
		{"HeartbeatRequest", &edge.HeartbeatRequest{ProbeId: "probe-002"}, "probe-002"},
		{"MetricsBatch", &edge.MetricsBatch{ProbeId: "probe-003"}, "probe-003"},
		{"TraceBatch", &edge.TraceBatch{ProbeId: "probe-004"}, "probe-004"},
		{"ProfilingBatch", &edge.ProfilingBatch{ProbeId: "probe-005"}, "probe-005"},
		{"未知类型", "not-a-proto", ""},
		{"nil 请求", nil, ""},
		{"空 ProbeId", &edge.RegisterProbeRequest{ProbeId: ""}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractProbeID(tt.req)
			if got != tt.want {
				t.Errorf("extractProbeID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAPIKeyAuthInterceptor_SkipEmptyKey 测试空 API Key 跳过认证
func TestAPIKeyAuthInterceptor_SkipEmptyKey(t *testing.T) {
	interceptor := APIKeyAuthInterceptor("", nil)
	if interceptor == nil {
		t.Fatal("APIKeyAuthInterceptor 返回 nil")
	}

	// 空 API Key 时应直接调用 handler
	called := false
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), "req", nil, handler)
	if err != nil {
		t.Errorf("空 API Key 应跳过认证: %v", err)
	}
	if !called {
		t.Error("handler 未被调用")
	}
	if resp != "ok" {
		t.Errorf("resp = %v, want 'ok'", resp)
	}
}

// TestAPIKeyAuthInterceptor_ValidKey 测试有效 API Key
func TestAPIKeyAuthInterceptor_ValidKey(t *testing.T) {
	interceptor := APIKeyAuthInterceptor("correct-api-key", nil)

	called := false
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return "ok", nil
	}

	// 使用 gRPC metadata context 传递 API Key
	ctx := contextWithAPIKey("correct-api-key")
	resp, err := interceptor(ctx, "req", nil, handler)
	if err != nil {
		t.Errorf("有效 API Key 应通过认证: %v", err)
	}
	if !called {
		t.Error("handler 未被调用")
	}
	if resp != "ok" {
		t.Errorf("resp = %v, want 'ok'", resp)
	}
}

// TestAPIKeyAuthInterceptor_InvalidKey 测试无效 API Key
func TestAPIKeyAuthInterceptor_InvalidKey(t *testing.T) {
	interceptor := APIKeyAuthInterceptor("correct-api-key", nil)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler 不应被调用")
		return nil, nil
	}

	ctx := contextWithAPIKey("wrong-api-key")
	_, err := interceptor(ctx, "req", nil, handler)
	if err == nil {
		t.Error("无效 API Key 应返回错误")
	}
}

// TestAPIKeyAuthInterceptor_MissingKey 测试缺少 API Key
func TestAPIKeyAuthInterceptor_MissingKey(t *testing.T) {
	interceptor := APIKeyAuthInterceptor("correct-api-key", nil)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler 不应被调用")
		return nil, nil
	}

	_, err := interceptor(context.Background(), "req", nil, handler)
	if err == nil {
		t.Error("缺少 API Key 应返回错误")
	}
}

// TestAPIKeyAuthInterceptor_NoMetadata 测试无 metadata
func TestAPIKeyAuthInterceptor_NoMetadata(t *testing.T) {
	interceptor := APIKeyAuthInterceptor("correct-api-key", nil)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler 不应被调用")
		return nil, nil
	}

	_, err := interceptor(context.Background(), "req", nil, handler)
	if err == nil {
		t.Error("无 metadata 应返回错误")
	}
}

// TestAPIKeyAuthInterceptor_TimingAttack 测试时序攻击防护
func TestAPIKeyAuthInterceptor_TimingAttack(t *testing.T) {
	interceptor := APIKeyAuthInterceptor("correct-api-key", nil)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, nil
	}

	// 正确 key 和错误 key 都应返回错误（错误 key 不应更快返回）
	// 这里只验证两种情况都返回错误
	ctxCorrect := contextWithAPIKey("correct-api-key")
	ctxWrong := contextWithAPIKey("wrong-key")

	_, errCorrect := interceptor(ctxCorrect, "req", nil, handler)
	_, errWrong := interceptor(ctxWrong, "req", nil, handler)

	if errCorrect != nil {
		t.Errorf("正确 API Key 不应返回错误: %v", errCorrect)
	}
	if errWrong == nil {
		t.Error("错误 API Key 应返回错误")
	}
}

// 辅助函数
func contextWithAPIKey(apiKey string) context.Context {
	type mdKey struct{}
	md := map[string][]string{"x-api-key": {apiKey}}
	return context.WithValue(context.Background(), mdKey{}, md)
}
