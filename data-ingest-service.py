#!/usr/bin/env python3
"""
CloudFlow Data Ingest Service
- 接收探针 HTTP 数据上报
- 缓冲到 Redis
- 批量写入 ClickHouse
- 数据过滤与采样
- Prometheus 指标暴露
- 降级与容灾策略
- 替代 edge 的 ClickHouse 写入能力
"""

import json
import logging
import time
import threading
import os
import random
from datetime import datetime
from http.server import HTTPServer, BaseHTTPRequestHandler
import redis
from quota import QuotaManager
from clickhouse_driver import Client
from clickhouse_driver.errors import NetworkError, SocketTimeoutError, ServerException

# 尝试导入 Prometheus 客户端，如果不可用则提供 mock
try:
    from prometheus_client import Counter, Histogram, Gauge, start_http_server, CollectorRegistry
    PROMETHEUS_AVAILABLE = True
except ImportError:
    PROMETHEUS_AVAILABLE = False
    logging.warning("prometheus_client not available, metrics disabled")
    # Mock classes for when prometheus_client is not available
    class Counter:
        def __init__(self, *args, **kwargs): pass
        def inc(self, *args, **kwargs): pass
        def labels(self, *args, **kwargs): return self
    class Histogram:
        def __init__(self, *args, **kwargs): pass
        def observe(self, *args, **kwargs): pass
        def labels(self, *args, **kwargs): return self
        def time(self): return _NullContext()
    class Gauge:
        def __init__(self, *args, **kwargs): pass
        def set(self, *args, **kwargs): pass
        def inc(self, *args, **kwargs): pass
        def dec(self, *args, **kwargs): pass
        def labels(self, *args, **kwargs): return self
    class _NullContext:
        def __enter__(self): return self
        def __exit__(self, *args): pass
    def start_http_server(*args, **kwargs): pass
    class CollectorRegistry:
        def __init__(self): pass

# 配置
REDIS_HOST = os.getenv("REDIS_HOST", "localhost")
REDIS_PORT = int(os.getenv("REDIS_PORT", "6379"))
REDIS_DB = int(os.getenv("REDIS_DB", "0"))
REDIS_PASSWORD = os.getenv("REDIS_PASSWORD", "")
CLICKHOUSE_HOST = os.getenv("CLICKHOUSE_HOST", "127.0.0.1")
CLICKHOUSE_PORT = int(os.getenv("CLICKHOUSE_PORT", "9000"))
CLICKHOUSE_DB = os.getenv("CLICKHOUSE_DB", "cloudflow")
CLICKHOUSE_USER = os.getenv("CLICKHOUSE_USER", "default")
CLICKHOUSE_PASSWORD = os.getenv("CLICKHOUSE_PASSWORD", "")
HTTP_PORT = int(os.getenv("HTTP_PORT", "9104"))
METRICS_PORT = int(os.getenv("METRICS_PORT", "9105"))
BATCH_SIZE = int(os.getenv("BATCH_SIZE", "10000"))
FLUSH_INTERVAL = int(os.getenv("FLUSH_INTERVAL", "2"))

# 数据过滤与采样配置
SAMPLING_RATE = float(os.getenv("SAMPLING_RATE", "1.0"))  # 1.0 = 100%, 0.1 = 10%

# 按类别差异化采样率：高频噪音事件降低采样，安全/异常事件全量保留
CATEGORY_SAMPLING = {
    "file_events": float(os.getenv("SAMPLING_FILE", "0.01")),      # 1% — 文件操作是噪音大户
    "network_events": float(os.getenv("SAMPLING_NETWORK", "0.10")), # 10% — 网络事件量中等
    "process_events": float(os.getenv("SAMPLING_PROCESS", "0.10")), # 10% — 进程事件量中等
    "security_events": float(os.getenv("SAMPLING_SECURITY", "1.00")), # 100% — 安全事件必须全量
}

