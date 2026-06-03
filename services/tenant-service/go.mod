module cloudflow/services/tenant-service

go 1.22

require (
	cloudflow/pkg v0.0.0
	cloudflow/services/proto v0.0.0
	cloudflow/services/shared/auth v0.0.0
	github.com/go-sql-driver/mysql v1.8.1
	google.golang.org/grpc v1.80.0
	google.golang.org/grpc/health v0.0.0
	google.golang.org/grpc/health/grpc_health_v1 v0.0.0
)

replace (
	cloudflow/pkg => ../../pkg
	cloudflow/services/proto => ../../services/proto
	cloudflow/services/shared/auth => ../shared/auth
)
