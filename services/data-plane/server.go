// Package dataplane gRPC 服务实现
package dataplane

import (
	"context"

	"github.com/meinanzilinzhengying/cloudflow/pkg/flow"
	"google.golang.org/grpc"

	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
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
	// 将 FlowBatch 转换为 UnifiedFlow 列表并转发到 VictoriaMetrics
	var flows []*flow.UnifiedFlow
	for _, flowMap := range req.Flows {
		f := mapToUnifiedFlow(flowMap)
		if f != nil {
			flows = append(flows, f)
		}
	}

	// 写入 VictoriaMetrics
	if err := g.svc.WriteToVictoriaMetrics(flows); err != nil {
		return &svcproto.IngestResponse{Accepted: 0, Success: false}, err
	}

	return &svcproto.IngestResponse{Accepted: len(flows), Success: true}, nil
}

func (g *dataPlaneGRPC) ApplyConfig(ctx context.Context, req *svcproto.UpdateIngestConfigRequest) (*svcproto.UpdateIngestConfigResponse, error) {
	return &svcproto.UpdateIngestConfigResponse{Success: true, Message: "config applied"}, nil
}
