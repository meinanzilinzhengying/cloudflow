# CloudFlow 版本升级指南

## v0.x → v1.0 升级步骤

### 升级前准备

#### 1. 备份数据
```bash
# 执行完整备份
./scripts/backup.sh

# 验证备份文件
ls -lh /var/backups/cloudflow/
```

#### 2. 检查当前版本
```bash
# 检查各服务版本
./cloudflow-agent --version
./cloudflow-center --version
```

#### 3. 停止服务
```bash
# 停止所有服务
systemctl stop cloudflow-agent
docker-compose stop
```

---

### 升级步骤

#### 步骤1：拉取新版本代码
```bash
cd /opt/cloudflow
git pull origin main
git checkout v1.0.0
```

#### 步骤2：数据库迁移

##### MySQL 迁移
```bash
# 执行迁移脚本
mysql -u root -p cloudflow < migrations/v1.0.0/001_init_schema.sql
mysql -u root -p cloudflow < migrations/v1.0.0/002_add_vxlan_support.sql
mysql -u root -p cloudflow < migrations/v1.0.0/003_token_blacklist.sql
```

##### ClickHouse 迁移
```bash
# 添加VXLAN字段
clickhouse-client -q "
ALTER TABLE flows ADD COLUMN IF NOT EXISTS vni UInt32 DEFAULT 0;
ALTER TABLE flows ADD INDEX IF NOT EXISTS vni_idx vni TYPE minmax;
"
```

#### 步骤3：更新配置文件
```bash
# 备份旧配置
cp config.yaml config.yaml.bak

# 使用新配置模板
cp config/production.yaml config.yaml

# 编辑配置，更新数据库连接、密钥等
vim config.yaml
```

**重要配置变更：**
- 新增 `relational_db` 配置块（支持国产数据库）
- 新增 `timeseries_db` 配置块
- 新增 `kv_store` 配置块
- 新增 `dual_write` 双写模式配置
- JWT密钥必须通过环境变量设置

#### 步骤4：重新编译
```bash
# 编译所有模块
go mod tidy
go build ./...

# 编译eBPF字节码（需要clang环境）
cd cloud-flow-agent
make ebpf-build
make ebpf-verify
cd ..
```

#### 步骤5：启动服务
```bash
# 启动平台服务
docker-compose up -d

# 等待数据库就绪
sleep 30

# 启动Agent
systemctl start cloudflow-agent
```

---

### 升级后验证

#### 1. 检查服务状态
```bash
# 检查容器状态
docker-compose ps

# 检查Agent状态
systemctl status cloudflow-agent

# 检查健康检查
curl http://localhost:8080/health
```

#### 2. 功能验证
```bash
# 验证认证
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"xxx"}'

# 验证流量查询
curl http://localhost:8080/api/v1/flows \
  -H "Authorization: Bearer <token>"
```

#### 3. 日志检查
```bash
# 检查服务日志
docker-compose logs -f

# 检查Agent日志
journalctl -u cloudflow-agent -f
```

---

### 回滚方案

#### 紧急回滚（5分钟内完成）
```bash
# 1. 停止服务
systemctl stop cloudflow-agent
docker-compose stop

# 2. 恢复代码版本
git checkout v0.x.x

# 3. 恢复配置
cp config.yaml.bak config.yaml

# 4. 恢复数据库（如需要）
./scripts/restore.sh <备份时间戳>

# 5. 重新编译并启动
go build ./...
docker-compose up -d
systemctl start cloudflow-agent
```

---

### 兼容性说明

#### 配置兼容性
- v1.0 配置文件与 v0.x 不兼容
- 必须使用新的配置模板

#### API兼容性
- REST API v1 完全兼容
- gRPC API 新增配置下发接口

#### 数据库兼容性
- MySQL 表结构向后兼容
- ClickHouse 新增字段有默认值

---

### 常见问题

#### Q: 升级后Agent无法连接？
A: 检查以下几点：
1. 控制平面地址配置是否正确
2. JWT密钥是否一致
3. 防火墙是否开放gRPC端口

#### Q: 数据库连接失败？
A: 检查：
1. 数据库地址、端口、用户名、密码
2. 国产数据库驱动是否正确配置
3. 双写模式是否正确设置

#### Q: eBPF采集不工作？
A: 检查：
1. 内核版本 >= 5.4
2. .bpf.o 文件是否已编译（make ebpf-build）
3. Agent是否以root权限运行
