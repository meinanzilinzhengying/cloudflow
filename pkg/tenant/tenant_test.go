package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	sharedTenant "github.com/meinanzilinzhengying/cloudflow/services/shared/tenant"
)

// ============================================================================
// 一、StorageRowFilter 测试
// ============================================================================

func TestNewStorageRowFilter(t *testing.T) {
	f := NewStorageRowFilter(true)
	assert.NotNil(t, f)
	assert.True(t, f.strictMode)
}

func TestFilterSQL(t *testing.T) {
	f := NewStorageRowFilter(true)

	// 有租户 context
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})

	// 基本查询注入
	sql, err := f.FilterSQL(ctx, "SELECT * FROM flows")
	assert.NoError(t, err)
	assert.Contains(t, sql, "tenant_id = 'tenant-123'")

	// 已有 WHERE 的查询注入
	sql, err = f.FilterSQL(ctx, "SELECT * FROM flows WHERE status = 'active'")
	assert.NoError(t, err)
	assert.Contains(t, sql, "AND tenant_id = 'tenant-123'")

	// 无租户 context（严格模式）
	ctxNoTenant := context.Background()
	_, err = f.FilterSQL(ctxNoTenant, "SELECT * FROM flows")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing tenant_id")
}

func TestFilterSQL_PlatformAdmin(t *testing.T) {
	f := NewStorageRowFilter(true)

	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID:        "admin-tenant",
		IsPlatformAdmin: true,
	})

	// 平台管理员不注入过滤
	sql, err := f.FilterSQL(ctx, "SELECT * FROM flows")
	assert.NoError(t, err)
	assert.Equal(t, "SELECT * FROM flows", sql)
}

func TestFilterSQL_WithGroupBy(t *testing.T) {
	f := NewStorageRowFilter(true)
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})

	sql, err := f.FilterSQL(ctx, "SELECT * FROM flows GROUP BY service")
	assert.NoError(t, err)
	assert.Contains(t, sql, "WHERE tenant_id = 'tenant-123'")
	assert.Contains(t, sql, "GROUP BY service")
}

func TestFilterSQL_WithLimit(t *testing.T) {
	f := NewStorageRowFilter(true)
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})

	sql, err := f.FilterSQL(ctx, "SELECT * FROM flows LIMIT 100")
	assert.NoError(t, err)
	assert.Contains(t, sql, "WHERE tenant_id = 'tenant-123'")
	assert.Contains(t, sql, "LIMIT 100")
}

func TestFilterClickHouseQuery(t *testing.T) {
	f := NewStorageRowFilter(true)
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})

	sql, err := f.FilterClickHouseQuery(ctx, "SELECT * FROM flows WHERE timestamp > now() - 3600")
	assert.NoError(t, err)
	assert.Contains(t, sql, "AND tenant_id = 'tenant-123'")
}

func TestFilterStoragePath(t *testing.T) {
	f := NewStorageRowFilter(true)
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})

	path, err := f.FilterStoragePath(ctx, "data/flows/2024")
	assert.NoError(t, err)
	assert.Equal(t, "tenant-123/data/flows/2024", path)

	// 已包含租户前缀
	path, err = f.FilterStoragePath(ctx, "tenant-123/data/flows/2024")
	assert.NoError(t, err)
	assert.Equal(t, "tenant-123/data/flows/2024", path)

	// 目录穿越防护
	path, err = f.FilterStoragePath(ctx, "../data/flows")
	assert.NoError(t, err)
	assert.Equal(t, "tenant-123/data/flows", path)
}

func TestValidateStoragePath(t *testing.T) {
	f := NewStorageRowFilter(true)
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})

	// 合法路径
	err := f.ValidateStoragePath(ctx, "tenant-123/data/flows")
	assert.NoError(t, err)

	// 跨租户路径
	err = f.ValidateStoragePath(ctx, "tenant-456/data/flows")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong to tenant")
}

