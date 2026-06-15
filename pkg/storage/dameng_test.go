package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDamengDialect_ConvertSQL(t *testing.T) {
	dialect := &DamengDialect{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "IFNULL to NVL",
			input:    "SELECT IFNULL(col, 0) FROM table",
			expected: "SELECT NVL(col, 0) FROM table",
		},
		{
			name:     "NOW() to SYSDATE",
			input:    "SELECT NOW() FROM table",
			expected: "SELECT SYSDATE FROM table",
		},
		{
			name:     "Backtick to double quote",
			input:    "SELECT `col1`, `col2` FROM `table`",
			expected: "SELECT \"col1\", \"col2\" FROM \"table\"",
		},
		{
			name:     "LIMIT syntax",
			input:    "SELECT * FROM table LIMIT 10",
			expected: "SELECT * FROM table LIMIT 10",
		},
		{
			name:     "LIMIT OFFSET syntax",
			input:    "SELECT * FROM table LIMIT 10 OFFSET 20",
			expected: "SELECT * FROM table LIMIT 10 OFFSET 20",
		},
		{
			name:     "Multiple IFNULL conversions",
			input:    "SELECT IFNULL(a, 0), IFNULL(b, '') FROM t",
			expected: "SELECT NVL(a, 0), NVL(b, '') FROM t",
		},
		{
			name:     "Mixed backticks and IFNULL",
			input:    "SELECT IFNULL(`name`, 'unknown') FROM `users`",
			expected: "SELECT NVL(\"name\", 'unknown') FROM \"users\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dialect.ConvertSQL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDamengDialect_ConvertCreateTable(t *testing.T) {
	dialect := &DamengDialect{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "AUTO_INCREMENT to IDENTITY",
			input:    "id INT AUTO_INCREMENT PRIMARY KEY",
			expected: "id INT IDENTITY PRIMARY KEY",
		},
		{
			name:     "Remove UNSIGNED",
			input:    "count INT UNSIGNED",
			expected: "count INT",
		},
		{
			name:     "Remove ENGINE=InnoDB",
			input:    ") ENGINE=InnoDB DEFAULT CHARSET=utf8",
			expected: ")",
		},
		{
			name:     "Multiple conversions",
			input:    "id INT AUTO_INCREMENT, count INT UNSIGNED) ENGINE=InnoDB",
			expected: "id INT IDENTITY, count INT)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dialect.ConvertCreateTable(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDamengDialect_QuoteIdentifier(t *testing.T) {
	dialect := &DamengDialect{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple identifier",
			input:    "column_name",
			expected: "\"column_name\"",
		},
		{
			name:     "table name",
			input:    "user_table",
			expected: "\"user_table\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dialect.QuoteIdentifier(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDamengDialect_GetName(t *testing.T) {
	dialect := &DamengDialect{}
	assert.Equal(t, "dameng", dialect.GetName())
}

func TestConvertIFNULL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single IFNULL",
			input:    "IFNULL(col, default_val)",
			expected: "NVL(col, default_val)",
		},
		{
			name:     "IFNULL with spaces",
			input:    "IFNULL( col , 0 )",
			expected: "NVL( col , 0 )",
		},
		{
			name:     "no IFNULL",
			input:    "SELECT col FROM table",
			expected: "SELECT col FROM table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertIFNULL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertNOW(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "NOW() function",
			input:    "NOW()",
			expected: "SYSDATE",
		},
		{
			name:     "NOW() in query",
			input:    "SELECT NOW() FROM t",
			expected: "SELECT SYSDATE FROM t",
		},
		{
			name:     "no NOW()",
			input:    "SELECT col FROM table",
			expected: "SELECT col FROM table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertNOW(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertBackticks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single backtick",
			input:    "`column`",
			expected: "\"column\"",
		},
		{
			name:     "multiple backticks",
			input:    "`col1`, `col2`",
			expected: "\"col1\", \"col2\"",
		},
		{
			name:     "no backticks",
			input:    "column",
			expected: "column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertBackticks(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveUnsigned(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "INT UNSIGNED",
			input:    "INT UNSIGNED",
			expected: "INT",
		},
		{
			name:     "BIGINT UNSIGNED",
			input:    "BIGINT UNSIGNED",
			expected: "BIGINT",
		},
		{
			name:     "no UNSIGNED",
			input:    "INT",
			expected: "INT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeUnsigned(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveEngine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ENGINE=InnoDB",
			input:    ") ENGINE=InnoDB",
			expected: ")",
		},
		{
			name:     "ENGINE=InnoDB with charset",
			input:    ") ENGINE=InnoDB DEFAULT CHARSET=utf8",
			expected: ")",
		},
		{
			name:     "no ENGINE",
			input:    ")",
			expected: ")",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeEngine(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertAutoIncrement(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "AUTO_INCREMENT",
			input:    "AUTO_INCREMENT",
			expected: "IDENTITY",
		},
		{
			name:     "INT AUTO_INCREMENT",
			input:    "INT AUTO_INCREMENT",
			expected: "INT IDENTITY",
		},
		{
			name:     "no AUTO_INCREMENT",
			input:    "INT",
			expected: "INT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertAutoIncrement(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDamengDialect_Integration(t *testing.T) {
	dialect := &DamengDialect{}

	// Test a complete SQL conversion
	input := "SELECT `id`, IFNULL(`name`, ''), NOW() FROM `users` LIMIT 10"
	expected := "SELECT \"id\", NVL(\"name\", ''), SYSDATE FROM \"users\" LIMIT 10"

	result := dialect.ConvertSQL(input)
	assert.Equal(t, expected, result)
}

func TestDamengDialect_CreateTableIntegration(t *testing.T) {
	dialect := &DamengDialect{}

	// Test a complete CREATE TABLE conversion
	input := `CREATE TABLE users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(100),
		age INT UNSIGNED
	) ENGINE=InnoDB DEFAULT CHARSET=utf8`

	// The engine and unsigned should be removed
	result := dialect.ConvertCreateTable(input)
	assert.NotContains(t, result, "AUTO_INCREMENT")
	assert.NotContains(t, result, "UNSIGNED")
	assert.NotContains(t, result, "ENGINE")
}
