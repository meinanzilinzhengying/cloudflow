// Package grpcclient 提供与边缘节点通信的 gRPC 客户端
// 支持负载均衡、自动故障转移和健康检查
package grpcclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"cloud-flow-agent/pkg/logger"
	"cloud-flow/pkg/grpcutil"
	edge "cloud-flow/proto"
)

const (
	// 默认 RPC 超时
	defaultRPCTimeout = 10 * time.Second
	// 默认健康检查间隔
	defaultHealthCheckInterval = 15 * time.Second
	// 连接切换冷却时间
	switchCooldown = 5 * time.Second
	// 最大重试次数
	maxRetries = 3
)

// ClusterDiscovery 集群服务发现接口（从 cloud-flow-edge 复制以避免循环依赖）
type ClusterDiscovery interface {
	GetInstances() []EdgeInstance
	Watch(callback func(instances []EdgeInstance))
}

// EdgeInstance Edge 集群实例（从 cloud-flow-edge 复制）
type EdgeInstance struct {
	ID              string
	Address         string
	Port            int
	Weight          int
	Healthy         bool
	LastHeartbeat   time.Time
	Tags            map[string]string
	ConnectionCount int
}

// FullAddress 返回完整的地址
func (e *EdgeInstance) FullAddress() string {
	return fmt.Sprintf("%s:%d", e.Address, e.Port)
}

// LBClient 负载均衡 gRPC 客户端
type LBClient struct {
	discovery   ClusterDiscovery    // 服务发现组件
	currentConn *grpc.ClientConn    // 当前连接
	currentAddr string              // 当前连接地址
	currentID   string              // 当前实例ID
	mu          sync.RWMutex        // 连接锁

	apiKey      string              // API Key
	tlsCfg      TLSConfig           // TLS 配置
	logger      *logger.Logger      // 日志器

	stopCh      chan struct{}       // 停止信号
	stopped     sync.Once           // 确保只停止一次

	// 负载均衡状态
	instanceStats map[string]*instanceStats // 实例统计信息
	statsMu       sync.RWMutex
	lastSwitch    time.Time                 // 上次切换时间

	// 连接状态
	healthy     atomic.Bool  // 健康状态
	reconnectCh chan struct{} // 重连信号
}

// instanceStats 实例统计信息
type instanceStats struct {
	id              string
	address         string
	connectionCount int64
	requestCount    int64
	errorCount      int64
	lastUsed        time.Time
	healthy         bool
}

// NewLBClient 创建负载均衡 gRPC 客户端
func NewLBClient(discovery ClusterDiscovery, apiKey string, tlsCfg TLSConfig, log *logger.Logger) (*LBClient, error) {
	lb := &LBClient{
		discovery:     discovery,
		apiKey:        apiKey,
		tlsCfg:        tlsCfg,
		logger:        log,
		stopCh:        make(chan struct{}),
		instanceStats: make(map[string]*instanceStats),
		reconnectCh:   make(chan struct{}, 1),
		lastSwitch:    time.Now(),
	}
	lb.healthy.Store(true)

	// 获取初始实例列表
	instances := discovery.GetInstances()
	if len(instances) == 0 {
		return nil, fmt.Errorf("没有可用的 Edge 实例")
	}

	// 初始化实例统计
	for _, inst := range instances {
		lb.instanceStats[inst.ID] = &instanceStats{
			id:      inst.ID,
			address: inst.FullAddress(),
			healthy: inst.Healthy,
		}
	}

	// 连接到第一个可用实例
	if err := lb.connectToInstance(&instances[0]); err != nil {
		return nil, fmt.Errorf("连接初始实例失败: %w", err)
	}

	// 启动后台任务
	go lb.watchInstances()
	go lb.healthCheckLoop()
	go lb.reconnectLoop()

	log.Infof("负载均衡客户端已创建，当前连接: %s", lb.currentAddr)
	return lb, nil
}

