#!/usr/bin/env python3
import json, os, subprocess
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs

CONFIG_FILE = '/opt/cloudflow/config.json'
DEFAULT_CONFIG = {
    "logs": {
        "retention_days":   {"value": 7,   "type": "number", "min": 1,   "max": 90,  "description": "日志保留天数（ClickHouse TTL）"},
        "collect_interval": {"value": 15,  "type": "number", "min": 5,   "max": 300, "description": "日志采集间隔（秒）"},
        "level_filter":     {"value": "INFO", "type": "select", "options": ["DEBUG","INFO","WARN","ERROR"], "description": "采集的最低日志级别"},
    },
    "alerts": {
        "eval_interval":     {"value": 15,  "type": "number", "min": 5,   "max": 300,  "description": "告警评估间隔（秒）"},
        "cpu_threshold":     {"value": 80,  "type": "number", "min": 50,  "max": 100,  "description": "CPU使用率阈值（%）"},
        "memory_threshold":  {"value": 85,  "type": "number", "min": 50,  "max": 100,  "description": "内存使用率阈值（%）"},
        "disk_threshold":    {"value": 90,  "type": "number", "min": 50,  "max": 100,  "description": "磁盘使用率阈值（%）"},
    },
    "collectors": {
        "link_metrics_interval": {"value": 10,  "type": "number", "min": 5,   "max": 300,  "description": "链路指标采集间隔（秒）"},
        "system_stats_interval": {"value": 15,  "type": "number", "min": 5,   "max": 300,  "description": "系统指标采集间隔（秒）"},
        "data_retention_days":  {"value": 30,  "type": "number", "min": 1,   "max": 365,  "description": "eBPF数据保留天数"},
    },
    "services": {
        "auto_restart":        {"value": True,  "type": "boolean", "description": "服务崩溃时自动重启"},
        "health_check_interval": {"value": 30,   "type": "number", "min": 10,  "max": 600, "description": "健康检查间隔（秒）"},
    },
}

def load_config():
    try:
        with open(CONFIG_FILE) as f:
            return json.load(f)
    except:
        save_config(DEFAULT_CONFIG)
        return DEFAULT_CONFIG

def save_config(config):
    try:
        with open(CONFIG_FILE, 'w') as f:
            json.dump(config, f, ensure_ascii=False, indent=2)
        return True
    except Exception as e:
        print(f'[config] Save error: {e}')
        return False

def validate_config_value(cfg, new_value):
    if cfg['type'] == 'number':
        try:
            new_value = int(new_value)
            if 'min' in cfg and new_value < cfg['min']:
                return False, f"值不能小于 {cfg['min']}"
            if 'max' in cfg and new_value > cfg['max']:
                return False, f"值不能大于 {cfg['max']}"
            return True, new_value
        except:
            return False, "请输入数字"
    elif cfg['type'] == 'select':
        if new_value not in cfg.get('options', []):
            return False, f"可选值：{cfg.get('options', [])}"
        return True, new_value
    elif cfg['type'] == 'boolean':
        return True, bool(new_value)
    return True, new_value

def apply_config(config):
    try:
        retention = config['logs']['retention_days']['value']
        sql = f"ALTER TABLE cloudflow.platform_logs MODIFY TTL timestamp + INTERVAL {retention} DAY"
        subprocess.run(['curl', '-s', 'http://localhost:8123/', '--data-binary', sql], timeout=10)
        return True, f"配置已应用（日志保留={retention}天）"
    except Exception as e:
        return False, str(e)

class ConfigHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def send_json(self, data, status=200):
        body = json.dumps(data, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json; charset=utf-8')
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Content-Length', len(body))
        self.end_headers()
        self.wfile.write(body)

    def do_OPTIONS(self):
        self.send_response(200)
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type')
        self.end_headers()

    def do_GET(self):
        parsed = urlparse(self.path)
        path = parsed.path
        params = parse_qs(parsed.query)

        if path == '/api/config':
            config = load_config()
            category = params.get('category', [None])[0]
            key = params.get('key', [None])[0]
            if category and key:
                if category in config and key in config[category]:
                    return self.send_json({category: {key: config[category][key]}})
                else:
                    return self.send_json({'error': 'Not found'}, 404)
            return self.send_json(config)
        
        self.send_json({'error': 'Not found'}, 404)

    def do_POST(self):
        parsed = urlparse(self.path)
        path = parsed.path
        
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length) if content_length > 0 else b'{}'
        try:
            data = json.loads(body)
        except:
            return self.send_json({'error': 'Invalid JSON'}, 400)

        if path == '/api/config':
            category = data.get('category')
            key = data.get('key')
            if not category or not key:
                return self.send_json({'error': 'Missing category or key'}, 400)
            
            config = load_config()
            if category not in config:
                return self.send_json({'error': f'Category not found: {category}'}, 404)
            if key not in config[category]:
                return self.send_json({'error': f'Config key not found: {key}'}, 404)
            
            new_value = data.get('value')
            if new_value is None:
                return self.send_json({'error': 'Missing value field'}, 400)
            
            valid, result = validate_config_value(config[category][key], new_value)
            if not valid:
                return self.send_json({'error': result}, 400)
            
            config[category][key]['value'] = result
            if save_config(config):
                return self.send_json({'success': True, 'message': '配置已更新'})
            else:
                return self.send_json({'error': 'Failed to save config'}, 500)
        
        elif path == '/api/config/apply':
            config = load_config()
            success, msg = apply_config(config)
            if success:
                return self.send_json({'success': True, 'message': msg})
            else:
                return self.send_json({'error': msg}, 500)
        
        self.send_json({'error': 'Not found'}, 404)

def run_server(port=9108):
    server = HTTPServer(('0.0.0.0', port), ConfigHandler)
    print(f'[config] Service started on port {port}')
    server.serve_forever()

if __name__ == '__main__':
    run_server()
