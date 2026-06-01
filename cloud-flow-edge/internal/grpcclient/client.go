// Package grpcclient 提供与中心服务通信的 gRPC 客户端
// 支持 TLS/mTLS、API Key 认证、自动重连、指数退避
package grpcclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"cloud-flow-edge/internal/config"
	"cloud-flow-edge/pkg/logger"
	"cloud-flow/pkg/grpcutil"
	edge "cloud-flow/proto"
)

const (
	reconnectBaseDelay = 1 * time.Second
	reconnectMaxDelay  = 30 * time.Second
	rpcTimeout         = 10 * time.Second
)

// Client 中心服务 gRPC 客户端
type Client struct {
	mu     sync.Mutex
	conn   *grpc.ClientConn
	client edge.CenterServiceClient
	logger *logger.Logger
	addr   string
	opts   []grpc.DialOption
	apiKey string
	stopCh chan struct{}
	stopped sync.Once
	watchCtx    context.Context
	watchCancel context.CancelFunc
}

// NewClient 创建并连接中心服务客户端
func NewClient(addr string, tlsCfg config.TLSConfig, apiKey string, log *logger.Logger) (*Client, error) {
	c := &Client{
		logger: log,
		addr:   addr,
		apiKey: apiKey,
		stopCh: make(chan struct{}),
	}
	c.watchCtx, c.watchCancel = context.WithCancel(context.Background())

	// 构建连接选项
	if tlsCfg.Enabled {
		creds, err := buildClientTLS(tlsCfg, addr)
		if err != nil {
			return nil, fmt.Errorf("构建 TLS 凭证失败: %w", err)
		}
		c.opts = append(c.opts, grpc.WithTransportCredentials(creds))
		log.Infof("中心服务连接启用 TLS, serverName=%s", tlsCfg.ServerName)
	} else {
		c.opts = append(c.opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		log.Warn("中心服务连接未启用 TLS，将使用明文传输")
	}

	conn, err := c.connect()
	if err != nil {
		return nil, err
	}

	c.conn = conn
	c.client = edge.NewCenterServiceClient(conn)

	// 启动后台连接监控
	go c.watchConnection()

	log.Infof("已连接中心服务: %s", addr)
	return c, nil
}

// connect 执行连接（不持有锁）
func (c *Client) connect() (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	dialOpts := append([]grpc.DialOption(nil), c.opts...)
	// 移除 grpc.WithBlock()，使用非阻塞连接

	conn, err := grpc.DialContext(ctx, c.addr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("连接中心服务失败 [%s]: %w", c.addr, err)
	}

	// 等待连接状态变为 READY
	// 注意：如果连接在 DialContext 返回后已经处于非 Connecting 状态，
	// WaitForStateChange 会立即返回 false，因此需要先检查当前状态。
	currentState := conn.GetState()
	if currentState == connectivity.Ready {
		return conn, nil
	}
	if currentState != connectivity.Connecting {
		_ = conn.Close()
		return nil, fmt.Errorf("连接中心服务失败，状态: %v", currentState)
	}
	if !conn.WaitForStateChange(ctx, connectivity.Connecting) {
		_ = conn.Close()
		return nil, fmt.Errorf("连接中心服务超时 [%s]", c.addr)
	}

	if conn.GetState() != connectivity.Ready {
		_ = conn.Close()
		return nil, fmt.Errorf("连接中心服务失败，状态: %v", conn.GetState())
	}

	return conn, nil
}

// reconnect 执行重连（不持有锁的情况下调用）
func (c *Client) reconnect() error {
	// 先进行连接操作，不持有锁
	newConn, err := c.connect()
	if err != nil {
		return err
	}

	// 只在更新连接时持有锁
	c.mu.Lock()
	defer c.mu.Unlock()

	// 关闭旧连接
	if c.conn != nil {
		_ = c.conn.Close()
	}

	// 更新连接和客户端
	c.conn = newConn
	c.client = edge.NewCenterServiceClient(newConn)
	return nil
}

// watchConnection 监控连接状态，断线后自动重连
func (c *Client) watchConnection() {
	for {
		select {
		case <-c.stopCh:
			return
		case <-c.watchCtx.Done():
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		// 在持有读锁期间获取连接状态，避免 TOCTOU 竞态：
		// 如果在 RUnlock 后 conn 被关闭，GetState() 可能返回不一致的状态。
		// 在锁内获取状态确保 conn 在获取状态时不会被关闭。
		state := connectivity.Idle
		if conn != nil {
			state = conn.GetState()
		}
		c.mu.RUnlock()

		if conn == nil {
			c.reconnectWithBackoff()
			continue
		}

		// 使用局部变量 currentConn 保存连接引用，避免在使用过程中被其他 goroutine 修改
		currentConn := conn
		if state == connectivity.Shutdown || state == connectivity.TransientFailure {
			c.logger.Warnf("检测到中心服务连接断开 (state=%v)，开始重连...", state)
			c.reconnectWithBackoff()
			continue
		}

		// 使用 watchCtx 替代 goroutine 监听 stopCh，避免 goroutine 泄漏
		select {
		case <-c.watchCtx.Done():
			return
		case <-time.After(5 * time.Second):
			// 超时后重新检查连接状态
		}

		if !currentConn.WaitForStateChange(c.watchCtx, state) {
			select {
			case <-c.stopCh:
				return
			case <-c.watchCtx.Done():
				return
			default:
			}
		}
	}
}

// reconnectWithBackoff 指数退避重连（无限重试）
func (c *Client) reconnectWithBackoff() {
	delay := reconnectBaseDelay
	attempt := 0

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		attempt++
		c.logger.Warnf("尝试重连中心服务 (第 %d 次)，等待 %s...", attempt, delay)

		select {
		case <-c.stopCh:
			return
		case <-time.After(delay):
		}

		if err := c.reconnect(); err != nil {
			c.logger.Errorf("重连中心服务失败 (第 %d 次): %v", attempt, err)
			delay *= 2
			if delay > reconnectMaxDelay {
				delay = reconnectMaxDelay
			}
			continue
		}

		c.logger.Infof("重连中心服务成功: %s", c.addr)
		return
	}
}

// buildClientTLS 构建客户端 TLS 凭证
func buildClientTLS(cfg config.TLSConfig, addr string) (credentials.TransportCredentials, error) {
	// 如果 ServerName 未配置，使用连接地址作为 ServerName
	serverName := cfg.ServerName
	if serverName == "" {
		// 从地址中提取 host（移除端口）
		if host, _, err := splitHostPort(addr); err == nil {
			serverName = host
		} else {
			serverName = addr
		}
		// 添加警告日志说明这可能导致 TLS 握手失败（如果使用 IP 地址而非域名）
		// NOTE: 此处使用 log.Printf 而非结构化日志，因为此函数在客户端初始化阶段调用，
		// 此时 logger 可能尚未完全配置。在后续版本中应将 logger 注入此函数。
		log.Printf("[WARNING] TLS ServerName 未配置，使用连接地址 %s 作为 ServerName。如果使用自签名证书，请确保证书包含此地址作为 SAN\n", serverName)
	}

	tlsConfig := &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: false, // 不跳过验证，确保安全
	}

	if cfg.CACert != "" {
		caPEM, err := os.ReadFile(cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("读取 CA 证书失败: %w", err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("解析 CA 证书失败")
		}
		tlsConfig.RootCAs = certPool
	}

	if cfg.ClientCert != "" && cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("加载客户端证书失败: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// splitHostPort 分离地址中的 host 和 port
func splitHostPort(addr string) (string, error) {
	host, _, err := net.SplitHostPort(addr)
	return host, err
}

// Close 关闭连接
func (c *Client) Close() error {
	var err error
	c.stopped.Do(func() {
		close(c.stopCh)
		c.watchCancel()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.conn != nil {
			c.logger.Info("关闭中心服务连接")
			err = c.conn.Close()
		}
	})
	return err
}



// ReportProbes 上报探针列表到中心服务
func (c *Client) ReportProbes(edgeNodeID, platform, region string, probes []*edge.ProbeInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	req := &edge.ReportProbesRequest{
		EdgeNodeId:    edgeNodeID,
		CloudPlatform: platform,
		Region:        region,
		Probes:        probes,
	}

	resp, err := c.client.ReportProbes(grpcutil.WithAuth(ctx, c.apiKey), req)
	if err != nil {
		return fmt.Errorf("上报探针列表失败: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("中心服务拒绝上报探针列表")
	}
	return nil
}

// ForwardMetrics 转发指标数据到中心服务
func (c *Client) ForwardMetrics(batch *edge.MetricsBatch) error {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	resp, err := c.client.ForwardMetrics(grpcutil.WithAuth(ctx, c.apiKey), batch)
	if err != nil {
		return fmt.Errorf("转发指标数据失败: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("中心服务拒绝指标数据")
	}
	return nil
}

// ForwardTraces 转发链路追踪数据到中心服务
func (c *Client) ForwardTraces(batch *edge.TraceBatch) error {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	resp, err := c.client.ForwardTraces(grpcutil.WithAuth(ctx, c.apiKey), batch)
	if err != nil {
		return fmt.Errorf("转发链路追踪数据失败: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("中心服务拒绝链路追踪数据")
	}
	return nil
}

// ForwardProfiling 转发性能分析数据到中心服务
func (c *Client) ForwardProfiling(batch *edge.ProfilingBatch) error {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	resp, err := c.client.ForwardProfiling(grpcutil.WithAuth(ctx, c.apiKey), batch)
	if err != nil {
		return fmt.Errorf("转发性能分析数据失败: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("中心服务拒绝性能分析数据")
	}
	return nil
}

// SendHeartbeat 发送边缘节点心跳到中心服务
// H3 修复: 返回 Center 下发的心跳间隔，允许动态调整
// P1 修复: 携带 Edge 地址，供 Center 维护 Edge 注册表
func (c *Client) SendHeartbeat(edgeNodeID, platform, region string, probeCount int32) (heartbeatInterval int32, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	req := &edge.EdgeHeartbeatRequest{
		EdgeNodeId:    edgeNodeID,
		EdgeAddress:   c.addr, // P1: 携带 Edge 自身地址，供 Center 注册
		CloudPlatform: platform,
		Region:        region,
		Timestamp:     time.Now().Unix(),
		ProbeCount:    probeCount,
	}

	resp, err := c.client.Heartbeat(grpcutil.WithAuth(ctx, c.apiKey), req)
	if err != nil {
		return 0, fmt.Errorf("发送心跳失败: %w", err)
	}
	if !resp.Success {
		return 0, fmt.Errorf("中心服务拒绝心跳")
	}
	// H3: 使用 Center 返回的心跳间隔，默认 30 秒
	if resp.HeartbeatInterval > 0 {
		return resp.HeartbeatInterval, nil
	}
	return 30, nil
}

// GetEdgeStatus 查询 Edge 节点状态列表（P1: Center Edge 注册表）
func (c *Client) GetEdgeStatus(edgeNodeID string) (*edge.GetEdgeStatusResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	req := &edge.GetEdgeStatusRequest{EdgeNodeId: edgeNodeID}
	return c.client.GetEdgeStatus(grpcutil.WithAuth(ctx, c.apiKey), req)
}
