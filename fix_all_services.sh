#!/bin/bash
# 修复所有后端服务的路由问题（去掉/api前缀）

SERVICES=(
  'services/alert-engine/service.go'
  'services/control-plane/service.go'
  'services/data-plane/service.go'
  'services/tenant-service/service.go'
  'services/topology-engine/service.go'
)

echo '开始修复后端服务路由...'

for svc in ""; do
  if [ -f "/opt/cloudflow/" ]; then
    echo "处理: "
    # 备份原文件
    cp "/opt/cloudflow/" "/opt/cloudflow/.bak"
    
    # 使用Python安全地替换路由（只替换mux.HandleFunc中的/api前缀）
    python3 << 'PYEOF'
import re

file_path = '/opt/cloudflow/' + ''
try:
    with open(file_path, 'r') as f:
        content = f.read()
    
    # 只替换mux.HandleFunc中的"/api/为"/（避免破坏import）
    # 使用更精确的正则表达式
    pattern = r'mux\.HandleFunc\("/api/'
    replacement = 'mux.HandleFunc("/'
    
    # 只替换route registration，不替换import
    lines = content.split('\n')
    new_lines = []
    for line in lines:
        # 如果是import section，跳过
        if '"github.com' in line and line.strip().startswith('"'):
            new_lines.append(line)
        else:
            # 替换route中的/api前缀
            new_line = line.replace('mux.HandleFunc("/api/', 'mux.HandleFunc("/')
            new_line = new_line.replace('mux.Handle("/api/', 'mux.Handle("/')
            new_lines.append(new_line)
    
    content = '\n'.join(new_lines)
    
    with open(file_path, 'w') as f:
        f.write(content)
    print(f'✓ 已修复: {file_path}')
except Exception as e:
    print(f'✗ 错误: {e}')
PYEOF
  else
    echo "跳过（文件不存在）: "
  fi
done

echo ''
echo '修复完成！现在检查Dockerfile...'

# 修复Dockerfile（将alpine:3.20替换为nginx:alpine）
for dockerfile in services/*/deployments/Dockerfile; do
  if [ -f "" ]; then
    echo "检查: "
    if grep -q 'alpine:3.20' ""; then
      sed -i 's/FROM alpine:3.20/FROM nginx:alpine/' ""
      echo "  ✓ 已修复: "
    fi
  fi
done

echo ''
echo '所有修复完成！'
ENDSCRIPT'
chmod +x /opt/cloudflow/fix_all_services.sh
echo '脚本已创建'
