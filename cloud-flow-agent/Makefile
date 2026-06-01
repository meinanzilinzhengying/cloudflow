# Cloud Flow Agent - 跨架构 eBPF 编译 Makefile
# 支持: x86_64 (内核>=3.10), ARM64 (内核>=4.19.90-24)
# 芯片: Intel, AMD, 海光C86, 鲲鹏920

# 工具链配置
CLANG ?= clang
LLC ?= llc
ARCH ?= $(shell uname -m)
BPFTOOL ?= bpftool

# 内核头文件路径
KERN_HEADERS ?= /usr/src/kernels/$(shell uname -r)

# 目标架构映射
ifeq ($(ARCH),x86_64)
  BPF_TARGET = x86
  BPF_ARCH_CFLAGS = -D__TARGET_ARCH_x86
else ifeq ($(ARCH),aarch64)
  BPF_TARGET = arm64
  BPF_ARCH_CFLAGS = -D__TARGET_ARCH_arm64
endif

# 通用编译选项
BPF_CFLAGS = -g -O2 -target bpf \
  -D__BPF_TRACING__ \
  $(BPF_ARCH_CFLAGS) \
  -I$(KERN_HEADERS)/include \
  -I$(KERN_HEADERS)/arch/$(BPF_TARGET)/include \
  -I./internal/ebpfcollector/bpf \
  -Wall

# BPF 源文件
BPF_SRCS = $(wildcard internal/ebpfcollector/bpf/*.bpf.c)
BPF_OBJS = $(patsubst %.bpf.c, %.bpf.o, $(BPF_SRCS))

# Go 编译选项
GOFLAGS = -v
CGO_ENABLED = 1
ifeq ($(ARCH),aarch64)
  GOARCH = arm64
else
  GOARCH = amd64
endif

# 目标
.PHONY: all bpf build clean help

all: bpf build

# 编译所有 BPF 程序
bpf: $(BPF_OBJS)

%.bpf.o: %.bpf.c
	@echo "BPF CC $< -> $@"
	@$(CLANG) $(BPF_CFLAGS) -c $< -o $@

# 编译 Go 程序
build: bpf
	@echo "Building agent for $(ARCH)..."
	CGO_ENABLED=$(CGO_ENABLED) GOARCH=$(GOARCH) go build $(GOFLAGS) -o cloud-flow-agent ./cmd/

# 交叉编译 ARM64
build-arm64:
	$(MAKE) ARCH=aarch64 build

# 交叉编译 x86_64
build-x86_64:
	$(MAKE) ARCH=x86_64 build

# 清理
clean:
	rm -f $(BPF_OBJS)
	rm -f cloud-flow-agent

# 帮助
help:
	@echo "Cloud Flow Agent - 跨架构编译"
	@echo ""
	@echo "用法:"
	@echo "  make              - 编译当前架构"
	@echo "  make build-arm64  - 交叉编译 ARM64"
	@echo "  make build-x86_64 - 交叉编译 x86_64"
	@echo "  make bpf          - 仅编译 BPF 程序"
	@echo "  make clean        - 清理"
	@echo ""
	@echo "变量:"
	@echo "  CLANG=...         - 指定 clang 路径"
	@echo "  KERN_HEADERS=...  - 指定内核头文件路径"
	@echo "  ARCH=...          - 目标架构 (x86_64/aarch64)"
