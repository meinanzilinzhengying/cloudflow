package collector

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"

	"github.com/meinanzilinzhengying/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/output"
)

// NetQualityCollector tracks TCP retransmissions and UDP multicast
type NetQualityCollector struct {
	output  output.Writer
	probeID string
	running bool
	stopCh  chan struct{}
	reader  *perf.Reader
	links   []link.Link
	bpfPath string
}

func NewNetQualityCollector(out output.Writer, probeID string) *NetQualityCollector {
	return &NetQualityCollector{output: out, probeID: probeID, stopCh: make(chan struct{}), bpfPath: "/data/local/tmp/cloudflow/ebpf-probe/tcp_retransmit.bpf.o"}
}

func (n *NetQualityCollector) Name() string     { return "net_quality" }
func (n *NetQualityCollector) Category() string { return "security" }

func (n *NetQualityCollector) Init(cap kernel.Capabilities) error {
	bpfData, err := os.ReadFile(n.bpfPath)
	if err != nil {
		return fmt.Errorf("read tcp_retransmit bpf: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(bpfData))
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}

	var perfMap *ebpf.Map
	for _, m := range coll.Maps {
		if m.Type() == ebpf.PerfEventArray {
			perfMap = m
			break
		}
	}
	if perfMap == nil {
		return fmt.Errorf("no perf event map")
	}

	reader, err := perf.NewReader(perfMap, os.Getpagesize()*4)
	if err != nil {
		return fmt.Errorf("perf reader: %w", err)
	}
	n.reader = reader

	for name, prog := range coll.Programs {
		if prog == nil || prog.Type() != ebpf.Kprobe {
			continue
		}
		kernelFunc := extractKernelFunc(name)
		l, err := link.Kprobe(kernelFunc, prog, nil)
		if err != nil {
			log.Printf("[NETQ] attach %s failed: %v", name, err)
			continue
		}
		n.links = append(n.links, l)
		log.Printf("[NETQ] Attached %s -> %s", name, kernelFunc)
	}

	return nil
}

func (n *NetQualityCollector) Start(ctx context.Context) error {
	if n.reader == nil {
		return fmt.Errorf("net_quality collector not initialized")
	}
	n.running = true

	go func() {
		defer n.reader.Close()
		for n.running {
			record, err := n.reader.Read()
			if err != nil {
				continue
			}
			if len(record.RawSample) < 4 {
				continue
			}

			pid := binary.LittleEndian.Uint32(record.RawSample[0:4])
			comm := ""
			if len(record.RawSample) >= 20 {
				comm = string(record.RawSample[4:20])
			}

			ev := &output.Event{
				Timestamp: time.Now(),
				ProbeID:   n.probeID,
				Category:  "security",
				EventType: "tcp_retransmit",
				Service:   comm,
				Details:   fmt.Sprintf(`{"pid":%d}`, pid),
			}
			n.output.WriteEvent(ev)
		}
	}()

	log.Printf("[NETQ] Started")
	return nil
}

func (n *NetQualityCollector) Stop() {
	n.running = false
	close(n.stopCh)
	for _, l := range n.links {
		l.Close()
	}
}

func (n *NetQualityCollector) Status() map[string]interface{} {
	return map[string]interface{}{"running": n.running}
}

var _ = unsafe.Sizeof(0)
