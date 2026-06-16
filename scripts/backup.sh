#!/bin/bash
# CloudFlow 备份脚本

set -e

BACKUP_DIR="/var/backups/cloudflow"
DATE=$(date +%Y%m%d_%H%M%S)
RETENTION_DAYS=7

# 创建备份目录
mkdir -p "$BACKUP_DIR"

echo "=== CloudFlow 备份开始 $(date) ==="

# 1. MySQL备份
echo "1/4 备份MySQL数据库..."
if command -v mysqldump &> /dev/null; then
    MYSQL_HOST="${MYSQL_HOST:-localhost}"
    MYSQL_PORT="${MYSQL_PORT:-3306}"
    MYSQL_USER="${MYSQL_USER:-root}"
    MYSQL_PASSWORD="${MYSQL_PASSWORD:-}"
    MYSQL_DATABASE="${MYSQL_DATABASE:-cloudflow}"
    
    mysqldump -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" \
        --single-transaction --routines --triggers "$MYSQL_DATABASE" \
        > "$BACKUP_DIR/mysql_$DATE.sql"
    echo "   MySQL备份完成: $BACKUP_DIR/mysql_$DATE.sql"
else
    echo "   mysqldump未找到，跳过MySQL备份"
fi

# 2. ClickHouse备份
echo "2/4 备份ClickHouse数据库..."
if command -v clickhouse-client &> /dev/null; then
    CLICKHOUSE_HOST="${CLICKHOUSE_HOST:-localhost}"
    CLICKHOUSE_PORT="${CLICKHOUSE_PORT:-9000}"
    CLICKHOUSE_USER="${CLICKHOUSE_USER:-default}"
    CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD:-}"
    CLICKHOUSE_DATABASE="${CLICKHOUSE_DATABASE:-cloudflow}"
    
    clickhouse-client -h "$CLICKHOUSE_HOST" --port "$CLICKHOUSE_PORT" \
        -u "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" \
        --query="BACKUP DATABASE $CLICKHOUSE_DATABASE TO Disk('backups', 'clickhouse_$DATE')"
    echo "   ClickHouse备份完成"
else
    echo "   clickhouse-client未找到，跳过ClickHouse备份"
fi

# 3. 配置文件备份
echo "3/4 备份配置文件..."
CONFIG_DIR="${CONFIG_DIR:-/opt/cloudflow/config}"
if [ -d "$CONFIG_DIR" ]; then
    tar -czf "$BACKUP_DIR/config_$DATE.tar.gz" -C "$(dirname "$CONFIG_DIR")" "$(basename "$CONFIG_DIR")"
    echo "   配置文件备份完成: $BACKUP_DIR/config_$DATE.tar.gz"
else
    echo "   配置目录不存在，跳过"
fi

# 4. 清理旧备份
echo "4/4 清理${RETENTION_DAYS}天前的旧备份..."
find "$BACKUP_DIR" -type f -mtime +$RETENTION_DAYS -delete
echo "   清理完成"

echo "=== CloudFlow 备份完成 $(date) ==="
echo "备份目录: $BACKUP_DIR"
ls -lh "$BACKUP_DIR"
