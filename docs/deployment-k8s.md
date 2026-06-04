# CloudFlow K8s 部署指南

## 概述

CloudFlow 是一个云原生网络流量分析平台，采用 Agent → Edge → Center 三层架构设计。本指南详细描述如何在 Kubernetes 集群上部署 CloudFlow。

---

## 目录

1. [环境要求](#1-环境要求)
2. [部署前准备](#2-部署前准备)
3. [部署架构](#3-部署架构)
4. [Helm 部署](#4-helm-部署)
5. [手动部署](#5-手动部署)
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