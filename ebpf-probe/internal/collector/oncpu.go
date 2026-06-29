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
	"github.com/cilium/ebpf/perf"
	"golang.org/x/sys/unix"

	"github.com/meinanzilinzhengying/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/output"
)

type OnCPUCollector struct {
	output  output.Writer
	probeID string
	running bool
	stopCh  chan struct{}
	reader  *perf.Reader
	perfFds []*os.File
}

func NewOnCPUCollector(out output.Writer, probeID string) *OnCPUCollector {
	return &OnCPUCollector{output: out, probeID: probeID, stopCh: make(chan struct{})}
}

func (o *OnCPUCollector) Name() string     { return "oncpu" }
func (o *OnCPUCollector) Category() string { return "performance" }

func (o *OnCPUCollector) Init(cap kernel.Capabilities) error {
	// Load the cpu_sample BPF program
	bpfPath := "/data/local/tmp/cloudflow/ebpf-probe/cpu_sample.bpf.o"
	bpfData, err := os.ReadFile(bpfPath)
	if err != nil {
		return fmt.Errorf("read cpu_sample bpf: %w", err)
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
	o.reader = reader

	// Get the BPF program
	prog := coll.Programs["cpu_profile"]
	if prog == nil {
		return fmt.Errorf("cpu_profile program not found")
	}

	// Create perf event on each CPU with 99Hz sampling
	freq := uint64(99)
	attrs := &unix.PerfEventAttr{
		Type:        unix.PERF_TYPE_SOFTWARE,
		Config:      unix.PERF_COUNT_SW_CPU_CLOCK,
		Sample_type: unix.PERF_SAMPLE_RAW,
		Sample:      freq,
		Wakeup:      1,
		Size:        uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
	}

	for i := 0; i < 4; i++ { // Assume 4 CPUs max on STB
		fd, err := unix.PerfEventOpen(attrs, -1, i, -1, 0)
		if err != nil {
			log.Printf("[ONCPU] perf_event_open cpu %d failed: %v", i, err)
			continue
		}

		progFd := prog.FD()
		ret, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.PERF_EVENT_IOC_SET_BPF, uintptr(progFd))
		if ret != 0 {
			log.Printf("[ONCPU] set_bpf cpu %d failed: %v", i, errno)
			unix.Close(fd)
			continue
		}

		ret, _, errno = unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.PERF_EVENT_IOC_ENABLE, 0)
		if ret != 0 {
			log.Printf("[ONCPU] enable cpu %d failed: %v", i, errno)
			unix.Close(fd)
			continue
		}

		o.perfFds = append(o.perfFds, os.NewFile(uintptr(fd), fmt.Sprintf("perf_event_cpu_%d", i)))
		log.Printf("[ONCPU] Attached to CPU %d", i)
	}

	return nil
}

func (o *OnCPUCollector) Start(ctx context.Context) error {
	if o.reader == nil {
		return fmt.Errorf("oncpu collector not initialized")
	}
	o.running = true

	go func() {
		defer o.reader.Close()
		for o.running {
			record, err := o.reader.Read()
			if err != nil {
				continue
			}
			if len(record.RawSample) < 20 {
				continue
			}

			pid := binary.LittleEndian.Uint32(record.RawSample[0:4])
			cpu := binary.LittleEndian.Uint32(record.RawSample[4:8])
			comm := ""
			if len(record.RawSample) >= 24 {
				comm = string(record.RawSample[8:24])
			}

			ev := &output.Event{
				Timestamp: time.Now(),
				ProbeID:   o.probeID,
				Category:  "performance",
				EventType: "on_cpu_sample",
				Service:   comm,
				Details:   fmt.Sprintf(`{"pid":%d,"cpu":%d}`, pid, cpu),
			}
			o.output.WriteEvent(ev)
		}
	}()

	log.Printf("[ONCPU] Started with %d perf fds", len(o.perfFds))
	return nil
}

func (o *OnCPUCollector) Stop() {
	o.running = false
	close(o.stopCh)
	for _, fd := range o.perfFds {
		fd.Close()
	}
}

func (o *OnCPUCollector) Status() map[string]interface{} {
	return map[string]interface{}{"running": o.running, "cpus": len(o.perfFds)}
}

var _ = unsafe.Sizeof(0)
