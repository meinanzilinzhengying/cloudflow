module cloud-flow/services/control-plane

go 1.22

require (
	cloud-flow/pkg v0.0.0
	cloud-flow/services/proto v0.0.0
	go.etcd.io/etcd/client/v3 v3.5.13
	google.golang.org/grpc v1.80.0
	google.golang.org/grpc/credentials v0.0.0
	google.golang.org/grpc/credentials/insecure v0.0.0
	google.golang.org/grpc/health v0.0.0
	google.golang.org/grpc/health/grpc_health_v1 v0.0.0
)

replace cloud-flow/pkg => ../../../pkg
replace cloud-flow/services/proto => ../../../proto
