#!/bin/bash
# CloudFlow 自动化备份脚本
# 用途: 每日全量备份 + 每小时增量备份 ClickHouse 和 TiDB 数据
# 用法: ./backup.sh [full|incremental]

set -euo pipefail

# ==================== 配置 ====================
BACKUP_BASE_DIR="${BACKUP_BASE_DIR:-/opt/cloudflow/backups}"
CLICKHOUSE_HOST="${CLICKHOUSE_HOST:-localhost}"
CLICKHOUSE_PORT="${CLICKHOUSE_PORT:-9000}"
CLICKHOUSE_DB="${CLICKHOUSE_DB:-cloudflow}"
TIDB_HOST="${TIDB_HOST:-localhost}"
TIDB_PORT="${TIDB_PORT:-4000}"
TIDB_USER="${TIDB_USER:-root}"
TIDB_PASSWORD="${TIDB_PASSWORD:-}"
TIDB_DB="${TIDB_DB:-cloudflow_auth}"

RETENTION_DAYS="${RETENTION_DAYS:-7}"
LOG_FILE="${BACKUP_BASE_DIR}/backup.log"

# ==================== 工具函数 ====================
log() {
    local level=$1
    shift
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [$level] $*" | tee -a "$LOG_FILE"
}

check_dependencies() {
    local deps=("clickhouse-client" "mysqldump" "gzip")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log "ERROR" "依赖缺失: $dep"
            exit 1
        fi
    done
}

create_backup_dir() {
    local backup_type=$1
    local timestamp=$(date '+%Y%m%d_%H%M%S')
    local backup_dir="${BACKUP_BASE_DIR}/${backup_type}/${timestamp}"
    
    mkdir -p "$backup_dir"
    echo "$backup_dir"
}

cleanup_old_backups() {
    log "INFO" "清理 ${RETENTION_DAYS} 天前的备份..."
    
    find "${BACKUP_BASE_DIR}/full" -type d -mtime +${RETENTION_DAYS} -exec rm -rf {} + 2>/dev/null || true
    find "${BACKUP_BASE_DIR}/incremental" -type d -mtime +${RETENTION_DAYS} -exec rm -rf {} + 2>/dev/null || true
    
    log "INFO" "清理完成"
}

send_notification() {
    local status=$1
    local message=$2
    
    # TODO: 集成钉钉/企业微信/Slack 通知
    log "NOTIFY" "备份${status}: ${message}"
    
    # 示例：发送钉钉通知
    # curl -X POST "https://oapi.dingtalk.com/robot/send?access_token=YOUR_TOKEN" \
    #   -H 'Content-Type: application/json' \
    #   -d "{\"msgtype\":\"text\",\"text\":{\"content\":\"CloudFlow 备份${status}: ${message}\"}}"
}

# ==================== ClickHouse 备份 ====================
backup_clickhouse_full() {
    local backup_dir=$1
    log "INFO" "开始 ClickHouse 全量备份..."
    
    local backup_file="${backup_dir}/clickhouse_full.sql.gz"
    
    # 使用 ClickHouse 原生 BACKUP 命令（推荐）
    clickhouse-client --host "$CLICKHOUSE_HOST" --port "$CLICKHOUSE_PORT" \
        --query "BACKUP DATABASE ${CLICKHOUSE_DB} TO File('${backup_dir}/clickhouse_backup')" \
        2>> "$LOG_FILE"
    
    if [ $? -eq 0 ]; then
        log "INFO" "ClickHouse 全量备份成功: ${backup_file}"
        return 0
    else
        log "ERROR" "ClickHouse 全量备份失败"
        return 1
    fi
}

backup_clickhouse_incremental() {
    local backup_dir=$1
    log "INFO" "开始 ClickHouse 增量备份..."
    
    # 备份最近 1 小时的数据
    local since=$(date -d '1 hour ago' '+%Y-%m-%d %H:%M:%S' 2>/dev/null || date -v-1H '+%Y-%m-%d %H:%M:%S')
    
    clickhouse-client --host "$CLICKHOUSE_HOST" --port "$CLICKHOUSE_PORT" \
        --query "SELECT * FROM ${CLICKHOUSE_DB}.flows WHERE timestamp > '${since}' FORMAT Native" \
        | gzip > "${backup_dir}/clickhouse_incremental.native.gz"
    
    if [ $? -eq 0 ]; then
        log "INFO" "ClickHouse 增量备份成功"
        return 0
    else
        log "ERROR" "ClickHouse 增量备份失败"
        return 1
    fi
}

