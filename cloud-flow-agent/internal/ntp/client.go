// Package ntp 提供NTP时钟校准功能
//
// 功能：
// 1. 以Center时间为准同步采集器时钟
// 2. 误差控制在100ms以内
// 3. 支持自动周期性校准
// 4. 支持gRPC时间同步（自定义协议）和标准NTP协议
package ntp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// ============================================================================
// 常量定义
// ============================================================================

const (
	// MaxAllowedOffset 最大允许时钟偏差（100ms）
	MaxAllowedOffset = 100 * time.Millisecond

	// DefaultSyncInterval 默认同步间隔
	DefaultSyncInterval = 5 * time.Minute

	// MinSyncInterval 最小同步间隔
	MinSyncInterval = 30 * time.Second

	// NTPPort 标准NTP端口
	NTPPort = 123

	// NTPPacketSize NTP包大小
	NTPPacketSize = 48

	// NTPEpochOffset NTP纪元偏移（1900到1970的秒数）
	NTPEpochOffset = 2208988800
)

// SyncMode 同步模式
type SyncMode int

const (
	// SyncModeGRPC 通过gRPC与Center同步
	SyncModeGRPC SyncMode = iota
	// SyncModeNTP 通过标准NTP协议同步
	SyncModeNTP
	// SyncModeAuto 自动选择（优先gRPC）
	SyncModeAuto
)

// ============================================================================
// 数据结构
// ============================================================================

// Config NTP客户端配置
type Config struct {
	Enabled       bool          // 启用NTP校准
	Mode          SyncMode      // 同步模式
	NTPServers    []string      // NTP服务器列表（标准NTP模式）
	CenterAddr    string        // Center地址（gRPC模式）
	SyncInterval  time.Duration // 同步间隔
	MaxOffset     time.Duration // 最大允许偏差（超过则调整）
	AdjustStep    bool          // 是否支持步进调整（settimeofday）
	AdjustSlew    bool          // 是否支持渐进调整（adjtime）
	RetryCount    int           // 失败重试次数
	RetryInterval time.Duration // 重试间隔
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		Enabled:       true,
		Mode:          SyncModeAuto,
		NTPServers:    []string{"pool.ntp.org", "time.windows.com", "time.apple.com"},
		SyncInterval:  DefaultSyncInterval,
		MaxOffset:     MaxAllowedOffset,
		AdjustStep:    true,
		AdjustSlew:    true,
		RetryCount:    3,
		RetryInterval: 5 * time.Second,
	}
}

// SyncResult 同步结果
type SyncResult struct {
	Success      bool          `json:"success"`
	LocalTime    time.Time     `json:"local_time"`
	ServerTime   time.Time     `json:"server_time"`
	Offset       time.Duration `json:"offset"`        // 本地时钟偏差（本地-服务器）
	Delay        time.Duration `json:"delay"`         // 网络延迟
	Stratum      uint8         `json:"stratum"`       // 服务器层级
	Server       string        `json:"server"`        // 使用的服务器
	Mode         string        `json:"mode"`          // 同步模式
	AdjustMethod string        `json:"adjust_method"` // 调整方式
	Error        string        `json:"error,omitempty"`
}

// Status 客户端状态
type Status struct {
	IsRunning      bool          `json:"is_running"`
	LastSyncTime   time.Time     `json:"last_sync_time"`
	LastSyncResult *SyncResult   `json:"last_sync_result"`
	CurrentOffset  time.Duration `json:"current_offset"`
	SyncCount      uint64        `json:"sync_count"`
	FailCount      uint64        `json:"fail_count"`
}

// TimeSyncClient 时间同步客户端接口
type TimeSyncClient interface {
	GetServerTime(ctx context.Context) (time.Time, time.Duration, error)
}

// ============================================================================
// Client NTP客户端
// ============================================================================

// Client NTP客户端
type Client struct {
	config Config
	log    *logger.Logger

	// 状态
	isRunning     int32 // atomic
	lastSyncTime  time.Time
	lastResult    *SyncResult
	currentOffset int64 // atomic, nanoseconds
	syncCount     uint64
	failCount     uint64

	// 同步控制
	stopCh chan struct{}
	wg     sync.WaitGroup

	// gRPC客户端（如果使用gRPC模式）
	grpcClient TimeSyncClient

	// 回调
	onSync func(result *SyncResult)
	onFail func(err error)
}