func TestEnforceTenantAccess(t *testing.T) {
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})

	// 同租户访问
	err := EnforceTenantAccess(ctx, "tenant-123")
	assert.NoError(t, err)

	// 跨租户访问
	err = EnforceTenantAccess(ctx, "tenant-456")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cross-tenant access")

	// 平台管理员
	ctxAdmin := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID:        "admin-tenant",
		IsPlatformAdmin: true,
	})
	err = EnforceTenantAccess(ctxAdmin, "tenant-456")
	assert.NoError(t, err)
}

func TestEnforceTenantAccessWithResource(t *testing.T) {
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})

	// 合法资源访问
	err := EnforceTenantAccessWithResource(ctx, "tenant-123", "flow", "tenant-123-uuid-1")
	assert.NoError(t, err)

	// 资源归属不匹配
	err = EnforceTenantAccessWithResource(ctx, "tenant-123", "flow", "tenant-456-uuid-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource ownership mismatch")
}

func TestFilterTenantList(t *testing.T) {
	type Item struct {
		ID       string
		TenantID string
	}

	items := []Item{
		{ID: "1", TenantID: "tenant-123"},
		{ID: "2", TenantID: "tenant-456"},
		{ID: "3", TenantID: "tenant-123"},
	}

	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})

	filtered := FilterTenantList(ctx, items, func(i Item) string { return i.TenantID })
	assert.Len(t, filtered, 2)
	assert.Equal(t, "1", filtered[0].ID)
	assert.Equal(t, "3", filtered[1].ID)
}

func TestMustHaveTenantID(t *testing.T) {
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})
	assert.Equal(t, "tenant-123", MustHaveTenantID(ctx))

	// 无租户 context
	assert.Panics(t, func() {
		MustHaveTenantID(context.Background())
	})
}

func TestGetTenantID(t *testing.T) {
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})
	assert.Equal(t, "tenant-123", GetTenantID(ctx))
	assert.Equal(t, "", GetTenantID(context.Background()))
}

func TestIsPlatformAdmin(t *testing.T) {
	ctxAdmin := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID:        "admin",
		IsPlatformAdmin: true,
	})
	assert.True(t, IsPlatformAdmin(ctxAdmin))

	ctxNormal := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-123",
	})
	assert.False(t, IsPlatformAdmin(ctxNormal))
}

// ============================================================================
// 二、滑动窗口计数器测试
// ============================================================================

func TestSlidingWindowCounter(t *testing.T) {
	c := NewSlidingWindowCounter(100 * time.Millisecond)

	c.Add(5)
	c.Add(3)
	assert.Equal(t, int64(8), c.Get())

	// 等待窗口过期
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int64(0), c.Get())

	// 新计数
	c.Add(10)
	assert.Equal(t, int64(10), c.Get())
}

func TestSlidingWindowCounter_Reset(t *testing.T) {
	c := NewSlidingWindowCounter(time.Minute)
	c.Add(5)
	assert.Equal(t, int64(5), c.Get())
	c.Reset()
	assert.Equal(t, int64(0), c.Get())
}

// ============================================================================
// 三、配额管理器测试
// ============================================================================

func TestNewQuotaManager(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	assert.NotNil(t, m)
	assert.Equal(t, OverflowReject, m.GetOverflowPolicy())
	assert.Equal(t, CheckModeSync, m.GetCheckMode())
	assert.Equal(t, 0, m.GetTenantCount())
}

func TestRegisterAndGetQuotaConfig(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	config := &QuotaConfig{
		MaxFlowsPerMin:  5000,
		MaxStorageBytes: 10 * 1024 * 1024 * 1024,
		MaxAgentCount:   50,
	}
	m.RegisterTenant("tenant-123", config)

	retrieved := m.GetQuotaConfig("tenant-123")
	assert.NotNil(t, retrieved)
	assert.Equal(t, int64(5000), retrieved.MaxFlowsPerMin)
	assert.Equal(t, int64(10*1024*1024*1024), retrieved.MaxStorageBytes)
	assert.Equal(t, 50, retrieved.MaxAgentCount)
	assert.Equal(t, "tenant-123", retrieved.TenantID)

	assert.Equal(t, 1, m.GetTenantCount())
}

