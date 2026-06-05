# CloudFlow 部署指南

## 概述

CloudFlow 是一个云原生网络流量分析平台，采用 Agent → Edge → Center 三层架构设计。本指南详细描述如何部署 CloudFlow。

---

## 快速开始（一键部署）

### 方式1：Docker Compose 一键部署（推荐用于测试环境）

```bash
# 一键部署脚本（Linux 服务器）
curl -sSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/scripts/deploy.sh | bash
```

**脚本功能**：
- 自动安装 Docker 和 Docker Compose
- 克隆代码仓库
- 配置环境变量（自动生成随机密码）
- 启动所有服务
- 输出访问地址和凭证

### 方式2：手动 Docker Compose 部署

```bash
# 克隆代码
git clone https://github.com/meinanzilinzhengying/cloudflow.git
cd cloudflow

# 配置环境变量
cp .env.example .env
# 编辑 .env 设置数据库密码等配置

# 启动服务
docker compose up -d

# 查看服务状态
docker compose ps
```

### 方式3：Kubernetes 部署（生产环境）

```bash
# 使用 Helm 部署
helm repo add cloudflow https://meinanzilinzhengying.github.io/cloudflow
helm install cloudflow cloudflow/cloudflow --namespace cloudflow --create-namespace
```

---

## 目录

