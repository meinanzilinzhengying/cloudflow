package reliable

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/pkg/logger"
	"github.com/stretchr/testify/assert"
)

func TestReporterConfig(t *testing.T) {
	config := Config{
		CacheDir:            "/tmp/test-cache",
		MaxCacheDuration:    1 * time.Hour,
		RetransmitBatchSize: 100,
		RetransmitInterval:  100 * time.Millisecond,
		SendTimeout:         10 * time.Second,
		EnableChecksum:      true,
		MaxCacheSizeBytes:   100 * 1024 * 1024, // 100MB
	}

	log := logger.New(logger.Config{Level: "info"})

	// 创建临时目录用于测试
	tmpDir, err := os.MkdirTemp("", "reliable-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config.CacheDir = tmpDir

	reporter, err := NewReporter(config, nil, nil, log)
	assert.NoError(t, err, "Reporter 应成功创建")
	assert.NotNil(t, reporter, "Reporter 不应为 nil")
}

func TestCacheDirectoryCreation(t *testing.T) {
	log := logger.New(logger.Config{Level: "info"})

	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "cache-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cacheDir := filepath.Join(tmpDir, "subdir", "cache")

	config := Config{
		CacheDir:         cacheDir,
		MaxCacheDuration: 1 * time.Hour,
		SendTimeout:      5 * time.Second,
	}

	reporter, err := NewReporter(config, nil, nil, log)
	assert.NoError(t, err, "应自动创建缓存目录")
	assert.NotNil(t, reporter)

	// 验证目录已创建
	_, err = os.Stat(cacheDir)
	assert.NoError(t, err, "缓存目录应存在")
}

func TestConfigValidation(t *testing.T) {
	log := logger.New(logger.Config{Level: "info"})

	tmpDir, err := os.MkdirTemp("", "config-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				CacheDir:         tmpDir,
				MaxCacheDuration: 1 * time.Hour,
				SendTimeout:      5 * time.Second,
			},
			expectError: false,
		},
		{
			name: "empty cache dir",
			config: Config{
				CacheDir:         "",
				MaxCacheDuration: 1 * time.Hour,
				SendTimeout:      5 * time.Second,
			},
			expectError: false, // 应该使用默认值
		},
		{
			name: "zero timeout",
			config: Config{
				CacheDir:         tmpDir,
				MaxCacheDuration: 1 * time.Hour,
				SendTimeout:      0,
			},
			expectError: false, // 应该使用默认值
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter, err := NewReporter(tt.config, nil, nil, log)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, reporter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, reporter)
			}
		})
	}
}

func TestCacheSizeLimit(t *testing.T) {
	config := Config{
		CacheDir:          "/tmp/test-cache-size",
		MaxCacheDuration:  1 * time.Hour,
		SendTimeout:       5 * time.Second,
		MaxCacheSizeBytes: 1024, // 1KB 限制
	}

	log := logger.New(logger.Config{Level: "info"})

	tmpDir, err := os.MkdirTemp("", "size-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config.CacheDir = tmpDir

	reporter, err := NewReporter(config, nil, nil, log)
	assert.NoError(t, err)
	assert.NotNil(t, reporter)

	// 验证配置已应用
	assert.Equal(t, int64(1024), reporter.config.MaxCacheSizeBytes)
}

func TestRetransmitConfig(t *testing.T) {
	config := Config{
		CacheDir:            "/tmp/test-retransmit",
		MaxCacheDuration:    30 * time.Minute,
		RetransmitBatchSize: 50,
		RetransmitInterval:  200 * time.Millisecond,
		SendTimeout:         10 * time.Second,
	}

	log := logger.New(logger.Config{Level: "info"})

	tmpDir, err := os.MkdirTemp("", "retransmit-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config.CacheDir = tmpDir

	reporter, err := NewReporter(config, nil, nil, log)
	assert.NoError(t, err)
	assert.NotNil(t, reporter)

	// 验证重传配置
	assert.Equal(t, 50, reporter.config.RetransmitBatchSize)
	assert.Equal(t, 200*time.Millisecond, reporter.config.RetransmitInterval)
}

func TestChecksumEnabled(t *testing.T) {
	config := Config{
		CacheDir:         "/tmp/test-checksum",
		MaxCacheDuration: 1 * time.Hour,
		SendTimeout:      5 * time.Second,
		EnableChecksum:   true,
	}

	log := logger.New(logger.Config{Level: "info"})

	tmpDir, err := os.MkdirTemp("", "checksum-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config.CacheDir = tmpDir

	reporter, err := NewReporter(config, nil, nil, log)
	assert.NoError(t, err)
	assert.True(t, reporter.config.EnableChecksum, "校验和应启用")
}

func TestDefaultValues(t *testing.T) {
	log := logger.New(logger.Config{Level: "info"})

	tmpDir, err := os.MkdirTemp("", "default-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := Config{
		CacheDir: tmpDir,
		// 其他字段使用默认值
	}

	reporter, err := NewReporter(config, nil, nil, log)
	assert.NoError(t, err)
	assert.NotNil(t, reporter)

	// 验证默认值已应用
	assert.NotZero(t, reporter.config.MaxCacheDuration, "MaxCacheDuration 应有默认值")
	assert.NotZero(t, reporter.config.SendTimeout, "SendTimeout 应有默认值")
}
