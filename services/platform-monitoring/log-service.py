#!/usr/bin/env python3
"""CloudFlow 日志服务 :9106 - 采集 + 查询 (v2)
修复：跳过 data-ingest 的访问日志（量太大）
"""
import json, time, subprocess, threading, re
from datetime import datetime
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.request import urlopen, Request
from urllib.error import URLError
from urllib.parse import urlparse, parse_qs



# ========== 配置管理 API ==========
import os as os_config

CONFIG_FILE = '/opt/cloudflow/config.json'
CONFIG_CACHE = {}
CONFIG_MTIME = 0

def load_config():
    """加载配置文件"""
    global CONFIG_CACHE, CONFIG_MTIME
    try:
        mtime = os_config.path.getmtime(CONFIG_FILE)
        if mtime > CONFIG_MTIME:
            with open(CONFIG_FILE) as f:
                CONFIG_CACHE = json.load(f)
            CONFIG_MTIME = mtime
            print(f'[config] Loaded config from {CONFIG_FILE}')
    except Exception as e:
        print(f'[config] Error loading config: {e}')
        CONFIG_CACHE = {}

def save_config():
    """保存配置到文件"""
    try:
        with open(CONFIG_FILE, 'w') as f:
            json.dump(CONFIG_CACHE, f, indent=2, ensure_ascii=False)
        print(f'[config] Saved config to {CONFIG_FILE}')
        return True
    except Exception as e:
        print(f'[config] Error saving config: {e}')
        return False

def update_config_item(category, key, value):
    """更新某个配置项"""
    load_config()
    if category not in CONFIG_CACHE:
        return False, "Category not found"
    if key not in CONFIG_CACHE[category]:
        return False, "Config key not found"
    
    # 验证类型
    config_item = CONFIG_CACHE[category][key]
    if config_item['type'] == 'number':
        if not isinstance(value, (int, float)):
            return False, "Value must be a number"
        if 'min' in config_item and value < config_item['min']:
            return False, f"Value must be >= {config_item['min']}"
        if 'max' in config_item and value > config_item['max']:
            return False, f"Value must be <= {config_item['max']}"
    elif config_item['type'] == 'boolean':
        if not isinstance(value, bool):
            return False, "Value must be a boolean"
    elif config_item['type'] == 'select':
        if value not in config_item['options']:
            return False, f"Value must be one of {config_item['options']}"
    
    CONFIG_CACHE[category][key]['value'] = value
    save_config()
    return True, "OK"

# =======================================

CH_HOST = '127.0.0.1'
CH_PORT = 8123
CH_TABLE = 'cloudflow.platform_logs'

SERVICES = [
    'cloudflow-ai', 'cloudflow-alert-engine', 'cloudflow-control-plane',
    'cloudflow-edge-health', 'cloudflow-edge',
    'cloudflow-link-metrics', 'cloudflow-system-stats',
    # data-ingest 日志量过大（每秒数百条应用日志），暂时禁用采集
    # 需要查看时用：journalctl -u cloudflow-data-ingest --no-pager
]

# data-ingest 访问日志正则：INFO:__main__:IP - "METHOD URI PROTO" STATUS
ACCESS_LOG_RE = re.compile(r'^\S+:\S+:(\d{1,3}\.){3}\d{1,3} - "')

LEVEL_MAP = {'emerg':'ERROR','alert':'ERROR','crit':'ERROR',
             'err':'ERROR','warning':'WARN','notice':'INFO',
             'info':'INFO','debug':'DEBUG'}

LAST_COLLECT = {}
TABLE_READY = False

def ensure_table():
    global TABLE_READY
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
        TABLE_READY = True
        print('[log] Table ready')
    except Exception as e:
        print(f'[log] Table init error: {e}')

