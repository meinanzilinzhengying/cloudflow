package collector

import (
	"bytes"
	"context"
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

type SyscallCollector struct {
	output  output.Writer
	probeID string
	running bool
	stopCh  chan struct{}
	reader  *perf.Reader
	links   []link.Link
	bpfPath string
}

func NewSyscallCollector(out output.Writer, probeID string) *SyscallCollector {
	return &SyscallCollector{output: out, probeID: probeID, stopCh: make(chan struct{}), bpfPath: "/data/local/tmp/cloudflow/ebpf-probe/syscall.bpf.o"}
}

func (s *SyscallCollector) Name() string     { return "syscall" }
func (s *SyscallCollector) Category() string { return "syscall" }

func (s *SyscallCollector) Init(cap kernel.Capabilities) error {
	// Read BPF program from file
	bpfData, err := os.ReadFile(s.bpfPath)
	if err != nil {
		return fmt.Errorf("read bpf file: %w", err)
	}

	// Load BPF object
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
	for name, m := range coll.Maps {
		if m.Type() == ebpf.PerfEventArray {
			perfMap = m
			log.Printf("[SYSCALL] Found perf map: %s", name)
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
	s.reader = reader

	// Attach BPF programs to tracepoints
	for name, prog := range coll.Programs {
		if prog == nil {
			continue
		}

		var l link.Link
		var attachErr error

		switch prog.Type() {
		case ebpf.TracePoint:
			l, attachErr = attachTracepoint(name, prog)
		case ebpf.Kprobe:
			l, attachErr = attachKprobe(name, prog)
		default:
			log.Printf("[SYSCALL] Skip program %s (type: %v)", name, prog.Type())
			continue
		}

		if attachErr != nil {
			log.Printf("[SYSCALL] Attach %s failed: %v", name, attachErr)
			continue
		}
		s.links = append(s.links, l)
		log.Printf("[SYSCALL] Attached %s", name)
	}

	return nil
}

func attachTracepoint(name string, prog *ebpf.Program) (link.Link, error) {
	// Parse "tracepoint/category/name"
	// For now, try common syscall tracepoints
	switch {
	case name == "trace_syscall_exit" || name == "tracepoint/raw_syscalls/sys_exit":
		l, err := link.Tracepoint("raw_syscalls", "sys_exit", prog, nil)
		if err != nil {
			return nil, fmt.Errorf("attach sys_exit: %w", err)
		}
		return l, nil
	case name == "trace_syscall_enter" || name == "tracepoint/raw_syscalls/sys_enter":
		l, err := link.Tracepoint("raw_syscalls", "sys_enter", prog, nil)
		if err != nil {
			return nil, fmt.Errorf("attach sys_enter: %w", err)
		}
		return l, nil
	default:
		return nil, fmt.Errorf("unknown tracepoint: %s", name)
	}
}

func attachKprobe(name string, prog *ebpf.Program) (link.Link, error) {
	// Extract kernel function from section name or map known names
	kernelFunc := name
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		kernelFunc = name[idx+1:]
	}
	// Map BPF function names to kernel function names
	knownFuncs := map[string]string{
		"trace_connect_entry":    "tcp_connect",
		"trace_connect_exit":     "tcp_connect",
		"trace_file_open":        "do_filp_open",
		"trace_file_open_entry":  "do_filp_open",
		"trace_file_open_exit":   "do_filp_open",
		"trace_syscall_entry":    "tcp_connect",
		"trace_syscall_exit":     "tcp_connect",
		"trace_net_entry":        "tcp_connect",
		"trace_net_exit":         "tcp_connect",
		"trace_close_entry":      "__arm64_sys_close",
		"trace_close_exit":       "__arm64_sys_close",
	}
	if kf, ok := knownFuncs[kernelFunc]; ok {
		kernelFunc = kf
	}
	l, err := link.Kprobe(kernelFunc, prog, nil)
	if err != nil {
		return nil, fmt.Errorf("attach kprobe %s (kernel=%s): %w", name, kernelFunc, err)
	}
	return l, nil
}

func (s *SyscallCollector) Start(ctx context.Context) error {
	if s.reader == nil {
		return fmt.Errorf("syscall collector not initialized")
	}
	s.running = true

	go func() {
		defer s.reader.Close()
		for s.running {
			record, err := s.reader.Read()
			if err != nil {
				if s.running {
					log.Printf("[SYSCALL] perf read: %v", err)
				}
				continue
			}

			if len(record.RawSample) < 4 {
				continue
			}

			// Parse file event
			pid := binary.LittleEndian.Uint32(record.RawSample[0:4])
			opType := "open"
			if len(record.RawSample) >= 8 {
				opCode := binary.LittleEndian.Uint32(record.RawSample[4:8])
				if opCode == 1 {
					opType = "close"
				}
			}
			comm := ""
			if len(record.RawSample) >= 24 {
				comm = string(record.RawSample[8:24])
			}

			ev := &output.Event{
				Timestamp: time.Now(),
				ProbeID:   s.probeID,
				Category:  "syscall",
				EventType: opType,
				Service:   comm,
				Details:   fmt.Sprintf(`{"pid":%d,"op":"%s"}`, pid, opType),
			}

			if err := s.output.WriteEvent(ev); err != nil {
				log.Printf("[SYSCALL] write event: %v", err)
			}
		}
	}()

	log.Printf("[SYSCALL] Started")
	return nil
}

func (s *SyscallCollector) Stop() {
	s.running = false
	close(s.stopCh)
	for _, l := range s.links {
		l.Close()
	}
}

func (s *SyscallCollector) Status() map[string]interface{} {
	return map[string]interface{}{
		"running": s.running,
	}
}

// Unused import suppressor
var _ = unsafe.Sizeof(0)
