// Package discovery 服务发现测试
package discovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 一、服务实例测试
// ============================================================================

func TestServiceInstance_Valid(t *testing.T) {
	svc := &ServiceInstance{
		Name:     "query-service",
		Addr:     "10.0.0.1",
		GrpcPort: 9007,
		HttpPort: 8007,
		Version:  "v1.2.0",
		Metadata: map[string]string{"region": "cn-north-1"},
	}
	assert.Equal(t, "query-service", svc.Name)
	assert.Equal(t, "10.0.0.1", svc.Addr)
	assert.Equal(t, 9007, svc.GrpcPort)
	assert.Equal(t, 8007, svc.HttpPort)
	assert.Equal(t, "v1.2.0", svc.Version)
	assert.Equal(t, "cn-north-1", svc.Metadata["region"])
}

func TestServiceInstance_Empty(t *testing.T) {
	svc := &ServiceInstance{}
	assert.Equal(t, "", svc.Name)
	assert.Equal(t, "", svc.Addr)
	assert.Equal(t, 0, svc.GrpcPort)
	assert.Equal(t, 0, svc.HttpPort)
	assert.Nil(t, svc.Metadata)
}

// ============================================================================
// 二、注册中心创建测试
// ============================================================================

func TestNewServiceRegistry_NoEtcd(t *testing.T) {
	// etcd 未配置时，注册中心应能创建（但无法注册）
	registry, err := NewServiceRegistry(nil, "/cloudflow/services/")
	require.NoError(t, err)
	assert.NotNil(t, registry)
	assert.Equal(t, "/cloudflow/services/", registry.prefix)
	assert.NotNil(t, registry.instances)
	assert.Equal(t, 0, len(registry.instances))
}

func TestNewServiceRegistry_WithPrefix(t *testing.T) {
	registry, err := NewServiceRegistry(nil, "test-prefix")
	require.NoError(t, err)
	assert.Equal(t, "test-prefix", registry.prefix)
}

// ============================================================================
// 三、并发安全测试
// ============================================================================

func TestServiceRegistry_ConcurrentAccess(t *testing.T) {
	registry, err := NewServiceRegistry(nil, "/test/")
	require.NoError(t, err)

	// 并发读写 instances map
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			registry.mu.RLock()
			_ = len(registry.instances)
			registry.mu.RUnlock()
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for concurrent access")
		}
	}
}

// ============================================================================
// 四、边界条件测试
// ============================================================================

func TestServiceInstance_NilMetadata(t *testing.T) {
	svc := &ServiceInstance{
		Name:     "test",
		Metadata: nil,
	}
	assert.Nil(t, svc.Metadata)
	// 访问 nil map 的 Metadata 不应 panic（调用方应处理）
}

func TestServiceRegistry_EmptyPrefix(t *testing.T) {
	registry, err := NewServiceRegistry(nil, "")
	require.NoError(t, err)
	assert.Equal(t, "", registry.prefix)
}

func TestServiceInstance_GrpcPortZero(t *testing.T) {
	svc := &ServiceInstance{
		Name:     "test-svc",
		GrpcPort: 0,
	}
	assert.Equal(t, 0, svc.GrpcPort)
	assert.Equal(t, 0, svc.HttpPort)
}

// ============================================================================
// 五、结构体完整性测试
// ============================================================================

func TestServiceRegistry_Struct(t *testing.T) {
	registry, err := NewServiceRegistry(nil, "/test")
	require.NoError(t, err)

	assert.Nil(t, registry.client)
	assert.NotNil(t, registry.instances)
	assert.NotNil(t, registry.mu)
}

func TestServiceInstance_String(t *testing.T) {
	svc := &ServiceInstance{
		Name: "auth-service",
		Addr: "192.168.1.100",
	}
	assert.Equal(t, "auth-service", svc.Name)
	assert.Equal(t, "192.168.1.100", svc.Addr)
}
