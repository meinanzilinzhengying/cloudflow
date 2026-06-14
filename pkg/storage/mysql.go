package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLDriver MySQL驱动实现
type MySQLDriver struct{}

// MySQLStorage MySQL关系型存储实现
type MySQLStorage struct {
	db   *sql.DB
	cfg  *Config
}

// MySQLDialect MySQL方言实现
type MySQLDialect struct{}

func init() {
	RegisterDriver(DatabaseMySQL, &MySQLDriver{})
}

// ==================== MySQL驱动 ====================

func (d *MySQLDriver) Type() DatabaseType {
	return DatabaseMySQL
}

func (d *MySQLDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql failed: %w", err)
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
		return nil, fmt.Errorf("ping mysql failed: %w", err)
	}

	return &MySQLStorage{
		db:  db,
		cfg: cfg,
	}, nil
}

func (d *MySQLDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	return nil, fmt.Errorf("mysql does not support time series storage")
}

func (d *MySQLDriver) OpenKV(cfg *Config) (KVStorage, error) {
	return nil, fmt.Errorf("mysql does not support kv storage")
}

func (d *MySQLDriver) GetDialect() Dialect {
	return &MySQLDialect{}
}

// ==================== MySQL存储实现 ====================

func (s *MySQLStorage) Type() DatabaseType {
	return DatabaseMySQL
}

func (s *MySQLStorage) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	res, err := s.db.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &mysqlResult{res}, nil
}

func (s *MySQLStorage) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &mysqlRows{rows}, nil
}

func (s *MySQLStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	row := s.db.QueryRowContext(ctx, sql, args...)
	return &mysqlRow{row}
}

func (s *MySQLStorage) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &mysqlTx{tx}, nil
}

func (s *MySQLStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *MySQLStorage) Close() error {
	return s.db.Close()
}

func (s *MySQLStorage) RawDB() *sql.DB {
	return s.db
}

// ==================== MySQL方言实现 ====================

func (d *MySQLDialect) ConvertCreateTable(sql string) string {
	// MySQL语法原生支持
	return sql
}

func (d *MySQLDialect) ConvertCreateIndex(sql string) string {
	return sql
}

func (d *MySQLDialect) ConvertSelect(sql string) string {
	return sql
}

func (d *MySQLDialect) ConvertInsert(sql string) string {
	return sql
}

func (d *MySQLDialect) ConvertUpdate(sql string) string {
	return sql
}

func (d *MySQLDialect) ConvertDelete(sql string) string {
	return sql
}

func (d *MySQLDialect) MapFunction(funcName string, args ...string) string {
	funcName = strings.ToLower(funcName)
	switch funcName {
	case "ifnull":
		if len(args) >= 2 {
			return fmt.Sprintf("IFNULL(%s, %s)", args[0], args[1])
		}
	case "now":
		return "NOW()"
	case "unix_timestamp":
		return "UNIX_TIMESTAMP()"
	case "from_unixtime":
		if len(args) >= 1 {
			return fmt.Sprintf("FROM_UNIXTIME(%s)", args[0])
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))
	}
	return funcName + "()"
}

func (d *MySQLDialect) ApplyPagination(sql string, offset, limit int) string {
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, limit, offset)
}

func (d *MySQLDialect) ConvertPlaceholder(sql string, argCount int) string {
	// MySQL使用?作为占位符，无需转换
	return sql
}

// ==================== 包装类 ====================

type mysqlResult struct {
	sql.Result
}

type mysqlRows struct {
	*sql.Rows
}

type mysqlRow struct {
	*sql.Row
}

type mysqlTx struct {
	*sql.Tx
}

func (tx *mysqlTx) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	res, err := tx.Tx.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &mysqlResult{res}, nil
}

func (tx *mysqlTx) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	rows, err := tx.Tx.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &mysqlRows{rows}, nil
}

func (tx *mysqlTx) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	row := tx.Tx.QueryRowContext(ctx, sql, args...)
	return &mysqlRow{row}
}
