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
			name:     "反引号转双引号",
			input:    "SELECT `col1`, `col2` FROM `table`",
			expected: "SELECT \"col1\", \"col2\" FROM \"table\"",
		},
		{
			name:     "LIMIT语法",
			input:    "SELECT * FROM table LIMIT 10 OFFSET 20",
			expected: "SELECT * FROM table LIMIT 10 OFFSET 20",
		},
		{
			name:     "组合转换",
			input:    "SELECT `id`, IFNULL(`name`, '') FROM `users` WHERE created_at < NOW() LIMIT 10",
			expected: "SELECT \"id\", NVL(\"name\", '') FROM \"users\" WHERE created_at < SYSDATE LIMIT 10",
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

	input := `CREATE TABLE users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		age INT UNSIGNED,
		created_at DATETIME
	) ENGINE=InnoDB;`

	expected := `CREATE TABLE users (
		id INT IDENTITY PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		age INT,
		created_at DATETIME
	);`

	result := dialect.ConvertCreateTable(input)
	assert.Equal(t, expected, result)
}

func TestDamengDialect_QuoteIdentifier(t *testing.T) {
	dialect := &DamengDialect{}

	assert.Equal(t, "\"table_name\"", dialect.QuoteIdentifier("table_name"))
	assert.Equal(t, "\"column_name\"", dialect.QuoteIdentifier("column_name"))
}

func TestDamengDialect_GetPlaceHolder(t *testing.T) {
	dialect := &DamengDialect{}

	assert.Equal(t, "?", dialect.GetPlaceHolder(1))
	assert.Equal(t, "?", dialect.GetPlaceHolder(2))
}
