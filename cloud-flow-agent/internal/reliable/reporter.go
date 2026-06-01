// Package reliable 提供可靠的数据上报功能
//
// 核心能力：
//   - 数据校验和：为每批数据计算 SHA256 校验和，确保传输完整性
//   - 离线缓存：网络异常时本地缓存数据，最多缓存 1 小时
//   - 自动重传：网络恢复后按序列号顺序重传缓存数据
//   - 去重保证：通过序列号机制避免重复发送
package reliable

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"cloud-flow-agent/pkg/logger"
	edge "cloud-flow/proto"
)

// Sender 数据发送接口
type Sender interface {
	SendMetrics(ctx context.Context, batch *edge.MetricsBatch) error
}

// NetworkChecker 网络可用性检查接口
type NetworkChecker interface {
	IsAvailable() bool
}

// Config 可靠上报配置
type Config struct {
	// 缓存目录
	CacheDir string

	// 最大缓存时长（默认1小时）
	MaxCacheDuration time.Duration

	// 重传批次大小（默认100条）
	RetransmitBatchSize int

	// 重传间隔（默认100ms）
	RetransmitInterval time.Duration

	// 发送超时（默认10秒）
	SendTimeout time.Duration

	// 是否启用校验和（默认true）
	EnableChecksum bool

	// 最大缓存文件大小（默认100MB）
	MaxCacheSizeBytes int64
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		CacheDir:            "/tmp/cloud-flow-cache",
		MaxCacheDuration:    1 * time.Hour,
		RetransmitBatchSize: 100,
		RetransmitInterval:  100 * time.Millisecond,
		SendTimeout:         10 * time.Second,
		EnableChecksum:      true,
		MaxCacheSizeBytes:   100 * 1024 * 1024, // 100MB
	}
}

// Reporter 可靠上报器
type Reporter struct {
	cfg    Config
	log    *logger.Logger
	sender Sender
	net    NetworkChecker

	mu       sync.RWMutex
	seqId    atomic.Int64 // 全局序列号
	stopCh   chan struct{}

	// 缓存状态
	cacheDir     string
	cacheCount   atomic.Int64 // 缓存条目数
	cacheSize    atomic.Int64 // 缓存大小
	wasOffline   bool         // 上一次是否离线

	// 统计
	stats struct {
		sentTotal     atomic.Int64 // 发送总数
		sentSuccess   atomic.Int64 // 发送成功
		sentFailed    atomic.Int64 // 发送失败
		cachedTotal   atomic.Int64 // 缓存总数
		retransmitted atomic.Int64 // 重传总数
		retransFailed atomic.Int64 // 重传失败
		deduplicated  atomic.Int64 // 去重数
	}
}

// NewReporter 创建可靠上报器
func NewReporter(cfg Config, sender Sender, net NetworkChecker, log *logger.Logger) (*Reporter, error) {
	if cfg.CacheDir == "" {
		cfg.CacheDir = DefaultConfig().CacheDir
	}
	if cfg.MaxCacheDuration <= 0 {
		cfg.MaxCacheDuration = DefaultConfig().MaxCacheDuration
	}
	if cfg.RetransmitBatchSize <= 0 {
		cfg.RetransmitBatchSize = DefaultConfig().RetransmitBatchSize
	}
	if cfg.RetransmitInterval <= 0 {
		cfg.RetransmitInterval = DefaultConfig().RetransmitInterval
	}
	if cfg.SendTimeout <= 0 {
		cfg.SendTimeout = DefaultConfig().SendTimeout
	}

	// 确保缓存目录存在
	if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}

	r := &Reporter{
		cfg:      cfg,
		log:      log,
		sender:   sender,
		net:      net,
		cacheDir: cfg.CacheDir,
		stopCh:   make(chan struct{}),
	}

	// 恢复序列号（从已有缓存文件中获取最大值）
	r.recoverSeqId()

	// 启动后台协程
	go r.cleanupLoop()
	go r.retransmitLoop()

	return r, nil
}

// Stop 停止上报器
func (r *Reporter) Stop() {
	close(r.stopCh)
	r.log.Info("[可靠上报] 已停止")
}

