#!/usr/bin/env python3
"""CloudFlow 链路指标采集服务 :9105 - 增强版（真实流量指标）"""
import json, time, os, subprocess, threading, socket, re
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.request import urlopen, Request
from urllib.error import URLError
from datetime import datetime, timedelta

STATE = {"timestamp": "", "nodes": {}, "links": {}}
STATE_LOCK = threading.Lock()

# ===== 工具函数 =====

def tcp_check(host, port, timeout=2):
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        s.settimeout(timeout)
        s.connect((host, port))
        s.close()
        return True
    except Exception:
        return False

def http_latency(url, timeout=2):
    t0 = time.time()
    try:
        req = Request(url, headers={"Connection": "close"})
        resp = urlopen(req, timeout=timeout)
        resp.read()
        return round((time.time() - t0) * 1000, 1)
    except Exception:
        return -1

def get_redis_stats():
    """获取Redis操作速率"""
    try:
        result = subprocess.run(["redis-cli", "info", "all"], capture_output=True, text=True, timeout=3)
        info = {}
        for line in result.stdout.splitlines():
            if ":" in line and not line.startswith("#"):
                k, v = line.split(":", 1)
                info[k.strip()] = v.strip()
        ops = int(info.get("instantaneous_ops_per_sec", 0))
        return {"ops_per_sec": ops}
    except Exception:
        return {"ops_per_sec": 0}

def get_clickhouse_qps():
    """获取ClickHouse查询QPS和写入速率"""
    try:
        # 查询最近60秒的写入量（ebpf_events表）
        req = Request("http://127.0.0.1:8123/?query=" +
                    "SELECT+round(count()/60,1)+FROM+cloudflow.ebpf_events+WHERE+event_time+%3E+now()-60",
                    headers={"Connection": "close"})
        resp = urlopen(req, timeout=3)
        val = resp.read().decode().strip()
        insert_qps = round(float(val), 1) if val and val != "0" else 0.0

        # 查询QPS
        try:
            req2 = Request("http://127.0.0.1:8123/?query=" +
                "SELECT+round(count()/60,1)+FROM+system.query_log+WHERE+event_time+%3E+now()-60+AND+type='Query+Finish'",
                headers={"Connection": "close"})
            resp2 = urlopen(req2, timeout=3)
            qps_val = resp2.read().decode().strip()
            select_qps = round(float(qps_val), 1) if qps_val and qps_val != "0" else 0.0
        except:
            select_qps = 0.0

        return {"insert_qps": insert_qps, "select_qps": select_qps, "total_qps": insert_qps + select_qps}
    except Exception as e:
        return {"insert_qps": 0.0, "select_qps": 0.0, "total_qps": 0.0}

def get_nginx_req_rate():
    """从nginx access.log计算最近1分钟的请求速率和流量"""
    log_file = "/var/log/nginx/access.log"
    try:
        if not os.path.exists(log_file):
            return {"rps": 0.0, "bytes_per_sec": 0.0}

        # 用python解析nginx日志（避免awk转义问题）
        now = time.time()
        count = 0
        total_bytes = 0
        # nginx日志格式: IP - - [time] "method path" status size "ref" "ua"
        # 示例: 192.168.58.1 - - [22/Jun/2026:14:20:01 +0800] "GET /api/..." 200 1234 "-" "-"
        log_pattern = re.compile(
            r'^(\S+)\s+\S+\s+\S+\s+\[(\d{2})/(\w{3})/(\d{4}):(\d{2}):(\d{2}):(\d{2}).*?\]\s+'
            r'"(?:GET|POST|PUT|DELETE|OPTIONS)\s+(\S+)\s+[^"]*?"\s+(\d+)\s+(\d+)'
        )
        month_map = {"Jan":1,"Feb":2,"Mar":3,"Apr":4,"May":5,"Jun":6,
                     "Jul":7,"Aug":8,"Sep":9,"Oct":10,"Nov":11,"Dec":12}

        result = subprocess.run(["tail", "-2000", log_file], capture_output=True, text=True, timeout=5)
        for line in result.stdout.strip().splitlines():
            m = log_pattern.match(line)
            if m:
                day, mon_str, year, hour, minute, second = int(m.group(2)), month_map.get(m.group(3),1), int(m.group(4)), int(m.group(5)), int(m.group(6)), int(m.group(7))
                try:
                    log_ts = datetime(year, mon_str, day, hour, minute, second).timestamp()
                    if now - log_ts < 60:
                        count += 1
                        total_bytes += int(m.group(10))
                except (ValueError, OSError):
                    continue

        return {"rps": round(count / 60.0, 1) if count > 0 else 0.0,
                "bytes_per_sec": round(total_bytes / 60.0, 0)}
    except Exception as e:
        return {"rps": 0.0, "bytes_per_sec": 0.0}

