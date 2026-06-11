// Package alertengine gRPC 服务实现
package alertengine

import (
	"context"

	"google.golang.org/grpc"

	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
)

// RegisterAlertService 注册 gRPC 服务
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

func (g *alertGRPC) CreateRule(ctx context.Context, req *svcproto.CreateAlertRuleRequest) (*svcproto.CreateAlertRuleResponse, error) {
	return g.svc.CreateRule(ctx, req)
}

func (g *alertGRPC) GetRule(ctx context.Context, req *svcproto.GetAlertRuleRequest) (*svcproto.GetAlertRuleResponse, error) {
	return g.svc.GetRule(ctx, req)
}

func (g *alertGRPC) ListRules(ctx context.Context, req *svcproto.ListAlertRulesRequest) (*svcproto.ListAlertRulesResponse, error) {
	return g.svc.ListRules(ctx, req)
}

func (g *alertGRPC) UpdateRule(ctx context.Context, req *svcproto.UpdateAlertRuleRequest) (*svcproto.UpdateAlertRuleResponse, error) {
	return g.svc.UpdateRule(ctx, req)
}

func (g *alertGRPC) DeleteRule(ctx context.Context, req *svcproto.DeleteAlertRuleRequest) (*svcproto.DeleteAlertRuleResponse, error) {
	return g.svc.DeleteRule(ctx, req)
}

func (g *alertGRPC) CreateAlert(ctx context.Context, req *svcproto.CreateAlertRequest) (*svcproto.CreateAlertResponse, error) {
	return g.svc.CreateAlert(ctx, req)
}

func (g *alertGRPC) GetAlert(ctx context.Context, req *svcproto.GetAlertRequest) (*svcproto.GetAlertResponse, error) {
	return g.svc.GetAlert(ctx, req)
}

func (g *alertGRPC) UpdateAlert(ctx context.Context, req *svcproto.UpdateAlertRequest) (*svcproto.UpdateAlertResponse, error) {
	return g.svc.UpdateAlert(ctx, req)
}

func (g *alertGRPC) ListAlerts(ctx context.Context, req *svcproto.ListAlertsRequest) (*svcproto.ListAlertsResponse, error) {
	return g.svc.ListAlerts(ctx, req)
}

func (g *alertGRPC) CreateNotification(ctx context.Context, req *svcproto.CreateNotificationRequest) (*svcproto.CreateNotificationResponse, error) {
	return g.svc.CreateNotification(ctx, req)
}

func (g *alertGRPC) UpdateNotification(ctx context.Context, req *svcproto.UpdateNotificationRequest) (*svcproto.UpdateNotificationResponse, error) {
	return g.svc.UpdateNotification(ctx, req)
}

func (g *alertGRPC) ListNotifications(ctx context.Context, req *svcproto.ListNotificationsRequest) (*svcproto.ListNotificationsResponse, error) {
	return g.svc.ListNotifications(ctx, req)
}

func (g *alertGRPC) EvaluateRules(ctx context.Context, req *svcproto.EvaluateRulesRequest) (*svcproto.EvaluateRulesResponse, error) {
	return g.svc.EvaluateRules(ctx, req)
}

func (g *alertGRPC) EvaluateAlerts(ctx context.Context, req *svcproto.EvaluateAlertsRequest) (*svcproto.EvaluateAlertsResponse, error) {
	return g.svc.EvaluateAlerts(ctx, req)
}
