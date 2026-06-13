package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/meinanzilinzhengying/cloudflow/pkg/utils"
)

// loadAPIKey 从环境变量或配置文件加载 API Key
func loadAPIKey() (string, error) {
	if keyFile := os.Getenv("CLOUD_FLOW_API_KEY_FILE"); keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return "", fmt.Errorf("读取 API Key 文件失败: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	if apiKey := os.Getenv("CLOUD_FLOW_API_KEY"); apiKey != "" {
		return apiKey, nil
	}

	apiKey := viper.GetString("api_key")
	if apiKey != "" {
		if os.Getenv("CLOUD_FLOW_ENV") != "development" {
			fmt.Fprintln(os.Stderr, "⚠️  安全警告: API Key 从配置文件加载，仅建议用于开发环境")
		}
		return apiKey, nil
	}

	return "", nil
}

type CollectConfig struct {
	CPU     bool
	Memory  bool
	Network bool
	Disk    bool
}

type LogConfig struct {
	Level  string
	Format string
}

type TLSConfig struct {
	Enabled    bool
	ServerName string
	CACert     string
	ClientCert string
	ClientKey  string
}

type NetworkConfig struct {
	MgmtIface            string
	MgmtIP               string
	LocalAddr            string
	PreferredSourceIface string
}

type TCPMetricsConfig struct {
	Enabled        bool
	ConnectLatency bool
	Retransmit     bool
	ZeroWindow     bool
	QueueOverflow  bool
	ConnectionFail bool
}

type HTTPMetricsConfig struct {
	Enabled         bool
	SuccessRate     bool
	ResponseLatency bool
	ErrorRate       bool
	RequestCount    bool
	ResponseCount   bool
}

type BaseTrafficConfig struct {
	Enabled        bool
	CollectBytes   bool
	CollectPackets bool
}

type ProtocolParsingConfig struct {
	Enabled   bool
	HTTPFull  bool
	DNSFull   bool
	MySQLFull bool
}

type ResourceLimitConfig struct {
	Enabled       bool
	MaxCPUCore    float64
	MaxMemoryMB   float64
	MaxGoroutines int
	UseCgroup     bool
}

type CircuitBreakerConfig struct {
	Enabled                   bool
	MaxFailures               int
	ResetTimeout              time.Duration
	SilentDuration            time.Duration
	CheckInterval             time.Duration
	CPUDegradedThreshold      float64
	CPUSilentThreshold        float64
	MemDegradedThreshold      float64
	MemSilentThreshold        float64
	CPUDegradedDuration       time.Duration
	CPURecoverThreshold       float64
	MemRecoverThreshold       float64
	SilentCPURecoverThreshold float64
	SilentMemRecoverThreshold float64
}

type SelfMonitorConfig struct {
	Enabled                   bool
	CollectInterval           time.Duration
	ReportInterval            time.Duration
	HeartbeatTimeout          time.Duration
	AlertHeartbeatFailCount   int
	AlertCPUPercent           float64
	AlertMemoryPercent        float64
	AlertPacketDropRate       float64
	AlertReportSuccessRateMin float64
}

type PerfOptimizerConfig struct {
	Enabled         bool
	SampleRate      float64
	BatchSize       int
	MaxEventsPerSec int
	HighLoadMode    bool
	EnableAdaptive  bool
}

type CPUProfilerConfig struct {
	Enabled       bool
	SampleFreq    int
	TargetPID     uint32
	MaxStackDepth int
	DurationSec   int
	OutputDir     string
	AutoDetect    bool
}

type SQLAggregatorConfig struct {
	Enabled              bool
	SlowQueryThresholdMs uint64
	EnableCorrelation    bool
	MaxSnapshots         int
	CPUMaxThreshold      float64
	MemoryMaxMB          float64
	LatencyMaxMs         float64
	SlowQueryMax         uint64
	ConnMax              uint64
}

type StorageConfig struct {
	Enabled              bool
	BaseDir              string
	RetentionDays        int
	ChunkSize            int
	WriteBufferSize      int
	CompressionType      string
	EnableIndex          bool
	RetentionIntervalMin int
	MetricRetentionDays  int
	LogRetentionDays     int
	TraceRetentionDays   int
	EventRetentionDays   int
	WriteRateMin         int
	QueryLatencyMaxMs    int
}

type AlertConfig struct {
	Enabled               bool                `yaml:"enabled" json:"enabled"`
	EvaluationInterval    time.Duration       `yaml:"evaluation_interval" json:"evaluation_interval"`
	ResolveTimeout        time.Duration       `yaml:"resolve_timeout" json:"resolve_timeout"`
	EnableAutoResolve     bool                `yaml:"enable_auto_resolve" json:"enable_auto_resolve"`
	MaxActiveAlerts       int                 `yaml:"max_active_alerts" json:"max_active_alerts"`
	EnableLatencyAlert    bool                `yaml:"enable_latency_alert" json:"enable_latency_alert"`
	EnablePacketLossAlert bool                `yaml:"enable_packet_loss_alert" json:"enable_packet_loss_alert"`
	EnableRetransmitAlert bool                `yaml:"enable_retransmit_alert" json:"enable_retransmit_alert"`
	EnableCPUAlert        bool                `yaml:"enable_cpu_alert" json:"enable_cpu_alert"`
	EnableMemoryAlert     bool                `yaml:"enable_memory_alert" json:"enable_memory_alert"`
	EnableErrorRateAlert  bool                `yaml:"enable_error_rate_alert" json:"enable_error_rate_alert"`
	NotifyKafka           KafkaNotifyConfig   `yaml:"notify_kafka" json:"notify_kafka"`
	NotifyAPI             APINotifyConfig     `yaml:"notify_api" json:"notify_api"`
	NotifyWebhook         WebhookNotifyConfig `yaml:"notify_webhook" json:"notify_webhook"`
}

type KafkaNotifyConfig struct {
	Enabled bool     `yaml:"enabled" json:"enabled"`
	Brokers []string `yaml:"brokers" json:"brokers"`
	Topic   string   `yaml:"topic" json:"topic"`
}