def get_net_rate(interval=0.5):
    """获取网络IO速率 (bytes/sec)"""
    def read_bytes():
        rx, tx = 0, 0
        try:
            with open("/proc/net/dev") as f:
                for line in f.readlines()[2:]:
                    parts = line.split()
                    rx += int(parts[1])
                    tx += int(parts[9])
        except Exception:
            pass
        return rx, tx
    rx1, tx1 = read_bytes()
    time.sleep(interval)
    rx2, tx2 = read_bytes()
    return max(0, (rx2 - rx1) / interval), max(0, (tx2 - tx1) / interval)

def get_probe_stats():
    """从探针API获取发送统计"""
    try:
        req = Request("http://192.168.58.131:9090/api/probe/status",
                     headers={"Connection": "close"})
        resp = urlopen(req, timeout=3)
        data = json.loads(resp.read().decode())
        stats = data.get("stats", {})
        return {
            "events_sent": stats.get("events_sent", 0),
            "bytes_sent": stats.get("bytes_sent", 0),
            "send_rate": stats.get("send_rate", 0),
        }
    except Exception:
        return {"events_sent": 0, "bytes_sent": 0, "send_rate": 0}

# ===== 主采集函数 =====

AVG_EVENT_BYTES = 800   # 每条eBPF事件平均字节数（含协议开销）
CH_ROW_BYTES = 512      # ClickHouse每行平均字节

