package bpf

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/cilium/ebpf"
)

type networkMap struct {
	NetworkMap *ebpf.Map `ebpf:"network_map"`
}

type tcProg struct {
	TCProg *ebpf.Program `ebpf:"tc_prog"`
}

type Objects struct {
	tcProg
	networkMap
}

func (o *Objects) Close() error {
	var errs []error
	if o.NetworkMap != nil {
		if err := o.NetworkMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TCProg != nil {
		if err := o.TCProg.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("closing objects: %v", errs)
	}
	return nil
}

func (o *Objects) Load(opts *ebpf.CollectionOptions) error {
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(BuiltInBPF))
	if err != nil {
		return fmt.Errorf("loading spec: %w", err)
	}
	return spec.LoadAndAssign(o, opts)
}

func CheckRequiredGoFiles() error {
	if len(BuiltInBPF) == 0 {
		return fmt.Errorf("eBPF 程序未嵌入")
	}
	return nil
}