func TestCheckFlows(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	m.RegisterTenant("tenant-123", &QuotaConfig{
		MaxFlowsPerMin: 100,
		MaxFlowsPerDay: 10000,
	})

	ctx := context.Background()

	// 未超限
	result, err := m.CheckFlows(ctx, "tenant-123", 50)
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.False(t, result.Exceeded)

	// 记录使用后再检查
	m.RecordFlows("tenant-123", 80)
	result, err = m.CheckFlows(ctx, "tenant-123", 30)
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.Exceeded)
	assert.Contains(t, result.Message, "exceeded")
}

func TestCheckStorage(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	m.RegisterTenant("tenant-123", &QuotaConfig{
		MaxStorageBytes: 100 * 1024 * 1024 * 1024, // 100GB
	})

	ctx := context.Background()

	// 设置使用量
	m.RecordStorage("tenant-123", 80*1024*1024*1024)

	// 未超限
	result, err := m.CheckStorage(ctx, "tenant-123", 10*1024*1024*1024)
	assert.NoError(t, err)
	assert.True(t, result.Allowed)

	// 超限
	result, err = m.CheckStorage(ctx, "tenant-123", 30*1024*1024*1024)
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.Exceeded)
}

func TestCheckAgentCount(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	m.RegisterTenant("tenant-123", &QuotaConfig{
		MaxAgentCount: 10,
	})

	ctx := context.Background()
	m.RecordAgentCount("tenant-123", 8)

	// 未超限
	result, err := m.CheckAgentCount(ctx, "tenant-123", 2)
	assert.NoError(t, err)
	assert.True(t, result.Allowed)

	// 超限
	result, err = m.CheckAgentCount(ctx, "tenant-123", 5)
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.Exceeded)
}

func TestCheckAlertRules(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	m.RegisterTenant("tenant-123", &QuotaConfig{
		MaxAlertRules: 5,
	})

	ctx := context.Background()
	m.RecordAlertRuleCount("tenant-123", 4)

	result, err := m.CheckAlertRules(ctx, "tenant-123", 2)
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.Exceeded)
}

func TestCheckAPIRate(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	m.RegisterTenant("tenant-123", &QuotaConfig{
		MaxAPIRateLimit: 100,
	})

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		m.RecordAPICall("tenant-123")
	}

	result, err := m.CheckAPIRate(ctx, "tenant-123")
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.Exceeded)
}

func TestCheckAll(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	m.RegisterTenant("tenant-123", &QuotaConfig{
		MaxFlowsPerMin:  100,
		MaxStorageBytes: 1024 * 1024 * 1024,
		MaxAgentCount:   10,
		MaxAlertRules:   5,
		MaxAPIRateLimit: 100,
	})

	ctx := context.Background()
	m.RecordFlows("tenant-123", 150) // 超限
	m.RecordStorage("tenant-123", 2*1024*1024*1024) // 超限

	results := m.CheckAll(ctx, "tenant-123")
	assert.True(t, len(results) > 0)
	assert.NotNil(t, results[QuotaTypeFlowsPerMin])
	assert.NotNil(t, results[QuotaTypeStorageBytes])
}

func TestOverflowPolicy(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowThrottle, CheckModeSync)
	m.RegisterTenant("tenant-123", &QuotaConfig{
		MaxFlowsPerMin: 100,
	})
	m.RecordFlows("tenant-123", 150)

	ctx := context.Background()
	result, err := m.CheckFlows(ctx, "tenant-123", 10)
	assert.NoError(t, err)
	assert.True(t, result.Allowed) // Throttle 策略允许通过
	assert.True(t, result.Exceeded)
	assert.Contains(t, result.Message, "throttled")
}

func TestQuotaManager_UpdateQuota(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	m.RegisterTenant("tenant-123", &QuotaConfig{
		MaxFlowsPerMin: 100,
	})

	ctx := context.Background()
	newConfig := &QuotaConfig{
		MaxFlowsPerMin:  200,
		MaxStorageBytes: 200 * 1024 * 1024 * 1024,
	}
	m.SetPersistFunc(func(ctx context.Context, tenantID string, config *QuotaConfig) error {
		return nil
	})

	err := m.UpdateQuota(ctx, "tenant-123", newConfig)
	assert.NoError(t, err)

	retrieved := m.GetQuotaConfig("tenant-123")
	assert.Equal(t, int64(200), retrieved.MaxFlowsPerMin)
}

