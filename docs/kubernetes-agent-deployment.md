# CloudFlow 探针Kubernetes部署指南

## 概述

CloudFlow探针支持三种部署方式：

| 部署方式 | 适用场景 | 部署位置 | 权限要求 |
|---------|---------|---------|---------|
| **ECS部署** | 传统云服务器监控 | 直接在ECS上安装 | SSH/root权限 |
| **Node节点部署** | Kubernetes节点监控 | 在K8s节点上安装 | SSH/root权限 |
| **Pod节点部署** | 容器集群全栈监控 | 作为DaemonSet部署 | K8s ServiceAccount + RBAC权限 |

---

## 第三种方式：作为Pod节点部署（推荐）

### 核心问题回答

**问题：是否需要通过token读取容器集群的所有配置信息？**

**答案：需要，但不是"所有配置"，而是通过Kubernetes ServiceAccount + RBAC权限体系来获取必要的信息。**

### 权限设计原则

我们采用"最小权限原则"（Least Privilege Principle）：

| 权限 | 用途 | 是否必需 |
|------|------|---------|
| `pods/list/watch` | 发现Pod元数据、关联流量 | 🔴 必需 |
| `pods/get` | 获取Pod详情 | 🔴 必需 |
| `services/list/watch` | 发现服务元数据 | 🟡 推荐 |
| `services/get` | 获取服务详情 | 🟡 推荐 |
| `endpoints/list/watch` | 发现端点 | 🟡 推荐 |
| `nodes/list` | 发现节点信息 | 🟡 推荐 |
| `nodes/get` | 获取节点详情 | 🟡 推荐 |
| `namespace/list` | 发现命名空间 | 🟢 可选 |
| `pods/log` | 读取Pod日志 | 🟢 可选 |
| `secrets/*` | 获取密钥 | ❌ 不需要 |
| `configmaps/*` | 获取配置 | ❌ 不需要 |

---

## 完整部署方案

### 1. 创建Namespace

```yaml
# 00-namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: cloudflow
  labels:
    name: cloudflow
```

### 2. 创建ServiceAccount

```yaml
# 01-serviceaccount.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: cloudflow-agent
  namespace: cloudflow
  labels:
    app: cloudflow-agent
---
# 自动创建Secret（K8s 1.24+需要手动创建）
apiVersion: v1
kind: Secret
metadata:
  name: cloudflow-agent-token
  namespace: cloudflow
  annotations:
    kubernetes.io/service-account.name: cloudflow-agent
type: kubernetes.io/service-account-token
```

### 3. 创建RBAC权限

```yaml
# 02-rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cloudflow-agent
rules:
  # 核心权限 - 必需
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  
  # 推荐权限 - 增强功能
  - apiGroups: [""]
    resources: ["services", "endpoints", "nodes", "namespaces"]
    verbs: ["get", "list", "watch"]
  
  # 可选权限 - 日志关联
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]

---
# 绑定ClusterRole到ServiceAccount
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: cloudflow-agent
subjects:
  - kind: ServiceAccount
    name: cloudflow-agent
    namespace: cloudflow
roleRef:
  kind: ClusterRole
  name: cloudflow-agent
  apiGroup: rbac.authorization.k8s.io
```

### 4. 创建ConfigMap（配置文件）

```yaml
# 03-configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cloudflow-agent-config
  namespace: cloudflow
data:
  config.yaml: |
    # 探针配置
    probe_id: ""                  # 留空自动生成
    edge_addr: "cloudflow-edge.cloudflow.svc.cluster.local:50051"
    metrics_port: "9090"
    health_port: "8080"
    collect_interval: 10
    batch_size: 10
    
    # K8s集成配置
    kubernetes:
      enabled: true
      kubeconfig_path: ""         # 留空使用ServiceAccount
      kube_namespace: ""          # 留空监控所有命名空间
      sync_interval: "30s"
      label_selector: ""
      field_selector: ""
      include_namespaces: []      # 留空包含所有
      exclude_namespaces: ["kube-system", "kube-public"]
      include_labels: {}
      exclude_labels: {}
    
    # API Key - 建议通过Secret挂载
    api_key: ""
    
    # eBPF配置
    ebpf:
      enabled: true
      tcp_metrics:
        enabled: true
      http_metrics:
        enabled: true
      base_traffic:
        enabled: true
    
    # 资源限制
    ebpf.resource_limit:
      enabled: true
      max_cpu_core: 1.0
      max_memory_mb: 1024
    
    # 自监控
    ebpf.self_monitor:
      enabled: true
```

### 5. 创建Secret（敏感配置）

```yaml
# 04-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflow-agent-secrets
  namespace: cloudflow
type: Opaque
data:
  # 使用 base64 编码的值
  api-key: YXBpa2V5X2Zvcl9jbG91ZGZsb3c=  # 替换为真实值的base64
---
# 或者使用Docker Secret（更安全）
# 先创建Secret文件
```

