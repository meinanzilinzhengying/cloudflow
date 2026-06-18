package collector

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/cloudflow/ebpf-probe/internal/output"
)

type Collector interface {
	Name() string
	Category() string
	Init(cap kernel.Capabilities) error
	Start(ctx context.Context) error
	Stop()
	Status() map[string]interface{}
}

type Manager struct {
	output     *output.ClickHouse
	probeID    string
	ifaceName  string
	collectAll bool
	collectors []Collector
	mu         sync.RWMutex
}

func NewManager(out *output.ClickHouse, probeID, iface string, all bool) *Manager {
	return &Manager{output: out, probeID: probeID, ifaceName: iface, collectAll: all}
}

func (m *Manager) Init(cap kernel.Capabilities) error {
	if cap.HasBPFTC || cap.HasBPFXDP {
		m.collectors = append(m.collectors, NewNetworkCollector(m.output, m.probeID, m.ifaceName))
	}
	if cap.HasBPFKprobe || cap.HasBPFTracepoint {
		m.collectors = append(m.collectors, NewPerformanceCollector(m.output, m.probeID))
	}
	m.collectors = append(m.collectors, NewProtocolCollector(m.output, m.probeID, m.ifaceName))
	if cap.HasBPFLSM || cap.HasBPFKprobe {
		m.collectors = append(m.collectors, NewSecurityCollector(m.output, m.probeID))
	}
	m.collectors = append(m.collectors, NewHostMetricsCollector(m.output, m.probeID))

	for _, c := range m.collectors {
		if err := c.Init(cap); err != nil {
			log.Printf("[COLLECTOR] %s 初始化失败: %v", c.Name(), err)
			if m.collectAll { return err }
		} else {
			log.Printf("[COLLECTOR] %s 已就绪", c.Name())
		}
	}
	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	for _, c := range m.collectors {
		if err := c.Start(ctx); err != nil {
			log.Printf("[COLLECTOR] %s 启动失败: %v", c.Name(), err)
		}
	}
	return nil
}

func (m *Manager) Stop() {
	for _, c := range m.collectors {
		c.Stop()
	}
}

func (m *Manager) Status() map[string]interface{} {
	status := map[string]interface{}{"probe_id": m.probeID, "collectors": []map[string]interface{}{}}
	for _, c := range m.collectors {
		status["collectors"] = append(status["collectors"].([]map[string]interface{}), c.Status())
	}
	return status
}

func (m *Manager) CollectorNames() []string {
	var names []string
	for _, c := range m.collectors {
		names = append(names, c.Name())
	}
	return names
}

type HostMetricsCollector struct {
	output  *output.ClickHouse
	probeID string
	running bool
	stopCh  chan struct{}
}

func NewHostMetricsCollector(out *output.ClickHouse, probeID string) *HostMetricsCollector {
	return &HostMetricsCollector{output: out, probeID: probeID, stopCh: make(chan struct{})}
}

func (h *HostMetricsCollector) Name() string   { return "host_metrics" }
func (h *HostMetricsCollector) Category() string { return "performance" }
func (h *HostMetricsCollector) Init(cap kernel.Capabilities) error { return nil }
func (h *HostMetricsCollector) Start(ctx context.Context) error {
	h.running = true
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m := getHostMetrics()
				_ = h.output.WriteMetrics(h.probeID, m.CPUPercent, m.MemoryPercent, m.DiskPercent, m.NetRxBytes, m.NetTxBytes, m.DiskReadBytes, m.DiskWriteBytes)
			case <-h.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}
func (h *HostMetricsCollector) Stop() {
	close(h.stopCh)
	h.running = false
}
func (h *HostMetricsCollector) Status() map[string]interface{} {
	return map[string]interface{}{"name": h.Name(), "running": h.running, "category": h.Category()}
}
