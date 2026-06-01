// Package grpcclient 提供与边缘节点通信的 gRPC 客户端
// 支持 TLS/mTLS、API Key 认证、自动重连、指数退避
package grpcclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"sync"
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
	maxReconnectAttempts = 0 // 0 = 无限重试
	reconnectBaseDelay   = 1 * time.Second
	reconnectMaxDelay    = 30 * time.Second
	rpcTimeout           = 10 * time.Second
)

// TLSConfig TLS配置
type TLSConfig struct {
	Enabled    bool
	ServerName string
	CACert     string
	ClientCert string
	ClientKey  string
}

type Client struct {
	mu     sync.RWMutex
	conn   *grpc.ClientConn
	client edge.ProbeServiceClient
	logger *logger.Logger
	addr   string
	apiKey string
	tlsCfg TLSConfig
	localAddr string // 本地绑定地址
	stopCh chan struct{}
	stopped sync.Once
	watchCtx    context.Context
	watchCancel context.CancelFunc
}

// NewClient 创建gRPC客户端
// localAddr: 本地绑定地址，为空则不绑定特定地址
func NewClient(addr string, apiKey string, localAddr string, tlsCfg TLSConfig, log *logger.Logger) (*Client, error) {
	c := &Client{
		logger:    log,
		addr:      addr,
		apiKey:    apiKey,
		localAddr: localAddr,
		tlsCfg:    tlsCfg,
		stopCh:    make(chan struct{}),
	}
	c.watchCtx, c.watchCancel = context.WithCancel(context.Background())

	if err := c.connect(); err != nil {
		return nil, err
	}

	// 启动后台连接监控
	go c.watchConnection()

	log.Infof("已连接边缘节点: %s (本地地址: %s)", addr, localAddr)
	return c, nil
}

// connect 建立 gRPC 连接（非阻塞模式 + 等待 READY）
func (c *Client) connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	// 1. 构建连接选项（无锁）
	var opts []grpc.DialOption
	if c.tlsCfg.Enabled {
		creds, err := c.buildClientTLS()
		if err != nil {
			return fmt.Errorf("构建 TLS 凭证失败: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
		c.logger.Infof("边缘节点连接启用 TLS, serverName=%s", c.tlsCfg.ServerName)
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		c.logger.Warn("边缘节点连接未启用 TLS，将使用明文传输")
	}

	// 如果指定了本地地址，使用自定义Dialer绑定本地地址
	if c.localAddr != "" {
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := &net.Dialer{
				LocalAddr: &net.TCPAddr{IP: net.ParseIP(c.localAddr)},
				Timeout:   10 * time.Second,
			}
			return d.DialContext(ctx, "tcp", addr)
		}))
		c.logger.Infof("使用本地地址绑定: %s", c.localAddr)
	}

	// 移除 grpc.WithBlock()，使用非阻塞连接

	// 2. 建立连接（无锁）
	conn, err := grpc.DialContext(ctx, c.addr, opts...)
	if err != nil {
		return fmt.Errorf("连接边缘节点失败 [%s]: %w", c.addr, err)
	}

	// 3. 等待连接状态变为 READY（无锁，复用 ctx 超时）
	// 注意：如果连接在 DialContext 返回后已经处于非 Connecting 状态，
	// WaitForStateChange 会立即返回 false，因此需要先检查当前状态。
	currentState := conn.GetState()
	if currentState == connectivity.Ready {
		// 连接已经就绪，无需等待
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.conn = conn
		c.client = edge.NewProbeServiceClient(conn)
		c.mu.Unlock()
		return nil
	}
	if currentState != connectivity.Connecting {
		_ = conn.Close()
		return fmt.Errorf("连接边缘节点失败，状态: %v", currentState)
	}
	if !conn.WaitForStateChange(ctx, connectivity.Connecting) {
		_ = conn.Close()
		return fmt.Errorf("连接边缘节点超时 [%s]", c.addr)
	}

	if conn.GetState() != connectivity.Ready {
		_ = conn.Close()
		return fmt.Errorf("连接边缘节点失败，状态: %v", conn.GetState())
	}

	// 4. 关闭旧连接并更新新连接（加锁保护，保证原子性）
	c.mu.Lock()
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = conn
	c.client = edge.NewProbeServiceClient(conn)
	c.mu.Unlock()

	return nil
}

