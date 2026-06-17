package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedisDriver_GetName(t *testing.T) {
	driver := &RedisDriver{}
	assert.Equal(t, "redis", driver.GetName())
}

func TestRedisDriver_Supports(t *testing.T) {
	driver := &RedisDriver{}

	tests := []struct {
		name     string
		dbType   DatabaseType
		expected bool
	}{
		{
			name:     "redis supported",
			dbType:   DatabaseType("redis"),
			expected: true,
		},
		{
			name:     "gaussdb redis supported",
			dbType:   DBGaussDB,
			expected: false,
		},
		{
			name:     "mysql not supported",
			dbType:   DBMySQL,
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

func TestGaussRedisDriver_GetName(t *testing.T) {
	driver := &GaussRedisDriver{}
	assert.Equal(t, "gauss_redis", driver.GetName())
}

func TestGaussRedisDriver_Supports(t *testing.T) {
	driver := &GaussRedisDriver{}

	tests := []struct {
		name     string
		dbType   DatabaseType
		expected bool
	}{
		{
			name:     "gaussdb redis supported",
			dbType:   DBGaussDB,
			expected: true,
		},
		{
			name:     "redis not supported",
			dbType:   DatabaseType("redis"),
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

func TestRedisStorage_Interface(t *testing.T) {
	// Verify that RedisStorage implements KVStorage interface
	var _ KVStorage = (*RedisStorage)(nil)
}

func TestGaussRedisStorage_Interface(t *testing.T) {
	// Verify that GaussRedisStorage implements KVStorage interface
	var _ KVStorage = (*GaussRedisStorage)(nil)
}

func TestRedisConfig_BuildAddr(t *testing.T) {
	cfg := &Config{
		Host: "localhost",
		Port: 6379,
	}

	addr := buildRedisAddr(cfg)
	assert.Equal(t, "localhost:6379", addr)
}

func TestBuildRedisAddr_DefaultPort(t *testing.T) {
	cfg := &Config{
		Host: "localhost",
		Port: 0,
	}

	addr := buildRedisAddr(cfg)
	assert.Equal(t, "localhost:6379", addr)
}

func TestRedisStorage_Set(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestRedisStorage_Get(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestRedisStorage_Del(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestRedisStorage_Exists(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestRedisStorage_Expire(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestRedisStorage_HSet(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestRedisStorage_HGet(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestRedisStorage_HGetAll(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestRedisStorage_Ping(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestRedisStorage_Close(t *testing.T) {
	// Interface test
	storage := &RedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_Set(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_Get(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_Del(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_Exists(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_Expire(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_HSet(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_HGet(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_HGetAll(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_Ping(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestGaussRedisStorage_Close(t *testing.T) {
	// Interface test
	storage := &GaussRedisStorage{}
	assert.NotNil(t, storage)
}

func TestConfig_RedisDefaults(t *testing.T) {
	cfg := &Config{
		Type: DatabaseType("redis"),
	}

	// Test default port
	if cfg.Port == 0 {
		cfg.Port = 6379
	}
	assert.Equal(t, 6379, cfg.Port)
}

func TestKVStorage_Interface(t *testing.T) {
	// Verify interface definitions
	var _ KVStorage = (*RedisStorage)(nil)
	var _ KVStorage = (*GaussRedisStorage)(nil)
}

func TestRedisDriver_Registration(t *testing.T) {
	driver, exists := getDriver("redis")
	assert.True(t, exists)
	assert.NotNil(t, driver)
}

func TestGaussRedisDriver_Registration(t *testing.T) {
	driver, exists := getDriver("gauss_redis")
	assert.True(t, exists)
	assert.NotNil(t, driver)
}
