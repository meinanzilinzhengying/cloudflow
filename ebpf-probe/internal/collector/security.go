package collector

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"

	"github.com/meinanzilinzhengying/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/output"
)

type SecurityCollector struct {
	output     output.Writer
	probeID    string
	running    bool
	stopCh     chan struct{}
	fileReader *perf.Reader
	tcpReader  *perf.Reader
	fileLinks  []link.Link
	tcpLinks   []link.Link
	tcpBpfPath string
}

func NewSecurityCollector(out output.Writer, probeID string) *SecurityCollector {
	return &SecurityCollector{output: out, probeID: probeID, stopCh: make(chan struct{}), tcpBpfPath: "/data/local/tmp/cloudflow/ebpf-probe/tcp_connect.bpf.o"}
}

func (s *SecurityCollector) Name() string     { return "security" }
func (s *SecurityCollector) Category() string { return "security" }

func (s *SecurityCollector) Init(cap kernel.Capabilities) error {
	if !cap.HasBPFKprobe {
		return fmt.Errorf("no kprobe support")
	}

	// Read BPF program from file
	bpfData, err := os.ReadFile(s.tcpBpfPath)
	if err != nil {
		return fmt.Errorf("read tcp_connect bpf file: %w", err)
	}

	// Load tcp_connect BPF
	tcpSpec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(bpfData))
	if err != nil {
		return fmt.Errorf("load tcp_connect spec: %w", err)
	}
	tcpColl, err := ebpf.NewCollection(tcpSpec)
	if err != nil {
		return fmt.Errorf("load tcp_connect collection: %w", err)
	}

	// Find perf map
	for _, m := range tcpColl.Maps {
		if m.Type() == ebpf.PerfEventArray {
			reader, err := perf.NewReader(m, os.Getpagesize()*4)
			if err != nil {
				return fmt.Errorf("tcp_connect perf reader: %w", err)
			}
			s.tcpReader = reader
			break
		}
	}

	// Attach programs
	for name, prog := range tcpColl.Programs {
		if prog == nil {
			continue
		}
		log.Printf("[SEC] Program %s type: %v", name, prog.Type())
		if prog.Type() != ebpf.Kprobe {
			log.Printf("[SEC] Skip non-kprobe: %s (type: %v)", name, prog.Type())
			continue
		}

		// Extract kernel function name from section name
		// e.g., "kprobe/tcp_connect" -> "tcp_connect"
		//       "kretprobe/tcp_connect" -> "tcp_connect"
		kernelFunc := extractKernelFunc(name)

		l, err := link.Kprobe(kernelFunc, prog, nil)
		if err != nil {
			log.Printf("[SEC] attach %s (kernel=%s) failed: %v", name, kernelFunc, err)
			continue
		}
		s.tcpLinks = append(s.tcpLinks, l)
		log.Printf("[SEC] Attached %s -> %s", name, kernelFunc)
	}

	return nil
}

func (s *SecurityCollector) Start(ctx context.Context) error {
	s.running = true

	// Start file_open reader
	if s.fileReader != nil {
		go s.readPerfLoop(s.fileReader, "file_open")
	}

	// Start tcp_connect reader
	if s.tcpReader != nil {
		go s.readPerfLoop(s.tcpReader, "tcp_connect")
	}

	log.Printf("[SEC] Started")
	return nil
}

func (s *SecurityCollector) readPerfLoop(reader *perf.Reader, label string) {
	defer reader.Close()
	for s.running {
		record, err := reader.Read()
		if err != nil {
			if s.running {
				log.Printf("[SEC] %s perf read: %v", label, err)
			}
			continue
		}

		// Parse event based on label
		switch label {
		case "file_open":
			if len(record.RawSample) >= 80 {
				pid := bytesToUint32(record.RawSample[0:4])
				comm := string(record.RawSample[4:20])
				filename := string(record.RawSample[20:80])
				if err := s.output.WriteFileEvent(time.Now(), s.probeID, pid, comm, filename, "open", 0); err != nil {
					log.Printf("[SEC] WriteFileEvent failed: %v", err)
				}
			}
		case "tcp_connect":
			if len(record.RawSample) >= 28 {
				pid := bytesToUint32(record.RawSample[0:8])
				latencyNs := bytesToUint64(record.RawSample[8:16])
				comm := string(record.RawSample[16:32])
				if err := s.output.WriteEvent(&output.Event{
					Timestamp: time.Now(),
					ProbeID:   s.probeID,
					Category:  "security",
					EventType: "tcp_connect",
					LatencyMs: float64(latencyNs) / 1e6,
					Service:   comm,
					Details:   fmt.Sprintf(`{"pid":%d}`, pid),
				}); err != nil {
					log.Printf("[SEC] WriteEvent failed: %v", err)
				}
			}
		}
	}
}

func (s *SecurityCollector) Stop() {
	s.running = false
	close(s.stopCh)
	for _, l := range s.fileLinks {
		l.Close()
	}
	for _, l := range s.tcpLinks {
		l.Close()
	}
}

func (s *SecurityCollector) Status() map[string]interface{} {
	return map[string]interface{}{
		"running": s.running,
	}
}

// extractKernelFunc maps BPF program name to kernel function name
// Moved to utils.go to avoid redeclaration

func bytesToUint32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func bytesToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}
