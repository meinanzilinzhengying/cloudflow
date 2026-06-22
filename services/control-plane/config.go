package controlplane

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// LoadConfig 加载配置（支持 yaml/env/defaults）
func LoadConfig() *Config {
	viper.SetDefault("service_name", "control-plane")
	viper.SetDefault("version", "1.0.0")
	viper.SetDefault("grpc_addr", ":9001")
	viper.SetDefault("http_addr", ":8001")
	viper.SetDefault("etcd_endpoints", []string{"localhost:2379"})
	viper.SetDefault("etcd_prefix", "github.com/meinanzilinzhengying/cloudflow/services/")
	viper.SetDefault("data_plane_addr", "")
	viper.SetDefault("auth_addr", "auth-service:9003")
	viper.SetDefault("tenant_addr", "tenant-service:9010")

	viper.SetDefault("agent_ttl", "90s")
	viper.SetDefault("heartbeat_timeout", "60s")

	viper.SetDefault("tls_enabled", false)
	viper.SetDefault("tls_ca_file", "")
	viper.SetDefault("tls_cert_file", "")
	viper.SetDefault("tls_key_file", "")
	viper.SetDefault("tls_client_auth", false)
	viper.SetDefault("tls_insecure_skip", false)

	viper.SetDefault("http_read_timeout", "30s")
	viper.SetDefault("http_write_timeout", "30s")
	viper.SetDefault("http_idle_timeout", "120s")
	viper.SetDefault("etcd_dial_timeout", "5s")
	viper.SetDefault("grpc_dial_timeout", "5s")
	viper.SetDefault("http_shutdown_timeout", "10s")
	viper.SetDefault("grpc_shutdown_timeout", "15s")
	viper.SetDefault("etcd_restore_timeout", "10s")

	viper.AutomaticEnv()
	viper.SetEnvPrefix("CLOUDFLOW")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(err)
	}

	// Duration 字段显式转换
	if cfg.AgentTTL == 0 {
		cfg.AgentTTL = 90 * time.Second
	}
	if cfg.HeartbeatTimeout == 0 {
		cfg.HeartbeatTimeout = 60 * time.Second
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
	if cfg.EtcdDialTimeout == 0 {
		cfg.EtcdDialTimeout = 5 * time.Second
	}
	if cfg.GRPCDialTimeout == 0 {
		cfg.GRPCDialTimeout = 5 * time.Second
	}
	if cfg.HTTPShutdownTimeout == 0 {
		cfg.HTTPShutdownTimeout = 10 * time.Second
	}
	if cfg.GRPCShutdownTimeout == 0 {
		cfg.GRPCShutdownTimeout = 15 * time.Second
	}
	if cfg.EtcdRestoreTimeout == 0 {
		cfg.EtcdRestoreTimeout = 10 * time.Second
	}

	return &cfg
}
