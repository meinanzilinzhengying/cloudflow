package ratelimit

import (
	"context"
	"net/http"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// HTTPMiddleware HTTP 速率限制中间件
func HTTPMiddleware(limiter *Limiter, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			key := keyFunc(r)
			
			info, err := limiter.AllowWithInfo(ctx, key)
			if err != nil {
				// 记录错误但不过度限流
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			
			// 设置限流响应头
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(info.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(info.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(info.ResetAt.Unix(), 10))
			
			if !info.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(info.ResetAt.Sub(info.ResetAt.Add(-info.ResetAt.Sub(info.ResetAt)).Seconds()))))
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// GRPCUnaryInterceptor gRPC 一元拦截器
func GRPCUnaryInterceptor(limiter *Limiter, keyFunc func(context.Context, string) string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		key := keyFunc(ctx, info.FullMethod)
		
		allowed, err := limiter.Allow(ctx, key)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "rate limiter error: %v", err)
		}
		
		if !allowed {
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
		}
		
		return handler(ctx, req)
	}
}

// GRPCStreamInterceptor gRPC 流式拦截器
func GRPCStreamInterceptor(limiter *Limiter, keyFunc func(context.Context, string) string) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := stream.Context()
		key := keyFunc(ctx, info.FullMethod)
		
		allowed, err := limiter.Allow(ctx, key)
		if err != nil {
			return status.Errorf(codes.Internal, "rate limiter error: %v", err)
		}
		
		if !allowed {
			return status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
		}
		
		return handler(srv, stream)
	}
}

// DefaultHTTPKeyFunc 默认 HTTP key 提取函数（基于 IP）
func DefaultHTTPKeyFunc() func(*http.Request) string {
	return func(r *http.Request) string {
		// 优先使用 X-Forwarded-For
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			return "http:" + forwarded
		}
		
		// 使用 RemoteAddr
		return "http:" + r.RemoteAddr
	}
}

// DefaultGRPCKeyFunc 默认 gRPC key 提取函数（基于 metadata）
func DefaultGRPCKeyFunc() func(context.Context, string) string {
	return func(ctx context.Context, method string) string {
		// 从 metadata 获取 client ID
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if clientID := md.Get("x-client-id"); len(clientID) > 0 {
				return "grpc:" + clientID[0] + ":" + method
			}
		}
		
		// 使用方法名作为 key
		return "grpc:" + method
	}
}

// PerEndpointRules 为不同 endpoint 配置不同限流规则
func PerEndpointRules(limiter *Limiter, rules map[string]Rule) {
	for pattern, rule := range rules {
		limiter.config.CustomRules[pattern] = rule
	}
}
