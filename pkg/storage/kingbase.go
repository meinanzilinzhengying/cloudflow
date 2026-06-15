package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// KingBaseDriver 人大金仓KingBaseES V8驱动实现
type KingBaseDriver struct{}

// KingBaseStorage 人大金仓关系型存储实现
type KingBaseStorage struct {
	db   *sql.DB
	cfg  *Config
}

// KingBaseDialect 人大金仓方言实现
type KingBaseDialect struct{}

func init() {
	RegisterDriver(DatabaseKingBase, &KingBaseDriver{})
}

// ==================== 人大金仓驱动 ====================

func (d *KingBaseDriver) Type() DatabaseType {
	return DatabaseKingBase
}

func (d *KingBaseDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	// 人大金仓连接串格式（兼容PostgreSQL协议）
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database)

	db, err := sql.Open("kingbase", dsn)
	if err != nil {
		return nil, fmt.Errorf("open kingbase failed: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 100
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 20
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
		return nil, fmt.Errorf("ping kingbase failed: %w", err)
	}

	return &KingBaseStorage{
		db:  db,
		cfg: cfg,
	}, nil
}

func (d *KingBaseDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	return nil, fmt.Errorf("kingbase time series storage not implemented yet")
}

func (d *KingBaseDriver) OpenKV(cfg *Config) (KVStorage, error) {
	return nil, fmt.Errorf("kingbase kv storage not implemented yet")
}

func (d *KingBaseDriver) GetDialect() Dialect {
	return &KingBaseDialect{}
}

// ==================== 人大金仓存储实现 ====================

func (s *KingBaseStorage) Type() DatabaseType {
	return DatabaseKingBase
}

func (s *KingBaseStorage) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	// KingBase高度兼容MySQL，大部分SQL无需转换
	res, err := s.db.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &kingbaseResult{res}, nil
}

func (s *KingBaseStorage) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &kingbaseRows{rows}, nil
}

func (s *KingBaseStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	row := s.db.QueryRowContext(ctx, sql, args...)
	return &kingbaseRow{row}
}

func (s *KingBaseStorage) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &kingbaseTx{tx}, nil
}

func (s *KingBaseStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *KingBaseStorage) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *KingBaseStorage) Close() error {
	return s.db.Close()
}

func (s *KingBaseStorage) RawDB() *sql.DB {
	return s.db
}

// ==================== 人大金仓方言实现 ====================

func (d *KingBaseDialect) ConvertCreateTable(sql string) string {
	// KingBase高度兼容MySQL语法
	result := sql
	
	// 少量转换：IFNULL -> NVL（可选）
	result = strings.ReplaceAll(result, "IFNULL(", "NVL(")
	
	// KingBase支持 ON UPDATE CURRENT_TIMESTAMP
	// KingBase支持 AUTO_INCREMENT
	
	return result
}

func (d *KingBaseDialect) ConvertCreateIndex(sql string) string {
	return sql
}

func (d *KingBaseDialect) ConvertSelect(sql string) string {
	return sql
}

func (d *KingBaseDialect) ConvertInsert(sql string) string {
	return sql
}

func (d *KingBaseDialect) ConvertUpdate(sql string) string {
	return sql
}

func (d *KingBaseDialect) ConvertDelete(sql string) string {
	return sql
}

func (d *KingBaseDialect) MapFunction(funcName string, args ...string) string {
	funcName = strings.ToLower(funcName)
	switch funcName {
	case "ifnull":
		if len(args) >= 2 {
			return fmt.Sprintf("NVL(%s, %s)", args[0], args[1])
		}
	case "now":
		return "NOW()"
	}
	if len(args) > 0 {
		return fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))
	}
	return funcName + "()"
}

func (d *KingBaseDialect) ApplyPagination(sql string, offset, limit int) string {
	// KingBase支持 LIMIT OFFSET
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, limit, offset)
}

func (d *KingBaseDialect) ConvertPlaceholder(sql string, argCount int) string {
	// KingBase兼容MySQL的?占位符
	return sql
}

// ==================== 包装类 ====================

type kingbaseResult struct {
	sql.Result
}

type kingbaseRows struct {
	*sql.Rows
}

type kingbaseRow struct {
	*sql.Row
}

type kingbaseTx struct {
	*sql.Tx
}

func (tx *kingbaseTx) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	res, err := tx.Tx.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &kingbaseResult{res}, nil
}

func (tx *kingbaseTx) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	rows, err := tx.Tx.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &kingbaseRows{rows}, nil
}

func (tx *kingbaseTx) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	row := tx.Tx.QueryRowContext(ctx, sql, args...)
	return &kingbaseRow{row}
}
