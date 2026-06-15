module github.com/meinanzilinzhengying/ebpf-probe

go 1.22

require (
	github.com/cilium/ebpf v0.16.0
	golang.org/x/net v0.33.0
)

replace golang.org/x/sys => golang.org/x/sys v0.18.0

replace github.com/prometheus/common => github.com/prometheus/common v0.48.0