### 6. 创建DaemonSet（核心部署）

```yaml
# 05-daemonset.yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: cloudflow-agent
  namespace: cloudflow
  labels:
    app: cloudflow-agent
    component: agent
spec:
  selector:
    matchLabels:
      app: cloudflow-agent
  template:
    metadata:
      labels:
        app: cloudflow-agent
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: cloudflow-agent
      priorityClassName: system-node-critical  # 高优先级
      tolerations:
        - key: node-role.kubernetes.io/master
          effect: NoSchedule
        - key: node-role.kubernetes.io/control-plane
          effect: NoSchedule
        - key: node.cloud.google.com/gke-spot
          operator: Exists
          effect: NoSchedule
      hostNetwork: true                    # 必需：访问宿主机网络
      hostPID: true                        # 可选：增强进程发现
      dnsPolicy: ClusterFirstWithHostNet
      containers:
        - name: agent
          image: registry.cloudflow.io/cloudflow-agent:latest
          imagePullPolicy: IfNotPresent
          
          # 必需：eBPF需要特权模式
          securityContext:
            privileged: true
            capabilities:
              add:
                - SYS_ADMIN
                - NET_ADMIN
                - SYS_RESOURCE
                - IPC_LOCK
            readOnlyRootFilesystem: false
          
          # 资源限制
          resources:
            requests:
              cpu: "100m"
              memory: "256Mi"
              hugepages-2Mi: "128Mi"
            limits:
              cpu: "2"
              memory: "2Gi"
              hugepages-2Mi: "128Mi"
          
          # 环境变量
          env:
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: POD_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.podIP
            - name: EDGE_ADDR
              value: "cloudflow-edge.cloudflow.svc.cluster.local:50051"
            - name: CLOUD_FLOW_API_KEY
              valueFrom:
                secretKeyRef:
                  name: cloudflow-agent-secrets
                  key: api-key
          
          # 端口暴露
          ports:
            - name: metrics
              containerPort: 9090
              protocol: TCP
            - name: health
              containerPort: 8080
              protocol: TCP
          
          # 健康检查
          livenessProbe:
            httpGet:
              path: /healthz
              port: health
            initialDelaySeconds: 30
            periodSeconds: 30
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /readyz
              port: health
            initialDelaySeconds: 10
            periodSeconds: 10
          
          # 数据卷挂载
          volumeMounts:
            # 必需：eBPF相关
            - name: sys
              mountPath: /sys
            - name: proc
              mountPath: /proc
            - name: bpffs
              mountPath: /sys/fs/bpf
            - name: debug
              mountPath: /sys/kernel/debug
            
            # 配置文件
            - name: config
              mountPath: /etc/cloudflow-agent
              readOnly: true
            
            # 大页内存
            - name: hugepages
              mountPath: /dev/hugepages
            
            # 本地存储（可选）
            - name: agent-data
              mountPath: /var/lib/cloudflow-agent
            
            # 日志（可选）
            - name: log
              mountPath: /var/log/cloudflow-agent
      
      # 数据卷定义
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
        - name: debug
          hostPath:
            path: /sys/kernel/debug
        - name: hugepages
          hostPath:
            path: /dev/hugepages
        - name: config
          configMap:
            name: cloudflow-agent-config
            items:
              - key: config.yaml
                path: config.yaml
        - name: agent-data
          hostPath:
            path: /var/lib/cloudflow-agent
            type: DirectoryOrCreate
        - name: log
          hostPath:
            path: /var/log/cloudflow-agent
            type: DirectoryOrCreate
```

---

## 一键安装脚本

```bash
#!/bin/bash
# deploy-cloudflow-agent.sh

set -e

echo "🚀 CloudFlow Agent K8s部署开始..."

# 1. 创建Namespace
echo "📦 创建命名空间..."
kubectl apply -f 00-namespace.yaml

# 2. 创建ServiceAccount
echo "🔐 创建ServiceAccount..."
kubectl apply -f 01-serviceaccount.yaml

# 3. 配置RBAC
echo "🔑 配置RBAC权限..."
kubectl apply -f 02-rbac.yaml

# 4. 创建ConfigMap
echo "⚙️ 配置ConfigMap..."
kubectl apply -f 03-configmap.yaml

# 5. 创建Secret（交互式）
echo "🔒 配置Secret..."
read -p "请输入CloudFlow API Key: " api_key
echo "${api_key}" | base64 > temp.txt
kubectl create secret generic cloudflow-agent-secrets -n cloudflow \
  --from-file=api-key=temp.txt || true
rm temp.txt

# 6. 部署DaemonSet
echo "🚢 部署DaemonSet..."
kubectl apply -f 05-daemonset.yaml

# 7. 等待Pod启动
echo "⏳ 等待Pod启动..."
kubectl -n cloudflow rollout status daemonset/cloudflow-agent --timeout=300s

# 8. 检查状态
echo "✅ 检查部署状态..."
kubectl -n cloudflow get pods -o wide -l app=cloudflow-agent
kubectl -n cloudflow get daemonset cloudflow-agent

echo "🎉 CloudFlow Agent部署完成！"
echo ""
echo "📊 查看日志: kubectl -n cloudflow logs -f daemonset/cloudflow-agent"
echo "📈 查看监控: kubectl -n cloudflow port-forward svc/... 3000:3000"
```