FILTER_CATEGORIES = os.getenv("FILTER_CATEGORIES", "").split(",")
FILTER_CATEGORIES = [c.strip() for c in FILTER_CATEGORIES if c.strip()]
MAX_EVENT_BYTES = int(os.getenv("MAX_EVENT_BYTES", "1048576"))  # 1MB per event

# 降级策略
REDIS_BACKUP_DIR = os.getenv("REDIS_BACKUP_DIR", "/tmp/cloudflow_backup")
MAX_REDIS_BACKUP_SIZE_MB = int(os.getenv("MAX_REDIS_BACKUP_SIZE_MB", "500"))
CLICKHOUSE_RETRY_MAX = int(os.getenv("CLICKHOUSE_RETRY_MAX", "3"))
CLICKHOUSE_RETRY_DELAY = float(os.getenv("CLICKHOUSE_RETRY_DELAY", "1.0"))

# 日志
import sys
handler = logging.StreamHandler(sys.stdout)
handler.setLevel(logging.INFO)
handler.setFormatter(logging.Formatter('%(asctime)s [%(levelname)s] %(message)s'))
root_logger = logging.getLogger()
root_logger.setLevel(logging.INFO)
if not root_logger.handlers:
    root_logger.addHandler(handler)
logger = logging.getLogger(__name__)

# Prometheus 指标
registry = CollectorRegistry()
EVENTS_INGESTED_TOTAL = Counter(
    'cloudflow_events_ingested_total',
    'Total events ingested from probes',
    ['tenant_id', 'category'],
    registry=registry
)
EVENTS_FLUSHED_TOTAL = Counter(
    'cloudflow_events_flushed_total',
    'Total events flushed to ClickHouse',
    ['tenant_id', 'status'],
    registry=registry
)
EVENTS_DROPPED_TOTAL = Counter(
    'cloudflow_events_dropped_total',
    'Total events dropped by filtering/sampling',
    ['reason'],
    registry=registry
)
FLUSH_DURATION = Histogram(
    'cloudflow_flush_duration_seconds',
    'ClickHouse flush duration',
    ['status'],
    registry=registry
)
REDIS_QUEUE_LENGTH = Gauge(
    'cloudflow_redis_queue_length',
    'Current Redis queue length',
    registry=registry
)
CH_CONNECTION_STATUS = Gauge(
    'cloudflow_clickhouse_connection_status',
    'ClickHouse connection status (1=up, 0=down)',
    registry=registry
)
BACKUP_FILE_SIZE = Gauge(
    'cloudflow_backup_file_size_bytes',
    'Current backup file size',
    registry=registry
)


