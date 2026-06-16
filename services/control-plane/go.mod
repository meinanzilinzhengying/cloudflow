module github.com/meinanzilinzhengying/cloudflow/services/control-plane

go 1.22

require (
	github.com/meinanzilinzhengying/cloudflow/services/proto v0.0.0
	github.com/prometheus/client_golang v1.21.1
	github.com/shirou/gopsutil/v3 v3.24.5
	go.etcd.io/etcd/client/v3 v3.5.13
	google.golang.org/grpc v1.62.1
)

require (
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	go.etcd.io/etcd/api/v3 v3.5.13 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.13 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.17.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)

replace github.com/prometheus/common => github.com/prometheus/common v0.48.0
replace golang.org/x/sys => golang.org/x/sys v0.18.0
