FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git sed
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build

# Copy entire project
COPY . .

# Fix replace directives in go.mod for Docker build environment
WORKDIR /build/cloud-flow-center
RUN sed -i 's|../pkg|/build/pkg|g' go.mod && \
    sed -i 's|../proto|/build/proto|g' go.mod

# Run go mod tidy to resolve all dependencies
ENV GOWORK=off
ENV GOPROXY=off

    RUN go mod tidy
    

    RUN go build -o /app/main ./

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /build/cloud-flow-center ./cmd/main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /build/cloud-flow-center .
COPY --from=builder /build/cloud-flow-center/configs/ ./configs/
RUN adduser -D -u 1000 appuser
USER appuser
EXPOSE 9090
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:9191/metrics || exit 1
ENTRYPOINT ["./cloud-flow-center"]
