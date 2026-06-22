package topologyengine

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// LoadConfig 加载配置（支持 yaml/env/defaults）
func LoadConfig() *Config {
	viper.SetDefault("service_name", "topology-engine")
	viper.SetDefault("version", "1.0.0")
	viper.SetDefault("grpc_addr", ":9004")
	viper.SetDefault("http_addr", ":8008")

	viper.SetDefault("clickhouse_addr", "clickhouse:8123")
	viper.SetDefault("clickhouse_db", "cloudflow")
	viper.SetDefault("clickhouse_user", "default")
	viper.SetDefault("clickhouse_pass", "")

	viper.SetDefault("compute_interval", "30s")
	viper.SetDefault("max_nodes", 50000)
	viper.SetDefault("max_edges", 1000000)

	viper.SetDefault("cache_max_entries", 1000)
	viper.SetDefault("cache_max_memory_mb", 512)
	viper.SetDefault("cache_ttl", "5m")

	viper.SetDefault("heatmap_interval", 60)

	viper.SetDefault("update_interval", "5s")
	viper.SetDefault("stale_threshold", "60s")

	viper.SetDefault("tls_enabled", false)
	viper.SetDefault("tls_ca_file", "")
	viper.SetDefault("tls_cert_file", "")
	viper.SetDefault("tls_key_file", "")
	viper.SetDefault("tls_client_auth", false)
	viper.SetDefault("tls_insecure_skip", false)

	viper.AutomaticEnv()
	viper.SetEnvPrefix("CLOUDFLOW")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(err)
	}

	// Duration 字段显式转换
	if cfg.ComputeInterval == 0 {
		cfg.ComputeInterval = 30 * time.Second
	}
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 5 * time.Second
	}
	if cfg.StaleThreshold == 0 {
		cfg.StaleThreshold = 60 * time.Second
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}

	return &cfg
}
