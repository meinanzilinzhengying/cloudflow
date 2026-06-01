// Package controlplane gRPC 服务实现
package controlplane

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	svcproto "cloud-flow/services/proto"
)

// ============================================================================
// gRPC 服务注册
// ============================================================================

// RegisterControlPlaneService 注册 gRPC 服务
func RegisterControlPlaneService(s *grpc.Server, svc *Service) {
	svcproto.RegisterControlPlaneServiceServer(s, &controlPlaneGRPC{svc: svc})
}

// controlPlaneGRPC gRPC 服务实现
type controlPlaneGRPC struct {
	svcproto.UnimplementedControlPlaneServiceServer
	svc *Service
}

// HealthCheck 健康检查
func (g *controlPlaneGRPC) HealthCheck(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.HealthCheckResponse, error) {
	return &svcproto.HealthCheckResponse{
		Healthy: true,
		Version: g.svc.config.Version,
		Uptime:  int64(g.svc.startTime.Unix()),
	}, nil
}

// ListAgents 列出 Agent
func (g *controlPlaneGRPC) ListAgents(ctx context.Context, req *svcproto.ListAgentsRequest) (*svcproto.ListAgentsResponse, error) {
	agents := g.svc.ListAgents(req.TenantId, req.Region, req.Status)
	return &svcproto.ListAgentsResponse{
		Agents: agents,
		Total:  len(agents),
	}, nil
}

// GetAgent 获取 Agent
func (g *controlPlaneGRPC) GetAgent(ctx context.Context, req *svcproto.AgentInfo) (*svcproto.AgentInfo, error) {
	agent, ok := g.svc.GetAgent(req.AgentId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "agent %s not found", req.AgentId)
	}
	return agent, nil
}

// ListEdges 列出 Edge
func (g *controlPlaneGRPC) ListEdges(ctx context.Context, req *svcproto.ListEdgesRequest) (*svcproto.ListEdgesResponse, error) {
	edges := g.svc.ListEdges(req.TenantId, req.Region, req.Status)
	return &svcproto.ListEdgesResponse{
		Edges: edges,
		Total:  len(edges),
	}, nil
}

// GetEdge 获取 Edge
func (g *controlPlaneGRPC) GetEdge(ctx context.Context, req *svcproto.EdgeInfo) (*svcproto.EdgeInfo, error) {
	edge, ok := g.svc.GetEdge(req.EdgeId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "edge %s not found", req.EdgeId)
	}
	return edge, nil
}

// UpdateIngestConfig 更新数据面配置
func (g *controlPlaneGRPC) UpdateIngestConfig(ctx context.Context, req *svcproto.UpdateIngestConfigRequest) (*svcproto.UpdateIngestConfigResponse, error) {
	// TODO: 通过 gRPC 调用 data-plane 的 ApplyConfig
	return &svcproto.UpdateIngestConfigResponse{
		Success: true,
		Message: "config updated",
	}, nil
}
