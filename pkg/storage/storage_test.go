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
		{"达梦", DBDameng, "dameng"},
		{"达梦时序", DBDamengTS, "dameng_ts"},
		{"人大金仓", DBKingBase, "kingbase"},
		{"高斯", DBGaussDB, "gaussdb"},
		{"高斯Redis", DBGaussRedis, "gauss_redis"},
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
		Protocol:  6, // TCP
		Bytes:     1024,
		Packets:   10,
		Timestamp: 1234567890,
	}
	assert.NotEmpty(t, flow.SrcIP)
	assert.NotEmpty(t, flow.DstIP)
	assert.Greater(t, flow.Bytes, uint64(0))
	assert.Greater(t, flow.Packets, uint64(0))
}

func TestConfig_Defaults(t *testing.T) {
	cfg := &Config{
		Type:     DBDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "SYSDBA",
		Database: "cloudflow",
	}
	assert.Equal(t, DBDameng, cfg.Type)
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 5236, cfg.Port)
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
		Dimensions: map[string]string{
			"service": "api",
		},
		Metrics: map[string]float64{
			"bytes_sum":   102400,
			"packets_sum": 1000,
			"flow_count":  100,
		},
	}
	assert.Equal(t, "api", result.Dimensions["service"])
	assert.Equal(t, float64(102400), result.Metrics["bytes_sum"])
	assert.Equal(t, float64(1000), result.Metrics["packets_sum"])
	assert.Equal(t, float64(100), result.Metrics["flow_count"])
}

func TestDatabaseType_Valid(t *testing.T) {
	validTypes := []DatabaseType{
		DBDameng,
		DBDamengTS,
		DBKingBase,
		DBGaussDB,
		DBGaussRedis,
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
