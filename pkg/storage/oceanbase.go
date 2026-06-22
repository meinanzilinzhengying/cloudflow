// Package storage 数据库抽象层
// OceanBase 适配实现
// OceanBase高度兼容MySQL协议
package storage

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"strings"
	"time"
)

// OceanBaseDriver OceanBase驱动实现
type OceanBaseDriver struct{}

// OceanBaseStorage OceanBase关系型存储实现
type OceanBaseStorage struct {
	db  *sql.DB
	cfg *Config
}

// OceanBaseDialect OceanBase方言实现
type OceanBaseDialect struct{}

func init() {
	RegisterDriver(DatabaseOceanBase, &OceanBaseDriver{})
	RegisterDriver(DatabaseMySQL, &OceanBaseDriver{})
}

// ==================== OceanBase驱动 ====================

func (d *OceanBaseDriver) Type() DatabaseType {
	return DatabaseOceanBase
}

func (d *OceanBaseDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	// OceanBase连接串（兼容MySQL协议）
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open oceanbase failed: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 50
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 10
	}
	maxLifetime := cfg.MaxLifetime
	if maxLifetime <= 0 {
		maxLifetime = 3600
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Duration(maxLifetime) * time.Second)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping oceanbase failed: %w", err)
	}

	return &OceanBaseStorage{
		db:  db,
		cfg: cfg,
	}, nil
}

func (d *OceanBaseDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	return nil, fmt.Errorf("oceanbase does not support time series storage")
}

func (d *OceanBaseDriver) OpenKV(cfg *Config) (KVStorage, error) {
	return nil, fmt.Errorf("oceanbase does not support kv storage")
}

func (d *OceanBaseDriver) GetDialect() Dialect {
	return &OceanBaseDialect{}
}

// ==================== OceanBase关系型存储实现 ====================

func (s *OceanBaseStorage) Type() DatabaseType {
	return DatabaseOceanBase
}

func (s *OceanBaseStorage) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	res, err := s.db.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlResult{res: res}, nil
}

func (s *OceanBaseStorage) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows: rows}, nil
}

func (s *OceanBaseStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	row := s.db.QueryRowContext(ctx, sql, args...)
	return &sqlRow{row: row}
}

func (s *OceanBaseStorage) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqlTx{tx: tx}, nil
}

func (s *OceanBaseStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *OceanBaseStorage) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *OceanBaseStorage) Close() error {
	return s.db.Close()
}

func (s *OceanBaseStorage) RawDB() *sql.DB {
	return s.db
}

// ==================== OceanBase方言实现 ====================

func (d *OceanBaseDialect) ConvertCreateTable(sql string) string {
	// OceanBase高度兼容MySQL，基本不需要转换
	return sql
}

func (d *OceanBaseDialect) ConvertCreateIndex(sql string) string {
	return sql
}

func (d *OceanBaseDialect) ConvertSelect(sql string) string {
	return sql
}

func (d *OceanBaseDialect) ConvertInsert(sql string) string {
	return sql
}

func (d *OceanBaseDialect) ConvertUpdate(sql string) string {
	return sql
}

func (d *OceanBaseDialect) ConvertDelete(sql string) string {
	return sql
}

func (d *OceanBaseDialect) MapFunction(funcName string, args ...string) string {
	if len(args) > 0 {
		return fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))
	}
	return funcName + "()"
}

func (d *OceanBaseDialect) ApplyPagination(sql string, offset, limit int) string {
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, limit, offset)
}

func (d *OceanBaseDialect) ConvertPlaceholder(sql string, argCount int) string {
	// OceanBase使用MySQL风格?占位符
	return sql
}
