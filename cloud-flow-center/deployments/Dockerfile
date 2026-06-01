FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build
COPY go.mod go.sum ./
COPY proto ../proto
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /build/cloud-flow-center ./cmd/main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /build/cloud-flow-center .
COPY configs/ ./configs/
RUN adduser -D -u 1000 appuser
USER appuser
EXPOSE 9090
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:9191/metrics || exit 1
ENTRYPOINT ["./cloud-flow-center"]
