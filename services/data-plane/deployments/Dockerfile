# =============================================================================
# CloudFlow data-plane - Dockerfile (Debian slim + prebuilt binary)
# =============================================================================
FROM docker.m.daocloud.io/library/debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata curl && rm -rf /var/lib/apt/lists/*
ENV TZ=Asia/Shanghai
WORKDIR /app

# Copy prebuilt binary from host
COPY data-plane .

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3     CMD curl -f http://localhost:9102/health || exit 1

EXPOSE 9002 9102
ENTRYPOINT ["./data-plane", "-grpc-addr=:9002", "-metrics-addr=:9102"]
