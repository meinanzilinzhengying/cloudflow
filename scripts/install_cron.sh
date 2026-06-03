# CloudFlow 自动备份 Cron 配置
# 安装方法: crontab install_cron.sh

# ==================== 环境变量 ====================
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
BACKUP_BASE_DIR=/opt/cloudflow/backups
CLICKHOUSE_HOST=localhost
TIDB_HOST=localhost
TIDB_USER=root

# ==================== 备份任务 ====================

# 每日凌晨 2:00 执行全量备份
0 2 * * * /opt/cloudflow/scripts/backup.sh full >> /opt/cloudflow/backups/backup.log 2>&1

# 每小时执行增量备份（整点）
0 * * * * /opt/cloudflow/scripts/backup.sh incremental >> /opt/cloudflow/backups/backup.log 2>&1

# 每周日凌晨 3:00 执行恢复测试（在隔离环境）
# 0 3 * * 0 /opt/cloudflow/scripts/restore.sh /opt/cloudflow/backups/full/latest --dry-run >> /opt/cloudflow/backups/restore_test.log 2>&1

# ==================== 维护任务 ====================

# 每天清理 7 天前的备份
@daily find /opt/cloudflow/backups -type d -mtime +7 -exec rm -rf {} + 2>/dev/null || true

# 每周生成备份报告
@weekly /opt/cloudflow/scripts/generate_backup_report.sh >> /opt/cloudflow/backups/report.log 2>&1