type APINotifyConfig struct {
	Enabled   bool              `yaml:"enabled" json:"enabled"`
	URL       string            `yaml:"url" json:"url"`
	Headers   map[string]string `yaml:"headers" json:"headers"`
	AuthType  string            `yaml:"auth_type" json:"auth_type"`
	AuthToken string            `yaml:"auth_token" json:"auth_token"`
}

type WebhookNotifyConfig struct {
	Enabled bool              `yaml:"enabled" json:"enabled"`
	URL     string            `yaml:"url" json:"url"`
	Secret  string            `yaml:"secret" json:"secret"`
	Headers map[string]string `yaml:"headers" json:"headers"`
}

type TopologyConfig struct {
	Enabled                bool          `yaml:"enabled" json:"enabled"`
	RefreshInterval        time.Duration `yaml:"refresh_interval" json:"refresh_interval"`
	AutoDiscovery          bool          `yaml:"auto_discovery" json:"auto_discovery"`
	DefaultLayout          string        `yaml:"default_layout" json:"default_layout"`
	MaxNodes               int           `yaml:"max_nodes" json:"max_nodes"`
	IncludePods            bool          `yaml:"include_pods" json:"include_pods"`
	IncludeVMs             bool          `yaml:"include_vms" json:"include_vms"`
	IncludePhysical        bool          `yaml:"include_physical" json:"include_physical"`
	EnableAlertIntegration bool          `yaml:"enable_alert_integration" json:"enable_alert_integration"`
}

type TenantConfig struct {
	Enabled           bool `yaml:"enabled" json:"enabled"`
	MultiTenant       bool `yaml:"multi_tenant" json:"multi_tenant"`
	MaxTenants        int  `yaml:"max_tenants" json:"max_tenants"`
	MaxUsersPerTenant int  `yaml:"max_users_per_tenant" json:"max_users_per_tenant"`
}

type DashboardConfig struct {
	Enabled          bool          `yaml:"enabled" json:"enabled"`
	RefreshInterval  time.Duration `yaml:"refresh_interval" json:"refresh_interval"`
	EnableDrillDown  bool          `yaml:"enable_drill_down" json:"enable_drill_down"`
	MaxAssetsPerPage int           `yaml:"max_assets_per_page" json:"max_assets_per_page"`
}

type MetricsConfig struct {
	Enabled        bool          `yaml:"enabled" json:"enabled"`
	NetworkEnabled bool          `yaml:"network_enabled" json:"network_enabled"`
	AppEnabled     bool          `yaml:"app_enabled" json:"app_enabled"`
	SQLEnabled     bool          `yaml:"sql_enabled" json:"sql_enabled"`
	MaxDataPoints  int           `yaml:"max_data_points" json:"max_data_points"`
	RetentionTime  time.Duration `yaml:"retention_time" json:"retention_time"`
}

type CMDBConfig struct {
	Enabled        bool          `yaml:"enabled" json:"enabled"`
	Type           string        `yaml:"type" json:"type"`
	Endpoint       string        `yaml:"endpoint" json:"endpoint"`
	AuthType       string        `yaml:"auth_type" json:"auth_type"`
	AuthToken      string        `yaml:"auth_token" json:"auth_token"`
	APIKey         string        `yaml:"api_key" json:"api_key"`
	Username       string        `yaml:"username" json:"username"`
	Password       string        `yaml:"password" json:"password"`
	SyncInterval   time.Duration `yaml:"sync_interval" json:"sync_interval"`
	FullSyncStart  bool          `yaml:"full_sync_on_start" json:"full_sync_on_start"`
	Incremental    bool          `yaml:"incremental_sync" json:"incremental_sync"`
	LabelSync      bool          `yaml:"label_sync" json:"label_sync"`
	ConfigSync     bool          `yaml:"config_sync" json:"config_sync"`
	RelationSync   bool          `yaml:"relation_sync" json:"relation_sync"`
	ConflictPolicy string        `yaml:"conflict_policy" json:"conflict_policy"`
	Timeout        time.Duration `yaml:"timeout" json:"timeout"`
	BatchSize      int           `yaml:"batch_size" json:"batch_size"`
}

type TraceConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	SampleRate        float64       `yaml:"sample_rate" json:"sample_rate"`
	MaxTraces         int           `yaml:"max_traces" json:"max_traces"`
	MaxSpansPerTrace  int           `yaml:"max_spans_per_trace" json:"max_spans_per_trace"`
	RetentionTime     time.Duration `yaml:"retention_time" json:"retention_time"`
	EnableCorrelation bool          `yaml:"enable_correlation" json:"enable_correlation"`
	FlameGraphEnabled bool          `yaml:"flamegraph_enabled" json:"flamegraph_enabled"`
	FlameSampleFreq   int           `yaml:"flame_sample_freq" json:"flame_sample_freq"`
	FlameMaxDepth     int           `yaml:"flame_max_depth" json:"flame_max_depth"`
	FlameDurationSec  int           `yaml:"flame_duration_sec" json:"flame_duration_sec"`
	FlameOutputDir    string        `yaml:"flame_output_dir" json:"flame_output_dir"`
	FlameMaxStored    int           `yaml:"flame_max_stored" json:"flame_max_stored"`
}

type EBPFConfig struct {
	Enabled         bool
	TCPMetrics      TCPMetricsConfig
	HTTPMetrics     HTTPMetricsConfig
	BaseTraffic     BaseTrafficConfig
	ProtocolParsing ProtocolParsingConfig
	ResourceLimit   ResourceLimitConfig
	CircuitBreaker  CircuitBreakerConfig
	SelfMonitor     SelfMonitorConfig
	VXLAN           VXLANConfig
	PluginFramework PluginFrameworkConfig
	DropMonitor     DropMonitorConfig
	NTP             NTPConfig
	PerfOptimizer   PerfOptimizerConfig
	CPUProfiler     CPUProfilerConfig
	SQLAggregator   SQLAggregatorConfig
}

type VXLANConfig struct {
	Enabled            bool
	EnableTapMirror    bool
	TapDeviceName      string
	ParseInnerProtocol bool
}

type PluginFrameworkConfig struct {
	Enabled       bool
	PluginDir     string
	AutoDiscovery bool
	CheckInterval time.Duration
	MaxMemoryMB   int
	GRPCTimeout   time.Duration
	EnableBuiltin bool
}

