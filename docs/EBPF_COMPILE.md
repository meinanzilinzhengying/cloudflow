# eBPF 程序编译指南

## 概述

CloudFlow Agent 使用 eBPF（extended Berkeley Packet Filter）技术进行网络数据包捕获和分析。
所有 eBPF 程序需要使用 clang 编译为 BPF 字节码后，才能被 Go 程序通过 go:embed 加载。

---

## 前置依赖安装

### CentOS 7/8 / RHEL / 麒麟V10

```bash
# 安装 clang/llvm
yum install -y clang llvm llvm-devel libbpf-devel

# 安装内核头文件
yum install -y kernel-devel-$(uname -r)

# 验证安装
clang --version
llc --version
```

### Ubuntu 20.04+ / Debian 11

```bash
# 安装 clang/llvm
apt-get update
apt-get install -y clang llvm libbpf-dev

# 安装内核头文件
apt-get install -y linux-headers-$(uname -r)

# 验证安装
clang --version
llc --version
```

### 从源码安装 clang（旧系统）

```bash
# Ubuntu 18.04 需要安装 clang-12
apt-get install -y clang-12 llvm-12
update-alternatives --install /usr/bin/clang clang /usr/bin/clang-12 100
update-alternatives --install /usr/bin/llc llc /usr/bin/llc-12 100
```

---

## 编译 eBPF 程序

### 一键编译（推荐）

```bash
cd cloud-flow-agent

# 编译所有 eBPF 程序 + Go 二进制
make

# 仅编译 eBPF 程序
make bpf

# 仅编译 Go 程序（快速迭代，eBPF已编译）
make build-go
```

### 手动编译（逐个文件）

```bash
cd cloud-flow-agent

# DNS 协议解析
clang -O2 -target bpf -c internal/ebpfcollector/bpf/dns_full_bpf.bpf.c \
  -o internal/ebpfcollector/bpf/dns_full_bpf.bpf.o

# HTTP 完整报文捕获
clang -O2 -target bpf -c internal/ebpfcollector/bpf/http_full_bpf.bpf.c \
  -o internal/ebpfcollector/bpf/http_full_bpf.bpf.o

# HTTP 请求/响应指标统计
clang -O2 -target bpf -c internal/ebpfcollector/bpf/http_metrics_bpf.bpf.c \
  -o internal/ebpfcollector/bpf/http_metrics_bpf.bpf.o

# MySQL 协议 SQL 解析
clang -O2 -target bpf -c internal/ebpfcollector/bpf/mysql_full_bpf.bpf.c \
  -o internal/ebpfcollector/bpf/mysql_full_bpf.bpf.o

# TC 流量控制入口
clang -O2 -target bpf -c internal/ebpfcollector/bpf/tc_bpf.bpf.c \
  -o internal/ebpfcollector/bpf/tc_bpf.bpf.o

# TCP RTT/重传/丢包指标
clang -O2 -target bpf -c internal/ebpfcollector/bpf/tcp_metrics_bpf.bpf.c \
  -o internal/ebpfcollector/bpf/tcp_metrics_bpf.bpf.o
```

---

## 验证编译结果

### 检查文件大小（必须 > 0 字节）

```bash
cd cloud-flow-agent
ls -lh internal/ebpfcollector/bpf/*.bpf.o
```

**正确输出示例：**
```
-rw-r--r-- 1 root root  15K Jun 15 10:00 dns_full_bpf.bpf.o
-rw-r--r-- 1 root root  22K Jun 15 10:00 http_full_bpf.bpf.o
-rw-r--r-- 1 root root  18K Jun 15 10:00 http_metrics_bpf.bpf.o
-rw-r--r-- 1 root root  12K Jun 15 10:00 mysql_full_bpf.bpf.o
-rw-r--r-- 1 root root  8.2K Jun 15 10:00 tc_bpf.bpf.o
-rw-r--r-- 1 root root  14K Jun 15 10:00 tcp_metrics_bpf.bpf.o
```

**❌ 错误情况（0字节）：**
```
-rw-r--r-- 1 root root    0 Jun 15 10:00 dns_full_bpf.bpf.o
```
→ 需要重新编译

### 验证 BPF 字节码有效性

```bash
# 使用 bpftool 验证
bpftool prog load internal/ebpfcollector/bpf/tc_bpf.bpf.o /dev/null

# 使用 llvm-objdump 反汇编检查
llvm-objdump -h internal/ebpfcollector/bpf/tc_bpf.bpf.o
```

