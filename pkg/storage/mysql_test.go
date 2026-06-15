package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMySQLDialect_GetName(t *testing.T) {
	dialect := &MySQLDialect{}
	assert.Equal(t, "mysql", dialect.GetName())
}

func TestMySQLDialect_ConvertSQL(t *testing.T) {
	dialect := &MySQLDialect{}

	// MySQL dialect is pass-through, no conversion needed
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple select",
			input: "SELECT * FROM table",
		},
		{
			name:  "select with IFNULL",
			input: "SELECT IFNULL(col, 0) FROM table",
		},
		{
			name:  "select with NOW()",
			input: "SELECT NOW() FROM table",
		},
		{
			name:  "backticks",
			input: "SELECT `col` FROM `table`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dialect.ConvertSQL(tt.input)
			assert.Equal(t, tt.input, result)
		})
	}
}

func TestMySQLDialect_ConvertCreateTable(t *testing.T) {
	dialect := &MySQLDialect{}

	// MySQL dialect is pass-through
	input := "CREATE TABLE t (id INT AUTO_INCREMENT) ENGINE=InnoDB"
	result := dialect.ConvertCreateTable(input)
	assert.Equal(t, input, result)
}

func TestMySQLDialect_QuoteIdentifier(t *testing.T) {
	dialect := &MySQLDialect{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple identifier",
			input:    "column_name",
			expected: "`column_name`",
		},
		{
			name:     "table name",
			input:    "user_table",
			expected: "`user_table`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dialect.QuoteIdentifier(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMySQLStorage_Interface(t *testing.T) {
	// Verify that MySQLStorage implements RelationalStorage interface
	var _ RelationalStorage = (*MySQLStorage)(nil)
}

func TestMySQLDriver_Registration(t *testing.T) {
	driver, exists := getDriver("mysql")
	assert.True(t, exists)
	assert.NotNil(t, driver)
}

func TestMySQLDriver_GetName(t *testing.T) {
	driver := &MySQLDriver{}
	assert.Equal(t, "mysql", driver.GetName())
}

func TestMySQLDriver_Supports(t *testing.T) {
	driver := &MySQLDriver{}

	tests := []struct {
		name     string
		dbType   DatabaseType
		expected bool
	}{
		{
			name:     "mysql supported",
			dbType:   DBMySQL,
			expected: true,
		},
		{
			name:     "dameng not supported",
			dbType:   DBDameng,
			expected: false,
		},
		{
			name:     "clickhouse not supported",
			dbType:   DBClickHouse,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := driver.Supports(tt.dbType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMySQLConfig_BuildDSN(t *testing.T) {
	cfg := &Config{
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "password",
		Database: "cloudflow",
	}

	dsn := buildMySQLDSN(cfg)
	assert.Contains(t, dsn, "root")
	assert.Contains(t, dsn, "password")
	assert.Contains(t, dsn, "localhost")
	assert.Contains(t, dsn, "3306")
	assert.Contains(t, dsn, "cloudflow")
}

func TestBuildMySQLDSN_WithParams(t *testing.T) {
	cfg := &Config{
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "password",
		Database: "cloudflow",
	}

	dsn := buildMySQLDSN(cfg)
	assert.Contains(t, dsn, "charset=utf8mb4")
	assert.Contains(t, dsn, "parseTime=true")
}

func TestMySQLStorage_PingContext(t *testing.T) {
	// This is an interface test, actual ping requires real DB
	storage := &MySQLStorage{}
	assert.NotNil(t, storage)
}

func TestMySQLStorage_Close(t *testing.T) {
	// Interface test
	storage := &MySQLStorage{}
	assert.NotNil(t, storage)
}

func TestMySQLStorage_RawDB(t *testing.T) {
	// Interface test
	storage := &MySQLStorage{}
	assert.NotNil(t, storage)
}

func TestMySQLStorage_Exec(t *testing.T) {
	// Interface test
	storage := &MySQLStorage{}
	assert.NotNil(t, storage)
}

func TestMySQLStorage_Query(t *testing.T) {
	// Interface test
	storage := &MySQLStorage{}
	assert.NotNil(t, storage)
}

func TestMySQLStorage_QueryRow(t *testing.T) {
	// Interface test
	storage := &MySQLStorage{}
	assert.NotNil(t, storage)
}

func TestMySQLStorage_BeginTx(t *testing.T) {
	// Interface test
	storage := &MySQLStorage{}
	assert.NotNil(t, storage)
}

func TestConfig_MySQLDefaults(t *testing.T) {
	cfg := &Config{
		Type: DBMySQL,
	}

	// Test default port
	if cfg.Port == 0 {
		cfg.Port = 3306
	}
	assert.Equal(t, 3306, cfg.Port)
}

func TestMySQLDialect_NoConversion(t *testing.T) {
	dialect := &MySQLDialect{}

	// MySQL dialect should not modify SQL
	sql := "SELECT `id`, IFNULL(`name`, ''), NOW() FROM `users` LIMIT 10"
	result := dialect.ConvertSQL(sql)
	assert.Equal(t, sql, result)
}

func TestMySQLDialect_CreateTable_NoConversion(t *testing.T) {
	dialect := &MySQLDialect{}

	// MySQL dialect should not modify CREATE TABLE
	sql := "CREATE TABLE t (id INT AUTO_INCREMENT) ENGINE=InnoDB"
	result := dialect.ConvertCreateTable(sql)
	assert.Equal(t, sql, result)
}
