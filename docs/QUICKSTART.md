# CloudFlow 快速入门指南

## 概述

CloudFlow 是一个完整的分布式流量分析和监控平台，包含以下核心组件：

- **业务监控前端** (Business Frontend)：负责监控业务流量、分析网络数据
- **平台自监控前端** (Platform Frontend)：负责监控 CloudFlow 平台自身的运行状态
- **微服务架构**：包含多个独立服务，支持水平扩展
- **完整的可观测性**：内置 Prometheus、Grafana、Loki、Jaeger 等工具

## 一键部署

### 前置要求

- Linux 服务器（推荐 Ubuntu 22.04 或 CentOS 7+）
- 4GB+ 可用内存
- 50GB+ 可用磁盘空间
- 能够访问互联网（用于下载 Docker 镜像）

### 部署步骤

#### 方式一：一键部署脚本（推荐）

```bash
# 直接运行一键部署脚本
curl -sSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/scripts/deploy.sh | bash
```

#### 方式二：手动部署

```bash
# 1. 克隆代码仓库
git clone https://github.com/meinanzilinzhengying/cloudflow.git
cd cloudflow

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env 文件，根据需要修改配置

# 3. 启动所有服务
docker compose up -d --build

# 4. 等待服务启动完成（约 5-10 分钟）
docker compose ps
```

## 访问指南

部署完成后，您可以通过以下地址访问各服务：

### 前端服务

| 服务 | 地址 | 说明 |
|------|------|------|
| 业务监控前端 | http://your-server-ip:8080 | 流量分析、服务拓扑、链路追踪、告警管理 |
| 平台自监控前端 | http://your-server-ip:3003 | 平台自监控、探针管理、AI 分析 |

### 可观测性工具

| 工具 | 地址 | 默认用户 | 默认密码 | 说明 |
|------|------|----------|----------|------|
| Grafana | http://your-server-ip:3001 | admin | 随机生成 | 监控仪表盘 |
| Prometheus | http://your-server-ip:9091 | - | - | 指标查询 |
| Jaeger | http://your-server-ip:16686 | - | - | 分布式追踪 |

### API 端点

| 服务 | 地址 | 说明 |
|------|------|------|
| Auth | http://your-server-ip:8006 | 认证服务 |
| Control | http://your-server-ip:8001 | 控制平面 |
| Query | http://your-server-ip:8007 | 查询服务 |

## 自监控架构

### 设计理念

CloudFlow 不需要单独安装探针来监控自身，因为它内置了完整的可观测性体系：

```
┌─────────────────────────────────────────────────────────┐
│                    自监控架构                             │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────┐     ┌─────────────────────────────┐  │
│  │   Platform   │     │      Grafana Dashboards     │  │
│  │   Frontend   │───▶ │ (自定义平台监控面板)        │  │
│  └──────────────┘     └─────────────────────────────┘  │
│         │                       │                       │
│         ▼                       ▼                       │
│  ┌──────────────────────────────────────────────┐     │
│  │            Prometheus + Alertmanager         │     │
│  │            (指标收集 + 告警)                  │     │
│  └──────────────────────────────────────────────┘     │
│         │                       │                       │
│         ▼                       ▼                       │
│  ┌──────────────┐      ┌──────────────────┐            │
│  │     Loki     │      │      Jaeger      │            │
│  │  (日志收集)  │      │  (分布式追踪)    │            │
│  └──────────────┘      └──────────────────┘            │
│         │                       │                       │
│         └──────────┬────────────┘                       │
│                    │                                    │
│                    ▼                                    │
│  ┌───────────────────────────────────────────────┐     │
│  │     CloudFlow 微服务和基础设施层               │     │
│  └───────────────────────────────────────────────┘     │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### 自监控功能

1. **平台概览**：监控所有服务的运行状态、CPU/内存使用
2. **健康检查**：定期检查所有服务的健康状态
3. **日志查询**：通过 Loki 查询和分析所有服务的日志
4. **告警管理**：配置和管理平台自身的告警规则
5. **进程监控**：监控关键进程的运行状态

## 探针部署

### 通过 SSH 远程安装（推荐）

1. 访问平台自监控前端：http://your-server-ip:3003
2. 导航到"探针管理"页面
3. 点击"SSH 安装"按钮
4. 填写目标服务器信息：
   - 主机 IP
   - SSH 端口（默认 22）
   - 用户名
   - 认证方式（密码或 SSH 密钥）
5. 配置探针参数
6. 点击"开始安装"

### 手动安装

参见 [cloud-flow-agent 部署文档](./agent-deployment.md)

## 常见问题

### 服务无法启动

```bash
# 查看服务状态
docker compose ps

# 查看服务日志
docker compose logs -f [service-name]

# 重启服务
docker compose restart [service-name]
```

### 端口冲突

编辑 `docker-compose.yml` 文件，修改相关端口映射。

### Grafana 无法访问

```bash
# 检查 Grafana 容器状态
docker compose ps grafana

# 查看 Grafana 日志
docker compose logs -f grafana
```

## 管理命令

```bash
# 进入部署目录
cd /opt/cloudflow

# 查看所有服务状态
docker compose ps

# 查看服务日志
docker compose logs -f

# 停止所有服务
docker compose down

# 停止服务并清理数据卷（慎用）
docker compose down -v

# 重启服务
docker compose restart

# 更新服务
docker compose pull
docker compose up -d
```

## 下一步

1. 访问平台自监控前端，了解平台运行状态
2. 通过 SSH 安装功能部署一个或多个探针
3. 查看业务监控前端，开始分析流量数据
4. 配置 Grafana 仪表板，定制您的监控视图

## 技术支持

- 报告问题：https://github.com/meinanzilinzhengying/cloudflow/issues
- 完整文档：./README.md
