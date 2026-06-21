// Package persistence 提供本地持久化存储
//
// 实现带校验和的数据持久化，防止数据损坏
package persistence

import (
	"hash/crc32"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ============================================================================
// CRC32 校验和工具
// ============================================================================

// WriteWithChecksum 写入数据并附加CRC32校验和
// 文件格式: [4字节CRC32校验和][数据内容]
func WriteWithChecksum(path string, data []byte) error {
	// 计算校验和
	checksum := crc32.ChecksumIEEE(data)

	// 构造写入缓冲区
	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[0:4], checksum)
	copy(buf[4:], data)

	// 先写入临时文件
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, buf, 0644); err != nil {
		return fmt.Errorf("write temp file failed: %w", err)
	}

	// 原子重命名
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("atomic rename failed: %w", err)
	}

	return nil
}

// ReadWithChecksum 读取数据并验证CRC32校验和
// 如果校验失败，将原文件备份为 .corrupted 并返回错误
func ReadWithChecksum(path string) ([]byte, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %w", err)
	}

	if len(buf) < 4 {
		// 文件太短，备份损坏文件
		backupCorruptedFile(path)
		return nil, fmt.Errorf("file too short: expected at least 4 bytes, got %d", len(buf))
	}

	// 提取存储的校验和
	storedChecksum := binary.BigEndian.Uint32(buf[0:4])

	// 计算实际数据的校验和
	actualChecksum := crc32.ChecksumIEEE(buf[4:])

	// 校验不匹配
	if storedChecksum != actualChecksum {
		backupCorruptedFile(path)
		return nil, fmt.Errorf("checksum mismatch: stored=0x%08x, actual=0x%08x",
			storedChecksum, actualChecksum)
	}

	return buf[4:], nil
}

// backupCorruptedFile 备份损坏的文件
func backupCorruptedFile(path string) {
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.corrupted.%s", path, timestamp)

	// 尝试复制备份
	data, err := os.ReadFile(path)
	if err == nil {
		os.WriteFile(backupPath, data, 0644)
	}

	// 重命名原文件
	os.Rename(path, path+".corrupted")
}

// ============================================================================
// 持久化管理器集成校验和
// ============================================================================

// saveWithChecksum 保存数据带校验和
func (p *Persistence) saveWithChecksum(key string, data []byte) error {
	path := filepath.Join(p.config.DataDir, key+".dat")
	return WriteWithChecksum(path, data)
}

// loadWithChecksum 加载数据并验证校验和
func (p *Persistence) loadWithChecksum(key string) ([]byte, error) {
	path := filepath.Join(p.config.DataDir, key+".dat")
	return ReadWithChecksum(path)
}

// ChecksumStats 校验和统计信息
type ChecksumStats struct {
	TotalFiles      int   `json:"total_files"`
	ValidFiles      int   `json:"valid_files"`
	CorruptedFiles  int   `json:"corrupted_files"`
	TotalBytes      int64 `json:"total_bytes"`
}

// GetChecksumStats 获取校验和统计
func (p *Persistence) GetChecksumStats() (*ChecksumStats, error) {
	stats := &ChecksumStats{}

	files, err := os.ReadDir(p.config.DataDir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) != ".dat" {
			continue
		}

		stats.TotalFiles++

		path := filepath.Join(p.config.DataDir, file.Name())
		info, err := file.Info()
		if err == nil {
			stats.TotalBytes += info.Size()
		}

		_, err = ReadWithChecksum(path)
		if err == nil {
			stats.ValidFiles++
		} else {
			stats.CorruptedFiles++
		}
	}

	return stats, nil
}
