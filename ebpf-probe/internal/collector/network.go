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
	"syscall"
	"time"
	"unsafe"

	"github.com/meinanzilinzhengying/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/output"
)

//go:embed network_flow.bpf.o
var networkFlowBpfO []byte

type NetworkCollector struct {
	output  output.Writer
	probeID string
	iface   string
	running bool
	stopCh  chan struct{}
	fd      int
}

func NewNetworkCollector(out output.Writer, probeID, iface string) *NetworkCollector {
	return &NetworkCollector{output: out, probeID: probeID, iface: iface, stopCh: make(chan struct{})}
}

func (n *NetworkCollector) Name() string     { return "network" }
func (n *NetworkCollector) Category() string { return "network" }

func (n *NetworkCollector) Init(cap kernel.Capabilities) error {
	// Create AF_PACKET socket
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(syscall.ETH_P_ALL)))
	if err != nil {
		return fmt.Errorf("AF_PACKET socket: %w", err)
	}

	// Attach BPF filter
	if len(networkFlowBpfO) > 0 {
		// Try to load BPF filter from compiled .o
		// For now, use raw socket without BPF filter
		log.Printf("[NETWORK] Using AF_PACKET raw socket (BPF filter not loaded)")
	}

	n.fd = fd
	return nil
}

func (n *NetworkCollector) Start(ctx context.Context) error {
	if n.fd < 0 {
		return fmt.Errorf("network collector not initialized")
	}
	n.running = true

	go func() {
		defer syscall.Close(n.fd)
		buf := make([]byte, 65536)

		for n.running {
			select {
			case <-n.stopCh:
				return
			default:
			}

			// Set read timeout
			syscall.SetsockoptTimeval(n.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{Sec: 1})

			n, err := syscall.Read(n.fd, buf)
			if err != nil {
				if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
					continue
				}
				if n.running {
					log.Printf("[NETWORK] read error: %v", err)
				}
				continue
			}

			if n < 14 {
				continue
			}

			// Parse Ethernet header
			ethType := binary.BigEndian.Uint16(buf[12:14])
			if ethType != 0x0800 { // IPv4
				continue
			}

			if n < 34 {
				continue
			}

			// Parse IP header
			srcIP := binary.BigEndian.Uint16(buf[26:28])
			dstIP := binary.BigEndian.Uint16(buf[28:30])
			proto := buf[29]
			srcPort := binary.BigEndian.Uint16(buf[34:36])
			dstPort := binary.BigEndian.Uint16(buf[36:38])

			// Create event
			ev := &output.Event{
				Timestamp: time.Now(),
				ProbeID:   n.probeID,
				Category:  "network",
				EventType: "flow",
				SrcIP:     fmt.Sprintf("%d.%d.%d.%d", buf[26], buf[27], buf[28], buf[29]),
				DstIP:     fmt.Sprintf("%d.%d.%d.%d", buf[30], buf[31], buf[32], buf[33]),
				SrcPort:   srcPort,
				DstPort:   dstPort,
				Protocol:  map[byte]string{6: "TCP", 17: "UDP", 1: "ICMP"}[proto],
				Bytes:     uint64(n),
				Packets:   1,
			}

			_ = srcIP
			_ = dstIP

			if err := n.output.WriteEvent(ev); err != nil {
				log.Printf("[NETWORK] write event: %v", err)
			}
		}
	}()

	log.Printf("[NETWORK] Started on %s", n.iface)
	return nil
}

func (n *NetworkCollector) Stop() {
	n.running = false
	close(n.stopCh)
}

func htons(v uint16) uint16 {
	return (v << 8) | (v >> 8)
}

// Unused import suppressor
var _ = net.IP{}
var _ = unsafe.Sizeof(0)
