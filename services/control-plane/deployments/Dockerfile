# =============================================================================
# CloudFlow control-plane - Dockerfile
# =============================================================================
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git sed
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build

# Copy entire project
COPY . .

# Add replace directives for local modules in Docker build environment
WORKDIR /build/services/control-plane
RUN echo '' >> go.mod && \
    echo 'replace (' >> go.mod && \
    echo '    github.com/meinanzilinzhengying/cloudflow/pkg => /build/pkg' >> go.mod && \
    echo '    github.com/meinanzilinzhengying/cloudflow/proto => /build/proto' >> go.mod && \
    echo '    github.com/meinanzilinzhengying/cloudflow/services/proto => /build/services/proto' >> go.mod && \
    echo '    github.com/meinanzilinzhengying/cloudflow/services/shared => /build/services/shared' >> go.mod && \
    echo '    github.com/meinanzilinzhengying/cloudflow/services/shared/resilience => /build/services/shared/resilience' >> go.mod && \
    echo '    github.com/meinanzilinzhengying/cloudflow/services/shared/auth => /build/services/shared/auth' >> go.mod && \
    echo ')' >> go.mod

# Run go mod tidy to resolve all dependencies
ENV GOWORK=off
ENV GONOSUMCHECK=github.com/meinanzilinzhengying/cloudflow/*
ENV GONOSUMDB=github.com/meinanzilinzhengying/cloudflow/*

    RUN go mod tidy
    
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /build/control-plane ./cmd/main.go

# Final stage
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl docker-cli
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /build/control-plane .

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:8001/healthz || exit 1

EXPOSE 8001
ENTRYPOINT ["./control-plane"]
