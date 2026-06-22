package authservice

import (
	"strings"

	"github.com/spf13/viper"
)

// LoadConfig 加载配置（支持 yaml/env/defaults）
func LoadConfig() *Config {
	viper.SetDefault("service_name", "auth-service")
	viper.SetDefault("version", "1.0.0")
	viper.SetDefault("grpc_addr", ":9006")
	viper.SetDefault("http_addr", ":8006")

	viper.SetDefault("jwt_secret", "")
	viper.SetDefault("jwt_issuer", "cloudflow")
	viper.SetDefault("jwt_expire_sec", 86400)
	viper.SetDefault("jwt_refresh_sec", 604800)

	viper.SetDefault("oidc_issuer", "")
	viper.SetDefault("oidc_client_id", "")
	viper.SetDefault("oidc_client_secret", "")
	viper.SetDefault("oidc_redirect_url", "")
	viper.SetDefault("oidc_scopes", "")

	viper.SetDefault("super_admin_role", "super_admin")

	viper.SetDefault("relational_db_type", "oceanbase")
	viper.SetDefault("relational_db_host", "mysql")
	viper.SetDefault("relational_db_port", 3306)
	viper.SetDefault("relational_db_user", "root")
	viper.SetDefault("relational_db_password", "")
	viper.SetDefault("relational_db_database", "cloudflow_auth")

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

	// 填充零值字段
	if cfg.JWTExpireSec == 0 {
		cfg.JWTExpireSec = 86400
	}
	if cfg.JWTRefreshSec == 0 {
		cfg.JWTRefreshSec = 604800
	}
	if cfg.RelationalDBPort == 0 {
		cfg.RelationalDBPort = 3306
	}

	return &cfg
}
