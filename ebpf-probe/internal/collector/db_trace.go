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

//go:embed db_trace.bpf.o
var dbTraceBpfO []byte

type DBTraceCollector struct {
	output    *output.ClickHouse
	probeID   string
	running   bool
	stopCh    chan struct{}
	coll      *ebpf.Collection
	links     []link.Link
	reader    *ringbuf.Reader
}

func NewDBTraceCollector(out *output.ClickHouse, probeID string) *DBTraceCollector {
	return &DBTraceCollector{output: out, probeID: probeID, stopCh: make(chan struct{})}
}

func (d *DBTraceCollector) Name() string   { return "db_trace" }
func (d *DBTraceCollector) Category() string { return "protocol" }

func (d *DBTraceCollector) Init(cap kernel.Capabilities) error {
	if !cap.HasBPFKprobe {
		return fmt.Errorf("no kprobe support")
	}
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(dbTraceBpfO))
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}
	loaded, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}
	d.coll = loaded

	if prog := loaded.Programs["trace_db_tcp_sendmsg"]; prog != nil {
		l, err := link.Kprobe("tcp_sendmsg", prog, nil)
		if err != nil { log.Printf("[DB] attach tcp_sendmsg: %v", err) } else { d.links = append(d.links, l) }
	}
	if prog := loaded.Programs["trace_db_tcp_recvmsg"]; prog != nil {
		l, err := link.Kprobe("tcp_recvmsg", prog, nil)
		if err != nil { log.Printf("[DB] attach tcp_recvmsg: %v", err) } else { d.links = append(d.links, l) }
	}

	rbMap := loaded.Maps["rb"]
	if rbMap == nil {
		return fmt.Errorf("ringbuf map not found")
	}
	reader, err := ringbuf.NewReader(rbMap)
	if err != nil {
		return fmt.Errorf("ringbuf reader: %w", err)
	}
	d.reader = reader
	return nil
}

func (d *DBTraceCollector) Start(ctx context.Context) error {
	d.running = true
	go func() {
		defer d.reader.Close()
		for d.running {
			record, err := d.reader.Read()
			if err != nil {
				if d.running { log.Printf("[DB] ringbuf read: %v", err) }
				continue
			}
			d.handleEvent(record.RawSample)
		}
	}()
	return nil
}

func (d *DBTraceCollector) handleEvent(data []byte) {
	if len(data) < 48 { return }
	etype := binary.LittleEndian.Uint32(data[8:12])
	srcIP := binary.LittleEndian.Uint32(data[20:24])
	dstIP := binary.LittleEndian.Uint32(data[24:28])
	srcPort := binary.LittleEndian.Uint16(data[28:30])
	dstPort := binary.LittleEndian.Uint16(data[30:32])
	payload := data[72:328]
	now := time.Now()

	var rec *protocol.DBRecord
	if etype == 10 { // MySQL
		rec = protocol.ParseMySQL(payload, ipToString(srcIP), ipToString(dstIP), srcPort, dstPort, 0)
	} else if etype == 11 { // Redis
		rec = protocol.ParseRedis(payload, ipToString(srcIP), ipToString(dstIP), srcPort, dstPort, 0)
	}
	if rec == nil { return }
	rec.ProbeID = d.probeID
	_ = d.output.WriteEvent(&output.Event{
		Timestamp: now, ProbeID: d.probeID, Category: "protocol", EventType: "db",
		SrcIP: rec.SrcIP, DstIP: rec.DstIP, SrcPort: rec.SrcPort, DstPort: rec.DstPort,
		Protocol: rec.DBType, Details: rec.Query,
		Tags: fmt.Sprintf("slow=%v", rec.IsSlow),
	})
}

func (d *DBTraceCollector) Stop() {
	close(d.stopCh)
	d.running = false
	if d.reader != nil { d.reader.Close() }
	for _, l := range d.links { l.Close() }
	if d.coll != nil { d.coll.Close() }
}

func (d *DBTraceCollector) Status() map[string]interface{} {
	return map[string]interface{}{"name": d.Name(), "running": d.running, "category": d.Category()}
}
