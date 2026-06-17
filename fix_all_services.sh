#!/bin/bash
# 修复所有后端服务的路由问题（去掉 /api 前缀）

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$REPO_ROOT"

SERVICES=(
  'services/alert-engine/service.go'
  'services/control-plane/service.go'
  'services/data-plane/service.go'
  'services/tenant-service/service.go'
  'services/topology-engine/service.go'
)

echo '开始修复后端服务路由...'

for svc in "${SERVICES[@]}"; do
  full_path="$REPO_ROOT/$svc"
  if [ -f "$full_path" ]; then
    echo "处理: $svc"
    cp "$full_path" "${full_path}.bak"
    sed -i 's/mux\.HandleFunc("\/api\//mux.HandleFunc("\//g' "$full_path"
    sed -i 's/mux\.Handle("\/api\//mux.Handle("\//g' "$full_path"
    echo "  ✓ 已修复: $svc"
  else
    echo "  跳过（文件不存在）: $svc"
  fi
done

echo ''
echo '修复完成！备份文件保存在 .bak'
