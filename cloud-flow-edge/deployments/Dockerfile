# 多阶段构建 - 边缘节点服务
# 第一阶段：编译
FROM golang:1.22-alpine AS builder

# 安装必要工具
RUN apk add --no-cache git

# 配置国内 Go 代理
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /build

# 先复制依赖文件，利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /build/edge-node ./cmd/main.go

# 第二阶段：运行
FROM alpine:3.20

# 安装 CA 证书（用于 TLS 连接）
RUN apk add --no-cache ca-certificates tzdata

# 设置时区
ENV TZ=Asia/Shanghai

WORKDIR /app

# 从编译阶段复制二进制
COPY --from=builder /build/edge-node .

# 复制配置文件
COPY configs/ ./configs/

# 非 root 用户运行
RUN adduser -D -u 1000 appuser
USER appuser

# gRPC 端口 + Prometheus metrics 端口
EXPOSE 9091 9092

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:9092/metrics || exit 1

ENTRYPOINT ["./edge-node"]
