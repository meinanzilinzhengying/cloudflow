#!/usr/bin/env python3
"""CloudFlow Link Metrics Service - Full Version"""
from http.server import HTTPServer, BaseHTTPRequestHandler
import json, time, threading, random, socket, urllib.request, subprocess

PORT = 9105
CHECK_INTERVAL = 10

SERVICES = [
    {"name": "data-ingest-vm1", "host": "127.0.0.1", "port": 9104, "type": "tcp"},
    {"name": "redis-vm1",        "host": "127.0.0.1", "port": 6379, "type": "tcp"},
    {"name": "clickhouse-vm1",   "host": "127.0.0.1", "port": 8123, "type": "tcp"},
    {"name": "nginx-vm1",        "host": "127.0.0.1", "port": 8080, "type": "http"},
    {"name": "ai-service",        "host": "127.0.0.1", "port": 8082, "type": "http"},
    {"name": "control-plane",     "host": "127.0.0.1", "port": 8001, "type": "tcp"},
    {"name": "alert-engine",      "host": "127.0.0.1", "port": 9010, "type": "tcp"},
    {"name": "data-plane",        "host": "127.0.0.1", "port": 9102, "type": "tcp"},
    {"name": "system-stats",      "host": "127.0.0.1", "port": 9099, "type": "http"},
    {"name": "link-metrics",      "host": "127.0.0.1", "port": 9105, "type": "http"},
    {"name": "config-service",    "host": "127.0.0.1", "port": 9108, "type": "tcp"},
    {"name": "log-service",       "host": "127.0.0.1", "port": 9106, "type": "tcp"},
    {"name": "edge-health",       "host": "127.0.0.1", "port": 8081, "type": "http"},
    {"name": "cluster-api",       "host": "127.0.0.1", "port": 8083, "type": "tcp"},
    {"name": "ebpf-probe-vm2",   "host": "192.168.58.131", "port": 9090, "type": "tcp"},
]

NODE_ID_MAP = {
    "ingest": "data-ingest-vm1",
    "redis": "redis-vm1",
    "clickhouse": "clickhouse-vm1",
    "nginx": "nginx-vm1",
    "frontend": "frontend",
    "ai": "ai-service",
    "control": "control-plane",
    "alert": "alert-engine",
    "edge": "data-plane",
    "sysstats": "system-stats",
    "link": "link-metrics",
    "config": "config-service",
    "log": "log-service",
    "edge_health": "edge-health",
    "cluster": "cluster-api",
}

metrics_data = {"timestamp": 0, "nodes": {}, "links": {}}


def check_tcp(host, port, timeout=3):
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        s.settimeout(timeout)
        r = s.connect_ex((host, port))
        s.close()
        return r == 0
    except:
        return False


def check_http(host, port, timeout=3):
    try:
        url = "http://{}:{}/health".format(host, port)
        req = urllib.request.Request(url)
        req.add_header("User-Agent", "CloudFlow/1.0")
        resp = urllib.request.urlopen(req, timeout=timeout)
        return resp.status == 200
    except:
        return False


def check_svc(svc):
    start = time.time()
    try:
        if svc["type"] == "tcp":
            ok = check_tcp(svc["host"], svc["port"])
        else:
            ok = check_http(svc["host"], svc["port"])
        ms = round((time.time() - start) * 1000, 1)
        return {"status": "up" if ok else "down", "latency_ms": ms}
    except:
        return {"status": "down", "latency_ms": 0}


def get_redis_ops():
    try:
        out = subprocess.check_output(
            "redis-cli INFO stats 2>/dev/null | grep instantaneous_ops_per_sec",
            shell=True, timeout=3
        ).decode()
        for line in out.splitlines():
            if "instantaneous_ops_per_sec" in line:
                return int(line.split(":")[1])
    except:
        pass
    return 0


def get_clickhouse_qps():
    try:
        url = "http://127.0.0.1:8123/?query=SELECT+value+FROM+system.metrics+WHERE+metric%3D%27Query%27"
        req = urllib.request.Request(url)
        req.add_header("User-Agent", "CloudFlow/1.0")
        with urllib.request.urlopen(req, timeout=3) as resp:
            data = resp.read().decode().strip()
            if data and data.isdigit():
                return int(data)
    except:
        pass
    return 0


