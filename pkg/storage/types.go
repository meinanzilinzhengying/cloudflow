// Package storage 数据库抽象层
// 提供统一的数据库访问接口，支持多种国产数据库适配
package storage

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// DatabaseType 数据库类型
type DatabaseType string

const (
	DatabaseDameng    DatabaseType = "dameng"
	DatabaseDamengTS  DatabaseType = "dameng_ts"
	DatabaseKingBase  DatabaseType = "kingbase"
	DatabaseGaussDB   DatabaseType = "gaussdb"
	DatabaseGaussRedis DatabaseType = "gauss_redis"
	DatabaseOceanBase  DatabaseType = "oceanbase"
	DatabaseClickHouse DatabaseType = "clickhouse"
	DatabaseMySQL     DatabaseType = "mysql"

	// 别名，兼容旧代码
	DBDameng     = DatabaseDameng
	DBDamengTS   = DatabaseDamengTS
	DBKingBase   = DatabaseKingBase
	DBGaussDB    = DatabaseGaussDB
	DBGaussRedis = DatabaseGaussRedis
	DBOceanBase  = DatabaseOceanBase
	DBClickHouse = DatabaseClickHouse
	DBMySQL      = DatabaseMySQL
)

// DualWriteMode 双写模式
type DualWriteMode int

const (
	ModeOldOnly     DualWriteMode = iota // 仅写旧库
	ModeAsyncWrite                       // 异步双写
	ModeSyncWrite                        // 同步双写
	ModeReadSplit                        // 读流量切分
	ModeNewOnly                          // 仅写新库
)

