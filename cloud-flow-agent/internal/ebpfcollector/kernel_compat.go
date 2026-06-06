//go:build linux
// +build linux

package ebpfcollector

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// KernelVersion 内核版本信息
type KernelVersion struct {
	Major int
	Minor int
	Patch int
	Distro string
}

// String 输出内核版本字符串
func (kv KernelVersion) String() string {
	if kv.Distro != "" {
		return fmt.Sprintf("%d.%d.%d (%s)", kv.Major, kv.Minor, kv.Patch, kv.Distro)
	}
	return fmt.Sprintf("%d.%d.%d", kv.Major, kv.Minor, kv.Patch)
}

// KernelFeature 内核支持的 eBPF 特性
type KernelFeature string

const (
	// KernelFeatureBTF BTF 支持（5.4+）
	KernelFeatureBTF KernelFeature = "btf"
	// KernelFeatureCO-RE CO-RE 支持（5.8+）
	KernelFeatureCO_RE KernelFeature = "core"
	// KernelFeatureKprobe 通用 kprobe 支持（4.15+）
	KernelFeatureKprobe KernelFeature = "kprobe"
	// KernelFeatureTracing  tracing 支持（5.2+）
	KernelFeatureTracing KernelFeature = "tracing"
	// KernelFeatureRingBuffer ringbuffer 支持（5.8+）
	KernelFeatureRingBuffer KernelFeature = "ringbuf"
)

// KernelConfig 内核配置
type KernelConfig struct {
	Version  KernelVersion
	Features map[KernelFeature]bool
	Arch     string
}

// BPFVersion BPF 程序版本
type BPFVersion string

const (
	// BPFVersionLegacy 传统版本（4.14-4.19）
	BPFVersionLegacy BPFVersion = "legacy"
	// BPFVersionModern 现代版本（5.0-5.7）
	BPFVersionModern BPFVersion = "modern"
	// BPFVersionLatest 最新版本（5.8+）
	BPFVersionLatest BPFVersion = "latest"
)

// GetKernelVersion 获取当前内核版本
func GetKernelVersion() (KernelVersion, error) {
	// 首先尝试从 /proc/version_signature 获取（主要是 Ubuntu/Debian）
	if version, err := getVersionFromSignature(); err == nil {
		return version, nil
	}

	// 尝试从 uname 获取
	versionStr, err := getUnameRelease()
	if err != nil {
		return KernelVersion{}, fmt.Errorf("获取内核版本失败: %w", err)
	}

	return parseKernelVersion(versionStr)
}

// getUnameRelease 从 uname 获取内核发行版
func getUnameRelease() (string, error) {
	file, err := os.Open("/proc/version")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 3 {
			return parts[2], nil
		}
	}
	return "", fmt.Errorf("无法解析 /proc/version")
}

// getVersionFromSignature 从 /proc/version_signature 获取版本
func getVersionFromSignature() (KernelVersion, error) {
	data, err := os.ReadFile("/proc/version_signature")
	if err != nil {
		return KernelVersion{}, err
	}

	// Ubuntu/Debian 格式类似: "Ubuntu 4.15.0-142.146-generic 4.15.18"
	parts := strings.Fields(string(data))
	for _, part := range parts {
		if strings.Contains(part, ".") {
			if version, err := parseKernelVersion(part); err == nil {
				return version, nil
			}
		}
	}
	return KernelVersion{}, fmt.Errorf("无法从 version_signature 解析")
}

// parseKernelVersion 解析内核版本字符串
func parseKernelVersion(versionStr string) (KernelVersion, error) {
	result := KernelVersion{}
	
	// 移除 '-' 后面的内容
	if idx := strings.Index(versionStr, "-"); idx != -1 {
		versionStr = versionStr[:idx]
	}
	
	parts := strings.Split(versionStr, ".")
	if len(parts) < 2 {
		return result, fmt.Errorf("无效的内核版本格式: %s", versionStr)
	}
	
	var err error
	result.Major, err = strconv.Atoi(parts[0])
	if err != nil {
		return result, fmt.Errorf("解析主版本号失败: %w", err)
	}
	
	result.Minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return result, fmt.Errorf("解析次版本号失败: %w", err)
	}
	
	if len(parts) >= 3 {
		result.Patch, _ = strconv.Atoi(parts[2])
	}
	
	return result, nil
}

// CheckKernelFeature 检查内核是否支持特定特性
func CheckKernelFeature(feature KernelFeature, version KernelVersion) bool {
	switch feature {
	case KernelFeatureBTF:
		// BTF 支持从 5.4 开始，但 5.8+ 更稳定
		if version.Major > 5 || (version.Major == 5 && version.Minor >= 4) {
			return true
		}
		// 检查 /sys/kernel/btf/vmlinux 是否存在
		if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil {
			return true
		}
		return false
	
	case KernelFeatureCO_RE:
		// CO-RE 支持从 5.8 开始
		return version.Major > 5 || (version.Major == 5 && version.Minor >= 8)
	
	case KernelFeatureKprobe:
		// kprobe 支持从 4.15 开始
		return version.Major > 4 || (version.Major == 4 && version.Minor >= 15)
	
	case KernelFeatureTracing:
		// tracing 支持从 5.2 开始
		return version.Major > 5 || (version.Major == 5 && version.Minor >= 2)
	
	case KernelFeatureRingBuffer:
		// ringbuffer 支持从 5.8 开始
		return version.Major > 5 || (version.Major == 5 && version.Minor >= 8)
	}
	return false
}

