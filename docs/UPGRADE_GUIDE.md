# CloudFlow 版本升级指南

## v0.x → v1.0 升级步骤

### 升级前准备

#### 1. 备份数据
```bash
# 执行完整备份
./scripts/backup.sh /data/backup/cloudflow

# 验证备份文件
ls -lh /data/backup/cloudflow/*.tar.gz
```

#### 2. 检查当前版本
```bash
# 检查各服务版本
cloudflow-center --version
cloudflow-agent --version
```

#### 3. 停止所有服务
```bash
# 停止平台服务
systemctl stop cloudflow-center
systemctl stop cloudflow-query
systemctl stop cloudflow-alert

# 停止所有Agent（批量）
ansible all -m systemd -a "name=cloudflow-agent state=stopped"
```

---

### 升级步骤

#### 步骤1：下载新版本
```bash
# 下载最新版本
cd /opt
wget https://github.com/meinanzilinzhengying/cloudflow/releases/download/v1.0.0/cloudflow-v1.0.0.tar.gz

# 解压
tar -xzf cloudflow-v1.0.0.tar.gz
cd cloudflow-v1.0.0
```

#### 步骤2：数据库迁移

##### MySQL 数据库迁移
```bash
# 执行迁移脚本
mysql -u root -p cloudflow < scripts/mysql/migration_v0_to_v1.sql

# 验证迁移结果
mysql -u root -p cloudflow -e "SHOW TABLES;"
```

##### ClickHouse 数据库迁移
```bash
# 执行迁移脚本
clickhouse-client -d cloudflow < scripts/clickhouse/migration_v0_to_v1.sql

# 验证迁移结果
clickhouse-client -d cloudflow -q "SHOW TABLES;"
```

##### 国产数据库迁移（达梦/金仓）
```bash
# 达梦DM8
disql SYSDBA/SYSDBA@localhost:5236 @scripts/dameng/migration_v0_to_v1.sql

# 人大金仓
ksql -U system -d cloudflow -f scripts/kingbase/migration_v0_to_v1.sql
```

#### 步骤3：更新配置文件

**重要变更说明：**

1. **数据库配置变更**
```yaml
# 旧配置（v0.x）
mysql:
  host: localhost
  port: 3306

# 新配置（v1.0）
relational_db:
  type: mysql           # mysql/dameng/kingbase/gaussdb/oceanbase
  host: localhost
  port: 3306
  user: root
  password: ${MYSQL_PASSWORD}
  database: cloudflow
  enable_dual_write: false
  dual_write_mode: ModeOldOnly
```

2. **时序数据库配置**
```yaml
# 旧配置（v0.x）
clickhouse:
  addr: localhost:9000

# 新配置（v1.0）
timeseries_db:
  type: clickhouse      # clickhouse/dameng
  host: localhost
  port: 9000
  user: default
  password: ${CLICKHOUSE_PASSWORD}
  database: cloudflow
```

3. **KV存储配置**
```yaml
# 旧配置（v0.x）
redis:
  addr: localhost:6379

# 新配置（v1.0）
kv_store:
  type: redis           # redis/gaussdb
  host: localhost
  port: 6379
  password: ${REDIS_PASSWORD}
  database: 0
```

4. **安全配置**
```yaml
# 新增：JWT密钥（必须32位以上）
security:
  jwt_secret: ${CLOUD_FLOW_JWT_SECRET}
  cors_allowed_origins:
    - http://localhost:3000
    - http://localhost:8080
```

#### 步骤4：部署新版本
```bash
# 备份旧版本
mv /opt/cloudflow /opt/cloudflow-v0.x.bak

# 部署新版本
cp -r cloudflow-v1.0.0 /opt/cloudflow

# 复制配置文件
cp /opt/cloudflow-v0.x.bak/config.yaml /opt/cloudflow/config.yaml
# 手动更新配置文件中的新字段

# 设置权限
chown -R cloudflow:cloudflow /opt/cloudflow
```

