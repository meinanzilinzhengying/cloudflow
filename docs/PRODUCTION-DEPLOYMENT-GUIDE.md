# CloudFlow Production Deployment Guide

> **Version:** 1.0  
> **Last Updated:** 2024-06-12  
> **Target Audience:** SRE, DevOps, Platform Engineers  
> **Applies to:** CloudFlow v1.x

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Step-by-Step Deployment Instructions](#step-by-step-deployment-instructions)
- [Environment Configuration Reference](#environment-configuration-reference)
- [Troubleshooting Guide](#troubleshooting-guide)
- [Monitoring and Alerting Setup](#monitoring-and-alerting-setup)

---

## Prerequisites

### Hardware Requirements

| Component | Minimum (PoC) | Recommended (Production) | Notes |
|---|---|---|---|
| **Control Plane Nodes** | 2 vCPU, 4 GB RAM | 4 vCPU, 8 GB RAM | 2+ nodes for HA |
| **Data Plane Nodes** | 4 vCPU, 8 GB RAM | 8 vCPU, 16 GB RAM | Scales with ingestion volume |
| **MySQL** | 2 vCPU, 4 GB RAM | 4 vCPU, 16 GB RAM | SSD storage required |
| **ClickHouse** | 4 vCPU, 8 GB RAM | 8 vCPU, 32 GB RAM | NVMe SSD for hot data |
| **Kafka** | 4 vCPU, 8 GB RAM | 8 vCPU, 16 GB RAM | 3 brokers for HA |
| **Redis** | 2 vCPU, 4 GB RAM | 4 vCPU, 8 GB RAM | Sentinel or Cluster mode |
| **etcd** | 2 vCPU, 4 GB RAM | 4 vCPU, 8 GB RAM | 3+ members for quorum |
| **Prometheus + Grafana** | 2 vCPU, 4 GB RAM | 4 vCPU, 16 GB RAM | Long-term storage needs more disk |

### Software Requirements

| Software | Version | Purpose |
|---|---|---|
| Docker | 24.0+ | Container runtime |
| Docker Compose | 2.20+ | Local / single-node orchestration |
| Go | 1.21+ | Building services from source |
| Kubernetes | 1.28+ | Recommended for production orchestration |
| Helm | 3.12+ | Kubernetes package management |
| kubectl | 1.28+ | Kubernetes CLI |
| OpenSSL | 3.0+ | TLS certificate generation |
| Terraform | 1.6+ | Infrastructure as Code (optional) |

### Network Requirements

| Port | Service | Direction | Notes |
|---|---|---|---|
| 80 | nginx | Inbound | HTTP traffic (redirect to 443) |
| 443 | nginx | Inbound | HTTPS traffic |
| 3306 | MySQL | Internal | Restrict to application subnet |
| 8123, 9000 | ClickHouse | Internal | HTTP and native ports |
| 9092, 29092 | Kafka | Internal | PLAINTEXT and PLAINTEXT_HOST |
| 2181 | Zookeeper | Internal | Kafka coordination |
| 6379 | Redis | Internal | Caching and leader election |
| 2379, 2380 | etcd | Internal | Client and peer ports |
| 9090 | Prometheus | Internal / VPN | Metrics UI (do not expose publicly) |
| 3000 | Grafana | Internal / VPN | Dashboards (proxy via nginx if public) |
| 8001-8006 | CloudFlow Services | Internal | HTTP API ports |
| 9001-9006 | CloudFlow Services | Internal | gRPC ports |
| 9101-9106 | CloudFlow Services | Internal | Prometheus metrics endpoints |

### Pre-Deployment Checklist

Before proceeding, ensure you have completed the [Production Readiness Checklist](PRODUCTION-READINESS-CHECKLIST.md) **Pre-Deployment** section.

---

## Step-by-Step Deployment Instructions

### Step 1: Prepare the Environment

```bash
# 1.1 Create project directory
mkdir -p /opt/cloudflow/{config,scripts,logs,backups}
cd /opt/cloudflow

# 1.2 Set environment variables
export CF_ENV=production
export CF_DOMAIN=cloudflow.example.com
export CF_VERSION=v1.2.3

# 1.3 Verify Docker and Docker Compose
docker --version
docker-compose --version

# 1.4 Verify network connectivity
ping -c 3 8.8.8.8
nc -zv mysql.internal 3306
nc -zv clickhouse.internal 9000
```

### Step 2: Configure Secrets

```bash
# 2.1 Create secrets directory (ensure permissions are 600)
mkdir -p /opt/cloudflow/secrets
chmod 700 /opt/cloudflow/secrets

# 2.2 Generate strong random passwords
openssl rand -base64 32 > /opt/cloudflow/secrets/mysql_root_password
openssl rand -base64 32 > /opt/cloudflow/secrets/mysql_app_password
openssl rand -base64 32 > /opt/cloudflow/secrets/clickhouse_password
openssl rand -base64 32 > /opt/cloudflow/secrets/redis_password
openssl rand -base64 64 > /opt/cloudflow/secrets/jwt_secret
openssl rand -base64 32 > /opt/cloudflow/secrets/grafana_admin_password

# 2.3 Generate TLS certificates (or use your organization's CA)
mkdir -p /opt/cloudflow/config/nginx/ssl
openssl req -x509 -nodes -days 365 -newkey rsa:4096 \
  -keyout /opt/cloudflow/config/nginx/ssl/cloudflow.key \
  -out /opt/cloudflow/config/nginx/ssl/cloudflow.crt \
  -subj "/C=US/ST=California/L=San Francisco/O=CloudFlow/OU=Platform/CN=${CF_DOMAIN}"

# 2.4 Set restrictive permissions on secrets
chmod 600 /opt/cloudflow/secrets/*
chmod 600 /opt/cloudflow/config/nginx/ssl/*
```

### Step 3: Deploy Infrastructure Dependencies

```bash
# 3.1 Deploy MySQL (or use managed RDS / Cloud SQL)
# Example using Docker Compose for a single-node deployment (not HA)
docker-compose -f docker-compose.staging.yml up -d mysql

# 3.2 Verify MySQL is healthy
# Wait for health check to pass
sleep 30
docker-compose -f docker-compose.staging.yml ps mysql

# 3.3 Initialize databases (run migrations)
# If using a managed MySQL, connect via mysql client:
mysql -h mysql.internal -u root -p < /opt/cloudflow/scripts/init-databases.sql

# 3.4 Deploy ClickHouse
docker-compose -f docker-compose.staging.yml up -d clickhouse

# 3.5 Deploy Kafka and Zookeeper
docker-compose -f docker-compose.staging.yml up -d zookeeper kafka

# 3.6 Create Kafka topics
# Wait for Kafka to be healthy, then:
docker-compose -f docker-compose.staging.yml exec kafka kafka-topics \
  --bootstrap-server localhost:9092 \
  --create --topic metrics.ingestion --partitions 12 --replication-factor 3 || true

docker-compose -f docker-compose.staging.yml exec kafka kafka-topics \
  --bootstrap-server localhost:9092 \
  --create --topic alerts.notifications --partitions 6 --replication-factor 3 || true

# 3.7 Deploy Redis and etcd
docker-compose -f docker-compose.staging.yml up -d redis etcd

# 3.8 Verify all infrastructure is healthy
docker-compose -f docker-compose.staging.yml ps
```

### Step 4: Deploy CloudFlow Services

```bash
# 4.1 Build or pull service images
# If building from source:
export GOWORK=/opt/cloudflow/go.work
export GOROOT=/usr/local/go
export PATH=/usr/local/go/bin:$PATH

# Build all services
make build
# OR build individually:
go build -o bin/auth-service ./cmd/auth-engine
# ... repeat for each service

# 4.2 Deploy services in dependency order
docker-compose -f docker-compose.staging.yml up -d auth-service
docker-compose -f docker-compose.staging.yml up -d tenant-service
docker-compose -f docker-compose.staging.yml up -d control-plane
docker-compose -f docker-compose.staging.yml up -d data-plane
docker-compose -f docker-compose.staging.yml up -d query-service
docker-compose -f docker-compose.staging.yml up -d alert-engine

# 4.3 Verify all services are healthy
docker-compose -f docker-compose.staging.yml ps
# Wait for all services to report healthy
```

### Step 5: Deploy Reverse Proxy

```bash
# 5.1 Create nginx configuration
cat > /opt/cloudflow/config/nginx/nginx.conf << 'EOF'
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 4096;
    use epoll;
    multi_accept on;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for" '
                    'rt=$request_time uct="$upstream_connect_time" '
                    'uht="$upstream_header_time" urt="$upstream_response_time"';

    access_log /var/log/nginx/access.log main;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    client_max_body_size 50M;

    # Rate limiting zones
    limit_req_zone $binary_remote_addr zone=general:10m rate=100r/s;
    limit_req_zone $binary_remote_addr zone=auth:10m rate=10r/s;
    limit_conn_zone $binary_remote_addr zone=addr:10m;

    # Upstream services
    upstream auth_service {
        server auth-service:8001;
        keepalive 32;
    }
    upstream tenant_service {
        server tenant-service:8002;
        keepalive 32;
    }
    upstream control_plane {
        server control-plane:8003;
        keepalive 32;
    }
    upstream data_plane {
        server data-plane:8004;
        keepalive 32;
    }
    upstream query_service {
        server query-service:8005;
        keepalive 32;
    }
    upstream alert_engine {
        server alert-engine:8006;
        keepalive 32;
    }
    upstream grafana {
        server grafana:3000;
        keepalive 32;
    }

    # HTTPS server
    server {
        listen 443 ssl http2;
        server_name cloudflow.example.com;

        ssl_certificate /etc/nginx/ssl/cloudflow.crt;
        ssl_certificate_key /etc/nginx/ssl/cloudflow.key;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;
        ssl_prefer_server_ciphers on;
        ssl_session_cache shared:SSL:10m;
        ssl_session_timeout 10m;

        # Security headers
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-XSS-Protection "1; mode=block" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;
        add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline';" always;

        # Health check endpoint
        location /health {
            access_log off;
            return 200 "healthy\n";
            add_header Content-Type text/plain;
        }

        # Auth service
        location /api/v1/auth/ {
            limit_req zone=auth burst=20 nodelay;
            proxy_pass http://auth_service/api/v1/auth/;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_connect_timeout 5s;
            proxy_send_timeout 10s;
            proxy_read_timeout 10s;
        }

        # Tenant service
        location /api/v1/tenants/ {
            limit_req zone=general burst=50 nodelay;
            proxy_pass http://tenant_service/api/v1/tenants/;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        # Control plane
        location /api/v1/control/ {
            limit_req zone=general burst=50 nodelay;
            proxy_pass http://control_plane/api/v1/control/;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        # Data plane
        location /api/v1/data/ {
            limit_req zone=general burst=100 nodelay;
            proxy_pass http://data_plane/api/v1/data/;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            client_max_body_size 50M;
        }

        # Query service
        location /api/v1/query/ {
            limit_req zone=general burst=50 nodelay;
            proxy_pass http://query_service/api/v1/query/;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_connect_timeout 5s;
            proxy_send_timeout 30s;
            proxy_read_timeout 30s;
        }

        # Alert engine
        location /api/v1/alerts/ {
            limit_req zone=general burst=50 nodelay;
            proxy_pass http://alert_engine/api/v1/alerts/;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        # Grafana (optional, behind auth)
        location /grafana/ {
            proxy_pass http://grafana/;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header X-Forwarded-Host $host;
            proxy_set_header X-Forwarded-Server $host;
        }
    }

    # HTTP redirect to HTTPS
    server {
        listen 80;
        server_name cloudflow.example.com;
        return 301 https://$server_name$request_uri;
    }
}
EOF

# 5.2 Deploy nginx
docker-compose -f docker-compose.staging.yml up -d nginx

# 5.3 Test nginx configuration
docker-compose -f docker-compose.staging.yml exec nginx nginx -t
```

### Step 6: Deploy Monitoring Stack

```bash
# 6.1 Create Prometheus configuration
cat > /opt/cloudflow/config/prometheus/prometheus.yml << 'EOF'
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: cloudflow-production
    replica: '{{.ExternalURL}}'

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']

rule_files:
  - /etc/prometheus/rules/*.yml

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  - job_name: 'cloudflow-services'
    static_configs:
      - targets:
          - 'auth-service:9101'
          - 'tenant-service:9102'
          - 'control-plane:9103'
          - 'data-plane:9104'
          - 'query-service:9105'
          - 'alert-engine:9106'
    metrics_path: /metrics
    relabel_configs:
      - source_labels: [__address__]
        target_label: instance
      - source_labels: [__address__]
        regex: '([^:]+):.*'
        target_label: service
        replacement: '${1}'

  - job_name: 'mysql-exporter'
    static_configs:
      - targets: ['mysql-exporter:9104']

  - job_name: 'redis-exporter'
    static_configs:
      - targets: ['redis-exporter:9121']

  - job_name: 'kafka-exporter'
    static_configs:
      - targets: ['kafka-exporter:9308']

  - job_name: 'node-exporter'
    static_configs:
      - targets: ['node-exporter:9100']
EOF

# 6.2 Deploy Prometheus and Grafana
docker-compose -f docker-compose.staging.yml up -d prometheus grafana

# 6.3 Verify Prometheus targets
curl -s http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'

# 6.4 Import Grafana dashboards
# Place dashboard JSON files in ./config/grafana/dashboards/
# and datasource config in ./config/grafana/datasources/
```

### Step 7: Validate Deployment

```bash
# 7.1 Run health checks
curl -f http://localhost:8001/health || echo "auth-service unhealthy"
curl -f http://localhost:8002/health || echo "tenant-service unhealthy"
curl -f http://localhost:8003/health || echo "control-plane unhealthy"
curl -f http://localhost:8004/health || echo "data-plane unhealthy"
curl -f http://localhost:8005/health || echo "query-service unhealthy"
curl -f http://localhost:8006/health || echo "alert-engine unhealthy"
curl -f http://localhost/health || echo "nginx unhealthy"

# 7.2 Run load test
go run scripts/load-test.go -target http://localhost:80 -concurrency 50 -duration 2m

# 7.3 Verify end-to-end flow
# 1. Login
curl -X POST http://localhost/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"changeme"}'

# 2. Create tenant
curl -X POST http://localhost/api/v1/tenants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"name":"production-test","plan":"enterprise"}'

# 3. Ingest test metrics
curl -X POST http://localhost/api/v1/data/ingest \
  -H "Content-Type: application/json" \
  -d '{"metrics":[{"metric":"cpu_usage","value":45.2,"timestamp":'$(date +%s)',"labels":{"instance":"test-01"}}]}'

# 4. Query metrics
curl "http://localhost/api/v1/query/metrics?metric=cpu_usage&start=-1h"

# 7.4 Sign off
echo "Deployment validation complete. Proceed to monitoring checklist."
```

---

## Environment Configuration Reference

### Common Environment Variables

| Variable | Service | Default | Description |
|---|---|---|---|
| `CF_ENV` | All | `staging` | Environment name (staging, production) |
| `CF_HTTP_PORT` | All | varies | HTTP API port |
| `CF_GRPC_PORT` | All | varies | gRPC port |
| `CF_LOG_LEVEL` | All | `info` | Log level (debug, info, warn, error) |
| `CF_METRICS_PORT` | All | `910x` | Prometheus metrics port |
| `CF_DB_HOST` | MySQL-dependent | `mysql` | MySQL hostname |
| `CF_DB_PORT` | MySQL-dependent | `3306` | MySQL port |
| `CF_DB_USER` | MySQL-dependent | `cloudflow` | MySQL username |
| `CF_DB_PASSWORD` | MySQL-dependent | — | MySQL password (secret) |
| `CF_DB_NAME` | MySQL-dependent | varies | MySQL database name |
| `CF_CLICKHOUSE_HOST` | CH-dependent | `clickhouse` | ClickHouse hostname |
| `CF_CLICKHOUSE_PORT` | CH-dependent | `9000` | ClickHouse native port |
| `CF_CLICKHOUSE_DB` | CH-dependent | `cloudflow` | ClickHouse database |
| `CF_CLICKHOUSE_USER` | CH-dependent | `cloudflow` | ClickHouse username |
| `CF_CLICKHOUSE_PASSWORD` | CH-dependent | — | ClickHouse password (secret) |
| `CF_REDIS_HOST` | Redis-dependent | `redis` | Redis hostname |
| `CF_REDIS_PORT` | Redis-dependent | `6379` | Redis port |
| `CF_REDIS_PASSWORD` | Redis-dependent | `''` | Redis password (secret) |
| `CF_ETCD_ENDPOINTS` | etcd-dependent | `etcd:2379` | Comma-separated etcd endpoints |
| `CF_KAFKA_BROKERS` | Kafka-dependent | `kafka:9092` | Comma-separated Kafka brokers |
| `CF_JWT_SECRET` | auth-service | — | JWT signing secret (secret) |
| `CF_QUERY_TIMEOUT` | query-service | `30s` | Maximum query execution time |
| `CF_MAX_CONCURRENT_QUERIES` | query-service | `100` | Query concurrency limit |
| `CF_EVALUATION_INTERVAL` | alert-engine | `30s` | Alert rule evaluation interval |
| `CF_INGESTION_BATCH_SIZE` | data-plane | `1000` | Metrics batch size before flush |
| `CF_INGESTION_FLUSH_INTERVAL` | data-plane | `5s` | Maximum time before flush |

### Per-Service Port Reference

| Service | HTTP Port | gRPC Port | Metrics Port |
|---|---|---|---|
| auth-service | 8001 | 9001 | 9101 |
| tenant-service | 8002 | 9002 | 9102 |
| control-plane | 8003 | 9003 | 9103 |
| data-plane | 8004 | 9004 | 9104 |
| query-service | 8005 | 9005 | 9105 |
| alert-engine | 8006 | 9006 | 9106 |

---

## Troubleshooting Guide

### Service Won't Start

**Symptom:** Container exits immediately or reports unhealthy.

**Steps:**

1. Check logs: `docker-compose logs --tail=100 <service-name>`
2. Verify dependencies are healthy: `docker-compose ps`
3. Check for port conflicts: `ss -tlnp | grep <port>`
4. Verify configuration: `docker-compose config`
5. Check disk space: `df -h`
6. Check memory: `free -h` — OOMKilled is common if limits are too low.

### Database Connection Failures

**Symptom:** Service logs show `connection refused` or `timeout` for MySQL / ClickHouse.

**Steps:**

1. Verify database is running: `docker-compose ps mysql clickhouse`
2. Test connectivity: `mysql -h mysql -u cloudflow -p -e "SELECT 1"` or `clickhouse-client -h clickhouse -u cloudflow`
3. Check credentials: Ensure environment variables match the database users.
4. Verify network: Ensure both services are on the same Docker network.
5. Check max connections: `SHOW VARIABLES LIKE 'max_connections';` in MySQL.

### Kafka Consumer Lag

**Symptom:** Data ingestion delays; `kafka-consumer-groups` shows high lag.

**Steps:**

1. Check consumer status: `kafka-consumer-groups --bootstrap-server kafka:9092 --describe --group <group-id>`
2. Increase partitions if necessary: `kafka-topics --alter --topic metrics.ingestion --partitions 24`
3. Scale data-plane instances horizontally (ensure they use different consumer group IDs if using partition assignment).
4. Check for slow ClickHouse inserts: Monitor `INSERT` latency in ClickHouse.

### High Query Latency

**Symptom:** P99 query latency exceeds SLA (> 2s).

**Steps:**

1. Check ClickHouse query log: `SELECT * FROM system.query_log ORDER BY event_time DESC LIMIT 10;`
2. Identify slow queries and optimize with indexes or materialized views.
3. Check Redis cache hit rate: `INFO stats` and look for `keyspace_hits` / `keyspace_misses`.
4. Verify query-service concurrency isn't saturated: Check `CF_MAX_CONCURRENT_QUERIES`.
5. Scale query-service horizontally if CPU is saturated.

### Alert Engine Not Firing

**Symptom:** Alerts are not triggering despite metrics exceeding thresholds.

**Steps:**

1. Check alert rules are loaded: `curl http://alert-engine:8006/api/v1/alerts/rules`
2. Verify evaluation logs: Look for `evaluating rule` and `result` log lines.
3. Check notification channel configuration: Test webhooks manually.
4. Verify alert-engine can reach ClickHouse: `curl http://clickhouse:8123/ping`
5. Check for silencing or inhibition rules that may suppress alerts.

### Nginx 502 Bad Gateway

**Symptom:** HTTP requests return 502.

**Steps:**

1. Check upstream service health: `docker-compose ps <service>`
2. Verify nginx upstream config matches service names and ports.
3. Check nginx error logs: `docker-compose logs nginx`
4. Verify services are listening on the expected port: `ss -tlnp` inside the container.
5. Check for connection limits: `ulimit -n` and nginx worker_connections.

### Memory Leaks

**Symptom:** Container memory usage grows continuously until OOMKilled.

**Steps:**

1. Enable Go profiling: `curl http://<service>:<metrics-port>/debug/pprof/heap > heap.prof`
2. Analyze with `go tool pprof heap.prof`.
3. Check for goroutine leaks: `curl http://<service>:<metrics-port>/debug/pprof/goroutine`
4. Review for unclosed database connections, gRPC streams, or HTTP response bodies.

---

## Monitoring and Alerting Setup

### Key Metrics to Monitor

| Metric | Source | Warning | Critical | Action |
|---|---|---|---|---|
| CPU Usage | node-exporter | > 70% | > 90% | Scale horizontally or investigate hot paths |
| Memory Usage | node-exporter | > 70% | > 90% | Increase limits or investigate leaks |
| Disk Usage | node-exporter | > 70% | > 85% | Clean up logs, expand volumes, or archive data |
| MySQL Connections | mysql-exporter | > 150 | > 180 | Increase max_connections or investigate connection leaks |
| MySQL Replication Lag | mysql-exporter | > 1s | > 10s | Investigate replica performance or network issues |
| ClickHouse Merge Speed | ClickHouse metrics | < 1 merge/s | < 0.1 merge/s | Investigate disk I/O or part count |
| Kafka Consumer Lag | kafka-exporter | > 10,000 | > 100,000 | Scale consumers or increase partitions |
| Redis Memory | redis-exporter | > 70% | > 90% | Enable eviction or increase memory |
| HTTP Request Rate | service metrics | baseline + 50% | baseline + 100% | Scale or investigate traffic source |
| HTTP P99 Latency | service metrics | > 500ms | > 2s | Optimize queries or scale services |
| HTTP Error Rate | service metrics | > 1% | > 5% | Rollback or investigate root cause |
| gRPC Error Rate | service metrics | > 1% | > 5% | Check network and service dependencies |
| Alert Engine Evaluation Time | alert-engine metrics | > 20s | > 30s | Reduce rule complexity or scale alert-engine |

### Recommended Grafana Dashboards

| Dashboard | Purpose | Key Panels |
|---|---|---|
| CloudFlow Overview | System health | Service status, RPS, latency, error rate |
| CloudFlow Data Plane | Ingestion health | Kafka lag, ClickHouse inserts, batch sizes |
| CloudFlow Query | Query performance | Query latency, concurrency, cache hit rate |
| CloudFlow Alerts | Alerting health | Evaluation time, notification rate, firing alerts |
| CloudFlow Infrastructure | Node health | CPU, memory, disk, network for all nodes |
| MySQL Overview | Database health | Connections, QPS, replication, slow queries |
| ClickHouse Overview | Analytics health | Queries, merges, parts, replication |
| Kafka Overview | Messaging health | Throughput, lag, partition count, broker status |

### Recommended Prometheus Alert Rules

```yaml
groups:
  - name: cloudflow-critical
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High error rate on {{ $labels.service }}"
          description: "Error rate is {{ $value | humanizePercentage }} for {{ $labels.service }}"

      - alert: HighLatency
        expr: histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m])) > 2
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High P99 latency on {{ $labels.service }}"
          description: "P99 latency is {{ $value }}s for {{ $labels.service }}"

      - alert: ServiceDown
        expr: up{job=~"cloudflow-services"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Service {{ $labels.service }} is down"
          description: "Service {{ $labels.service }} has been down for more than 2 minutes"

      - alert: KafkaHighLag
        expr: kafka_consumer_group_lag > 100000
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "High Kafka consumer lag"
          description: "Consumer group {{ $labels.group }} lag is {{ $value }}"

      - alert: MySQLReplicationLag
        expr: mysql_slave_lag_seconds > 10
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "MySQL replication lag is high"
          description: "Replication lag is {{ $value }}s"

      - alert: DiskSpaceLow
        expr: (node_filesystem_avail_bytes / node_filesystem_size_bytes) < 0.15
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Low disk space on {{ $labels.instance }}"
          description: "Disk space is below 15% on {{ $labels.instance }}"

      - alert: MemoryPressure
        expr: (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) < 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage on {{ $labels.instance }}"
          description: "Available memory is below 10% on {{ $labels.instance }}"
```

### On-Call Runbook Quick Reference

| Alert | First Action | Second Action | Escalation |
|---|---|---|---|
| ServiceDown | Check container status and logs | Restart service if needed | Engage service owner if restart fails |
| HighErrorRate | Identify error patterns in logs | Check downstream dependencies | Page on-call engineer if > 10% |
| HighLatency | Check database query performance | Scale service horizontally | Engage DBA or query optimization team |
| KafkaHighLag | Check consumer group status | Scale data-plane consumers | Engage data platform team |
| DiskSpaceLow | Identify largest directories | Clean up or expand volume | Engage infrastructure team |
| MySQLReplicationLag | Check replica I/O and SQL threads | Restart replication if stuck | Engage DBA team |

---

## Appendix

### Version History

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2024-06-12 | CloudFlow SRE Team | Initial release |

### References

- [Production Readiness Checklist](PRODUCTION-READINESS-CHECKLIST.md)
- [Contributing Guide](../.github/CONTRIBUTING.md)
- Docker Compose documentation: https://docs.docker.com/compose/
- Prometheus documentation: https://prometheus.io/docs/
- Grafana documentation: https://grafana.com/docs/
- ClickHouse documentation: https://clickhouse.com/docs
- Kafka documentation: https://kafka.apache.org/documentation/
