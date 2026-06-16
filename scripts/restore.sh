#!/bin/bash
# CloudFlow 恢复脚本

set -e

BACKUP_DIR="/var/backups/cloudflow"

echo "=== CloudFlow 恢复开始 $(date) ==="

if [ -z "$1" ]; then
    echo "用法: $0 <备份时间戳>"
    echo "可用备份:"
    ls -lh "$BACKUP_DIR"
    exit 1
fi

TIMESTAMP="$1"

# 1. MySQL恢复
echo "1/3 恢复MySQL数据库..."
if [ -f "$BACKUP_DIR/mysql_${TIMESTAMP}.sql" ]; then
    MYSQL_HOST="${MYSQL_HOST:-localhost}"
    MYSQL_PORT="${MYSQL_PORT:-3306}"
    MYSQL_USER="${MYSQL_USER:-root}"
    MYSQL_PASSWORD="${MYSQL_PASSWORD:-}"
    MYSQL_DATABASE="${MYSQL_DATABASE:-cloudflow}"
    
    mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" \
        "$MYSQL_DATABASE" < "$BACKUP_DIR/mysql_${TIMESTAMP}.sql"
    echo "   MySQL恢复完成"
else
    echo "   MySQL备份文件不存在，跳过"
fi

# 2. 配置文件恢复
echo "2/3 恢复配置文件..."
if [ -f "$BACKUP_DIR/config_${TIMESTAMP}.tar.gz" ]; then
    tar -xzf "$BACKUP_DIR/config_${TIMESTAMP}.tar.gz" -C /
    echo "   配置文件恢复完成"
else
    echo "   配置备份文件不存在，跳过"
fi

# 3. 验证
echo "3/3 验证恢复..."
echo "   请手动验证服务状态"

echo "=== CloudFlow 恢复完成 $(date) ==="
