package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type Config struct {
	AI struct {
		Port     int    `mapstructure:"port"`
		DataDir  string `mapstructure:"data_dir"`
		Log      LogConfig `mapstructure:"log"`
		JWT      JWTConfig `mapstructure:"jwt"`
		LLM      LLMConfig `mapstructure:"llm"`
		Analysis AnalysisConfig `mapstructure:"analysis"`
	} `mapstructure:"ai"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"`
	LogDir     string `mapstructure:"log_dir"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

type JWTConfig struct {
	SecretKey      string `mapstructure:"secret_key"`
	TokenDuration  int    `mapstructure:"token_duration"`
}

type LLMConfig struct {
	Provider     string        `mapstructure:"provider"`
	Models       []LLMModel    `mapstructure:"models"`
	DefaultModel string        `mapstructure:"default_model"`
}

type LLMModel struct {
	Name          string  `mapstructure:"name"`
	Provider      string  `mapstructure:"provider"`
	APIURL        string  `mapstructure:"api_url"`
	APIKey        string  `mapstructure:"api_key"`
	MaxTokens     int     `mapstructure:"max_tokens"`
	Temperature   float64 `mapstructure:"temperature"`
}

type AnalysisConfig struct {
	CacheTTL            int `mapstructure:"cache_ttl"`
	MaxAnalysisHistory  int `mapstructure:"max_analysis_history"`
}

type ConfigManager struct {
	mu       sync.RWMutex
	config   *Config
	viper    *viper.Viper
	callbacks []func(old, new *Config)
	stopCh   chan struct{}
}

type Option func(*ConfigManager)

func NewConfigManager(opts ...Option) *ConfigManager {
	cm := &ConfigManager{
		viper:  viper.New(),
		config: &Config{},
		stopCh: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(cm)
	}
	return cm
}

func WithConfigPath(path string) Option {
	return func(cm *ConfigManager) {
		if path != "" {
			cm.viper.SetConfigFile(path)
		} else {
			cm.viper.SetConfigName("config")
			cm.viper.SetConfigType("yaml")
			cm.viper.AddConfigPath(".")
			cm.viper.AddConfigPath("./configs")
			cm.viper.AddConfigPath("/etc/cloud-flow")
		}
	}
}

func expandEnvVars() {
}

func (cm *ConfigManager) Load() (*Config, error) {
	cm.viper.AutomaticEnv()
	cm.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	cm.viper.SetEnvPrefix("CLOUDFLOW")

	if err := cm.viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置失败: %v", err)
	}

	// 读取默认配置
	setDefaults(cm.viper)

	if err := cm.viper.Unmarshal(&cm.config); err != nil {
		return nil, fmt.Errorf("解析配置失败: %v", err)
	}

	// 替换环境变量
	cm.replaceEnvVars(cm.config)

	return cm.config, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("ai.port", 8082)
	v.SetDefault("ai.data_dir", "./data")
	v.SetDefault("ai.log.level", "info")
	v.SetDefault("ai.log.format", "json")
	v.SetDefault("ai.log.output", "stdout")
	v.SetDefault("ai.llm.default_model", "gpt-4o")
	v.SetDefault("ai.analysis.cache_ttl", 3600)
	v.SetDefault("ai.analysis.max_analysis_history", 100)
}

func (cm *ConfigManager) replaceEnvVars(cfg *Config) {
	for i := range cfg.AI.LLM.Models {
		cfg.AI.LLM.Models[i].APIKey = os.ExpandEnv(cfg.AI.LLM.Models[i].APIKey)
	}
	cfg.AI.JWT.SecretKey = os.ExpandEnv(cfg.AI.JWT.SecretKey)
}

func (cm *ConfigManager) LoadAndWatch() error {
	if _, err := cm.Load(); err != nil {
		return err
	}

	cm.viper.WatchConfig()
	cm.viper.OnConfigChange(func(e fsnotify.Event) {
		cm.mu.Lock()
		old := cm.config
		newCfg, _ := cm.Load()
		cm.mu.Unlock()

		for _, cb := range cm.callbacks {
			cb(old, newCfg)
		}
	})

	return nil
}

func (cm *ConfigManager) RegisterCallback(cb func(old, new *Config)) {
	cm.mu.Lock()
	cm.callbacks = append(cm.callbacks, cb)
	cm.mu.Unlock()
}

func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	cfg := *cm.config
	cm.mu.RUnlock()
	return &cfg
}

func (cm *ConfigManager) Stop() {
	close(cm.stopCh)
}

func (cfg *Config) Summary() string {
	return fmt.Sprintf("port=%d, log.level=%s", cfg.AI.Port, cfg.AI.Log.Level)
}
