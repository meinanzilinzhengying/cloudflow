# CloudFlow 分步部署教程

## 目录
1. [环境准备](#环境准备)
2. [单机部署](#单机部署)
3. [集群部署](#集群部署)
4. [国产数据库配置](#国产数据库配置)
5. [eBPF Agent部署](#ebpf-agent部署)
6. [功能验证](#功能验证)
7. [问题排查](#问题排查)

---

## 环境准备

### 系统要求

| 组件 | 最低要求 | 推荐配置 |
|------|---------|---------|
| 操作系统 | CentOS 7.9+, Ubuntu 20.04+, Debian 11+, 麒麟V10 | Ubuntu 22.04 LTS |
| 内核版本 | >= 5.4 | >= 5.10 |
| CPU | 4核 | 8核+ |
| 内存 | 8GB | 16GB+ |
| 磁盘 | 100GB | 500GB SSD |
| 网络 | 千兆网卡 | 万兆网卡 |

### 软件依赖

```bash
# Ubuntu/Debian
apt update && apt install -y \
    docker.io docker-compose \
    clang llvm libbpf-dev \
    linux-headers-$(uname -r) \
    golang-1.22 git make

# CentOS/RHEL
yum install -y \
    docker docker-compose \
    clang llvm libbpf-devel \
    kernel-devel-$(uname -r) \
    golang git make

# 麒麟/UOS
yum install -y \
    docker clang llvm libbpf-devel \
    kernel-devel golang git make
```

### 验证环境

```bash
# 验证Docker
docker --version
docker-compose --version

# 验证Go
go version

# 验证clang
clang --version

# 验证内核
uname -r
```

---

## 单机部署

### 步骤1：克隆代码

```bash
git clone https://github.com/meinanzilinzhengying/cloudflow.git
cd cloudflow
```

### 步骤2：配置环境变量

```bash
# 创建环境变量文件
cat > .env << EOF
# 数据库密码（必须修改！）
MYSQL_ROOT_PASSWORD=your_secure_password_here
MYSQL_PASSWORD=your_secure_password_here
CLICKHOUSE_PASSWORD=your_secure_password_here
REDIS_PASSWORD=your_secure_password_here

# JWT密钥（必须32位以上！）
CLOUD_FLOW_JWT_SECRET=your_32_character_jwt_secret_key_here

# CORS白名单
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:8080
EOF
```

### 步骤3：启动中间件

```bash
# 启动MySQL/ClickHouse/Redis/Kafka
docker-compose up -d mysql clickhouse redis kafka

# 验证服务状态
docker-compose ps

# 等待服务启动（约30秒）
sleep 30
```

### 步骤4：编译eBPF字节码

```bash
cd cloud-flow-agent
make ebpf-build

# 验证编译结果（所有文件必须 > 0字节）
make ebpf-verify

# 预期输出：
# Verifying eBPF bytecode files...
# All eBPF bytecode files verified successfully!
```

### 步骤5：编译所有服务

```bash
cd ..
go mod tidy
go build ./...

# 验证编译成功
echo $?  # 应该输出 0
```

### 步骤6：初始化数据库

```bash
# MySQL初始化
docker-compose exec mysql mysql -u root -p${MYSQL_ROOT_PASSWORD} \
    -e "CREATE DATABASE IF NOT EXISTS cloudflow;"

# ClickHouse初始化
docker-compose exec clickhouse clickhouse-client \
    -q "CREATE DATABASE IF NOT EXISTS cloudflow;"
```

### 步骤7：启动平台服务

```bash
# 方式1：前台启动（调试用）
go run services/control-plane/main.go --config config.yaml

# 方式2：后台启动（生产用）
nohup go run services/control-plane/main.go --config config.yaml > center.log 2>&1 &
nohup go run services/query-service/main.go --config config.yaml > query.log 2>&1 &
nohup go run services/data-plane/main.go --config config.yaml > data.log 2>&1 &
nohup go run services/alert-engine/main.go --config config.yaml > alert.log 2>&1 &
```

### 步骤8：验证平台服务

```bash
# 健康检查
curl http://localhost:8080/health

# 预期输出：
# {
#   "status": "healthy",
#   "timestamp": "..."
# }
```

---

## 集群部署

### 架构说明

```
┌─────────────────────────────────────────────────────────┐
│                     负载均衡层 (Nginx/LVS)                │
└───────────────────────────┬─────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────┐
│                   控制平面集群 (3节点)                    │
│  cloudflow-center-1  cloudflow-center-2  cloudflow-center-3 │
└───────────────────────────┬─────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────┐
│                   数据平面集群 (N节点)                    │
│  cloudflow-data-1  cloudflow-data-2  ...  cloudflow-data-N │
└───────────────────────────┬─────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────┐
│                     存储层集群                           │
│  MySQL集群    ClickHouse集群    Redis集群    Kafka集群    │
└─────────────────────────────────────────────────────────┘
```

### 部署步骤

#### 1. 部署存储集群

**MySQL集群（主从复制）：**
```bash
# 参考MySQL官方主从复制文档
# 建议使用：MySQL 8.0 + GTID + 半同步复制
```

**ClickHouse集群：**
```bash
# 建议3节点分片集群
# 参考ClickHouse官方集群部署文档
```

**Redis集群：**
```bash
# 建议3主3从哨兵模式
redis-cli --cluster create ...
```

**Kafka集群：**
```bash
# 建议3节点Broker + Zookeeper集群
```

#### 2. 部署控制平面

```bash
# 在3个控制节点上分别执行
# 节点1
go run services/control-plane/main.go --config config.yaml \
    --node-id center-1 --cluster-enabled true

# 节点2
go run services/control-plane/main.go --config config.yaml \
    --node-id center-2 --cluster-enabled true

# 节点3
go run services/control-plane/main.go --config config.yaml \
    --node-id center-3 --cluster-enabled true
```

#### 3. 部署数据平面

```bash
# 在N个数据节点上执行
go run services/data-plane/main.go --config config.yaml \
    --node-id data-1 --cluster-enabled true
```

#### 4. 配置负载均衡

```nginx
# Nginx配置示例
upstream cloudflow_center {
    server center-1:8080;
    server center-2:8080;
    server center-3:8080;
}

server {
    listen 80;
    location / {
        proxy_pass http://cloudflow_center;
        proxy_set_header Host $host;
    }
}
```

---

## 国产数据库配置

### 达梦DM8配置

```yaml
# config.yaml
relational_db:
  type: dameng
  host: localhost
  port: 5236
  user: SYSDBA
  password: ${DM_PASSWORD}
  database: cloudflow
  enable_dual_write: true
  dual_write_mode: ModeSyncWrite

timeseries_db:
  type: dameng
  host: localhost
  port: 5236
  user: SYSDBA
  password: ${DM_PASSWORD}
  database: cloudflow
```

### 人大金仓配置

```yaml
relational_db:
  type: kingbase
  host: localhost
  port: 54321
  user: system
  password: ${KINGBASE_PASSWORD}
  database: cloudflow
```

### GaussDB配置

```yaml
kv_store:
  type: gaussdb
  host: localhost
  port: 6379
  password: ${GAUSS_PASSWORD}
  database: 0
```

### OceanBase配置

```yaml
relational_db:
  type: oceanbase
  host: localhost
  port: 2881
  user: root@sys
  password: ${OCEANBASE_PASSWORD}
  database: cloudflow
```

---

## eBPF Agent部署

### 一键部署（推荐）

```bash
# 在业务服务器上执行
curl -sSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/cloud-flow-agent/deploy/install.sh | sudo bash
```

### 手动部署

```bash
# 1. 编译Agent
cd cloud-flow-agent
make release

# 2. 创建目录
mkdir -p /opt/cloudflow/agent

# 3. 复制文件
cp cloudflow-agent /opt/cloudflow/agent/
cp deploy/config.yaml /opt/cloudflow/agent/

# 4. 安装systemd服务
cp deploy/cloudflow-agent.service /etc/systemd/system/

# 5. 启动服务
systemctl daemon-reload
systemctl enable cloudflow-agent
systemctl start cloudflow-agent

# 6. 验证状态
systemctl status cloudflow-agent
journalctl -u cloudflow-agent -f
```

### 批量部署（Ansible）

```yaml
# deploy.yml
- hosts: all
  tasks:
    - name: 复制Agent二进制
      copy:
        src: cloudflow-agent
        dest: /opt/cloudflow/agent/
        mode: '0755'
    
    - name: 复制配置文件
      copy:
        src: config.yaml
        dest: /opt/cloudflow/agent/
    
    - name: 安装systemd服务
      copy:
        src: cloudflow-agent.service
        dest: /etc/systemd/system/
    
    - name: 启动Agent
      systemd:
        name: cloudflow-agent
        state: started
        enabled: yes
        daemon_reload: yes
```

```bash
# 执行批量部署
ansible-playbook -i hosts.ini deploy.yml
```

---

## 功能验证

### 1. 验证Agent注册

```bash
# 查看Agent列表
curl http://localhost:8080/api/v1/agents

# 预期输出：
# {
#   "agents": [
#     {
#       "id": "agent-xxx",
#       "status": "online",
#       "last_heartbeat": "..."
#     }
#   ]
# }
```

### 2. 验证流量采集

```bash
# 查看流量数据
curl "http://localhost:8080/api/v1/flows?limit=10"

# 查看VXLAN流量
curl "http://localhost:8080/api/v1/flows?vni=100"
```

### 3. 验证告警功能

```bash
# 创建告警规则
curl -X POST http://localhost:8080/api/v1/alerts/rules \
    -H "Content-Type: application/json" \
    -d '{
        "name": "高流量告警",
        "condition": "bytes > 1000000",
        "threshold": 1000000
    }'

# 查看告警列表
curl http://localhost:8080/api/v1/alerts
```

### 4. 验证数据库双写

```bash
# 切换到双写模式
# 修改config.yaml: dual_write_mode: ModeSyncWrite

# 重启服务后验证
# 检查两个数据库数据一致性
```

---

## 问题排查

### 常见问题

#### 问题1：eBPF编译失败

**症状：** `.bpf.o` 文件为0字节

**解决：**
```bash
# 1. 检查clang版本
clang --version  # 需要 >= 10.0

# 2. 安装内核头文件
apt install linux-headers-$(uname -r)
# 或
yum install kernel-devel-$(uname -r)

# 3. 重新编译
make ebpf-build
make ebpf-verify
```

#### 问题2：Agent无法连接Center

**症状：** Agent日志显示连接失败

**解决：**
```bash
# 1. 检查网络连通性
telnet center-ip 50051

# 2. 检查防火墙
iptables -L -n | grep 50051

# 3. 检查Agent配置
cat /opt/cloudflow/agent/config.yaml | grep center
```

#### 问题3：数据库连接失败

**症状：** 服务日志显示数据库连接错误

**解决：**
```bash
# 1. 检查数据库服务状态
systemctl status mysql
systemctl status clickhouse-server

# 2. 检查配置文件密码
cat config.yaml | grep -A5 password

# 3. 手动测试连接
mysql -h host -u user -p database
clickhouse-client -h host -d database
```

#### 问题4：没有采集到流量

**症状：** flows接口返回空

**解决：**
```bash
# 1. 检查Agent状态
systemctl status cloudflow-agent

# 2. 检查网卡配置
cat /opt/cloudflow/agent/config.yaml | grep interface

# 3. 检查eBPF加载
lsmod | grep bpf

# 4. 查看Agent日志
journalctl -u cloudflow-agent -f
```

### 日志查看

```bash
# Center日志
journalctl -u cloudflow-center -f

# Agent日志
journalctl -u cloudflow-agent -f

# Docker容器日志
docker-compose logs -f mysql
docker-compose logs -f clickhouse
docker-compose logs -f kafka
```

### 性能调优

```bash
# 提高文件描述符限制
ulimit -n 65536

# 优化网络参数
sysctl -w net.core.rmem_max=16777216
sysctl -w net.core.wmem_max=16777216
sysctl -w net.core.netdev_max_backlog=5000
```