// GetClient 获取当前 gRPC 客户端
// 如果连接不健康，会尝试切换到其他实例
func (lb *LBClient) GetClient() edge.ProbeServiceClient {
	lb.mu.RLock()
	conn := lb.currentConn
	addr := lb.currentAddr
	lb.mu.RUnlock()

	// 检查连接状态
	if conn == nil || conn.GetState() != connectivity.Ready {
		lb.logger.Warnf("当前连接 %s 不健康，尝试切换实例", addr)
		if err := lb.SwitchInstance(false); err != nil {
			lb.logger.Errorf("切换实例失败: %v", err)
		}
	}

	lb.mu.RLock()
	defer lb.mu.RUnlock()
	if lb.currentConn == nil {
		return nil
	}
	return edge.NewProbeServiceClient(lb.currentConn)
}

// GetCurrentAddr 获取当前连接的地址
func (lb *LBClient) GetCurrentAddr() string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.currentAddr
}

// SwitchInstance 切换到新的 Edge 实例
// force: 是否强制切换（忽略冷却时间）
func (lb *LBClient) SwitchInstance(force bool) error {
	// 检查冷却时间
	if !force {
		lb.mu.RLock()
		timeSinceLastSwitch := time.Since(lb.lastSwitch)
		lb.mu.RUnlock()

		if timeSinceLastSwitch < switchCooldown {
			return fmt.Errorf("切换过于频繁，请等待 %v", switchCooldown-timeSinceLastSwitch)
		}
	}

	// 获取可用实例
	instances := lb.discovery.GetInstances()
	if len(instances) == 0 {
		return fmt.Errorf("没有可用的 Edge 实例")
	}

	// 选择最佳实例（优先最少连接，其次轮询）
	selected := lb.selectBestInstance(instances)
	if selected == nil {
		return fmt.Errorf("无法选择合适的实例")
	}

	// 如果选中的是当前实例，不需要切换
	lb.mu.RLock()
	currentID := lb.currentID
	lb.mu.RUnlock()

	if selected.ID == currentID {
		return nil
	}

	// 执行切换
	return lb.switchToInstance(selected)
}

// selectBestInstance 选择最佳实例
// 策略：优先最少连接数，其次健康状态
func (lb *LBClient) selectBestInstance(instances []EdgeInstance) *EdgeInstance {
	lb.statsMu.RLock()
	defer lb.statsMu.RUnlock()

	var best *EdgeInstance
	minConnections := int64(-1)

	for i := range instances {
		inst := &instances[i]
		if !inst.Healthy {
			continue
		}

		stats, ok := lb.instanceStats[inst.ID]
		if !ok {
			// 新实例，优先选择
			return inst
		}

		if !stats.healthy {
			continue
		}

		if minConnections == -1 || stats.connectionCount < minConnections {
			minConnections = stats.connectionCount
			best = inst
		}
	}

	// 如果没有找到合适的，选择第一个健康的
	if best == nil {
		for i := range instances {
			if instances[i].Healthy {
				return &instances[i]
			}
		}
	}

	return best
}

