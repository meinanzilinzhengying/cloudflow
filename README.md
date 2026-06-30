# CloudFlow - eBPF 网络可观测平台

[![Go Version](https://img.shields.io/badge/Go-1.22-blue)](https://go.dev/)
[![Vue 3](https://img.shields.io/badge/Vue-3.4-brightgreen)](https://vuejs.org/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.4-blue)](https://www.typescriptlang.org/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

> **CloudFlow** 是一款基于 eBPF 技术的网络可观测性平台，提供从内核态数据采集、协议解析、可视化展示到智能分析的全链路解决方案。对标阿里云 ARMS、腾讯云 APM、Datadog 等产品。

---

## 架构概览

```
┌─────────────────────────────────────────────────────────┐
│                   可视化层 (Frontend)                     │
│  ┌─────────────────┐  ┌──────────────────────────────┐ │
│  │ 业务监控前端      │  │ 平台自监控前端                │ │
│  │ Vue 3 + ECharts │  │ React 18 + ECharts           │ │
│  │ Port: 8080       │  │ Port: 3003                   │ │
│  └────────┬────────┘  └───────────┬──────────────────┘ │
└───────────┼──────────────────────┼──────────────────────┘
            │ HTTP/API             │ HTTP/API
┌───────────┼──────────────────────┼──────────────────────┐
│           ▼                      ▼                       │
│  ┌─────────────────────────────────────────────────┐    │
│  │           Nginx 反向代理 / API 网关               │    │
│  └──────┬──────────┬──────────┬──────────┬─────────┘    │
│         │          │          │          │               │
│  ┌──────▼──┐ ┌─────▼───┐ ┌───▼────┐ ┌───▼────────┐    │
│  │ 中心服务 │ │ AI 分析 │ │ 查询服务│ │ 告警引擎    │    │
│  │ :9090   │ │ :8082   │ │ :8007  │ │ :9002      │    │
│  └──────┬──┘ └─────────┘ └───┬────┘ └───┬────────┘    │
│         │                    │           │               │
│  ┌──────▼────────────────────▼───────────▼──────────┐ │
│  │               ClickHouse 数据仓库                 │ │
│  │              (HTTP 8123 / Native 9000)            │ │
│  └─────────────────────┬────────────────────────────┘ │
│                        │                              │
│  ┌─────────────────────▼────────────────────────────┐ │
│  │          Data Ingest 服务 (Python)               │ │
│  │          HTTP POST /api/v1/ingest :9104          │ │
│  │          Redis 缓冲 + 去重 + 批量写入             │ │
│  └──────┬──────────────────────────────┬───────────┘ │
│         │                              │               │
│  ┌──────▼──────────┐       ┌───────────▼───────────┐ │
│  │ eBPF Agent (x86)│       │ eBPF Agent (ARM STB)   │ │
│  │ VM2 探针        │       │ Android机顶盒探针       │ │
│  │ 6个eBPF采集器   │       │ 6个eBPF采集器           │ │
│  │ + 2个非eBPF采集 │       │ + 2个非eBPF采集         │ │
│  └─────────────────┘       └───────────────────────┘ │
│                                                        │
│   ✅ 数据管道: 探针 → EdgeClient → data-ingest →      │
│               Redis → ClickHouse → 前端             │
└─────────────────────────────────────────────────────────┘
```

---

## 核心功能

### 内核级数据采集
- **eBPF Agent**：基于Go + AF_PACKET + kprobe，无需修改应用代码
- **6个eBPF采集器**：网络流、TCP连接跟踪、文件I/O、进程执行、On-CPU采样、TCP重传检测
- **2个非eBPF采集器**：协议分析（HTTP/DNS）、主机指标（CPU/内存/磁盘/网络IO）
- **ARM32支持**：已验证在Android机顶盒（Linux 5.4.210，无BTF）上稳定运行

### 多维度可视观测
- **总览仪表盘**：流量趋势、协议分布、TOP主机、告警概览
- **网络流量**：实时流日志、通信矩阵、网络拓扑图
- **L7协议**：HTTP请求详情、DNS查询日志
- **系统性能**：CPU/内存/IO/进程 2×2监控网格
- **安全审计**：事件时间线、异常检测
- **探针管理**：状态监控、配置热更新

### 零侵入部署
- 无需修改业务代码
- 无需重启应用
- ADB推送即可部署到存量STB设备
- 资源开销：CPU <5%，内存 <45MB

---

## 目录结构

```
cloudflow/
├── cloud-flow-center/       # 中心API服务器 (Go, :9090)
├── cloud-flow-agent/        # eBPF探针Agent (Go)
├── cloud-flow-frontend/     # 业务监控前端 (Vue 3 + TS + ECharts, :8080)
├── cloud-flow-platform/     # 平台自监控前端 (React 18 + Vite, :3003)
├── cloud-flow-edge/         # 边缘数据处理服务 (Go, :9102)
├── cloud-flow-ai/           # AI分析服务 (Go/Python, :8082)
├── ebpf-probe/              # eBPF探针 (12 BPF采集器 + 7用户态分析器)
├── services/                # Go微服务集群
│   ├── alert-engine/        #   告警引擎 (:9002)
│   ├── data-plane/          #   数据面 (:9102)
│   ├── control-plane/       #   控制面 (:8001)
│   ├── query-service/       #   查询服务 (:8007)
│   ├── auth-service/        #   认证服务 (:8003)
│   ├── tenant-service/      #   租户服务
│   └── topology-engine/     #   拓扑引擎 (:8004)
├── pkg/                     # 共享Go包
├── internal/                # 内部包
├── proto/                   # gRPC协议定义
├── config/                  # 配置文件
├── scripts/                 # 部署脚本
├── k8s/                     # Kubernetes资源清单
├── helm/                    # Helm Charts
├── monitoring/              # 监控配置 (Prometheus/Grafana/Alertmanager)
├── docs/                    # 文档
│
├── data-ingest-service.py   # 数据接入服务 (Python, :9104)
├── docker-compose.yml       # Docker Compose编排
├── go.work                  # Go多模块工作区
├── Makefile                 # 构建命令
└── README.md
```

---

## 技术栈

### 后端 (Go)

| 组件 | 框架/库 | 用途 |
|------|---------|------|
| Gin | v1.10 | HTTP路由框架 |
| gRPC | protobuf v3 | 服务间通信 |
| ClickHouse | clickhouse-go/v2 | 时序数据存储 |
| Redis | go-redis | 队列+缓存 |
| eBPF | libbpf + Cilium ebpf | 内核态数据采集 |
| Prometheus | client_golang | 指标暴露 |

### 前端

| 项目 | 框架 | 构建工具 |
|------|------|---------|
| 业务监控 (cloud-flow-frontend) | Vue 3 + TS | Vite |
| 平台自监控 (cloud-flow-platform) | React 18 + TS | Vite |
| 可视化库 | ECharts 5 + vue-echarts | — |
| UI组件 | Element Plus / Ant Design | — |

### 基础设施

| 组件 | 版本 | 用途 |
|------|------|------|
| ClickHouse | 26.7.1 | 数据仓库 |
| Redis | 6+ | 消息队列 |
| Nginx | 1.24 | 反向代理 |
| Prometheus | 2.x | 指标采集 |
| Grafana | 10.x | 可视化面板 |
| Alertmanager | 0.27 | 告警管理 |
| Jaeger | 1.x | 分布式追踪 |

---

## 快速开始

### 前置要求

- Go 1.22+
- Node.js 20+
- ClickHouse 26+
- Docker + Docker Compose (可选)

### 后端编译

```bash
# Go多模块工作区
cd cloudflow-build
go work init
go work use ./...

# 编译各服务
go build -o bin/cloudflow-center    ./cloud-flow-center
go build -o bin/cloudflow-agent     ./cloud-flow-agent
go build -o bin/alert-engine        ./services/alert-engine/cmd
go build -o bin/data-plane          ./services/data-plane/cmd
go build -o bin/control-plane       ./services/control-plane/cmd
go build -o bin/query-service       ./services/query-service/cmd
go build -o bin/auth-service        ./services/auth-service/cmd
```

### 前端构建

```bash
# 业务监控前端 (cloud-flow-frontend)
cd cloud-flow-frontend
npm install
npm run dev       # 开发模式 :8080
npm run build     # 生产构建

# 平台自监控前端 (cloud-flow-platform)
cd cloud-flow-platform
npm install
npm run dev       # 开发模式 :3003
npm run build     # 生产构建
```

### Docker 部署

```bash
cd cloudflow-build
docker compose up -d              # 启动全部服务
docker compose ps                 # 查看状态
docker compose logs -f <service>  # 查看日志
```

---

## eBPF 探针

### 系统要求

- Linux 内核 ≥ 4.15 (eBPF支持)
- ARMv7 32-bit / AArch64 64-bit
- Clang 12+ (BPF程序编译)
- Go 1.22+
- root 或 CAP_BPF 权限

### 构建与运行

```bash
cd cloudflow-build/ebpf-probe
make all         # 编译全部BPF + Go二进制
sudo ./ebpf-probe # 启动探针
```

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `EDGE_ADDR` | Edge数据上报地址 | `localhost:9102` |
| `CLICKHOUSE_ADDR` | ClickHouse地址 | `localhost` |
| `PROBE_ID` | 探针ID | `hostname` |
| `INTERFACE` | 监听网口 | `eth0` |
| `LOG_LEVEL` | 日志级别 | `info` |

---

## STB 嵌入式部署

CloudFlow 已完整验证在 Android 机顶盒 (ARM32, Linux 5.4.210, 无BTF) 上的部署。

### 部署命令

```bash
# 1. ADB连接
adb connect <STB_IP>:60001

# 2. 推送探针
adb push stb-ebpf-probe-v3 /data/local/tmp/cloudflow/ebpf-probe/
adb push *.bpf.o /data/local/tmp/cloudflow/ebpf-probe/

# 3. 启动
adb shell "su 0 env EDGE_ADDR=<VM_IP>:8081 \
  CLICKHOUSE_ADDR=<VM_IP> \
  /data/local/tmp/cloudflow/ebpf-probe/stb-ebpf-probe-v3 &"
```

### 已验证能力

| 采集器 | 挂载方式 | 验证状态 |
|--------|----------|----------|
| 网络抓包 | AF_PACKET socket filter | ✅ 2500+ 事件/5分钟 |
| TCP连接跟踪 | kprobe/tcp_connect | ✅ 20 事件/5分钟 |
| 文件I/O | kprobe/do_filp_open + close | ✅ 320 事件/5分钟 |
| 进程执行 | kprobe/do_execve | ✅ 已附加 |
| On-CPU采样 | perf_event 99Hz | ✅ 304 采样/5分钟 |
| TCP重传 | kprobe/tcp_retransmit_skb | ✅ 已附加 |

---

## 监控栈

| 组件 | 端口 | 用途 |
|------|------|------|
| Grafana | 3001 | 可视化面板 |
| Prometheus | 9091 | 指标存储 |
| Alertmanager | 9094 | 告警管理 |
| Jaeger | 16686 | 分布式追踪 |

---

## API 文档

核心接口前缀 `/api/v1`，通过 Nginx 反向代理到各后端服务。

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/auth/login` | POST | 登录认证 |
| `/api/v1/dashboard/overview` | GET | 仪表盘总览 |
| `/api/v1/probes` | GET | 探针列表 |
| `/api/v1/network/flows` | GET | 网络流日志 |
| `/api/v1/protocol/http` | GET | HTTP协议日志 |
| `/api/v1/protocol/dns` | GET | DNS查询日志 |
| `/api/v1/performance/:host` | GET | 主机性能数据 |
| `/api/v1/security/events` | GET | 安全事件 |
| `/api/v1/network/topology` | GET | 网络拓扑数据 |

---

## 开发规范

- **提交规范**：遵循 Conventional Commits (`feat:`, `fix:`, `chore:`)
- **分支管理**：GitHub Flow
- **代码风格**：gofmt + goimports + golangci-lint
- **预提交检查**：`bash scripts/pre-commit.sh`
- **版本管理**：遵循 Semantic Versioning

---

## 许可证

MIT © 2026 CloudFlow Team
