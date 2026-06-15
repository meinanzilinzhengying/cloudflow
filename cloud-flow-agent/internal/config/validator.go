// Package config 配置管理
//
// 配置版本校验与热更新
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ============================================================================
// SHA256 配置校验工具
// ============================================================================

// CalculateChecksum 计算配置的SHA256校验和
func CalculateChecksum(configYaml []byte) string {
	hash := sha256.Sum256(configYaml)
	return hex.EncodeToString(hash[:])
}

// ValidateChecksum 验证配置校验和
func ValidateChecksum(configYaml []byte, expectedChecksum string) error {
	actual := CalculateChecksum(configYaml)
	if actual != expectedChecksum {
		return fmt.Errorf("config checksum mismatch: expected %s, actual %s",
			expectedChecksum, actual)
	}
	return nil
}

// ============================================================================
// 配置版本管理
// ============================================================================

// VersionedConfig 带版本和校验和的配置
type VersionedConfig struct {
	Version     int64  `json:"version"`
	ConfigYaml  []byte `json:"config_yaml"`
	Checksum    string `json:"checksum"`
	AppliedAt   int64  `json:"applied_at"`
}

// Validate 验证配置完整性
func (vc *VersionedConfig) Validate() error {
	// 验证校验和
	if err := ValidateChecksum(vc.ConfigYaml, vc.Checksum); err != nil {
		return err
	}

	// 验证版本号
	if vc.Version <= 0 {
		return fmt.Errorf("invalid config version: %d", vc.Version)
	}

	return nil
}

// ============================================================================
// 配置变更追踪
// ============================================================================

// ConfigHistory 配置历史记录
type ConfigHistory struct {
	history []VersionedConfig
	maxSize int
}

// NewConfigHistory 创建配置历史记录
func NewConfigHistory(maxSize int) *ConfigHistory {
	return &ConfigHistory{
		history: make([]VersionedConfig, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add 添加配置到历史记录
func (ch *ConfigHistory) Add(cfg VersionedConfig) {
	// 移除最旧的记录如果超过大小限制
	if len(ch.history) >= ch.maxSize {
		ch.history = ch.history[1:]
	}
	ch.history = append(ch.history, cfg)
}

// GetLatest 获取最新配置
func (ch *ConfigHistory) GetLatest() *VersionedConfig {
	if len(ch.history) == 0 {
		return nil
	}
	return &ch.history[len(ch.history)-1]
}

// Rollback 回滚到上一个版本
func (ch *ConfigHistory) Rollback() (*VersionedConfig, error) {
	if len(ch.history) < 2 {
		return nil, fmt.Errorf("no previous config to rollback")
	}

	// 移除当前版本
	ch.history = ch.history[:len(ch.history)-1]

	return ch.GetLatest(), nil
}

// GetHistory 获取配置历史
func (ch *ConfigHistory) GetHistory() []VersionedConfig {
	result := make([]VersionedConfig, len(ch.history))
	copy(result, ch.history)
	return result
}

// ============================================================================
// 配置变更检测
// ============================================================================

// ConfigChangeType 配置变更类型
type ConfigChangeType string

const (
	ConfigChangeNone     ConfigChangeType = "none"
	ConfigChangeFull     ConfigChangeType = "full"
	ConfigChangePartial  ConfigChangeType = "partial"
	ConfigChangeInvalid  ConfigChangeType = "invalid"
)

// DetectConfigChange 检测配置变更类型
func DetectConfigChange(oldCfg, newCfg *CollectionConfig) ConfigChangeType {
	if oldCfg == nil {
		return ConfigChangeFull
	}

	// 简单比较：序列化后比较
	oldYaml := oldCfg.ToYAML()
	newYaml := newCfg.ToYAML()

	if string(oldYaml) == string(newYaml) {
		return ConfigChangeNone
	}

	return ConfigChangeFull
}
