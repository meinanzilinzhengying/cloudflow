// Package isolation 租户隔离测试
package isolation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/meinanzilinzhengying/cloudflow/services/shared/tenant"
)

func TestIsolationLevel_String(t *testing.T) {
	assert.Equal(t, "strict", IsolationStrict.String())
	assert.Equal(t, "platform", IsolationPlatform.String())
	assert.Equal(t, "unknown(99)", IsolationLevel(99).String())
}

func TestIsolationLevel_Values(t *testing.T) {
	assert.Equal(t, IsolationLevel(1), IsolationStrict)
	assert.Equal(t, IsolationLevel(2), IsolationPlatform)
}

func TestNewIsolationGuard(t *testing.T) {
	g := NewIsolationGuard(IsolationStrict)
	assert.NotNil(t, g)
	assert.Equal(t, IsolationStrict, g.Level())

	g2 := NewIsolationGuard(IsolationPlatform)
	assert.Equal(t, IsolationPlatform, g2.Level())
}

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

func TestBuildNamespaceFilter_NoNamespaces(t *testing.T) {
	result := BuildNamespaceFilter("tenant-1", nil)
	assert.Equal(t, "tenant_id = 'tenant-1'", result)
}

func TestBuildNamespaceFilter_WithNamespaces(t *testing.T) {
	result := BuildNamespaceFilter("tenant-1", []string{"ns1", "ns2"})
	assert.Equal(t, "tenant_id = 'tenant-1' AND namespace IN ('ns1','ns2')", result)
}

func TestBuildNamespaceFilter_EmptyTenant(t *testing.T) {
	result := BuildNamespaceFilter("", []string{"ns1"})
	assert.Equal(t, "tenant_id = '' AND namespace IN ('ns1')", result)
}

func TestValidateNamespaceOwnership_Valid(t *testing.T) {
	assert.NoError(t, ValidateNamespaceOwnership("tenant-1", "tenant-1-app"))
	assert.NoError(t, ValidateNamespaceOwnership("tenant-1", "shared-public"))
}

func TestValidateNamespaceOwnership_Invalid(t *testing.T) {
	err := ValidateNamespaceOwnership("tenant-1", "tenant-2-app")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "namespace ownership violation")
}

func TestValidateNamespaceOwnership_EmptyTenant(t *testing.T) {
	err := ValidateNamespaceOwnership("", "ns1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_id is empty")
}

func TestValidateNamespaceOwnership_EmptyNamespace(t *testing.T) {
	err := ValidateNamespaceOwnership("tenant-1", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is empty")
}

func TestEnforceQueryFilter_NoTenantContext(t *testing.T) {
	g := NewIsolationGuard(IsolationStrict)
	ctx := context.Background()
	_, err := g.EnforceQueryFilter(ctx, "SELECT * FROM flows")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tenant_id in context")
}

func TestEnforceQueryFilter_WithTenantContext(t *testing.T) {
	g := NewIsolationGuard(IsolationStrict)
	tc := &tenant.TenantContext{TenantID: "tenant-1"}
	ctx := tenant.NewContext(context.Background(), tc)

	result, err := g.EnforceQueryFilter(ctx, "SELECT * FROM flows WHERE 1=1")
	assert.NoError(t, err)
	assert.Contains(t, result, "tenant_id")
	assert.Contains(t, result, "tenant-1")
}

func TestEnforceQueryFilter_NoWhereClause(t *testing.T) {
	g := NewIsolationGuard(IsolationStrict)
	tc := &tenant.TenantContext{TenantID: "tenant-1"}
	ctx := tenant.NewContext(context.Background(), tc)

	result, err := g.EnforceQueryFilter(ctx, "SELECT * FROM flows")
	assert.NoError(t, err)
	assert.Contains(t, result, "WHERE")
	assert.Contains(t, result, "tenant_id = 'tenant-1'")
}

func TestEnforceClickHouseQuery(t *testing.T) {
	g := NewIsolationGuard(IsolationStrict)
	tc := &tenant.TenantContext{TenantID: "tenant-1"}
	ctx := tenant.NewContext(context.Background(), tc)

	result, err := g.EnforceClickHouseQuery(ctx, "SELECT * FROM flows")
	assert.NoError(t, err)
	assert.Contains(t, result, "tenant_id")
}

func TestEnforceTiDBQuery(t *testing.T) {
	g := NewIsolationGuard(IsolationStrict)
	tc := &tenant.TenantContext{TenantID: "tenant-1"}
	ctx := tenant.NewContext(context.Background(), tc)

	result, err := g.EnforceTiDBQuery(ctx, "SELECT * FROM flows")
	assert.NoError(t, err)
	assert.Contains(t, result, "tenant_id")
}

func TestQueryFilter_EmptyTenantID(t *testing.T) {
	filter := &QueryFilter{TenantID: ""}
	assert.Equal(t, "", filter.TenantID)
}

func TestQueryFilter_SpecialCharacters(t *testing.T) {
	filter := &QueryFilter{TenantID: "tenant-123_abc.456"}
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
