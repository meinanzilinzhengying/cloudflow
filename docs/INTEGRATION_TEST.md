# CloudFlow 集成测试指南

## 目录

1. [环境准备](#环境准备)
2. [数据库环境搭建](#数据库环境搭建)
3. [eBPF编译验证](#ebpf编译验证)
4. [核心功能测试](#核心功能测试)
5. [gRPC通信测试](#grpc通信测试)
6. [告警规则验证](#告警规则验证)
7. [双写模式5级迁移验证](#双写模式5级迁移验证)
8. [常见问题排查](#常见问题排查)

---

## 环境准备

### 1.1 系统要求

| 组件 | 版本要求 | 说明 |
|------|----------|------|
| Go | 1.22.x | 所有模块统一版本 |
| Clang/LLVM | 10+ | eBPF程序编译 |
| Docker | 20.10+ | 中间件容器化部署 |
| Docker Compose | 2.0+ | 服务编排 |
| Linux Kernel | >= 5.4 | eBPF支持，推荐5.10+ |

### 1.2 开发环境安装

#### CentOS/RHEL 7/8
```bash
# 安装Go 1.22
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz

# 安装编译工具
yum install -y clang llvm libbpf-devel make gcc

# 安装Docker
yum install -y docker docker-compose
systemctl start docker
```

#### Ubuntu/Debian
```bash
# 安装Go 1.22
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz

# 安装编译工具
apt install -y clang llvm libbpf-dev make gcc

# 安装Docker
apt install -y docker.io docker-compose
systemctl start docker
```

#### 麒麟V10/UOS
```bash
# 使用系统源安装
yum install -y clang llvm libbpf-devel
# 或从官网下载Go 1.22
```

### 1.3 环境变量配置

```bash
# 必须配置（生产环境）
export CLOUD_FLOW_JWT_SECRET="your-32-character-secret-key-here-min-32-chars"
export CORS_ALLOWED_ORIGINS="http://localhost:3000,http://localhost:8080"

# 数据库配置
export MYSQL_PASSWORD="your-mysql-password"
export CLICKHOUSE_PASSWORD="your-clickhouse-password"

# 可选配置
export CLOUD_FLOW_ALLOWED_ORIGINS="http://your-domain.com"
```

---

## 数据库环境搭建

### 2.1 MySQL 环境（关系型存储）

#### Docker 部署
```bash
docker run -d \
  --name cloudflow-mysql \
  -p 3306:3306 \
  -e MYSQL_ROOT_PASSWORD=CloudFlow2024 \
  -e MYSQL_DATABASE=cloudflow \
  -v mysql-data:/var/lib/mysql \
  mysql:8.0 \
  --default-authentication-plugin=mysql_native_password
```

#### 初始化数据库
```bash
# 执行初始化脚本
mysql -h 127.0.0.1 -u root -pCloudFlow2024 cloudflow < scripts/mysql/schema.sql

# 验证表结构
mysql -h 127.0.0.1 -u root -pCloudFlow2024 -e "SHOW TABLES;" cloudflow
```

### 2.2 ClickHouse 环境（时序存储）

#### Docker 部署
```bash
docker run -d \
  --name cloudflow-clickhouse \
  -p 8123:8123 \
  -p 9000:9000 \
  -e CLICKHOUSE_DB=cloudflow \
  -e CLICKHOUSE_USER=default \
  -e CLICKHOUSE_PASSWORD=ClickHouse2024 \
  -v clickhouse-data:/var/lib/clickhouse \
  clickhouse/clickhouse-server:23.8
```

#### 验证连接
```bash
curl "http://localhost:8123/?query=SELECT%20version()"
```

### 2.3 达梦DM8 环境（国产数据库）

#### Docker 部署
```bash
# 拉取达梦镜像
docker pull dm8/dm8:latest

# 启动容器
docker run -d \
  --name cloudflow-dm8 \
  -p 5236:5236 \
  -e PAGE_SIZE=8 \
  -e LD_LIBRARY_PATH=/opt/dmdbms/bin \
  -v dm8-data:/opt/dmdbms/data \
  dm8/dm8:latest
```

#### 配置连接
```yaml
# config.yaml 达梦配置示例
relational_db:
  type: dameng
  host: 127.0.0.1
  port: 5236
  user: SYSDBA
  password: SYSDBA
  database: CLOUDFLOW
```

### 2.4 Redis 环境（KV存储）

```bash
docker run -d \
  --name cloudflow-redis \
  -p 6379:6379 \
  -v redis-data:/data \
  redis:7-alpine
```

---

## eBPF编译验证

### 3.1 编译前检查

```bash
cd cloud-flow-agent

# 检查clang版本
clang --version

# 检查内核头文件
ls /lib/modules/$(uname -r)/build
```

### 3.2 编译eBPF程序

```bash
cd cloud-flow-agent

# 编译所有eBPF程序
make ebpf-build

# 验证编译结果（必须全部>0字节）
make ebpf-verify
```

**预期输出**:
```
Verifying eBPF bytecode files...
All eBPF bytecode files verified successfully!
```

### 3.3 手动编译命令

```bash
cd cloud-flow-agent/internal/ebpfcollector/bpf

# 逐个编译
clang -O2 -target bpf -c dns_full_bpf.bpf.c -o dns_full_bpf.bpf.o
clang -O2 -target bpf -c http_full_bpf.bpf.c -o http_full_bpf.bpf.o
clang -O2 -target bpf -c http_metrics_bpf.bpf.c -o http_metrics_bpf.bpf.o
clang -O2 -target bpf -c mysql_full_bpf.bpf.c -o mysql_full_bpf.bpf.o
clang -O2 -target bpf -c tc_bpf.bpf.c -o tc_bpf.bpf.o
clang -O2 -target bpf -c tcp_metrics_bpf.bpf.c -o tcp_metrics_bpf.bpf.o

# 验证大小
ls -lh *.bpf.o
```

### 3.4 常见编译错误

| 错误 | 原因 | 解决方案 |
|------|------|----------|
| `clang: command not found` | 未安装clang | `yum install clang llvm` |
| `linux/bpf.h: No such file` | 缺少内核头文件 | `yum install kernel-devel-$(uname -r)` |
| `CO-RE target built without BTF` | 内核未开启BTF | 使用非CO-RE模式或升级内核 |

---

## 核心功能测试

### 4.1 编译验证

```bash
# 所有模块编译
export GOPATH=/tmp/gopath
go build ./...

# 静态检查
go vet ./...

# 依赖检查
go mod tidy
```

### 4.2 数据库抽象层测试

```bash
cd pkg/storage

# 运行单元测试
go test -v ./... -cover

# 测试覆盖率
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### 4.3 双写模式测试

#### Mode 0: OldOnly（仅旧库）
```yaml
storage:
  dual_write_mode: 0  # ModeOldOnly
```
验证：仅写入MySQL，达梦无写入

#### Mode 1: AsyncWrite（异步双写）
```yaml
storage:
  dual_write_mode: 1  # ModeAsyncWrite
```
验证：MySQL成功后异步写达梦，不阻塞主流程

#### Mode 2: SyncWrite（同步双写）
```yaml
storage:
  dual_write_mode: 2  # ModeSyncWrite
```
验证：两个库都成功才返回

#### Mode 3: ReadSplit（读流量切分）
```yaml
storage:
  dual_write_mode: 3  # ModeReadSplit
  read_split_ratio: 0.5  # 50%流量到新库
```
验证：读请求按比例分发

#### Mode 4: NewOnly（仅新库）
```yaml
storage:
  dual_write_mode: 4  # ModeNewOnly
```
验证：所有读写都走新库

---

## gRPC通信测试

### 5.1 服务端启动

```bash
# 启动控制平面
cd services/control-plane
go run cmd/main.go --config ../../config.yaml

# 启动数据平面
cd services/data-plane
go run cmd/main.go --config ../../config.yaml
```

### 5.2 客户端连接测试

```bash
# 检查gRPC端口监听
netstat -tlnp | grep 50051

# 使用grpcurl测试
grpcurl -plaintext localhost:50051 list

# 测试心跳接口
grpcurl -plaintext -d '{"probe_id": "test-agent"}' \
  localhost:50051 cloudflow.ProbeService/Heartbeat
```

### 5.3 配置下发测试

```bash
# 测试GetConfig接口
grpcurl -plaintext -d '{"probe_id": "test-agent"}' \
  localhost:50051 cloudflow.ProbeService/GetConfig
```

预期响应包含：
- `config_version`: 配置版本号
- `config_yaml`: YAML配置内容
- `checksum`: SHA256校验和

### 5.4 流式数据上报测试

验证双向流：
1. Agent建立StreamData连接
2. 批量发送Flow/Metrics数据
3. 服务端返回ACK
4. 服务端可下发指令

---

## 告警规则验证

### 6.1 告警引擎启动

```bash
cd services/alert-engine
go run cmd/main.go --config ../../config.yaml
```

### 6.2 规则验证

```bash
# 创建阈值告警规则
curl -X POST http://localhost:8080/api/v1/alerts/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "高流量告警",
    "metric": "bytes_total",
    "threshold": 1000000000,
    "operator": ">",
    "duration": "5m"
  }'

# 查看告警列表
curl http://localhost:8080/api/v1/alerts
```

### 6.3 通知渠道测试

```bash
# 测试Webhook通知
curl -X POST http://localhost:8080/api/v1/alerts/test \
  -H "Content-Type: application/json" \
  -d '{"channel": "webhook", "url": "https://your-webhook.com"}'
```

---

## 双写模式5级迁移验证

### 7.1 迁移流程验证

| 阶段 | 操作 | 验证点 |
|------|------|--------|
| 1 | Mode 0 → Mode 1 | 双写开始，从库数据同步 |
| 2 | Mode 1 → Mode 2 | 同步双写，一致性校验 |
| 3 | 全量数据迁移 | 行数校验、抽样校验 |
| 4 | Mode 2 → Mode 3 | 读流量灰度 1%→10%→50%→100% |
| 5 | Mode 3 → Mode 4 | 下线旧库 |

### 7.2 数据一致性校验

```bash
# 行数校验
SELECT COUNT(*) FROM flows;  -- MySQL
SELECT COUNT(*) FROM flows;  -- 达梦

# 抽样校验（随机100条对比）
SELECT * FROM flows ORDER BY RAND() LIMIT 100;

# 自动化校验脚本
bash scripts/verify_data_consistency.sh
```

### 7.3 回滚验证

任何阶段都可立即回滚：
```yaml
# 立即回滚到Mode 0
storage:
  dual_write_mode: 0
```
验证：5分钟内恢复旧库独立运行

---

## 常见问题排查

### 8.1 编译问题

**Q: prometheus/common版本冲突**
```
go: module github.com/prometheus/common@v0.66.1 requires go >= 1.23.0
```
A: 已修复，所有go.mod都添加了replace到v0.48.0

**Q: eBPF .bpf.o都是0字节**
A: 执行 `cd cloud-flow-agent && make ebpf-build`

### 8.2 运行时问题

**Q: panic: JWT secret key must be set**
A: 设置环境变量 `CLOUD_FLOW_JWT_SECRET`（至少32位）

**Q: CORS跨域错误**
A: 设置 `CORS_ALLOWED_ORIGINS` 环境变量

### 8.3 数据库问题

**Q: 达梦SQL语法错误**
A: SQL方言自动转换已实现，检查是否使用了存储抽象层

**Q: 双写模式数据不一致**
A: 检查日志中的 `dual write failed` 错误

### 8.4 性能问题

**Q: 内存持续增长**
A: 检查goroutine泄漏，确保所有循环都有context取消通道

**Q: 高CPU占用**
A: 调整采样率，检查eBPF map大小

---

## 测试验收标准

| 检查项 | 通过标准 |
|--------|----------|
| 所有模块编译 | `go build ./...` 无错误 |
| 单元测试 | 覆盖率 > 80% |
| eBPF编译 | 所有.bpf.o > 0字节 |
| 数据库连接 | MySQL/ClickHouse/达梦都能连接 |
| 双写5级模式 | 所有模式切换正常 |
| gRPC通信 | 心跳/配置/流式都正常 |
| 告警规则 | 阈值触发+通知正常 |
| 回滚机制 | 5分钟内可回滚 |

---

**文档版本**: v1.0
**最后更新**: 2026-06-15
