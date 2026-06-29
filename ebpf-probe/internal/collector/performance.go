package collector

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"

	"github.com/meinanzilinzhengying/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/output"
)

//go:embed process_exec.bpf.o
var processExecBpfO []byte

type PerformanceCollector struct {
	output  output.Writer
	probeID string
	running bool
	stopCh  chan struct{}
	reader  *perf.Reader
	links   []link.Link
	bpfPath string
}

func NewPerformanceCollector(out output.Writer, probeID string) *PerformanceCollector {
	return &PerformanceCollector{output: out, probeID: probeID, stopCh: make(chan struct{}), bpfPath: "/data/local/tmp/cloudflow/ebpf-probe/process_exec.bpf.o"}
}

func (p *PerformanceCollector) Name() string     { return "performance" }
func (p *PerformanceCollector) Category() string { return "performance" }

func (p *PerformanceCollector) Init(cap kernel.Capabilities) error {
	// Read BPF program from file
	bpfData, err := os.ReadFile(p.bpfPath)
	if err != nil {
		return fmt.Errorf("read process_exec bpf file: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(bpfData))
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}

	// Find perf event array map
	var perfMap *ebpf.Map
	for _, m := range coll.Maps {
		if m.Type() == ebpf.PerfEventArray {
			perfMap = m
			break
		}
	}

	if perfMap == nil {
		return fmt.Errorf("no perf event map found")
	}

	reader, err := perf.NewReader(perfMap, os.Getpagesize()*4)
	if err != nil {
		return fmt.Errorf("perf reader: %w", err)
	}
	p.reader = reader

	// Attach kprobes
	for name, prog := range coll.Programs {
		if prog == nil || prog.Type() != ebpf.Kprobe {
			continue
		}

		kernelFunc := name
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			kernelFunc = name[idx+1:]
		}
		// Map BPF function names to kernel function names
		knownFuncs := map[string]string{
			"trace_process_exec": "do_execve",
			"trace_process_exit": "do_execve",
			"trace_sched_exec":   "do_execve",
			"trace_sched_exit":   "do_execve",
			"trace_exec":         "do_execve",
			"trace_exit":         "do_execve",
		}
		if kf, ok := knownFuncs[kernelFunc]; ok {
			kernelFunc = kf
		}

		l, err := link.Kprobe(kernelFunc, prog, nil)
		if err != nil {
			log.Printf("[PERF] attach %s (kernel=%s) failed: %v", name, kernelFunc, err)
			continue
		}
		p.links = append(p.links, l)
		log.Printf("[PERF] Attached %s -> %s", name, kernelFunc)
	}

	return nil
}

func (p *PerformanceCollector) Start(ctx context.Context) error {
	if p.reader == nil {
		return fmt.Errorf("performance collector not initialized")
	}
	p.running = true

	go func() {
		defer p.reader.Close()
		for p.running {
			record, err := p.reader.Read()
			if err != nil {
				if p.running {
					log.Printf("[PERF] perf read: %v", err)
				}
				continue
			}
			p.handleEvent(record.RawSample)
		}
	}()

	log.Printf("[PERF] Started")
	return nil
}

func (p *PerformanceCollector) handleEvent(data []byte) {
	if len(data) < 4 {
		return
	}

	pid := binary.LittleEndian.Uint32(data[0:4])
	comm := ""
	if len(data) >= 20 {
		comm = string(data[4:20])
	}

	ev := &output.Event{
		Timestamp: time.Now(),
		ProbeID:   p.probeID,
		Category:  "process",
		EventType: "exec",
		Service:   comm,
		Details:   fmt.Sprintf(`{"pid":%d}`, pid),
	}

	if err := p.output.WriteEvent(ev); err != nil {
		log.Printf("[PERF] write event: %v", err)
	}
}

func (p *PerformanceCollector) Stop() {
	p.running = false
	close(p.stopCh)
	for _, l := range p.links {
		l.Close()
	}
}

func (p *PerformanceCollector) Status() map[string]interface{} {
	return map[string]interface{}{
		"running": p.running,
	}
}

var _ = unsafe.Sizeof(0)