// switchToInstance 切换到指定实例
func (lb *LBClient) switchToInstance(instance *EdgeInstance) error {
	lb.logger.Infof("正在切换到实例 %s (%s)", instance.ID, instance.FullAddress())

	// 建立新连接
	newConn, err := lb.createConnection(instance.FullAddress())
	if err != nil {
		lb.markInstanceUnhealthy(instance.ID)
		return fmt.Errorf("连接到新实例失败: %w", err)
	}

	// 原子替换连接
	lb.mu.Lock()
	oldConn := lb.currentConn
	oldAddr := lb.currentAddr
	oldID := lb.currentID

	lb.currentConn = newConn
	lb.currentAddr = instance.FullAddress()
	lb.currentID = instance.ID
	lb.lastSwitch = time.Now()
	lb.mu.Unlock()

	// 延迟关闭旧连接，避免中断正在进行的请求
	if oldConn != nil {
		go func(conn *grpc.ClientConn, addr string) {
			time.Sleep(5 * time.Second)
			conn.Close()
			lb.logger.Infof("旧连接 %s 已关闭", addr)
		}(oldConn, oldAddr)
	}

	// 更新统计信息
	lb.statsMu.Lock()
	if stats, ok := lb.instanceStats[oldID]; ok {
		stats.connectionCount = 0
	}
	if stats, ok := lb.instanceStats[instance.ID]; ok {
		stats.healthy = true
	} else {
		lb.instanceStats[instance.ID] = &instanceStats{
			id:      instance.ID,
			address: instance.FullAddress(),
			healthy: true,
		}
	}
	lb.statsMu.Unlock()

	lb.logger.Infof("已切换到实例 %s (%s)", instance.ID, instance.FullAddress())
	return nil
}

// connectToInstance 连接到指定实例
func (lb *LBClient) connectToInstance(instance *EdgeInstance) error {
	conn, err := lb.createConnection(instance.FullAddress())
	if err != nil {
		return err
	}

	lb.mu.Lock()
	lb.currentConn = conn
	lb.currentAddr = instance.FullAddress()
	lb.currentID = instance.ID
	lb.mu.Unlock()

	return nil
}

