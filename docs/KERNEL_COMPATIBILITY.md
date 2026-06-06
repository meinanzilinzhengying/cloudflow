# 内核兼容性解决方案

## 概述

本系统提供了完整的内核兼容性解决方案，解决了 eBPF 程序与内核版本强相关的问题。

## 问题背景

eBPF 程序对内核版本有严格的依赖：
- 不同内核版本的 eBPF 特性支持不同
- BTF（BPF Type Format）仅在较新的内核中可用
- 某些 eBPF 辅助函数在旧内核中不存在
- 直接在一个内核上编译的 BPF 程序在另一个内核上可能无法运行

## 解决方案

我们采用多层次的兼容性策略：

### 1. 多版本 BPF 程序编译

支持三个主要内核版本分支：

| 版本       | 内核范围   | 特性                         |
|------------|------------|------------------------------|
| **legacy** | 4.14-4.19  | 基础 eBPF，使用 perf buffer |
| **modern** | 5.0-5.7    | 增强特性，BTF 支持可选       |
| **latest** | 5.8+       | CO-RE，BTF，ring buffer     |

### 2. 运行时自动检测和选择

程序启动时自动检测内核特性：

```go
// 获取内核配置
kernelConfig, _ := GetKernelConfig()

// 选择合适的 BPF 版本
bpfVersion := SelectBPFVersion(kernelConfig)

// 查找兼容的 BPF 文件
bpfFile, _ := FindCompatibleBPFFile(basePath, bpfVersion, arch)
```

### 3. 传统模式降级

如果 eBPF 不可用，自动降级到传统采集模式：

```go
collector, err := NewEnhancedFallbackCollector(opts)
if err != nil {
    // 自动使用传统模式
}
```

## 使用方式

### 编译多版本 BPF 程序

```bash
cd cloud-flow-agent

# 编译所有版本
make -f Makefile.ebpf all

# 单独编译特定版本
make -f Makefile.ebpf legacy
make -f Makefile.ebpf modern
make -f Makefile.ebpf latest
```

### 安装

```bash
sudo ./scripts/install.sh
```

安装脚本会自动：
- 检测当前内核版本
- 选择合适的 BPF 程序版本
- 安装多个版本的 BPF 程序作为备选

### 运行模式配置

通过环境变量强制使用特定后端：

```bash
# 使用 libbpf 后端
export CLOUD_FLOW_BPF_BACKEND=libbpf

# 使用 cilium/ebpf 后端
export CLOUD_FLOW_BPF_BACKEND=cilium

# 自动选择（默认）
export CLOUD_FLOW_BPF_BACKEND=auto
```

## 文件组织

```
/etc/cloud-flow-agent/bpf/
├── tc.legacy.amd64.bpf.o      # 4.14-4.19, AMD64
├── tc.legacy.arm64.bpf.o      # 4.14-4.19, ARM64
├── tc.legacy.bpf.o            # 4.14-4.19, 通用
├── tc.modern.amd64.bpf.o      # 5.0-5.7, AMD64
├── tc.modern.arm64.bpf.o      # 5.0-5.7, ARM64
├── tc.modern.bpf.o            # 5.0-5.7, 通用
├── tc.latest.amd64.bpf.o      # 5.8+, AMD64
├── tc.latest.arm64.bpf.o      # 5.8+, ARM64
├── tc.latest.bpf.o            # 5.8+, 通用
└── tc.bpf.o -> tc.latest.bpf.o  # 链接到当前最佳版本
```

## 内核兼容性信息

程序启动时会输出详细的内核兼容性信息：

```
==========================================
  内核兼容性信息
==========================================
  内核版本:   5.10.0-23-generic
  架构:      amd64
  特性:
    BTF:      ✓ 支持
    CO-RE:    ✓ 支持
    kprobe:   ✓ 支持
    tracing:  ✓ 支持
    ringbuf:  ✓ 支持
  推荐 BPF 版本: latest
==========================================
```

## 常见问题

### Q: 如果没有兼容的 BPF 程序怎么办？

A: 程序会自动降级到传统采集模式，基本功能仍然可用。

### Q: 如何知道当前使用的是哪个版本？

A: 检查日志中的 "[eBPF]" 前缀，或查看平台自监控页面。

### Q: 可以强制使用特定版本的 BPF 程序吗？

A: 可以通过创建符号链接实现：
```bash
sudo ln -sf tc.modern.bpf.o /etc/cloud-flow-agent/bpf/tc.bpf.o
```

### Q: BPF 程序在哪里编译？

A: 最佳实践是在目标系统上编译，确保最大兼容性。

## 开发者指南

### 添加新的内核特性检测

在 `kernel_compat.go` 中添加：

```go
func CheckMyNewFeature(version KernelVersion) bool {
    return version.Major >= 5 && version.Minor >= 12
}
```

### 在 BPF 代码中添加版本条件

```c
#ifdef KERNEL_LATEST
// 仅在最新内核中使用的代码
struct bpf_spin_lock lock;
#elif KERNEL_MODERN
// 在现代内核中使用的代码
#else
// 传统内核的兼容代码
#endif
```

## 相关文件

- `cloud-flow-agent/internal/ebpfcollector/kernel_compat.go` - 内核检测和选择
- `cloud-flow-agent/internal/ebpfcollector/traditional_collector.go` - 传统采集模式
- `cloud-flow-agent/Makefile.ebpf` - 多版本编译
- `cloud-flow-agent/scripts/install.sh` - 智能安装脚本
