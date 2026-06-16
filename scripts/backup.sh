#!/bin/bash
# CloudFlow 数据备份脚本
# 功能：备份MySQL/ClickHouse数据 + 配置文件
# 用法：./backup.sh [backup_dir]

set -e

# 配置
BACKUP_DIR=${1:-"/data/backup/cloudflow"}
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_PATH="${BACKUP_DIR}/${BACKUP_DATE}"
RETENTION_DAYS=7

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

# 配置文件路径
CONFIG_FILES=(
    "/etc/cloudflow/config.yaml"
    "/opt/cloudflow/agent/config.yaml"
)

echo "=========================================="
echo "CloudFlow 数据备份开始"
echo "备份目录: ${BACKUP_PATH}"
echo "=========================================="

# 创建备份目录
mkdir -p "${BACKUP_PATH}"

# ============================================
# 1. 备份MySQL数据库
# ============================================
echo "[1/4] 备份MySQL数据库..."
if [ -n "${MYSQL_PASSWORD}" ]; then
    mysqldump -h "${MYSQL_HOST}" -P "${MYSQL_PORT}" -u "${MYSQL_USER}" -p"${MYSQL_PASSWORD}" \
        --single-transaction --routines --triggers "${MYSQL_DATABASE}" \
        > "${BACKUP_PATH}/mysql_${MYSQL_DATABASE}.sql"
else
    mysqldump -h "${MYSQL_HOST}" -P "${MYSQL_PORT}" -u "${MYSQL_USER}" \
        --single-transaction --routines --triggers "${MYSQL_DATABASE}" \
        > "${BACKUP_PATH}/mysql_${MYSQL_DATABASE}.sql"
fi
echo "✓ MySQL备份完成: ${BACKUP_PATH}/mysql_${MYSQL_DATABASE}.sql"

# ============================================
# 2. 备份ClickHouse数据库
# ============================================
echo "[2/4] 备份ClickHouse数据库..."
clickhouse-client -h "${CLICKHOUSE_HOST}" --port "${CLICKHOUSE_PORT}" \
    -u "${CLICKHOUSE_USER}" --password "${CLICKHOUSE_PASSWORD}" \
    --query="BACKUP DATABASE ${CLICKHOUSE_DATABASE} TO Disk('${BACKUP_PATH}/clickhouse')"
echo "✓ ClickHouse备份完成: ${BACKUP_PATH}/clickhouse"

# ============================================
# 3. 备份配置文件
# ============================================
echo "[3/4] 备份配置文件..."
mkdir -p "${BACKUP_PATH}/config"
for config_file in "${CONFIG_FILES[@]}"; do
    if [ -f "${config_file}" ]; then
        cp "${config_file}" "${BACKUP_PATH}/config/"
        echo "  ✓ ${config_file}"
    fi
done
echo "✓ 配置文件备份完成"

# ============================================
# 4. 压缩备份
# ============================================
echo "[4/4] 压缩备份文件..."
cd "${BACKUP_DIR}"
tar -czf "${BACKUP_DATE}.tar.gz" "${BACKUP_DATE}"
rm -rf "${BACKUP_DATE}"
echo "✓ 备份压缩完成: ${BACKUP_DIR}/${BACKUP_DATE}.tar.gz"

# ============================================
# 清理过期备份
# ============================================
echo "清理${RETENTION_DAYS}天前的备份..."
find "${BACKUP_DIR}" -name "*.tar.gz" -type f -mtime +${RETENTION_DAYS} -delete
echo "✓ 过期备份清理完成"

# ============================================
# 备份验证
# ============================================
BACKUP_SIZE=$(du -sh "${BACKUP_DIR}/${BACKUP_DATE}.tar.gz" | cut -f1)
echo ""
echo "=========================================="
echo "✅ 备份完成!"
echo "备份文件: ${BACKUP_DIR}/${BACKUP_DATE}.tar.gz"
echo "备份大小: ${BACKUP_SIZE}"
echo "=========================================="
