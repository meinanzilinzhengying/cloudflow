module github.com/meinanzilinzhengying/cloudflow/agent

go 1.22

require (
	github.com/cilium/ebpf v0.11.0
	github.com/meinanzilinzhengying/cloudflow/pkg v0.0.0
	github.com/meinanzilinzhengying/cloudflow/proto v0.0.0
	github.com/prometheus/client_golang v1.19.1
	github.com/prometheus/client_model v0.5.0
	github.com/spf13/viper v1.16.0
	github.com/stretchr/testify v1.11.1
	github.com/vishvananda/netlink v1.3.1
	go.uber.org/zap v1.27.0
	golang.org/x/sys v0.18.0
	google.golang.org/grpc v1.62.1
)

replace (
	github.com/meinanzilinzhengying/cloudflow/pkg => ../pkg
	github.com/meinanzilinzhengying/cloudflow/proto => ../proto
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pelletier/go-toml/v2 v2.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/exp v0.0.0-20230905200255-921286631fa9 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace golang.org/x/sys => golang.org/x/sys v0.18.0
