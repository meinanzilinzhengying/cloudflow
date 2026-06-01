// Package alertengine gRPC 服务实现
package alertengine

import (
	"context"

	"google.golang.org/grpc"

	svcproto "cloud-flow/services/proto"
)

func RegisterAlertService(s *grpc.Server, svc *Service) {
	svcproto.RegisterAlertServiceServer(s, &alertGRPC{svc: svc})
}

type alertGRPC struct {
	svcproto.UnimplementedAlertServiceServer
	svc *Service
}

func (g *alertGRPC) HealthCheck(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.HealthCheckResponse, error) {
	return &svcproto.HealthCheckResponse{Healthy: true, Version: g.svc.config.Version}, nil
}
func (g *alertGRPC) CreateRule(ctx context.Context, req *svcproto.AlertRule) (*svcproto.AlertRule, error) {
	return g.svc.CreateRule(req)
}
func (g *alertGRPC) GetRule(ctx context.Context, req *svcproto.AlertRule) (*svcproto.AlertRule, error) {
	return g.svc.GetRule(req)
}
func (g *alertGRPC) ListRules(ctx context.Context, req *svcproto.ListTenantsRequest) ([]*svcproto.AlertRule, error) {
	return g.svc.ListRules()
}
func (g *alertGRPC) DeleteRule(ctx context.Context, req *svcproto.AlertRule) (*svcproto.AlertRule, error) {
	return g.svc.DeleteRule(req)
}
func (g *alertGRPC) ListAlerts(ctx context.Context, req *svcproto.ListTenantsRequest) ([]*svcproto.Alert, error) {
	return g.svc.ListAlerts()
}
func (g *alertGRPC) EvaluateAlerts(ctx context.Context, req *svcproto.EvaluateAlertsRequest) (*svcproto.EvaluateAlertsResponse, error) {
	return g.svc.EvaluateAlerts(ctx, req)
}