// createConnection 创建 gRPC 连接
func (lb *LBClient) createConnection(addr string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRPCTimeout)
	defer cancel()

	var opts []grpc.DialOption

	// 配置 TLS
	if lb.tlsCfg.Enabled {
		creds, err := lb.buildClientTLS()
		if err != nil {
			return nil, fmt.Errorf("构建 TLS 凭证失败: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("连接失败 [%s]: %w", addr, err)
	}

	// 等待连接就绪
	if conn.GetState() != connectivity.Ready {
		if !conn.WaitForStateChange(ctx, connectivity.Connecting) {
			conn.Close()
			return nil, fmt.Errorf("连接超时 [%s]", addr)
		}
		if conn.GetState() != connectivity.Ready {
			conn.Close()
			return nil, fmt.Errorf("连接失败 [%s]，状态: %v", addr, conn.GetState())
		}
	}

	return conn, nil
}

// buildClientTLS 构建客户端 TLS 凭证
func (lb *LBClient) buildClientTLS() (credentials.TransportCredentials, error) {
	tlsConfig := &tls.Config{
		ServerName: lb.tlsCfg.ServerName,
	}

	if lb.tlsCfg.CACert != "" {
		caPEM, err := os.ReadFile(lb.tlsCfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("读取 CA 证书失败: %w", err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("解析 CA 证书失败")
		}
		tlsConfig.RootCAs = certPool
	}

	if lb.tlsCfg.ClientCert != "" && lb.tlsCfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(lb.tlsCfg.ClientCert, lb.tlsCfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("加载客户端证书失败: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// watchInstances 监听实例变化
func (lb *LBClient) watchInstances() {
	lb.discovery.Watch(func(instances []EdgeInstance) {
		lb.logger.Infof("实例列表发生变化，当前 %d 个实例", len(instances))

		// 更新实例统计信息
		lb.statsMu.Lock()
		newStats := make(map[string]*instanceStats)
		for _, inst := range instances {
			if oldStats, ok := lb.instanceStats[inst.ID]; ok {
				oldStats.address = inst.FullAddress()
				oldStats.healthy = inst.Healthy
				newStats[inst.ID] = oldStats
			} else {
				newStats[inst.ID] = &instanceStats{
					id:      inst.ID,
					address: inst.FullAddress(),
					healthy: inst.Healthy,
				}
			}
		}
		lb.instanceStats = newStats
		lb.statsMu.Unlock()

		// 检查当前实例是否仍在列表中
		lb.mu.RLock()
		currentID := lb.currentID
		lb.mu.RUnlock()

		found := false
		for _, inst := range instances {
			if inst.ID == currentID {
				found = true
				break
			}
		}

		// 如果当前实例被移除，触发切换
		if !found {
			lb.logger.Warnf("当前实例 %s 已被移除，触发切换", currentID)
			select {
			case lb.reconnectCh <- struct{}{}:
			default:
			}
		}
	})
}

// healthCheckLoop 健康检查循环
func (lb *LBClient) healthCheckLoop() {
	ticker := time.NewTicker(defaultHealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lb.performHealthCheck()
		case <-lb.stopCh:
			return
		}
	}
}

// performHealthCheck 执行健康检查
func (lb *LBClient) performHealthCheck() {
	lb.mu.RLock()
	conn := lb.currentConn
	addr := lb.currentAddr
	lb.mu.RUnlock()

	if conn == nil {
		lb.healthy.Store(false)
		select {
		case lb.reconnectCh <- struct{}{}:
		default:
		}
		return
	}

	state := conn.GetState()
	if state != connectivity.Ready {
		lb.logger.Warnf("连接 %s 状态异常: %v", addr, state)
		lb.healthy.Store(false)
		select {
		case lb.reconnectCh <- struct{}{}:
		default:
		}
	} else {
		lb.healthy.Store(true)
	}
}

// reconnectLoop 重连循环
func (lb *LBClient) reconnectLoop() {
	for {
		select {
		case <-lb.reconnectCh:
			if err := lb.SwitchInstance(true); err != nil {
				lb.logger.Errorf("重连失败: %v", err)
			}
		case <-lb.stopCh:
			return
		}
	}
}

// markInstanceUnhealthy 标记实例为不健康
func (lb *LBClient) markInstanceUnhealthy(instanceID string) {
	lb.statsMu.Lock()
	defer lb.statsMu.Unlock()

	if stats, ok := lb.instanceStats[instanceID]; ok {
		stats.healthy = false
		lb.logger.Warnf("实例 %s 被标记为不健康", instanceID)
	}
}

// updateConnectionStats 更新连接统计
func (lb *LBClient) updateConnectionStats(instanceID string, delta int64) {
	lb.statsMu.Lock()
	defer lb.statsMu.Unlock()

	if stats, ok := lb.instanceStats[instanceID]; ok {
		stats.connectionCount += delta
		if stats.connectionCount < 0 {
			stats.connectionCount = 0
		}
		stats.lastUsed = time.Now()
	}
}

// Close 关闭客户端
func (lb *LBClient) Close() error {
	lb.stopped.Do(func() {
		close(lb.stopCh)

		lb.mu.Lock()
		if lb.currentConn != nil {
			lb.currentConn.Close()
		}
		lb.mu.Unlock()

		lb.logger.Info("负载均衡客户端已关闭")
	})
	return nil
}

// IsHealthy 返回客户端健康状态
func (lb *LBClient) IsHealthy() bool {
	return lb.healthy.Load()
}

// GetInstanceStats 获取实例统计信息
func (lb *LBClient) GetInstanceStats() map[string]map[string]interface{} {
	lb.statsMu.RLock()
	defer lb.statsMu.RUnlock()

	result := make(map[string]map[string]interface{})
	for id, stats := range lb.instanceStats {
		result[id] = map[string]interface{}{
			"address":          stats.address,
			"connection_count": stats.connectionCount,
			"request_count":    stats.requestCount,
			"error_count":      stats.errorCount,
			"last_used":        stats.lastUsed,
			"healthy":          stats.healthy,
		}
	}
	return result
}

// ExecuteWithRetry 带重试的执行函数
func (lb *LBClient) ExecuteWithRetry(ctx context.Context, operation func(ctx context.Context, client edge.ProbeServiceClient) error) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		client := lb.GetClient()
		if client == nil {
			return fmt.Errorf("无法获取 gRPC 客户端")
		}

		// 添加超时
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultRPCTimeout)
			defer cancel()
		}

		// 添加认证
		authCtx := grpcutil.WithAuth(ctx, lb.apiKey)

		if err := operation(authCtx, client); err != nil {
			lastErr = err
			lb.logger.Warnf("操作失败 (尝试 %d/%d): %v", i+1, maxRetries, err)

			// 如果是连接错误，尝试切换实例
			if isConnectionError(err) {
				if switchErr := lb.SwitchInstance(false); switchErr != nil {
					lb.logger.Errorf("切换实例失败: %v", switchErr)
				}
			}

			// 指数退避
			if i < maxRetries-1 {
				time.Sleep(time.Duration(i+1) * time.Second)
			}
			continue
		}

		return nil
	}

	return fmt.Errorf("操作失败，已重试 %d 次: %w", maxRetries, lastErr)
}

