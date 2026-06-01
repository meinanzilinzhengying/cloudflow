# =============================================================================
# 云内流量监测系统 (Cloud Flow Agent) - 优化版多阶段构建 Dockerfile
# 优化点:
#   - BuildKit 缓存挂载加速 go mod download
#   - 静态链接 CGO 禁用
#   - 使用 scratch 最小化镜像
#   - 多架构支持 (amd64/arm64)
# =============================================================================

# syntax=docker/dockerfile:1.7

# ---------------------------------------------------------------------------
# Stage 1: 依赖下载 - 利用缓存层
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS deps

WORKDIR /build

# 安装 git 用于 go mod download
RUN apk add --no-cache git

# 设置 GOPROXY
ENV GOPROXY=https://goproxy.cn,direct

# 复制依赖文件
COPY go.mod go.sum ./
COPY proto ../proto

# 利用 BuildKit 缓存挂载加速 go mod download
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download

# ---------------------------------------------------------------------------
# Stage 2: 构建阶段 - 支持多架构
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

WORKDIR /build

# 安装构建依赖
RUN apk add --no-cache git clang llvm make

# 从 deps 阶段复制缓存
COPY --from=deps /go/pkg/mod /go/pkg/mod

# 复制源码
COPY . .
COPY proto ../proto

# 构建参数
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG CGO_ENABLED=0

# 静态编译，去除符号表和调试信息
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=${CGO_ENABLED} GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -extldflags '-static'" \
    -a -installsuffix cgo \
    -o /build/cloud-flow-agent ./cmd/main.go

# 验证二进制文件
RUN ls -lh /build/cloud-flow-agent && \
    file /build/cloud-flow-agent

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
COPY configs/ ./configs/

# 创建非特权用户 (eBPF 需要特权，但其他操作可以用普通用户)
RUN addgroup -g 1000 -S agent && \
    adduser -u 1000 -S agent -G agent && \
    chown -R agent:agent /app

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# eBPF 需要特权模式运行
USER root

ENTRYPOINT ["./cloud-flow-agent"]