// Send 发送指标数据（带校验和）
// 返回 true 表示发送成功，false 表示已缓存
func (r *Reporter) Send(batch *edge.MetricsBatch) bool {
	if batch == nil || len(batch.Metrics) == 0 {
		return true
	}

	// 分配序列号
	seq := r.seqId.Add(1)
	batch.SeqId = seq
	batch.Timestamp = time.Now().Unix()

	// 计算校验和
	if r.cfg.EnableChecksum {
		batch.Checksum = r.calculateChecksum(batch)
	}

	// 尝试发送
	if r.net != nil && r.net.IsAvailable() {
		err := r.doSend(batch)
		if err == nil {
			r.stats.sentSuccess.Add(1)
			r.wasOffline = false
			return true
		}
		r.log.Warnf("[可靠上报] 发送失败(seq=%d): %v，写入缓存", seq, err)
		r.stats.sentFailed.Add(1)
	}

	// 发送失败或网络不可用，写入缓存
	if err := r.writeToCache(batch); err != nil {
		r.log.Errorf("[可靠上报] 写入缓存失败(seq=%d): %v", seq, err)
	} else {
		r.stats.cachedTotal.Add(1)
		r.wasOffline = true
		r.log.Debugf("[可靠上报] 数据已缓存(seq=%d, metrics=%d)", seq, len(batch.Metrics))
	}

	return false
}

// SendWithChecksum 发送带校验和的指标数据（公开方法）
func (r *Reporter) SendWithChecksum(batch *edge.MetricsBatch) error {
	if batch == nil || len(batch.Metrics) == 0 {
		return nil
	}

	seq := r.seqId.Add(1)
	batch.SeqId = seq
	batch.Timestamp = time.Now().Unix()

	if r.cfg.EnableChecksum {
		batch.Checksum = r.calculateChecksum(batch)
	}

	return r.doSend(batch)
}

// calculateChecksum 计算批次数据的 SHA256 校验和
// 校验范围：ProbeId + SeqId + 所有 MetricData 的关键字段
func (r *Reporter) calculateChecksum(batch *edge.MetricsBatch) string {
	h := sha256.New()

	// 写入固定字段
	h.Write([]byte(batch.ProbeId))
	h.Write([]byte(fmt.Sprintf("%d", batch.SeqId)))

	// 按固定顺序写入每条 MetricData 的关键字段
	for _, m := range batch.Metrics {
		h.Write([]byte(m.ProbeId))
		h.Write([]byte(fmt.Sprintf("%d", m.Timestamp)))
		h.Write([]byte(m.SrcIp))
		h.Write([]byte(m.DstIp))
		h.Write([]byte(fmt.Sprintf("%d:%d", m.SrcPort, m.DstPort)))
		h.Write([]byte(m.Protocol))
		h.Write([]byte(fmt.Sprintf("%d:%d", m.Bytes, m.Packets)))
		h.Write([]byte(fmt.Sprintf("%f", m.Value)))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// doSend 执行实际发送
func (r *Reporter) doSend(batch *edge.MetricsBatch) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.SendTimeout)
	defer cancel()

	r.stats.sentTotal.Add(1)
	return r.sender.SendMetrics(ctx, batch)
}

// writeToCache 将批次数据写入本地缓存
func (r *Reporter) writeToCache(batch *edge.MetricsBatch) error {
	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	// 检查缓存大小限制
	currentSize := r.cacheSize.Add(int64(len(data)))
	if currentSize > r.cfg.MaxCacheSizeBytes {
		r.cacheSize.Add(-int64(len(data)))
		// 缓存已满，清理最旧的文件
		r.evictOldest()
	}

	// 写入文件：文件名为 seq_id.jsonl
	fileName := fmt.Sprintf("%020d.jsonl", batch.SeqId)
	filePath := filepath.Join(r.cacheDir, fileName)

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		r.cacheSize.Add(-int64(len(data)))
		return fmt.Errorf("写入文件失败: %w", err)
	}

	r.cacheCount.Add(1)
	return nil
}

// retransmitLoop 重传循环
func (r *Reporter) retransmitLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 仅在从离线恢复时触发重传
			if r.wasOffline && r.net != nil && r.net.IsAvailable() {
				r.retransmitCached()
				r.wasOffline = false
			}
		case <-r.stopCh:
			return
		}
	}
}

