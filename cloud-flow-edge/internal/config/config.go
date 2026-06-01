// Package config 提供配置加载功能
// 支持从 config.yaml 文件和环境变量读取配置，环境变量优先级更高
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"

	"cloud-flow-edge/pkg/logger"
	"cloud-flow/pkg/utils"
)

// LogConfig 日志配置
type LogConfig struct {
	Level  string // 日志级别: debug, info, warn, error
	Format string // 日志格式: json, console
}

// TLSConfig TLS 配置
type TLSConfig struct {
	Enabled    bool   // 是否启用 TLS
	CACert     string // CA 证书路径
	ClientCert string // 客户端证书路径（mTLS 可选）
	ClientKey  string // 客户端私钥路径（mTLS 可选）
	ServerCert string // 服务端证书路径
	ServerKey  string // 服务端私钥路径
	ServerName string // TLS 握手验证域名
}

// ServiceDiscoveryConfig 服务发现配置
type ServiceDiscoveryConfig struct {
	Enabled         bool     // 是否启用服务发现
	Type            string   // 服务发现类型: etcd, consul, dns, nacos
	Endpoints       []string // 服务发现端点
	ServiceName     string   // 服务名称
	RefreshInterval int      // 刷新间隔（秒）
	Port            int      // 服务端口号（用于 DNS 发现）
	Namespace       string   // Nacos命名空间
	Group           string   // Nacos服务分组
}

// ClusterConfig Edge集群配置
type ClusterConfig struct {
	Enabled                bool          // 启用集群模式
	NodeID                 string        // 当前节点ID
	ServiceName            string        // 集群服务名称（用于服务发现）
	HeartbeatInterval      time.Duration // 心跳间隔
	HealthCheckTimeout     time.Duration // 健康检查超时
	LoadBalanceStrategy    string        // 负载均衡策略: least-connections/round-robin/consistent-hash
	MaxConnectionsPerNode  int           // 每个节点最大连接数
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Enabled    bool // 是否启用限流
	BucketSize int  // 令牌桶容量
	RefillRate int  // 每秒填充令牌数
}

// ConnectionPoolConfig 连接池配置
type ConnectionPoolConfig struct {
	MaxConnections  int           // 最大连接数（默认10000）
	StaleTimeout    time.Duration // 连接过期超时（默认5分钟）
	CleanupInterval time.Duration // 清理间隔（默认1分钟）
}

// IPLimitConfig 单IP限流配置
type IPLimitConfig struct {
	MaxQPSPerIP     int           // 每IP最大QPS（默认100）
	BurstSize       int           // 突发大小（默认200）
	CleanupInterval time.Duration // 清理间隔（默认1分钟）
	StaleDuration   time.Duration // IP过期时间（默认10分钟）
}

// GoPoolConfig goroutine池配置
type GoPoolConfig struct {
	Workers  int // worker数量（默认500）
	QueueCap int // 任务队列容量（默认10000）
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	FailureThreshold    float64       // 失败率阈值（0-1，默认0.5）
	MinRequests         int           // 最小请求数（默认100）
	RecoveryTimeout     time.Duration // 恢复超时（默认30s）
	HalfOpenMaxRequests int           // 半开状态最大请求数（默认10）
}

// KafkaConfig Kafka 配置（P0: Flow Ingest Pipeline）
type KafkaConfig struct {
	Enabled           bool     // 启用 Kafka 转发（默认 false，兼容旧模式）
	Brokers           []string // Kafka broker 地址列表
	Compression       string   // 压缩算法: snappy/gzip/lz4/zstd
	MaxMessageBytes   int      // 单条消息最大字节数
	BatchSize         int      // 批量发送大小
	LingerMs          int      // 批量等待时间（毫秒）
	BufferMemory      int      // Producer 缓冲区大小（字节）
	Retries           int      // 发送重试次数
	RetryBackoffMs    int      // 重试基础延迟（毫秒）
	ChannelBufferSize int      // 内部通道缓冲区大小
}

