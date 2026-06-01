module cloud-flow/services/query-service

go 1.22

require (
	cloud-flow/services/proto v0.0.0
	github.com/ClickHouse/clickhouse-go/v2 v2.24.2
	google.golang.org/grpc v1.80.0
	google.golang.org/grpc/health v0.0.0
	google.golang.org/grpc/health/grpc_health_v1 v0.0.0
	google.golang.org/protobuf v1.36.11
)

replace cloud-flow/services/proto => ../../../proto
