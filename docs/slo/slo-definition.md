# CloudFlow SLI/SLO 定义

> 版本: v1.0
> 生效日期: 2026-06-21
> 责任人: SRE Team

---

## 1. 概述

本文档定义 CloudFlow 边缘节点（Edge）和探针（Agent）的服务等级指标（SLI）和服务等级目标（SLO），以及错误预算管理策略。

| 术语 | 定义 |
|---|---|
| **SLI** | Service Level Indicator - 服务质量指标，可量化的服务健康度量 |
| **SLO** | Service Level Objective - 服务质量目标，SLI 的目标值 |
| **错误预算** | 在给定时间内允许的最大不可用时间/错误量 |

---

## 2. 服务分级

| 服务级别 | 服务 | 可用性 SLO | 说明 |
|---|---|---|---|
| **P0 - 核心** | Edge 数据转发、Agent 数据采集 | 99.9% | 直接影响数据完整性 |
| **P1 - 重要** | 探针注册、心跳上报 | 99.5% | 影响监控覆盖度 |
| **P2 - 辅助** | 自监控、指标暴露 | 99.0% | 影响可观测性 |

---

## 3. SLI 定义

### 3.1 Edge 服务 SLI

| SLI ID | 名称 | 类型 | 计算方法 | 数据来源 |
|---|---|---|---|---|
| `edge-availability` | 可用性 | 比率 | `1 - (forward_errors / forward_total)` | Prometheus: `cloud_flow_edge_forward_total`, `cloud_flow_edge_forward_errors_total` |
| `edge-latency-p99` | P99 转发延迟 | 延迟 | `histogram_quantile(0.99, forward_duration)` | Prometheus: `cloud_flow_edge_forward_duration_seconds_bucket` |
| `edge-throughput` | 吞吐量 | 速率 | `forward_bytes / 60s` | Prometheus: `cloud_flow_edge_forward_bytes_total` |
| `edge-buffer-drop` | 缓冲区丢弃率 | 比率 | `data_dropped / (data_dropped + forward_total)` | Prometheus: `cloud_flow_edge_data_dropped_total` |
| `edge-memory-usage` | 内存使用率 | 百分比 | `memory_heap_alloc / max_memory_limit` | Prometheus: `cloud_flow_edge_memory_heap_alloc_bytes` |

### 3.2 Agent 探针 SLI

| SLI ID | 名称 | 类型 | 计算方法 | 数据来源 |
|---|---|---|---|---|
| `agent-availability` | 可用性 | 比率 | `1 - (send_errors / send_total)` | Prometheus: `cloud_flow_agent_send_total`, `cloud_flow_agent_send_errors_total` |
| `agent-latency-p99` | P99 采集+上报延迟 | 延迟 | `collect_duration_p99 + send_duration_p99` | Prometheus: `cloud_flow_agent_*_duration_seconds_bucket` |
| `agent-heartbeat-success` | 心跳成功率 | 比率 | `1 - (heartbeat_errors / heartbeat_total)` | Prometheus: `cloud_flow_agent_heartbeat_total`, `cloud_flow_agent_heartbeat_errors_total` |
| `agent-data-loss` | 数据丢失率 | 比率 | `data_dropped / (send_total + data_dropped)` | Prometheus: `cloud_flow_agent_data_dropped_total` |

---

## 4. SLO 目标

### 4.1 Edge 服务 SLO

| SLO ID | SLI | 目标 | 测量窗口 | 合规阈值 |
|---|---|---|---|---|
| `SLO-EDGE-001` | 可用性 ≥ 99.9% | 每月 ≤ 43.8 分钟不可用 | 30 天 | 连续 5 分钟低于阈值触发告警 |
| `SLO-EDGE-002` | P99 转发延迟 ≤ 500ms | 99% 的请求在 500ms 内完成 | 1 小时 | 连续 3 个窗口超过阈值触发告警 |
| `SLO-EDGE-003` | 缓冲区丢弃率 ≤ 0.1% | 每月丢弃 ≤ 0.1% 的数据 | 30 天 | 单日丢弃率 > 0.5% 触发告警 |
| `SLO-EDGE-004` | 内存使用率 ≤ 80% | 峰值内存不超过限制的 80% | 1 小时 | 连续 2 个窗口超过触发告警 |

### 4.2 Agent 探针 SLO

| SLO ID | SLI | 目标 | 测量窗口 | 合规阈值 |
|---|---|---|---|---|
| `SLO-AGENT-001` | 可用性 ≥ 99.5% | 每月 ≤ 3.6 小时不可用 | 30 天 | 连续 10 分钟低于阈值触发告警 |
| `SLO-AGENT-002` | P99 上报延迟 ≤ 1s | 99% 的上报在 1s 内完成 | 1 小时 | 连续 3 个窗口超过阈值触发告警 |
| `SLO-AGENT-003` | 心跳成功率 ≥ 99.5% | 每月心跳失败率 ≤ 0.5% | 30 天 | 连续 5 个心跳失败触发告警 |
| `SLO-AGENT-004` | 数据丢失率 ≤ 0.01% | 每月数据丢失 ≤ 0.01% | 30 天 | 单日丢失率 > 0.1% 触发告警 |

---

## 5. 错误预算管理

### 5.1 错误预算计算

```
错误预算 = (1 - SLO 目标) × 总时间窗口
```

| 服务 | SLO | 月度错误预算 | 消耗速率警戒线 |
|---|---|---|---|
| Edge 可用性 | 99.9% | 43.8 分钟/月 | 1 周消耗 > 25% |
| Agent 可用性 | 99.5% | 3.6 小时/月 | 1 周消耗 > 25% |

