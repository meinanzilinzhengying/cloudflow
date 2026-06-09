# =============================================================================
# 云内流量监测系统 (Cloud Flow Agent) - 优化版多阶段构建 Dockerfile
# 优化点:
#   - BuildKit 缓存挂载加速 go mod download
#   - 支持 CGO 启用/禁用（eBPF 需要 CGO_ENABLED=1）
#   - 使用 alpine:3.20 作为生产镜像（提供必要的基础库）
#   - 多架构支持 (amd64/arm64)
#   - eBPF 功能需要 root 权限和 CAP_SYS_ADMIN/CAP_BPF capabilities
# =============================================================================

# syntax=docker/dockerfile:1.7

# ---------------------------------------------------------------------------
# Stage 1: 构建阶段 - 支持多架构
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /build

# 安装构建依赖
RUN apk add --no-cache git clang llvm make sed

# 设置 GOPROXY
ENV GOPROXY=https://goproxy.cn,direct

# 复制整个项目
COPY . .

# Fix replace directives in go.mod for Docker build environment
WORKDIR /build/cloud-flow-agent
RUN sed -i 's|../pkg|/build/pkg|g' go.mod && \
    sed -i 's|../proto|/build/proto|g' go.mod

# 构建参数
# 注意：如果启用 eBPF 功能，需要设置 CGO_ENABLED=1
# 如果只需要指标采集（无 eBPF），可以设置 CGO_ENABLED=0 以减小镜像体积
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG CGO_ENABLED=1

# 下载依赖并构建
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy && \
    CGO_ENABLED=${CGO_ENABLED} GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -extldflags '-static'" \
    -a -installsuffix cgo \
    -o /build/cloud-flow-agent ./cmd/main.go

# 验证二进制文件
RUN ls -lh /build/cloud-flow-agent && \
    (file /build/cloud-flow-agent 2>/dev/null || true)

# ---------------------------------------------------------------------------
# Stage 3: 生产阶段 - 使用最小化镜像
# ---------------------------------------------------------------------------
FROM alpine:3.20 AS production

# 安装必要依赖
RUN apk add --no-cache ca-certificates tzdata curl && \
    rm -rf /var/cache/apk/* /tmp/*

ENV TZ=Asia/Shanghai

WORKDIR /app

# 复制二进制文件
COPY --from=builder /build/cloud-flow-agent .

# 复制配置文件
COPY --from=builder /build/cloud-flow-agent/configs/ ./configs/

# 创建非特权用户 (eBPF 需要特权，但其他操作可以用普通用户)
RUN addgroup -g 1000 -S agent && \
    adduser -u 1000 -S agent -G agent && \
    chown -R agent:agent /app

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# eBPF 采集器需要 root 权限和以下 capabilities:
# - CAP_SYS_ADMIN: 加载 eBPF 程序
# - CAP_BPF: BPF 相关操作
# - CAP_NET_ADMIN: 网络监控
# - CAP_PERFMON: 性能监控
# 在生产环境中，建议使用 Linux capabilities 而不是全 root 权限
USER root

ENTRYPOINT ["./cloud-flow-agent"]
