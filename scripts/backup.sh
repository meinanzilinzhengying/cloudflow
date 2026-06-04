#!/bin/bash
# CloudFlow 自动备份脚本
# Usage: ./backup.sh [full|incremental|redis|all]

set -e

BACKUP_DIR="/backup/cloudflow"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOG_FILE="$BACKUP_DIR/backup_$TIMESTAMP.log"

# 创建备份目录
mkdir -p $BACKUP_DIR

log() {
    echo "[$(date)] $1" | tee -a $LOG_FILE
}

log "Starting CloudFlow backup..."

case "$1" in
    full)
        log "Performing FULL backup..."
        
        log "Backing up ClickHouse (full)..."
        clickhouse-backup create --name full_$TIMESTAMP
        clickhouse-backup upload full_$TIMESTAMP
        
        log "Backing up Redis..."
        redis-cli BGSAVE
        sleep 10
        cp /var/lib/redis/dump.rdb $BACKUP_DIR/redis_full_$TIMESTAMP.rdb
        
        log "FULL backup completed"
        ;;
    
    incremental)
        log "Performing INCREMENTAL backup..."
        
        log "Backing up ClickHouse (incremental)..."
        clickhouse-backup create --name incr_$TIMESTAMP --incremental
        clickhouse-backup upload incr_$TIMESTAMP
        
        log "Backing up Redis..."
        redis-cli BGSAVE
        sleep 10
        cp /var/lib/redis/dump.rdb $BACKUP_DIR/redis_incr_$TIMESTAMP.rdb
        
        log "Backing up VictoriaMetrics..."
        vmbackup -storageDataPath=/storage/victoria-metrics-data \
                 -snapshot.createURL=http://localhost:8428/snapshot/create \
                 -dst=gs://victoria-backup/$TIMESTAMP
        
        log "INCREMENTAL backup completed"
        ;;
    
    redis)
        log "Backing up Redis only..."
        redis-cli BGSAVE
        sleep 10
        cp /var/lib/redis/dump.rdb $BACKUP_DIR/redis_$TIMESTAMP.rdb
        log "Redis backup completed"
        ;;
    
    all)
        log "Performing ALL backups..."
        
        log "Backing up ClickHouse..."
        clickhouse-backup create --name full_$TIMESTAMP
        clickhouse-backup upload full_$TIMESTAMP
        
        log "Backing up Redis..."
        redis-cli BGSAVE
        sleep 10
        cp /var/lib/redis/dump.rdb $BACKUP_DIR/redis_$TIMESTAMP.rdb
        
        log "Backing up VictoriaMetrics..."
        vmbackup -storageDataPath=/storage/victoria-metrics-data \
                 -snapshot.createURL=http://localhost:8428/snapshot/create \
                 -dst=gs://victoria-backup/$TIMESTAMP
        
        log "Archiving Kafka data..."
        kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
          --export --group backup-consumer \
          --topic cloudflow-traffic > $BACKUP_DIR/kafka_$TIMESTAMP.json
        
        log "ALL backups completed"
        ;;
    
    *)
        echo "Usage: $0 [full|incremental|redis|all]"
        exit 1
        ;;
esac

log "Backup completed successfully"