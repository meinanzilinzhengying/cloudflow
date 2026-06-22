package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type ClickHouseDriver struct{}

type ClickHouseStorage struct {
	conn chdriver.Conn
	db   *sql.DB
	cfg  *Config
}

func init() {
	RegisterDriver(DatabaseClickHouse, &ClickHouseDriver{})
}

func (d *ClickHouseDriver) Type() DatabaseType { return DatabaseClickHouse }

func (d *ClickHouseDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	return nil, fmt.Errorf("clickhouse does not support relational storage")
}

func (d *ClickHouseDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if cfg.Port == 0 { cfg.Port = 9000 }
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{Database: cfg.Database, Username: cfg.User, Password: cfg.Password},
		DialTimeout: 10 * time.Second, MaxOpenConns: 50, MaxIdleConns: 10,
	})
	if err != nil { return nil, fmt.Errorf("open clickhouse failed: %w", err) }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil { return nil, fmt.Errorf("ping clickhouse failed: %w", err) }
	dsn := fmt.Sprintf("clickhouse://%s:%s@%s/%s", cfg.User, cfg.Password, addr, cfg.Database)
	db, err := sql.Open("clickhouse", dsn)
	if err != nil { return nil, fmt.Errorf("open clickhouse sql: %w", err) }
	return &ClickHouseStorage{conn: conn, db: db, cfg: cfg}, nil
}

func (d *ClickHouseDriver) OpenKV(cfg *Config) (KVStorage, error) {
	return nil, fmt.Errorf("clickhouse does not support kv storage")
}
func (d *ClickHouseDriver) GetDialect() Dialect { return &ClickHouseDialect{} }

func (s *ClickHouseStorage) Type() DatabaseType { return DatabaseClickHouse }
func (s *ClickHouseStorage) InsertFlow(ctx context.Context, flow *Flow) error {
	return nil
}
func (s *ClickHouseStorage) InsertFlows(ctx context.Context, flows []*Flow) error { return nil }
func (s *ClickHouseStorage) QueryFlows(ctx context.Context, query *FlowQuery) ([]*Flow, error) { return nil, nil }
func (s *ClickHouseStorage) AggregateFlows(ctx context.Context, agg *FlowAggregate) ([]*AggregateResult, error) { return nil, nil }
func (s *ClickHouseStorage) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	err := s.conn.Exec(ctx, sql, args...)
	if err != nil { return nil, err }
	return &chResult{}, nil
}
func (s *ClickHouseStorage) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil { return nil, err }
	return rows, nil
}
func (s *ClickHouseStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	return s.conn.QueryRow(ctx, sql, args...)
}
func (s *ClickHouseStorage) Ping(ctx context.Context) error { return s.conn.Ping(ctx) }
func (s *ClickHouseStorage) PingContext(ctx context.Context) error { return s.conn.Ping(ctx) }
func (s *ClickHouseStorage) Close() error { if s.db != nil { s.db.Close() }
	return nil
}
func (s *ClickHouseStorage) RawDB() *sql.DB { return s.db }

type ClickHouseDialect struct{}
func (d *ClickHouseDialect) ConvertCreateTable(sql string) string { return sql }
func (d *ClickHouseDialect) ConvertCreateIndex(sql string) string { return sql }
func (d *ClickHouseDialect) ConvertSelect(sql string) string { return sql }
func (d *ClickHouseDialect) ConvertInsert(sql string) string { return sql }
func (d *ClickHouseDialect) ConvertUpdate(sql string) string { return sql }
func (d *ClickHouseDialect) ConvertDelete(sql string) string { return sql }
func (d *ClickHouseDialect) MapFunction(funcName string, args ...string) string {
	if len(args) > 0 { return fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", ")) }
	return funcName + "()"
}
func (d *ClickHouseDialect) ApplyPagination(sql string, offset, limit int) string { return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, limit, offset) }
func (d *ClickHouseDialect) ConvertPlaceholder(sql string, argCount int) string { return sql }

type chResult struct{}
func (r *chResult) LastInsertId() (int64, error) { return 0, nil }
func (r *chResult) RowsAffected() (int64, error) { return 0, nil }

type chRows struct{ rows chdriver.Rows }
func (r *chRows) Next() bool { return r.rows.Next() }
func (r *chRows) Scan(dest ...interface{}) error { return r.rows.Scan(dest...) }
func (r *chRows) Close() error { return r.rows.Close() }
func (r *chRows) Err() error { return r.rows.Err() }
func (r *chRows) Columns() ([]string, error) { cols := r.rows.Columns(); return cols, nil }

type chRow struct{ row chdriver.Row }
func (r *chRow) Scan(dest ...interface{}) error { return r.row.Scan(dest...) }