### 5.2 错误预算消耗速率

| 消耗速率 | 状态 | 行动 |
|---|---|---|
| 0-25% | 🟢 健康 | 正常工作 |
| 25-50% | 🟡 注意 | 监控加强，准备预案 |
| 50-75% | 🟠 警告 | 启动 RCA 分析，限制变更 |
| 75-100% | 🔴 危险 | 冻结所有非紧急变更，全员 On-Call |
| > 100% | 🔴 超标 | 启动事故复盘，制定改进计划 |

### 5.3 错误预算自动计算（Go 代码）

参见 `cloud-flow-edge/pkg/slo/budget.go` 和 `cloud-flow-agent/pkg/slo/budget.go`。

---

## 6. Prometheus 告警规则

### 6.1 Edge 告警规则

```yaml
groups:
  - name: cloudflow-edge-slo
    rules:
      - alert: EdgeAvailabilitySLOBurn
        expr: |
          (
            sum(rate(cloud_flow_edge_forward_errors_total[1h]))
            /
            sum(rate(cloud_flow_edge_forward_total[1h]))
          ) > 0.001  # 1 - 99.9%
        for: 5m
        labels:
          severity: critical
          slo: edge-availability
        annotations:
          summary: "Edge 可用性低于 SLO 目标"
          description: "过去1小时错误率 {{ $value | humanizePercentage }}，超过 SLO 1‰"

      - alert: EdgeLatencyP99SLOBurn
        expr: |
          histogram_quantile(0.99,
            sum(rate(cloud_flow_edge_forward_duration_seconds_bucket[5m])) by (le)
          ) > 0.5
        for: 3m
        labels:
          severity: warning
          slo: edge-latency
        annotations:
          summary: "Edge P99 转发延迟超过 500ms"
          description: "P99 延迟 {{ $value }}s，超过 SLO 500ms"

      - alert: EdgeBufferDropRateHigh
        expr: |
          rate(cloud_flow_edge_data_dropped_total[1h]) /
          (
            rate(cloud_flow_edge_data_dropped_total[1h]) +
            rate(cloud_flow_edge_forward_total[1h])
          ) > 0.001
        for: 5m
        labels:
          severity: warning
          slo: edge-buffer-drop
        annotations:
          summary: "Edge 缓冲区丢弃率超过 SLO"
          description: "丢弃率 {{ $value | humanizePercentage }}，超过 SLO 0.1%"

      - alert: EdgeMemoryUsageHigh
        expr: |
          cloud_flow_edge_memory_heap_alloc_bytes / (512 * 1024 * 1024) > 0.8
        for: 5m
        labels:
          severity: warning
          slo: edge-memory
        annotations:
          summary: "Edge 内存使用率超过 80%"
          description: "内存使用率 {{ $value | humanizePercentage }}，超过 SLO 80%"
```

### 6.2 Agent 告警规则

```yaml
groups:
  - name: cloudflow-agent-slo
    rules:
      - alert: AgentAvailabilitySLOBurn
        expr: |
          (
            sum(rate(cloud_flow_agent_send_errors_total[1h]))
            /
            sum(rate(cloud_flow_agent_send_total[1h]))
          ) > 0.005  # 1 - 99.5%
        for: 10m
        labels:
          severity: warning
          slo: agent-availability
        annotations:
          summary: "Agent 可用性低于 SLO 目标"

      - alert: AgentHeartbeatFailureRateHigh
        expr: |
          rate(cloud_flow_agent_heartbeat_errors_total[1h]) /
          rate(cloud_flow_agent_heartbeat_total[1h]) > 0.005
        for: 5m
        labels:
          severity: warning
          slo: agent-heartbeat
        annotations:
          summary: "Agent 心跳失败率超过 SLO"

      - alert: AgentDataLossRateHigh
        expr: |
          rate(cloud_flow_agent_data_dropped_total[1h]) /
          (
            rate(cloud_flow_agent_data_dropped_total[1h]) +
            rate(cloud_flow_agent_send_total[1h])
          ) > 0.0001
        for: 5m
        labels:
          severity: warning
          slo: agent-data-loss
        annotations:
          summary: "Agent 数据丢失率超过 SLO"
```

---

## 7. SLO 监控 Dashboard

### 7.1 Grafana Dashboard 面板

| 面板 | 指标 | 用途 |
|---|---|---|
| **SLO 合规率** | `1 - (errors / total)` | 实时显示当前 SLO 达成率 |
| **错误预算消耗** | `累计错误时间 / 月度错误预算` | 显示本月错误预算消耗进度 |
| **P99 延迟趋势** | `histogram_quantile(0.99, ...)` | 延迟趋势可视化 |
| **SLI 热力图** | 各 SLI 达标情况 | 一图看清所有 SLI 状态 |

---

## 8. 持续改进流程

1. **月度 SLO 回顾**：每月 1 日 Review 上月 SLO 达成情况
2. **错误预算耗尽处理**：自动触发变更冻结流程
3. **SLO 调整**：每季度评估 SLO 合理性，必要时调整
4. **SLI 新增**：新功能上线时评估是否需要新增 SLI

---

## 9. 相关代码

| 文件 | 说明 |
|---|---|
| `cloud-flow-edge/pkg/slo/slo.go` | Edge SLO 计算器 |
| `cloud-flow-edge/pkg/slo/budget.go` | 错误预算管理 |
| `cloud-flow-agent/pkg/slo/slo.go` | Agent SLO 计算器 |
| `docs/slo/prometheus-rules.yml` | Prometheus 告警规则 |
| `docs/slo/slo-definition.md` | 本文档 |
