// Package dataplane gRPC 服务实现
package dataplane

import (
	"context"

	"google.golang.org/grpc"

	svcproto "cloud-flow/services/proto"
)

// RegisterDataPlaneService 注册 gRPC 服务
func RegisterDataPlaneService(s *grpc.Server, svc *Service) {
	svcproto.RegisterDataPlaneServiceServer(s, &dataPlaneGRPC{svc: svc})
}

type dataPlaneGRPC struct {
	svcproto.UnimplementedDataPlaneServiceServer
	svc *Service
}

func (g *dataPlaneGRPC) HealthCheck(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.HealthCheckResponse, error) {
	return &svcproto.HealthCheckResponse{
		Healthy: true,
		Version: g.svc.config.Version,
	}, nil
}

func (g *dataPlaneGRPC) IngestFlows(ctx context.Context, req *svcproto.FlowBatch) (*svcproto.IngestResponse, error) {
	return g.svc.IngestFlow(ctx, req)
}

func (g *dataPlaneGRPC) IngestMetrics(ctx context.Context, req *svcproto.FlowBatch) (*svcproto.IngestResponse, error) {
	// TODO: 转发到 VictoriaMetrics
	return &svcproto.IngestResponse{Accepted: 0, Success: true}, nil
}

func (g *dataPlaneGRPC) ApplyConfig(ctx context.Context, req *svcproto.UpdateIngestConfigRequest) (*svcproto.UpdateIngestConfigResponse, error) {
	return &svcproto.UpdateIngestConfigResponse{Success: true, Message: "config applied"}, nil
}