// Config 数据库配置
type Config struct {
	Type         DatabaseType `yaml:"type" json:"type"`
	Host         string       `yaml:"host" json:"host"`
	Port         int          `yaml:"port" json:"port"`
	User         string       `yaml:"user" json:"user"`
	Password     string       `yaml:"password" json:"password"`
	Database     string       `yaml:"database" json:"database"`
	MaxOpenConns int          `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns int          `yaml:"max_idle_conns" json:"max_idle_conns"`
	MaxLifetime  int          `yaml:"max_lifetime" json:"max_lifetime"`

	// 双写配置
	EnableDualWrite bool         `yaml:"enable_dual_write" json:"enable_dual_write"`
	DualWriteMode   DualWriteMode `yaml:"dual_write_mode" json:"dual_write_mode"`
	SecondaryType   DatabaseType  `yaml:"secondary_type" json:"secondary_type"`
	SecondaryHost   string        `yaml:"secondary_host" json:"secondary_host"`
	SecondaryPort   int           `yaml:"secondary_port" json:"secondary_port"`
}

// Result SQL执行结果
type Result interface {
	LastInsertId() (int64, error)
	RowsAffected() (int64, error)
}

// Rows 查询结果集
type Rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close() error
	Err() error
	Columns() ([]string, error)
}

// Row 单行查询结果
type Row interface {
	Scan(dest ...interface{}) error
}

// Tx 事务接口
type Tx interface {
	Commit() error
	Rollback() error
	Exec(ctx context.Context, sql string, args ...interface{}) (Result, error)
	Query(ctx context.Context, sql string, args ...interface{}) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) Row
}

// RelationalStorage 关系型存储接口
type RelationalStorage interface {
	// 基础操作
	Exec(ctx context.Context, sql string, args ...interface{}) (Result, error)
	Query(ctx context.Context, sql string, args ...interface{}) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) Row
	// 事务
	BeginTx(ctx context.Context) (Tx, error)
	// 连接管理
	Ping(ctx context.Context) error
	PingContext(ctx context.Context) error
	Close() error
	// 获取原生DB（用于特殊场景）
	RawDB() *sql.DB
	// 数据库类型
	Type() DatabaseType
}

// IsNotFound 判断是否为记录未找到错误
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "no rows in result set") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "record not found")
}

// Flow 流量数据
type Flow struct {
	Timestamp    int64
	FlowID       uint32
	SrcIP        string
	DstIP        string
	SrcPort      uint16
	DstPort      uint16
	Protocol     uint8
	L7Protocol   uint8
	Bytes        uint64
	Packets      uint64
	LatencyNs    uint64
	TenantID     string
	Namespace    string
	Service      string
	Pod          string
	Node         string
	Tags         map[string]string
}

// FlowQuery 流量查询条件
type FlowQuery struct {
	TenantID   string
	StartTime  int64
	EndTime    int64
	SrcIP      string
	DstIP      string
	Protocol   *uint8
	Limit      int
	Offset     int
}

// AggregateResult 聚合结果
type AggregateResult struct {
	Timestamp  time.Time
	Dimensions map[string]string
	Metrics    map[string]float64
}

// FlowAggregate 流量聚合查询
type FlowAggregate struct {
	TenantID   string
	StartTime  int64
	EndTime    int64
	GroupBy    []string
	Metrics    []string
}

// TimeSeriesStorage 时序存储接口
type TimeSeriesStorage interface {
	// 流量数据操作
	InsertFlow(ctx context.Context, flow *Flow) error
	InsertFlows(ctx context.Context, flows []*Flow) error
	QueryFlows(ctx context.Context, query *FlowQuery) ([]*Flow, error)
	AggregateFlows(ctx context.Context, agg *FlowAggregate) ([]*AggregateResult, error)

	// 通用SQL操作（兼容现有代码）
	Exec(ctx context.Context, sql string, args ...interface{}) (Result, error)
	Query(ctx context.Context, sql string, args ...interface{}) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) Row

	// 连接管理
	Ping(ctx context.Context) error
	PingContext(ctx context.Context) error
	Close() error

	// 获取原生DB（用于特殊场景）
	RawDB() *sql.DB

	// 数据库类型
	Type() DatabaseType
}

// KVStorage KV存储接口
type KVStorage interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string, value interface{}) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error

	// Hash操作
	HSet(ctx context.Context, key, field string, value interface{}) error
	HGet(ctx context.Context, key, field string, value interface{}) error
	HGetAll(ctx context.Context, key string) (map[string]string, error)

	// 连接管理
	Ping(ctx context.Context) error
	Close() error
}

// Dialect SQL方言接口
type Dialect interface {
	// DDL转换
	ConvertCreateTable(sql string) string
	ConvertCreateIndex(sql string) string
	// DML转换
	ConvertSelect(sql string) string
	ConvertInsert(sql string) string
	ConvertUpdate(sql string) string
	ConvertDelete(sql string) string
	// 函数映射
	MapFunction(funcName string, args ...string) string
	// 分页转换
	ApplyPagination(sql string, offset, limit int) string
	// 占位符转换
	ConvertPlaceholder(sql string, argCount int) string
}

// ==================== SQL包装类实现 ====================

// sqlResult 包装sql.Result
type sqlResult struct {
	res sql.Result
}

func (r *sqlResult) LastInsertId() (int64, error) {
	return r.res.LastInsertId()
}

func (r *sqlResult) RowsAffected() (int64, error) {
	return r.res.RowsAffected()
}

// sqlRows 包装sql.Rows
type sqlRows struct {
	rows *sql.Rows
}

func (r *sqlRows) Next() bool {
	return r.rows.Next()
}

func (r *sqlRows) Scan(dest ...interface{}) error {
	return r.rows.Scan(dest...)
}

func (r *sqlRows) Close() error {
	return r.rows.Close()
}

func (r *sqlRows) Err() error {
	return r.rows.Err()
}

func (r *sqlRows) Columns() ([]string, error) {
	return r.rows.Columns()
}

// sqlRow 包装sql.Row
type sqlRow struct {
	row *sql.Row
}

func (r *sqlRow) Scan(dest ...interface{}) error {
	return r.row.Scan(dest...)
}

// sqlTx 包装sql.Tx
type sqlTx struct {
	tx *sql.Tx
}

func (t *sqlTx) Commit() error {
	return t.tx.Commit()
}

func (t *sqlTx) Rollback() error {
	return t.tx.Rollback()
}

func (t *sqlTx) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	res, err := t.tx.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlResult{res: res}, nil
}

func (t *sqlTx) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	rows, err := t.tx.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows: rows}, nil
}

func (t *sqlTx) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	row := t.tx.QueryRowContext(ctx, sql, args...)
	return &sqlRow{row: row}
}

// Driver 数据库驱动接口
type Driver interface {
	// 打开连接
	OpenRelational(cfg *Config) (RelationalStorage, error)
	OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error)
	OpenKV(cfg *Config) (KVStorage, error)
	// 获取方言
	GetDialect() Dialect
	// 驱动类型
	Type() DatabaseType
}
