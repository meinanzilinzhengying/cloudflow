package queryservice

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// LoadConfig 加载配置（支持 yaml/env/defaults）
func LoadConfig() *Config {
	viper.SetDefault("service_name", "query-service")
	viper.SetDefault("version", "1.0.0")
	viper.SetDefault("grpc_addr", ":9007")
	viper.SetDefault("http_addr", ":8007")

	viper.SetDefault("data_plane_addr", "")
	viper.SetDefault("topology_addr", "")
	viper.SetDefault("alert_addr", "")

	viper.SetDefault("time_series_db_type", "clickhouse")
	viper.SetDefault("time_series_db_host", "clickhouse")
	viper.SetDefault("time_series_db_port", 9000)
	viper.SetDefault("time_series_db_user", "default")
	viper.SetDefault("time_series_db_password", "")
	viper.SetDefault("time_series_db_database", "cloudflow")

	viper.SetDefault("victoria_metrics_addr", "http://victoriametrics:8428")
	viper.SetDefault("loki_addr", "http://loki:3100")

	viper.SetDefault("query_timeout", "30s")
	viper.SetDefault("max_concurrent_queries", 1000)

	viper.SetDefault("auth_addr", "")

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

	if cfg.QueryTimeout == 0 {
		cfg.QueryTimeout = 30 * time.Second
	}

	return &cfg
}
