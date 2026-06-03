// Package servicediscovery 提供服务发现功能
package servicediscovery

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"go.etcd.io/etcd/client/v3"
	"github.com/hashicorp/consul/api"

	"github.com/meinanzilinzhengying/cloudflow/edge/internal/config"
	"github.com/meinanzilinzhengying/cloudflow/edge/pkg/logger"
)

// Discovery 服务发现接口
type Discovery interface {
	GetServiceAddress(serviceName string) (string, error)
	Start()
	Stop()
	SetUpdateCallback(callback func(newAddr string))
}

// NewDiscovery 创建服务发现实例
func NewDiscovery(cfg config.ServiceDiscoveryConfig, log *logger.Logger) (Discovery, error) {
	switch cfg.Type {
	case "etcd":
		return NewEtcdDiscovery(cfg, log)
	case "consul":
		return NewConsulDiscovery(cfg, log)
	case "dns":
		return NewDNSDiscovery(cfg, log)
	default:
		return nil, fmt.Errorf("不支持的服务发现类型: %s", cfg.Type)
	}
}

// EtcdDiscovery etcd服务发现
type EtcdDiscovery struct {
	client         *clientv3.Client
	serviceName    string
	refreshInterval time.Duration
	lastAddress    string
	addrMu         sync.RWMutex // 保护 lastAddress 的读写锁
	log            *logger.Logger
	stopCh         chan struct{}
	stopped        sync.Once
	updateCallback func(newAddr string)
	callbackMutex  sync.RWMutex // 保护 updateCallback 的读写锁
}

// NewEtcdDiscovery 创建etcd服务发现实例
func NewEtcdDiscovery(cfg config.ServiceDiscoveryConfig, log *logger.Logger) (*EtcdDiscovery, error) {
	// 连接etcd
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("连接etcd失败: %w", err)
	}

	return &EtcdDiscovery{
		client:         client,
		serviceName:    cfg.ServiceName,
		refreshInterval: time.Duration(cfg.RefreshInterval) * time.Second,
		log:            log,
		stopCh:         make(chan struct{}),
	}, nil
}

// GetServiceAddress 获取服务地址
func (d *EtcdDiscovery) GetServiceAddress(serviceName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 从etcd中获取服务地址
	resp, err := d.client.Get(ctx, fmt.Sprintf("/services/%s", serviceName))
	if err != nil {
		return "", fmt.Errorf("从etcd获取服务地址失败: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return "", fmt.Errorf("服务 %s 未注册", serviceName)
	}

	address := string(resp.Kvs[0].Value)
	d.addrMu.Lock()
	d.lastAddress = address
	d.addrMu.Unlock()
	return address, nil
}

// Start 启动服务发现
func (d *EtcdDiscovery) Start() {
	go d.refreshLoop()
	d.log.Info("etcd服务发现已启动")
}

// Stop 停止服务发现
func (d *EtcdDiscovery) Stop() {
	d.stopped.Do(func() {
		close(d.stopCh)
		d.client.Close()
		d.log.Info("etcd服务发现已停止")
	})
}

// SetUpdateCallback 设置地址更新回调
func (d *EtcdDiscovery) SetUpdateCallback(callback func(newAddr string)) {
	d.callbackMutex.Lock()
	defer d.callbackMutex.Unlock()
	d.updateCallback = callback
}

// refreshLoop 刷新服务地址
// TODO(AE-M09): 当 etcd 连接断开时，当前实现仅打印警告日志，不会尝试重连 etcd 客户端。
// 建议在连续失败达到阈值后重建 etcd client 连接，并在重连成功后恢复刷新循环。
func (d *EtcdDiscovery) refreshLoop() {
	ticker := time.NewTicker(d.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			address, err := d.GetServiceAddress(d.serviceName)
			if err != nil {
				d.log.Warnf("刷新服务地址失败: %v", err)
			} else {
				d.addrMu.RLock()
				lastAddr := d.lastAddress
				d.addrMu.RUnlock()
				if address != lastAddr {
					d.log.Infof("服务地址已更新: %s", address)
					d.addrMu.Lock()
					d.lastAddress = address
					d.addrMu.Unlock()
					d.callbackMutex.RLock()
					callback := d.updateCallback
					d.callbackMutex.RUnlock()
					if callback != nil {
						callback(address)
					}
				}
			}
		case <-d.stopCh:
			return
		}
	}
}

