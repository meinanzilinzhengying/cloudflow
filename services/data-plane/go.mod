module cloud-flow/services/data-plane

go 1.22

require (
	cloud-flow/pkg v0.0.0
	cloud-flow/services/proto v0.0.0
	github.com/ClickHouse/clickhouse-go/v2 v2.24.2
	google.golang.org/grpc v1.80.0
	google.golang.org/grpc/health v0.0.0
	google.golang.org/grpc/health/grpc_health_v1 v0.0.0
)

replace (
	cloud-flow/pkg => ../../../pkg
	cloud-flow/services/proto => ../../../proto
	cloud-flow/services/shared/auth => ../shared/auth
)