func TestQuotaManager_DeleteTenant(t *testing.T) {
	m := NewQuotaManager(time.Minute, OverflowReject, CheckModeSync)
	m.RegisterTenant("tenant-123", &QuotaConfig{
		MaxFlowsPerMin: 100,
	})
	assert.Equal(t, 1, m.GetTenantCount())

	m.DeleteTenant("tenant-123")
	assert.Equal(t, 0, m.GetTenantCount())
	assert.Nil(t, m.GetQuotaConfig("tenant-123"))
}

func TestPlanQuotas(t *testing.T) {
	assert.NotNil(t, PlanQuotas["free"])
	assert.NotNil(t, PlanQuotas["pro"])
	assert.NotNil(t, PlanQuotas["enterprise"])

	assert.Greater(t, PlanQuotas["pro"].MaxFlowsPerMin, PlanQuotas["free"].MaxFlowsPerMin)
	assert.Greater(t, PlanQuotas["enterprise"].MaxFlowsPerMin, PlanQuotas["pro"].MaxFlowsPerMin)
}

// ============================================================================
// 四、生命周期管理器测试
// ============================================================================

func TestNewTenantLifecycleManager(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	assert.NotNil(t, m)
	assert.Equal(t, 0, m.GetTenantCount())
}

func TestRegisterTenant(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusActive)

	info := m.GetTenantInfo("tenant-123")
	assert.NotNil(t, info)
	assert.Equal(t, TenantStatusActive, info.Status)
	assert.Equal(t, "tenant-123", info.TenantID)
	assert.Equal(t, 7, info.GracePeriodDays)
	assert.Equal(t, int64(1), info.Version)
}

func TestSuspendTenant(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusActive)

	ctx := context.Background()
	err := m.SuspendTenant(ctx, "tenant-123", "payment_overdue", "admin")
	assert.NoError(t, err)

	info := m.GetTenantInfo("tenant-123")
	assert.Equal(t, TenantStatusSuspended, info.Status)
	assert.Equal(t, "payment_overdue", info.SuspendedReason)
	assert.Equal(t, "admin", info.SuspendedBy)
	assert.NotNil(t, info.SuspendedAt)
}

func TestSuspendTenant_Idempotent(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusSuspended)

	ctx := context.Background()
	err := m.SuspendTenant(ctx, "tenant-123", "test", "admin")
	assert.NoError(t, err) // 已暂停，幂等
}

func TestSuspendTenant_Deleted(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusDeleted)

	ctx := context.Background()
	err := m.SuspendTenant(ctx, "tenant-123", "test", "admin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already deleted")
}

func TestActivateTenant(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusSuspended)

	ctx := context.Background()
	err := m.ActivateTenant(ctx, "tenant-123")
	assert.NoError(t, err)

	info := m.GetTenantInfo("tenant-123")
	assert.Equal(t, TenantStatusActive, info.Status)
	assert.Nil(t, info.SuspendedAt)
	assert.Equal(t, "", info.SuspendedReason)
}

func TestRequestDeletion(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusActive)

	ctx := context.Background()
	err := m.RequestDeletion(ctx, "tenant-123", "user_request", "user-1")
	assert.NoError(t, err)

	info := m.GetTenantInfo("tenant-123")
	assert.Equal(t, TenantStatusPendingDeletion, info.Status)
	assert.NotNil(t, info.DeletionRequestedAt)
	assert.Equal(t, "user_request", info.DeletionReason)
	assert.Equal(t, "user-1", info.DeletionRequestedBy)
}

func TestCancelDeletion(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusPendingDeletion)

	ctx := context.Background()
	err := m.CancelDeletion(ctx, "tenant-123")
	assert.NoError(t, err)

	info := m.GetTenantInfo("tenant-123")
	assert.Equal(t, TenantStatusActive, info.Status)
	assert.Nil(t, info.DeletionRequestedAt)
}