// retransmitCached 重传所有缓存数据
func (r *Reporter) retransmitCached() {
	// 读取所有缓存文件
	entries, err := os.ReadDir(r.cacheDir)
	if err != nil {
		r.log.Warnf("[可靠上报] 读取缓存目录失败: %v", err)
		return
	}

	if len(entries) == 0 {
		return
	}

	// 按序列号排序（文件名即序列号）
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	r.log.Infof("[可靠上报] 开始重传缓存数据: %d个批次", len(entries))

	sentCount := 0
	failCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// 检查是否过期
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > r.cfg.MaxCacheDuration {
			// 过期数据，删除
			os.Remove(filepath.Join(r.cacheDir, entry.Name()))
			r.cacheCount.Add(-1)
			continue
		}

		// 读取并解析
		filePath := filepath.Join(r.cacheDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var batch edge.MetricsBatch
		if err := json.Unmarshal(data, &batch); err != nil {
			r.log.Warnf("[可靠上报] 解析缓存文件失败: %s, err: %v", entry.Name(), err)
			os.Remove(filePath)
			r.cacheCount.Add(-1)
			continue
		}

		// 校验校验和
		if r.cfg.EnableChecksum && batch.Checksum != "" {
			expectedChecksum := r.calculateChecksum(&batch)
			if expectedChecksum != batch.Checksum {
				r.log.Warnf("[可靠上报] 校验和不匹配(seq=%d): expected=%s, got=%s",
					batch.SeqId, expectedChecksum, batch.Checksum)
				os.Remove(filePath)
				r.cacheCount.Add(-1)
				continue
			}
		}

		// 发送
		err = r.doSend(&batch)
		if err != nil {
			r.log.Warnf("[可靠上报] 重传失败(seq=%d): %v", batch.SeqId, err)
			failCount++
			r.stats.retransFailed.Add(1)
			// 连续失败时停止重传，等待下一轮
			if failCount >= 3 {
				r.log.Warn("[可靠上报] 连续重传失败3次，暂停重传等待下一轮")
				break
			}
			continue
		}

		// 发送成功，删除缓存文件
		os.Remove(filePath)
		r.cacheCount.Add(-1)
		sentCount++
		r.stats.retransmitted.Add(1)

		// 重传间隔
		time.Sleep(r.cfg.RetransmitInterval)
	}

	if sentCount > 0 || failCount > 0 {
		r.log.Infof("[可靠上报] 重传完成: 成功=%d, 失败=%d", sentCount, failCount)
	}
}

// cleanupLoop 定期清理过期缓存
func (r *Reporter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.cleanupExpired()
		case <-r.stopCh:
			return
		}
	}
}

// cleanupExpired 清理过期缓存文件
func (r *Reporter) cleanupExpired() {
	entries, err := os.ReadDir(r.cacheDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-r.cfg.MaxCacheDuration)
	cleaned := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			filePath := filepath.Join(r.cacheDir, entry.Name())
			if err := os.Remove(filePath); err == nil {
				r.cacheCount.Add(-1)
				cleaned++
			}
		}
	}

	if cleaned > 0 {
		r.log.Infof("[可靠上报] 清理过期缓存: %d个文件", cleaned)
	}
}

// evictOldest 淘汰最旧的缓存文件
func (r *Reporter) evictOldest() {
	entries, err := os.ReadDir(r.cacheDir)
	if err != nil || len(entries) == 0 {
		return
	}

	// 找到最旧的文件
	var oldest string
	var oldestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if oldest == "" || info.ModTime().Before(oldestTime) {
			oldest = entry.Name()
			oldestTime = info.ModTime()
		}
	}

	if oldest != "" {
		filePath := filepath.Join(r.cacheDir, oldest)
		if info, err := os.Stat(filePath); err == nil {
			r.cacheSize.Add(-info.Size())
		}
		os.Remove(filePath)
		r.cacheCount.Add(-1)
	}
}

// recoverSeqId 从缓存文件中恢复最大序列号
func (r *Reporter) recoverSeqId() {
	entries, err := os.ReadDir(r.cacheDir)
	if err != nil || len(entries) == 0 {
		return
	}

	maxSeq := int64(0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var seq int64
		if _, err := fmt.Sscanf(entry.Name(), "%020d.jsonl", &seq); err == nil {
			if seq > maxSeq {
				maxSeq = seq
			}
		}
	}

	if maxSeq > 0 {
		r.seqId.Store(maxSeq)
		r.log.Infof("[可靠上报] 恢复序列号: %d", maxSeq)
	}

	// 统计缓存大小
	var totalSize int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if info, err := entry.Info(); err == nil {
			totalSize += info.Size()
		}
	}
	r.cacheSize.Store(totalSize)
	r.cacheCount.Store(int64(len(entries)))
}

// ForceRetransmit 强制重传所有缓存数据
func (r *Reporter) ForceRetransmit() (sent, failed int) {
	r.retransmitCached()
	return int(r.stats.retransmitted.Load()), int(r.stats.retransFailed.Load())
}

// Stats 返回上报统计
func (r *Reporter) Stats() (sentTotal, sentSuccess, sentFailed, cached, retransmitted int64) {
	return r.stats.sentTotal.Load(),
		r.stats.sentSuccess.Load(),
		r.stats.sentFailed.Load(),
		r.stats.cachedTotal.Load(),
		r.stats.retransmitted.Load()
}

// CacheInfo 返回缓存信息
func (r *Reporter) CacheInfo() (count, sizeBytes int64) {
	return r.cacheCount.Load(), r.cacheSize.Load()
}