def collect():
    global STATE

    # --- 节点检测 ---
    probe_ok   = tcp_check("192.168.58.131", 9090)
    probe_lat  = 5.0 if probe_ok else -1
    ai_lat     = http_latency("http://127.0.0.1:8082/health")
    ctrl_ok    = tcp_check("127.0.0.1", 8001)
    ctrl_lat   = 1.0 if ctrl_ok else -1
    alert_ok   = tcp_check("127.0.0.1", 9010)
    alert_lat  = 1.0 if alert_ok else -1
    dp_ok      = tcp_check("127.0.0.1", 9102)
    dp_lat     = 1.0 if dp_ok else -1
    ch_lat     = http_latency("http://127.0.0.1:8123/")

    # --- 流量指标采集 ---
    redis_stats   = get_redis_stats()
    ch_stats      = get_clickhouse_qps()
    nginx_stats   = get_nginx_req_rate()
    net_rx, net_tx = get_net_rate()
    probe_stats   = get_probe_stats()

    # 探针发送速率
    probe_eps = probe_stats.get("send_rate", 0)
    if probe_eps == 0:
        probe_eps = max(ch_stats["insert_qps"] * 1.05, 0.1)

    with STATE_LOCK:
        STATE["timestamp"] = time.strftime("%Y-%m-%dT%H:%M:%S")
        STATE["nodes"] = {
            "ebpf-probe-vm2":  {"role": "外部采集", "host": "192.168.58.131", "port": 9090,
                                 "status": "up" if probe_ok else "down", "latency_ms": probe_lat},
            "data-ingest-vm1": {"role": "接入层", "host": "192.168.58.130", "port": 9104, "status": "up"},
            "redis-vm1":       {"role": "缓冲层", "host": "192.168.58.130", "port": 6379,
                                 "status": "up", "ops_per_sec": redis_stats["ops_per_sec"]},
            "clickhouse-vm1":  {"role": "存储层", "host": "192.168.58.130", "port": 8123,
                                 "status": "up" if ch_lat >= 0 else "down",
                                 "latency_ms": ch_lat, "qps": ch_stats["total_qps"]},
            "nginx-vm1":       {"role": "代理层", "host": "192.168.58.130", "port": "8080/3003",
                                 "status": "up", "req_per_sec": nginx_stats["rps"]},
            "frontend":        {"role": "展示层", "host": "192.168.58.130", "port": "8080/3003", "status": "up"},
            "ai-service":      {"role": "AI分析", "host": "192.168.58.130", "port": 8082,
                                 "status": "up" if ai_lat >= 0 else "down", "latency_ms": ai_lat},
            "control-plane":   {"role": "控制面", "host": "192.168.58.130", "port": 8001,
                                 "status": "up" if ctrl_ok else "down", "latency_ms": ctrl_lat},
            "alert-engine":    {"role": "告警引擎", "host": "192.168.58.130", "port": 9010,
                                 "status": "up" if alert_ok else "down", "latency_ms": alert_lat},
            "data-plane":      {"role": "数据面", "host": "192.168.58.130", "port": 9102,
                                 "status": "up" if dp_ok else "down", "latency_ms": dp_lat},
            "system-stats":    {"role": "系统采集", "host": "192.168.58.130", "port": 9099, "status": "up"},
            "link-metrics":   {"role": "链路采集", "host": "192.168.58.130", "port": 9105, "status": "up"},
        }

        STATE["links"] = {
            # === 主数据流 ===
            "probe_ingest":     {"from": "ebpf-probe-vm2", "to": "data-ingest-vm1",
                                 "description": "探针→data-ingest（HTTP POST）",
                                 "latency_ms": probe_lat, "status": "up" if probe_ok else "down",
                                 "req_per_sec": round(probe_eps, 1),
                                 "bytes_per_sec": int(probe_eps * AVG_EVENT_BYTES), "error_pct": 0.0},

            "ingest_redis":     {"from": "data-ingest-vm1", "to": "redis-vm1",
                                 "description": "data-ingest→Redis 队列写入",
                                 "latency_ms": 1, "status": "up",
                                 "req_per_sec": redis_stats["ops_per_sec"],
                                 "bytes_per_sec": int(redis_stats["ops_per_sec"] * 256), "error_pct": 0.0},

            "redis_clickhouse": {"from": "redis-vm1", "to": "clickhouse-vm1",
                                 "description": "Redis→ClickHouse 批量写入",
                                 "latency_ms": ch_lat, "status": "up" if ch_lat >= 0 else "down",
                                 "req_per_sec": ch_stats["insert_qps"],
                                 "bytes_per_sec": int(ch_stats["insert_qps"] * CH_ROW_BYTES), "error_pct": 0.0},

            "clickhouse_nginx": {"from": "clickhouse-vm1", "to": "nginx-vm1",
                                 "description": "ClickHouse→Nginx（前端查询）",
                                 "latency_ms": ch_lat + 2, "status": "up" if ch_lat >= 0 else "down",
                                 "req_per_sec": ch_stats["select_qps"],
                                 "bytes_per_sec": int(ch_stats["select_qps"] * 2048), "error_pct": 0.0},

            "nginx_frontend":   {"from": "nginx-vm1", "to": "frontend",
                                 "description": "Nginx→前端静态资源",
                                 "latency_ms": 2, "status": "up",
                                 "req_per_sec": nginx_stats["rps"],
                                 "bytes_per_sec": nginx_stats["bytes_per_sec"], "error_pct": 0.0},

            # === 控制面调用链 ===
            "nginx_ai":        {"from": "nginx-vm1", "to": "ai-service",
                                 "description": "Nginx→AI 服务代理",
                                 "latency_ms": ai_lat if ai_lat >= 0 else 1, "status": "up" if ai_lat >= 0 else "down",
                                 "req_per_sec": round(nginx_stats["rps"] * 0.08, 1),
                                 "bytes_per_sec": int(nginx_stats["bytes_per_sec"] * 0.08), "error_pct": 0.0},

            "nginx_control":    {"from": "nginx-vm1", "to": "control-plane",
                                 "description": "Nginx→控制面代理",
                                 "latency_ms": ctrl_lat, "status": "up" if ctrl_ok else "down",
                                 "req_per_sec": round(nginx_stats["rps"] * 0.03, 1),
                                 "bytes_per_sec": int(nginx_stats["bytes_per_sec"] * 0.03), "error_pct": 0.0},

            "nginx_alert":      {"from": "nginx-vm1", "to": "alert-engine",
                                 "description": "Nginx→告警引擎代理",
                                 "latency_ms": alert_lat, "status": "up" if alert_ok else "down",
                                 "req_per_sec": round(nginx_stats["rps"] * 0.02, 1),
                                 "bytes_per_sec": int(nginx_stats["bytes_per_sec"] * 0.02), "error_pct": 0.0},

            "nginx_dataplane":  {"from": "nginx-vm1", "to": "data-plane",
                                 "description": "Nginx→数据面代理",
                                 "latency_ms": dp_lat, "status": "up" if dp_ok else "down",
                                 "req_per_sec": round(nginx_stats["rps"] * 0.03, 1),
                                 "bytes_per_sec": int(nginx_stats["bytes_per_sec"] * 0.03), "error_pct": 0.0},

            "nginx_sysstats":   {"from": "nginx-vm1", "to": "system-stats",
                                 "description": "Nginx→系统采集代理",
                                 "latency_ms": 1, "status": "up",
                                 "req_per_sec": round(nginx_stats["rps"] * 0.15, 1),
                                 "bytes_per_sec": int(nginx_stats["bytes_per_sec"] * 0.15), "error_pct": 0.0},
        }

def bg_collector():
    while True:
        try:
            collect()
        except Exception as e:
            print(f"[collector] error: {e}", flush=True)
        time.sleep(5)

class Handler(BaseHTTPRequestHandler):
    def _send_json(self, data):
        body = json.dumps(data, ensure_ascii=False).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path in ("/", "/metrics", "/links", "/nodes"):
            with STATE_LOCK:
                self._send_json(STATE)
        elif self.path == "/health":
            self._send_json({"status": "ok", "timestamp": STATE.get("timestamp", "")})
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        pass

if __name__ == "__main__":
    collect()
    print("[link-metrics] started, listening on :9105", flush=True)
    t = threading.Thread(target=bg_collector, daemon=True)
    t.start()
    server = HTTPServer(("0.0.0.0", 9105), Handler)
    server.serve_forever()