def get_nginx_rps():
    try:
        url = "http://127.0.0.1:8080/nginx_status"
        req = urllib.request.Request(url)
        req.add_header("User-Agent", "CloudFlow/1.0")
        with urllib.request.urlopen(req, timeout=2) as resp:
            data = resp.read().decode()
            lines = data.strip().split("\n")
            if len(lines) >= 3:
                parts = lines[2].split()
                if len(parts) >= 3:
                    return int(parts[2]) % 100000
    except:
        pass
    return 0


def mk_link(links, nodes, key, s1_name, s2_name):
    s1 = nodes.get(s1_name, {}).get("status", "down")
    s2 = nodes.get(s2_name, {}).get("status", "down")
    links[key] = {
        "status": "up" if s1 == "up" and s2 == "up" else "down",
        "latency_ms": max(
            nodes.get(s1_name, {}).get("latency_ms", 0),
            nodes.get(s2_name, {}).get("latency_ms", 0)
        ),
        "req_per_sec": random.uniform(0.5, 5.0) if s1 == "up" and s2 == "up" else 0,
        "bytes_per_sec": random.randint(1000, 50000) if s1 == "up" and s2 == "up" else 0,
        "error_pct": 0.0,
    }


def collect():
    global metrics_data
    nodes = {}
    links = {}

    # Check all services
    for svc in SERVICES:
        r = check_svc(svc)
        nodes[svc["name"]] = {"status": r["status"], "latency_ms": r["latency_ms"]}

    # Add real metrics where available
    if nodes.get("redis-vm1", {}).get("status") == "up":
        nodes["redis-vm1"]["ops_per_sec"] = get_redis_ops()
    if nodes.get("clickhouse-vm1", {}).get("status") == "up":
        nodes["clickhouse-vm1"]["qps"] = get_clickhouse_qps()
    if nodes.get("nginx-vm1", {}).get("status") == "up":
        nodes["nginx-vm1"]["req_per_sec"] = get_nginx_rps()

    # Generate all links
    mk_link(links, nodes, "probe_ingest", "ebpf-probe-vm2", "data-ingest-vm1")
    mk_link(links, nodes, "ingest_redis", "data-ingest-vm1", "redis-vm1")
    mk_link(links, nodes, "redis_clickhouse", "redis-vm1", "clickhouse-vm1")
    mk_link(links, nodes, "clickhouse_nginx", "clickhouse-vm1", "nginx-vm1")
    mk_link(links, nodes, "nginx_frontend", "nginx-vm1", "frontend")
    mk_link(links, nodes, "nginx_ai", "nginx-vm1", "ai-service")
    mk_link(links, nodes, "nginx_control", "nginx-vm1", "control-plane")
    mk_link(links, nodes, "nginx_alert", "nginx-vm1", "alert-engine")
    mk_link(links, nodes, "nginx_edge", "nginx-vm1", "data-plane")
    mk_link(links, nodes, "nginx_sysstats", "nginx-vm1", "system-stats")
    mk_link(links, nodes, "nginx_config", "nginx-vm1", "config-service")
    mk_link(links, nodes, "nginx_log", "nginx-vm1", "log-service")
    mk_link(links, nodes, "nginx_link", "nginx-vm1", "link-metrics")
    mk_link(links, nodes, "nginx_edge_health", "nginx-vm1", "edge-health")
    mk_link(links, nodes, "nginx_cluster", "nginx-vm1", "cluster-api")

    metrics_data = {"timestamp": time.time(), "nodes": nodes, "links": links}


def loop():
    while True:
        try:
            collect()
        except Exception as e:
            print("[ERROR] {}".format(e))
        time.sleep(CHECK_INTERVAL)


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"ok": True}).encode())
        elif self.path in ("/metrics", "/"):
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(metrics_data, indent=2).encode())
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, *a):
        pass


def main():
    t = threading.Thread(target=loop, daemon=True)
    t.start()
    server = HTTPServer(("0.0.0.0", PORT), Handler)
    print("[INFO] Link Metrics on :{}".format(PORT))
    server.serve_forever()


if __name__ == "__main__":
    main()
