package grpcutil

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc/metadata"
)

// WithAuth 将 API Key 注入 gRPC metadata
func WithAuth(ctx context.Context, apiKey string) context.Context {
	md := metadata.Pairs("x-api-key", apiKey)
	return metadata.NewOutgoingContext(ctx, md)
}

// CheckAPIKey 从 context 中提取并校验 API Key
// 使用恒定时间比较，防止时序攻击。
// FIX: 当 expectedAPIKey 为空时，返回 false 而非 true，
// 防止 API Key 未配置时所有请求绕过认证。
func CheckAPIKey(ctx context.Context, expectedAPIKey string) bool {
	if expectedAPIKey == "" {
		return false
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	vals := md.Get("x-api-key")
	if len(vals) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(vals[0]), []byte(expectedAPIKey)) == 1
}