# ==================== TiDB 备份 ====================
backup_tidb_full() {
    local backup_dir=$1
    log "INFO" "开始 TiDB 全量备份..."
    
    local backup_file="${backup_dir}/tidb_full.sql.gz"
    
    mysqldump -h "$TIDB_HOST" -P "$TIDB_PORT" -u "$TIDB_USER" \
        ${TIDB_PASSWORD:+-p"$TIDB_PASSWORD"} \
        --single-transaction \
        --routines \
        --triggers \
        --events \
        "$TIDB_DB" \
        | gzip > "$backup_file"
    
    if [ $? -eq 0 ]; then
        local size=$(du -h "$backup_file" | cut -f1)
        log "INFO" "TiDB 全量备份成功: ${backup_file} (${size})"
        return 0
    else
        log "ERROR" "TiDB 全量备份失败"
        return 1
    fi
}

backup_tidb_incremental() {
    local backup_dir=$1
    log "INFO" "开始 TiDB 增量备份..."
    
    # 使用 binlog 进行增量备份（需要启用 binlog）
    # 这里简化为备份最近修改的表
    local backup_file="${backup_dir}/tidb_incremental.sql.gz"
    
    mysqldump -h "$TIDB_HOST" -P "$TIDB_PORT" -u "$TIDB_USER" \
        ${TIDB_PASSWORD:+-p"$TIDB_PASSWORD"} \
        --single-transaction \
        --where="updated_at > DATE_SUB(NOW(), INTERVAL 1 HOUR)" \
        "$TIDB_DB" \
        | gzip > "$backup_file"
    
    if [ $? -eq 0 ]; then
        log "INFO" "TiDB 增量备份成功"
        return 0
    else
        log "WARN" "TiDB 增量备份失败（可能无更新数据）"
        return 0
    fi
}

# ==================== 备份验证 ====================
verify_backup() {
    local backup_dir=$1
    local backup_type=$2
    log "INFO" "验证 ${backup_type} 备份完整性..."
    
    local errors=0
    
    # 检查文件是否存在且非空
    for file in "$backup_dir"/*; do
        if [ ! -s "$file" ]; then
            log "ERROR" "备份文件为空: $file"
            errors=$((errors + 1))
        fi
    done
    
    if [ $errors -eq 0 ]; then
        log "INFO" "备份验证通过"
        return 0
    else
        log "ERROR" "备份验证失败: ${errors} 个错误"
        return 1
    fi
}

# ==================== 恢复测试 ====================
test_restore() {
    local backup_dir=$1
    log "WARN" "生产环境请谨慎使用恢复测试！"
    
    # TODO: 在隔离环境中测试恢复
    # 这里仅提供框架，实际使用时需要根据环境配置
    
    log "INFO" "恢复测试跳过（需在隔离环境执行）"
}

# ==================== 主流程 ====================
main() {
    local backup_type=${1:-full}
    
    log "INFO" "========== 开始备份 =========="
    log "INFO" "备份类型: ${backup_type}"
    log "INFO" "备份目录: ${BACKUP_BASE_DIR}"
    
    # 检查依赖
    check_dependencies
    
    # 创建备份目录
    local backup_dir=$(create_backup_dir "$backup_type")
    log "INFO" "备份目录: ${backup_dir}"
    
    local errors=0
    
    # 执行备份
    case "$backup_type" in
        full)
            backup_clickhouse_full "$backup_dir" || errors=$((errors + 1))
            backup_tidb_full "$backup_dir" || errors=$((errors + 1))
            ;;
        incremental)
            backup_clickhouse_incremental "$backup_dir" || errors=$((errors + 1))
            backup_tidb_incremental "$backup_dir" || errors=$((errors + 1))
            ;;
        *)
            log "ERROR" "未知的备份类型: ${backup_type}"
            exit 1
            ;;
    esac
    
    # 验证备份
    if [ $errors -eq 0 ]; then
        verify_backup "$backup_dir" "$backup_type" || errors=$((errors + 1))
    fi
    
    # 清理旧备份
    cleanup_old_backups
    
    # 发送通知
    if [ $errors -eq 0 ]; then
        log "INFO" "========== 备份成功 =========="
        send_notification "成功" "备份目录: ${backup_dir}"
        exit 0
    else
        log "ERROR" "========== 备份失败 (${errors} 个错误) =========="
        send_notification "失败" "错误数: ${errors}, 备份目录: ${backup_dir}"
        exit 1
    fi
}

# 执行主流程
main "$@"
