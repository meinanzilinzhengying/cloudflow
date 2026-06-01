// Package storage 提供存储引擎工厂
package storage

import (
	"fmt"

	"cloud-flow-center/internal/config"
	"cloud-flow-center/pkg/logger"
)

// NewStorageEngine 创建 TiDB 存储引擎
func NewStorageEngine(cfg config.StorageConfig, log *logger.Logger) (StorageEngine, error) {
	retDays := cfg.RetDays
	if retDays <= 0 {
		retDays = 7
	}

	if cfg.DSN == "" {
		return nil, fmt.Errorf("TiDB DSN 未配置，请设置 center.storage.dsn 配置项")
	}

	return NewTiDB(cfg.DSN, retDays, log)
}
