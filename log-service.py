#!/usr/bin/env python3
"""CloudFlow 日志服务 :9106 - 采集 + 查询"""
import json, time, subprocess, threading
from datetime import datetime
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.request import urlopen, Request
from urllib.error import URLError
from urllib.parse import urlparse, parse_qs

CH_HOST = '127.0.0.1'
CH_PORT = 8123
CH_TABLE = 'cloudflow.platform_logs'

SERVICES = [
    'cloudflow-ai', 'cloudflow-alert-engine', 'cloudflow-control-plane',
    'cloudflow-data-ingest', 'cloudflow-edge-health', 'cloudflow-edge',
    'cloudflow-link-metrics', 'cloudflow-system-stats', 'cloudflow-data-plane',
]

LEVEL_MAP = {'emerg':'ERROR','alert':'ERROR','crit':'ERROR',
             'err':'ERROR','warning':'WARN','notice':'INFO',
             'info':'INFO','debug':'DEBUG'}

LAST_COLLECT = {}

def ensure_table():
    sql = ('CREATE TABLE IF NOT EXISTS cloudflow.platform_logs ('
            'timestamp DateTime64(3), service String, level String, '
            'message String, host String DEFAULT \'192.168.58.130\', '
            'module String DEFAULT \'\') '
            'ENGINE = MergeTree() '
            'ORDER BY (service, timestamp) '
            'TTL timestamp + INTERVAL 7 DAY')
    try:
        data = sql.replace('\n', ' ').encode()
        req = Request(f'http://{CH_HOST}:{CH_PORT}/', data=data)
        urlopen(req, timeout=5)
        print('[log] Table ready')
    except Exception as e:
        print(f'[log] Table init: {e}')

def import_logs():
    total = 0
    for svc in SERVICES:
        since = LAST_COLLECT.get(svc, '5 min ago')
        cmd = ['journalctl', '-u', svc, '--no-pager', '--output=json', '--since', since]
        try:
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
            rows = []
            for line in result.stdout.strip().split('\n'):
                if not line.strip():
                    continue
                try:
                    entry = json.loads(line)
                    ts_us = int(entry.get('__REALTIME_TIMESTAMP', '0'))
                    ts = datetime.fromtimestamp(ts_us / 1_000_000)
                    prio = entry.get('PRIORITY', '6')
                    level = LEVEL_MAP.get(prio, 'INFO')
                    message = entry.get('MESSAGE', '')[:500]
                    if not message:
                        continue
                    rows.append((ts, svc.replace('cloudflow-',''), level, message))
                except (json.JSONDecodeError, ValueError):
                    continue

            if rows:
                tsv_lines = []
                for r in rows:
                    msg = r[3].replace('\\', '\\\\').replace('\t', ' ').replace('\n', ' ')
                    line = f"{r[0].strftime('%Y-%m-%d %H:%M:%S.%f')[:-3]}\t{r[1]}\t{r[2]}\t{msg}\t192.168.58.130\t\n"
                    tsv_lines.append(line)
                tsv_data = ''.join(tsv_lines)
                url_path = f"?query=INSERT+INTO+{CH_TABLE}+FORMAT+TSV"
                req = Request(f'http://{CH_HOST}:{CH_PORT}/{url_path}', data=tsv_data.encode())
                urlopen(req, timeout=10)
                total += len(rows)
                print(f'[log] {svc}: +{len(rows)}')
                LAST_COLLECT[svc] = '1 min ago'
        except Exception as e:
            print(f'[log] Import {svc} error: {e}')
    return total

def query_logs(params):
    where = []
    service = params.get('service', [''])[0]
    level = params.get('level', [''])[0]
    keyword = params.get('keyword', [''])[0]
    start = params.get('start', [''])[0]
    end = params.get('end', [''])[0]
    limit = int(params.get('limit', ['200'])[0])

    if service: where.append(f"service = '{service}'")
    if level: where.append(f"level = '{level}'")
    if keyword: where.append(f"message LIKE '%{keyword}%'")
    if start: where.append(f"timestamp >= '{start}'")
    if end: where.append(f"timestamp <= '{end}'")

    where_str = 'WHERE ' + ' AND '.join(where) if where else ''
    sql = f"SELECT timestamp,service,level,message,host FROM {CH_TABLE} {where_str} ORDER BY timestamp DESC LIMIT {limit}"
    sql = sql.replace('\n', ' ')

    try:
        url_path = f"?query={sql.replace(' ', '+')}"
        req = Request(f'http://{CH_HOST}:{CH_PORT}/{url_path}')
        resp = urlopen(req, timeout=10)
        rows = []
        for line in resp.read().decode().strip().split('\n'):
            if not line.strip(): continue
            parts = line.split('\t')
            if len(parts) >= 4:
                rows.append({'timestamp': parts[0], 'service': parts[1],
                             'level': parts[2], 'message': parts[3],
                             'host': parts[4] if len(parts) > 4 else ''})
        return {'logs': rows, 'total': len(rows)}
    except Exception as e:
        return {'error': str(e)}

def collect_loop():
    while True:
        try:
            n = import_logs()
            if n == 0:
                print(f'[log] No new logs ({datetime.now().strftime("%H:%M:%S")})')
        except Exception as e:
            print(f'[log] Collect error: {e}')
        time.sleep(15)

class Handler(BaseHTTPRequestHandler):
    def _send(self, data, status=200):
        body = json.dumps(data, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json; charset=utf-8')
        self.send_header('Content-Length', str(len(body)))
        self.send_header('Access-Control-Allow-Origin', '*')
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        # nginx proxy_pass 带尾斜杠时会剥离 /api/logs/ 前缀
        # 所以实际路径可能是 /query 或 /health 等
        path = self.path.split('?')[0]
        if path.startswith('/api/logs/query') or path.startswith('/query'):
            parsed = urlparse(self.path)
            params = parse_qs(parsed.query)
            result = query_logs(params)
            self._send(result)
        elif path == '/api/logs/health' or path == '/health':
            self._send({'status': 'ok'})
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        path = self.path.split('?')[0]
        if path == '/api/logs/import' or path == '/import':
            def do_import():
                n = import_logs()
                print(f'[log] Import completed: {n} logs')
            t = threading.Thread(target=do_import, daemon=True)
            t.start()
            self._send({'status': 'import started'})
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, *args):
        pass

if __name__ == '__main__':
    ensure_table()
    threading.Thread(target=collect_loop, daemon=True).start()
    print('[log] Service started :9106')
    HTTPServer(('0.0.0.0', 9106), Handler).serve_forever()
