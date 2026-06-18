module github.com/meinanzilinzhengying/cloudflow/agent

go 1.24.0

require (
	github.com/cilium/ebpf v0.11.0
	github.com/fsnotify/fsnotify v1.9.0
	github.com/meinanzilinzhengying/cloudflow/pkg v0.0.0
	github.com/meinanzilinzhengying/cloudflow/proto v0.0.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.1
	github.com/sirupsen/logrus v1.9.4
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/vishvananda/netlink v1.3.1
	go.uber.org/zap v1.28.0
	golang.org/x/sys v0.45.0
	google.golang.org/grpc v1.71.0
)

replace (
	github.com/meinanzilinzhengying/cloudflow/pkg => ../pkg
	github.com/meinanzilinzhengying/cloudflow/proto => ../proto
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	go.opentelemetry.io/otel v1.41.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.39.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace golang.org/x/sys => golang.org/x/sys v0.18.0


replace golang.org/x/net => golang.org/x/net v0.24.0

replace google.golang.org/genproto/googleapis/rpc => google.golang.org/genproto/googleapis/rpc v0.0.0-20240227224415-6ceb2ff114de

replace github.com/prometheus/procfs => github.com/prometheus/procfs v0.12.0

replace golang.org/x/text => golang.org/x/text v0.14.0
