package collector

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"

	"github.com/meinanzilinzhengying/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/output"
)

//go:embed network_flow.bpf.o
var networkFlowBpfO []byte

type NetworkCollector struct {
	output   output.Writer
	probeID  string
	iface    string
	running  bool
	stopCh   chan struct{}
	coll     *ebpf.Collection
	reader   *ringbuf.Reader
}

func NewNetworkCollector(out output.Writer, probeID, iface string) *NetworkCollector {
	return &NetworkCollector{output: out, probeID: probeID, iface: iface, stopCh: make(chan struct{})}
}

func (n *NetworkCollector) Name() string   { return "network" }
func (n *NetworkCollector) Category() string { return "network" }

func (n *NetworkCollector) Init(cap kernel.Capabilities) error {
	if !cap.HasBPFTC && !cap.HasBPFXDP {
		return fmt.Errorf("no tc/xdp support")
	}
	// 加载 BPF 对象
	coll, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(networkFlowBpfO))
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}
	loaded, err := ebpf.NewCollection(coll)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}
	n.coll = loaded

	// 保存 .o 到临时文件用于 tc 加载
	tmpFile, err := os.CreateTemp("", "network_flow_*.o")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(networkFlowBpfO); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	tmpFile.Close()

	_ = exec.Command("tc", "qdisc", "add", "dev", n.iface, "clsact").Run()
	if err := attachTC(tmpFile.Name(), n.iface, "ingress", "tc"); err != nil {
		log.Printf("[NETWORK] tc ingress attach: %v", err)
	}
	if err := attachTC(tmpFile.Name(), n.iface, "egress", "tc"); err != nil {
		log.Printf("[NETWORK] tc egress attach: %v", err)
	}

	// 获取 ringbuf map
	rbMap := loaded.Maps["rb"]
	if rbMap == nil {
		return fmt.Errorf("ringbuf map not found")
	}
	reader, err := ringbuf.NewReader(rbMap)
	if err != nil {
		return fmt.Errorf("ringbuf reader: %w", err)
	}
	n.reader = reader
	return nil
}

func attachTC(oFile, iface, direction, section string) error {
	cmd := exec.Command("tc", "filter", "add", "dev", iface, direction, "bpf", "obj", oFile, "sec", section, "direct-action")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tc attach: %w, %s", err, string(out))
	}
	return nil
}

func (n *NetworkCollector) Start(ctx context.Context) error {
	n.running = true
	go func() {
		defer n.reader.Close()
		for n.running {
			record, err := n.reader.Read()
			if err != nil {
				if n.running {
					log.Printf("[NETWORK] ringbuf read: %v", err)
				}
				continue
			}
			n.handleEvent(record.RawSample)
		}
	}()
	return nil
}

func (n *NetworkCollector) handleEvent(data []byte) {
	if len(data) < 48 {
		return
	}
	_ = binary.LittleEndian.Uint64(data[0:8]) // timestampNs
	etype := binary.LittleEndian.Uint32(data[8:12])
	pid := binary.LittleEndian.Uint32(data[12:16])
	ppid := binary.LittleEndian.Uint32(data[16:20])
	srcIP := binary.LittleEndian.Uint32(data[20:24])
	dstIP := binary.LittleEndian.Uint32(data[24:28])
	srcPort := binary.LittleEndian.Uint16(data[28:30])
	dstPort := binary.LittleEndian.Uint16(data[30:32])
	protocol := data[32]
	pktBytes := binary.LittleEndian.Uint64(data[40:48])
	packets := binary.LittleEndian.Uint64(data[48:56])
	// latency := binary.LittleEndian.Uint64(data[56:64])
	// count := binary.LittleEndian.Uint64(data[64:72])
	comm := string(bytes.Trim(data[72:88], "\x00"))
	_ = comm
	_ = etype
	_ = pid
	_ = ppid

	proto := "IP"
	switch protocol {
	case 6:
		proto = "TCP-BPF"
	case 17:
		proto = "UDP-BPF"
	case 1:
		proto = "ICMP-BPF"
	}
	now := time.Now()
	_ = n.output.WriteEvent(&output.Event{
		Timestamp: now, ProbeID: n.probeID + "-bpf", Category: "network", EventType: "flow",
		SrcIP: ipToString(srcIP), DstIP: ipToString(dstIP),
		SrcPort: srcPort, DstPort: dstPort, Protocol: proto,
		Bytes: pktBytes, Packets: packets,
	})
}

func ipToString(ip uint32) string {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, ip)
	return net.IP(b).String()
}

func (n *NetworkCollector) Stop() {
	close(n.stopCh)
	n.running = false
	if n.reader != nil {
		n.reader.Close()
	}
	_ = exec.Command("tc", "filter", "del", "dev", n.iface, "ingress").Run()
	_ = exec.Command("tc", "filter", "del", "dev", n.iface, "egress").Run()
	_ = exec.Command("tc", "qdisc", "del", "dev", n.iface, "clsact").Run()
	if n.coll != nil {
		n.coll.Close()
	}
}

func (n *NetworkCollector) Status() map[string]interface{} {
	return map[string]interface{}{"name": n.Name(), "running": n.running, "category": n.Category(), "interface": n.iface}
}