def import_logs():
    global LAST_COLLECT
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
                    # 跳过 systemd 自身消息（SYSLOG_IDENTIFIER=systemd）
                    if entry.get('SYSLOG_IDENTIFIER') == 'systemd':
                        continue
                    ts_us = int(entry.get('__REALTIME_TIMESTAMP', '0'))
                    ts = datetime.fromtimestamp(ts_us / 1_000_000)
                    prio = entry.get('PRIORITY', '6')
                    level = LEVEL_MAP.get(prio, 'INFO')
                    message = entry.get('MESSAGE', '')[:500]
                    if not message:
                        continue
                    # 跳过 data-ingest 的访问日志（已禁用，保留此过滤作为双重保护）
                    if svc == 'cloudflow-data-ingest' and ACCESS_LOG_RE.match(message):
                        continue
                    # 跳过纯健康检查日志
                    if 'GET /health HTTP' in message or 'GET /metrics HTTP' in message:
                        continue
                    rows.append((ts, svc.replace('cloudflow-', ''), level, message))
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
    """查询日志"""
    conditions = []
    service = params.get("service", [""])[0]
    level   = params.get("level", [""])[0]
    keyword = params.get("keyword", [""])[0]
    start   = params.get("start", [""])[0]
    end     = params.get("end", [""])[0]
    limit   = int(params.get("limit", ["200"])[0])

    if service:
        conditions.append(f"service = '{service}'")
    if level:
        conditions.append(f"level = '{level}'")
    if keyword:
        conditions.append(f"message ILIKE '%{keyword}%'")
    if start:
        conditions.append(f"timestamp >= '{start}'")
    if end:
        conditions.append(f"timestamp <= '{end}'")

    where_clause = ("WHERE " + " AND ".join(conditions)) if conditions else ""

    query = (f"SELECT timestamp, service, level, message, host "
             f"FROM {CH_TABLE} {where_clause} "
             f"ORDER BY timestamp DESC LIMIT {limit} FORMAT JSON")

    print(f"[log] Query: {query[:200]}")

    try:
        import subprocess
        result = subprocess.run(
            ["curl", "-s", f"http://{CH_HOST}:{CH_PORT}/", "--data-binary", query],
            capture_output=True, text=True, timeout=15
        )
        if result.returncode == 0:
            data = json.loads(result.stdout)
            return data.get("data", [])
        else:
            print(f"[log] Curl error: {result.stderr}")
            return []
    except Exception as e:
        print(f"[log] Query error: {e}")
        return []


def collect_loop():
    while True:
        try:
            if TABLE_READY:
                import_logs()
        except Exception as e:
            print(f'[log] Collect loop error: {e}')
        time.sleep(15)

class LogHandler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass  # 禁用访问日志

    def do_GET(self):
        parsed = urlparse(self.path)
        params = parse_qs(parsed.query)
        if parsed.path.endswith('/logs/query') or parsed.path == '/logs/query':
            logs = query_logs(params)
            body = json.dumps({'logs': logs, 'total': len(logs)}, ensure_ascii=False).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json; charset=utf-8')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
            self.wfile.write(body)
        elif parsed.path == '/config' or parsed.path == '/config/':
        # 配置管理 API
            load_config()
            body = json.dumps(CONFIG_CACHE, ensure_ascii=False).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json; charset=utf-8')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
            self.wfile.write(body)
        
        elif parsed.path.startswith('/config/'):
            parts = parsed.path.split('/')
            # 移除空字符串
            parts = [p for p in parts if p]
            # 路径格式：api/logs/config/<category> 或 api/logs/config/<category>/<key>
            if len(parts) >= 4:
                category = parts[3]
                if len(parts) >= 5:
                    key = parts[4]
                    item = CONFIG_CACHE.get(category, {}).get(key)
                    if item:
                        body = json.dumps(item, ensure_ascii=False).encode()
                        self.send_response(200)
                    else:
                        body = json.dumps({'error': 'Not found'}, ensure_ascii=False).encode()
                        self.send_response(404)
                else:
                    cat_config = CONFIG_CACHE.get(category, {})
                    body = json.dumps(cat_config, ensure_ascii=False).encode()
                    self.send_response(200)
                
                self.send_header('Content-Type', 'application/json; charset=utf-8')
                self.send_header('Access-Control-Allow-Origin', '*')
                self.end_headers()
                self.wfile.write(body)
                return
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        parsed = urlparse(self.path)
        if parsed.path.endswith('/logs/import') or parsed.path == '/logs/import':
            count = import_logs()
            body = json.dumps({'imported': count}, ensure_ascii=False).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json; charset=utf-8')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
            self.wfile.write(body)
        # 配置更新 API
        elif parsed.path.startswith('/config/'):
            parts = parsed.path.split('/')
            parts = [p for p in parts if p]
            # 路径格式：api/logs/config/<category>/<key>
            if len(parts) >= 5:
                category = parts[3]
                key = parts[4]
                
                content_length = int(self.headers['Content-Length'])
                body = self.rfile.read(content_length)
                data = json.loads(body)
                new_value = data.get('value')
                
                success, msg = update_config_item(category, key, new_value)
                
                if success:
                    self.send_response(200)
                    response = json.dumps({'success': True, 'message': msg}, ensure_ascii=False).encode()
                else:
                    self.send_response(400)
                    response = json.dumps({'success': False, 'message': msg}, ensure_ascii=False).encode()
                
                self.send_header('Content-Type', 'application/json; charset=utf-8')
                self.send_header('Access-Control-Allow-Origin', '*')
                self.end_headers()
                self.wfile.write(response)
                return
            self.send_response(404)
            self.end_headers()

if __name__ == '__main__':
    ensure_table()
    t = threading.Thread(target=collect_loop, daemon=True)
    t.start()
    print('[log] Log service started on :9106')
    server = HTTPServer(('0.0.0.0', 9106), LogHandler)
    server.serve_forever()
