// Package servicediscovery 提供服务发现功能
// Nacos 服务发现适配器实现
package servicediscovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	// Nacos API 路径
	nacosInstanceListPath   = "/nacos/v1/ns/instance/list"
	nacosRegisterPath       = "/nacos/v1/ns/instance"
	nacosDeregisterPath     = "/nacos/v1/ns/instance"
	nacosHeartbeatPath      = "/nacos/v1/ns/instance/beat"
	// 默认缓存 TTL
	defaultCacheTTL = 10 * time.Second
	// 默认刷新间隔
	defaultRefreshInterval = 5 * time.Second
	// HTTP 超时
	defaultHTTPTimeout = 10 * time.Second
)

// NacosInstance Nacos 实例响应结构
type NacosInstance struct {
	InstanceId string            `json:"instanceId"`
	Ip         string            `json:"ip"`
	Port       int               `json:"port"`
	Weight     float64           `json:"weight"`
	Healthy    bool              `json:"healthy"`
	Enabled    bool              `json:"enabled"`
	Ephemeral  bool              `json:"ephemeral"`
	ClusterName string           `json:"clusterName"`
	ServiceName string           `json:"serviceName"`
	Metadata   map[string]string `json:"metadata"`
}

// NacosInstanceListResponse Nacos 实例列表响应
type NacosInstanceListResponse struct {
	Count     int             `json:"count"`
	Instances []NacosInstance `json:"hosts"`
}

// NacosDiscovery Nacos 服务发现实现
type NacosDiscovery struct {
	serverAddr     string           // Nacos 服务器地址
	namespace      string           // 命名空间
	group          string           // 服务分组
	serviceName    string           // 服务名称
	
	client         *http.Client     // HTTP 客户端
	cache          []EdgeInstance   // 实例缓存
	cacheMu        sync.RWMutex     // 缓存锁
	cacheExpiry    time.Time        // 缓存过期时间
	cacheTTL       time.Duration    // 缓存 TTL
	
	refreshIntv    time.Duration    // 刷新间隔
	stopCh         chan struct{}    // 停止信号
	stopped        sync.Once        // 确保只停止一次
	
	watchers       []func([]EdgeInstance) // 监听器列表
	watchersMu     sync.RWMutex           // 监听器锁
	
	logger         interface { // 简化日志接口
		Infof(format string, args ...interface{})
		Warnf(format string, args ...interface{})
		Errorf(format string, args ...interface{})
	}
}

// NacosOption 配置选项函数
type NacosOption func(*NacosDiscovery)

// WithNacosCacheTTL 设置缓存 TTL
func WithNacosCacheTTL(ttl time.Duration) NacosOption {
	return func(nd *NacosDiscovery) {
		nd.cacheTTL = ttl
	}
}

// WithNacosRefreshInterval 设置刷新间隔
func WithNacosRefreshInterval(interval time.Duration) NacosOption {
	return func(nd *NacosDiscovery) {
		nd.refreshIntv = interval
	}
}

// WithNacosLogger 设置日志器
func WithNacosLogger(logger interface {
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}) NacosOption {
	return func(nd *NacosDiscovery) {
		nd.logger = logger
	}
}

// NewNacosDiscovery 创建 Nacos 服务发现实例
func NewNacosDiscovery(serverAddr, namespace, group, serviceName string, opts ...NacosOption) *NacosDiscovery {
	nd := &NacosDiscovery{
		serverAddr:    serverAddr,
		namespace:     namespace,
		group:         group,
		serviceName:   serviceName,
		client:        &http.Client{Timeout: defaultHTTPTimeout},
		cache:         make([]EdgeInstance, 0),
		cacheTTL:      defaultCacheTTL,
		refreshIntv:   defaultRefreshInterval,
		stopCh:        make(chan struct{}),
		watchers:      make([]func([]EdgeInstance), 0),
		logger:        &noopLogger{},
	}

	// 应用配置选项
	for _, opt := range opts {
		opt(nd)
	}

	// 启动后台刷新循环
	go nd.refreshLoop()

	return nd
}

// GetInstances 获取所有实例（实现 ClusterDiscovery 接口）
func (nd *NacosDiscovery) GetInstances() []EdgeInstance {
	// 检查缓存是否过期
	nd.cacheMu.RLock()
	if time.Now().Before(nd.cacheExpiry) && len(nd.cache) > 0 {
		result := make([]EdgeInstance, len(nd.cache))
		copy(result, nd.cache)
		nd.cacheMu.RUnlock()
		return result
	}
	nd.cacheMu.RUnlock()

	// 缓存过期，从 Nacos 获取
	return nd.fetchInstances()
}

