// Package storage 数据库抽象层
// GaussDB(高斯数据库) 适配实现
// GaussDB高度兼容PostgreSQL协议
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// GaussDBDriver 高斯数据库驱动实现
type GaussDBDriver struct{}

// GaussDBStorage 高斯数据库关系型存储实现
type GaussDBStorage struct {
	db  *sql.DB
	cfg *Config
}

// GaussDBDialect 高斯数据库方言实现
type GaussDBDialect struct{}

func init() {
	RegisterDriver(DatabaseGaussDB, &GaussDBDriver{})
}

// ==================== GaussDB驱动 ====================

func (d *GaussDBDriver) Type() DatabaseType {
	return DatabaseGaussDB
}

func (d *GaussDBDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	// GaussDB连接串（兼容PostgreSQL协议）
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open gaussdb failed: %w", err)
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
		return nil, fmt.Errorf("ping gaussdb failed: %w", err)
	}

	return &GaussDBStorage{
		db:  db,
		cfg: cfg,
	}, nil
}

func (d *GaussDBDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	return nil, fmt.Errorf("gaussdb does not support time series storage")
}

func (d *GaussDBDriver) OpenKV(cfg *Config) (KVStorage, error) {
	return nil, fmt.Errorf("gaussdb does not support kv storage")
}

func (d *GaussDBDriver) GetDialect() Dialect {
	return &GaussDBDialect{}
}

// ==================== GaussDB关系型存储实现 ====================

func (s *GaussDBStorage) Type() DatabaseType {
	return DatabaseGaussDB
}

func (s *GaussDBStorage) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	// SQL语法转换
	dialect := &GaussDBDialect{}
	sql = dialect.ConvertUpdate(sql)
	sql = dialect.ConvertDelete(sql)
	sql = dialect.ConvertInsert(sql)
	sql = dialect.ConvertPlaceholder(sql, len(args))

	res, err := s.db.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlResult{res: res}, nil
}

func (s *GaussDBStorage) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	// SQL语法转换
	dialect := &GaussDBDialect{}
	sql = dialect.ConvertSelect(sql)
	sql = dialect.ConvertPlaceholder(sql, len(args))

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows: rows}, nil
}

func (s *GaussDBStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	// SQL语法转换
	dialect := &GaussDBDialect{}
	sql = dialect.ConvertSelect(sql)
	sql = dialect.ConvertPlaceholder(sql, len(args))

	row := s.db.QueryRowContext(ctx, sql, args...)
	return &sqlRow{row: row}
}

func (s *GaussDBStorage) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqlTx{tx: tx}, nil
}

func (s *GaussDBStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *GaussDBStorage) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *GaussDBStorage) Close() error {
	return s.db.Close()
}

func (s *GaussDBStorage) RawDB() *sql.DB {
	return s.db
}

// ==================== GaussDB方言实现 ====================

func (d *GaussDBDialect) ConvertCreateTable(sql string) string {
	// GaussDB高度兼容PostgreSQL，基本不需要转换
	result := sql
	// MySQL自增转换
	result = strings.ReplaceAll(result, "AUTO_INCREMENT", "SERIAL")
	result = strings.ReplaceAll(result, "INT AUTO_INCREMENT", "SERIAL")
	result = strings.ReplaceAll(result, "BIGINT AUTO_INCREMENT", "BIGSERIAL")
	return result
}

func (d *GaussDBDialect) ConvertCreateIndex(sql string) string {
	return sql
}

func (d *GaussDBDialect) ConvertSelect(sql string) string {
	return sql
}

func (d *GaussDBDialect) ConvertInsert(sql string) string {
	return sql
}

func (d *GaussDBDialect) ConvertUpdate(sql string) string {
	return sql
}

func (d *GaussDBDialect) ConvertDelete(sql string) string {
	return sql
}

func (d *GaussDBDialect) MapFunction(funcName string, args ...string) string {
	funcName = strings.ToLower(funcName)
	switch funcName {
	case "ifnull":
		if len(args) >= 2 {
			return fmt.Sprintf("COALESCE(%s, %s)", args[0], args[1])
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))
	}
	return funcName + "()"
}

func (d *GaussDBDialect) ApplyPagination(sql string, offset, limit int) string {
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, limit, offset)
}

func (d *GaussDBDialect) ConvertPlaceholder(sql string, argCount int) string {
	// MySQL ? 占位符转换为 PostgreSQL $1, $2, ...
	result := sql
	for i := 1; i <= argCount; i++ {
		result = strings.Replace(result, "?", fmt.Sprintf("$%d", i), 1)
	}
	return result
}
