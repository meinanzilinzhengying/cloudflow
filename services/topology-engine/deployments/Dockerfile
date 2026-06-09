# =============================================================================
# CloudFlow topology-engine - Dockerfile
# =============================================================================
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git sed
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build

# Copy entire project
COPY . .

# Fix replace directives in go.mod for Docker build environment
WORKDIR /build/services/topology-engine
RUN sed -i 's|../../../pkg|/build/pkg|g' go.mod && \
    sed -i 's|../../../proto|/build/proto|g' go.mod && \
    sed -i 's|../../../shared|/build/services/shared|g' go.mod && \
    sed -i 's|../../../shared/resilience|/build/services/shared/resilience|g' go.mod && \
    sed -i 's|../../../shared/auth|/build/services/shared/auth|g' go.mod

# Run go mod tidy to resolve all dependencies
ENV GOWORK=off
ENV GOPROXY=off

    RUN go mod tidy
    

    RUN go build -o /app/main ./

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /build/topology-engine ./cmd/main.go

# Final stage
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /build/topology-engine .

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:8008/healthz || exit 1

EXPOSE 8008
ENTRYPOINT ["./topology-engine"]
