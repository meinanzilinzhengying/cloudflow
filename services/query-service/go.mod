module github.com/meinanzilinzhengying/cloudflow/services/query-service

go 1.22

require (
	github.com/ClickHouse/clickhouse-go/v2 v2.46.0
	github.com/meinanzilinzhengying/cloudflow/pkg v0.0.0
	github.com/meinanzilinzhengying/cloudflow/services/proto v0.0.0
	google.golang.org/grpc v1.80.0
)

replace github.com/meinanzilinzhengying/cloudflow/services/proto => ../../services/proto

replace github.com/meinanzilinzhengying/cloudflow/services/shared => ../../services/shared

replace github.com/meinanzilinzhengying/cloudflow/services/shared/auth => ../../services/shared/auth

replace github.com/meinanzilinzhengying/cloudflow/services/shared/resilience => ../../services/shared/resilience

replace github.com/meinanzilinzhengying/cloudflow/pkg => ../../pkg
