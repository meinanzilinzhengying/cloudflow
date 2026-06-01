module cloud-flow/services/alert-engine

go 1.22

require (
	cloud-flow/proto v0.0.0
	github.com/go-sql-driver/mysql v1.8.1
	google.golang.org/grpc v1.80.0
	google.golang.org/grpc/health v0.0.0
	google.golang.org/grpc/health/grpc_health_v1 v0.0.0
)

replace (
	cloud-flow/proto => ../../proto
	cloud-flow/services/shared/auth => ../shared/auth
)
