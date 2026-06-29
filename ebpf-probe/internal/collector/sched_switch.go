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

type SchedSwitchCollector struct {
	output  output.Writer
	probeID string
	running bool
	stopCh  chan struct{}
	reader  *perf.Reader
	links   []link.Link
	bpfPath string
}

func NewSchedSwitchCollector(out output.Writer, probeID string) *SchedSwitchCollector {
	return &SchedSwitchCollector{output: out, probeID: probeID, stopCh: make(chan struct{}), bpfPath: "/data/local/tmp/cloudflow/ebpf-probe/sched_switch.bpf.o"}
}

func (s *SchedSwitchCollector) Name() string     { return "sched_switch" }
func (s *SchedSwitchCollector) Category() string { return "performance" }

func (s *SchedSwitchCollector) Init(cap kernel.Capabilities) error {
	bpfData, err := os.ReadFile(s.bpfPath)
	if err != nil {
		return fmt.Errorf("read sched_switch bpf file: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(bpfData))
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}

	// Find perf event array
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
	s.reader = reader

	// Attach kprobes
	for name, prog := range coll.Programs {
		if prog == nil || prog.Type() != ebpf.Kprobe {
			continue
		}
		kernelFunc := extractKernelFunc(name)
		l, err := link.Kprobe(kernelFunc, prog, nil)
		if err != nil {
			log.Printf("[SCHED] attach %s failed: %v", name, err)
			continue
		}
		s.links = append(s.links, l)
		log.Printf("[SCHED] Attached %s -> %s", name, kernelFunc)
	}

	return nil
}

func (s *SchedSwitchCollector) Start(ctx context.Context) error {
	if s.reader == nil {
		return fmt.Errorf("sched_switch collector not initialized")
	}
	s.running = true

	go func() {
		defer s.reader.Close()
		for s.running {
			record, err := s.reader.Read()
			if err != nil {
				continue
			}
			if len(record.RawSample) < 8 {
				continue
			}

			pid := binary.LittleEndian.Uint32(record.RawSample[0:4])
			offCpuNs := binary.LittleEndian.Uint64(record.RawSample[8:16])
			comm := ""
			if len(record.RawSample) >= 24 {
				comm = string(record.RawSample[16:24])
			}

			ev := &output.Event{
				Timestamp: time.Now(),
				ProbeID:   s.probeID,
				Category:  "performance",
				EventType: "off_cpu",
				Service:   comm,
				LatencyMs: float64(offCpuNs) / 1e6,
				Details:   fmt.Sprintf(`{"pid":%d,"off_cpu_ns":%d}`, pid, offCpuNs),
			}
			s.output.WriteEvent(ev)
		}
	}()

	log.Printf("[SCHED] Started")
	return nil
}

func (s *SchedSwitchCollector) Stop() {
	s.running = false
	close(s.stopCh)
	for _, l := range s.links {
		l.Close()
	}
}

func (s *SchedSwitchCollector) Status() map[string]interface{} {
	return map[string]interface{}{"running": s.running}
}

var _ = unsafe.Sizeof(0)