---

## 三种部署方式对比

### 方式1：ECS部署

```bash
# 在ECS上直接执行
curl -sSL https://install.cloudflow.io/agent.sh | bash -s \
  --edge-addr=edge.example.com:50051 \
  --api-key=your-api-key
```

**优点：**
- 简单直接，不依赖K8s
- 适合混合部署

**缺点：**
- 需要SSH访问
- 需要手动管理

---

### 方式2：Node节点部署

```bash
# 在每个K8s节点上执行
curl -sSL https://install.cloudflow.io/agent.sh | bash -s \
  --edge-addr=edge.cloudflow.svc:50051 \
  --api-key=your-api-key \
  --k8s-namespace=cloudflow
```

**优点：**
- 独立于K8s控制面
- 适用于K8s但不希望作为Pod运行

**缺点：**
- 节点级部署
- 无法利用K8s编排能力

---

### 方式3：Pod节点部署（推荐）

```yaml
# 如上面的DaemonSet配置
```

**优点：**
- ✅ 自动化部署、自愈、滚动升级
- ✅ 利用K8s原生能力（Service、RBAC等）
- ✅ 可观测性集成完善
- ✅ 弹性伸缩方便

**缺点：**
- 需要一定的K8s知识
- 资源隔离（可通过配置调整）

---

## 安全最佳实践

### 1. 限制命名空间

```yaml
# 只监控特定命名空间
apiVersion: v1
kind: ConfigMap
metadata:
  name: cloudflow-agent-config
data:
  config.yaml: |
    kubernetes:
      exclude_namespaces: ["kube-system", "kube-public", "kube-node-lease"]
      include_namespaces: ["production", "staging"]
```

### 2. 使用网络策略

```yaml
# 06-networkpolicy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: cloudflow-agent
  namespace: cloudflow
spec:
  podSelector:
    matchLabels:
      app: cloudflow-agent
  policyTypes:
    - Ingress
    - Egress
  egress:
    # 允许访问Edge服务
    - to:
        - podSelector:
            matchLabels:
              app: cloudflow-edge
      ports:
        - protocol: TCP
          port: 50051
    # 允许访问API Server
    - to:
        - podSelector:
            matchLabels:
              component: kube-apiserver
      ports:
        - protocol: TCP
          port: 6443
```

### 3. 使用Pod安全策略

```yaml
# 07-psp.yaml
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: cloudflow-agent
spec:
  privileged: true
  hostNetwork: true
  hostPID: true
  volumes:
    - hostPath
    - configMap
    - secret
    - emptyDir
  allowedHostPaths:
    - pathPrefix: /sys
    - pathPrefix: /proc
    - pathPrefix: /sys/fs/bpf
    - pathPrefix: /sys/kernel/debug
  allowedCapabilities:
    - SYS_ADMIN
    - NET_ADMIN
    - SYS_RESOURCE
    - IPC_LOCK
  allowPrivilegeEscalation: true
```

---

## 验证部署

```bash
# 1. 检查Pod状态
kubectl -n cloudflow get pods -o wide -l app=cloudflow-agent

# 2. 检查日志
kubectl -n cloudflow logs -f daemonset/cloudflow-agent

# 3. 检查RBAC是否生效
kubectl -n cloudflow auth can-i get pods --as=system:serviceaccount:cloudflow:cloudflow-agent

# 4. 检查K8s发现功能
kubectl -n cloudflow exec -it deploy/cloudflow-agent -- ls -la /var/run/secrets/kubernetes.io/serviceaccount
```

---

## 卸载

```bash
# 完全卸载
kubectl delete -f 05-daemonset.yaml
kubectl delete -f 04-secret.yaml
kubectl delete -f 03-configmap.yaml
kubectl delete -f 02-rbac.yaml
kubectl delete -f 01-serviceaccount.yaml
kubectl delete -f 00-namespace.yaml

# 或使用一行命令
kubectl delete namespace cloudflow --wait=true
```

---

## 附录：完整文件结构

```
./kubernetes/
├── 00-namespace.yaml
├── 01-serviceaccount.yaml
├── 02-rbac.yaml
├── 03-configmap.yaml
├── 04-secret.yaml
├── 05-daemonset.yaml
├── 06-networkpolicy.yaml
├── 07-psp.yaml
└── deploy-cloudflow-agent.sh
```

---

**文档版本**: v1.0  
**最后更新**: 2024-01-15  
**适用版本**: CloudFlow v1.0+
