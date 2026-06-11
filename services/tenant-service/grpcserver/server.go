// Package tenantservice gRPC 服务实现
package tenantservice

import (
	"context"

	"google.golang.org/grpc"

	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
)

// RegisterTenantService 注册 gRPC 服务
func RegisterTenantService(s *grpc.Server, svc *Service) {
	svcproto.RegisterTenantServiceServer(s, &tenantGRPC{svc: svc})
}

type tenantGRPC struct {
	svcproto.UnimplementedTenantServiceServer
	svc *Service
}

func (g *tenantGRPC) HealthCheck(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.HealthCheckResponse, error) {
	return &svcproto.HealthCheckResponse{Healthy: true, Version: g.svc.config.Version}, nil
}

func (g *tenantGRPC) CreateTenant(ctx context.Context, req *svcproto.CreateTenantRequest) (*svcproto.CreateTenantResponse, error) {
	return g.svc.CreateTenant(ctx, req)
}

func (g *tenantGRPC) GetTenant(ctx context.Context, req *svcproto.GetTenantRequest) (*svcproto.GetTenantResponse, error) {
	return g.svc.GetTenant(ctx, req)
}

func (g *tenantGRPC) UpdateTenant(ctx context.Context, req *svcproto.UpdateTenantRequest) (*svcproto.UpdateTenantResponse, error) {
	return g.svc.UpdateTenant(ctx, req)
}

func (g *tenantGRPC) DeleteTenant(ctx context.Context, req *svcproto.DeleteTenantRequest) (*svcproto.DeleteTenantResponse, error) {
	return g.svc.DeleteTenant(ctx, req)
}

func (g *tenantGRPC) ListTenants(ctx context.Context, req *svcproto.ListTenantsRequest) (*svcproto.ListTenantsResponse, error) {
	return g.svc.ListTenants(ctx, req)
}

func (g *tenantGRPC) UpdateQuota(ctx context.Context, req *svcproto.UpdateTenantQuotaRequest) (*svcproto.UpdateTenantQuotaResponse, error) {
	return g.svc.UpdateQuota(ctx, req)
}

func (g *tenantGRPC) CreateProject(ctx context.Context, req *svcproto.CreateProjectRequest) (*svcproto.CreateProjectResponse, error) {
	return g.svc.CreateProject(ctx, req)
}

func (g *tenantGRPC) GetProject(ctx context.Context, req *svcproto.GetProjectRequest) (*svcproto.GetProjectResponse, error) {
	return g.svc.GetProject(ctx, req)
}

func (g *tenantGRPC) ListProjects(ctx context.Context, req *svcproto.ListProjectsRequest) (*svcproto.ListProjectsResponse, error) {
	return g.svc.ListProjects(ctx, req)
}

func (g *tenantGRPC) AddTenantMember(ctx context.Context, req *svcproto.AddTenantMemberRequest) (*svcproto.AddTenantMemberResponse, error) {
	return g.svc.AddTenantMember(ctx, req)
}

func (g *tenantGRPC) RemoveTenantMember(ctx context.Context, req *svcproto.RemoveTenantMemberRequest) (*svcproto.RemoveTenantMemberResponse, error) {
	return g.svc.RemoveTenantMember(ctx, req)
}

func (g *tenantGRPC) ListTenantMembers(ctx context.Context, req *svcproto.ListTenantMembersRequest) (*svcproto.ListTenantMembersResponse, error) {
	return g.svc.ListTenantMembers(ctx, req)
}
