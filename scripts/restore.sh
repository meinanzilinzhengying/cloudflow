#!/bin/bash
# CloudFlow 数据恢复脚本
# 功能：从备份恢复MySQL/ClickHouse数据 + 配置文件
# 用法：./restore.sh <backup_file.tar.gz>

set -e

if [ $# -lt 1 ]; then
    echo "用法: $0 <backup_file.tar.gz>"
    echo "示例: $0 /data/backup/cloudflow/20240616_120000.tar.gz"
    exit 1
fi

BACKUP_FILE="$1"
RESTORE_TMP_DIR="/tmp/cloudflow_restore_$(date +%s)"

# MySQL配置
MYSQL_HOST=${MYSQL_HOST:-"localhost"}
MYSQL_PORT=${MYSQL_PORT:-"3306"}
MYSQL_USER=${MYSQL_USER:-"root"}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-""}
MYSQL_DATABASE=${MYSQL_DATABASE:-"cloudflow"}

# ClickHouse配置
CLICKHOUSE_HOST=${CLICKHOUSE_HOST:-"localhost"}
CLICKHOUSE_PORT=${CLICKHOUSE_PORT:-"9000"}
CLICKHOUSE_USER=${CLICKHOUSE_USER:-"default"}
CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD:-""}
CLICKHOUSE_DATABASE=${CLICKHOUSE_DATABASE:-"cloudflow"}

echo "=========================================="
echo "CloudFlow 数据恢复开始"
echo "备份文件: ${BACKUP_FILE}"
echo "临时目录: ${RESTORE_TMP_DIR}"
echo "=========================================="

# 验证备份文件存在
if [ ! -f "${BACKUP_FILE}" ]; then
    echo "❌ 备份文件不存在: ${BACKUP_FILE}"
    exit 1
fi

# 创建临时目录
mkdir -p "${RESTORE_TMP_DIR}"

# ============================================
# 1. 解压备份
# ============================================
echo "[1/4] 解压备份文件..."
tar -xzf "${BACKUP_FILE}" -C "${RESTORE_TMP_DIR}"
BACKUP_CONTENT=$(ls "${RESTORE_TMP_DIR}")
echo "✓ 备份解压完成"

# ============================================
# 2. 恢复MySQL数据库
# ============================================
echo "[2/4] 恢复MySQL数据库..."
MYSQL_DUMP=$(find "${RESTORE_TMP_DIR}" -name "mysql_*.sql" | head -1)
if [ -n "${MYSQL_DUMP}" ]; then
    echo "  恢复文件: ${MYSQL_DUMP}"
    if [ -n "${MYSQL_PASSWORD}" ]; then
        mysql -h "${MYSQL_HOST}" -P "${MYSQL_PORT}" -u "${MYSQL_USER}" -p"${MYSQL_PASSWORD}" \
            "${MYSQL_DATABASE}" < "${MYSQL_DUMP}"
    else
        mysql -h "${MYSQL_HOST}" -P "${MYSQL_PORT}" -u "${MYSQL_USER}" \
            "${MYSQL_DATABASE}" < "${MYSQL_DUMP}"
    fi
    echo "✓ MySQL恢复完成"
else
    echo "⚠️  未找到MySQL备份文件，跳过"
fi

# ============================================
# 3. 恢复ClickHouse数据库
# ============================================
echo "[3/4] 恢复ClickHouse数据库..."
CLICKHOUSE_BACKUP=$(find "${RESTORE_TMP_DIR}" -name "clickhouse" -type d | head -1)
if [ -n "${CLICKHOUSE_BACKUP}" ]; then
    echo "  恢复目录: ${CLICKHOUSE_BACKUP}"
    clickhouse-client -h "${CLICKHOUSE_HOST}" --port "${CLICKHOUSE_PORT}" \
        -u "${CLICKHOUSE_USER}" --password "${CLICKHOUSE_PASSWORD}" \
        --query="RESTORE DATABASE ${CLICKHOUSE_DATABASE} FROM Disk('${CLICKHOUSE_BACKUP}')"
    echo "✓ ClickHouse恢复完成"
else
    echo "⚠️  未找到ClickHouse备份文件，跳过"
fi

# ============================================
# 4. 恢复配置文件
# ============================================
echo "[4/4] 恢复配置文件..."
CONFIG_BACKUP=$(find "${RESTORE_TMP_DIR}" -name "config" -type d | head -1)
if [ -n "${CONFIG_BACKUP}" ]; then
    echo "  配置文件目录: ${CONFIG_BACKUP}"
    ls -la "${CONFIG_BACKUP}/"
    echo ""
    echo "⚠️  请手动复制配置文件到目标位置:"
    echo "  cp ${CONFIG_BACKUP}/* /etc/cloudflow/"
    echo "  cp ${CONFIG_BACKUP}/* /opt/cloudflow/agent/"
else
    echo "⚠️  未找到配置文件备份，跳过"
fi

# 清理临时目录
rm -rf "${RESTORE_TMP_DIR}"

echo ""
echo "=========================================="
echo "✅ 数据恢复完成!"
echo "=========================================="
echo ""
echo "⚠️  恢复后检查清单:"
echo "  1. 验证数据库数据完整性"
echo "  2. 重启所有服务使配置生效"
echo "  3. 检查服务日志确认正常运行"