// ConsulDiscovery consul服务发现
type ConsulDiscovery struct {
	client         *api.Client
	serviceName    string
	refreshInterval time.Duration
	lastAddress    string
	addrMu         sync.RWMutex // 保护 lastAddress 的读写锁
	log            *logger.Logger
	stopCh         chan struct{}
	stopped        sync.Once
	updateCallback func(newAddr string)
	callbackMutex  sync.RWMutex // 保护 updateCallback 的读写锁
}

// NewConsulDiscovery 创建consul服务发现实例
func NewConsulDiscovery(cfg config.ServiceDiscoveryConfig, log *logger.Logger) (*ConsulDiscovery, error) {
	// 构建consul配置
	consulConfig := &api.Config{}
	if len(cfg.Endpoints) > 0 {
		consulConfig.Address = cfg.Endpoints[0]
	}

	// 连接consul
	client, err := api.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("连接consul失败: %w", err)
	}

	return &ConsulDiscovery{
		client:         client,
		serviceName:    cfg.ServiceName,
		refreshInterval: time.Duration(cfg.RefreshInterval) * time.Second,
		log:            log,
		stopCh:         make(chan struct{}),
	}, nil
}

// GetServiceAddress 获取服务地址
func (d *ConsulDiscovery) GetServiceAddress(serviceName string) (string, error) {
	// 从consul中获取服务（带超时）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	services, _, err := d.client.Catalog().Service(serviceName, "", &api.QueryOptions{WaitTime: 5 * time.Second, Context: ctx})
	if err != nil {
		return "", fmt.Errorf("从consul获取服务失败: %w", err)
	}

	if len(services) == 0 {
		return "", fmt.Errorf("服务 %s 未注册", serviceName)
	}

	// 选择第一个服务实例
	service := services[0]
	address := fmt.Sprintf("%s:%d", service.ServiceAddress, service.ServicePort)
	d.addrMu.Lock()
	d.lastAddress = address
	d.addrMu.Unlock()
	return address, nil
}

// Start 启动服务发现
func (d *ConsulDiscovery) Start() {
	go d.refreshLoop()
	d.log.Info("consul服务发现已启动")
}

// Stop 停止服务发现
func (d *ConsulDiscovery) Stop() {
	d.stopped.Do(func() {
		close(d.stopCh)
		if d.client != nil {
			d.client.Close()
		}
		d.log.Info("consul服务发现已停止")
	})
}

// SetUpdateCallback 设置地址更新回调
func (d *ConsulDiscovery) SetUpdateCallback(callback func(newAddr string)) {
	d.callbackMutex.Lock()
	defer d.callbackMutex.Unlock()
	d.updateCallback = callback
}

// refreshLoop 刷新服务地址
func (d *ConsulDiscovery) refreshLoop() {
	ticker := time.NewTicker(d.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			address, err := d.GetServiceAddress(d.serviceName)
			if err != nil {
				d.log.Warnf("刷新服务地址失败: %v", err)
			} else {
				d.addrMu.RLock()
				lastAddr := d.lastAddress
				d.addrMu.RUnlock()
				if address != lastAddr {
					d.log.Infof("服务地址已更新: %s", address)
					d.addrMu.Lock()
					d.lastAddress = address
					d.addrMu.Unlock()
					d.callbackMutex.RLock()
					callback := d.updateCallback
					d.callbackMutex.RUnlock()
					if callback != nil {
						callback(address)
					}
				}
			}
		case <-d.stopCh:
			return
		}
	}
}