// GetKernelConfig 获取完整的内核配置
func GetKernelConfig() (*KernelConfig, error) {
	version, err := GetKernelVersion()
	if err != nil {
		return nil, err
	}
	
	config := &KernelConfig{
		Version: version,
		Arch:    runtime.GOARCH,
		Features: map[KernelFeature]bool{
			KernelFeatureBTF:         CheckKernelFeature(KernelFeatureBTF, version),
			KernelFeatureCO_RE:       CheckKernelFeature(KernelFeatureCO_RE, version),
			KernelFeatureKprobe:      CheckKernelFeature(KernelFeatureKprobe, version),
			KernelFeatureTracing:     CheckKernelFeature(KernelFeatureTracing, version),
			KernelFeatureRingBuffer:  CheckKernelFeature(KernelFeatureRingBuffer, version),
		},
	}
	
	return config, nil
}

// SelectBPFVersion 根据内核版本选择合适的 BPF 程序版本
func SelectBPFVersion(config KernelConfig) BPFVersion {
	version := config.Version
	
	if version.Major > 5 || (version.Major == 5 && version.Minor >= 8) {
		// 5.8+ 使用最新版本
		return BPFVersionLatest
	} else if version.Major > 5 || (version.Major == 5 && version.Minor >= 0) {
		// 5.0-5.7 使用现代版本
		return BPFVersionModern
	}
	
	// 4.14-4.19 使用传统版本
	return BPFVersionLegacy
}

// FindCompatibleBPFFile 查找兼容的 BPF 文件
func FindCompatibleBPFFile(basePath string, bpfVersion BPFVersion, arch string) (string, error) {
	// 搜索顺序:
	// 1. basePath/bpf/tc.{version}.{arch}.bpf.o
	// 2. basePath/bpf/tc.{version}.bpf.o
	// 3. basePath/bpf/tc.{arch}.bpf.o
	// 4. basePath/bpf/tc.bpf.o
	
	versionStr := string(bpfVersion)
	
	candidates := []string{
		filepath.Join(basePath, "bpf", fmt.Sprintf("tc.%s.%s.bpf.o", versionStr, arch)),
		filepath.Join(basePath, "bpf", fmt.Sprintf("tc.%s.bpf.o", versionStr)),
		filepath.Join(basePath, "bpf", fmt.Sprintf("tc.%s.bpf.o", arch)),
		filepath.Join(basePath, "bpf", "tc.bpf.o"),
	}
	
	// 也尝试绝对路径
	candidates = append(candidates, 
		fmt.Sprintf("/etc/cloud-flow-agent/bpf/tc.%s.%s.bpf.o", versionStr, arch),
		fmt.Sprintf("/etc/cloud-flow-agent/bpf/tc.%s.bpf.o", versionStr),
		"/etc/cloud-flow-agent/bpf/tc.bpf.o",
		"/usr/share/cloud-flow-agent/bpf/tc.bpf.o",
	)
	
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			log.Printf("[eBPF] 找到兼容的 BPF 文件: %s", path)
			return path, nil
		}
	}
	
	return "", fmt.Errorf("未找到兼容的 BPF 程序文件，尝试了: %v", candidates)
}

// LogKernelCompatibilityInfo 记录内核兼容性信息
func LogKernelCompatibilityInfo(config KernelConfig) {
	log.Println("==========================================")
	log.Println("  内核兼容性信息")
	log.Println("==========================================")
	log.Printf("  内核版本:  %s", config.Version)
	log.Printf("  架构:      %s", config.Arch)
	log.Printf("  特性:")
	log.Printf("    BTF:      %s", formatStatus(config.Features[KernelFeatureBTF]))
	log.Printf("    CO-RE:    %s", formatStatus(config.Features[KernelFeatureCO_RE]))
	log.Printf("    kprobe:   %s", formatStatus(config.Features[KernelFeatureKprobe]))
	log.Printf("    tracing:  %s", formatStatus(config.Features[KernelFeatureTracing]))
	log.Printf("    ringbuf:  %s", formatStatus(config.Features[KernelFeatureRingBuffer]))
	log.Printf("  推荐 BPF 版本: %s", SelectBPFVersion(config))
	log.Println("==========================================")
}

func formatStatus(supported bool) string {
	if supported {
		return "✓ 支持"
	}
	return "✗ 不支持"
}

// IsKernelSupported 检查内核是否满足最低要求
func IsKernelSupported(version KernelVersion) bool {
	// 最低要求 4.14
	if version.Major < 4 {
		return false
	}
	if version.Major == 4 && version.Minor < 14 {
		return false
	}
	return true
}
