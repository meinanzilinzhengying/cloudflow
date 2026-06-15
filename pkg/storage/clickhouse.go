// Package storage 数据库抽象层
// ClickHouse时序存储驱动实现
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// ClickHouseDriver ClickHouse驱动实现
type ClickHouseDriver struct{}

// ClickHouseStorage ClickHouse时序存储实现
type ClickHouseStorage struct {
	db  *sql.DB
	cfg *Config
}

// ClickHouseDialect ClickHouse方言实现
type ClickHouseDialect struct{}

func init() {
	RegisterDriver(DatabaseClickHouse, &ClickHouseDriver{})
}

// ==================== ClickHouse驱动 ====================

func (d *ClickHouseDriver) Type() DatabaseType {
	return DatabaseClickHouse
}

func (d *ClickHouseDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	return nil, fmt.Errorf("clickhouse does not support relational storage")
}

func (d *ClickHouseDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	// ClickHouse连接串格式
	dsn := fmt.Sprintf("clickhouse://%s:%s@%s:%d/%s?dial_timeout=5s&max_execution_time=60",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse failed: %w", err)
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
		return nil, fmt.Errorf("ping clickhouse failed: %w", err)
	}

	return &ClickHouseStorage{
		db:  db,
		cfg: cfg,
	}, nil
}

func (d *ClickHouseDriver) OpenKV(cfg *Config) (KVStorage, error) {
	return nil, fmt.Errorf("clickhouse does not support kv storage")
}

func (d *ClickHouseDriver) GetDialect() Dialect {
	return &ClickHouseDialect{}
}

// ==================== ClickHouse存储实现 ====================

func (s *ClickHouseStorage) Type() DatabaseType {
	return DatabaseClickHouse
}

func (s *ClickHouseStorage) InsertFlow(ctx context.Context, flow *Flow) error {
	query := `
		INSERT INTO flows (
			timestamp, flow_id, src_ip, dst_ip, src_port, dst_port,
			protocol, l7_protocol, bytes, packets, latency_ns,
			tenant_id, namespace, service, pod, node
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	tagsJSON := "{}"
	if flow.Tags != nil {
		// 简单JSON序列化
		var parts []string
		for k, v := range flow.Tags {
			parts = append(parts, fmt.Sprintf(`"%s":"%s"`, k, v))
		}
		tagsJSON = "{" + strings.Join(parts, ",") + "}"
	}

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

func (s *ClickHouseStorage) InsertFlows(ctx context.Context, flows []*Flow) error {
	// ClickHouse批量插入
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

func (s *ClickHouseStorage) QueryFlows(ctx context.Context, query *FlowQuery) ([]*Flow, error) {
	sql := `
		SELECT 
			toUnixTimestamp64Nano(timestamp), flow_id, src_ip, dst_ip, src_port, dst_port,
			protocol, l7_protocol, bytes, packets, latency_ns,
			tenant_id, namespace, service, pod, node
		FROM flows
		WHERE tenant_id = ?
		  AND timestamp >= toDateTime64(?, 9)
		  AND timestamp <= toDateTime64(?, 9)
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

func (s *ClickHouseStorage) AggregateFlows(ctx context.Context, agg *FlowAggregate) ([]*AggregateResult, error) {
	// 构建聚合查询
	selectCols := []string{"toStartOfMinute(timestamp) as ts"}
	groupByCols := []string{"ts"}

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
		  AND timestamp >= toDateTime64(?, 9)
		  AND timestamp <= toDateTime64(?, 9)
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

		// 动态扫描结果
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

func (s *ClickHouseStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *ClickHouseStorage) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *ClickHouseStorage) Close() error {
	return s.db.Close()
}

// ==================== ClickHouse方言实现 ====================

func (d *ClickHouseDialect) ConvertCreateTable(sql string) string {
	// ClickHouse原生语法
	return sql
}

func (d *ClickHouseDialect) ConvertCreateIndex(sql string) string {
	return sql
}

func (d *ClickHouseDialect) ConvertSelect(sql string) string {
	return sql
}

func (d *ClickHouseDialect) ConvertInsert(sql string) string {
	return sql
}

func (d *ClickHouseDialect) ConvertUpdate(sql string) string {
	return sql
}

func (d *ClickHouseDialect) ConvertDelete(sql string) string {
	return sql
}

func (d *ClickHouseDialect) MapFunction(funcName string, args ...string) string {
	funcName = strings.ToLower(funcName)
	switch funcName {
	case "tostartofminute":
		if len(args) >= 1 {
			return fmt.Sprintf("toStartOfMinute(%s)", args[0])
		}
	case "toyyyymmdd":
		if len(args) >= 1 {
			return fmt.Sprintf("toYYYYMMDD(%s)", args[0])
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))
	}
	return funcName + "()"
}

func (d *ClickHouseDialect) ApplyPagination(sql string, offset, limit int) string {
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, limit, offset)
}

func (d *ClickHouseDialect) ConvertPlaceholder(sql string, argCount int) string {
	// ClickHouse使用?作为占位符
	return sql
}
