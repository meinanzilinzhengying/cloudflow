# CloudFlow Helm Chart

> P23: Kubernetes 部署 Helm Chart，支持版本管理和参数配置

## 概述

本 Helm Chart 用于在 Kubernetes 集群中部署 CloudFlow 云原生可观测平台。

## 组件

- **Center**: 中心服务（API + gRPC）
- **Edge**: 边缘节点（数据转发）
- **Agent**: 探针（DaemonSet，eBPF 采集）
- **Frontend**: 前端（可选）

## 快速安装

```bash
helm install cloudflow ./helm/cloudflow \
  --namespace cloudflow \
  --create-namespace
```

## 生产环境配置

```yaml
# values-production.yaml
center:
  replicaCount: 3
  autoscaling:
    enabled: true
    minReplicas: 3
    maxReplicas: 10

edge:
  replicaCount: 5
  autoscaling:
    enabled: true
    minReplicas: 5
    maxReplicas: 50
  persistence:
    enabled: true
    storageClass: "ssd"
    size: 50Gi

secrets:
  jwtSecret: "your-32-char-secret-key-here!"
```

## 升级

```bash
helm upgrade cloudflow ./helm/cloudflow -f values-production.yaml --namespace cloudflow
```

## 卸载

```bash
helm uninstall cloudflow --namespace cloudflow
```

## 许可证

Apache License 2.0
