package dataplane

import (
	"strings"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/services/data-plane/sampling"
	"github.com/spf13/viper"
)

// LoadConfig 加载配置（支持 yaml/env/defaults）
func LoadConfig() *Config {
	viper.SetDefault("service_name", "data-plane")
	viper.SetDefault("version", "1.0.0")
	viper.SetDefault("grpc_addr", ":9002")
	viper.SetDefault("metrics_addr", ":9102")

	viper.SetDefault("batch_size", 10000)
	viper.SetDefault("flush_interval", "1s")
	viper.SetDefault("queue_size", 100000)
	viper.SetDefault("worker_count", 4)

	viper.SetDefault("time_series_db_host", "clickhouse")
	viper.SetDefault("time_series_db_port", 9000)
	viper.SetDefault("time_series_db_user", "default")
	viper.SetDefault("time_series_db_password", "")
	viper.SetDefault("time_series_db_database", "cloudflow")

	viper.SetDefault("victoria_metrics_addr", "http://victoriametrics:8428")
	viper.SetDefault("loki_addr", "http://loki:3100")

	viper.SetDefault("control_plane_addr", "")
	viper.SetDefault("topology_addr", "")
	viper.SetDefault("auth_addr", "auth-service:9003")

	viper.SetDefault("tls_enabled", false)
	viper.SetDefault("tls_ca_file", "")
	viper.SetDefault("tls_cert_file", "")
	viper.SetDefault("tls_key_file", "")
	viper.SetDefault("tls_client_auth", false)
	viper.SetDefault("tls_insecure_skip", false)

	viper.SetDefault("http_read_timeout", "30s")
	viper.SetDefault("http_write_timeout", "30s")
	viper.SetDefault("http_idle_timeout", "120s")
	viper.SetDefault("graceful_shutdown_timeout", "30s")
	viper.SetDefault("grpc_shutdown_timeout", "30s")
	viper.SetDefault("client_timeout", "30s")
	viper.SetDefault("client_idle_conn_timeout", "90s")
	viper.SetDefault("ch_ping_timeout", "5s")

	viper.AutomaticEnv()
	viper.SetEnvPrefix("CLOUDFLOW")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(err)
	}

	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.HTTPReadTimeout == 0 {
		cfg.HTTPReadTimeout = 30 * time.Second
	}
	if cfg.HTTPWriteTimeout == 0 {
		cfg.HTTPWriteTimeout = 30 * time.Second
	}
	if cfg.HTTPIdleTimeout == 0 {
		cfg.HTTPIdleTimeout = 120 * time.Second
	}
	if cfg.GracefulShutdownTimeout == 0 {
		cfg.GracefulShutdownTimeout = 30 * time.Second
	}
	if cfg.GRPCShutdownTimeout == 0 {
		cfg.GRPCShutdownTimeout = 30 * time.Second
	}
	if cfg.ClientTimeout == 0 {
		cfg.ClientTimeout = 30 * time.Second
	}
	if cfg.ClientIdleConnTimeout == 0 {
		cfg.ClientIdleConnTimeout = 90 * time.Second
	}
	if cfg.CHPingTimeout == 0 {
		cfg.CHPingTimeout = 5 * time.Second
	}
	if cfg.Sampling == nil {
		cfg.Sampling = sampling.NewSamplingConfig()
	}

	return &cfg
}