type DropMonitorConfig struct {
	Enabled          bool
	EnableKernelDrop bool
	EnableUserDrop   bool
	RingBufSize      int
	SampleRate       float64
	SnapshotInterval time.Duration
	AlertThreshold   float64
}

type NTPConfig struct {
	Enabled      bool
	Mode         string
	NTPServers   []string
	SyncInterval time.Duration
	MaxOffset    time.Duration
	AdjustStep   bool
	AdjustSlew   bool
}

type Config struct {
	ProbeID              string
	EdgeAddr             string
	MetricsPort          string
	HealthPort           string
	MaxRetries           int
	ConnectTimeout       int
	CollectInterval      int
	BatchSize            int
	APIKey               string
	TLS                  TLSConfig
	Collect              CollectConfig
	Log                  LogConfig
	Network              NetworkConfig
	EBPF                 EBPFConfig
	Storage              StorageConfig
	Alert                AlertConfig
	Topology             TopologyConfig
	Tenant               TenantConfig
	Dashboard            DashboardConfig
	Metrics              MetricsConfig
	CMDB                 CMDBConfig
	Trace                TraceConfig
	FlushInterval        time.Duration
	ReconnectBaseDelay   time.Duration
	ReconnectMaxDelay    time.Duration
	MaxReconnectAttempts int
	MaxBufferLimit       int
}