### 验证 Go embed 加载

```bash
cd cloud-flow-agent
go build ./...
```

**成功：** 无错误输出

**失败：**
```
pattern dns_full.bpf.o: no matching files found
```
→ 检查 .bpf.o 文件是否存在且 > 0 字节

---

## CO-RE (Compile Once - Run Everywhere) 支持

### 什么是 CO-RE？

CO-RE 允许编译一次的 eBPF 程序在不同内核版本上运行，无需重新编译。

### CO-RE 编译要求

1. **内核版本 >= 5.4**（推荐 5.10+）
2. **BTF 支持**：`/sys/kernel/btf/vmlinux` 存在
3. **libbpf >= 0.6**

### 检查 BTF 支持

```bash
# 检查内核是否开启 BTF
ls -la /sys/kernel/btf/vmlinux

# 检查内核配置
zcat /proc/config.gz | grep BTF
```

### CO-RE 编译命令

```bash
# 带 vmlinux.h 的 CO-RE 编译（推荐）
bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h

clang -O2 -target bpf -D__TARGET_ARCH_x86 \
  -I. \
  -c program.bpf.c -o program.bpf.o
```

---

## 常见编译错误与解决

### 错误 1：`linux/bpf.h: No such file or directory`

**原因：** 缺少内核头文件或 libbpf

**解决：**
```bash
# CentOS
yum install -y kernel-devel libbpf-devel

# Ubuntu
apt-get install -y linux-headers-$(uname -r) libbpf-dev
```

### 错误 2：`unknown type name '__u64'`

**原因：** 缺少内核类型定义

**解决：** 在 .bpf.c 文件顶部添加：
```c
#include <linux/types.h>
#include <linux/bpf.h>
```

### 错误 3：`section is not recognized`

**原因：** clang 版本过低，不支持 BPF target

**解决：** 升级 clang 到 10+
```bash
# Ubuntu
apt-get install -y clang-12
export CLANG=clang-12
```

### 错误 4：`invalid relo for insn`

**原因：** 访问的结构体字段在内核中不存在或偏移不匹配

**解决：**
1. 使用 CO-RE + BTF
2. 或使用 `bpf_core_read()` 辅助函数

### 错误 5：`Go embed: no matching files`

**原因：** .bpf.o 文件是 0 字节或不存在

**解决：**
```bash
# 重新编译
make clean && make bpf

# 验证文件大小
ls -lh *.bpf.o
```

---

## 交叉编译

### x86_64 → ARM64

```bash
# 安装交叉编译工具链
apt-get install -y gcc-aarch64-linux-gnu

# 编译 ARM64 版本
make build-arm64
```

### ARM64 → x86_64

```bash
make build-x86_64
```

---

## 发布版本编译

### 静态链接（推荐生产环境）

```bash
# x86_64 静态链接
make release-x86_64

# ARM64 静态链接
make release-arm64
```

### 验证静态链接

```bash
file cloud-flow-agent
# 输出: ELF 64-bit LSB executable, x86-64, statically linked

ldd cloud-flow-agent
# 输出: not a dynamic executable
```

---

## 编译环境 Docker 镜像

### 使用预配置的编译环境

```dockerfile
# Dockerfile.ebpf-builder
FROM golang:1.22-bookworm

RUN apt-get update && apt-get install -y \
    clang-15 llvm-15 \
    libbpf-dev \
    linux-headers-amd64 \
    bpftool \
    && rm -rf /var/lib/apt/lists/*

RUN update-alternatives --install /usr/bin/clang clang /usr/bin/clang-15 100
RUN update-alternatives --install /usr/bin/llc llc /usr/bin/llc-15 100

WORKDIR /workspace
```

```bash
# 构建镜像
docker build -f Dockerfile.ebpf-builder -t cloudflow/ebpf-builder .

# 编译
docker run --rm -v $(pwd):/workspace cloudflow/ebpf-builder make
```

---

## 编译检查清单

发布前必须验证：

- [ ] 所有 6 个 .bpf.o 文件 > 0 字节
- [ ] `go build ./...` 无错误
- [ ] `go vet ./...` 无警告
- [ ] `bpftool prog load` 验证通过
- [ ] 静态链接二进制可执行
- [ ] ARM64/x86_64 交叉编译通过

---

## 下一步

编译完成后，参考 [EBPF_AGENT_DEPLOYMENT.md](./EBPF_AGENT_DEPLOYMENT.md) 进行部署。