// fetchInstances 从 Nacos 获取实例列表
func (nd *NacosDiscovery) fetchInstances() []EdgeInstance {
	// 构建请求 URL
	params := url.Values{}
	params.Set("serviceName", nd.serviceName)
	if nd.group != "" {
		params.Set("groupName", nd.group)
	}
	if nd.namespace != "" {
		params.Set("namespaceId", nd.namespace)
	}

	reqURL := fmt.Sprintf("%s%s?%s", nd.serverAddr, nacosInstanceListPath, params.Encode())

	// 发送请求
	resp, err := nd.client.Get(reqURL)
	if err != nil {
		nd.logger.Errorf("从 Nacos 获取实例列表失败: %v", err)
		// 返回缓存数据（即使已过期）
		nd.cacheMu.RLock()
		result := make([]EdgeInstance, len(nd.cache))
		copy(result, nd.cache)
		nd.cacheMu.RUnlock()
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		nd.logger.Errorf("Nacos 返回错误状态码 %d: %s", resp.StatusCode, string(body))
		// 返回缓存数据
		nd.cacheMu.RLock()
		result := make([]EdgeInstance, len(nd.cache))
		copy(result, nd.cache)
		nd.cacheMu.RUnlock()
		return result
	}

	// 解析响应
	var nacosResp NacosInstanceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&nacosResp); err != nil {
		nd.logger.Errorf("解析 Nacos 响应失败: %v", err)
		// 返回缓存数据
		nd.cacheMu.RLock()
		result := make([]EdgeInstance, len(nd.cache))
		copy(result, nd.cache)
		nd.cacheMu.RUnlock()
		return result
	}

	// 转换为 EdgeInstance
	instances := make([]EdgeInstance, 0, len(nacosResp.Instances))
	for _, ni := range nacosResp.Instances {
		instance := EdgeInstance{
			ID:            ni.InstanceId,
			Address:       ni.Ip,
			Port:          ni.Port,
			Weight:        int(ni.Weight),
			Healthy:       ni.Healthy && ni.Enabled,
			LastHeartbeat: time.Now(),
			Tags:          ni.Metadata,
		}
		if instance.ID == "" {
			instance.ID = fmt.Sprintf("%s:%d", ni.Ip, ni.Port)
		}
		instances = append(instances, instance)
	}

	// 更新缓存
	nd.updateCache(instances)

	return instances
}

// updateCache 更新缓存
func (nd *NacosDiscovery) updateCache(instances []EdgeInstance) {
	nd.cacheMu.Lock()
	defer nd.cacheMu.Unlock()

	nd.cache = instances
	nd.cacheExpiry = time.Now().Add(nd.cacheTTL)
}

// Watch 监听实例变化（实现 ClusterDiscovery 接口）
func (nd *NacosDiscovery) Watch(callback func(instances []EdgeInstance)) {
	nd.watchersMu.Lock()
	defer nd.watchersMu.Unlock()
	nd.watchers = append(nd.watchers, callback)
}

// Register 注册实例（实现 ClusterDiscovery 接口）
func (nd *NacosDiscovery) Register(instance EdgeInstance) error {
	params := url.Values{}
	params.Set("serviceName", nd.serviceName)
	params.Set("ip", instance.Address)
	params.Set("port", strconv.Itoa(instance.Port))
	params.Set("weight", strconv.Itoa(instance.Weight))
	params.Set("healthy", "true")
	params.Set("enabled", "true")
	params.Set("ephemeral", "true")
	
	if nd.group != "" {
		params.Set("groupName", nd.group)
	}
	if nd.namespace != "" {
		params.Set("namespaceId", nd.namespace)
	}
	
	// 添加元数据
	if len(instance.Tags) > 0 {
		metadataJSON, _ := json.Marshal(instance.Tags)
		params.Set("metadata", string(metadataJSON))
	}

	reqURL := fmt.Sprintf("%s%s", nd.serverAddr, nacosRegisterPath)
	
	resp, err := nd.client.PostForm(reqURL, params)
	if err != nil {
		return fmt.Errorf("向 Nacos 注册实例失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 Nacos 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Nacos 注册失败，状态码 %d: %s", resp.StatusCode, string(body))
	}

	// Nacos 返回 "ok" 表示成功
	if string(body) != "ok" {
		return fmt.Errorf("Nacos 注册失败: %s", string(body))
	}

	nd.logger.Infof("实例 %s 注册到 Nacos 成功", instance.ID)
	return nil
}

// Deregister 注销实例（实现 ClusterDiscovery 接口）
func (nd *NacosDiscovery) Deregister(instanceID string) error {
	// 从缓存中查找实例信息
	nd.cacheMu.RLock()
	var targetInstance *EdgeInstance
	for i := range nd.cache {
		if nd.cache[i].ID == instanceID {
			targetInstance = &nd.cache[i]
			break
		}
	}
	nd.cacheMu.RUnlock()

	if targetInstance == nil {
		return fmt.Errorf("实例 %s 未找到", instanceID)
	}

	params := url.Values{}
	params.Set("serviceName", nd.serviceName)
	params.Set("ip", targetInstance.Address)
	params.Set("port", strconv.Itoa(targetInstance.Port))
	
	if nd.group != "" {
		params.Set("groupName", nd.group)
	}
	if nd.namespace != "" {
		params.Set("namespaceId", nd.namespace)
	}

	reqURL := fmt.Sprintf("%s%s?%s", nd.serverAddr, nacosDeregisterPath, params.Encode())
	
	req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("创建注销请求失败: %w", err)
	}

	resp, err := nd.client.Do(req)
	if err != nil {
		return fmt.Errorf("向 Nacos 注销实例失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 Nacos 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Nacos 注销失败，状态码 %d: %s", resp.StatusCode, string(body))
	}

	if string(body) != "ok" {
		return fmt.Errorf("Nacos 注销失败: %s", string(body))
	}

	nd.logger.Infof("实例 %s 从 Nacos 注销成功", instanceID)
	return nil
}