class DataIngestService:
    def __init__(self):
        self._init_redis()
        self._init_clickhouse()
        self._stop = threading.Event()
        self._flush_thread = threading.Thread(target=self._flush_loop)
        self._flush_thread.daemon = True
        self._metrics_thread = threading.Thread(target=self._metrics_loop)
        self._metrics_thread.daemon = True
        self._backup_lock = threading.Lock()
        self._backup_file = None
        self._backup_size = 0
        self.quota_manager = QuotaManager()
        os.makedirs(REDIS_BACKUP_DIR, exist_ok=True)

    def _init_redis(self):
        try:
            self.redis_client = redis.Redis(
                host=REDIS_HOST, port=REDIS_PORT, db=REDIS_DB,
                password=REDIS_PASSWORD, decode_responses=True,
                socket_connect_timeout=5, socket_timeout=5
            )
            self.redis_client.ping()
            logger.info("Redis connected")
        except Exception as e:
            logger.error(f"Redis connection failed: {e}")
            self.redis_client = None

    def _init_clickhouse(self):
        try:
            self.ch_client = Client(
                host=CLICKHOUSE_HOST, port=CLICKHOUSE_PORT,
                database=CLICKHOUSE_DB,
                user=CLICKHOUSE_USER, password=CLICKHOUSE_PASSWORD,
                settings={'max_execution_time': 30}
            )
            self.ch_client.execute("SELECT 1")
            CH_CONNECTION_STATUS.set(1)
            logger.info("ClickHouse connected")
        except Exception as e:
            logger.error(f"ClickHouse connection failed: {e}")
            self.ch_client = None
            CH_CONNECTION_STATUS.set(0)

    def start(self):
        self._flush_thread.start()
        self._metrics_thread.start()
        if PROMETHEUS_AVAILABLE:
            start_http_server(METRICS_PORT, registry=registry)
            logger.info(f"Prometheus metrics exposed on :{METRICS_PORT}")
        logger.info("DataIngestService started")

    def stop(self):
        self._stop.set()
        self._flush_thread.join(timeout=10)
        self._metrics_thread.join(timeout=5)
        if self._backup_file:
            self._backup_file.close()
        logger.info("DataIngestService stopped")

    def _reconnect_clickhouse(self):
        """尝试重连 ClickHouse"""
        for attempt in range(CLICKHOUSE_RETRY_MAX):
            try:
                self.ch_client = Client(
                    host=CLICKHOUSE_HOST, port=CLICKHOUSE_PORT,
                    database=CLICKHOUSE_DB,
                    user=CLICKHOUSE_USER, password=CLICKHOUSE_PASSWORD,
                    settings={'max_execution_time': 30}
                )
                self.ch_client.execute("SELECT 1")
                CH_CONNECTION_STATUS.set(1)
                logger.info("ClickHouse reconnected")
                return True
            except Exception as e:
                logger.warning(f"ClickHouse reconnect attempt {attempt+1} failed: {e}")
                time.sleep(CLICKHOUSE_RETRY_DELAY * (2 ** attempt))
        CH_CONNECTION_STATUS.set(0)
        return False

    def ingest(self, events):
        """将事件推入 Redis 队列，支持过滤和采样。返回 True 表示成功，False 表示失败。"""
        if not events:
            return True

        filtered = []
        for ev in events:
            # 大小限制
            ev_size = len(json.dumps(ev))
            if ev_size > MAX_EVENT_BYTES:
                EVENTS_DROPPED_TOTAL.labels(reason="size_limit").inc()
                continue

            # 类别过滤
            category = ev.get("category", "")
            if FILTER_CATEGORIES and category not in FILTER_CATEGORIES:
                EVENTS_DROPPED_TOTAL.labels(reason="category_filter").inc()
                continue

            # 按类别差异化采样
            rate = CATEGORY_SAMPLING.get(category, SAMPLING_RATE)
            if rate < 1.0 and random.random() > rate:
                EVENTS_DROPPED_TOTAL.labels(reason="sampling").inc()
                continue

            # 补充 tenant_id 默认值
            tenant_id = ev.get("tenant_id", "default")
            if not tenant_id:
                ev["tenant_id"] = "default"
                tenant_id = "default"

            # Check tenant quota
            if self.quota_manager.is_disabled(tenant_id):
                logger.warning(f"Tenant {tenant_id} is disabled, dropping events")
                return False
            if not self.quota_manager.check_event_rate(tenant_id):
                logger.warning(f"Tenant {tenant_id} rate limit exceeded")
                EVENTS_DROPPED_TOTAL.labels(reason="quota_exceeded").inc()
                continue

            filtered.append(ev)
            EVENTS_INGESTED_TOTAL.labels(
                tenant_id=ev.get("tenant_id", "default"),
                category=category or "unknown"
            ).inc()

        if not filtered:
            return True

        # 降级策略：Redis 不可用时写入本地备份文件
        if self.redis_client is None:
            self._backup_to_local(filtered)
            return False

        try:
            pipeline = self.redis_client.pipeline()
            for ev in filtered:
                pipeline.rpush("cloudflow:events", json.dumps(ev))
            pipeline.execute()
            logger.info(f"Queued {len(filtered)} events to Redis")
            return True
        except Exception as e:
            logger.error(f"Redis push failed, falling back to local backup: {e}")
            self._backup_to_local(filtered)
            return False

    def _backup_to_local(self, events):
        """本地文件备份（降级策略）"""
        with self._backup_lock:
            try:
                backup_path = os.path.join(REDIS_BACKUP_DIR, f"events_{datetime.now().strftime('%Y%m%d')}.jsonl")
                if self._backup_file is None or self._backup_file.name != backup_path:
                    if self._backup_file:
                        self._backup_file.close()
                    self._backup_file = open(backup_path, "a")
                    self._backup_size = os.path.getsize(backup_path)

                for ev in events:
                    line = json.dumps(ev) + "\n"
                    self._backup_file.write(line)
                    self._backup_size += len(line.encode('utf-8'))
                self._backup_file.flush()
                BACKUP_FILE_SIZE.set(self._backup_size)
                logger.info(f"Backed up {len(events)} events to {backup_path}")

                # 检查备份大小，超过限制则清理旧文件
                self._cleanup_old_backups()
            except Exception as e:
                logger.error(f"Local backup failed: {e}")

    def _cleanup_old_backups(self):
        """清理超过大小限制的备份文件"""
        try:
            total_size = 0
            files = []
            for f in os.listdir(REDIS_BACKUP_DIR):
                path = os.path.join(REDIS_BACKUP_DIR, f)
                if os.path.isfile(path):
                    size = os.path.getsize(path)
                    files.append((path, size, os.path.getmtime(path)))
                    total_size += size

            max_size = MAX_REDIS_BACKUP_SIZE_MB * 1024 * 1024
            if total_size > max_size:
                files.sort(key=lambda x: x[2])  # 按修改时间排序
                for path, size, _ in files:
                    if total_size <= max_size:
                        break
                    os.remove(path)
                    total_size -= size
                    logger.info(f"Removed old backup: {path}")
        except Exception as e:
            logger.error(f"Backup cleanup failed: {e}")

    def _cleanup_disk(self):
        """如果磁盘使用率超过 85%，清理 ClickHouse 旧数据"""
        try:
            import shutil
            usage = shutil.disk_usage("/")
            percent = usage.used / usage.total * 100
            if percent > 85:
                logger.warning(f"Disk usage {percent:.1f}% > 85%, cleaning old ClickHouse data")
                # 删除 7 天前的数据
                self.ch_client.execute("ALTER TABLE cloudflow.ebpf_events DELETE WHERE timestamp < now() - INTERVAL 7 DAY")
                # 执行 OPTIMIZE
                self.ch_client.execute("OPTIMIZE TABLE cloudflow.ebpf_events FINAL")
        except Exception as e:
            logger.error(f"Disk cleanup failed: {e}")

    def _flush_loop(self):
        while not self._stop.wait(timeout=FLUSH_INTERVAL):
            self._cleanup_disk()
            self._flush()

    def _metrics_loop(self):
        while not self._stop.wait(timeout=10):
            try:
                if self.redis_client:
                    length = self.redis_client.llen("cloudflow:events")
                    REDIS_QUEUE_LENGTH.set(length)
                else:
                    REDIS_QUEUE_LENGTH.set(0)
            except Exception:
                pass

    def _flush(self):
        if self.redis_client is None:
            # 尝试重连 Redis
            try:
                self._init_redis()
            except Exception:
                return

        try:
            # 批量从 Redis 中取出，使用 lrange+ltrim 避免 5000 次 lpop 网络往返
            pipe = self.redis_client.pipeline()
            pipe.lrange("cloudflow:events", 0, BATCH_SIZE - 1)
            pipe.ltrim("cloudflow:events", BATCH_SIZE, -1)
            raw_list, _ = pipe.execute()

            events = []
            for event_json in raw_list:
                try:
                    events.append(json.loads(event_json))
                except json.JSONDecodeError:
                    EVENTS_DROPPED_TOTAL.labels(reason="json_decode").inc()
                    continue

            if not events:
                return

            # 批量写入 ClickHouse
            rows = []
            for ev in events:
                ts = ev.get("timestamp", datetime.now().isoformat())
                if isinstance(ts, str):
                    try:
                        dt = datetime.fromisoformat(ts.replace("Z", "+00:00"))
                        ts = int(dt.timestamp() * 1_000_000_000)
                    except Exception:
                        ts = int(datetime.now().timestamp() * 1_000_000_000)
                elif isinstance(ts, datetime):
                    ts = int(ts.timestamp() * 1_000_000_000)
                elif not isinstance(ts, int):
                    ts = int(datetime.now().timestamp() * 1_000_000_000)
                rows.append([
                    ts, ev.get("probe_id", ""), ev.get("category", ""),
                    ev.get("event_type", ""), ev.get("src_ip", ""),
                    ev.get("dst_ip", ""), int(ev.get("src_port", 0) or 0),
                    int(ev.get("dst_port", 0) or 0), ev.get("protocol", ""),
                    int(ev.get("bytes", 0) or 0), int(ev.get("packets", 0) or 0),
                    int(ev.get("latency_ms", 0) or 0), ev.get("service", ""),
                    ev.get("details", ""), ev.get("tags", ""),
                    ev.get("tenant_id", "default")
                ])

            start = time.time()
            success = False
            for attempt in range(CLICKHOUSE_RETRY_MAX):
                try:
                    if self.ch_client is None:
                        if not self._reconnect_clickhouse():
                            raise Exception("ClickHouse not available")
                    self.ch_client.execute(
                        "INSERT INTO ebpf_events (timestamp, probe_id, category, event_type, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency_ms, service, details, tags, tenant_id) VALUES",
                        rows
                    )
                    success = True
                    break
                except (NetworkError, SocketTimeoutError, ServerException) as e:
                    logger.warning(f"ClickHouse insert attempt {attempt+1} failed: {e}")
                    self.ch_client = None
                    CH_CONNECTION_STATUS.set(0)
                    time.sleep(CLICKHOUSE_RETRY_DELAY * (2 ** attempt))

            duration = time.time() - start
            if success:
                FLUSH_DURATION.labels(status="success").observe(duration)
                for ev in events:
                    EVENTS_FLUSHED_TOTAL.labels(
                        tenant_id=ev.get("tenant_id", "default"),
                        status="success"
                    ).inc()
                # Track storage usage
                for ev in events:
                    tenant_id = ev.get("tenant_id", "default")
                    self.quota_manager.add_storage_usage(tenant_id, len(json.dumps(ev)))
                logger.info(f"Flushed {len(events)} events to ClickHouse in {duration:.3f}s")
            else:
                FLUSH_DURATION.labels(status="failure").observe(duration)
                for ev in events:
                    EVENTS_FLUSHED_TOTAL.labels(
                        tenant_id=ev.get("tenant_id", "default"),
                        status="failure"
                    ).inc()
                # 写入失败，先尝试放回 Redis 队列，失败再回退到本地备份
                logger.error(f"ClickHouse flush failed after {CLICKHOUSE_RETRY_MAX} retries, trying to push back to Redis")
                try:
                    if self.redis_client is not None:
                        pipe = self.redis_client.pipeline()
                        for ev in events:
                            pipe.lpush("cloudflow:events", json.dumps(ev))
                        pipe.execute()
                        logger.info(f"Pushed {len(events)} events back to Redis queue")
                    else:
                        raise Exception("Redis not available")
                except Exception as redis_err:
                    logger.error(f"Redis push back failed, falling back to local backup: {redis_err}")
                    self._backup_to_local(events)
        except Exception as e:
            logger.error(f"Flush failed: {e}")