func TestExecuteDeletion(t *testing.T) {
	m := NewTenantLifecycleManager(0) // 宽限期 0 天
	m.RegisterTenant("tenant-123", TenantStatusPendingDeletion)

	// 设置删除请求时间（过去）
	m.mu.Lock()
	now := time.Now().Unix()
	m.infos["tenant-123"].DeletionRequestedAt = &now
	m.mu.Unlock()

	ctx := context.Background()
	err := m.ExecuteDeletion(ctx, "tenant-123", "admin")
	assert.NoError(t, err)

	info := m.GetTenantInfo("tenant-123")
	assert.Equal(t, TenantStatusDeleted, info.Status)
	assert.NotNil(t, info.DeletedAt)
}

func TestExecuteDeletion_GracePeriodNotExpired(t *testing.T) {
	m := NewTenantLifecycleManager(7) // 宽限期 7 天
	m.RegisterTenant("tenant-123", TenantStatusPendingDeletion)

	now := time.Now().Unix()
	m.mu.Lock()
	m.infos["tenant-123"].DeletionRequestedAt = &now
	m.mu.Unlock()

	ctx := context.Background()
	err := m.ExecuteDeletion(ctx, "tenant-123", "admin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "grace period not expired")
}

func TestDeleteTenantImmediately(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusActive)

	// 平台管理员 context
	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID:        "admin",
		IsPlatformAdmin: true,
	})

	err := m.DeleteTenantImmediately(ctx, "tenant-123", "admin")
	assert.NoError(t, err)

	info := m.GetTenantInfo("tenant-123")
	assert.Equal(t, TenantStatusDeleted, info.Status)
}

func TestDeleteTenantImmediate_NotAdmin(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusActive)

	ctx := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-456",
	})

	err := m.DeleteTenantImmediately(ctx, "tenant-123", "user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only platform admin")
}

func TestValidateTenantStatus(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusActive)
	m.RegisterTenant("tenant-suspended", TenantStatusSuspended)
	m.RegisterTenant("tenant-deleted", TenantStatusDeleted)
	m.RegisterTenant("tenant-pending", TenantStatusPendingDeletion)

	ctx := context.Background()

	assert.NoError(t, m.ValidateTenantStatus(ctx, "tenant-123"))

	err := m.ValidateTenantStatus(ctx, "tenant-suspended")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "suspended")

	err = m.ValidateTenantStatus(ctx, "tenant-deleted")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deleted")

	err = m.ValidateTenantStatus(ctx, "tenant-pending")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pending deletion")
}

func TestValidateTenantWriteAccess(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusActive)
	m.RegisterTenant("tenant-suspended", TenantStatusSuspended)

	ctx := context.Background()

	assert.NoError(t, m.ValidateTenantWriteAccess(ctx, "tenant-123"))

	err := m.ValidateTenantWriteAccess(ctx, "tenant-suspended")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not allow write")
}

func TestGetExpiredPendingDeletionTenants(t *testing.T) {
	m := NewTenantLifecycleManager(0)
	m.RegisterTenant("tenant-123", TenantStatusPendingDeletion)
	m.RegisterTenant("tenant-456", TenantStatusPendingDeletion)

	now := time.Now().Unix()
	m.mu.Lock()
	m.infos["tenant-123"].DeletionRequestedAt = &now
	m.infos["tenant-456"].DeletionRequestedAt = &now
	m.mu.Unlock()

	expired := m.GetExpiredPendingDeletionTenants()
	assert.Len(t, expired, 2)
}

func TestGetAllTenantsByStatus(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-1", TenantStatusActive)
	m.RegisterTenant("tenant-2", TenantStatusActive)
	m.RegisterTenant("tenant-3", TenantStatusSuspended)

	active := m.GetAllTenantsByStatus(TenantStatusActive)
	assert.Len(t, active, 2)

	suspended := m.GetAllTenantsByStatus(TenantStatusSuspended)
	assert.Len(t, suspended, 1)
	assert.Equal(t, "tenant-3", suspended[0])
}