// buildClientTLS 构建客户端 TLS 凭证
func (c *Client) buildClientTLS() (credentials.TransportCredentials, error) {
	tlsConfig := &tls.Config{
		ServerName: c.tlsCfg.ServerName,
	}

	if c.tlsCfg.CACert != "" {
		caPEM, err := os.ReadFile(c.tlsCfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("读取 CA 证书失败: %w", err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("解析 CA 证书失败")
		}
		tlsConfig.RootCAs = certPool
	}

	if c.tlsCfg.ClientCert != "" && c.tlsCfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(c.tlsCfg.ClientCert, c.tlsCfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("加载客户端证书失败: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return credentials.NewTLS(tlsConfig), nil
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
		state := connectivity.Connecting
		if conn != nil {
			state = conn.GetState()
		}
		c.mu.RUnlock()

		if conn == nil {
			c.reconnectWithBackoff()
			continue
		}

		if state == connectivity.Shutdown || state == connectivity.TransientFailure {
			c.logger.Warnf("检测到连接断开 (state=%v)，开始重连...", state)
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

		// 等待状态变化
		c.mu.RLock()
		if !conn.WaitForStateChange(c.watchCtx, state) {
			c.mu.RUnlock()
			select {
			case <-c.stopCh:
				return
			case <-c.watchCtx.Done():
				return
			default:
			}
		} else {
			c.mu.RUnlock()
		}
	}
}

// reconnectWithBackoff 指数退避重连
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
		c.logger.Warnf("尝试重连边缘节点 (第 %d 次)，等待 %s...", attempt, delay)

		select {
		case <-c.stopCh:
			return
		case <-time.After(delay):
		}

		if err := c.connect(); err != nil {
			c.logger.Errorf("重连失败 (第 %d 次): %v", attempt, err)
			// 指数退避，最大 30s
			delay *= 2
			if delay > reconnectMaxDelay {
				delay = reconnectMaxDelay
			}
			continue
		}

		c.logger.Infof("重连成功: %s", c.addr)
		return
	}
}



func (c *Client) Register(ctx context.Context, probeID, hostIP, hostname, version string) (*edge.RegisterProbeResponse, error) {
	// 如果 context 已经有超时设置，使用它；否则添加默认超时
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
	}

	return c.client.RegisterProbe(grpcutil.WithAuth(ctx, c.apiKey), &edge.RegisterProbeRequest{
		ProbeId:  probeID,
		HostIp:   hostIP,
		Hostname: hostname,
		Version:  version,
	})
}

func (c *Client) Heartbeat(ctx context.Context, probeID string) error {
	// 如果 context 已经有超时设置，使用它；否则添加默认超时
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
	}

	resp, err := c.client.Heartbeat(grpcutil.WithAuth(ctx, c.apiKey), &edge.HeartbeatRequest{
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
}

func (c *Client) SendMetrics(ctx context.Context, batch *edge.MetricsBatch) error {
	// 如果 context 已经有超时设置，使用它；否则添加默认超时
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
	}

	resp, err := c.client.SendMetrics(grpcutil.WithAuth(ctx, c.apiKey), batch)
	if err != nil {
		return err
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("指标数据被拒绝")
	}
	return nil
}

func (c *Client) SendTraces(ctx context.Context, batch *edge.TraceBatch) error {
	// 如果 context 已经有超时设置，使用它；否则添加默认超时
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
	}

	resp, err := c.client.SendTraces(grpcutil.WithAuth(ctx, c.apiKey), batch)
	if err != nil {
		return err
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("链路追踪数据被拒绝")
	}
	return nil
}

func (c *Client) SendProfiling(ctx context.Context, batch *edge.ProfilingBatch) error {
	// 如果 context 已经有超时设置，使用它；否则添加默认超时
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
	}

	resp, err := c.client.SendProfiling(grpcutil.WithAuth(ctx, c.apiKey), batch)
	if err != nil {
		return err
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("性能分析数据被拒绝")
	}
	return nil
}

// GetConfig 获取远程配置
func (c *Client) GetConfig(ctx context.Context, req *edge.GetConfigRequest) (*edge.GetConfigResponse, error) {
	// 如果 context 已经有超时设置，使用它；否则添加默认超时
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
	}

	// 使用 ProbeService 的 GetConfig 方法（需要在proto中定义）
	// 这里暂时使用通用方法，后续需要服务端支持
	return nil, fmt.Errorf("GetConfig 需要服务端支持")
}

func (c *Client) Close() error {
	var err error
	c.stopped.Do(func() {
		close(c.stopCh)
		c.watchCancel()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.conn != nil {
			err = c.conn.Close()
		}
	})
	return err
}

// GetState 获取连接状态
func (c *Client) GetState() connectivity.State {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.GetState()
	}
	return connectivity.TransientFailure
}

// StreamData P2: 建立双向流式数据通道
// 复用单个 TCP 连接传输多种数据类型，减少连接开销
func (c *Client) StreamData(ctx context.Context) (edge.ProbeService_StreamDataClient, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()
	return client.StreamData(grpcutil.WithAuth(ctx, c.apiKey))
}
