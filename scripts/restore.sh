#!/bin/bash
# CloudFlow 数据恢复脚本
# Usage: ./restore.sh <backup_timestamp> [clickhouse|redis|victoria|all]

set -e

BACKUP_DIR="/backup/cloudflow"
TIMESTAMP=$1
LOG_FILE="/var/log/cloudflow/restore_$(date +%Y%m%d_%H%M%S).log"

log() {
    echo "[$(date)] $1" | tee -a $LOG_FILE
}

log "Starting CloudFlow restore from backup: $TIMESTAMP"

case "$2" in
    clickhouse)
        log "Restoring ClickHouse..."
        
        log "Downloading backup..."
        clickhouse-backup download full_$TIMESTAMP
        
        log "Stopping ClickHouse service..."
        systemctl stop cloudflow-clickhouse
        
        log "Restoring data..."
        clickhouse-backup restore full_$TIMESTAMP
        
        log "Starting ClickHouse service..."
        systemctl start cloudflow-clickhouse
        
        log "Verifying..."
        clickhouse-client -q "SELECT COUNT(*) FROM flow_logs"
        
        log "ClickHouse restore completed"
        ;;
    
    redis)
        log "Restoring Redis..."
        
        log "Stopping Redis service..."
        systemctl stop redis
        
        log "Restoring data..."
        cp $BACKUP_DIR/redis_$TIMESTAMP.rdb /var/lib/redis/dump.rdb
        chown redis:redis /var/lib/redis/dump.rdb
        
        log "Starting Redis service..."
        systemctl start redis
        
        log "Verifying..."
        redis-cli PING
        
        log "Redis restore completed"
        ;;
    
    victoria)
        log "Restoring VictoriaMetrics..."
        
        log "Stopping VictoriaMetrics service..."
        systemctl stop victoriametrics
        
        log "Restoring data..."
        vmrestore -storageDataPath=/storage/victoria-metrics-data \
                  -src=gs://victoria-backup/$TIMESTAMP
        
        log "Starting VictoriaMetrics service..."
        systemctl start victoriametrics
        
        log "Verifying..."
        curl -s http://localhost:8428/api/v1/label/job/values
        
        log "VictoriaMetrics restore completed"
        ;;
    
    all)
        log "Restoring ALL components..."
        
        log "Stopping CloudFlow services..."
        systemctl stop cloudflow-center
        systemctl stop cloudflow-edge
        systemctl stop cloudflow-agent
        
        log "Restoring Redis (P0)..."
        systemctl stop redis
        cp $BACKUP_DIR/redis_$TIMESTAMP.rdb /var/lib/redis/dump.rdb
        chown redis:redis /var/lib/redis/dump.rdb
        systemctl start redis
        
        log "Restoring ClickHouse (P1)..."
        systemctl stop cloudflow-clickhouse
        clickhouse-backup download full_$TIMESTAMP
        clickhouse-backup restore full_$TIMESTAMP
        systemctl start cloudflow-clickhouse
        
        log "Restoring VictoriaMetrics (P2)..."
        systemctl stop victoriametrics
        vmrestore -storageDataPath=/storage/victoria-metrics-data \
                  -src=gs://victoria-backup/$TIMESTAMP
        systemctl start victoriametrics
        
        log "Starting CloudFlow services..."
        systemctl start cloudflow-center
        systemctl start cloudflow-edge
        systemctl start cloudflow-agent
        
        log "Validating restore..."
        curl -s http://localhost:8080/api/healthz
        redis-cli PING
        clickhouse-client -q "SELECT COUNT(*) FROM flow_logs LIMIT 1"
        
        log "ALL components restored successfully"
        ;;
    
    *)
        echo "Usage: $0 <backup_timestamp> [clickhouse|redis|victoria|all]"
        exit 1
        ;;
esac

log "Restore completed successfully"