# ============================================================================
# Rate limiting (DDoS protection)
# ============================================================================
from collections import defaultdict

class RateLimiter:
    def __init__(self, max_requests=100, window_seconds=60):
        self.max_requests = max_requests
        self.window_seconds = window_seconds
        self.lock = threading.RLock()
        self.requests = defaultdict(list)

    def allow(self, client_ip):
        now = time.time()
        with self.lock:
            self.requests[client_ip] = [
                t for t in self.requests[client_ip]
                if now - t < self.window_seconds
            ]
            if len(self.requests[client_ip]) >= self.max_requests:
                return False
            self.requests[client_ip].append(now)
            return True

    def get_remaining(self, client_ip):
        now = time.time()
        with self.lock:
            self.requests[client_ip] = [
                t for t in self.requests[client_ip]
                if now - t < self.window_seconds
            ]
            return max(0, self.max_requests - len(self.requests[client_ip]))

IP_RATE_LIMITER = RateLimiter(
    max_requests=int(os.getenv("RATE_LIMIT_IP_MAX", "100")),
    window_seconds=int(os.getenv("RATE_LIMIT_IP_WINDOW", "60"))
)
MAX_CONCURRENT_CONNECTIONS = int(os.getenv("MAX_CONCURRENT_CONNECTIONS", "1000"))
_connection_semaphore = threading.Semaphore(MAX_CONCURRENT_CONNECTIONS)

