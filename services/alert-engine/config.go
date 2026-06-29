package alertengine

import (
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// LoadConfig 加载配置（支持 yaml/env/defaults）
func LoadConfig() *Config {
	viper.SetDefault("service_name", "alert-engine")
	viper.SetDefault("version", "1.0.0")
	viper.SetDefault("grpc_addr", ":9010")
	viper.SetDefault("http_addr", ":8009")

	viper.SetDefault("relational_db_type", "oceanbase")
	viper.SetDefault("relational_db_host", "mysql")
	viper.SetDefault("relational_db_port", 3306)
	viper.SetDefault("relational_db_user", "root")
	viper.SetDefault("relational_db_password", "")
	viper.SetDefault("relational_db_database", "cloudflow_alert")

	viper.SetDefault("auth_addr", "auth-service:9003")
	viper.SetDefault("data_plane_addr", "")
	viper.SetDefault("tenant_addr", "")

	viper.SetDefault("eval_interval", "15s")
	viper.SetDefault("max_rules", 10000)

	viper.SetDefault("http_read_timeout", "30s")
	viper.SetDefault("http_write_timeout", "30s")
	viper.SetDefault("http_idle_timeout", "120s")
	viper.SetDefault("graceful_shutdown_timeout", "30s")
	viper.SetDefault("grpc_shutdown_timeout", "30s")
	viper.SetDefault("db_ping_timeout", "5s")
	viper.SetDefault("ch_ping_timeout", "5s")
	viper.SetDefault("notification_timeout", "30s")
	viper.SetDefault("metrics_query_timeout", "5s")

	viper.SetDefault("tls_enabled", false)
	viper.SetDefault("tls_ca_file", "")
	viper.SetDefault("tls_cert_file", "")
	viper.SetDefault("tls_key_file", "")
	viper.SetDefault("tls_client_auth", false)
	viper.SetDefault("tls_insecure_skip", false)

	viper.SetDefault("clickhouse_host", "clickhouse")
	viper.SetDefault("clickhouse_port", 9000)
	viper.SetDefault("clickhouse_user", "default")
	viper.SetDefault("clickhouse_password", "")
	viper.SetDefault("clickhouse_database", "cloudflow")

	viper.SetDefault("mock_metrics_enabled", false)

	// 尝试加载配置文件
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/cloudflow/")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("[CONFIG] Warning: config file not found: %v", err)
	} else {
		log.Printf("[CONFIG] Loaded config file: %s", viper.ConfigFileUsed())
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("CLOUDFLOW")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(err)
	}

	// Duration 字段显式回退
	if cfg.EvalInterval == 0 {
		cfg.EvalInterval = 15 * time.Second
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
	if cfg.DBPingTimeout == 0 {
		cfg.DBPingTimeout = 5 * time.Second
	}
	if cfg.CHPingTimeout == 0 {
		cfg.CHPingTimeout = 5 * time.Second
	}
	if cfg.NotificationTimeout == 0 {
		cfg.NotificationTimeout = 30 * time.Second
	}
	if cfg.MetricsQueryTimeout == 0 {
		cfg.MetricsQueryTimeout = 5 * time.Second
	}

	return &cfg
}
