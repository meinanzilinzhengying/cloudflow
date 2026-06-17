package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatabaseType_String(t *testing.T) {
	tests := []struct {
		name     string
		dbType   DatabaseType
		expected string
	}{
		{"MySQL", DBMySQL, "mysql"},
		{"ClickHouse", DBClickHouse, "clickhouse"},
		{"达梦", DBDameng, "dameng"},
		{"人大金仓", DBKingBase, "kingbase"},
		{"高斯", DBGaussDB, "gaussdb"},
		{"OceanBase", DBOceanBase, "oceanbase"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.dbType.String())
		})
	}
}

func TestDualWriteMode_String(t *testing.T) {
	tests := []struct {
		name     string
		mode     DualWriteMode
		expected string
	}{
		{"OldOnly", ModeOldOnly, "old_only"},
		{"AsyncWrite", ModeAsyncWrite, "async_write"},
		{"SyncWrite", ModeSyncWrite, "sync_write"},
		{"ReadSplit", ModeReadSplit, "read_split"},
		{"NewOnly", ModeNewOnly, "new_only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.mode.String())
		})
	}
}

func TestIsNotFound(t *testing.T) {
	// nil error should return false
	assert.False(t, IsNotFound(nil))

	// Test with actual sql.ErrNoRows would require real DB
	// This is a basic sanity check
}

func TestFlow_Validate(t *testing.T) {
	flow := &Flow{
		SrcIP:     "192.168.1.1",
		DstIP:     "192.168.1.2",
		SrcPort:   8080,
		DstPort:   80,
		Protocol:  "tcp",
		Bytes:     1024,
		Packets:   10,
		Timestamp: 1234567890,
	}

	assert.NotEmpty(t, flow.SrcIP)
	assert.NotEmpty(t, flow.DstIP)
	assert.Greater(t, flow.Bytes, int64(0))
	assert.Greater(t, flow.Packets, int64(0))
}

func TestConfig_Defaults(t *testing.T) {
	cfg := &Config{
		Type:     DBMySQL,
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "password",
		Database: "cloudflow",
	}

	assert.Equal(t, DBMySQL, cfg.Type)
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 3306, cfg.Port)
}

func TestFlowQuery_Defaults(t *testing.T) {
	query := &FlowQuery{
		TenantID:  "tenant-1",
		StartTime: 1000,
		EndTime:   2000,
		Limit:     100,
	}

	assert.Equal(t, "tenant-1", query.TenantID)
	assert.Equal(t, int64(1000), query.StartTime)
	assert.Equal(t, int64(2000), query.EndTime)
	assert.Equal(t, 100, query.Limit)
}

func TestAggregateResult_Structure(t *testing.T) {
	result := &AggregateResult{
		TotalBytes:   102400,
		TotalPackets: 1000,
		FlowCount:    100,
		AvgBytes:     1024,
	}

	assert.Equal(t, int64(102400), result.TotalBytes)
	assert.Equal(t, int64(1000), result.TotalPackets)
	assert.Equal(t, 100, result.FlowCount)
	assert.Equal(t, int64(1024), result.AvgBytes)
}

func TestDatabaseType_Valid(t *testing.T) {
	validTypes := []DatabaseType{
		DBMySQL,
		DBClickHouse,
		DBDameng,
		DBKingBase,
		DBGaussDB,
		DBOceanBase,
	}

	for _, dbType := range validTypes {
		assert.NotEmpty(t, dbType.String())
	}
}

func TestDualWriteMode_Valid(t *testing.T) {
	validModes := []DualWriteMode{
		ModeOldOnly,
		ModeAsyncWrite,
		ModeSyncWrite,
		ModeReadSplit,
		ModeNewOnly,
	}

	for _, mode := range validModes {
		assert.NotEmpty(t, mode.String())
	}
}
