package storage

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DamengDriver 达梦DM8驱动实现
type DamengDriver struct{}

// DamengStorage 达梦DM8关系型存储实现
type DamengStorage struct {
	db  *sql.DB
	cfg *Config
}

// DamengDialect 达梦DM8方言实现
type DamengDialect struct{}

func init() {
	RegisterDriver(DatabaseDameng, &DamengDriver{})
}

// ==================== 达梦DM8驱动 ====================

func (d *DamengDriver) Type() DatabaseType {
	return DatabaseDameng
}

func (d *DamengDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	// 达梦DM8连接串格式
	dsn := fmt.Sprintf("dm://%s:%s@%s:%d?schema=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("dm", dsn)
	if err != nil {
		return nil, fmt.Errorf("open dameng failed: %w", err)
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
		return nil, fmt.Errorf("ping dameng failed: %w", err)
	}

	return &DamengStorage{
		db:  db,
		cfg: cfg,
	}, nil
}

func (d *DamengDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	// 达梦DM8时序版支持
	return nil, fmt.Errorf("dameng time series storage not implemented yet")
}

func (d *DamengDriver) OpenKV(cfg *Config) (KVStorage, error) {
	return nil, fmt.Errorf("dameng kv storage not implemented yet")
}

func (d *DamengDriver) GetDialect() Dialect {
	return &DamengDialect{}
}

// ==================== 达梦DM8存储实现 ====================

func (s *DamengStorage) Type() DatabaseType {
	return DatabaseDameng
}

func (s *DamengStorage) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
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

func (s *DamengStorage) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	// SQL语法转换
	dialect := &DamengDialect{}
	sql = dialect.ConvertSelect(sql)

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows: rows}, nil
}

func (s *DamengStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	// SQL语法转换
	dialect := &DamengDialect{}
	sql = dialect.ConvertSelect(sql)

	row := s.db.QueryRowContext(ctx, sql, args...)
	return &sqlRow{row: row}
}

func (s *DamengStorage) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &damengTx{tx: tx}, nil
}

func (s *DamengStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *DamengStorage) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *DamengStorage) Close() error {
	return s.db.Close()
}

func (s *DamengStorage) RawDB() *sql.DB {
	return s.db
}

// ==================== 达梦DM8方言实现 ====================

var (
	limitRegex    = regexp.MustCompile(`(?i)\s+LIMIT\s+(\d+)(?:\s+OFFSET\s+(\d+))?`)
	ifnullRegex   = regexp.MustCompile(`(?i)IFNULL\s*\(`)
	autoIncRegex  = regexp.MustCompile(`(?i)AUTO_INCREMENT`)
	unsignedRegex = regexp.MustCompile(`(?i)\s+UNSIGNED`)
	backtickRegex = regexp.MustCompile("`")
)

func (d *DamengDialect) ConvertCreateTable(sql string) string {
	result := sql

	// 转换反引号为双引号
	result = backtickRegex.ReplaceAllString(result, "\"")

	// 转换 AUTO_INCREMENT -> IDENTITY
	result = autoIncRegex.ReplaceAllString(result, "IDENTITY(1,1)")

	// 移除 UNSIGNED
	result = unsignedRegex.ReplaceAllString(result, "")

	// 转换 TIMESTAMP ON UPDATE
	result = strings.ReplaceAll(result, "ON UPDATE CURRENT_TIMESTAMP", "")

	// 转换 ENGINE=InnoDB
	result = regexp.MustCompile(`(?i)\s*ENGINE\s*=\s*\w+`).ReplaceAllString(result, "")

	return result
}

func (d *DamengDialect) ConvertCreateIndex(sql string) string {
	// 转换反引号为双引号
	return backtickRegex.ReplaceAllString(sql, "\"")
}

func (d *DamengDialect) ConvertSelect(sql string) string {
	result := sql

	// 转换反引号为双引号
	result = backtickRegex.ReplaceAllString(result, "\"")

	// 转换 IFNULL -> NVL
	result = ifnullRegex.ReplaceAllString(result, "NVL(")

	// 转换 NOW() -> SYSDATE
	result = regexp.MustCompile(`(?i)NOW\s*\(\s*\)`).ReplaceAllString(result, "SYSDATE")

	// 转换 LIMIT -> ROWNUM 或 达梦LIMIT语法（达梦支持LIMIT）
	// 达梦DM8支持标准 LIMIT OFFSET 语法，无需转换

	return result
}

func (d *DamengDialect) ConvertInsert(sql string) string {
	result := sql

	// 转换反引号为双引号
	result = backtickRegex.ReplaceAllString(result, "\"")

	// 转换 ON DUPLICATE KEY UPDATE -> MERGE
	// 注意：复杂的MERGE转换需要更复杂的处理，这里先标记
	if strings.Contains(strings.ToLower(result), "on duplicate key update") {
		// 简单场景：记录日志，后续手动处理
		// 实际项目中可以实现更复杂的MERGE转换
	}

	return result
}

func (d *DamengDialect) ConvertUpdate(sql string) string {
	// 转换反引号为双引号
	return backtickRegex.ReplaceAllString(sql, "\"")
}

func (d *DamengDialect) ConvertDelete(sql string) string {
	// 转换反引号为双引号
	return backtickRegex.ReplaceAllString(sql, "\"")
}

func (d *DamengDialect) MapFunction(funcName string, args ...string) string {
	funcName = strings.ToLower(funcName)
	switch funcName {
	case "ifnull":
		if len(args) >= 2 {
			return fmt.Sprintf("NVL(%s, %s)", args[0], args[1])
		}
	case "now":
		return "SYSDATE"
	case "unix_timestamp":
		return "(SYSDATE - TO_DATE('1970-01-01', 'YYYY-MM-DD')) * 86400"
	case "from_unixtime":
		if len(args) >= 1 {
			return fmt.Sprintf("TO_DATE('1970-01-01', 'YYYY-MM-DD') + %s/86400", args[0])
		}
	case "group_concat":
		if len(args) >= 1 {
			return fmt.Sprintf("LISTAGG(%s, ',') WITHIN GROUP (ORDER BY %s)", args[0], args[0])
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))
	}
	return funcName + "()"
}

func (d *DamengDialect) ApplyPagination(sql string, offset, limit int) string {
	// 达梦DM8支持 LIMIT OFFSET 语法
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, limit, offset)
}

func (d *DamengDialect) ConvertPlaceholder(sql string, argCount int) string {
	// 达梦DM8使用?作为占位符，与MySQL兼容
	return sql
}

// ==================== 达梦事务包装类（带SQL转换） ====================

type damengTx struct {
	tx *sql.Tx
}

func (tx *damengTx) Commit() error {
	return tx.tx.Commit()
}

func (tx *damengTx) Rollback() error {
	return tx.tx.Rollback()
}

func (tx *damengTx) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	dialect := &DamengDialect{}
	sql = dialect.ConvertUpdate(sql)

	res, err := tx.tx.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlResult{res: res}, nil
}

func (tx *damengTx) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	dialect := &DamengDialect{}
	sql = dialect.ConvertSelect(sql)

	rows, err := tx.tx.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows: rows}, nil
}

func (tx *damengTx) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	dialect := &DamengDialect{}
	sql = dialect.ConvertSelect(sql)

	row := tx.tx.QueryRowContext(ctx, sql, args...)
	return &sqlRow{row: row}
}