// isConnectionError 检查是否为连接错误
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// 检查常见的连接错误
	errStr := err.Error()
	connectionErrors := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"timeout",
		"deadline exceeded",
		"transport is closing",
		"Unavailable",
	}

	for _, ce := range connectionErrors {
		if contains(errStr, ce) {
			return true
		}
	}
	return false
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// RegisterProbe 注册探针（带重试）
func (lb *LBClient) RegisterProbe(ctx context.Context, probeID, hostIP, hostname, version string) (*edge.RegisterProbeResponse, error) {
	var resp *edge.RegisterProbeResponse
	err := lb.ExecuteWithRetry(ctx, func(ctx context.Context, client edge.ProbeServiceClient) error {
		var err error
		resp, err = client.RegisterProbe(ctx, &edge.RegisterProbeRequest{
			ProbeId:  probeID,
			HostIp:   hostIP,
			Hostname: hostname,
			Version:  version,
		})
		return err
	})
	return resp, err
}

// SendHeartbeat 发送心跳（带重试）
func (lb *LBClient) SendHeartbeat(ctx context.Context, probeID string) error {
	return lb.ExecuteWithRetry(ctx, func(ctx context.Context, client edge.ProbeServiceClient) error {
		resp, err := client.Heartbeat(ctx, &edge.HeartbeatRequest{
			ProbeId:   probeID,
			Timestamp: time.Now().Unix(),
		})
		if err != nil {
			return err
		}
		if !resp.GetSuccess() {
			return fmt.Errorf("心跳被拒绝")
		}
		return nil
	})
}

// SendMetrics 发送指标（带重试）
func (lb *LBClient) SendMetrics(ctx context.Context, batch *edge.MetricsBatch) error {
	return lb.ExecuteWithRetry(ctx, func(ctx context.Context, client edge.ProbeServiceClient) error {
		resp, err := client.SendMetrics(ctx, batch)
		if err != nil {
			return err
		}
		if !resp.GetSuccess() {
			return fmt.Errorf("指标数据被拒绝")
		}
		return nil
	})
}

// SendTraces 发送链路追踪（带重试）
func (lb *LBClient) SendTraces(ctx context.Context, batch *edge.TraceBatch) error {
	return lb.ExecuteWithRetry(ctx, func(ctx context.Context, client edge.ProbeServiceClient) error {
		resp, err := client.SendTraces(ctx, batch)
		if err != nil {
			return err
		}
		if !resp.GetSuccess() {
			return fmt.Errorf("链路追踪数据被拒绝")
		}
		return nil
	})
}

// SendProfiling 发送性能分析（带重试）
func (lb *LBClient) SendProfiling(ctx context.Context, batch *edge.ProfilingBatch) error {
	return lb.ExecuteWithRetry(ctx, func(ctx context.Context, client edge.ProbeServiceClient) error {
		resp, err := client.SendProfiling(ctx, batch)
		if err != nil {
			return err
		}
		if !resp.GetSuccess() {
			return fmt.Errorf("性能分析数据被拒绝")
		}
		return nil
	})
}
