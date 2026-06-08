# CloudFlow 云原生网络流量分析平台

<div align="center">

![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg?logo=go)
![Docker](https://img.shields.io/badge/docker-ready-2496ED.svg?logo=docker)
![Vue](https://img.shields.io/badge/vue-3.4-4FC08D.svg?logo=vue.js)

**高性能云原生网络流量分析平台 | 基于 eBPF 实现 L3-L7 全栈可观测性**

*CloudFlow is a high-performance cloud-native network traffic analysis platform for L3-L7 full-stack observability based on eBPF*

[English](README.md) | [简体中文](README.md)

</div>

---

## 🌟 核心特性

### 🚀 高性能架构
- **超高性能**: 支持 100K+ flows/sec 采集，<1% CPU 开销
- **微服务架构**: 所有后端模块容器化，Docker Compose 一键部署
- **全栈可视**: L3-L7 协议全覆盖，从网络层到应用层

### 🔍 三大探针部署方式
- **ECS 直接安装**: SSH 远程安装到目标服务器
- **K8s Node 节点**: DaemonSet 自动部署到集群所有节点
- **K8s Pod 探针**: 通过 API + Token 获取集群配置信息

### 🤖 AI 智能分析
- **多模型支持**: DeepSeek / Qwen / OpenAI GPT-4
- **智能问答**: AI 协助分析监控数据
- **自动诊断**: 智能识别异常和性能瓶颈

### 📊 双前端设计
- **业务监控前端** (`:3002`): 业务流量分析、应用性能监控
- **平台自监控前端** (`3003`): 平台自身监控、探针管理

---

## 🏗️ 系统架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         CloudFlow 微服务架构                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    前端层 (容器化)                            │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │   │
│  │  │ 业务监控前端 │  │ 平台监控前端 │  │  原生前端   │          │   │
│  │  │   (:3002)   │  │   (:3003)   │  │   (:8080)   │          │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘          │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                    │                                 │
│  ┌────────────────────────────────────────────────────────────────┐│
│  │                     微服务层 (容器化)                            ││
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            ││
│  │  │ Auth Service│  │Tenant Service│  │Control Plane│            ││
│  │  │   (:8006)   │  │   (:8010)   │  │   (:8001)   │            ││
│  │  └─────────────┘  └─────────────┘  └─────────────┘            ││
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            ││
│  │  │ Data Plane  │  │Query Service │  │ AI Service │            ││
│  │  │   (:9002)   │  │   (:8007)   │  │   (:8082)   │            ││
│  │  └─────────────┘  └─────────────┘  └─────────────┘            ││
│  │  ┌─────────────┐  ┌─────────────┐                            ││
│  │  │Topology Eng.│  │ Alert Engine│                            ││
│  │  │   (:8008)   │  │   (:8009)   │                            ││
│  │  └─────────────┘  └─────────────┘                            ││
│  └────────────────────────────────────────────────────────────────┘│
│                                    │                                 │
│  ┌────────────────────────────────────────────────────────────────┐│
│  │                    存储层 (容器化)                               ││
│  │  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐  ││
│  │  │  TiDB  │  │ Redis  │  │  etcd  │  │ClickHouse│ │ Kafka  │  ││
│  │  │:4000  │  │:6379  │  │:2379  │  │ :8123  │  │ :9092  │  ││
│  │  └────────┘  └────────┘  └────────┘  └────────┘  └────────┘  ││
│  └────────────────────────────────────────────────────────────────┘│
│                                                                      │
│  ┌────────────────────────────────────────────────────────────────┐│
│  │                  监控与可观测性 (容器化)                          ││
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌────────────┐  ││
│  │  │Prometheus│  │  Grafana  │  │   Loki    │  │   Jaeger   │  ││
│  │  │  (:9091)  │  │  (:3001)  │  │  (:3100)  │  │  (:16686)  │  ││
│  │  └───────────┘  └───────────┘  └───────────┘  └────────────┘  ││
│  └────────────────────────────────────────────────────────────────┘│
│                                                                      │
│  ═══════════════════════════════════════════════════════════════════ │
│                           探针层 (独立部署)                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │
│  │   ECS 探针   │  │ K8s Node 探针│  │ K8s Pod 探针│              │
│  │  (SSH 安装)   │  │ (DaemonSet)  │  │(API+Token)  │              │
│  └──────────────┘  └──────────────┘  └──────────────┘              │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 📦 支持的操作系统

### 🖥️ 一键部署支持
- ✅ **CentOS** 7/8/9
- ✅ **Red Hat Enterprise Linux** 7/8/9
- ✅ **Rocky Linux** 8/9
- ✅ **AlmaLinux** 8/9
- ✅ **麒麟 V10** (Kylin)
- ✅ **欧拉 openEuler** 20/21/22
- ✅ **华为 EulerOS**
- ✅ **Debian** 10/11/12
- ✅ **Ubuntu** 18/20/22/24 LTS
- ✅ **Fedora** 36/37/38

### 🔧 探针支持
- ✅ **x86_64** (Intel/AMD/海光)
- ✅ **aarch64** (ARM64/鲲鹏)

---

## 🚀 快速开始

### 一键部署（推荐）

```bash
# 下载并运行一键部署脚本
curl -fsSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/scripts/install.sh | bash

# 或下载脚本后运行
wget https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/scripts/install.sh
chmod +x install.sh
sudo ./install.sh
```

### 手动部署

```bash
# 1. 克隆代码
git clone https://github.com/meinanzilinzhengying/cloudflow.git
cd cloudflow

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env 文件设置密码和配置

# 3. 启动所有服务
docker compose up -d

# 4. 检查服务状态
docker compose ps

# 5. 查看日志
docker compose logs -f
```

### 部署选项

```bash
# 跳过 Docker 安装（已安装 Docker）
sudo ./install.sh --skip-docker

# 跳过 Git 克隆（已克隆代码）
sudo ./install.sh --skip-git

# 调试模式
sudo ./install.sh --debug
```

---

## 🌐 访问地址

部署完成后，访问以下地址：

| 服务 | 地址 | 说明 |
|------|------|------|
| 业务监控前端 | http://服务器IP:3002 | 业务流量分析 |
| 平台监控前端 | http://服务器IP:3003 | 平台自监控 |
| 原生前端 | http://服务器IP:8080 | 统一入口 |
| Grafana | http://服务器IP:3001 | 监控仪表盘 |
| Prometheus | http://服务器IP:9091 | 指标查询 |
| AI 服务 | http://服务器IP:8082 | AI 分析 API |

### 默认凭证

- **Grafana**: `admin` / `admin`
- **TiDB**: `root` / (见 .env 中的 `CLOUD_FLOW_DB_PASSWORD`)
- **ClickHouse**: `default` / `ClickHouse2024Secure`

---

## 🔍 探针部署

### 方式一：ECS 直接安装

```bash
# 在目标服务器上执行
curl -fsSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/cloud-flow-agent/scripts/install.sh | bash

# 或下载安装脚本
wget https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/cloud-flow-agent/scripts/install.sh
chmod +x install.sh
sudo ./install.sh
```

### 方式二：K8s Node 节点部署

```bash
# 通过前端页面部署
# 访问平台监控前端 → 探针管理 → K8s 部署
# 填写 K8s 集群信息即可自动部署 DaemonSet
```

### 方式三：K8s Pod 探针

```bash
# 通过前端页面配置
# 访问平台监控前端 → 探针管理 → K8s 部署
# 填写 API Server 地址和 Token
# 系统自动获取集群配置信息（Pod、命名空间、服务等）
```

---

## 🤖 AI 分析配置

### 配置 API Key

```bash
# 编辑 .env 文件
vim .env

# 添加以下配置
DEEPSEEK_API_KEY=your_deepseek_api_key
QWEN_API_KEY=your_qwen_api_key
OPENAI_API_KEY=your_openai_api_key

# 重启 AI 服务
docker compose restart ai-service
```

### API 调用示例

```bash
# 健康检查
curl http://localhost:8082/health

# 获取可用模型
curl http://localhost:8082/api/models

# 发送分析请求
curl -X POST http://localhost:8082/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "session_id": "user123",
    "messages": [
      {"role": "user", "content": "帮我分析今天的服务器监控数据"}
    ]
  }'
```

---

## 📂 项目结构

```
cloudflow/
├── cloud-flow-agent/              # 探针模块 (独立部署)
│   ├── cmd/                      # 入口点
│   ├── internal/
│   │   ├── ebpfcollector/        # eBPF 数据采集
│   │   ├── l7parser/             # L7 协议解析
│   │   └── collector/            # 指标收集器
│   ├── deployments/
│   │   ├── k8s/                  # K8s 部署配置
│   │   │   ├── 00-namespace.yaml
│   │   │   ├── 01-serviceaccount.yaml
│   │   │   ├── 02-rbac.yaml
│   │   │   ├── 03-configmap.yaml
│   │   │   ├── 04-secret.yaml
│   │   │   └── 05-daemonset.yaml
│   │   └── Dockerfile
│   └── scripts/
│       └── install.sh            # 安装脚本
│
├── cloud-flow-ai/                # AI 服务 (容器化)
│   ├── cmd/main.go              # 服务入口
│   ├── configs/config.yaml      # 配置文件
│   ├── deployments/Dockerfile
│   └── internal/
│       ├── config/              # 配置管理
│       ├── llm/                 # LLM 客户端
│       └── server/              # HTTP 服务
│
├── services/                     # 微服务模块 (容器化)
│   ├── auth-service/            # 认证服务
│   ├── tenant-service/         # 租户服务
│   ├── control-plane/          # 控制平面
│   ├── data-plane/             # 数据平面
│   ├── query-service/          # 查询服务
│   ├── topology-engine/        # 拓扑引擎
│   ├── alert-engine/           # 告警引擎
│   ├── deployments/migrations/ # 数据库迁移
│   └── shared/                 # 共享库
│
├── cloud-flow-business/         # 业务监控前端 (容器化)
│   ├── src/
│   │   ├── components/         # 组件
│   │   ├── views/              # 页面
│   │   └── App.vue            # 入口
│   ├── Dockerfile
│   └── nginx.conf
│
├── cloud-flow-platform/         # 平台监控前端 (容器化)
│   ├── src/
│   │   ├── components/         # 组件
│   │   ├── views/             # 页面
│   │   ├── config/            # 功能配置
│   │   └── App.vue           # 入口
│   ├── Dockerfile
│   └── nginx.conf
│
├── cloud-flow-frontend/         # 原生前端 (容器化)
│
├── monitoring/                  # 监控配置
│   ├── prometheus/             # Prometheus
│   ├── grafana/                # Grafana
│   ├── promtail/              # 日志采集
│   └── alertmanager/          # 告警管理
│
├── scripts/
│   └── install.sh              # ⭐ 一键部署脚本
│
├── docker-compose.yml           # ⭐ Docker Compose 配置
├── .env.example                # 环境变量模板
└── README.md                   # 本文档
```

---

## 🔧 配置说明

### 环境变量 (.env)

```bash
# 数据库配置
CLOUD_FLOW_DB_PASSWORD=your_secure_password

# Grafana
GRAFANA_ADMIN_PASSWORD=your_grafana_password

# ClickHouse
CLICKHOUSE_USER=default
CLICKHOUSE_PASSWORD=ClickHouse2024Secure

# AI 服务 (可选)
DEEPSEEK_API_KEY=
QWEN_API_KEY=
OPENAI_API_KEY=

# JWT 密钥
CLOUDFLOW_JWT_SECRET_KEY=your_jwt_secret_key
```

### 探针配置

```yaml
# cloud-flow-agent/configs/config.yaml
collector:
  enabled: true
  sample_rate: 1.0
  max_flows_per_second: 100000

ebpf:
  buffer_size: 8388608
  perf_ring_size: 65536

l7:
  parsers:
    - http
    - grpc
    - mysql
    - redis
    - kafka
    - dns

kubernetes:
  enabled: true
  kubeconfig: ~/.kube/config
```

---

## 📊 功能特性

### 业务监控前端 (`:3002`)
- 🌐 **流量分析**: 实时业务流量监控和分析
- 📈 **应用性能**: APM 应用性能监控
- 🔗 **拓扑视图**: 服务依赖关系可视化
- 🚨 **告警中心**: 智能告警规则和通知

### 平台监控前端 (`:3003`)
- 🖥️ **探针管理**: 三种部署方式一键下发
  - ECS 直接安装 (SSH)
  - K8s Node 节点 (DaemonSet)
  - K8s Pod 探针 (API + Token)
- 📊 **平台自监控**: 平台自身健康状态
- 🤖 **AI 分析**: AI 智能分析和诊断
- ⚙️ **系统设置**: 配置管理和用户管理

### AI 服务
- 💬 **智能问答**: 自然语言查询监控数据
- 🔍 **异常诊断**: AI 自动识别异常
- 📋 **分析报告**: 自动生成分析报告
- 🔄 **多模型切换**: 支持 DeepSeek/Qwen/GPT-4

---

## 🐛 故障排查

### 服务无法启动

```bash
# 查看所有服务日志
docker compose logs -f

# 查看特定服务
docker compose logs -f auth-service

# 重启所有服务
docker compose restart

# 完全重建
docker compose down -v
docker compose up -d
```

### 探针无法连接

```bash
# 检查探针状态
docker exec -it cloudflow-control-plane curl http://localhost:8001/healthz

# 检查网络连通性
docker exec -it cloudflow-control-plane ping cloudflow-agent

# 查看探针日志
docker compose logs -f cloudflow-agent
```

### 前端无法访问

```bash
# 检查前端服务
docker compose ps frontend

# 检查 Nginx 配置
docker exec -it cloudflow-business-frontend cat /etc/nginx/nginx.conf

# 重启前端
docker compose restart business-frontend platform-frontend
```

---

## 🔐 安全建议

### 生产环境部署

1. **修改默认密码**: 立即修改所有默认密码
2. **配置 HTTPS**: 使用反向代理配置 SSL/TLS
3. **网络隔离**: 使用 Docker 网络隔离
4. **防火墙**: 限制端口访问
5. **定期更新**: 保持镜像最新版本

### Docker 安全

```bash
# 使用 Docker 网络
docker network create cloudflow-net

# 配置资源限制
docker update --memory 2G --cpus 2 <container_id>

# 启用 Docker 日志轮转
sudo tee /etc/docker/daemon.json <<EOF
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "100m",
    "max-file": "3"
  }
}
EOF
sudo systemctl restart docker
```

---

## 📈 性能优化

### 推荐配置

| 组件 | CPU | 内存 | 磁盘 |
|------|-----|------|------|
| TiDB | 4 核 | 8GB | 100GB SSD |
| ClickHouse | 8 核 | 16GB | 200GB SSD |
| Kafka | 4 核 | 8GB | 100GB |
| Redis | 2 核 | 4GB | 10GB |
| 微服务 (x7) | 2 核 x7 | 2GB x7 | - |

### 高可用配置

生产环境建议：
- TiDB: 3 节点集群
- Kafka: 3 节点集群，副本因子 3
- ClickHouse: 3 节点集群
- Redis: 主从 + Sentinel

---

## 🤝 贡献指南

欢迎贡献！请阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 了解更多。

### 开发环境

```bash
# 克隆代码
git clone https://github.com/meinanzilinzhengying/cloudflow.git
cd cloudflow

# 安装 Go 依赖
go mod download

# 安装前端依赖
cd cloud-flow-platform && npm install
cd ../cloud-flow-business && npm install

# 运行测试
go test ./...
npm test
```

---

## 📜 许可证

本项目采用 **Apache License 2.0** 开源许可证。

详见 [LICENSE](LICENSE) 文件。

---

## 🙏 致谢

感谢以下开源项目：

- [cilium/ebpf](https://github.com/cilium/ebpf) - eBPF Go 库
- [ClickHouse](https://clickhouse.com/) - 高性能列式数据库
- [TiDB](https://tidb.io/) - 分布式 NewSQL 数据库
- [Kafka](https://kafka.apache.org/) - 分布式流处理平台
- [Prometheus](https://prometheus.io/) - 监控系统
- [Grafana](https://grafana.com/) - 可视化平台
- [Vue.js](https://vuejs.org/) - 前端框架
- [Go](https://go.dev/) - 编程语言

---

## 📞 联系方式

- 📧 邮件: [cloudflow@meinanzilinzhengying.com](mailto:cloudflow@meinanzilinzhengying.com)
- 💬 Issues: [GitHub Issues](https://github.com/meinanzilinzhengying/cloudflow/issues)
- 📖 文档: [Wiki](https://github.com/meinanzilinzhengying/cloudflow/wiki)

---

<div align="center">

**如果 CloudFlow 对您有帮助，请给我们一个 ⭐ Star！**

Made with ❤️ by CloudFlow Team

</div>