1. [环境要求](#1-环境要求)
2. [部署架构](#2-部署架构)
3. [Docker Compose 部署](#3-docker-compose-部署)
4. [Kubernetes Helm 部署](#4-kubernetes-helm-部署)
5. [Kubernetes 手动部署](#5-kubernetes-手动部署)
6. [配置说明](#6-配置说明)
7. [验证部署](#7-验证部署)
8. [升级与回滚](#8-升级与回滚)
9. [卸载](#9-卸载)

---

## 1. 环境要求

| 组件 | 版本要求 | 说明 |
|------|----------|------|
| Kubernetes | >= 1.22 | 推荐使用 1.24+ |
| Helm | >= 3.8 | 包管理工具 |
| Docker | >= 20.10 | 容器运行时 |
| ClickHouse | >= 22.3 | 时序数据库（可选） |
| Redis | >= 6.0 | 缓存服务（可选） |

---

## 2. 部署前准备

### 2.1 创建命名空间

```bash
kubectl create namespace cloudflow
```

### 2.2 创建 Secret（敏感配置）

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflow-secrets
  namespace: cloudflow
type: Opaque
data:
  jwt-secret: <base64-encoded-jwt-secret>
  api-key: <base64-encoded-api-key>
```

### 2.3 存储类配置

确保集群中有可用的存储类用于持久化数据：

```bash
kubectl get storageclass
```

---

## 3. 部署架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                       │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐      │
│  │  cloudflow-  │    │  cloudflow-  │    │  cloudflow-  │      │
│  │    agent     │───▶│    edge      │───▶│    center    │      │
│  │  (DaemonSet)│    │  (Deployment)│    │  (Deployment)│      │
│  └──────────────┘    └──────────────┘    └──────────────┘      │
│         │                   │                   │               │
│         ▼                   ▼                   ▼               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐      │
│  │   eBPF       │    │  gRPC        │    │  REST API    │      │
│  │  流量采集    │    │   数据处理   │    │   可视化     │      │
│  └──────────────┘    └──────────────┘    └──────────────┘      │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────────────┐      │
│  │                    存储层                            │      │
│  │  ClickHouse  │  Redis  │  VictoriaMetrics  │  Loki  │      │
│  └──────────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────────┘
```

---

## 4. Helm 部署

### 4.1 添加 Helm Chart 仓库

```bash
helm repo add cloudflow https://charts.cloudflow.io
helm repo update
```

### 4.2 安装 CloudFlow

```bash
helm install cloudflow cloudflow/cloudflow \
  --namespace cloudflow \
  --set clickhouse.enabled=true \
  --set redis.enabled=true \
  --set victoriaMetrics.enabled=true \
  --set loki.enabled=true \
  --set center.service.type=LoadBalancer
```

### 4.3 自定义配置

创建 `values.yaml` 文件：

```yaml
# values.yaml

global:
  namespace: cloudflow
  imageRegistry: registry.cloudflow.io
  imageTag: latest

center:
  replicaCount: 3
  resources:
    requests:
      cpu: "100m"
      memory: "256Mi"
    limits:
      cpu: "500m"
      memory: "1Gi"
  service:
    type: ClusterIP

edge:
  replicaCount: 2
  resources:
    requests:
      cpu: "200m"
      memory: "512Mi"
    limits:
      cpu: "1"
      memory: "2Gi"

agent:
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "200m"
      memory: "256Mi"
  privileged: true  # eBPF 需要特权模式

clickhouse:
  enabled: true
  persistence:
    size: 50Gi

redis:
  enabled: true

victoriaMetrics:
  enabled: true

loki:
  enabled: true
```

然后安装：

```bash
helm install cloudflow cloudflow/cloudflow \
  --namespace cloudflow \
  -f values.yaml
```

---

## 5. 手动部署

### 5.1 部署 Center 服务

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cloudflow-center
  namespace: cloudflow
  labels:
    app: cloudflow-center
spec:
  replicas: 3
  selector:
    matchLabels:
      app: cloudflow-center
  template:
    metadata:
      labels:
        app: cloudflow-center
    spec:
      containers:
      - name: center
        image: registry.cloudflow.io/cloudflow-center:latest
        ports:
        - containerPort: 8080
        - containerPort: 9001
        env:
        - name: CLOUDFLOW_CENTER_API_KEY
          valueFrom:
            secretKeyRef:
              name: cloudflow-secrets
              key: api-key
        - name: CLOUDFLOW_JWT_SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: cloudflow-secrets
              key: jwt-secret
        - name: CLICKHOUSE_ADDR
          value: "clickhouse.cloudflow.svc.cluster.local:9000"
        - name: REDIS_ADDR
          value: "redis.cloudflow.svc.cluster.local:6379"
        resources:
          requests:
            cpu: "100m"
            memory: "256Mi"
          limits:
            cpu: "500m"
            memory: "1Gi"
```

### 5.2 部署 Edge 服务

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cloudflow-edge
  namespace: cloudflow
  labels:
    app: cloudflow-edge
spec:
  replicas: 2
  selector:
    matchLabels:
      app: cloudflow-edge
  template:
    metadata:
      labels:
        app: cloudflow-edge
    spec:
      containers:
      - name: edge
        image: registry.cloudflow.io/cloudflow-edge:latest
        ports:
        - containerPort: 9002
        env:
        - name: CENTER_ADDR
          value: "cloudflow-center.cloudflow.svc.cluster.local:9001"
        resources:
          requests:
            cpu: "200m"
            memory: "512Mi"
          limits:
            cpu: "1"
            memory: "2Gi"
```

### 5.3 部署 Agent（DaemonSet）

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: cloudflow-agent
  namespace: cloudflow
  labels:
    app: cloudflow-agent
spec:
  selector:
    matchLabels:
      app: cloudflow-agent
  template:
    metadata:
      labels:
        app: cloudflow-agent
    spec:
      containers:
      - name: agent
        image: registry.cloudflow.io/cloudflow-agent:latest
        ports:
        - containerPort: 9003
        env:
        - name: EDGE_ADDR
          value: "cloudflow-edge.cloudflow.svc.cluster.local:9002"
        securityContext:
          privileged: true
          capabilities:
            add:
            - SYS_ADMIN
            - NET_ADMIN
        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
          limits:
            cpu: "200m"
            memory: "256Mi"
        volumeMounts:
        - name: sys
          mountPath: /sys
        - name: proc
          mountPath: /proc
        - name: bpffs
          mountPath: /sys/fs/bpf
      volumes:
      - name: sys
        hostPath:
          path: /sys
      - name: proc
        hostPath:
          path: /proc
      - name: bpffs
        hostPath:
          path: /sys/fs/bpf
```

### 5.4 部署 Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: cloudflow-center
  namespace: cloudflow
spec:
  type: ClusterIP
  selector:
    app: cloudflow-center
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: grpc
    port: 9001
    targetPort: 9001

---

apiVersion: v1
kind: Service
metadata:
  name: cloudflow-edge
  namespace: cloudflow
spec:
  type: ClusterIP
  selector:
    app: cloudflow-edge
  ports:
  - name: grpc
    port: 9002
    targetPort: 9002
```

---

## 6. 配置说明

### 6.1 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `CLOUDFLOW_CENTER_API_KEY` | API 访问密钥 | - |
| `CLOUDFLOW_JWT_SECRET_KEY` | JWT 签名密钥 | - |
| `CLICKHOUSE_ADDR` | ClickHouse 地址 | `localhost:9000` |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `CENTER_ADDR` | Center 服务地址 | - |
| `EDGE_ADDR` | Edge 服务地址 | - |
| `LOG_LEVEL` | 日志级别 | `info` |
| `DATA_RETENTION_DAYS` | 数据保留天数 | `7` |

### 6.2 配置文件

Center 服务配置文件 `config.yaml`：

```yaml
server:
  http_port: 8080
  grpc_port: 9001

security:
  api_key: ${CLOUDFLOW_CENTER_API_KEY}
  jwt_secret: ${CLOUDFLOW_JWT_SECRET_KEY}
  token_duration_hours: 24

storage:
  clickhouse:
    addr: ${CLICKHOUSE_ADDR}
    database: cloudflow
    username: default
    password: ""
  
  redis:
    addr: ${REDIS_ADDR}
    password: ""
    db: 0

alerting:
  enabled: true
  check_interval_seconds: 10
  notify_channels:
    - email
    - webhook

rate_limit:
  enabled: true
  login:
    bucket_size: 10
    refill_rate: 1
  query:
    bucket_size: 100
    refill_rate: 10
  api:
    bucket_size: 500
    refill_rate: 50
```

---

## 7. 验证部署

### 7.1 检查 Pod 状态

```bash
kubectl get pods -n cloudflow
```

预期输出：

```
NAME                                 READY   STATUS    RESTARTS   AGE
cloudflow-center-7f9d98c6d9-2xqkd   1/1     Running   0          5m
cloudflow-center-7f9d98c6d9-5j7kl   1/1     Running   0          5m
cloudflow-center-7f9d98c6d9-8z4mn   1/1     Running   0          5m
cloudflow-edge-5d78c9d76b-4d8nf     1/1     Running   0          5m
cloudflow-edge-5d78c9d76b-9k2fp     1/1     Running   0          5m
cloudflow-agent-h2x9m                1/1     Running   0          5m
cloudflow-agent-k7v5n                1/1     Running   0          5m
cloudflow-agent-m8t4q                1/1     Running   0          5m
```

### 7.2 检查服务状态

```bash
kubectl get services -n cloudflow
```

### 7.3 健康检查

```bash
# 检查 Center 服务
curl http://cloudflow-center.cloudflow.svc.cluster.local/api/healthz

# 检查 Edge 服务（gRPC）
grpcurl -plaintext cloudflow-edge.cloudflow.svc.cluster.local:9002 grpc.health.v1.Health/Check
```

---

## 8. 升级与回滚

### 8.1 升级

```bash
helm upgrade cloudflow cloudflow/cloudflow \
  --namespace cloudflow \
  --set imageTag=v1.1.0
```

### 8.2 回滚

```bash
helm rollback cloudflow <revision> -n cloudflow
```

查看历史版本：

```bash
helm history cloudflow -n cloudflow
```

---

## 9. 卸载

```bash
helm uninstall cloudflow -n cloudflow
kubectl delete namespace cloudflow
```

---

## 附录：资源配置建议

| 环境 | Center | Edge | Agent |
|------|--------|------|-------|
| 开发 | 1 CPU / 512MB | 1 CPU / 512MB | 0.5 CPU / 256MB |
| 测试 | 2 CPU / 1GB | 2 CPU / 2GB | 1 CPU / 512MB |
| 生产 | 4 CPU / 4GB | 8 CPU / 8GB | 2 CPU / 1GB |

---

**文档版本**: v1.0  
**最后更新**: 2024-01-15  
**适用版本**: CloudFlow v1.0+