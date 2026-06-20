#!/usr/bin/env python3
"""CloudFlow Edge HTTP Health Check Service
- 监听 :8081，提供 HTTP 健康检查端点
- 检查 edge 进程是否存活（gRPC 9102 端口是否监听）
- 返回 JSON 健康状态，兼容负载均衡器和监控系统
"""

import json
import logging
import os
import subprocess
import sys
from datetime import datetime
from http.server import HTTPServer, BaseHTTPRequestHandler

EDGE_GRPC_PORT = int(os.getenv("EDGE_GRPC_PORT", "9102"))
HTTP_PORT = int(os.getenv("EDGE_HEALTH_PORT", "8081"))

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger(__name__)

start_time = datetime.now()


def check_edge_alive():
    """检查 edge 进程是否在监听 gRPC 端口"""
    try:
        result = subprocess.run(
            ["ss", "-tlnp", f"sport = :{EDGE_GRPC_PORT}"],
            capture_output=True, text=True, timeout=5
        )
        return "cloudflow-edge" in result.stdout or str(EDGE_GRPC_PORT) in result.stdout
    except Exception as e:
        logger.warning(f"Health check failed: {e}")
        return False


class HealthHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path in ("/health", "/ready", "/live"):
            alive = check_edge_alive()
            status = "healthy" if alive else "unhealthy"
            code = 200 if alive else 503

            response = {
                "status": status,
                "timestamp": datetime.now().isoformat(),
                "uptime": str(datetime.now() - start_time),
                "edge_grpc_port": EDGE_GRPC_PORT,
                "edge_alive": alive
            }

            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(response).encode())
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        logger.info("%s - %s" % (self.address_string(), format % args))


def main():
    server = HTTPServer(("0.0.0.0", HTTP_PORT), HealthHandler)
    logger.info(f"Edge health check server listening on :{HTTP_PORT}")
    logger.info(f"Monitoring edge gRPC port :{EDGE_GRPC_PORT}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
