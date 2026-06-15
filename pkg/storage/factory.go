package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
)

var (
	driverRegistry = make(map[DatabaseType]Driver)
	registryMu     sync.RWMutex
)

// RegisterDriver 注册数据库驱动
func RegisterDriver(dbType DatabaseType, driver Driver) {
	registryMu.Lock()
	defer registryMu.Unlock()
	driverRegistry[dbType] = driver
}

// GetDriver 获取数据库驱动
func GetDriver(dbType DatabaseType) (Driver, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	driver, ok := driverRegistry[dbType]
	return driver, ok
}

// GetSupportedDrivers 获取所有支持的驱动
func GetSupportedDrivers() []DatabaseType {
	registryMu.RLock()
	defer registryMu.RUnlock()
	types := make([]DatabaseType, 0, len(driverRegistry))
	for t := range driverRegistry {
		types = append(types, t)
	}
	return types
}

// OpenRelational 打开关系型数据库
func OpenRelational(cfg *Config) (RelationalStorage, error) {
	driver, ok := GetDriver(cfg.Type)
	if !ok {
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
	return driver.OpenRelational(cfg)
}

// OpenTimeSeries 打开时序数据库
func OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	driver, ok := GetDriver(cfg.Type)
	if !ok {
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
	return driver.OpenTimeSeries(cfg)
}

// OpenKV 打开KV数据库
func OpenKV(cfg *Config) (KVStorage, error) {
	driver, ok := GetDriver(cfg.Type)
	if !ok {
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
	return driver.OpenKV(cfg)
}

// GetDialect 获取SQL方言
func GetDialect(dbType DatabaseType) (Dialect, error) {
	driver, ok := GetDriver(dbType)
	if !ok {
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
	return driver.GetDialect(), nil
}

// ==================== 双写兼容层 ====================

// DualWriteRelationalStorage 双写关系型存储
type DualWriteRelationalStorage struct {
	primary   RelationalStorage
	secondary RelationalStorage
	mode      DualWriteMode
}

// NewDualWriteRelationalStorage 创建双写存储
func NewDualWriteRelationalStorage(primary, secondary RelationalStorage, mode DualWriteMode) *DualWriteRelationalStorage {
	return &DualWriteRelationalStorage{
		primary:   primary,
		secondary: secondary,
		mode:      mode,
	}
}

func (d *DualWriteRelationalStorage) Type() DatabaseType {
	return d.primary.Type()
}

func (d *DualWriteRelationalStorage) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	// 先写主库
	res, err := d.primary.Exec(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	// 根据模式决定是否写从库
	if d.mode >= ModeAsyncWrite && d.secondary != nil {
		if d.mode == ModeSyncWrite {
			// 同步双写
			_, secErr := d.secondary.Exec(ctx, sql, args...)
			if secErr != nil {
				log.Printf("[ERROR] dual write sync failed: secondary database write error: %v, sql: %.100s", secErr, sql)
			}
		} else {
			// 异步双写
			go func(sql string, args ...interface{}) {
				_, secErr := d.secondary.Exec(context.Background(), sql, args...)
				if secErr != nil {
					log.Printf("[ERROR] dual write async failed: secondary database write error: %v, sql: %.100s", secErr, sql)
				}
			}(sql, args...)
		}
	}

	return res, nil
}

func (d *DualWriteRelationalStorage) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	// 根据模式决定读哪个库
	if d.mode >= ModeReadSplit && d.secondary != nil {
		// 简单策略：随机读，实际可以更复杂
		return d.secondary.Query(ctx, sql, args...)
	}
	return d.primary.Query(ctx, sql, args...)
}

func (d *DualWriteRelationalStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	if d.mode >= ModeReadSplit && d.secondary != nil {
		return d.secondary.QueryRow(ctx, sql, args...)
	}
	return d.primary.QueryRow(ctx, sql, args...)
}

func (d *DualWriteRelationalStorage) BeginTx(ctx context.Context) (Tx, error) {
	// 事务仅在主库执行，双写模式下不支持分布式事务
	return d.primary.BeginTx(ctx)
}

func (d *DualWriteRelationalStorage) Ping(ctx context.Context) error {
	if err := d.primary.Ping(ctx); err != nil {
		return err
	}
	if d.secondary != nil {
		_ = d.secondary.Ping(ctx)
	}
	return nil
}

func (d *DualWriteRelationalStorage) PingContext(ctx context.Context) error {
	if err := d.primary.PingContext(ctx); err != nil {
		return err
	}
	if d.secondary != nil {
		_ = d.secondary.PingContext(ctx)
	}
	return nil
}

func (d *DualWriteRelationalStorage) Close() error {
	if err := d.primary.Close(); err != nil {
		return err
	}
	if d.secondary != nil {
		_ = d.secondary.Close()
	}
	return nil
}

func (d *DualWriteRelationalStorage) RawDB() *sql.DB {
	return d.primary.RawDB()
}

// SetMode 设置双写模式
func (d *DualWriteRelationalStorage) SetMode(mode DualWriteMode) {
	d.mode = mode
}

// GetMode 获取双写模式
func (d *DualWriteRelationalStorage) GetMode() DualWriteMode {
	return d.mode
}

// GetPrimary 获取主库
func (d *DualWriteRelationalStorage) GetPrimary() RelationalStorage {
	return d.primary
}

// GetSecondary 获取从库
func (d *DualWriteRelationalStorage) GetSecondary() RelationalStorage {
	return d.secondary
}
