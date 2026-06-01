// Package authservice gRPC 服务实现
package authservice

import (
	"context"

	"google.golang.org/grpc"

	svcproto "cloud-flow/services/proto"
)

// RegisterAuthService 注册 gRPC 服务
func RegisterAuthService(s *grpc.Server, svc *Service) {
	svcproto.RegisterAuthServiceServer(s, &authGRPC{svc: svc})
}

type authGRPC struct {
	svcproto.UnimplementedAuthServiceServer
	svc *Service
}

func (g *authGRPC) HealthCheck(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.HealthCheckResponse, error) {
	return &svcproto.HealthCheckResponse{Healthy: true, Version: g.svc.config.Version}, nil
}

func (g *authGRPC) Authenticate(ctx context.Context, req *svcproto.AuthenticateRequest) (*svcproto.AuthenticateResponse, error) {
	return g.svc.Authenticate(ctx, req)
}

func (g *authGRPC) ValidateToken(ctx context.Context, req *svcproto.ValidateTokenRequest) (*svcproto.ValidateTokenResponse, error) {
	return g.svc.ValidateToken(ctx, req)
}

func (g *authGRPC) Authorize(ctx context.Context, req *svcproto.AuthorizeRequest) (*svcproto.AuthorizeResponse, error) {
	return g.svc.Authorize(ctx, req)
}

func (g *authGRPC) CreateRole(ctx context.Context, req *svcproto.CreateRoleRequest) (*svcproto.CreateRoleResponse, error) {
	return g.svc.CreateRole(ctx, req)
}

func (g *authGRPC) BindUserRole(ctx context.Context, req *svcproto.BindUserRoleRequest) (*svcproto.BindUserRoleResponse, error) {
	return g.svc.BindUserRole(ctx, req)
}

func (g *authGRPC) CreatePolicy(ctx context.Context, req *svcproto.CreatePolicyRequest) (*svcproto.CreatePolicyResponse, error) {
	return g.svc.CreatePolicy(ctx, req)
}

func (g *authGRPC) CheckPermission(ctx context.Context, req *svcproto.CheckPermissionRequest) (*svcproto.CheckPermissionResponse, error) {
	return g.svc.CheckPermission(ctx, req)
}

func (g *authGRPC) OIDCCallback(ctx context.Context, req *svcproto.OIDCCallbackRequest) (*svcproto.AuthenticateResponse, error) {
	return g.svc.OIDCCallback(ctx, req)
}
