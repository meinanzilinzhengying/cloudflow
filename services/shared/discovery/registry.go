// Package discovery 微服务服务发现
//
// 基于 etcd 的服务发现实现:
//   - 服务注册
//   - 服务发现
//   - 健康检查
//   - 负载均衡 (客户端)
package discovery

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ============================================================================
// 服务注册信息
// ============================================================================

// ServiceInstance 服务实例
type ServiceInstance struct {
	Name     string
	Addr     string
	GrpcPort int
	HttpPort int
	Version  string
	Metadata map[string]string
}

// ServiceRegistry 服务注册中心
type ServiceRegistry struct {
	client    *clientv3.Client
	prefix    string
	instances map[string][]*ServiceInstance // serviceName -> instances
	mu        sync.RWMutex
}

// NewServiceRegistry 创建服务注册中心
func NewServiceRegistry(etcdEndpoints []string, prefix string) (*ServiceRegistry, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd connect failed: %w", err)
	}

	return &ServiceRegistry{
		client:    client,
		prefix:    prefix,
		instances: make(map[string][]*ServiceInstance),
	}, nil
}

// Register 注册服务
func (r *ServiceRegistry) Register(ctx context.Context, svc *ServiceInstance, ttl time.Duration) error {
	key := fmt.Sprintf("%s/%s/%s:%d", r.prefix, svc.Name, svc.Addr, svc.GrpcPort)
	value := fmt.Sprintf("%s:%d|%d|%s", svc.Addr, svc.GrpcPort, svc.HttpPort, svc.Version)

	// 带租约的注册 (自动过期)
	lease, err := r.client.Grant(ctx, int64(ttl.Seconds()))
	if err != nil {
		return fmt.Errorf("grant lease failed: %w", err)
	}

	_, err = r.client.Put(ctx, key, value, clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("register failed: %w", err)
	}

	// 更新本地缓存
	r.mu.Lock()
	r.instances[svc.Name] = append(r.instances[svc.Name], svc)
	r.mu.Unlock()

	return nil
}

// Deregister 注销服务
func (r *ServiceRegistry) Deregister(ctx context.Context, svc *ServiceInstance) error {
	key := fmt.Sprintf("%s/%s/%s:%d", r.prefix, svc.Name, svc.Addr, svc.GrpcPort)

	_, err := r.client.Delete(ctx, key)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	instances := r.instances[svc.Name]
	for i, inst := range instances {
		if inst.Addr == svc.Addr && inst.GrpcPort == svc.GrpcPort {
			r.instances[svc.Name] = append(instances[:i], instances[i+1:]...)
			break
		}
	}

	return nil
}

// Discover 发现服务实例
func (r *ServiceRegistry) Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error) {
	// 先查本地缓存
	r.mu.RLock()
	if instances, ok := r.instances[serviceName]; ok && len(instances) > 0 {
		r.mu.RUnlock()
		return instances, nil
	}
	r.mu.RUnlock()

	// 从 etcd 查询
	prefix := fmt.Sprintf("%s/%s/", r.prefix, serviceName)
	resp, err := r.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	var instances []*ServiceInstance
	for _, kv := range resp.Kvs {
		inst := parseInstance(kv.Value)
		if inst != nil {
			inst.Name = serviceName
			instances = append(instances, inst)
		}
	}

	r.mu.Lock()
	r.instances[serviceName] = instances
	r.mu.Unlock()

	return instances, nil
}

// ListServices 列出所有服务
func (r *ServiceRegistry) ListServices(ctx context.Context) (map[string][]*ServiceInstance, error) {
	resp, err := r.client.Get(ctx, r.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	services := make(map[string][]*ServiceInstance)
	for _, kv := range resp.Kvs {
		// key: prefix/serviceName/addr:port
		parts := strings.Split(string(kv.Key), "/")
		if len(parts) < 3 {
			continue
		}
		serviceName := parts[len(parts)-2]
		inst := parseInstance(kv.Value)
		if inst != nil {
			inst.Name = serviceName
			services[serviceName] = append(services[serviceName], inst)
		}
	}

	return services, nil
}

// Watch 服务变更监听
func (r *ServiceRegistry) Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error) {
	ch := make(chan []*ServiceInstance, 10)

	prefix := fmt.Sprintf("%s/%s/", r.prefix, serviceName)
	watcher := r.client.Watch(ctx, prefix, clientv3.WithPrefix())

	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case resp := <-watcher:
				if resp.Err() != nil {
					return
				}
				// 重新发现
				instances, err := r.Discover(ctx, serviceName)
				if err == nil {
					ch <- instances
				}
			}
		}
	}()

	return ch, nil
}

// Close 关闭
func (r *ServiceRegistry) Close() error {
	return r.client.Close()
}

// parseInstance 解析实例信息
func parseInstance(value string) *ServiceInstance {
	parts := strings.Split(value, "|")
	if len(parts) < 3 {
		return nil
	}

	var addr string
	var grpcPort, httpPort int
	fmt.Sscanf(parts[0], "%s:%d", &addr, &grpcPort)
	fmt.Sscanf(parts[1], "%d", &httpPort)

	version := ""
	if len(parts) >= 3 {
		version = parts[2]
	}

	return &ServiceInstance{
		Addr:     addr,
		GrpcPort: grpcPort,
		HttpPort: httpPort,
		Version:  version,
	}
}
