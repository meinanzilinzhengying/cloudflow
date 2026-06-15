# CloudFlow eBPF Agent Dockerfile
# 多阶段构建：编译 eBPF 程序 + Go 二进制

# ============================================
# 阶段 1: eBPF 程序编译
# ============================================
FROM ubuntu:22.04 AS ebpf-builder

WORKDIR /build

# 安装编译依赖
RUN apt-get update && apt-get install -y \
    clang \
    llvm \
    libbpf-dev \
    linux-headers-generic \
    libelf-dev \
    zlib1g-dev \
    make \
    && rm -rf /var/lib/apt/lists/*

# 复制 eBPF 源文件
COPY internal/ebpfcollector/bpf/*.bpf.c ./bpf/
COPY internal/ebpfcollector/bpf/*.h ./bpf/
COPY Makefile ./

# 编译 eBPF 程序
RUN make ebpf

# ============================================
# 阶段 2: Go 二进制编译
# ============================================
FROM golang:1.22-bookworm AS go-builder

WORKDIR /build

# 安装系统依赖
RUN apt-get update && apt-get install -y \
    libbpf-dev \
    libelf-dev \
    zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

# 复制 Go 模块文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 从 ebpf-builder 阶段复制编译好的 eBPF 对象文件
COPY --from=ebpf-builder /build/bpf/*.bpf.o ./internal/ebpfcollector/bpf/

# 编译 Go 二进制（启用 CGO 以支持 libbpf）
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-s -w" \
    -tags "linux ebpf cgo" \
    -o cloud-flow-agent \
    ./cmd/main.go

# ============================================
# 阶段 3: 运行时镜像
# ============================================
FROM ubuntu:22.04

LABEL maintainer="CloudFlow Team"
LABEL description="CloudFlow eBPF Network Traffic Analysis Agent"

WORKDIR /app

# 安装运行时依赖
RUN apt-get update && apt-get install -y \
    libbpf1 \
    libelf1 \
    zlib1g \
    iproute2 \
    ethtool \
    && rm -rf /var/lib/apt/lists/*

# 从 go-builder 阶段复制二进制
COPY --from=go-builder /build/cloud-flow-agent .

# 复制配置文件
COPY configs/config.yaml ./configs/

# eBPF 需要特权模式
# 运行时需要挂载:
#   - /sys/fs/bpf (BPF 文件系统)
#   - /lib/modules (内核头文件)
#   - /usr/src (内核源码)

# 暴露指标端口
EXPOSE 9090

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:9090/healthz || exit 1

# 入口点
ENTRYPOINT ["/app/cloud-flow-agent"]

# 默认参数
CMD ["--config", "/app/configs/config.yaml"]