class IngestHandler(BaseHTTPRequestHandler):
    service = None

    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{"status":"healthy"}')
        elif self.path == "/metrics":
            if PROMETHEUS_AVAILABLE:
                from prometheus_client import generate_latest, CONTENT_TYPE_LATEST
                self.send_response(200)
                self.send_header("Content-Type", CONTENT_TYPE_LATEST)
                self.end_headers()
                self.wfile.write(generate_latest(registry))
            else:
                self.send_response(503)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
            self.wfile.write(b'{"error":"prometheus metrics not available"}')
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        client_ip = self.client_address[0]
        if not _connection_semaphore.acquire(blocking=False):
            self.send_response(503)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"error": "server_overloaded", "message": "too many concurrent connections"}).encode())
            return
        try:
            self._handle_post()
        finally:
            _connection_semaphore.release()

    def _handle_post(self):
        client_ip = self.client_address[0]
        if not IP_RATE_LIMITER.allow(client_ip):
            self.send_response(429)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Retry-After', '60')
            self.end_headers()
            self.wfile.write(json.dumps({"error": "rate_limit_exceeded", "message": "rate limit exceeded for IP " + client_ip, "retry_after": 60}).encode())
            return

        if self.path == "/api/v1/ingest":
            content_length = int(self.headers.get('Content-Length', 0))
            body = self.rfile.read(content_length)
            try:
                events = json.loads(body)
                if not isinstance(events, list):
                    events = [events]
                ok = self.service.ingest(events)
                if ok:
                    self.send_response(200)
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(b'{"status":"ok"}')
                else:
                    self.send_response(503)
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(json.dumps({"status": "error", "message": "redis_unavailable"}).encode())
            except Exception as e:
                self.send_response(400)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({"error": str(e)}).encode())
        elif self.path == "/health":
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{"status":"healthy"}')
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        logger.info("%s - %s" % (self.address_string(), format % args))


def main():
    service = DataIngestService()
    service.start()
    IngestHandler.service = service

    server = HTTPServer(("0.0.0.0", HTTP_PORT), IngestHandler)
    logger.info(f"HTTP server listening on :{HTTP_PORT}")
    logger.info(f"Health check endpoint: http://localhost:{HTTP_PORT}/health")
    if PROMETHEUS_AVAILABLE:
        logger.info(f"Metrics endpoint: http://localhost:{METRICS_PORT}/metrics")

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        service.stop()
        server.server_close()


if __name__ == "__main__":
    main()