// UpdateHealth 更新实例健康状态（实现 ClusterDiscovery 接口）
func (nd *NacosDiscovery) UpdateHealth(instanceID string, healthy bool) {
	// 从缓存中查找实例
	nd.cacheMu.Lock()
	var targetInstance *EdgeInstance
	for i := range nd.cache {
		if nd.cache[i].ID == instanceID {
			targetInstance = &nd.cache[i]
			break
		}
	}
	
	if targetInstance != nil {
		targetInstance.Healthy = healthy
		targetInstance.LastHeartbeat = time.Now()
	}
	nd.cacheMu.Unlock()

	if targetInstance == nil {
		nd.logger.Warnf("更新健康状态失败，实例 %s 未找到", instanceID)
		return
	}

	// 发送心跳到 Nacos
	go nd.sendHeartbeat(targetInstance)
}

// sendHeartbeat 发送心跳
func (nd *NacosDiscovery) sendHeartbeat(instance *EdgeInstance) {
	// 构建心跳请求体
	heartbeatData := map[string]interface{}{
		"serviceName": nd.serviceName,
		"ip":          instance.Address,
		"port":        instance.Port,
	}
	
	if nd.group != "" {
		heartbeatData["serviceName"] = nd.group + "@@" + nd.serviceName
	}

	jsonData, err := json.Marshal(heartbeatData)
	if err != nil {
		nd.logger.Errorf("编码心跳数据失败: %v", err)
		return
	}

	params := url.Values{}
	params.Set("serviceName", nd.serviceName)
	if nd.group != "" {
		params.Set("groupName", nd.group)
	}
	if nd.namespace != "" {
		params.Set("namespaceId", nd.namespace)
	}
	params.Set("beat", string(jsonData))

	reqURL := fmt.Sprintf("%s%s?%s", nd.serverAddr, nacosHeartbeatPath, params.Encode())

	req, err := http.NewRequest(http.MethodPut, reqURL, bytes.NewBuffer(jsonData))
	if err != nil {
		nd.logger.Errorf("创建心跳请求失败: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := nd.client.Do(req)
	if err != nil {
		nd.logger.Errorf("发送心跳失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		nd.logger.Errorf("心跳响应错误，状态码 %d: %s", resp.StatusCode, string(body))
		return
	}

	nd.logger.Infof("实例 %s 心跳发送成功", instance.ID)
}

// refreshLoop 后台刷新循环
func (nd *NacosDiscovery) refreshLoop() {
	ticker := time.NewTicker(nd.refreshIntv)
	defer ticker.Stop()

	// 立即执行一次
	nd.refresh()

	for {
		select {
		case <-ticker.C:
			nd.refresh()
		case <-nd.stopCh:
			return
		}
	}
}

// refresh 刷新实例列表
func (nd *NacosDiscovery) refresh() {
	oldInstances := nd.GetInstances()
	newInstances := nd.fetchInstances()

	// 检查是否有变化
	if nd.hasChanged(oldInstances, newInstances) {
		nd.logger.Infof("实例列表发生变化，旧: %d, 新: %d", len(oldInstances), len(newInstances))
		
		// 通知监听器
		nd.watchersMu.RLock()
		watchers := make([]func([]EdgeInstance), len(nd.watchers))
		copy(watchers, nd.watchers)
		nd.watchersMu.RUnlock()

		for _, watcher := range watchers {
			go watcher(newInstances)
		}
	}
}

// hasChanged 检查实例列表是否发生变化
func (nd *NacosDiscovery) hasChanged(old, new []EdgeInstance) bool {
	if len(old) != len(new) {
		return true
	}

	oldMap := make(map[string]EdgeInstance)
	for _, inst := range old {
		oldMap[inst.ID] = inst
	}

	for _, inst := range new {
		oldInst, ok := oldMap[inst.ID]
		if !ok {
			return true
		}
		if oldInst.Healthy != inst.Healthy ||
			oldInst.Address != inst.Address ||
			oldInst.Port != inst.Port {
			return true
		}
	}

	return false
}

// Stop 停止 Nacos 服务发现
func (nd *NacosDiscovery) Stop() {
	nd.stopped.Do(func() {
		close(nd.stopCh)
		nd.logger.Info("Nacos 服务发现已停止")
	})
}

// GetServiceName 获取服务名称
func (nd *NacosDiscovery) GetServiceName() string {
	return nd.serviceName
}

// GetNamespace 获取命名空间
func (nd *NacosDiscovery) GetNamespace() string {
	return nd.namespace
}

// GetGroup 获取服务分组
func (nd *NacosDiscovery) GetGroup() string {
	return nd.group
}