#### 步骤5：启动服务
```bash
# 启动平台服务
systemctl start cloudflow-center
systemctl start cloudflow-query
systemctl start cloudflow-alert

# 验证服务状态
systemctl status cloudflow-center
journalctl -u cloudflow-center -f

# 启动Agent（批量）
ansible all -m systemd -a "name=cloudflow-agent state=started"
```

#### 步骤6：功能验证
```bash
# 1. 检查健康状态
curl http://localhost:8080/health

# 2. 验证数据库连接
curl http://localhost:8080/api/v1/health/database

# 3. 验证Agent在线
curl http://localhost:8080/api/v1/agents

# 4. 验证数据采集
curl http://localhost:8080/api/v1/flows?limit=10
```

---

### 回滚方案

#### 紧急回滚（5分钟内完成）
```bash
# 1. 停止新版本服务
systemctl stop cloudflow-center
systemctl stop cloudflow-query
systemctl stop cloudflow-alert
ansible all -m systemd -a "name=cloudflow-agent state=stopped"

# 2. 恢复旧版本
rm -rf /opt/cloudflow
mv /opt/cloudflow-v0.x.bak /opt/cloudflow

# 3. 恢复数据库（如需要）
./scripts/restore.sh /data/backup/cloudflow/[备份文件].tar.gz

# 4. 启动旧版本服务
systemctl start cloudflow-center
systemctl start cloudflow-query
systemctl start cloudflow-alert
ansible all -m systemd -a "name=cloudflow-agent state=started"
```

---

### 兼容性说明

#### 数据库兼容性
| 数据库 | v0.x | v1.0 | 备注 |
|--------|------|------|------|
| MySQL | ✅ | ✅ | 完全兼容 |
| ClickHouse | ✅ | ✅ | 完全兼容 |
| Redis | ✅ | ✅ | 完全兼容 |
| 达梦DM8 | ❌ | ✅ | v1.0新增 |
| 人大金仓 | ❌ | ✅ | v1.0新增 |
| GaussDB | ❌ | ✅ | v1.0新增 |

#### API兼容性
- REST API: v1.0 完全向后兼容 v0.x
- gRPC API: v1.0 完全向后兼容 v0.x
- Agent协议: v1.0 Agent 可连接 v0.x Center（部分功能降级）

#### 配置兼容性
- v0.x 配置文件可在 v1.0 中使用（自动补全默认值）
- 建议迁移到新配置格式以使用国产数据库功能

---

### 常见问题

#### Q1: 升级后Agent无法连接？
**A**: 检查以下几点：
1. Center服务是否正常启动
2. 防火墙是否开放gRPC端口（50051）
3. Agent配置文件中的Center地址是否正确
4. 检查Agent日志：`journalctl -u cloudflow-agent -f`

#### Q2: 升级后数据库连接失败？
**A**: 检查以下几点：
1. 数据库服务是否正常运行
2. 配置文件中的数据库密码是否正确
3. 数据库用户权限是否足够
4. 检查Center日志中的数据库错误

#### Q3: 如何验证数据完整性？
**A**: 执行以下验证：
```bash
# 验证MySQL表行数
mysql -u root -p cloudflow -e "SELECT COUNT(*) FROM flows;"

# 验证ClickHouse表行数
clickhouse-client -d cloudflow -q "SELECT COUNT(*) FROM flows;"

# 对比升级前后的行数差异
```

#### Q4: 国产数据库切换注意事项？
**A**: 
1. 先在测试环境验证兼容性
2. 使用双写模式（ModeSyncWrite）并行运行至少1周
3. 逐步切读流量（ModeReadSplit）
4. 完全切换到新库（ModeNewOnly）
5. 保留旧库至少2周作为备份

---

### 升级检查清单

- [ ] 已执行完整数据备份
- [ ] 已验证备份文件完整性
- [ ] 已停止所有服务
- [ ] 已下载新版本
- [ ] 已执行数据库迁移脚本
- [ ] 已更新配置文件
- [ ] 已部署新版本二进制
- [ ] 已启动所有服务
- [ ] 已验证健康检查接口
- [ ] 已验证数据库连接
- [ ] 已验证Agent在线
- [ ] 已验证数据采集正常
- [ ] 已清理旧版本备份（可选）