// AggregatorConfig 数据聚合配置
type AggregatorConfig struct {
	Enabled           bool          // 启用数据聚合（默认启用）
	WindowSize        time.Duration // 聚合窗口大小（默认1分钟）
	MaxWindowCount    int           // 最大窗口数量（默认10个）
	Deduplication     bool          // 启用去重（默认启用）
	FilterInvalid     bool          // 启用无效数据过滤（默认启用）
	MinCompression    float64       // 最小压缩率要求（默认0.5，即50%）
}

// AuthConfig Agent接入鉴权配置
type AuthConfig struct {
	Enabled          bool          // 启用鉴权（默认启用）
	RequireMTLS      bool          // 要求mTLS双向证书认证
	RequireToken     bool          // 要求Agent Token认证
	RequireWhitelist bool          // 要求白名单校验
	TokenHeader      string        // Token metadata header键名
	RejectLog        bool          // 记录被拒绝连接的详细日志
	WhitelistSyncInterval time.Duration // Center白名单同步间隔
}

// Config 边缘节点服务配置
type Config struct {
	EdgeNodeID       string                 // 边缘节点唯一标识
	GRPCListenPort   int                    // gRPC 监听端口
	MetricsPort      int                    // Prometheus metrics HTTP 端口
	HealthPort       int                    // 健康检查 HTTP 端口
	CenterAddr       string                 // 中心服务地址
	ServiceDiscovery ServiceDiscoveryConfig // 服务发现配置
	CloudPlatform    string                 // 云平台标识
	Region           string                 // 区域标识
	BatchSize        int                    // 批量转发大小
	FlushInterval    int                    // 刷新间隔（秒）
	APIKey           string                 // API Key（探针注册时需携带）
	CenterAPIKey     string                 // Center API Key（转发数据到 Center 时需携带）
	RateLimit        RateLimitConfig        // 限流配置
	ConnectionPool   ConnectionPoolConfig   // 连接池配置
	IPLimit          IPLimitConfig          // 单IP限流配置
	GoPool           GoPoolConfig           // goroutine池配置
	CircuitBreaker   CircuitBreakerConfig   // 熔断器配置
	Aggregator       AggregatorConfig       // 数据聚合配置
	Auth             AuthConfig             // Agent接入鉴权配置
	Cluster          ClusterConfig          // Edge集群配置
	TLS              TLSConfig              // TLS 配置
	Log              LogConfig              // 日志配置
	Kafka            KafkaConfig            // P0: Kafka 配置（Flow Ingest Pipeline）

	// L1 修复: 可配置的定时任务间隔
	HeartbeatInterval       time.Duration // 心跳上报间隔（默认 30s）
	ProbeReportInterval     time.Duration // 探针列表上报间隔（默认 60s）
	SelfMonitorInterval     time.Duration // 自监控上报间隔（默认 30s）
	MaxBufferLimit          int           // 转发缓冲区上限（默认 1000）
	ReconnectBaseDelay      time.Duration // 重连基础延迟（默认 2s）
	ReconnectMaxDelay       time.Duration // 重连最大延迟（默认 30s）
	MaxReconnectAttempts    int           // 最大重连次数（默认 10）
}

