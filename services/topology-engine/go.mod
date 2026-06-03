module cloudflow/services/topology-engine

go 1.22

require (
	cloudflow/pkg v0.0.0
	cloudflow/services/proto v0.0.0
	github.com/ClickHouse/clickhouse-go/v2 v2.24.2
	github.com/redis/go-redis/v9 v9.7.0
	google.golang.org/grpc v1.80.0
	google.golang.org/grpc/health v0.0.0
	google.golang.org/grpc/health/grpc_health_v1 v0.0.0
)

replace cloudflow/pkg => ../../../pkg
replace cloudflow/services/proto => ../../../services/proto
