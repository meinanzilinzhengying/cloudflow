package tidb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStorageUserCRUD 测试用户 CRUD 操作
func TestStorageUserCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN:      "root:@tcp(localhost:4000)/cloudflow_auth",
		MaxOpenConns: 10,
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()

	// 创建用户
	user := &User{
		ID:       "test-user-001",
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "admin",
		TenantID: "tenant-001",
	}

	err = storage.CreateUser(ctx, user)
	assert.NoError(t, err)

	// 查询用户
	retrieved, err := storage.GetUserByID(ctx, user.ID)
	assert.NoError(t, err)
	assert.Equal(t, user.Username, retrieved.Username)

	// 更新用户
	user.Email = "updated@example.com"
	err = storage.UpdateUser(ctx, user)
	assert.NoError(t, err)

	// 验证更新
	retrieved, err = storage.GetUserByID(ctx, user.ID)
	assert.NoError(t, err)
	assert.Equal(t, "updated@example.com", retrieved.Email)

	// 删除用户
	err = storage.DeleteUser(ctx, user.ID)
	assert.NoError(t, err)

	// 验证删除
	_, err = storage.GetUserByID(ctx, user.ID)
	assert.Error(t, err)
}

// TestStorageTenantCRUD 测试租户 CRUD
func TestStorageTenantCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN: "root:@tcp(localhost:4000)/cloudflow_auth",
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()

	// 创建租户
	tenant := &Tenant{
		ID:          "test-tenant-001",
		Name:        "Test Tenant",
		Description: "Test description",
		Status:      "active",
	}

	err = storage.CreateTenant(ctx, tenant)
	assert.NoError(t, err)

	// 查询租户
	retrieved, err := storage.GetTenantByID(ctx, tenant.ID)
	assert.NoError(t, err)
	assert.Equal(t, tenant.Name, retrieved.Name)

	// 列出所有租户
	tenants, err := storage.ListTenants(ctx, ListOptions{
		Limit:  100,
		Offset: 0,
	})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(tenants), 1)

	// 清理
	err = storage.DeleteTenant(ctx, tenant.ID)
	assert.NoError(t, err)
}

// TestStorageAPIKey 测试 API Key 管理
func TestStorageAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN: "root:@tcp(localhost:4000)/cloudflow_auth",
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()

	// 创建 API Key
	apiKey := &APIKey{
		Key:        "test-api-key-12345",
		UserID:     "user-001",
		TenantID:   "tenant-001",
		Permission: "read_write",
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}

	err = storage.CreateAPIKey(ctx, apiKey)
	assert.NoError(t, err)

	// 验证 API Key
	valid, err := storage.ValidateAPIKey(ctx, apiKey.Key)
	assert.NoError(t, err)
	assert.True(t, valid)

	// 撤销 API Key
	err = storage.RevokeAPIKey(ctx, apiKey.Key)
	assert.NoError(t, err)

	// 验证已撤销
	valid, err = storage.ValidateAPIKey(ctx, apiKey.Key)
	assert.NoError(t, err)
	assert.False(t, valid)
}

// TestStorageTransaction 测试事务支持
func TestStorageTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN: "root:@tcp(localhost:4000)/cloudflow_auth",
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()

	// 测试事务提交
	err = storage.WithTransaction(ctx, func(tx Tx) error {
		user := &User{
			ID:       "tx-user-001",
			Username: "txuser",
			Email:    "tx@example.com",
			Role:     "user",
			TenantID: "tenant-001",
		}
		return tx.CreateUser(ctx, user)
	})
	assert.NoError(t, err)

	// 测试事务回滚
	err = storage.WithTransaction(ctx, func(tx Tx) error {
		user := &User{
			ID:       "tx-user-002",
			Username: "txuser2",
			Email:    "tx2@example.com",
			Role:     "user",
			TenantID: "tenant-001",
		}
		err := tx.CreateUser(ctx, user)
		if err != nil {
			return err
		}
		// 故意返回错误触发回滚
		return assert.AnError
	})
	assert.Error(t, err)

	// 验证回滚（第二个用户不应存在）
	_, err = storage.GetUserByID(ctx, "tx-user-002")
	assert.Error(t, err)
}

// TestStorageConnectionPool 测试连接池
func TestStorageConnectionPool(t *testing.T) {
	cfg := Config{
		DSN:           "root:@tcp(localhost:4000)/cloudflow_auth",
		MaxOpenConns:  20,
		MaxIdleConns:  10,
		ConnMaxLifetime: 30 * time.Minute,
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	stats := storage.DB().Stats()
	assert.Equal(t, 20, stats.MaxOpenConnections)
}

// TestStorageHealthCheck 测试健康检查
func TestStorageHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := Config{
		DSN: "root:@tcp(localhost:4000)/cloudflow_auth",
	}

	storage, err := NewStorage(cfg)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	
	err = storage.Ping(ctx)
	assert.NoError(t, err)
}