// loadAPIKey 从环境变量或配置文件加载 API Key
// 支持以下方式（按优先级排序）：
// 1. CLOUD_FLOW_API_KEY_FILE - 从文件读取
// 2. CLOUD_FLOW_API_KEY - 直接环境变量
// 3. VAULT_ADDR + VAULT_PATH - 从 HashiCorp Vault 读取
// 4. config.yaml - 配置文件（仅开发环境）
// 5. 自动生成（仅用于开发环境）
func loadAPIKey() (string, error) {
	// 1. 从文件读取（推荐用于 Docker Secrets 或 K8s Secrets）
	if keyFile := os.Getenv("CLOUD_FLOW_API_KEY_FILE"); keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return "", fmt.Errorf("从文件读取 API Key 失败: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// 2. 直接环境变量
	if apiKey := os.Getenv("CLOUD_FLOW_API_KEY"); apiKey != "" {
		return apiKey, nil
	}

	// 3. 从 Vault 读取（企业级方案）
	if vaultAddr := os.Getenv("VAULT_ADDR"); vaultAddr != "" {
		vaultPath := os.Getenv("VAULT_PATH")
		if vaultPath == "" {
			vaultPath = "secret/data/cloud-flow/api-key"
		}
		// 这里可以实现 Vault 客户端调用
		// 简化处理：如果配置了 Vault，但无法读取，返回错误
		return "", fmt.Errorf("Vault 集成需要实现 Vault 客户端")
	}

	// 4. 从配置文件读取（仅开发环境）
	apiKey := viper.GetString("api_key")
	if apiKey != "" {
		// 生产环境警告
		if os.Getenv("CLOUD_FLOW_ENV") != "development" {
			fmt.Fprintln(os.Stderr, "⚠️  安全警告: API Key 从配置文件加载，仅建议用于开发环境")
		}
		return apiKey, nil
	}

	// 5. 自动生成（仅用于开发环境）
	if os.Getenv("CLOUD_FLOW_ENV") != "development" {
		return "", fmt.Errorf("生产环境必须配置 API Key")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成 API Key 失败: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// loadCenterAPIKey 从环境变量或配置文件加载 Center API Key
// 支持以下方式（按优先级排序）：
// 1. CLOUD_FLOW_CENTER_API_KEY - 直接环境变量
// 2. config.yaml - 配置文件（仅开发环境）
func loadCenterAPIKey() (string, error) {
	// 1. 直接环境变量
	if apiKey := os.Getenv("CLOUD_FLOW_CENTER_API_KEY"); apiKey != "" {
		return apiKey, nil
	}

	// 2. 从配置文件读取（仅开发环境）
	apiKey := viper.GetString("center_api_key")
	if apiKey != "" {
		// 生产环境警告
		if os.Getenv("CLOUD_FLOW_ENV") != "development" {
			fmt.Fprintln(os.Stderr, "⚠️  安全警告: Center API Key 从配置文件加载，仅建议用于开发环境")
		}
		return apiKey, nil
	}

	return "", nil
}

// Load 加载配置
// 优先级：环境变量 > .env 文件 > config.yaml > 默认值
func Load() (*Config, error) {
	// 尝试加载 .env 文件（忽略错误，文件可能不存在）
	_ = godotenv.Load()

	// 设置 viper 默认值
	viper.SetDefault("edge_node_id", "")
	viper.SetDefault("grpc_listen_port", 50051)
	viper.SetDefault("metrics_port", 9092)
	viper.SetDefault("health_port", 8081)
	viper.SetDefault("center_addr", "localhost:9090")
	viper.SetDefault("cloud_platform", "onprem")
	viper.SetDefault("region", "local")
	viper.SetDefault("batch_size", 100)
	viper.SetDefault("flush_interval", 5)
	viper.SetDefault("tls.enabled", true)
	viper.SetDefault("tls.ca_cert", "")
	viper.SetDefault("tls.client_cert", "")
	viper.SetDefault("tls.client_key", "")
	viper.SetDefault("tls.server_cert", "")
	viper.SetDefault("tls.server_key", "")
	viper.SetDefault("tls.server_name", "")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")

	// 限流配置默认值
	viper.SetDefault("rate_limit.enabled", true)
	viper.SetDefault("rate_limit.bucket_size", 100)
	viper.SetDefault("rate_limit.refill_rate", 50)

	// 连接池配置默认值
	viper.SetDefault("connection_pool.max_connections", 10000)
	viper.SetDefault("connection_pool.stale_timeout", "5m")
	viper.SetDefault("connection_pool.cleanup_interval", "1m")

	// 单IP限流配置默认值
	viper.SetDefault("ip_limit.max_qps_per_ip", 100)
	viper.SetDefault("ip_limit.burst_size", 200)
	viper.SetDefault("ip_limit.cleanup_interval", "1m")
	viper.SetDefault("ip_limit.stale_duration", "10m")

	// goroutine池配置默认值
	viper.SetDefault("go_pool.workers", 500)
	viper.SetDefault("go_pool.queue_cap", 10000)

	// 熔断器配置默认值
	viper.SetDefault("circuit_breaker.failure_threshold", 0.5)
	viper.SetDefault("circuit_breaker.min_requests", 100)
	viper.SetDefault("circuit_breaker.recovery_timeout", "30s")
	viper.SetDefault("circuit_breaker.half_open_max_requests", 10)

	// 数据聚合配置默认值
	viper.SetDefault("aggregator.enabled", true)
	viper.SetDefault("aggregator.window_size", "1m")
	viper.SetDefault("aggregator.max_window_count", 10)
	viper.SetDefault("aggregator.deduplication", true)
	viper.SetDefault("aggregator.filter_invalid", true)
	viper.SetDefault("aggregator.min_compression", 0.5)

	// Agent接入鉴权配置默认值
	viper.SetDefault("auth.enabled", true)
	viper.SetDefault("auth.require_mtls", false)
	viper.SetDefault("auth.require_token", true)
	viper.SetDefault("auth.require_whitelist", false)
	viper.SetDefault("auth.token_header", "x-agent-token")
	viper.SetDefault("auth.reject_log", true)
	viper.SetDefault("auth.whitelist_sync_interval", "30s")

	// Edge集群配置默认值
	viper.SetDefault("cluster.enabled", false)
	viper.SetDefault("cluster.node_id", "")
	viper.SetDefault("cluster.service_name", "cloud-flow-edge")
	viper.SetDefault("cluster.heartbeat_interval", "5s")
	viper.SetDefault("cluster.health_check_timeout", "3s")
	viper.SetDefault("cluster.load_balance_strategy", "least-connections")
	viper.SetDefault("cluster.max_connections_per_node", 10000)

	viper.SetDefault("service_discovery.enabled", false)
	viper.SetDefault("service_discovery.type", "etcd")
	viper.SetDefault("service_discovery.endpoints", []string{"localhost:2379"})
	viper.SetDefault("service_discovery.service_name", "cloud-flow-center")
	viper.SetDefault("service_discovery.refresh_interval", 30)
	viper.SetDefault("service_discovery.port", 9090)

	// L1 修复: 可配置的定时任务间隔默认值
	viper.SetDefault("heartbeat_interval", "30s")
	viper.SetDefault("probe_report_interval", "60s")
	viper.SetDefault("self_monitor_interval", "30s")
	viper.SetDefault("max_buffer_limit", 1000)
	viper.SetDefault("reconnect_base_delay", "2s")
	viper.SetDefault("reconnect_max_delay", "30s")
	viper.SetDefault("max_reconnect_attempts", 10)

	// 尝试读取 config.yaml
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		// 配置文件不存在时不报错，仅使用默认值和环境变量
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	// 环境变量覆盖（支持大写+下划线格式）
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 安全加载 API Key
	apiKey, err := loadAPIKey()
	if err != nil {
		return nil, fmt.Errorf("加载 API Key 失败: %w", err)
	}

	// 安全加载 Center API Key
	centerAPIKey, err := loadCenterAPIKey()
	if err != nil {
		return nil, fmt.Errorf("加载 Center API Key 失败: %w", err)
	}

	cfg := &Config{
		EdgeNodeID:     viper.GetString("edge_node_id"),
		GRPCListenPort: viper.GetInt("grpc_listen_port"),
		MetricsPort:    viper.GetInt("metrics_port"),
		HealthPort:     viper.GetInt("health_port"),
		CenterAddr:     viper.GetString("center_addr"),
		ServiceDiscovery: ServiceDiscoveryConfig{
			Enabled:         viper.GetBool("service_discovery.enabled"),
			Type:            viper.GetString("service_discovery.type"),
			Port:            viper.GetInt("service_discovery.port"),
			Endpoints:       viper.GetStringSlice("service_discovery.endpoints"),
			ServiceName:     viper.GetString("service_discovery.service_name"),
			RefreshInterval: viper.GetInt("service_discovery.refresh_interval"),
		},
		CloudPlatform: viper.GetString("cloud_platform"),
		Region:        viper.GetString("region"),
		BatchSize:     viper.GetInt("batch_size"),
		FlushInterval: viper.GetInt("flush_interval"),
		APIKey:        apiKey,
		CenterAPIKey:  centerAPIKey,
		TLS: TLSConfig{
			Enabled:    viper.GetBool("tls.enabled"),
			CACert:     viper.GetString("tls.ca_cert"),
			ClientCert: viper.GetString("tls.client_cert"),
			ClientKey:  viper.GetString("tls.client_key"),
			ServerCert: viper.GetString("tls.server_cert"),
			ServerKey:  viper.GetString("tls.server_key"),
			ServerName: viper.GetString("tls.server_name"),
		},
		RateLimit: RateLimitConfig{
			Enabled:    viper.GetBool("rate_limit.enabled"),
			BucketSize: viper.GetInt("rate_limit.bucket_size"),
			RefillRate: viper.GetInt("rate_limit.refill_rate"),
		},
		ConnectionPool: ConnectionPoolConfig{
			MaxConnections:  viper.GetInt("connection_pool.max_connections"),
			StaleTimeout:    viper.GetDuration("connection_pool.stale_timeout"),
			CleanupInterval: viper.GetDuration("connection_pool.cleanup_interval"),
		},
		IPLimit: IPLimitConfig{
			MaxQPSPerIP:     viper.GetInt("ip_limit.max_qps_per_ip"),
			BurstSize:       viper.GetInt("ip_limit.burst_size"),
			CleanupInterval: viper.GetDuration("ip_limit.cleanup_interval"),
			StaleDuration:   viper.GetDuration("ip_limit.stale_duration"),
		},
		GoPool: GoPoolConfig{
			Workers:  viper.GetInt("go_pool.workers"),
			QueueCap: viper.GetInt("go_pool.queue_cap"),
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold:    viper.GetFloat64("circuit_breaker.failure_threshold"),
			MinRequests:         viper.GetInt("circuit_breaker.min_requests"),
			RecoveryTimeout:     viper.GetDuration("circuit_breaker.recovery_timeout"),
			HalfOpenMaxRequests: viper.GetInt("circuit_breaker.half_open_max_requests"),
		},
		Aggregator: AggregatorConfig{
			Enabled:        viper.GetBool("aggregator.enabled"),
			WindowSize:     viper.GetDuration("aggregator.window_size"),
			MaxWindowCount: viper.GetInt("aggregator.max_window_count"),
			Deduplication:  viper.GetBool("aggregator.deduplication"),
			FilterInvalid:  viper.GetBool("aggregator.filter_invalid"),
			MinCompression: viper.GetFloat64("aggregator.min_compression"),
		},
		Auth: AuthConfig{
			Enabled:             viper.GetBool("auth.enabled"),
			RequireMTLS:         viper.GetBool("auth.require_mtls"),
			RequireToken:        viper.GetBool("auth.require_token"),
			RequireWhitelist:    viper.GetBool("auth.require_whitelist"),
			TokenHeader:         viper.GetString("auth.token_header"),
			RejectLog:           viper.GetBool("auth.reject_log"),
			WhitelistSyncInterval: viper.GetDuration("auth.whitelist_sync_interval"),
		},
		Cluster: ClusterConfig{
			Enabled:               viper.GetBool("cluster.enabled"),
			NodeID:                viper.GetString("cluster.node_id"),
			ServiceName:           viper.GetString("cluster.service_name"),
			HeartbeatInterval:     viper.GetDuration("cluster.heartbeat_interval"),
			HealthCheckTimeout:    viper.GetDuration("cluster.health_check_timeout"),
			LoadBalanceStrategy:   viper.GetString("cluster.load_balance_strategy"),
			MaxConnectionsPerNode: viper.GetInt("cluster.max_connections_per_node"),
		},
		Log: LogConfig{
			Level:  viper.GetString("log.level"),
			Format: viper.GetString("log.format"),
		},
		// L1 修复: 可配置的定时任务间隔
		HeartbeatInterval:    viper.GetDuration("heartbeat_interval"),
		ProbeReportInterval:  viper.GetDuration("probe_report_interval"),
		SelfMonitorInterval:  viper.GetDuration("self_monitor_interval"),
		MaxBufferLimit:       viper.GetInt("max_buffer_limit"),
		ReconnectBaseDelay:   viper.GetDuration("reconnect_base_delay"),
		ReconnectMaxDelay:    viper.GetDuration("reconnect_max_delay"),
		MaxReconnectAttempts: viper.GetInt("max_reconnect_attempts"),
	}

	// edge_node_id 未配置时，使用 hostname 自动生成
	if cfg.EdgeNodeID == "" {
		hostname, _ := os.Hostname()
		if hostname != "" {
			cfg.EdgeNodeID = hostname
		} else {
			cfg.EdgeNodeID = "edge-unknown"
		}
	}

	return cfg, nil
}

// Summary 返回配置摘要字符串，用于启动日志
// 注意：API Key 会被脱敏处理，不会明文打印
func (c *Config) Summary() string {
	// API Key 脱敏处理：只显示前4位和后4位，中间用****代替
	apiKeyMasked := utils.MaskSecret(c.APIKey)
	centerAPIKeyMasked := utils.MaskSecret(c.CenterAPIKey)

	return fmt.Sprintf(
		"EdgeNodeID=%s, ListenPort=%d, MetricsPort=%d, HealthPort=%d, CenterAddr=%s, ServiceDiscovery=%v, Platform=%s, Region=%s, BatchSize=%d, FlushInterval=%ds, APIKey=%s, CenterAPIKey=%s, TLS=%v, LogLevel=%s, LogFormat=%s",
		c.EdgeNodeID, c.GRPCListenPort, c.MetricsPort, c.HealthPort, c.CenterAddr, c.ServiceDiscovery.Enabled, c.CloudPlatform, c.Region,
		c.BatchSize, c.FlushInterval, apiKeyMasked, centerAPIKeyMasked, c.TLS.Enabled, c.Log.Level, c.Log.Format,
	)
}

// StartConfigWatch 启动配置热加载
// 当配置文件变化时，会自动重新加载配置
func StartConfigWatch(callback func(*Config), log *logger.Logger) {
	// 监听配置文件变化
	viper.WatchConfig()

	// 防抖：记录上次回调时间，避免短时间内重复触发
	var lastCallbackTime time.Time
	var debounceMu sync.Mutex

	// 配置变化时的回调
	viper.OnConfigChange(func(e fsnotify.Event) {
		log.Infof("配置文件已变化: %s", e.Name)

		// 防抖检查：500ms 内不重复触发
		debounceMu.Lock()
		now := time.Now()
		if now.Sub(lastCallbackTime) < 500*time.Millisecond {
			debounceMu.Unlock()
			log.Debugf("配置变化防抖：跳过本次回调（距上次 %.0fms）", now.Sub(lastCallbackTime).Seconds()*1000)
			return
		}
		lastCallbackTime = now
		debounceMu.Unlock()

		// 重新加载配置
		cfg, err := Load()
		if err != nil {
			log.Errorf("重新加载配置失败: %v", err)
			return
		}

		// 调用回调函数
		if callback != nil {
			callback(cfg)
		}
	})

	log.Info("配置热加载已启动")
}
