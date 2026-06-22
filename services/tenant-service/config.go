package tenantservice

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// LoadConfig 加载配置（支持 yaml/env/defaults）
func LoadConfig() *Config {
	viper.SetDefault("service_name", "tenant-service")
	viper.SetDefault("version", "1.0.0")
	viper.SetDefault("grpc_addr", ":9010")
	viper.SetDefault("http_addr", ":8010")
	viper.SetDefault("auth_addr", "auth-service:9006")

	viper.SetDefault("db_type", "mysql")
	viper.SetDefault("db_host", "127.0.0.1")
	viper.SetDefault("db_port", 3306)
	viper.SetDefault("db_user", "")
	viper.SetDefault("db_password", "")
	viper.SetDefault("db_database", "cloudflow_tenant")
	viper.SetDefault("db_max_open_conns", 50)
	viper.SetDefault("db_max_idle_conns", 10)

	viper.SetDefault("db_enable_dual_write", false)
	viper.SetDefault("db_dual_write_mode", 0)

	viper.SetDefault("default_retention_days", 30)
	viper.SetDefault("default_max_agents", 100)
	viper.SetDefault("default_max_flows_per_day", 10000000)
	viper.SetDefault("default_max_storage_gb", 100)
	viper.SetDefault("default_max_alert_rules", 100)

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
	viper.SetDefault("db_ping_timeout", "5s")

	viper.AutomaticEnv()
	viper.SetEnvPrefix("CLOUDFLOW")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(err)
	}

	// Duration 字段需要显式转换（viper.Unmarshal 对 time.Duration 的字符串支持不稳定）
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
	if cfg.DBPingTimeout == 0 {
		cfg.DBPingTimeout = 5 * time.Second
	}

	return &cfg
}
