module github.com/meinanzilinzhengying/cloudflow/proto

go 1.25.0

require (
	github.com/golang/protobuf v1.5.4
	google.golang.org/grpc v1.71.0
)

require (
	go.opentelemetry.io/otel v1.41.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.39.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	github.com/meinanzilinzhengying/cloudflow/services/alert-engine => ../services/alert-engine
	github.com/meinanzilinzhengying/cloudflow/services/auth-service => ../services/auth-service
	github.com/meinanzilinzhengying/cloudflow/services/control-plane => ../services/control-plane
	github.com/meinanzilinzhengying/cloudflow/services/data-plane => ../services/data-plane
	github.com/meinanzilinzhengying/cloudflow/services/proto => ../services/proto
	github.com/meinanzilinzhengying/cloudflow/services/query-service => ../services/query-service
	github.com/meinanzilinzhengying/cloudflow/services/shared => ../services/shared
	github.com/meinanzilinzhengying/cloudflow/services/shared/auth => ../services/shared/auth
	github.com/meinanzilinzhengying/cloudflow/services/shared/resilience => ../services/shared/resilience
	github.com/meinanzilinzhengying/cloudflow/services/tenant-service => ../services/tenant-service
	github.com/meinanzilinzhengying/cloudflow/services/topology-engine => ../services/topology-engine
)

replace golang.org/x/sys => golang.org/x/sys v0.18.0

replace github.com/prometheus/common => github.com/prometheus/common v0.48.0
