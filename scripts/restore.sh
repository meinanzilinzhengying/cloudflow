#!/bin/bash
# CloudFlow 数据恢复脚本
# 用途: 从备份恢复 ClickHouse 和 TiDB 数据
# 用法: ./restore.sh <backup_dir> [--dry-run]

set -euo pipefail

# ==================== 配置 ====================
CLICKHOUSE_HOST="${CLICKHOUSE_HOST:-localhost}"
CLICKHOUSE_PORT="${CLICKHOUSE_PORT:-9000}"
CLICKHOUSE_DB="${CLICKHOUSE_DB:-cloudflow}"
TIDB_HOST="${TIDB_HOST:-localhost}"
TIDB_PORT="${TIDB_PORT:-4000}"
TIDB_USER="${TIDB_USER:-root}"
TIDB_PASSWORD="${TIDB_PASSWORD:-}"
TIDB_DB="${TIDB_DB:-cloudflow_auth}"

LOG_FILE="/opt/cloudflow/backups/restore.log"
DRY_RUN=false

# ==================== 参数解析 ====================
parse_args() {
    if [ $# -lt 1 ]; then
        echo "用法: $0 <backup_dir> [--dry-run]"
        echo ""
        echo "参数:"
        echo "  backup_dir  备份目录路径"
        echo "  --dry-run   仅显示恢复计划，不执行实际操作"
        exit 1
    fi
    
    BACKUP_DIR=$1
    shift
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            *)
                echo "未知参数: $1"
                exit 1
                ;;
        esac
    done
}

# ==================== 工具函数 ====================
log() {
    local level=$1
    shift
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [$level] $*" | tee -a "$LOG_FILE"
}

check_backup_dir() {
    if [ ! -d "$BACKUP_DIR" ]; then
        log "ERROR" "备份目录不存在: ${BACKUP_DIR}"
        exit 1
    fi
    
    # 检查备份文件
    local clickhouse_backup=$(find "$BACKUP_DIR" -name "clickhouse_*" -type f | head -1)
    local tidb_backup=$(find "$BACKUP_DIR" -name "tidb_*" -type f | head -1)
    
    if [ -z "$clickhouse_backup" ] && [ -z "$tidb_backup" ]; then
        log "ERROR" "备份目录中未找到有效的备份文件"
        exit 1
    fi
    
    log "INFO" "找到备份文件:"
    [ -n "$clickhouse_backup" ] && log "INFO" "  ClickHouse: $clickhouse_backup"
    [ -n "$tidb_backup" ] && log "INFO" "  TiDB: $tidb_backup"
}

confirm_restore() {
    if [ "$DRY_RUN" = true ]; then
        log "INFO" "[DRY RUN] 跳过确认步骤"
        return 0
    fi
    
    echo ""
    echo "⚠️  警告: 此操作将恢复数据，可能覆盖现有数据！"
    echo "备份目录: ${BACKUP_DIR}"
    echo ""
    read -p "确认继续？(yes/no): " confirm
    
    if [ "$confirm" != "yes" ]; then
        log "INFO" "用户取消恢复操作"
        exit 0
    fi
}

# ==================== ClickHouse 恢复 ====================
restore_clickhouse() {
    local backup_file=$1
    log "INFO" "开始恢复 ClickHouse 数据..."
    
    if [ "$DRY_RUN" = true ]; then
        log "INFO" "[DRY RUN] 将执行: clickhouse-client --query \"RESTORE DATABASE ...\""
        return 0
    fi
    
    # 使用 ClickHouse RESTORE 命令
    if [[ "$backup_file" == *.sql.gz ]]; then
        # SQL 格式恢复
        gunzip -c "$backup_file" | \
            clickhouse-client --host "$CLICKHOUSE_HOST" --port "$CLICKHOUSE_PORT" \
            --database "$CLICKHOUSE_DB" --multiquery
        
    elif [[ "$backup_file" == *.native.gz ]]; then
        # Native 格式恢复
        gunzip -c "$backup_file" | \
            clickhouse-client --host "$CLICKHOUSE_HOST" --port "$CLICKHOUSE_PORT" \
            --database "$CLICKHOUSE_DB" --format Native --query "INSERT INTO flows FORMAT Native"
        
    else
        # File 格式恢复（BACKUP/RESTORE）
        local backup_path=$(dirname "$backup_file")
        clickhouse-client --host "$CLICKHOUSE_HOST" --port "$CLICKHOUSE_PORT" \
            --query "RESTORE DATABASE ${CLICKHOUSE_DB} FROM File('${backup_path}/clickhouse_backup')"
    fi
    
    if [ $? -eq 0 ]; then
        log "INFO" "ClickHouse 数据恢复成功"
        return 0
    else
        log "ERROR" "ClickHouse 数据恢复失败"
        return 1
    fi
}

