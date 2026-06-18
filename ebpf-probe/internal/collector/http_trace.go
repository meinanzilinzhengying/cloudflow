package collector

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	"github.com/meinanzilinzhengying/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/output"
	"github.com/meinanzilinzhengying/ebpf-probe/pkg/protocol"
)

//go:embed http_trace.bpf.o
var httpTraceBpfO []byte

type HTTPTraceCollector struct {
	output    *output.ClickHouse
	probeID   string
	running   bool
	stopCh    chan struct{}
	coll      *ebpf.Collection
	links     []link.Link
	reader    *ringbuf.Reader
}

func NewHTTPTraceCollector(out *output.ClickHouse, probeID string) *HTTPTraceCollector {
	return &HTTPTraceCollector{output: out, probeID: probeID, stopCh: make(chan struct{})}
}

func (h *HTTPTraceCollector) Name() string   { return "http_trace" }
func (h *HTTPTraceCollector) Category() string { return "protocol" }

func (h *HTTPTraceCollector) Init(cap kernel.Capabilities) error {
	if !cap.HasBPFKprobe {
		return fmt.Errorf("no kprobe support")
	}
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(httpTraceBpfO))
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}
	loaded, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}
	h.coll = loaded

	if prog := loaded.Programs["trace_tcp_sendmsg"]; prog != nil {
		l, err := link.Kprobe("tcp_sendmsg", prog, nil)
		if err != nil { log.Printf("[HTTP] attach tcp_sendmsg: %v", err) } else { h.links = append(h.links, l) }
	}
	if prog := loaded.Programs["trace_tcp_recvmsg"]; prog != nil {
		l, err := link.Kprobe("tcp_recvmsg", prog, nil)
		if err != nil { log.Printf("[HTTP] attach tcp_recvmsg: %v", err) } else { h.links = append(h.links, l) }
	}

	rbMap := loaded.Maps["rb"]
	if rbMap == nil {
		return fmt.Errorf("ringbuf map not found")
	}
	reader, err := ringbuf.NewReader(rbMap)
	if err != nil {
		return fmt.Errorf("ringbuf reader: %w", err)
	}
	h.reader = reader
	return nil
}

func (h *HTTPTraceCollector) Start(ctx context.Context) error {
	h.running = true
	go func() {
		defer h.reader.Close()
		for h.running {
			record, err := h.reader.Read()
			if err != nil {
				if h.running { log.Printf("[HTTP] ringbuf read: %v", err) }
				continue
			}
			h.handleEvent(record.RawSample)
		}
	}()
	return nil
}

func (h *HTTPTraceCollector) handleEvent(data []byte) {
	if len(data) < 48 { return }
	etype := binary.LittleEndian.Uint32(data[8:12])
	if etype != 2 { return }
	pid := binary.LittleEndian.Uint32(data[12:16])
	srcIP := binary.LittleEndian.Uint32(data[20:24])
	dstIP := binary.LittleEndian.Uint32(data[24:28])
	srcPort := binary.LittleEndian.Uint16(data[28:30])
	dstPort := binary.LittleEndian.Uint16(data[30:32])
	direction := data[32]
	pktBytes := binary.LittleEndian.Uint64(data[40:48])
	payload := string(bytes.Trim(data[72:328], "\x00"))
	_ = pid
	_ = pktBytes

	rec := protocol.ParseHTTP([]byte(payload), ipToString(srcIP), ipToString(dstIP), srcPort, dstPort, direction)
	if rec == nil { return }
	rec.ProbeID = h.probeID
	now := time.Now()
	_ = h.output.WriteEvent(&output.Event{
		Timestamp: now, ProbeID: h.probeID, Category: "protocol", EventType: "http",
		SrcIP: rec.SrcIP, DstIP: rec.DstIP, SrcPort: rec.SrcPort, DstPort: rec.DstPort,
		Protocol: "HTTP", Bytes: rec.Bytes, LatencyMs: rec.LatencyMs,
		Service: rec.Host, Details: rec.Method + " " + rec.URL,
		Tags: fmt.Sprintf("status=%d,error=%v,slow=%v", rec.StatusCode, rec.IsError, rec.IsSlow),
	})
}

func (h *HTTPTraceCollector) Stop() {
	close(h.stopCh)
	h.running = false
	if h.reader != nil { h.reader.Close() }
	for _, l := range h.links { l.Close() }
	if h.coll != nil { h.coll.Close() }
}

func (h *HTTPTraceCollector) Status() map[string]interface{} {
	return map[string]interface{}{"name": h.Name(), "running": h.running, "category": h.Category()}
}
