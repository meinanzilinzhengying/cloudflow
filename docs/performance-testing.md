# CloudFlow 性能压测指南

## 概述

本文档定义了 CloudFlow 平台的性能压测方法、工具配置、测试场景和性能基准指标。

---

## 目录

1. [压测环境要求](#1-压测环境要求)
2. [压测工具](#2-压测工具)
3. [测试场景](#3-测试场景)
4. [性能指标](#4-性能指标)
5. [压测脚本](#5-压测脚本)
6. [结果分析](#6-结果分析)
7. [性能基准报告](#7-性能基准报告)

---

## 1. 压测环境要求

### 1.1 硬件配置

| 组件 | 最低配置 | 推荐配置 |
|------|----------|----------|
| Center | 4 CPU / 8GB RAM | 8 CPU / 16GB RAM |
| Edge | 8 CPU / 16GB RAM | 16 CPU / 32GB RAM |
| Agent | 2 CPU / 4GB RAM | 4 CPU / 8GB RAM |
| ClickHouse | 8 CPU / 32GB RAM | 16 CPU / 64GB RAM |
| Redis | 4 CPU / 8GB RAM | 8 CPU / 16GB RAM |

### 1.2 网络要求

- 压测机与集群网络延迟 < 5ms
- 压测机带宽 >= 10Gbps
- 支持同时 1000+ 并发连接

---

## 2. 压测工具

### 2.1 gRPC 压测工具

使用 `ghz` 工具进行 gRPC 压测：

```bash
# 安装 ghz
go install github.com/bojand/ghz@latest

# 安装 grpcurl（用于健康检查）
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

### 2.2 HTTP 压测工具

使用 `wrk` 或 `ab` 进行 HTTP API 压测：

```bash
# 安装 wrk
# Ubuntu/Debian
sudo apt-get install wrk

# macOS
brew install wrk
```

### 2.3 自定义压测工具

项目提供自定义压测工具：`tools/load-tester/`

---

## 3. 测试场景

### 3.1 流量摄入测试

**场景**：测试 Agent → Edge → Center 的流量摄入能力

**目标指标**：
- 10W+ flows/sec 摄入能力
- 丢包率 < 0.1%
- 端到端延迟 < 100ms

```bash
# 运行流量摄入压测
./load-tester --mode=ingest \
  --agent-count=10 \
  --flows-per-second=100000 \
  --duration=300s
```

### 3.2 API 查询测试

**场景**：测试 Center API 的查询性能

**目标指标**：
- QPS >= 1000
- P99 延迟 < 500ms
- 错误率 < 0.1%

```bash
# 运行 API 查询压测
./load-tester --mode=query \
  --concurrency=100 \
  --requests-per-second=1000 \
  --duration=300s
```

### 3.3 混合负载测试

**场景**：同时测试摄入和查询

```bash
# 运行混合负载压测
./load-tester --mode=mixed \
  --ingest-ratio=0.8 \
  --query-ratio=0.2 \
  --total-flows-per-second=100000 \
  --duration=300s
```

---

## 4. 性能指标

### 4.1 核心指标

| 指标 | 目标值 | 警告阈值 | 严重阈值 |
|------|--------|----------|----------|
| **吞吐量** | >= 100K flows/sec | < 80K flows/sec | < 50K flows/sec |
| **丢包率** | < 0.01% | > 0.1% | > 1% |
| **P50 延迟** | < 10ms | > 50ms | > 100ms |
| **P99 延迟** | < 100ms | > 500ms | > 1s |
| **P999 延迟** | < 500ms | > 2s | > 5s |
| **CPU 使用率** | < 70% | > 80% | > 90% |
| **内存使用率** | < 70% | > 80% | > 90% |

### 4.2 分组件指标

#### Agent 指标

| 指标 | 目标值 | 说明 |
|------|--------|------|
| eBPF 采集速率 | >= 50K flows/sec/agent | 单个 Agent 采集能力 |
| CPU 开销 | < 200m / agent | 每秒 CPU millicores |
| 内存开销 | < 256MB / agent | 每 Agent 内存占用 |

#### Edge 指标

| 指标 | 目标值 | 说明 |
|------|--------|------|
| 数据处理速率 | >= 200K flows/sec/edge | 单个 Edge 处理能力 |
| CPU 开销 | < 2 cores / edge | 每 Edge CPU 占用 |
| 内存开销 | < 2GB / edge | 每 Edge 内存占用 |
| gRPC 连接数 | <= 1000 | 最大 Agent 连接数 |

#### Center 指标

| 指标 | 目标值 | 说明 |
|------|--------|------|
| API QPS | >= 1000 | API 查询吞吐 |
| 写入 QPS | >= 10K | ClickHouse 写入吞吐 |
| CPU 开销 | < 4 cores | API 服务 CPU 占用 |
| 内存开销 | < 4GB | API 服务内存占用 |

---

## 5. 压测脚本

### 5.1 准备压测数据

```bash
# 生成测试流量数据
cd tools/load-tester
go run cmd/generator/main.go \
  --output=data/test-flows-100k.json \
  --count=100000
```

### 5.2 执行压测

```bash
# 1. 启动压测工具
cd tools/load-tester

# 2. 运行基础压测
./load-tester \
  --mode=ingest \
  --target=edge:9002 \
  --flows=100000 \
  --duration=300s \
  --report=report-ingest-100k.json

# 3. 运行 API 压测
./load-tester \
  --mode=query \
  --target=center:8080 \
  --concurrency=100 \
  --requests=100000 \
  --duration=300s \
  --report=report-query-100k.json

# 4. 运行混合负载
./load-tester \
  --mode=mixed \
  --target=center:8080 \
  --edge-target=edge:9002 \
  --total-flows=100000 \
  --duration=300s \
  --report=report-mixed-100k.json
```

### 5.3 监控资源使用

```bash
# 在压测期间监控资源
watch -n 1 'kubectl top pods -n cloudflow'

# 监控特定指标
kubectl exec -it <center-pod> -n cloudflow -- curl -s http://localhost:9090/metrics | grep -E "flow_|grpc_"
```

---

## 6. 结果分析

### 6.1 压测报告格式

```json
{
  "test_info": {
    "test_name": "100K flows/sec ingestion test",
    "start_time": "2024-01-15T10:00:00Z",
    "end_time": "2024-01-15T10:05:00Z",
    "duration_seconds": 300,
    "mode": "ingest"
  },
  "summary": {
    "total_flows_sent": 30000000,
    "total_flows_received": 29997000,
    "loss_rate_percent": 0.01,
    "avg_throughput_per_sec": 99990,
    "peak_throughput_per_sec": 105000
  },
  "latency": {
    "p50_ms": 5.2,
    "p90_ms": 12.8,
    "p99_ms": 45.6,
    "p999_ms": 120.3
  },
  "resource_usage": {
    "center": {
      "cpu_cores": 3.2,
      "memory_mb": 3500
    },
    "edge": {
      "cpu_cores": 7.5,
      "memory_mb": 1800
    },
    "agent": {
      "cpu_cores_per_instance": 0.15,
      "memory_mb_per_instance": 180
    }
  },
  "issues": []
}
```

### 6.2 瓶颈分析方法

1. **CPU 瓶颈**：
   - 检查是否有 Goroutine 阻塞
   - 分析 GC 暂停时间
   - 检查锁竞争情况

2. **内存瓶颈**：
   - 检查内存泄漏
   - 分析对象分配速率
   - 检查缓存配置

3. **网络瓶颈**：
   - 检查连接池配置
   - 分析带宽使用情况
   - 检查超时配置

### 6.3 优化建议

根据压测结果，提供以下优化建议：

| 问题 | 优化方案 |
|------|----------|
| CPU 使用率高 | 增加 Edge 副本数、优化数据处理逻辑 |
| 内存使用率高 | 调整缓冲区大小、启用压缩 |
| 延迟过高 | 启用批量写入、优化查询索引 |
| 丢包率高 | 增加 Edge 副本数、调整接收缓冲区 |

---

## 7. 性能基准报告

详见：`reports/performance-benchmark.md`

---

**文档版本**: v1.0  
**最后更新**: 2024-01-15  
**适用版本**: CloudFlow v1.0+