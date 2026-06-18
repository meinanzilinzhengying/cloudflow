package collector

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/output"
)

type Collector interface {
	Name() string
	Category() string
	Init(cap kernel.Capabilities) error
	Start(ctx context.Context) error
	Stop()
	Status() map[string]interface{}
}

// CollectorConfig 采集器配置
type CollectorConfig struct {
	NetworkFlow   bool `yaml:"network_flow"`
	ProcessExec   bool `yaml:"process_exec"`
	FileOpen      bool `yaml:"file_open"`
	TCPCConnect   bool `yaml:"tcp_connect"`
	Syscall       bool `yaml:"syscall"`
	HTTPTrace     bool `yaml:"http_trace"`
	DNSTrace      bool `yaml:"dns_trace"`
	DBTrace       bool `yaml:"db_trace"`
	SchedTrace    bool `yaml:"sched_trace"`
	MemTrace      bool `yaml:"mem_trace"`
	BlockTrace    bool `yaml:"block_trace"`
	SecurityTrace bool `yaml:"security_trace"`
	HostMetrics   bool `yaml:"host_metrics"`
}

func DefaultConfig() CollectorConfig {
	return CollectorConfig{
		NetworkFlow: true,
		ProcessExec: true,
		FileOpen:    true,
		TCPCConnect: true,
		Syscall:     false,
		HTTPTrace:   false,
		DNSTrace:    false,
		DBTrace:     false,
		SchedTrace:  false,
		MemTrace:    false,
		BlockTrace:  false,
		SecurityTrace: false,
		HostMetrics: true,
	}
}

type Manager struct {
	output     *output.ClickHouse
	probeID    string
	ifaceName  string
	config     CollectorConfig
	collectors []Collector
	mu         sync.RWMutex
}

func NewManager(out *output.ClickHouse, probeID, iface string, cfg CollectorConfig) *Manager {
	return &Manager{output: out, probeID: probeID, ifaceName: iface, config: cfg}
}

func (m *Manager) Init(cap kernel.Capabilities) error {
	// P0 核心采集器
	if m.config.NetworkFlow && (cap.HasBPFTC || cap.HasBPFXDP) {
		m.collectors = append(m.collectors, NewNetworkCollector(m.output, m.probeID, m.ifaceName))
	}
	if m.config.ProcessExec && (cap.HasBPFKprobe || cap.HasBPFTracepoint) {
		m.collectors = append(m.collectors, NewPerformanceCollector(m.output, m.probeID))
	}
	if m.config.TCPCConnect && cap.HasBPFKprobe {
		m.collectors = append(m.collectors, NewSecurityCollector(m.output, m.probeID))
	}
	m.collectors = append(m.collectors, NewProtocolCollector(m.output, m.probeID, m.ifaceName))

	// P1 扩展采集器
	if m.config.HTTPTrace && cap.HasBPFKprobe {
		m.collectors = append(m.collectors, NewHTTPTraceCollector(m.output, m.probeID))
	}
	if m.config.DNSTrace && cap.HasBPFKprobe {
		m.collectors = append(m.collectors, NewDNSTraceCollector(m.output, m.probeID))
	}
	if m.config.DBTrace && cap.HasBPFKprobe {
		m.collectors = append(m.collectors, NewDBTraceCollector(m.output, m.probeID))
	}
	if m.config.SchedTrace && cap.HasBPFTracepoint {
		m.collectors = append(m.collectors, NewSchedTraceCollector(m.output, m.probeID))
	}
	if m.config.MemTrace && cap.HasBPFKprobe {
		m.collectors = append(m.collectors, NewMemTraceCollector(m.output, m.probeID))
	}
	if m.config.BlockTrace && cap.HasBPFTracepoint {
		m.collectors = append(m.collectors, NewBlockTraceCollector(m.output, m.probeID))
	}
	if m.config.SecurityTrace && cap.HasBPFKprobe {
		m.collectors = append(m.collectors, NewSecurityTraceCollector(m.output, m.probeID))
	}

	// 主机指标（始终可用）
	if m.config.HostMetrics {
		m.collectors = append(m.collectors, NewHostMetricsCollector(m.output, m.probeID))
	}

	for _, c := range m.collectors {
		if err := c.Init(cap); err != nil {
			log.Printf("[COLLECTOR] %s 初始化失败: %v", c.Name(), err)
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

// HostMetricsCollector 用户态主机指标
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