// DNSDiscovery DNS服务发现
type DNSDiscovery struct {
	serviceName    string
	port           int
	refreshInterval time.Duration
	lastAddress    string
	addrMu         sync.RWMutex // 保护 lastAddress 的读写锁
	log            *logger.Logger
	stopCh         chan struct{}
	stopped        sync.Once
	updateCallback func(newAddr string)
	callbackMutex  sync.RWMutex // 保护 updateCallback 的读写锁
}

// NewDNSDiscovery 创建DNS服务发现实例
func NewDNSDiscovery(cfg config.ServiceDiscoveryConfig, log *logger.Logger) (*DNSDiscovery, error) {
	// 默认端口为9090
	port := cfg.Port
	if port == 0 {
		port = 9090
	}
	return &DNSDiscovery{
		serviceName:    cfg.ServiceName,
		port:           port,
		refreshInterval: time.Duration(cfg.RefreshInterval) * time.Second,
		log:            log,
		stopCh:         make(chan struct{}),
	}, nil
}

// GetServiceAddress 获取服务地址
func (d *DNSDiscovery) GetServiceAddress(serviceName string) (string, error) {
	// 尝试使用SRV记录查询
	_, srvRecords, err := net.LookupSRV("_grpc", "_tcp", serviceName)
	if err == nil && len(srvRecords) > 0 {
		// 选择第一个SRV记录
		srv := srvRecords[0]
		// 解析SRV记录指向的主机名
		addrs, err := net.LookupHost(srv.Target)
		if err == nil && len(addrs) > 0 {
			address := fmt.Sprintf("%s:%d", addrs[0], srv.Port)
			d.addrMu.Lock()
			d.lastAddress = address
			d.addrMu.Unlock()
			return address, nil
		}
	}

	// 尝试使用A记录查询
	addrs, err := net.LookupHost(serviceName)
	if err != nil {
		return "", fmt.Errorf("DNS查询失败: %w", err)
	}

	if len(addrs) == 0 {
		return "", fmt.Errorf("服务 %s 未注册", serviceName)
	}

	// 选择第一个IP地址，使用配置的端口
	address := fmt.Sprintf("%s:%d", addrs[0], d.port)
	d.addrMu.Lock()
	d.lastAddress = address
	d.addrMu.Unlock()
	return address, nil
}

// Start 启动服务发现
func (d *DNSDiscovery) Start() {
	go d.refreshLoop()
	d.log.Info("DNS服务发现已启动")
}

// Stop 停止服务发现
func (d *DNSDiscovery) Stop() {
	d.stopped.Do(func() {
		close(d.stopCh)
		d.log.Info("DNS服务发现已停止")
	})
}

// SetUpdateCallback 设置地址更新回调
func (d *DNSDiscovery) SetUpdateCallback(callback func(newAddr string)) {
	d.callbackMutex.Lock()
	defer d.callbackMutex.Unlock()
	d.updateCallback = callback
}

// refreshLoop 刷新服务地址
func (d *DNSDiscovery) refreshLoop() {
	ticker := time.NewTicker(d.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			address, err := d.GetServiceAddress(d.serviceName)
			if err != nil {
				d.log.Warnf("刷新服务地址失败: %v", err)
			} else {
				d.addrMu.RLock()
				lastAddr := d.lastAddress
				d.addrMu.RUnlock()
				if address != lastAddr {
					d.log.Infof("服务地址已更新: %s", address)
					d.addrMu.Lock()
					d.lastAddress = address
					d.addrMu.Unlock()
					d.callbackMutex.RLock()
					callback := d.updateCallback
					d.callbackMutex.RUnlock()
					if callback != nil {
						callback(address)
					}
				}
			}
		case <-d.stopCh:
			return
		}
	}
}
