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
	clientCfg      clientv3.Config // 保存配置用于重连
	serviceName    string
	refreshInterval time.Duration
	lastAddress    string
	addrMu         sync.RWMutex // 保护 lastAddress 的读写锁
	log            *logger.Logger
	stopCh         chan struct{}
	stopped        sync.Once
	updateCallback func(newAddr string)
	callbackMutex  sync.RWMutex // 保护 updateCallback 的读写锁
	
	// 重连机制相关字段
	consecutiveFailures int
	backoffDelay       time.Duration
	maxBackoffDelay    time.Duration
	reconnectMu        sync.Mutex // 保护重连过程
}

// NewEtcdDiscovery 创建etcd服务发现实例
func NewEtcdDiscovery(cfg config.ServiceDiscoveryConfig, log *logger.Logger) (*EtcdDiscovery, error) {
	// 保存配置用于重连
	clientCfg := clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: 5 * time.Second,
	}
	
	// 连接etcd
	client, err := clientv3.New(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("连接etcd失败: %w", err)
	}

	return &EtcdDiscovery{
		client:         client,
		clientCfg:      clientCfg,
		serviceName:    cfg.ServiceName,
		refreshInterval: time.Duration(cfg.RefreshInterval) * time.Second,
		log:            log,
		stopCh:         make(chan struct{}),
		backoffDelay:   1 * time.Second,  // 初始退避延迟
		maxBackoffDelay: 30 * time.Second, // 最大退避延迟
	}, nil
}

// reconnect 指数退避重连etcd
func (d *EtcdDiscovery) reconnect() error {
	d.reconnectMu.Lock()
	defer d.reconnectMu.Unlock()

	// 关闭旧连接
	if d.client != nil {
		d.client.Close()
	}

	// 指数退避: 1s → 2s → 4s → 8s → 16s → 30s
	d.consecutiveFailures++
	if d.consecutiveFailures > 1 {
		d.backoffDelay *= 2
		if d.backoffDelay > d.maxBackoffDelay {
			d.backoffDelay = d.maxBackoffDelay
		}
	}

	d.log.Errorf("etcd连接失败，第%d次重连，延迟: %v", d.consecutiveFailures, d.backoffDelay)
	
	// 等待退避时间
	select {
	case <-time.After(d.backoffDelay):
	case <-d.stopCh:
		return fmt.Errorf("服务停止")
	}

	// 尝试重连
	client, err := clientv3.New(d.clientCfg)
	if err != nil {
		d.log.Errorf("etcd重连失败: %v，下次重试延迟: %v", err, d.backoffDelay*2)
		return err
	}

	// 重连成功，重置退避计时器
	d.client = client
	d.consecutiveFailures = 0
	d.backoffDelay = 1 * time.Second
	d.log.Info("etcd重连成功")
	return nil
}

// healthCheck 健康检查心跳
func (d *EtcdDiscovery) healthCheck() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	
	_, err := d.client.Get(ctx, "/health")
	return err == nil
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

// refreshLoop 刷新服务地址（带指数退避重连机制）
func (d *EtcdDiscovery) refreshLoop() {
	ticker := time.NewTicker(d.refreshInterval)
	defer ticker.Stop()
	
	// 健康检查心跳: 每30s检查一次连接状态
	healthTicker := time.NewTicker(30 * time.Second)
	defer healthTicker.Stop()

	for {
		select {
		case <-ticker.C:
			address, err := d.GetServiceAddress(d.serviceName)
			if err != nil {
				d.log.Warnf("刷新服务地址失败: %v", err)
				// 连续失败3次触发重连
				if d.consecutiveFailures >= 3 {
					d.reconnect()
				}
			} else {
				// 成功，重置失败计数
				d.consecutiveFailures = 0
				d.backoffDelay = 1 * time.Second
				
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
		case <-healthTicker.C:
			// 定期健康检查
			if !d.healthCheck() {
				d.log.Warn("etcd健康检查失败，触发重连")
				d.reconnect()
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
