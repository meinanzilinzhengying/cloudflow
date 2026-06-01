module cloud-flow/services/tenant-service

go 1.22

require (
	cloud-flow/pkg v0.0.0
	cloud-flow/services/proto v0.0.0
	cloud-flow/services/shared/auth v0.0.0
	github.com/go-sql-driver/mysql v1.8.1
	google.golang.org/grpc v1.80.0
	google.golang.org/grpc/health v0.0.0
	google.golang.org/grpc/health/grpc_health_v1 v0.0.0
)

replace (
	cloud-flow/pkg => ../../pkg
	cloud-flow/services/proto => ../../services/proto
	cloud-flow/services/shared/auth => ../shared/auth
)