func TestTenantStatusMethods(t *testing.T) {
	assert.True(t, TenantStatusActive.IsActive())
	assert.True(t, TenantStatusSuspended.IsSuspended())
	assert.True(t, TenantStatusPendingDeletion.IsPendingDeletion())
	assert.True(t, TenantStatusDeleted.IsDeleted())

	assert.True(t, TenantStatusActive.CanAccess())
	assert.True(t, TenantStatusSuspended.CanAccess())
	assert.False(t, TenantStatusDeleted.CanAccess())

	assert.True(t, TenantStatusActive.CanWrite())
	assert.False(t, TenantStatusSuspended.CanWrite())
	assert.False(t, TenantStatusDeleted.CanWrite())

	assert.False(t, TenantStatusActive.CanDelete())
	assert.True(t, TenantStatusPendingDeletion.CanDelete())
}

func TestLifecycleHook(t *testing.T) {
	m := NewTenantLifecycleManager(7)
	m.RegisterTenant("tenant-123", TenantStatusActive)

	hookCalled := false
	m.RegisterHook(TenantStatusSuspended, func(ctx context.Context, tenantID string, from, to TenantStatus, info *TenantLifecycleInfo) error {
		hookCalled = true
		assert.Equal(t, "tenant-123", tenantID)
		assert.Equal(t, TenantStatusActive, from)
		assert.Equal(t, TenantStatusSuspended, to)
		return nil
	})

	ctx := context.Background()
	err := m.SuspendTenant(ctx, "tenant-123", "test", "admin")
	assert.NoError(t, err)
	assert.True(t, hookCalled)
}

// ============================================================================
// 五、使用量统计测试
// ============================================================================

func TestTenantUsage(t *testing.T) {
	u := NewTenantUsage(time.Minute)

	u.AddFlows(100)
	assert.Equal(t, int64(100), u.GetFlowsPerMin())

	u.SetStorageBytes(1024 * 1024 * 1024)
	assert.Equal(t, int64(1024*1024*1024), u.GetStorageBytes())

	u.SetAgentCount(10)
	assert.Equal(t, 10, u.GetAgentCount())

	u.SetAlertRuleCount(5)
	assert.Equal(t, 5, u.GetAlertRuleCount())

	u.AddAPICall()
	assert.Equal(t, 1, u.GetAPICallsPerMin())
}

func TestTenantUsage_Reset(t *testing.T) {
	u := NewTenantUsage(time.Minute)
	u.AddFlows(100)
	u.SetStorageBytes(1024)
	u.SetAgentCount(10)

	u.Reset()
	assert.Equal(t, int64(0), u.GetFlowsPerMin())
	assert.Equal(t, int64(0), u.GetStorageBytes())
	assert.Equal(t, 0, u.GetAgentCount())
}

func TestTenantUsage_Snapshot(t *testing.T) {
	u := NewTenantUsage(time.Minute)
	u.AddFlows(50)
	u.SetStorageBytes(1024)
	u.SetAgentCount(5)
	u.SetAlertRuleCount(3)
	u.SetUserCount(10)
	u.SetProjectCount(2)
	u.AddAPICall()

	snapshot := u.Snapshot()
	assert.Equal(t, int64(50), snapshot.FlowsPerMin)
	assert.Equal(t, int64(1024), snapshot.StorageBytes)
	assert.Equal(t, 5, snapshot.AgentCount)
	assert.Equal(t, 3, snapshot.AlertRuleCount)
	assert.Equal(t, 10, snapshot.UserCount)
	assert.Equal(t, 2, snapshot.ProjectCount)
	assert.Equal(t, 1, snapshot.APICallsPerMin)
	assert.Greater(t, snapshot.Timestamp, int64(0))
}

// ============================================================================
// 六、Context 集成测试
// ============================================================================

func TestContextIsolation_MultipleTenants(t *testing.T) {
	ctx1 := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-1",
	})
	ctx2 := sharedTenant.NewContext(context.Background(), &sharedTenant.TenantContext{
		TenantID: "tenant-2",
	})

	assert.Equal(t, "tenant-1", GetTenantID(ctx1))
	assert.Equal(t, "tenant-2", GetTenantID(ctx2))
}