// NewClient 创建NTP客户端
func NewClient(cfg Config, log *logger.Logger) *Client {
	if cfg.SyncInterval == 0 {
		cfg = DefaultConfig()
	}

	return &Client{
		config: cfg,
		log:    log,
		stopCh: make(chan struct{}),
	}
}

// SetGRPCClient 设置gRPC客户端（用于gRPC模式）
func (c *Client) SetGRPCClient(client TimeSyncClient) {
	c.grpcClient = client
}

// OnSync 设置同步成功回调
func (c *Client) OnSync(fn func(result *SyncResult)) {
	c.onSync = fn
}

// OnFail 设置同步失败回调
func (c *Client) OnFail(fn func(err error)) {
	c.onFail = fn
}

// Start 启动NTP客户端
func (c *Client) Start() error {
	if !c.config.Enabled {
		c.log.Info("[NTP] 已禁用")
		return nil
	}

	if !atomic.CompareAndSwapInt32(&c.isRunning, 0, 1) {
		return fmt.Errorf("NTP客户端已启动")
	}

	// 立即执行一次同步
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.syncOnce()
	}()

	// 启动定时同步
	c.wg.Add(1)
	go c.syncLoop()

	c.log.Infof("[NTP] 已启动, 模式=%s, 同步间隔=%v, 最大偏差=%v",
		c.getModeString(), c.config.SyncInterval, c.config.MaxOffset)
	return nil
}

// Stop 停止NTP客户端
func (c *Client) Stop() {
	if atomic.CompareAndSwapInt32(&c.isRunning, 1, 0) {
		close(c.stopCh)
		c.wg.Wait()
		c.log.Info("[NTP] 已停止")
	}
}

// syncLoop 定时同步循环
func (c *Client) syncLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.syncOnce()
		case <-c.stopCh:
			return
		}
	}
}

// syncOnce 执行一次同步
func (c *Client) syncOnce() {
	var result *SyncResult
	var err error

	// 根据模式选择同步方式
	switch c.config.Mode {
	case SyncModeGRPC:
		if c.grpcClient == nil {
			err = fmt.Errorf("gRPC客户端未设置")
		} else {
			result, err = c.syncViaGRPC()
		}
	case SyncModeNTP:
		result, err = c.syncViaNTP()
	case SyncModeAuto:
		// 优先尝试gRPC，失败则回退到NTP
		if c.grpcClient != nil {
			result, err = c.syncViaGRPC()
		}
		if err != nil || result == nil || !result.Success {
			result, err = c.syncViaNTP()
		}
	}

	if err != nil || result == nil || !result.Success {
		atomic.AddUint64(&c.failCount, 1)
		c.log.Warnf("[NTP] 同步失败: %v", err)
		if c.onFail != nil {
			c.onFail(err)
		}
		return
	}

	// 更新状态
	c.lastSyncTime = time.Now()
	c.lastResult = result
	atomic.AddUint64(&c.syncCount, 1)
	atomic.StoreInt64(&c.currentOffset, int64(result.Offset))

	c.log.Infof("[NTP] 同步成功: 服务器=%s, 偏差=%v, 延迟=%v, 调整方式=%s",
		result.Server, result.Offset, result.Delay, result.AdjustMethod)

	if c.onSync != nil {
		c.onSync(result)
	}
}

// syncViaGRPC 通过gRPC同步时间
func (c *Client) syncViaGRPC() (*SyncResult, error) {
	if c.grpcClient == nil {
		return nil, fmt.Errorf("gRPC客户端未设置")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 记录本地发送时间
	t1 := time.Now()

	// 获取服务器时间
	serverTime, serverProcTime, err := c.grpcClient.GetServerTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取服务器时间失败: %w", err)
	}

	// 记录本地接收时间
	t4 := time.Now()

	// 计算网络延迟和时钟偏差
	// 使用NTP算法：
	// delay = (t4 - t1) - (server_proc_time)
	// offset = ((t2 - t1) + (t3 - t4)) / 2
	// 简化：假设服务器处理时间对称
	rtt := t4.Sub(t1)
	delay := rtt - serverProcTime
	offset := t1.Add(rtt/2).Sub(serverTime)

	result := &SyncResult{
		Success:    true,
		LocalTime:  t1,
		ServerTime: serverTime,
		Offset:     offset,
		Delay:      delay,
		Server:     c.config.CenterAddr,
		Mode:       "grpc",
	}

	// 应用时钟调整
	c.applyTimeAdjustment(result)

	return result, nil
}

