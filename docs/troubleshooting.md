# CloudFlow 故障排查手册

## 概述

本手册提供 CloudFlow 平台常见问题的排查方法和解决方案，帮助运维团队快速定位和解决问题。

---

## 目录

1. [快速诊断](#1-快速诊断)
2. [Agent 问题](#2-agent-问题)
3. [Edge 问题](#3-edge-问题)
4. [Center 问题](#4-center-问题)
5. [存储问题](#5-存储问题)
6. [网络问题](#6-网络问题)
7. [eBPF 问题](#7-ebpf-问题)
8. [监控告警](#8-监控告警)
9. [日志分析](#9-日志分析)
10. [常见错误码](#10-常见错误码)

---

## 1. 快速诊断

### 1.1 健康检查命令

```bash
# 检查所有组件状态
kubectl get pods -n cloudflow

# 检查服务状态
kubectl get services -n cloudflow

# 检查节点状态
kubectl get nodes

# 查看事件
kubectl get events -n cloudflow --sort-by='.lastTimestamp'

# 检查 Center API
curl http://<center-ip>:8080/api/healthz

# 检查 Edge gRPC 服务
grpcurl -plaintext <edge-ip>:9002 grpc.health.v1.Health/Check
```

### 1.2 诊断流程图

```
用户报告问题
     │
     ▼
检查 Pod 状态 ───▶ Running?
     │
     ├─ Yes ──▶ 检查服务日志
     │
     └─ No ──▶ 检查事件 ──▶ 分析错误原因
                    │
                    ▼
              检查资源配置
                    │
                    ▼
              检查网络连通性
```

---

## 2. Agent 问题

### 2.1 Agent Pod 无法启动

**现象**: Agent Pod 状态为 `Pending` 或 `CrashLoopBackOff`

**排查步骤**:

```bash
# 查看 Pod 详情
kubectl describe pod <agent-pod-name> -n cloudflow

# 查看日志
kubectl logs <agent-pod-name> -n cloudflow

# 检查节点资源
kubectl describe node <node-name>
```

**常见原因**:

| 原因 | 解决方案 |
|------|----------|
| 节点资源不足 | 扩容节点或调整资源配额 |
| 权限不足（特权模式） | 确认 Pod 配置了 `privileged: true` |
| BPF 文件系统未挂载 | 在节点上执行 `mount -t bpf bpf /sys/fs/bpf` |
| SELinux 阻止 | 临时禁用或配置 SELinux 规则 |

### 2.2 Agent 无法连接到 Edge

**现象**: Agent 日志显示连接失败

**排查步骤**:

```bash
# 检查 Edge 服务是否正常
kubectl get svc cloudflow-edge -n cloudflow

# 在 Agent Pod 内测试连接
kubectl exec -it <agent-pod-name> -n cloudflow -- ping cloudflow-edge.cloudflow.svc.cluster.local

# 检查网络策略
kubectl get networkpolicy -n cloudflow
```

**解决方案**:

```yaml
# 确保 Edge 服务已正确配置
apiVersion: v1
kind: Service
metadata:
  name: cloudflow-edge
spec:
  type: ClusterIP
  selector:
    app: cloudflow-edge
  ports:
  - name: grpc
    port: 9002
    targetPort: 9002
```

### 2.3 eBPF 程序加载失败

**现象**: Agent 日志显示 `failed to load eBPF program`

**排查步骤**:

```bash
# 检查内核版本（需要 4.15+）
uname -r

# 检查 BPF 相关内核模块
lsmod | grep bpf

# 检查系统配置
sysctl kernel.unprivileged_bpf_disabled
```

**解决方案**:

```bash
# 启用 BPF（需要 root 权限）
sysctl -w kernel.unprivileged_bpf_disabled=0

# 永久生效（添加到 /etc/sysctl.conf）
echo "kernel.unprivileged_bpf_disabled=0" >> /etc/sysctl.conf
```

---

## 3. Edge 问题

### 3.1 Edge 无法接收数据

**现象**: Edge Pod 运行正常，但没有数据流入

**排查步骤**:

```bash
# 查看 Edge 日志
kubectl logs <edge-pod-name> -n cloudflow -f

# 检查 gRPC 连接状态
grpcurl -plaintext <edge-ip>:9002 list

# 检查与 Center 的连接
kubectl exec -it <edge-pod-name> -n cloudflow -- ping cloudflow-center.cloudflow.svc.cluster.local
```

### 3.2 Edge 内存占用过高

**现象**: Edge Pod 内存使用率超过 80%

**排查步骤**:

```bash
# 查看资源使用情况
kubectl top pod <edge-pod-name> -n cloudflow

# 检查流量峰值
kubectl exec -it <edge-pod-name> -n cloudflow -- curl -s http://localhost:9090/metrics | grep flow_rate
```

**解决方案**:

```yaml
# 调整资源限制
spec:
  containers:
  - name: edge
    resources:
      requests:
        cpu: "500m"
        memory: "1Gi"
      limits:
        cpu: "2"
        memory: "4Gi"
```

---

## 4. Center 问题

### 4.1 Center API 无响应

**现象**: 访问 API 返回超时或 503 错误

**排查步骤**:

```bash
# 检查 Pod 状态
kubectl get pods -n cloudflow -l app=cloudflow-center

# 检查服务端点
kubectl get endpoints cloudflow-center -n cloudflow

# 检查健康检查
curl http://<center-ip>:8080/api/healthz
```

### 4.2 认证失败

**现象**: API 返回 401 Unauthorized

**排查步骤**:

```bash
# 检查 JWT Secret 是否正确配置
kubectl get secret cloudflow-secrets -n cloudflow -o yaml

# 检查环境变量
kubectl exec -it <center-pod-name> -n cloudflow -- env | grep JWT
```

**解决方案**:

```bash
# 重新创建 Secret
kubectl delete secret cloudflow-secrets -n cloudflow
kubectl create secret generic cloudflow-secrets -n cloudflow \
  --from-literal=jwt-secret=<your-secret> \
  --from-literal=api-key=<your-api-key>
```

### 4.3 数据库连接失败

**现象**: Center 日志显示 ClickHouse/Redis 连接失败

**排查步骤**:

```bash
# 检查数据库服务
kubectl get svc -n cloudflow | grep -E "clickhouse|redis"

# 测试数据库连接
kubectl exec -it <center-pod-name> -n cloudflow -- clickhouse-client --host clickhouse.cloudflow.svc.cluster.local

kubectl exec -it <center-pod-name> -n cloudflow -- redis-cli -h redis.cloudflow.svc.cluster.local ping
```

---

## 5. 存储问题

### 5.1 ClickHouse 查询缓慢

**现象**: 查询响应时间超过 10 秒

**排查步骤**:

```bash
# 检查 ClickHouse 状态
kubectl exec -it <clickhouse-pod-name> -n cloudflow -- clickhouse-client -q "SELECT * FROM system.metrics"

# 检查磁盘 I/O
kubectl exec -it <clickhouse-pod-name> -n cloudflow -- iostat -x 1 5

# 检查内存使用
kubectl exec -it <clickhouse-pod-name> -n cloudflow -- free -h
```

**优化建议**:

```sql
-- 创建索引
CREATE INDEX idx_time ON flow_logs(time);
CREATE INDEX idx_src_ip ON flow_logs(src_ip);
CREATE INDEX idx_dst_ip ON flow_logs(dst_ip);

-- 分区表优化
ALTER TABLE flow_logs ADD PARTITION BY toYYYYMM(time);
```

### 5.2 Redis 缓存命中率低

**现象**: Redis 频繁写入但读取效率低

**排查步骤**:

```bash
# 查看 Redis 统计信息
kubectl exec -it <redis-pod-name> -n cloudflow -- redis-cli INFO stats

# 查看内存使用
kubectl exec -it <redis-pod-name> -n cloudflow -- redis-cli INFO memory
```

**优化建议**:

```bash
# 调整缓存策略
kubectl exec -it <redis-pod-name> -n cloudflow -- redis-cli CONFIG SET maxmemory-policy allkeys-lru
kubectl exec -it <redis-pod-name> -n cloudflow -- redis-cli CONFIG SET maxmemory 4GB
```

---

## 6. 网络问题

### 6.1 Pod 间通信失败

**现象**: 组件之间无法通信

**排查步骤**:

```bash
# 检查网络策略
kubectl get networkpolicy -n cloudflow

# 测试 Pod 间连通性
kubectl exec -it <source-pod> -n cloudflow -- ping <target-pod-ip>

# 检查 DNS 解析
kubectl exec -it <pod-name> -n cloudflow -- nslookup cloudflow-center.cloudflow.svc.cluster.local
```

**解决方案**:

```yaml
# 创建网络策略允许内部通信
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-internal
  namespace: cloudflow
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: cloudflow
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: cloudflow
```

### 6.2 外部访问失败

**现象**: 无法从集群外部访问 Center API

**排查步骤**:

```bash
# 检查 Service 类型
kubectl get svc cloudflow-center -n cloudflow

# 检查 Ingress 配置
kubectl get ingress -n cloudflow

# 检查防火墙规则
kubectl exec -it <center-pod-name> -n cloudflow -- curl -I http://localhost:8080
```

---

## 7. eBPF 问题

### 7.1 eBPF 程序编译失败

**现象**: Agent 启动时编译失败

**排查步骤**:

```bash
# 检查内核版本
uname -r

# 检查编译器
kubectl exec -it <agent-pod-name> -n cloudflow -- which clang

# 查看编译日志
kubectl logs <agent-pod-name> -n cloudflow | grep -i "compile\|clang"
```

**解决方案**:

确保使用支持的内核版本（4.15+），并检查容器内是否安装了 clang 编译器。

### 7.2 流量采集不完整

**现象**: 部分流量未被采集

**排查步骤**:

```bash
# 检查 eBPF 程序状态
kubectl exec -it <agent-pod-name> -n cloudflow -- bpftool prog list

# 检查映射表
kubectl exec -it <agent-pod-name> -n cloudflow -- bpftool map list

# 检查流量统计
kubectl exec -it <agent-pod-name> -n cloudflow -- cat /sys/fs/bpf/flow_stats
```

---

## 8. 监控告警

### 8.1 告警规则配置

```yaml
# 示例告警规则
groups:
- name: cloudflow
  rules:
  - alert: AgentDown
    expr: absent(up{app="cloudflow-agent"})
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Agent 服务已停止"
      description: "Agent {{ $labels.instance }} 已停止超过 5 分钟"

  - alert: HighMemoryUsage
    expr: container_memory_usage_bytes{app="cloudflow-edge"} / container_memory_limit_bytes > 0.8
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Edge 内存使用率过高"
      description: "Edge 内存使用率达到 {{ $value | humanizePercentage }}"

  - alert: ClickHouseDown
    expr: clickhouse_server_status != 1
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "ClickHouse 服务异常"
      description: "ClickHouse 服务状态为 {{ $value }}"
```

### 8.2 告警通知渠道

**配置示例**:

```yaml
receivers:
- name: 'email'
  email_configs:
  - to: 'ops@example.com'
    send_resolved: true

- name: 'webhook'
  webhook_configs:
  - url: 'https://api.example.com/alert'
    send_resolved: true
```

---

## 9. 日志分析

### 9.1 日志收集

```bash
# 收集所有组件日志
kubectl logs -n cloudflow --all-containers --prefix=true > cloudflow-logs.txt

# 收集特定组件日志
kubectl logs -n cloudflow -l app=cloudflow-center --all-containers > center-logs.txt

# 实时查看日志
kubectl logs -n cloudflow -l app=cloudflow-agent -f --tail=100
```

### 9.2 日志过滤

```bash
# 过滤错误日志
kubectl logs <pod-name> -n cloudflow | grep -i error

# 过滤警告日志
kubectl logs <pod-name> -n cloudflow | grep -i warn

# 按时间范围过滤
kubectl logs <pod-name> -n cloudflow --since=1h
```

### 9.3 常见日志模式

| 日志模式 | 含义 | 处理建议 |
|----------|------|----------|
| `connection refused` | 连接被拒绝 | 检查目标服务是否运行 |
| `timeout` | 连接超时 | 检查网络和服务状态 |
| `authentication failed` | 认证失败 | 检查密钥和证书 |
| `out of memory` | 内存不足 | 增加资源限制或优化代码 |
| `disk full` | 磁盘满 | 清理空间或扩容 |

---

## 10. 常见错误码

### 10.1 HTTP 错误码

| 错误码 | 含义 | 排查方向 |
|--------|------|----------|
| 400 Bad Request | 请求参数错误 | 检查请求参数格式 |
| 401 Unauthorized | 认证失败 | 检查 API Key 或 JWT Token |
| 403 Forbidden | 权限不足 | 检查用户权限配置 |
| 404 Not Found | 资源不存在 | 检查 API 路径是否正确 |
| 429 Too Many Requests | 请求过于频繁 | 检查限流配置或降低请求频率 |
| 500 Internal Server Error | 服务器内部错误 | 查看服务日志 |
| 502 Bad Gateway | 网关错误 | 检查后端服务状态 |
| 503 Service Unavailable | 服务不可用 | 检查 Pod 状态 |

### 10.2 gRPC 错误码

| 错误码 | 含义 | 排查方向 |
|--------|------|----------|
| `UNAVAILABLE` | 服务不可用 | 检查网络和服务状态 |
| `UNAUTHENTICATED` | 未认证 | 检查认证配置 |
| `DEADLINE_EXCEEDED` | 请求超时 | 检查网络延迟或增加超时时间 |
| `RESOURCE_EXHAUSTED` | 资源耗尽 | 检查资源限制 |

---

## 附录：紧急恢复流程

### A.1 单点故障恢复

```bash
# 删除故障 Pod（自动重建）
kubectl delete pod <faulty-pod> -n cloudflow

# 强制重启 Deployment
kubectl rollout restart deployment/cloudflow-center -n cloudflow
kubectl rollout restart deployment/cloudflow-edge -n cloudflow
kubectl rollout restart daemonset/cloudflow-agent -n cloudflow
```

### A.2 数据恢复

```bash
# 从备份恢复 ClickHouse
clickhouse-client --host clickhouse.cloudflow.svc.cluster.local -q "RESTORE DATABASE cloudflow FROM 's3://backup/cloudflow'"

# 从 RDB 文件恢复 Redis
kubectl cp backup.rdb <redis-pod>:/data/backup.rdb
kubectl exec -it <redis-pod> -n cloudflow -- redis-cli SHUTDOWN NOSAVE
kubectl exec -it <redis-pod> -n cloudflow -- cp /data/backup.rdb /data/dump.rdb
kubectl exec -it <redis-pod> -n cloudflow -- redis-server /etc/redis/redis.conf
```

---

**文档版本**: v1.0  
**最后更新**: 2024-01-15  
**适用版本**: CloudFlow v1.0+