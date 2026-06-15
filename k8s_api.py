#!/usr/bin/env python3
import json, subprocess, os
from http.server import HTTPServer, BaseHTTPRequestHandler

KUBECTL = '/usr/local/bin/kubectl'
KUBECONFIG = '/root/.kube/config'

# CORS白名单配置
CORS_ALLOWED_ORIGINS = os.environ.get('CORS_ALLOWED_ORIGINS', 'http://localhost:3000,http://localhost:8080').split(',')
CORS_ALLOWED_ORIGINS = [origin.strip() for origin in CORS_ALLOWED_ORIGINS]

def k8s_get(resource):
    cmd = [KUBECTL, 'get', resource, '-o', 'json', '--kubeconfig', KUBECONFIG, '--request-timeout', '10']
    if resource in ('pods', 'services'):
        cmd.append('--all-namespaces')
    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=15)
        if r.returncode != 0:
            return {'error': r.stderr[:500]}
        return json.loads(r.stdout)
    except Exception as e:
        return {'error': str(e)}

class H(BaseHTTPRequestHandler):
    def _set_cors_headers(self):
        origin = self.headers.get('Origin')
        if origin and origin in CORS_ALLOWED_ORIGINS:
            self.send_header('Access-Control-Allow-Origin', origin)
            self.send_header('Access-Control-Allow-Credentials', 'true')
        self.send_header('Access-Control-Allow-Methods', 'GET, OPTIONS')

    def do_OPTIONS(self):
        self.send_response(204)
        self._set_cors_headers()
        self.end_headers()

    def do_GET(self):
        path = self.path.rstrip('/')
        endpoints = {
            '/api/k8s/nodes': lambda: k8s_get('nodes'),
            '/api/k8s/pods': lambda: k8s_get('pods'),
            '/api/k8s/services': lambda: k8s_get('services'),
            '/api/k8s/namespaces': lambda: k8s_get('namespaces'),
        }
        fn = endpoints.get(path)
        if not fn:
            self.send_error(404)
            return
        d = fn()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self._set_cors_headers()
        self.end_headers()
        self.wfile.write(json.dumps(d).encode())

    def log_message(self, *a): pass

if __name__ == '__main__':
    p = int(os.getenv('PORT', '8011'))
    HTTPServer(('0.0.0.0', p), H).serve_forever()
