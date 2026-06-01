// Package grpcserver 提供 gRPC 服务端实现
// 实现健康检查协议
package grpcserver

import (
	"context"

	"google.golang.org/grpc/health/grpc_health_v1"
)

// HealthChecker 健康检查实现
type HealthChecker struct {
	grpc_health_v1.UnimplementedHealthServer
	server *Server
}

// NewHealthChecker 创建健康检查服务
func NewHealthChecker(server *Server) *HealthChecker {
	return &HealthChecker{
		server: server,
	}
}

// Check 实现健康检查方法
func (h *HealthChecker) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	// 检查服务状态
	status := grpc_health_v1.HealthCheckResponse_SERVING

	// 可以添加更详细的健康检查逻辑
	// 例如检查数据库连接、外部服务依赖等

	return &grpc_health_v1.HealthCheckResponse{
		Status: status,
	}, nil
}

// Watch 实现健康检查 watch 方法
func (h *HealthChecker) Watch(req *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	// 简单实现，直接发送当前状态
	status := grpc_health_v1.HealthCheckResponse_SERVING
	if err := stream.Send(&grpc_health_v1.HealthCheckResponse{
		Status: status,
	}); err != nil {
		return err
	}

	// 保持流打开
	<-stream.Context().Done()
	return nil
}
