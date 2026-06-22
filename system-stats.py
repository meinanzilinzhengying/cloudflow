#!/usr/bin/env python3
import json, time, os
from http.server import HTTPServer, BaseHTTPRequestHandler

def get_cpu():
    with open('/proc/stat') as f:
        parts = [int(x) for x in f.readline().split()[1:]]
    idle1 = parts[3] + parts[4]
    total1 = sum(parts)
    time.sleep(0.1)
    with open('/proc/stat') as f:
        parts = [int(x) for x in f.readline().split()[1:]]
    idle2 = parts[3] + parts[4]
    total2 = sum(parts)
    diff_total = total2 - total1
    if diff_total == 0:
        return 0.0
    usage = (diff_total - (idle2 - idle1)) / diff_total * 100
    return round(max(0.0, usage), 1)

def get_mem():
    info = {}
    with open('/proc/meminfo') as f:
        for line in f:
            p = line.split()
            info[p[0].rstrip(':')] = int(p[1])
    total = info.get('MemTotal', 1)
    avail = info.get('MemAvailable', info.get('MemFree', 0))
    used = total - avail
    return {
        'total_mb': round(total / 1024),
        'used_mb': round(used / 1024),
        'pct': round(used / total * 100, 1),
    }

def get_disk():
    info = os.statvfs('/')
    total = info.f_blocks * info.f_frsize
    free = info.f_bavail * info.f_frsize
    used = total - free
    return {
        'total_gb': round(total / 1024**3, 1),
        'used_gb': round(used / 1024**3, 1),
        'pct': round(used / total * 100, 1),
    }

def get_net():
    rx = 0
    tx = 0
    with open('/proc/net/dev') as f:
        for line in f.readlines()[2:]:
            parts = line.split()
            rx += int(parts[1])
            tx += int(parts[9])
    return rx, tx

def get_load():
    with open('/proc/loadavg') as f:
        return f.read().split()[:3]

def get_uptime():
    with open('/proc/uptime') as f:
        return int(float(f.read().split()[0]))

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/api/system/stats' or self.path == '/stats':
            mem = get_mem()
            disk = get_disk()
            load = get_load()
            upt = get_uptime()
            net_rx, net_tx = get_net()
            data = {
                'cpu_pct': get_cpu(),
                'mem_total_mb': mem['total_mb'],
                'mem_used_mb': mem['used_mb'],
                'mem_pct': mem['pct'],
                'disk_total_gb': disk['total_gb'],
                'disk_used_gb': disk['used_gb'],
                'disk_pct': disk['pct'],
                'load_1': float(load[0]),
                'load_5': float(load[1]),
                'load_15': float(load[2]),
                'uptime_sec': upt,
                'net_rx_mb': round(net_rx / 1024**2, 1),
                'net_tx_mb': round(net_tx / 1024**2, 1),
                'timestamp': time.strftime('%Y-%m-%dT%H:%M:%S'),
            }
            body = json.dumps(data).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
            self.wfile.write(body)
        elif self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{"status":"ok"}')
        else:
            self.send_response(404)
            self.end_headers()
    def log_message(self, *a):
        pass

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 9099), Handler)
    print('system-stats listening on :9099', flush=True)
    server.serve_forever()
