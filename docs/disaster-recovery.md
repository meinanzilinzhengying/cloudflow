# CloudFlow 容灾备份与恢复策略

## 概述

本文档定义了 CloudFlow 平台的数据备份与恢复策略，包括定时备份、灾备恢复流程和验证方法。

---

## 目录

1. [备份架构](#1-备份架构)
2. [备份策略](#2-备份策略)
3. [备份脚本](#3-备份脚本)
4. [恢复流程](#4-恢复流程)
5. [恢复脚本](#5-恢复脚本)
6. [验证方法](#6-验证方法)
7. [灾备演练](#7-灾备演练)

---

## 1. 备份架构

### 1.1 数据分类

| 数据类型 | 存储组件 | 备份策略 | 恢复目标 |
|----------|----------|----------|----------|
| 流量日志 | ClickHouse | 每日增量 + 每周全量 | 按时间范围恢复 |
| 配置数据 | Redis | 每日全量快照 | 全量恢复 |
| 指标数据 | VictoriaMetrics | 每日增量 | 按时间范围恢复 |
| 告警记录 | ClickHouse | 每日增量 | 全量恢复 |
| 仪表盘配置 | Redis | 实时同步 | 全量恢复 |

### 1.2 备份架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    生产环境                                │
├─────────────────────────────────────────────────────────────┤
│  ClickHouse │ Redis │ VictoriaMetrics │ Kafka             │
│       │         │           │              │               │
│       ▼         ▼           ▼              ▼               │
│  ┌─────────────────────────────────────────────┐          │
│  │           Backup Coordinator                │          │
│  └─────────────────────────────────────────────┘          │
│                       │                                   │
│                       ▼                                   │
│  ┌─────────────────────────────────────────────┐          │
│  │              Backup Storage                 │          │
│  │  Local: /backup                            │          │
│  │  Remote: S3/GCS/OSS                        │          │
│  └─────────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. 备份策略

### 2.1 ClickHouse 备份

**全量备份**：每周日 02:00 执行

```bash
# 全量备份命令
clickhouse-backup create --name weekly_full_$(date +%Y%m%d)
clickhouse-backup upload weekly_full_$(date +%Y%m%d)
```

**增量备份**：每日 02:00 执行（除周日）

```bash
# 增量备份命令
clickhouse-backup create --name daily_incr_$(date +%Y%m%d) --incremental
clickhouse-backup upload daily_incr_$(date +%Y%m%d)
```

**数据保留**：
- 全量备份：保留 4 周
- 增量备份：保留 7 天

### 2.2 Redis 备份

**全量快照**：每 6 小时执行

```bash
# Redis 快照备份
redis-cli BGSAVE
cp /var/lib/redis/dump.rdb /backup/redis/redis_$(date +%Y%m%d_%H%M).rdb
```

**数据保留**：保留 7 天

### 2.3 VictoriaMetrics 备份

**增量备份**：每日 01:00 执行

```bash
# 备份命令
vmbackup -storageDataPath=/storage/victoria-metrics-data \
         -snapshot.createURL=http://localhost:8428/snapshot/create \
         -dst=gs://victoria-backup/$(date +%Y%m%d)
```

**数据保留**：保留 30 天

### 2.4 Kafka 数据归档

**数据保留策略**：
- 消息保留时间：7 天
- 消息最大大小：100GB
- 自动删除策略：按时间或大小触发

**归档命令**：

```bash
# 手动归档到 S3
kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
  --export --group backup-consumer \
  --topic cloudflow-traffic > /backup/kafka/archive_$(date +%Y%m%d).json
```

---

## 3. 备份脚本

### 3.1 主备份脚本

```bash
#!/bin/bash
# CloudFlow 自动备份脚本
# Usage: ./backup.sh [full|incremental|redis|all]

set -e

BACKUP_DIR="/backup/cloudflow"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOG_FILE="$BACKUP_DIR/backup_$TIMESTAMP.log"

# 创建备份目录
mkdir -p $BACKUP_DIR

echo "[$(date)] Starting CloudFlow backup..." | tee -a $LOG_FILE

case "$1" in
    full)
        echo "[$(date)] Performing FULL backup..." | tee -a $LOG_FILE
        
        # ClickHouse 全量备份
        echo "[$(date)] Backing up ClickHouse (full)..." | tee -a $LOG_FILE
        clickhouse-backup create --name full_$TIMESTAMP
        clickhouse-backup upload full_$TIMESTAMP
        
        # Redis 快照
        echo "[$(date)] Backing up Redis..." | tee -a $LOG_FILE
        redis-cli BGSAVE
        sleep 10
        cp /var/lib/redis/dump.rdb $BACKUP_DIR/redis_full_$TIMESTAMP.rdb
        
        echo "[$(date)] FULL backup completed" | tee -a $LOG_FILE
        ;;
    
    incremental)
        echo "[$(date)] Performing INCREMENTAL backup..." | tee -a $LOG_FILE
        
        # ClickHouse 增量备份
        echo "[$(date)] Backing up ClickHouse (incremental)..." | tee -a $LOG_FILE
        clickhouse-backup create --name incr_$TIMESTAMP --incremental
        clickhouse-backup upload incr_$TIMESTAMP
        
        # Redis 快照
        echo "[$(date)] Backing up Redis..." | tee -a $LOG_FILE
        redis-cli BGSAVE
        sleep 10
        cp /var/lib/redis/dump.rdb $BACKUP_DIR/redis_incr_$TIMESTAMP.rdb
        
        # VictoriaMetrics 备份
        echo "[$(date)] Backing up VictoriaMetrics..." | tee -a $LOG_FILE
        vmbackup -storageDataPath=/storage/victoria-metrics-data \
                 -snapshot.createURL=http://localhost:8428/snapshot/create \
                 -dst=gs://victoria-backup/$TIMESTAMP
        
        echo "[$(date)] INCREMENTAL backup completed" | tee -a $LOG_FILE
        ;;
    
    redis)
        echo "[$(date)] Backing up Redis only..." | tee -a $LOG_FILE
        redis-cli BGSAVE
        sleep 10
        cp /var/lib/redis/dump.rdb $BACKUP_DIR/redis_$TIMESTAMP.rdb
        echo "[$(date)] Redis backup completed" | tee -a $LOG_FILE
        ;;
    
    all)
        echo "[$(date)] Performing ALL backups..." | tee -a $LOG_FILE
        
        # ClickHouse
        echo "[$(date)] Backing up ClickHouse..." | tee -a $LOG_FILE
        clickhouse-backup create --name full_$TIMESTAMP
        clickhouse-backup upload full_$TIMESTAMP
        
        # Redis
        echo "[$(date)] Backing up Redis..." | tee -a $LOG_FILE
        redis-cli BGSAVE
        sleep 10
        cp /var/lib/redis/dump.rdb $BACKUP_DIR/redis_$TIMESTAMP.rdb
        
        # VictoriaMetrics
        echo "[$(date)] Backing up VictoriaMetrics..." | tee -a $LOG_FILE
        vmbackup -storageDataPath=/storage/victoria-metrics-data \
                 -snapshot.createURL=http://localhost:8428/snapshot/create \
                 -dst=gs://victoria-backup/$TIMESTAMP
        
        # Kafka 归档
        echo "[$(date)] Archiving Kafka data..." | tee -a $LOG_FILE
        kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
          --export --group backup-consumer \
          --topic cloudflow-traffic > $BACKUP_DIR/kafka_$TIMESTAMP.json
        
        echo "[$(date)] ALL backups completed" | tee -a $LOG_FILE
        ;;
    
    *)
        echo "Usage: $0 [full|incremental|redis|all]"
        exit 1
        ;;
esac

echo "[$(date)] Backup completed successfully" | tee -a $LOG_FILE
```

### 3.2 Crontab 配置

```bash
# CloudFlow 备份定时任务

# 每日增量备份 (02:00)
0 2 * * * /opt/cloudflow/scripts/backup.sh incremental >> /var/log/cloudflow/backup.log 2>&1

# 每周日全量备份 (02:00)
0 2 * * 0 /opt/cloudflow/scripts/backup.sh full >> /var/log/cloudflow/backup.log 2>&1

# Redis 每6小时快照
0 */6 * * * /opt/cloudflow/scripts/backup.sh redis >> /var/log/cloudflow/backup.log 2>&1

# 清理7天前的本地备份
0 3 * * * find /backup/cloudflow -type f -mtime +7 -delete >> /var/log/cloudflow/backup-cleanup.log 2>&1
```

---

## 4. 恢复流程

### 4.1 恢复流程概览

```
故障发生
    │
    ▼
评估数据损失
    │
    ▼
选择恢复点
    │
    ▼
停止相关服务
    │
    ▼
恢复数据
    │
    ├─► ClickHouse 恢复
    ├─► Redis 恢复
    ├─► VictoriaMetrics 恢复
    └─► Kafka 恢复
    │
    ▼
启动服务
    │
    ▼
验证数据完整性
    │
    ▼
恢复完成
```

### 4.2 恢复优先级

| 优先级 | 组件 | 恢复时间目标 |
|--------|------|--------------|
| P0 | Redis | 15 分钟 |
| P1 | ClickHouse | 30 分钟 |
| P2 | VictoriaMetrics | 60 分钟 |
| P3 | Kafka | 120 分钟 |

---

## 5. 恢复脚本

### 5.1 主恢复脚本

```bash
#!/bin/bash
# CloudFlow 数据恢复脚本
# Usage: ./restore.sh <backup_timestamp> [clickhouse|redis|victoria|all]

set -e

BACKUP_DIR="/backup/cloudflow"
TIMESTAMP=$1
LOG_FILE="/var/log/cloudflow/restore_$(date +%Y%m%d_%H%M%S).log"

echo "[$(date)] Starting CloudFlow restore from backup: $TIMESTAMP" | tee -a $LOG_FILE

case "$2" in
    clickhouse)
        echo "[$(date)] Restoring ClickHouse..." | tee -a $LOG_FILE
        
        # 下载备份
        clickhouse-backup download full_$TIMESTAMP
        
        # 停止服务
        systemctl stop cloudflow-clickhouse
        
        # 恢复数据
        clickhouse-backup restore full_$TIMESTAMP
        
        # 启动服务
        systemctl start cloudflow-clickhouse
        
        # 验证
        clickhouse-client -q "SELECT COUNT(*) FROM flow_logs"
        
        echo "[$(date)] ClickHouse restore completed" | tee -a $LOG_FILE
        ;;
    
    redis)
        echo "[$(date)] Restoring Redis..." | tee -a $LOG_FILE
        
        # 停止服务
        systemctl stop redis
        
        # 恢复数据
        cp $BACKUP_DIR/redis_$TIMESTAMP.rdb /var/lib/redis/dump.rdb
        chown redis:redis /var/lib/redis/dump.rdb
        
        # 启动服务
        systemctl start redis
        
        # 验证
        redis-cli PING
        
        echo "[$(date)] Redis restore completed" | tee -a $LOG_FILE
        ;;
    
    victoria)
        echo "[$(date)] Restoring VictoriaMetrics..." | tee -a $LOG_FILE
        
        # 停止服务
        systemctl stop victoriametrics
        
        # 恢复数据
        vmrestore -storageDataPath=/storage/victoria-metrics-data \
                  -src=gs://victoria-backup/$TIMESTAMP
        
        # 启动服务
        systemctl start victoriametrics
        
        # 验证
        curl -s http://localhost:8428/api/v1/label/job/values
        
        echo "[$(date)] VictoriaMetrics restore completed" | tee -a $LOG_FILE
        ;;
    
    all)
        echo "[$(date)] Restoring ALL components..." | tee -a $LOG_FILE
        
        # 停止所有服务
        systemctl stop cloudflow-center
        systemctl stop cloudflow-edge
        systemctl stop cloudflow-agent
        
        # 恢复 Redis (P0)
        echo "[$(date)] Restoring Redis..." | tee -a $LOG_FILE
        systemctl stop redis
        cp $BACKUP_DIR/redis_$TIMESTAMP.rdb /var/lib/redis/dump.rdb
        chown redis:redis /var/lib/redis/dump.rdb
        systemctl start redis
        
        # 恢复 ClickHouse (P1)
        echo "[$(date)] Restoring ClickHouse..." | tee -a $LOG_FILE
        systemctl stop cloudflow-clickhouse
        clickhouse-backup download full_$TIMESTAMP
        clickhouse-backup restore full_$TIMESTAMP
        systemctl start cloudflow-clickhouse
        
        # 恢复 VictoriaMetrics (P2)
        echo "[$(date)] Restoring VictoriaMetrics..." | tee -a $LOG_FILE
        systemctl stop victoriametrics
        vmrestore -storageDataPath=/storage/victoria-metrics-data \
                  -src=gs://victoria-backup/$TIMESTAMP
        systemctl start victoriametrics
        
        # 启动服务
        echo "[$(date)] Starting CloudFlow services..." | tee -a $LOG_FILE
        systemctl start cloudflow-center
        systemctl start cloudflow-edge
        systemctl start cloudflow-agent
        
        # 验证
        echo "[$(date)] Validating restore..." | tee -a $LOG_FILE
        curl -s http://localhost:8080/api/healthz
        redis-cli PING
        clickhouse-client -q "SELECT COUNT(*) FROM flow_logs LIMIT 1"
        
        echo "[$(date)] ALL components restored successfully" | tee -a $LOG_FILE
        ;;
    
    *)
        echo "Usage: $0 <backup_timestamp> [clickhouse|redis|victoria|all]"
        exit 1
        ;;
esac

echo "[$(date)] Restore completed successfully" | tee -a $LOG_FILE
```

### 5.2 恢复验证脚本

```bash
#!/bin/bash
# CloudFlow 恢复验证脚本

echo "=== CloudFlow Restore Validation ==="

echo ""
echo "1. Checking Redis..."
REDIS_STATUS=$(redis-cli PING)
if [ "$REDIS_STATUS" == "PONG" ]; then
    echo "   ✅ Redis is running"
else
    echo "   ❌ Redis is NOT running"
    exit 1
fi

echo ""
echo "2. Checking ClickHouse..."
CLICKHOUSE_STATUS=$(clickhouse-client -q "SELECT 1" 2>/dev/null || echo "FAIL")
if [ "$CLICKHOUSE_STATUS" == "1" ]; then
    echo "   ✅ ClickHouse is running"
else
    echo "   ❌ ClickHouse is NOT running"
    exit 1
fi

echo ""
echo "3. Checking VictoriaMetrics..."
VM_STATUS=$(curl -s http://localhost:8428/health 2>/dev/null || echo "FAIL")
if [ "$VM_STATUS" == "OK" ]; then
    echo "   ✅ VictoriaMetrics is running"
else
    echo "   ❌ VictoriaMetrics is NOT running"
    exit 1
fi

echo ""
echo "4. Checking Center API..."
CENTER_STATUS=$(curl -s http://localhost:8080/api/healthz 2>/dev/null || echo "FAIL")
if [ "$CENTER_STATUS" == "ok" ]; then
    echo "   ✅ Center API is running"
else
    echo "   ❌ Center API is NOT running"
    exit 1
fi

echo ""
echo "5. Checking flow data..."
FLOW_COUNT=$(clickhouse-client -q "SELECT COUNT(*) FROM flow_logs" 2>/dev/null || echo "0")
echo "   Flow records: $FLOW_COUNT"
if [ "$FLOW_COUNT" -gt 0 ]; then
    echo "   ✅ Flow data exists"
else
    echo "   ⚠️ No flow data found"
fi

echo ""
echo "=== Validation Complete ==="
```

---

## 6. 验证方法

### 6.1 备份验证

```bash
# 验证 ClickHouse 备份
clickhouse-backup list
clickhouse-backup show full_$TIMESTAMP

# 验证 Redis 备份
ls -la /backup/cloudflow/redis_*.rdb
md5sum /backup/cloudflow/redis_$TIMESTAMP.rdb

# 验证 VictoriaMetrics 备份
gsutil ls gs://victoria-backup/$TIMESTAMP
```

### 6.2 恢复验证指标

| 指标 | 验证方法 | 预期结果 |
|------|----------|----------|
| Redis | `redis-cli PING` | PONG |
| ClickHouse | `clickhouse-client -q "SELECT 1"` | 1 |
| VictoriaMetrics | `curl http://localhost:8428/health` | OK |
| Center API | `curl http://localhost:8080/api/healthz` | ok |
| 流量数据 | ClickHouse 查询记录数 | > 0 |
| 仪表盘配置 | Redis 查询配置 | 存在 |

---

## 7. 灾备演练

### 7.1 演练频率

| 类型 | 频率 | 负责人 |
|------|------|--------|
| 月度演练 | 每月一次 | SRE 团队 |
| 季度演练 | 每季度一次 | 技术负责人 |
| 年度演练 | 每年一次 | 架构师 |

### 7.2 演练步骤

1. **准备阶段**
   - 确定演练时间（低峰期）
   - 通知相关团队
   - 备份当前数据

2. **演练执行**
   - 模拟故障场景
   - 执行恢复流程
   - 记录恢复时间

3. **验证阶段**
   - 运行验证脚本
   - 检查数据完整性
   - 验证服务可用性

4. **总结阶段**
   - 评估恢复时间
   - 识别改进点
   - 更新恢复文档

### 7.3 演练报告模板

```markdown
# CloudFlow 灾备演练报告

## 演练信息
- 演练日期: 2024-01-15
- 演练类型: 月度演练
- 演练场景: 全量数据恢复

## 恢复时间
| 组件 | 恢复时间 | SLA目标 | 是否达标 |
|------|----------|----------|----------|
| Redis | 5分钟 | 15分钟 | ✅ |
| ClickHouse | 25分钟 | 30分钟 | ✅ |
| VictoriaMetrics | 45分钟 | 60分钟 | ✅ |
| 整体恢复 | 60分钟 | 90分钟 | ✅ |

## 问题记录
| 序号 | 问题描述 | 影响 | 修复建议 |
|------|----------|------|----------|
| 1 | ClickHouse 恢复速度较慢 | 增加恢复时间 | 优化存储IO |
| 2 | 验证脚本缺少 Kafka 检查 | 无法验证 Kafka | 添加 Kafka 验证 |

## 改进计划
1. 优化 ClickHouse 存储配置
2. 更新验证脚本
3. 增加自动化验证步骤

## 结论
✅ 演练成功，所有组件在 SLA 时间内恢复
```

---

**文档版本**: v1.0  
**最后更新**: 2024-01-15  
**适用版本**: CloudFlow v1.0+