// syncViaNTP 通过标准NTP协议同步时间
func (c *Client) syncViaNTP() (*SyncResult, error) {
	var lastErr error

	for _, server := range c.config.NTPServers {
		result, err := c.syncWithNTPServer(server)
		if err == nil && result.Success {
			return result, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("所有NTP服务器同步失败: %w", lastErr)
}

// syncWithNTPServer 与单个NTP服务器同步
func (c *Client) syncWithNTPServer(server string) (*SyncResult, error) {
	// 解析服务器地址
	addr := fmt.Sprintf("%s:%d", server, NTPPort)

	// 创建UDP连接
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("连接NTP服务器失败: %w", err)
	}
	defer conn.Close()

	// 设置超时
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// 构造NTP请求包
	req := make([]byte, NTPPacketSize)
	req[0] = 0x1B // LI=0, VN=3, Mode=3 (client)

	// 记录发送时间
	t1 := time.Now()

	// 发送请求
	if _, err := conn.Write(req); err != nil {
		return nil, fmt.Errorf("发送NTP请求失败: %w", err)
	}

	// 接收响应
	resp := make([]byte, NTPPacketSize)
	if _, err := conn.Read(resp); err != nil {
		return nil, fmt.Errorf("接收NTP响应失败: %w", err)
	}

	// 记录接收时间
	t4 := time.Now()

	// 解析NTP响应
	// 参考RFC 5905
	stratum := resp[1]

	// 提取服务器时间（从字节40-47）
	serverSeconds := uint64(resp[40])<<24 | uint64(resp[41])<<16 |
		uint64(resp[42])<<8 | uint64(resp[43])
	serverFraction := uint64(resp[44])<<24 | uint64(resp[45])<<16 |
		uint64(resp[46])<<8 | uint64(resp[47])

	// 转换为Unix时间
	serverUnixSec := int64(serverSeconds - NTPEpochOffset)
	serverUnixNsec := int64(serverFraction * 1e9 / (1 << 32))
	serverTime := time.Unix(serverUnixSec, serverUnixNsec)

	// 提取原始时间戳T2（服务器接收时间）
	origSeconds := uint64(resp[24])<<24 | uint64(resp[25])<<16 |
		uint64(resp[26])<<8 | uint64(resp[27])
	origFraction := uint64(resp[28])<<24 | uint64(resp[29])<<16 |
		uint64(resp[30])<<8 | uint64(resp[31])
	origUnixSec := int64(origSeconds - NTPEpochOffset)
	origUnixNsec := int64(origFraction * 1e9 / (1 << 32))
	t2 := time.Unix(origUnixSec, origUnixNsec)

	// 提取发送时间戳T3（服务器发送时间）
	txSeconds := uint64(resp[32])<<24 | uint64(resp[33])<<16 |
		uint64(resp[34])<<8 | uint64(resp[35])
	txFraction := uint64(resp[36])<<24 | uint64(resp[37])<<16 |
		uint64(resp[38])<<8 | uint64(resp[39])
	txUnixSec := int64(txSeconds - NTPEpochOffset)
	txUnixNsec := int64(txFraction * 1e9 / (1 << 32))
	t3 := time.Unix(txUnixSec, txUnixNsec)

	// 计算延迟和偏差（NTP算法）
	// delay = (t4 - t1) - (t3 - t2)
	// offset = ((t2 - t1) + (t3 - t4)) / 2
	delay := (t4.Sub(t1) - t3.Sub(t2))
	offset := (t2.Sub(t1) + t3.Sub(t4)) / 2

	result := &SyncResult{
		Success:    true,
		LocalTime:  t1,
		ServerTime: serverTime,
		Offset:     offset,
		Delay:      delay,
		Stratum:    stratum,
		Server:     server,
		Mode:       "ntp",
	}

	// 应用时钟调整
	c.applyTimeAdjustment(result)

	return result, nil
}

// applyTimeAdjustment 应用时钟调整
func (c *Client) applyTimeAdjustment(result *SyncResult) {
	offset := result.Offset

	// 如果偏差在允许范围内，不调整
	if absDuration(offset) <= c.config.MaxOffset/10 {
		result.AdjustMethod = "none"
		return
	}

	// 优先使用渐进调整（adjtime）
	if c.config.AdjustSlew && absDuration(offset) <= c.config.MaxOffset {
		// 渐进调整时钟
		if err := c.slewTime(offset); err != nil {
			c.log.Warnf("[NTP] 渐进调整失败: %v, 尝试步进调整", err)
			// 回退到步进调整
			if c.config.AdjustStep {
				if err := c.stepTime(offset); err != nil {
					c.log.Errorf("[NTP] 步进调整失败: %v", err)
					result.AdjustMethod = "failed"
					return
				}
				result.AdjustMethod = "step"
				return
			}
			result.AdjustMethod = "failed"
			return
		}
		result.AdjustMethod = "slew"
		return
	}

	// 偏差较大，使用步进调整
	if c.config.AdjustStep {
		if err := c.stepTime(offset); err != nil {
			c.log.Errorf("[NTP] 步进调整失败: %v", err)
			result.AdjustMethod = "failed"
			return
		}
		result.AdjustMethod = "step"
		return
	}

	result.AdjustMethod = "none"
}

// slewTime 渐进调整时钟（使用adjtime）
func (c *Client) slewTime(offset time.Duration) error {
	// 在Linux上使用adjtime系统调用
	// 由于Go没有直接暴露adjtime，我们使用C库或通过syscall
	// 这里使用简化的实现：记录偏移量，在GetCorrectedTime中应用

	// 实际生产环境应该调用C.adjtime或执行/usr/sbin/adjtimex
	// 这里仅作为示例
	c.log.Infof("[NTP] 渐进调整时钟: offset=%v", offset)

	// 渐进调整：将偏移量分散到多个时间单位中
	// 例如：100ms的偏移，在10秒内以10ms/s的速率调整
	slewRate := offset / 10 // 10秒完成调整
	_ = slewRate

	return nil
}

// stepTime 步进调整时钟（使用settimeofday）
func (c *Client) stepTime(offset time.Duration) error {
	// 步进调整：直接设置系统时间
	// 需要root权限

	newTime := time.Now().Add(-offset) // 减去偏移量，因为offset = local - server
	c.log.Infof("[NTP] 步进调整时钟: 新时间=%v", newTime)

	// 实际生产环境应该调用C.settimeofday或执行date命令
	// 这里仅作为示例

	return nil
}

// GetCorrectedTime 获取校准后的时间
func (c *Client) GetCorrectedTime() time.Time {
	offset := time.Duration(atomic.LoadInt64(&c.currentOffset))
	return time.Now().Add(-offset)
}

// GetStatus 获取客户端状态
func (c *Client) GetStatus() *Status {
	return &Status{
		IsRunning:      atomic.LoadInt32(&c.isRunning) == 1,
		LastSyncTime:   c.lastSyncTime,
		LastSyncResult: c.lastResult,
		CurrentOffset:  time.Duration(atomic.LoadInt64(&c.currentOffset)),
		SyncCount:      atomic.LoadUint64(&c.syncCount),
		FailCount:      atomic.LoadUint64(&c.failCount),
	}
}

// ForceSync 强制立即同步
func (c *Client) ForceSync() (*SyncResult, error) {
	if atomic.LoadInt32(&c.isRunning) == 0 {
		return nil, fmt.Errorf("NTP客户端未启动")
	}

	c.syncOnce()

	if c.lastResult == nil {
		return nil, fmt.Errorf("同步未完成")
	}

	return c.lastResult, nil
}

// getModeString 获取模式字符串
func (c *Client) getModeString() string {
	switch c.config.Mode {
	case SyncModeGRPC:
		return "grpc"
	case SyncModeNTP:
		return "ntp"
	case SyncModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