func Load() (*Config, error) {
	configFile := flag.String("config", "", "配置文件路径")
	flag.Parse()

	viper.SetDefault("probe_id", "")
	viper.SetDefault("edge_addr", "edge:50051")
	viper.SetDefault("metrics_port", "9090")
	viper.SetDefault("health_port", "8080")
	viper.SetDefault("max_retries", 0)
	viper.SetDefault("connect_timeout", 30)
	viper.SetDefault("collect_interval", 10)
	viper.SetDefault("batch_size", 10)
	viper.SetDefault("api_key", "")
	viper.SetDefault("tls.enabled", false)
	viper.SetDefault("tls.server_name", "")
	viper.SetDefault("tls.ca_cert", "")
	viper.SetDefault("tls.client_cert", "")
	viper.SetDefault("tls.client_key", "")
	viper.SetDefault("collect.cpu", true)
	viper.SetDefault("collect.memory", true)
	viper.SetDefault("collect.network", true)
	viper.SetDefault("flush_interval", "30s")
	viper.SetDefault("reconnect_base_delay", "2s")
	viper.SetDefault("reconnect_max_delay", "30s")
	viper.SetDefault("max_reconnect_attempts", 10)
	viper.SetDefault("max_buffer_limit", 1000)
	viper.SetDefault("collect.disk", false)
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")
	viper.SetDefault("network.mgmt_iface", "")
	viper.SetDefault("network.mgmt_ip", "")
	viper.SetDefault("network.local_addr", "")
	viper.SetDefault("network.preferred_source_iface", "")
	viper.SetDefault("ebpf.enabled", true)
	viper.SetDefault("ebpf.tcp_metrics.enabled", true)
	viper.SetDefault("ebpf.tcp_metrics.connect_latency", true)
	viper.SetDefault("ebpf.tcp_metrics.retransmit", true)
	viper.SetDefault("ebpf.tcp_metrics.zero_window", true)
	viper.SetDefault("ebpf.tcp_metrics.queue_overflow", true)
	viper.SetDefault("ebpf.tcp_metrics.connection_fail", true)
	viper.SetDefault("ebpf.http_metrics.enabled", true)
	viper.SetDefault("ebpf.http_metrics.success_rate", true)
	viper.SetDefault("ebpf.http_metrics.response_latency", true)
	viper.SetDefault("ebpf.http_metrics.error_rate", true)
	viper.SetDefault("ebpf.http_metrics.request_count", true)
	viper.SetDefault("ebpf.http_metrics.response_count", true)
	viper.SetDefault("ebpf.base_traffic.enabled", true)
	viper.SetDefault("ebpf.base_traffic.collect_bytes", true)
	viper.SetDefault("ebpf.base_traffic.collect_packets", true)
	viper.SetDefault("ebpf.protocol_parsing.enabled", false)
	viper.SetDefault("ebpf.protocol_parsing.http_full", false)
	viper.SetDefault("ebpf.protocol_parsing.dns_full", false)
	viper.SetDefault("ebpf.protocol_parsing.mysql_full", false)
	viper.SetDefault("ebpf.resource_limit.enabled", true)
	viper.SetDefault("ebpf.resource_limit.max_cpu_core", 1.0)
	viper.SetDefault("ebpf.resource_limit.max_memory_mb", 1024)
	viper.SetDefault("ebpf.resource_limit.max_goroutines", 10000)
	viper.SetDefault("ebpf.resource_limit.use_cgroup", true)
	viper.SetDefault("ebpf.circuit_breaker.enabled", true)
	viper.SetDefault("ebpf.circuit_breaker.max_failures", 3)
	viper.SetDefault("ebpf.circuit_breaker.reset_timeout", 30)
	viper.SetDefault("ebpf.circuit_breaker.silent_duration", 60)
	viper.SetDefault("ebpf.circuit_breaker.check_interval", "3s")
	viper.SetDefault("ebpf.circuit_breaker.cpu_degraded_threshold", 80.0)
	viper.SetDefault("ebpf.circuit_breaker.cpu_silent_threshold", 95.0)
	viper.SetDefault("ebpf.circuit_breaker.mem_degraded_threshold", 90.0)
	viper.SetDefault("ebpf.circuit_breaker.mem_silent_threshold", 95.0)
	viper.SetDefault("ebpf.circuit_breaker.cpu_degraded_duration", "30s")
	viper.SetDefault("ebpf.circuit_breaker.cpu_recover_threshold", 80.0)
	viper.SetDefault("ebpf.circuit_breaker.mem_recover_threshold", 85.0)
	viper.SetDefault("ebpf.circuit_breaker.silent_cpu_recover_threshold", 70.0)
	viper.SetDefault("ebpf.circuit_breaker.silent_mem_recover_threshold", 80.0)
	viper.SetDefault("ebpf.self_monitor.enabled", true)
	viper.SetDefault("ebpf.self_monitor.collect_interval", "10s")
	viper.SetDefault("ebpf.self_monitor.report_interval", "10s")
	viper.SetDefault("ebpf.self_monitor.heartbeat_timeout", "5s")
	viper.SetDefault("ebpf.self_monitor.alert_heartbeat_fail_count", 3)
	viper.SetDefault("ebpf.self_monitor.alert_cpu_percent", 80.0)
	viper.SetDefault("ebpf.self_monitor.alert_memory_percent", 90.0)
	viper.SetDefault("ebpf.self_monitor.alert_packet_drop_rate", 5.0)
	viper.SetDefault("ebpf.self_monitor.alert_report_success_rate_min", 95.0)
	viper.SetDefault("ebpf.vxlan.enabled", false)
	viper.SetDefault("ebpf.vxlan.enable_tap_mirror", false)
	viper.SetDefault("ebpf.vxlan.tap_device_name", "vxlan-tap0")
	viper.SetDefault("ebpf.vxlan.parse_inner_protocol", true)
	viper.SetDefault("ebpf.plugin_framework.enabled", false)
	viper.SetDefault("ebpf.plugin_framework.plugin_dir", "/opt/github.com/meinanzilinzhengying/cloudflow/agent/plugins")
	viper.SetDefault("ebpf.plugin_framework.auto_discovery", true)
	viper.SetDefault("ebpf.plugin_framework.check_interval", "30s")
	viper.SetDefault("ebpf.plugin_framework.max_memory_mb", 256)
	viper.SetDefault("ebpf.plugin_framework.grpc_timeout", "5s")
	viper.SetDefault("ebpf.plugin_framework.enable_builtin", true)
	viper.SetDefault("ebpf.drop_monitor.enabled", false)
	viper.SetDefault("ebpf.drop_monitor.enable_kernel_drop", true)
	viper.SetDefault("ebpf.drop_monitor.enable_user_drop", true)
	viper.SetDefault("ebpf.drop_monitor.ringbuf_size", 262144)
	viper.SetDefault("ebpf.drop_monitor.sample_rate", 1.0)
	viper.SetDefault("ebpf.drop_monitor.snapshot_interval", "10s")
	viper.SetDefault("ebpf.drop_monitor.alert_threshold", 1.0)
	viper.SetDefault("ebpf.ntp.enabled", false)
	viper.SetDefault("ebpf.ntp.mode", "auto")
	viper.SetDefault("ebpf.ntp.ntp_servers", []string{"pool.ntp.org", "time.windows.com"})
	viper.SetDefault("ebpf.ntp.sync_interval", "5m")
	viper.SetDefault("ebpf.ntp.max_offset", "100ms")
	viper.SetDefault("ebpf.ntp.adjust_step", true)
	viper.SetDefault("ebpf.ntp.adjust_slew", true)
	viper.SetDefault("ebpf.perf_optimizer.enabled", true)
	viper.SetDefault("ebpf.perf_optimizer.sample_rate", 1.0)
	viper.SetDefault("ebpf.perf_optimizer.batch_size", 100)
	viper.SetDefault("ebpf.perf_optimizer.max_events_per_sec", 10000)
	viper.SetDefault("ebpf.perf_optimizer.high_load_mode", false)
	viper.SetDefault("ebpf.perf_optimizer.enable_adaptive", true)
	viper.SetDefault("ebpf.cpu_profiler.enabled", false)
	viper.SetDefault("ebpf.cpu_profiler.sample_freq", 99)
	viper.SetDefault("ebpf.cpu_profiler.target_pid", 0)
	viper.SetDefault("ebpf.cpu_profiler.max_stack_depth", 127)
	viper.SetDefault("ebpf.cpu_profiler.duration_sec", 0)
	viper.SetDefault("ebpf.cpu_profiler.output_dir", "/var/log/github.com/meinanzilinzhengying/cloudflow/agent/profiler")
	viper.SetDefault("ebpf.cpu_profiler.auto_detect", true)
	viper.SetDefault("ebpf.sql_aggregator.enabled", false)
	viper.SetDefault("ebpf.sql_aggregator.slow_query_threshold_ms", 1000)
	viper.SetDefault("ebpf.sql_aggregator.enable_correlation", true)
	viper.SetDefault("ebpf.sql_aggregator.max_snapshots", 60)
	viper.SetDefault("ebpf.sql_aggregator.cpu_max_threshold", 80.0)
	viper.SetDefault("ebpf.sql_aggregator.memory_max_mb", 1024.0)
	viper.SetDefault("ebpf.sql_aggregator.latency_max_ms", 1000.0)
	viper.SetDefault("ebpf.sql_aggregator.slow_query_max", 10)
	viper.SetDefault("ebpf.sql_aggregator.conn_max", 100)
	viper.SetDefault("storage.enabled", false)
	viper.SetDefault("storage.base_dir", "/var/lib/github.com/meinanzilinzhengying/cloudflow/agent/storage")
	viper.SetDefault("storage.retention_days", 60)
	viper.SetDefault("storage.chunk_size", 10000)
	viper.SetDefault("storage.write_buffer_size", 50000)
	viper.SetDefault("storage.compression_type", "zstd")
	viper.SetDefault("storage.enable_index", true)
	viper.SetDefault("storage.retention_interval_min", 60)
	viper.SetDefault("storage.metric_retention_days", 60)
	viper.SetDefault("storage.log_retention_days", 30)
	viper.SetDefault("storage.trace_retention_days", 7)
	viper.SetDefault("storage.event_retention_days", 90)
	viper.SetDefault("storage.write_rate_min", 50000)
	viper.SetDefault("storage.query_latency_max_ms", 1000)
	viper.SetDefault("alert.enabled", false)
	viper.SetDefault("alert.evaluation_interval", "1m")
	viper.SetDefault("alert.resolve_timeout", "5m")
	viper.SetDefault("alert.enable_auto_resolve", true)
	viper.SetDefault("alert.max_active_alerts", 1000)
	viper.SetDefault("alert.enable_latency_alert", true)
	viper.SetDefault("alert.enable_packet_loss_alert", true)
	viper.SetDefault("alert.enable_retransmit_alert", true)
	viper.SetDefault("alert.enable_cpu_alert", true)
	viper.SetDefault("alert.enable_memory_alert", true)
	viper.SetDefault("alert.enable_error_rate_alert", true)
	viper.SetDefault("alert.notify_kafka.enabled", false)
	viper.SetDefault("alert.notify_kafka.brokers", []string{"localhost:9092"})
	viper.SetDefault("alert.notify_kafka.topic", "alerts")
	viper.SetDefault("alert.notify_api.enabled", false)
	viper.SetDefault("alert.notify_api.url", "")
	viper.SetDefault("alert.notify_api.auth_type", "")
	viper.SetDefault("alert.notify_webhook.enabled", false)
	viper.SetDefault("alert.notify_webhook.url", "")
	viper.SetDefault("topology.enabled", false)
	viper.SetDefault("topology.refresh_interval", "5m")
	viper.SetDefault("topology.auto_discovery", true)
	viper.SetDefault("topology.default_layout", "vertical")
	viper.SetDefault("topology.max_nodes", 1000)
	viper.SetDefault("topology.include_pods", true)
	viper.SetDefault("topology.include_vms", true)
	viper.SetDefault("topology.include_physical", true)
	viper.SetDefault("topology.enable_alert_integration", true)
	viper.SetDefault("tenant.enabled", false)
	viper.SetDefault("tenant.multi_tenant", false)
	viper.SetDefault("tenant.max_tenants", 100)
	viper.SetDefault("tenant.max_users_per_tenant", 50)
	viper.SetDefault("dashboard.enabled", true)
	viper.SetDefault("dashboard.refresh_interval", "30s")
	viper.SetDefault("dashboard.enable_drill_down", true)
	viper.SetDefault("dashboard.max_assets_per_page", 100)
	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.network_enabled", true)
	viper.SetDefault("metrics.app_enabled", true)
	viper.SetDefault("metrics.sql_enabled", true)
	viper.SetDefault("metrics.max_data_points", 1440)
	viper.SetDefault("metrics.retention_time", "24h")
	viper.SetDefault("cmdb.enabled", false)
	viper.SetDefault("cmdb.type", "http")
	viper.SetDefault("cmdb.endpoint", "")
	viper.SetDefault("cmdb.auth_type", "none")
	viper.SetDefault("cmdb.sync_interval", "5m")
	viper.SetDefault("cmdb.full_sync_on_start", true)
	viper.SetDefault("cmdb.incremental_sync", true)
	viper.SetDefault("cmdb.label_sync", true)
	viper.SetDefault("cmdb.config_sync", true)
	viper.SetDefault("cmdb.relation_sync", true)
	viper.SetDefault("cmdb.conflict_policy", "cmdb_wins")
	viper.SetDefault("cmdb.timeout", "30s")
	viper.SetDefault("cmdb.batch_size", 100)
	viper.SetDefault("trace.enabled", false)
	viper.SetDefault("trace.sample_rate", 1.0)
	viper.SetDefault("trace.max_traces", 10000)
	viper.SetDefault("trace.max_spans_per_trace", 1000)
	viper.SetDefault("trace.retention_time", "24h")
	viper.SetDefault("trace.enable_correlation", true)
	viper.SetDefault("trace.flamegraph_enabled", false)
	viper.SetDefault("trace.flame_sample_freq", 99)
	viper.SetDefault("trace.flame_max_depth", 127)
	viper.SetDefault("trace.flame_duration_sec", 30)
	viper.SetDefault("trace.flame_output_dir", "/var/log/github.com/meinanzilinzhengying/cloudflow/agent/flamegraph")
	viper.SetDefault("trace.flame_max_stored", 100)

	if *configFile != "" {
		abs, err := filepath.Abs(*configFile)
		if err != nil {
			return nil, fmt.Errorf("解析配置文件路径失败: %w", err)
		}
		viper.SetConfigFile(abs)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("./configs")
		viper.AddConfigPath(".")
	}
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	apiKey, err := loadAPIKey()
	if err != nil {
		return nil, fmt.Errorf("加载 API Key 失败: %w", err)
	}

	cfg := &Config{
		ProbeID:         viper.GetString("probe_id"),
		EdgeAddr:        viper.GetString("edge_addr"),
		MetricsPort:     viper.GetString("metrics_port"),
		HealthPort:      viper.GetString("health_port"),
		MaxRetries:      viper.GetInt("max_retries"),
		ConnectTimeout:  viper.GetInt("connect_timeout"),
		CollectInterval: viper.GetInt("collect_interval"),
		BatchSize:       viper.GetInt("batch_size"),
		APIKey:          apiKey,
		TLS: TLSConfig{
			Enabled:    viper.GetBool("tls.enabled"),
			ServerName: viper.GetString("tls.server_name"),
			CACert:     viper.GetString("tls.ca_cert"),
			ClientCert: viper.GetString("tls.client_cert"),
			ClientKey:  viper.GetString("tls.client_key"),
		},
		Collect: CollectConfig{
			CPU:     viper.GetBool("collect.cpu"),
			Memory:  viper.GetBool("collect.memory"),
			Network: viper.GetBool("collect.network"),
			Disk:    viper.GetBool("collect.disk"),
		},
		Log: LogConfig{
			Level:  viper.GetString("log.level"),
			Format: viper.GetString("log.format"),
		},
		Network: NetworkConfig{
			MgmtIface:            viper.GetString("network.mgmt_iface"),
			MgmtIP:               viper.GetString("network.mgmt_ip"),
			LocalAddr:            viper.GetString("network.local_addr"),
			PreferredSourceIface: viper.GetString("network.preferred_source_iface"),
		},
		EBPF: EBPFConfig{
			Enabled: viper.GetBool("ebpf.enabled"),
			TCPMetrics: TCPMetricsConfig{
				Enabled:        viper.GetBool("ebpf.tcp_metrics.enabled"),
				ConnectLatency: viper.GetBool("ebpf.tcp_metrics.connect_latency"),
				Retransmit:     viper.GetBool("ebpf.tcp_metrics.retransmit"),
				ZeroWindow:     viper.GetBool("ebpf.tcp_metrics.zero_window"),
				QueueOverflow:  viper.GetBool("ebpf.tcp_metrics.queue_overflow"),
				ConnectionFail: viper.GetBool("ebpf.tcp_metrics.connection_fail"),
			},
			HTTPMetrics: HTTPMetricsConfig{
				Enabled:         viper.GetBool("ebpf.http_metrics.enabled"),
				SuccessRate:     viper.GetBool("ebpf.http_metrics.success_rate"),
				ResponseLatency: viper.GetBool("ebpf.http_metrics.response_latency"),
				ErrorRate:       viper.GetBool("ebpf.http_metrics.error_rate"),
				RequestCount:    viper.GetBool("ebpf.http_metrics.request_count"),
				ResponseCount:   viper.GetBool("ebpf.http_metrics.response_count"),
			},
			BaseTraffic: BaseTrafficConfig{
				Enabled:        viper.GetBool("ebpf.base_traffic.enabled"),
				CollectBytes:   viper.GetBool("ebpf.base_traffic.collect_bytes"),
				CollectPackets: viper.GetBool("ebpf.base_traffic.collect_packets"),
			},
			ProtocolParsing: ProtocolParsingConfig{
				Enabled:   viper.GetBool("ebpf.protocol_parsing.enabled"),
				HTTPFull:  viper.GetBool("ebpf.protocol_parsing.http_full"),
				DNSFull:   viper.GetBool("ebpf.protocol_parsing.dns_full"),
				MySQLFull: viper.GetBool("ebpf.protocol_parsing.mysql_full"),
			},
			ResourceLimit: ResourceLimitConfig{
				Enabled:       viper.GetBool("ebpf.resource_limit.enabled"),
				MaxCPUCore:    viper.GetFloat64("ebpf.resource_limit.max_cpu_core"),
				MaxMemoryMB:   viper.GetFloat64("ebpf.resource_limit.max_memory_mb"),
				MaxGoroutines: viper.GetInt("ebpf.resource_limit.max_goroutines"),
				UseCgroup:     viper.GetBool("ebpf.resource_limit.use_cgroup"),
			},
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:                   viper.GetBool("ebpf.circuit_breaker.enabled"),
				MaxFailures:               viper.GetInt("ebpf.circuit_breaker.max_failures"),
				ResetTimeout:              parseSecondsAsDuration(viper.Get("ebpf.circuit_breaker.reset_timeout")),
				SilentDuration:            parseSecondsAsDuration(viper.Get("ebpf.circuit_breaker.silent_duration")),
				CheckInterval:             viper.GetDuration("ebpf.circuit_breaker.check_interval"),
				CPUDegradedThreshold:      viper.GetFloat64("ebpf.circuit_breaker.cpu_degraded_threshold"),
				CPUSilentThreshold:        viper.GetFloat64("ebpf.circuit_breaker.cpu_silent_threshold"),
				MemDegradedThreshold:      viper.GetFloat64("ebpf.circuit_breaker.mem_degraded_threshold"),
				MemSilentThreshold:        viper.GetFloat64("ebpf.circuit_breaker.mem_silent_threshold"),
				CPUDegradedDuration:       viper.GetDuration("ebpf.circuit_breaker.cpu_degraded_duration"),
				CPURecoverThreshold:       viper.GetFloat64("ebpf.circuit_breaker.cpu_recover_threshold"),
				MemRecoverThreshold:       viper.GetFloat64("ebpf.circuit_breaker.mem_recover_threshold"),
				SilentCPURecoverThreshold: viper.GetFloat64("ebpf.circuit_breaker.silent_cpu_recover_threshold"),
				SilentMemRecoverThreshold: viper.GetFloat64("ebpf.circuit_breaker.silent_mem_recover_threshold"),
			},
			SelfMonitor: SelfMonitorConfig{
				Enabled:                   viper.GetBool("ebpf.self_monitor.enabled"),
				CollectInterval:           viper.GetDuration("ebpf.self_monitor.collect_interval"),
				ReportInterval:            viper.GetDuration("ebpf.self_monitor.report_interval"),
				HeartbeatTimeout:          viper.GetDuration("ebpf.self_monitor.heartbeat_timeout"),
				AlertHeartbeatFailCount:   viper.GetInt("ebpf.self_monitor.alert_heartbeat_fail_count"),
				AlertCPUPercent:           viper.GetFloat64("ebpf.self_monitor.alert_cpu_percent"),
				AlertMemoryPercent:        viper.GetFloat64("ebpf.self_monitor.alert_memory_percent"),
				AlertPacketDropRate:       viper.GetFloat64("ebpf.self_monitor.alert_packet_drop_rate"),
				AlertReportSuccessRateMin: viper.GetFloat64("ebpf.self_monitor.alert_report_success_rate_min"),
			},
			VXLAN: VXLANConfig{
				Enabled:            viper.GetBool("ebpf.vxlan.enabled"),
				EnableTapMirror:    viper.GetBool("ebpf.vxlan.enable_tap_mirror"),
				TapDeviceName:      viper.GetString("ebpf.vxlan.tap_device_name"),
				ParseInnerProtocol: viper.GetBool("ebpf.vxlan.parse_inner_protocol"),
			},
			PluginFramework: PluginFrameworkConfig{
				Enabled:       viper.GetBool("ebpf.plugin_framework.enabled"),
				PluginDir:     viper.GetString("ebpf.plugin_framework.plugin_dir"),
				AutoDiscovery: viper.GetBool("ebpf.plugin_framework.auto_discovery"),
				CheckInterval: viper.GetDuration("ebpf.plugin_framework.check_interval"),
				MaxMemoryMB:   viper.GetInt("ebpf.plugin_framework.max_memory_mb"),
				GRPCTimeout:   viper.GetDuration("ebpf.plugin_framework.grpc_timeout"),
				EnableBuiltin: viper.GetBool("ebpf.plugin_framework.enable_builtin"),
			},
			DropMonitor: DropMonitorConfig{
				Enabled:          viper.GetBool("ebpf.drop_monitor.enabled"),
				EnableKernelDrop: viper.GetBool("ebpf.drop_monitor.enable_kernel_drop"),
				EnableUserDrop:   viper.GetBool("ebpf.drop_monitor.enable_user_drop"),
				RingBufSize:      viper.GetInt("ebpf.drop_monitor.ringbuf_size"),
				SampleRate:       viper.GetFloat64("ebpf.drop_monitor.sample_rate"),
				SnapshotInterval: viper.GetDuration("ebpf.drop_monitor.snapshot_interval"),
				AlertThreshold:   viper.GetFloat64("ebpf.drop_monitor.alert_threshold"),
			},
			NTP: NTPConfig{
				Enabled:      viper.GetBool("ebpf.ntp.enabled"),
				Mode:         viper.GetString("ebpf.ntp.mode"),
				NTPServers:   viper.GetStringSlice("ebpf.ntp.ntp_servers"),
				SyncInterval: viper.GetDuration("ebpf.ntp.sync_interval"),
				MaxOffset:    viper.GetDuration("ebpf.ntp.max_offset"),
				AdjustStep:   viper.GetBool("ebpf.ntp.adjust_step"),
				AdjustSlew:   viper.GetBool("ebpf.ntp.adjust_slew"),
			},
			PerfOptimizer: PerfOptimizerConfig{
				Enabled:         viper.GetBool("ebpf.perf_optimizer.enabled"),
				SampleRate:      viper.GetFloat64("ebpf.perf_optimizer.sample_rate"),
				BatchSize:       viper.GetInt("ebpf.perf_optimizer.batch_size"),
				MaxEventsPerSec: viper.GetInt("ebpf.perf_optimizer.max_events_per_sec"),
				HighLoadMode:    viper.GetBool("ebpf.perf_optimizer.high_load_mode"),
				EnableAdaptive:  viper.GetBool("ebpf.perf_optimizer.enable_adaptive"),
			},
			CPUProfiler: CPUProfilerConfig{
				Enabled:       viper.GetBool("ebpf.cpu_profiler.enabled"),
				SampleFreq:    viper.GetInt("ebpf.cpu_profiler.sample_freq"),
				TargetPID:     uint32(viper.GetInt("ebpf.cpu_profiler.target_pid")),
				MaxStackDepth: viper.GetInt("ebpf.cpu_profiler.max_stack_depth"),
				DurationSec:   viper.GetInt("ebpf.cpu_profiler.duration_sec"),
				OutputDir:     viper.GetString("ebpf.cpu_profiler.output_dir"),
				AutoDetect:    viper.GetBool("ebpf.cpu_profiler.auto_detect"),
			},
			SQLAggregator: SQLAggregatorConfig{
				Enabled:              viper.GetBool("ebpf.sql_aggregator.enabled"),
				SlowQueryThresholdMs: viper.GetUint64("ebpf.sql_aggregator.slow_query_threshold_ms"),
				EnableCorrelation:    viper.GetBool("ebpf.sql_aggregator.enable_correlation"),
				MaxSnapshots:         viper.GetInt("ebpf.sql_aggregator.max_snapshots"),
				CPUMaxThreshold:      viper.GetFloat64("ebpf.sql_aggregator.cpu_max_threshold"),
				MemoryMaxMB:          viper.GetFloat64("ebpf.sql_aggregator.memory_max_mb"),
				LatencyMaxMs:         viper.GetFloat64("ebpf.sql_aggregator.latency_max_ms"),
				SlowQueryMax:         viper.GetUint64("ebpf.sql_aggregator.slow_query_max"),
				ConnMax:              viper.GetUint64("ebpf.sql_aggregator.conn_max"),
			},
		},
		Storage: StorageConfig{
			Enabled:              viper.GetBool("storage.enabled"),
			BaseDir:              viper.GetString("storage.base_dir"),
			RetentionDays:        viper.GetInt("storage.retention_days"),
			ChunkSize:            viper.GetInt("storage.chunk_size"),
			WriteBufferSize:      viper.GetInt("storage.write_buffer_size"),
			CompressionType:      viper.GetString("storage.compression_type"),
			EnableIndex:          viper.GetBool("storage.enable_index"),
			RetentionIntervalMin: viper.GetInt("storage.retention_interval_min"),
			MetricRetentionDays:  viper.GetInt("storage.metric_retention_days"),
			LogRetentionDays:     viper.GetInt("storage.log_retention_days"),
			TraceRetentionDays:   viper.GetInt("storage.trace_retention_days"),
			EventRetentionDays:   viper.GetInt("storage.event_retention_days"),
			WriteRateMin:         viper.GetInt("storage.write_rate_min"),
			QueryLatencyMaxMs:    viper.GetInt("storage.query_latency_max_ms"),
		},
		Alert: AlertConfig{
			Enabled:               viper.GetBool("alert.enabled"),
			EvaluationInterval:    viper.GetDuration("alert.evaluation_interval"),
			ResolveTimeout:        viper.GetDuration("alert.resolve_timeout"),
			EnableAutoResolve:     viper.GetBool("alert.enable_auto_resolve"),
			MaxActiveAlerts:       viper.GetInt("alert.max_active_alerts"),
			EnableLatencyAlert:    viper.GetBool("alert.enable_latency_alert"),
			EnablePacketLossAlert: viper.GetBool("alert.enable_packet_loss_alert"),
			EnableRetransmitAlert: viper.GetBool("alert.enable_retransmit_alert"),
			EnableCPUAlert:        viper.GetBool("alert.enable_cpu_alert"),
			EnableMemoryAlert:     viper.GetBool("alert.enable_memory_alert"),
			EnableErrorRateAlert:  viper.GetBool("alert.enable_error_rate_alert"),
			NotifyKafka: KafkaNotifyConfig{
				Enabled: viper.GetBool("alert.notify_kafka.enabled"),
				Brokers: viper.GetStringSlice("alert.notify_kafka.brokers"),
				Topic:   viper.GetString("alert.notify_kafka.topic"),
			},
			NotifyAPI: APINotifyConfig{
				Enabled:   viper.GetBool("alert.notify_api.enabled"),
				URL:       viper.GetString("alert.notify_api.url"),
				Headers:   viper.GetStringMapString("alert.notify_api.headers"),
				AuthType:  viper.GetString("alert.notify_api.auth_type"),
				AuthToken: viper.GetString("alert.notify_api.auth_token"),
			},
			NotifyWebhook: WebhookNotifyConfig{
				Enabled: viper.GetBool("alert.notify_webhook.enabled"),
				URL:     viper.GetString("alert.notify_webhook.url"),
				Secret:  viper.GetString("alert.notify_webhook.secret"),
				Headers: viper.GetStringMapString("alert.notify_webhook.headers"),
			},
		},
		Topology: TopologyConfig{
			Enabled:                viper.GetBool("topology.enabled"),
			RefreshInterval:        viper.GetDuration("topology.refresh_interval"),
			AutoDiscovery:          viper.GetBool("topology.auto_discovery"),
			DefaultLayout:          viper.GetString("topology.default_layout"),
			MaxNodes:               viper.GetInt("topology.max_nodes"),
			IncludePods:            viper.GetBool("topology.include_pods"),
			IncludeVMs:             viper.GetBool("topology.include_vms"),
			IncludePhysical:        viper.GetBool("topology.include_physical"),
			EnableAlertIntegration: viper.GetBool("topology.enable_alert_integration"),
		},
		Tenant: TenantConfig{
			Enabled:           viper.GetBool("tenant.enabled"),
			MultiTenant:       viper.GetBool("tenant.multi_tenant"),
			MaxTenants:        viper.GetInt("tenant.max_tenants"),
			MaxUsersPerTenant: viper.GetInt("tenant.max_users_per_tenant"),
		},
		Dashboard: DashboardConfig{
			Enabled:          viper.GetBool("dashboard.enabled"),
			RefreshInterval:  viper.GetDuration("dashboard.refresh_interval"),
			EnableDrillDown:  viper.GetBool("dashboard.enable_drill_down"),
			MaxAssetsPerPage: viper.GetInt("dashboard.max_assets_per_page"),
		},
		Metrics: MetricsConfig{
			Enabled:        viper.GetBool("metrics.enabled"),
			NetworkEnabled: viper.GetBool("metrics.network_enabled"),
			AppEnabled:     viper.GetBool("metrics.app_enabled"),
			SQLEnabled:     viper.GetBool("metrics.sql_enabled"),
			MaxDataPoints:  viper.GetInt("metrics.max_data_points"),
			RetentionTime:  viper.GetDuration("metrics.retention_time"),
		},
		CMDB: CMDBConfig{
			Enabled:        viper.GetBool("cmdb.enabled"),
			Type:           viper.GetString("cmdb.type"),
			Endpoint:       viper.GetString("cmdb.endpoint"),
			AuthType:       viper.GetString("cmdb.auth_type"),
			AuthToken:      viper.GetString("cmdb.auth_token"),
			APIKey:         viper.GetString("cmdb.api_key"),
			Username:       viper.GetString("cmdb.username"),
			Password:       viper.GetString("cmdb.password"),
			SyncInterval:   viper.GetDuration("cmdb.sync_interval"),
			FullSyncStart:  viper.GetBool("cmdb.full_sync_on_start"),
			Incremental:    viper.GetBool("cmdb.incremental_sync"),
			LabelSync:      viper.GetBool("cmdb.label_sync"),
			ConfigSync:     viper.GetBool("cmdb.config_sync"),
			RelationSync:   viper.GetBool("cmdb.relation_sync"),
			ConflictPolicy: viper.GetString("cmdb.conflict_policy"),
			Timeout:        viper.GetDuration("cmdb.timeout"),
			BatchSize:      viper.GetInt("cmdb.batch_size"),
		},
		Trace: TraceConfig{
			Enabled:           viper.GetBool("trace.enabled"),
			SampleRate:        viper.GetFloat64("trace.sample_rate"),
			MaxTraces:         viper.GetInt("trace.max_traces"),
			MaxSpansPerTrace:  viper.GetInt("trace.max_spans_per_trace"),
			RetentionTime:     viper.GetDuration("trace.retention_time"),
			EnableCorrelation: viper.GetBool("trace.enable_correlation"),
			FlameGraphEnabled: viper.GetBool("trace.flamegraph_enabled"),
			FlameSampleFreq:   viper.GetInt("trace.flame_sample_freq"),
			FlameMaxDepth:     viper.GetInt("trace.flame_max_depth"),
			FlameDurationSec:  viper.GetInt("trace.flame_duration_sec"),
			FlameOutputDir:    viper.GetString("trace.flame_output_dir"),
			FlameMaxStored:    viper.GetInt("trace.flame_max_stored"),
		},
		FlushInterval:        viper.GetDuration("flush_interval"),
		ReconnectBaseDelay:   viper.GetDuration("reconnect_base_delay"),
		ReconnectMaxDelay:    viper.GetDuration("reconnect_max_delay"),
		MaxReconnectAttempts: viper.GetInt("max_reconnect_attempts"),
		MaxBufferLimit:       viper.GetInt("max_buffer_limit"),
	}

	if cfg.ProbeID == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "probe-unknown"
		}
		cfg.ProbeID = fmt.Sprintf("probe-%s", hostname)
	}

	return cfg, nil
}

func (c *Config) Summary() string {
	apiKeyMasked := maskSecret(c.APIKey)
	return fmt.Sprintf("ProbeID=%s, EdgeAddr=%s, Interval=%ds, BatchSize=%d, APIKey=%s, CPU=%v, Mem=%v, Net=%v, MgmtIface=%s, EBPF=%v, ResourceLimit=%v",
		c.ProbeID, c.EdgeAddr, c.CollectInterval, c.BatchSize, apiKeyMasked,
		c.Collect.CPU, c.Collect.Memory, c.Collect.Network, c.Network.MgmtIface, c.EBPF.Enabled, c.EBPF.ResourceLimit.Enabled)
}

func maskSecret(s string) string {
	return utils.MaskSecret(s)
}

func parseSecondsAsDuration(val interface{}) time.Duration {
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v) * time.Second
	case string:
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		var secs int
		if _, err := fmt.Sscanf(v, "%d", &secs); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return 0
}
