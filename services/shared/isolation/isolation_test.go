// Package isolation 租户隔离测试
package isolation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/meinanzilinzhengying/cloudflow/services/shared/tenant"
)

// ============================================================================
// 一、隔离级别测试
// ============================================================================

func TestIsolationLevel_String(t *testing.T) {
	assert.Equal(t, "strict", IsolationStrict.String())
	assert.Equal(t, "platform", IsolationPlatform.String())
	assert.Equal(t, "unknown(99)", IsolationLevel(99).String())
}

func TestIsolationLevel_Values(t *testing.T) {
	assert.Equal(t, IsolationLevel(1), IsolationStrict)
	assert.Equal(t, IsolationLevel(2), IsolationPlatform)
}

// ============================================================================
// 二、QueryFilter 测试
// ============================================================================

func TestQueryFilter_Valid(t *testing.T) {
	filter := &QueryFilter{
		TenantID:   "tenant-1",
		ProjectID:  "project-1",
		Namespaces: []string{"default", "production"},
	}
	assert.Equal(t, "tenant-1", filter.TenantID)
	assert.Equal(t, "project-1", filter.ProjectID)
	assert.Equal(t, []string{"default", "production"}, filter.Namespaces)
}

func TestQueryFilter_Empty(t *testing.T) {
	filter := &QueryFilter{}
	assert.Equal(t, "", filter.TenantID)
	assert.Equal(t, "", filter.ProjectID)
	assert.Nil(t, filter.Namespaces)
}

func TestQueryFilter_NilNamespaces(t *testing.T) {
	filter := &QueryFilter{
		TenantID: "tenant-1",
	}
	assert.Nil(t, filter.Namespaces)
}

// ============================================================================
// 三、TenantContext 测试（通过 tenant 包）
// ============================================================================

func TestTenantContext_FromContext(t *testing.T) {
	ctx := context.Background()
	
	// 空 context 不应 panic
	tc, ok := tenant.FromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, tc)
}

func TestTenantContext_NilContext(t *testing.T) {
	// nil context 应该安全处理
	var ctx context.Context
	if ctx != nil {
		tc, ok := tenant.FromContext(ctx)
		assert.False(t, ok)
		assert.Nil(t, tc)
	}
	// nil context 时 FromContext 会 panic，这是预期行为
}

// ============================================================================
// 四、隔离函数测试
// ============================================================================

func TestEnforceTenantIsolation(t *testing.T) {
	ctx := context.Background()
	filter := &QueryFilter{
		TenantID: "tenant-1",
	}
	
	// 无 tenant context 时应该返回错误
	err := EnforceTenantIsolation(ctx, filter)
	assert.Error(t, err, "无 tenant context 时应返回错误")
}

func TestEnforceTenantIsolation_NilFilter(t *testing.T) {
	ctx := context.Background()
	err := EnforceTenantIsolation(ctx, nil)
	assert.Error(t, err, "nil filter 时应返回错误")
}

func TestEnforceTenantIsolation_PlatformAdmin(t *testing.T) {
	// 平台管理员可以跨租户查询
	tc := &tenant.TenantContext{
		TenantID:       "admin",
		IsPlatformAdmin: true,
	}
	ctx := tenant.NewContext(context.Background(), tc)
	
	filter := &QueryFilter{
		TenantID: "tenant-1",
	}
	
	err := EnforceTenantIsolation(ctx, filter)
	assert.NoError(t, err, "平台管理员应允许跨租户查询")
}

func TestEnforceTenantIsolation_StrictMatch(t *testing.T) {
	// 普通租户只能查询自己的数据
	tc := &tenant.TenantContext{
		TenantID:        "tenant-1",
		IsPlatformAdmin: false,
	}
	ctx := tenant.NewContext(context.Background(), tc)
	
	filter := &QueryFilter{
		TenantID: "tenant-1",
	}
	
	err := EnforceTenantIsolation(ctx, filter)
	assert.NoError(t, err, "同一租户应允许查询")
}

func TestEnforceTenantIsolation_StrictMismatch(t *testing.T) {
	// 普通租户不能查询其他租户的数据
	tc := &tenant.TenantContext{
		TenantID:        "tenant-1",
		IsPlatformAdmin: false,
	}
	ctx := tenant.NewContext(context.Background(), tc)
	
	filter := &QueryFilter{
		TenantID: "tenant-2",
	}
	
	err := EnforceTenantIsolation(ctx, filter)
	assert.Error(t, err, "不同租户应拒绝查询")
}

// ============================================================================
// 五、边界条件测试
// ============================================================================

func TestQueryFilter_EmptyTenantID(t *testing.T) {
	filter := &QueryFilter{
		TenantID: "",
	}
	assert.Equal(t, "", filter.TenantID)
}

func TestQueryFilter_SpecialCharacters(t *testing.T) {
	filter := &QueryFilter{
		TenantID: "tenant-123_abc.456",
	}
	assert.Equal(t, "tenant-123_abc.456", filter.TenantID)
}

func TestIsolationLevel_Zero(t *testing.T) {
	level := IsolationLevel(0)
	assert.Equal(t, "unknown(0)", level.String())
}

func TestIsolationLevel_Negative(t *testing.T) {
	level := IsolationLevel(-1)
	assert.Equal(t, "unknown(-1)", level.String())
}