# ==================== TiDB 恢复 ====================
restore_tidb() {
    local backup_file=$1
    log "INFO" "开始恢复 TiDB 数据..."
    
    if [ "$DRY_RUN" = true ]; then
        log "INFO" "[DRY RUN] 将执行: mysql < backup.sql"
        return 0
    fi
    
    # 恢复 SQL 备份
    gunzip -c "$backup_file" | \
        mysql -h "$TIDB_HOST" -P "$TIDB_PORT" -u "$TIDB_USER" \
        ${TIDB_PASSWORD:+-p"$TIDB_PASSWORD"} \
        "$TIDB_DB"
    
    if [ $? -eq 0 ]; then
        log "INFO" "TiDB 数据恢复成功"
        return 0
    else
        log "ERROR" "TiDB 数据恢复失败"
        return 1
    fi
}

# ==================== 验证恢复 ====================
verify_restore() {
    log "INFO" "验证数据恢复完整性..."
    
    local errors=0
    
    # 验证 ClickHouse
    if command -v clickhouse-client &> /dev/null; then
        local ch_count=$(clickhouse-client --host "$CLICKHOUSE_HOST" --port "$CLICKHOUSE_PORT" \
            --query "SELECT count() FROM ${CLICKHOUSE_DB}.flows" 2>/dev/null || echo "0")
        log "INFO" "ClickHouse flows 表记录数: ${ch_count}"
        
        if [ "$ch_count" = "0" ]; then
            log "WARN" "ClickHouse flows 表为空"
            errors=$((errors + 1))
        fi
    fi
    
    # 验证 TiDB
    if command -v mysql &> /dev/null; then
        local tidb_count=$(mysql -h "$TIDB_HOST" -P "$TIDB_PORT" -u "$TIDB_USER" \
            ${TIDB_PASSWORD:+-p"$TIDB_PASSWORD"} \
            -N -e "SELECT count(*) FROM ${TIDB_DB}.users" 2>/dev/null || echo "0")
        log "INFO" "TiDB users 表记录数: ${tidb_count}"
    fi
    
    if [ $errors -eq 0 ]; then
        log "INFO" "数据恢复验证通过"
        return 0
    else
        log "WARN" "数据恢复验证发现 ${errors} 个警告"
        return 0
    fi
}

# ==================== 主流程 ====================
main() {
    parse_args "$@"
    
    log "INFO" "========== 开始数据恢复 =========="
    log "INFO" "备份目录: ${BACKUP_DIR}"
    log "INFO" "Dry Run: ${DRY_RUN}"
    
    # 检查备份目录
    check_backup_dir
    
    # 确认操作
    confirm_restore
    
    local errors=0
    
    # 恢复 ClickHouse
    local clickhouse_backup=$(find "$BACKUP_DIR" -name "clickhouse_*" -type f | head -1)
    if [ -n "$clickhouse_backup" ]; then
        restore_clickhouse "$clickhouse_backup" || errors=$((errors + 1))
    fi
    
    # 恢复 TiDB
    local tidb_backup=$(find "$BACKUP_DIR" -name "tidb_*" -type f | head -1)
    if [ -n "$tidb_backup" ]; then
        restore_tidb "$tidb_backup" || errors=$((errors + 1))
    fi
    
    # 验证恢复
    if [ $errors -eq 0 ]; then
        verify_restore
    fi
    
    if [ $errors -eq 0 ]; then
        log "INFO" "========== 数据恢复成功 =========="
        exit 0
    else
        log "ERROR" "========== 数据恢复失败 (${errors} 个错误) =========="
        exit 1
    fi
}

# 执行主流程
main "$@"
