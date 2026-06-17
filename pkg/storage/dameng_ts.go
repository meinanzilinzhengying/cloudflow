// Package storage 数据库抽象层
// 达梦DM8时序版驱动实现
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DamengTSDriver 达梦DM8时序版驱动实现
type DamengTSDriver struct{}

// DamengTSStorage 达梦DM8时序存储实现
type DamengTSStorage struct {
	db  *sql.DB
	cfg *Config
}

// DamengTSDialect 达梦DM8时序方言实现
type DamengTSDialect struct{}

func init() {
	RegisterDriver(DatabaseDamengTS, &DamengTSDriver{})
}

// ==================== 达梦DM8时序版驱动 ====================

func (d *DamengTSDriver) Type() DatabaseType {
	return DatabaseDamengTS
}

func (d *DamengTSDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	return nil, fmt.Errorf("dameng ts does not support relational storage")
}

func (d *DamengTSDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	// 达梦DM8时序版连接串
	dsn := fmt.Sprintf("dm://%s:%s@%s:%d?schema=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("dm", dsn)
	if err != nil {
		return nil, fmt.Errorf("open dameng ts failed: %w", err)
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
		return nil, fmt.Errorf("ping dameng ts failed: %w", err)
	}

	return &DamengTSStorage{
		db:  db,
		cfg: cfg,
	}, nil
}

func (d *DamengTSDriver) OpenKV(cfg *Config) (KVStorage, error) {
	return nil, fmt.Errorf("dameng ts does not support kv storage")
}

func (d *DamengTSDriver) GetDialect() Dialect {
	return &DamengTSDialect{}
}

// ==================== 达梦DM8时序存储实现 ====================

func (s *DamengTSStorage) Type() DatabaseType {
	return DatabaseDamengTS
}

func (s *DamengTSStorage) InsertFlow(ctx context.Context, flow *Flow) error {
	query := `
		INSERT INTO flows (
			timestamp, flow_id, src_ip, dst_ip, src_port, dst_port,
			protocol, l7_protocol, bytes, packets, latency_ns,
			tenant_id, namespace, service, pod, node
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		time.Unix(0, flow.Timestamp),
		flow.FlowID,
		flow.SrcIP,
		flow.DstIP,
		flow.SrcPort,
		flow.DstPort,
		flow.Protocol,
		flow.L7Protocol,
		flow.Bytes,
		flow.Packets,
		flow.LatencyNs,
		flow.TenantID,
		flow.Namespace,
		flow.Service,
		flow.Pod,
		flow.Node,
	)
	return err
}

func (s *DamengTSStorage) InsertFlows(ctx context.Context, flows []*Flow) error {
	// 达梦批量插入
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO flows (
			timestamp, flow_id, src_ip, dst_ip, src_port, dst_port,
			protocol, l7_protocol, bytes, packets, latency_ns,
			tenant_id, namespace, service, pod, node
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, flow := range flows {
		_, err := stmt.ExecContext(ctx,
			time.Unix(0, flow.Timestamp),
			flow.FlowID,
			flow.SrcIP,
			flow.DstIP,
			flow.SrcPort,
			flow.DstPort,
			flow.Protocol,
			flow.L7Protocol,
			flow.Bytes,
			flow.Packets,
			flow.LatencyNs,
			flow.TenantID,
			flow.Namespace,
			flow.Service,
			flow.Pod,
			flow.Node,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *DamengTSStorage) QueryFlows(ctx context.Context, query *FlowQuery) ([]*Flow, error) {
	sql := `
		SELECT 
			CAST((timestamp - TO_DATE('1970-01-01', 'YYYY-MM-DD')) * 86400000000000 AS BIGINT),
			flow_id, src_ip, dst_ip, src_port, dst_port,
			protocol, l7_protocol, bytes, packets, latency_ns,
			tenant_id, namespace, service, pod, node
		FROM flows
		WHERE tenant_id = ?
		  AND timestamp >= TO_DATE('1970-01-01', 'YYYY-MM-DD') + ?/86400000000000
		  AND timestamp <= TO_DATE('1970-01-01', 'YYYY-MM-DD') + ?/86400000000000
	`

	args := []interface{}{query.TenantID, query.StartTime, query.EndTime}

	if query.SrcIP != "" {
		sql += " AND src_ip = ?"
		args = append(args, query.SrcIP)
	}
	if query.DstIP != "" {
		sql += " AND dst_ip = ?"
		args = append(args, query.DstIP)
	}
	if query.Protocol != nil {
		sql += " AND protocol = ?"
		args = append(args, *query.Protocol)
	}

	sql += " ORDER BY timestamp DESC"

	if query.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", query.Limit)
		if query.Offset > 0 {
			sql += fmt.Sprintf(" OFFSET %d", query.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flows []*Flow
	for rows.Next() {
		flow := &Flow{Tags: make(map[string]string)}
		err := rows.Scan(
			&flow.Timestamp,
			&flow.FlowID,
			&flow.SrcIP,
			&flow.DstIP,
			&flow.SrcPort,
			&flow.DstPort,
			&flow.Protocol,
			&flow.L7Protocol,
			&flow.Bytes,
			&flow.Packets,
			&flow.LatencyNs,
			&flow.TenantID,
			&flow.Namespace,
			&flow.Service,
			&flow.Pod,
			&flow.Node,
		)
		if err != nil {
			return nil, err
		}
		flows = append(flows, flow)
	}

	return flows, rows.Err()
}

func (s *DamengTSStorage) AggregateFlows(ctx context.Context, agg *FlowAggregate) ([]*AggregateResult, error) {
	// 达梦时序聚合查询
	selectCols := []string{"TRUNC(timestamp, 'MI') as ts"}
	groupByCols := []string{"TRUNC(timestamp, 'MI')"}

	for _, dim := range agg.GroupBy {
		selectCols = append(selectCols, dim)
		groupByCols = append(groupByCols, dim)
	}

	for _, metric := range agg.Metrics {
		switch metric {
		case "bytes_sum":
			selectCols = append(selectCols, "SUM(bytes) as bytes_sum")
		case "packets_sum":
			selectCols = append(selectCols, "SUM(packets) as packets_sum")
		case "flow_count":
			selectCols = append(selectCols, "COUNT(*) as flow_count")
		case "avg_latency":
			selectCols = append(selectCols, "AVG(latency_ns) as avg_latency")
		}
	}

	sql := fmt.Sprintf(`
		SELECT %s
		FROM flows
		WHERE tenant_id = ?
		  AND timestamp >= TO_DATE('1970-01-01', 'YYYY-MM-DD') + ?/86400000000000
		  AND timestamp <= TO_DATE('1970-01-01', 'YYYY-MM-DD') + ?/86400000000000
		GROUP BY %s
		ORDER BY ts DESC
	`, strings.Join(selectCols, ", "), strings.Join(groupByCols, ", "))

	rows, err := s.db.QueryContext(ctx, sql, agg.TenantID, agg.StartTime, agg.EndTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*AggregateResult
	for rows.Next() {
		result := &AggregateResult{
			Dimensions: make(map[string]string),
			Metrics:    make(map[string]float64),
		}

		var ts time.Time
		dest := []interface{}{&ts}
		for range agg.GroupBy {
			var s string
			dest = append(dest, &s)
		}
		for range agg.Metrics {
			var f float64
			dest = append(dest, &f)
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}

		result.Timestamp = ts
		idx := 1
		for _, dim := range agg.GroupBy {
			result.Dimensions[dim] = *dest[idx].(*string)
			idx++
		}
		for _, metric := range agg.Metrics {
			result.Metrics[metric] = *dest[idx].(*float64)
			idx++
		}

		results = append(results, result)
	}

	return results, rows.Err()
}

func (s *DamengTSStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *DamengTSStorage) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *DamengTSStorage) Close() error {
	return s.db.Close()
}

// ==================== 通用SQL操作 ====================

func (s *DamengTSStorage) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	// SQL语法转换
	dialect := &DamengDialect{}
	sql = dialect.ConvertUpdate(sql)
	sql = dialect.ConvertDelete(sql)
	sql = dialect.ConvertInsert(sql)

	res, err := s.db.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlResult{res: res}, nil
}

func (s *DamengTSStorage) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	// SQL语法转换
	dialect := &DamengDialect{}
	sql = dialect.ConvertSelect(sql)

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows: rows}, nil
}

func (s *DamengTSStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	// SQL语法转换
	dialect := &DamengDialect{}
	sql = dialect.ConvertSelect(sql)

	row := s.db.QueryRowContext(ctx, sql, args...)
	return &sqlRow{row: row}
}

func (s *DamengTSStorage) RawDB() *sql.DB {
	return s.db
}

// ==================== 达梦DM8时序方言实现 ====================

func (d *DamengTSDialect) ConvertCreateTable(sql string) string {
	// 转换ClickHouse语法到达梦
	result := sql
	result = strings.ReplaceAll(result, "ENGINE = MergeTree()", "")
	result = strings.ReplaceAll(result, "ORDER BY", "PRIMARY KEY")
	return result
}

func (d *DamengTSDialect) ConvertCreateIndex(sql string) string {
	return sql
}

func (d *DamengTSDialect) ConvertSelect(sql string) string {
	result := sql
	// ClickHouse函数转换
	result = strings.ReplaceAll(result, "toStartOfMinute(", "TRUNC(")
	result = strings.ReplaceAll(result, "toYYYYMMDD(", "TO_CHAR(")
	return result
}

func (d *DamengTSDialect) ConvertInsert(sql string) string {
	return sql
}

func (d *DamengTSDialect) ConvertUpdate(sql string) string {
	return sql
}

func (d *DamengTSDialect) ConvertDelete(sql string) string {
	return sql
}

func (d *DamengTSDialect) MapFunction(funcName string, args ...string) string {
	funcName = strings.ToLower(funcName)
	switch funcName {
	case "tostartofminute":
		if len(args) >= 1 {
			return fmt.Sprintf("TRUNC(%s, 'MI')", args[0])
		}
	case "toyyyymmdd":
		if len(args) >= 1 {
			return fmt.Sprintf("TO_CHAR(%s, 'YYYYMMDD')", args[0])
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))
	}
	return funcName + "()"
}

func (d *DamengTSDialect) ApplyPagination(sql string, offset, limit int) string {
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, limit, offset)
}

func (d *DamengTSDialect) ConvertPlaceholder(sql string, argCount int) string {
	return sql
}
