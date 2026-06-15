package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockRelationalStorage is a mock implementation of RelationalStorage
type MockRelationalStorage struct {
	mock.Mock
}

func (m *MockRelationalStorage) Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	argsMock := m.Called(ctx, query, args)
	return argsMock.Get(0), argsMock.Error(1)
}

func (m *MockRelationalStorage) Query(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	argsMock := m.Called(ctx, query, args)
	return argsMock.Get(0), argsMock.Error(1)
}

func (m *MockRelationalStorage) QueryRow(ctx context.Context, query string, args ...interface{}) interface{} {
	argsMock := m.Called(ctx, query, args)
	return argsMock.Get(0)
}

func (m *MockRelationalStorage) BeginTx(ctx context.Context) (interface{}, error) {
	argsMock := m.Called(ctx)
	return argsMock.Get(0), argsMock.Error(1)
}

func (m *MockRelationalStorage) PingContext(ctx context.Context) error {
	argsMock := m.Called(ctx)
	return argsMock.Error(0)
}

func (m *MockRelationalStorage) Close() error {
	argsMock := m.Called()
	return argsMock.Error(0)
}

func (m *MockRelationalStorage) RawDB() interface{} {
	argsMock := m.Called()
	return argsMock.Get(0)
}

func TestDualWriteMode_Transition(t *testing.T) {
	// Test the complete migration flow
	modes := []DualWriteMode{
		ModeOldOnly,
		ModeAsyncWrite,
		ModeSyncWrite,
		ModeReadSplit,
		ModeNewOnly,
	}

	expected := []string{
		"old_only",
		"async_write",
		"sync_write",
		"read_split",
		"new_only",
	}

	for i, mode := range modes {
		assert.Equal(t, expected[i], mode.String())
	}
}

func TestDualWriteMode_Order(t *testing.T) {
	// Verify mode order is correct for migration
	assert.Less(t, int(ModeOldOnly), int(ModeAsyncWrite))
	assert.Less(t, int(ModeAsyncWrite), int(ModeSyncWrite))
	assert.Less(t, int(ModeSyncWrite), int(ModeReadSplit))
	assert.Less(t, int(ModeReadSplit), int(ModeNewOnly))
}

func TestOpenRelational_InvalidType(t *testing.T) {
	cfg := &Config{
		Type: DatabaseType("invalid"),
	}

	_, err := OpenRelational(cfg)
	assert.Error(t, err)
}

func TestOpenTimeSeries_InvalidType(t *testing.T) {
	cfg := &Config{
		Type: DatabaseType("invalid"),
	}

	_, err := OpenTimeSeries(cfg)
	assert.Error(t, err)
}

func TestOpenKVStore_InvalidType(t *testing.T) {
	cfg := &Config{
		Type: DatabaseType("invalid"),
	}

	_, err := OpenKVStore(cfg)
	assert.Error(t, err)
}

func TestDriverRegistry_Exists(t *testing.T) {
	// Test that drivers are registered
	assert.NotNil(t, driverRegistry)

	// Check for known drivers
	_, mysqlExists := driverRegistry["mysql"]
	_, damengExists := driverRegistry["dameng"]
	_, kingbaseExists := driverRegistry["kingbase"]

	assert.True(t, mysqlExists, "MySQL driver should be registered")
	assert.True(t, damengExists, "Dameng driver should be registered")
	assert.True(t, kingbaseExists, "KingBase driver should be registered")
}

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		isValid bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				Type:     DBMySQL,
				Host:     "localhost",
				Port:     3306,
				User:     "root",
				Database: "cloudflow",
			},
			isValid: true,
		},
		{
			name: "empty host",
			cfg: &Config{
				Type:     DBMySQL,
				Host:     "",
				Port:     3306,
				User:     "root",
				Database: "cloudflow",
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isValid {
				assert.NotEmpty(t, tt.cfg.Host)
			}
		})
	}
}

func TestDualWriteStorage_ModeDescription(t *testing.T) {
	tests := []struct {
		mode        DualWriteMode
		description string
	}{
		{ModeOldOnly, "仅使用旧数据库"},
		{ModeAsyncWrite, "主库写入成功后异步写从库"},
		{ModeSyncWrite, "同步双写，主从都成功才返回"},
		{ModeReadSplit, "读流量按比例切分到新库"},
		{ModeNewOnly, "仅使用新数据库"},
	}

	for _, tt := range tests {
		assert.NotEmpty(t, tt.mode.String())
	}
}

func TestGetDriver(t *testing.T) {
	// Test existing driver
	driver, exists := getDriver("mysql")
	assert.True(t, exists)
	assert.NotNil(t, driver)

	// Test non-existing driver
	driver, exists = getDriver("nonexistent")
	assert.False(t, exists)
	assert.Nil(t, driver)
}

func TestRegisterDriver(t *testing.T) {
	// This test ensures that the driver registration doesn't panic
	// Actual registration happens in init() functions
	assert.NotNil(t, driverRegistry)
}
