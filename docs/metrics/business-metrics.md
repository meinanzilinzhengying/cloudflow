# P24: 业务指标埋点设计文档

> 解决业务层面可观测性不足：用户行为指标缺失、业务流程追踪不完整、租户级别指标不够细

## 目录

- [一、设计目标](#一设计目标)
- [二、指标分类](#二指标分类)
- [三、使用方法](#三使用方法)
- [四、关键业务流程埋点示例](#四关键业务流程埋点示例)
- [五、Prometheus 查询示例](#五prometheus-查询示例)
- [六、Grafana 看板建议](#六grafana-看板建议)
- [七、告警规则](#七告警规则)
- [八、注意事项](#八注意事项)

---

## 一、设计目标

| 问题 | 解决方案 |
|------|----------|
| 用户行为指标缺失 | 定义 4 个用户行为指标（登录/操作/会话/时长） |
| 业务流程追踪不完整 | 定义 4 个业务流程指标 + Tracer 追踪器 |
| 租户级别指标不够细 | 定义 6 个租户级别指标（API/资源/告警/配额） |

---

## 二、指标分类

### 1. 用户行为指标 (`cloudflow_user_*`)

| 指标名 | 类型 | Labels | 说明 |
|--------|------|--------|------|
| `cloudflow_user_login_total` | Counter | tenant_id, action, status | 登录/登出次数 |
| `cloudflow_user_actions_total` | Counter | tenant_id, user_id, action_type | 用户操作次数 |
| `cloudflow_user_active_sessions` | Gauge | tenant_id | 当前活跃会话数 |
| `cloudflow_user_session_duration_seconds` | Histogram | tenant_id | 会话时长分布 |

### 2. 业务流程指标 (`cloudflow_business_*`)

| 指标名 | 类型 | Labels | 说明 |
|--------|------|--------|------|
| `cloudflow_business_operation_total` | Counter | tenant_id, operation, status | 业务操作次数 |
| `cloudflow_business_operation_duration_seconds` | Histogram | tenant_id, operation, stage | 操作耗时（按阶段） |
| `cloudflow_business_pipeline_stage_total` | Counter | tenant_id, pipeline, stage, status | 流程阶段执行次数 |
| `cloudflow_business_pipeline_stage_latency_seconds` | Histogram | tenant_id, pipeline, stage | 流程阶段延迟 |

**operation 取值：** `alert_evaluate`, `flow_ingest`, `query_execute`, `topology_build`, `rule_create`, `user_login`

**stage 取值：** `total`, `validation`, `processing`, `storage`, `decode`, `enrich`, `evaluate`, `notify`

### 3. 租户级别指标 (`cloudflow_tenant_*`)

| 指标名 | 类型 | Labels | 说明 |
|--------|------|--------|------|
| `cloudflow_tenant_api_calls_total` | Counter | tenant_id, method, endpoint | 租户 API 调用 |
| `cloudflow_tenant_resource_usage` | Gauge | tenant_id, resource_type | 资源使用量 |
| `cloudflow_tenant_alert_count` | Gauge | tenant_id, severity | 活跃告警数 |
| `cloudflow_tenant_active_users` | Gauge | tenant_id | 活跃用户数 |
| `cloudflow_tenant_quota_usage_ratio` | Gauge | tenant_id, quota_type | 配额使用率 (0.0~1.0) |
| `cloudflow_tenant_data_ingest_rate_bytes` | Gauge | tenant_id | 数据摄入速率 (B/s) |

**resource_type 取值：** `flows/min`, `flows/hour`, `metrics`, `storage_bytes`, `agent_count`

**quota_type 取值：** `flows/min`, `metrics/min`, `storage`, `retention`, `agent_count`

---

## 三、使用方法

### 3.1 快速埋点函数

```go
import "github.com/cloudflow/pkg/metrics"

// 记录用户登录
metrics.RecordUserLogin(tenantID, "login", "success")

// 记录用户操作
metrics.RecordUserAction(tenantID, userID, "query")

// 记录业务操作
metrics.RecordBusinessOperation(tenantID, "query_flows", "success")

// 记录业务操作耗时
metrics.RecordBusinessOperationDuration(tenantID, "query_flows", "processing", duration)

// 设置租户资源使用
metrics.SetTenantResourceUsage(tenantID, "flows/min", 1200)
metrics.SetTenantQuotaUsage(tenantID, "flows/min", 0.6) // 60%
```

### 3.2 业务流程追踪器（带阶段追踪）

```go
// 方式一：手动管理 tracer
tracer := metrics.NewBusinessTracer("flow_ingest", tenantID)
defer tracer.Close(metrics.StatusSuccess)

span1 := tracer.Start("decode")
// ... 解码逻辑 ...
span1.End(metrics.StatusSuccess)

span2 := tracer.Start("enrich")
// ... 丰富逻辑 ...
span2.End(metrics.StatusSuccess)

span3 := tracer.Start("storage")
// ... 存储逻辑 ...
span3.End(metrics.StatusFailure)
```

```go
// 方式二：自动追踪（推荐）
err := metrics.TraceBusinessOperationWithStages(ctx, "flow_ingest", func(tracer *metrics.BusinessTracer) error {
    span := tracer.Start("decode")
    // ... 解码 ...
    span.End(metrics.StatusSuccess)

    span2 := tracer.Start("storage")
    // ... 存储 ...
    span2.End(metrics.StatusSuccess)

    return nil
})
```

```go
// 方式三：简单函数包装（无阶段追踪）
err := metrics.TraceFunc(ctx, "query_flows", "total", func() error {
    // ... 业务逻辑 ...
    return db.Query(...)
})
```

### 3.3 中间件集成

```go
// HTTP 中间件（在 Auth 和 Tenant 中间件之后使用）
router.Use(metrics.HttpMiddleware)           // 基础 HTTP 指标
router.Use(metrics.BusinessHTTPMiddleware)   // 业务 HTTP 指标

// gRPC 拦截器
grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(metrics.BusinessGRPCUnaryInterceptor()),
    grpc.StreamInterceptor(metrics.BusinessGRPCStreamInterceptor()),
)
```

### 3.4 从 Context 获取租户/用户 ID

```go
// 在 handler 中注入
ctx = metrics.WithTenantID(ctx, "tenant-123")
ctx = metrics.WithUserID(ctx, "user-456")

// 在后续调用中获取
tenantID := metrics.GetTenantID(ctx)
userID := metrics.GetUserID(ctx)
```

---

## 四、关键业务流程埋点示例

### 4.1 数据摄入流程 (`flow_ingest`)

```go
func (s *DataPlane) IngestFlows(ctx context.Context, batch *FlowBatch) error {
    return metrics.TraceBusinessOperationWithStages(ctx, "flow_ingest", func(tracer *metrics.BusinessTracer) error {
        // 阶段 1: 解码
        span := tracer.Start("decode")
        decoded, err := s.decodeBatch(batch)
        if err != nil {
            span.End(metrics.StatusFailure)
            return err
        }
        span.End(metrics.StatusSuccess)

        // 阶段 2: 验证
        span = tracer.Start("validation")
        if err := s.validateFlows(decoded); err != nil {
            span.End(metrics.StatusFailure)
            return err
        }
        span.End(metrics.StatusSuccess)

        // 阶段 3: 丰富
        span = tracer.Start("enrich")
        enriched := s.enrichFlows(decoded)
        span.End(metrics.StatusSuccess)

        // 阶段 4: 存储
        span = tracer.Start("storage")
        if err := s.storage.Write(enriched); err != nil {
            span.End(metrics.StatusFailure)
            return err
        }
        span.End(metrics.StatusSuccess)

        return nil
    })
}
```

### 4.2 告警评估流程 (`alert_evaluate`)

```go
func (e *AlertEngine) EvaluateRule(ctx context.Context, rule *AlertRule) error {
    return metrics.TraceBusinessOperationWithStages(ctx, "alert_evaluate", func(tracer *metrics.BusinessTracer) error {
        // 查询数据
        span := tracer.Start("query")
        data, err := e.queryService.Query(rule.Query)
        if err != nil {
            span.End(metrics.StatusFailure)
            return err
        }
        span.End(metrics.StatusSuccess)

        // 评估条件
        span = tracer.Start("evaluate")
        triggered, err := e.evaluateCondition(rule, data)
        if err != nil {
            span.End(metrics.StatusFailure)
            return err
        }
        span.End(metrics.StatusSuccess)

        // 触发告警
        if triggered {
            span = tracer.Start("notify")
            err := e.notify(rule)
            if err != nil {
                span.End(metrics.StatusFailure)
                return err
            }
            span.End(metrics.StatusSuccess)
        }

        return nil
    })
}
```

### 4.3 查询执行流程 (`query_execute`)

```go
func (s *QueryService) ExecuteQuery(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
    return metrics.TraceFuncWithValue(ctx, "query_execute", "total", func() (*QueryResponse, error) {
        // 解析查询
        metrics.TraceFunc(ctx, "query_execute", "parse", func() error {
            // ...
            return nil
        })

        // 执行查询
        metrics.TraceFunc(ctx, "query_execute", "execution", func() error {
            // ...
            return nil
        })

        // 格式化结果
        return s.formatResults(...)
    })
}
```

### 4.4 用户登录流程 (`user_login`)

```go
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
    tenantID := req.TenantID

    // 记录登录尝试
    metrics.RecordUserLogin(tenantID, "login", "attempt")

    resp, err := s.authenticate(ctx, req)
    if err != nil {
        metrics.RecordUserLogin(tenantID, "login", "failure")
        return nil, err
    }

    metrics.RecordUserLogin(tenantID, "login", "success")
    metrics.SetUserActiveSessions(tenantID, s.sessionStore.Count(tenantID))

    return resp, nil
}
```

### 4.5 租户配额更新

```go
func (s *TenantService) UpdateQuotaUsage(ctx context.Context, tenantID string) error {
    quota, used := s.getQuota(tenantID)

    metrics.SetTenantQuotaUsage(tenantID, "flows/min", used.FlowsPerMin/float64(quota.FlowsPerMin))
    metrics.SetTenantQuotaUsage(tenantID, "storage", used.StorageBytes/float64(quota.StorageBytes))
    metrics.SetTenantQuotaUsage(tenantID, "agent_count", float64(used.AgentCount)/float64(quota.AgentCount))

    metrics.SetTenantResourceUsage(tenantID, "flows/min", used.FlowsPerMin)
    metrics.SetTenantResourceUsage(tenantID, "storage_bytes", float64(used.StorageBytes))
    metrics.SetTenantResourceUsage(tenantID, "agent_count", float64(used.AgentCount))

    metrics.SetTenantActiveUsers(tenantID, float64(s.activeUserCount(tenantID)))

    return nil
}
```

---

## 五、Prometheus 查询示例

### 5.1 用户行为

```promql
# 各租户登录成功率（近 5 分钟）
sum by(tenant_id) (rate(cloudflow_user_login_total{action="login", status="success"}[5m]))
/
sum by(tenant_id) (rate(cloudflow_user_login_total{action="login"}[5m]))

# 各租户活跃会话数
cloudflow_user_active_sessions

# 用户操作分布（Top 10）
topk(10, sum by(action_type) (rate(cloudflow_user_actions_total[5m])))
```

### 5.2 业务流程

```promql
# 告警评估 P95 延迟
histogram_quantile(0.95,
  sum by(le, operation) (rate(cloudflow_business_operation_duration_seconds{operation="alert_evaluate", stage="total"}[5m]))
)

# 数据摄入各阶段延迟对比
sum by(stage) (rate(cloudflow_business_pipeline_stage_latency_seconds_sum{pipeline="flow_ingest"}[5m]))
/
sum by(stage) (rate(cloudflow_business_pipeline_stage_latency_seconds_count{pipeline="flow_ingest"}[5m]))

# 业务操作失败率（近 5 分钟）
sum by(operation) (rate(cloudflow_business_operation_total{status="failure"}[5m]))
/
sum by(operation) (rate(cloudflow_business_operation_total[5m]))
```

### 5.3 租户级别

```promql
# 各租户配额使用率（Top 10）
topk(10, cloudflow_tenant_quota_usage_ratio)

# 各租户告警数量
sum by(tenant_id) (cloudflow_tenant_alert_count)

# 各租户数据摄入速率（MB/s）
sum by(tenant_id) (cloudflow_tenant_data_ingest_rate_bytes) / 1024 / 1024

# 各租户 API 调用 QPS
sum by(tenant_id) (rate(cloudflow_tenant_api_calls_total[5m]))
```

---

## 六、Grafana 看板建议

### 看板 1：租户健康度总览

| Panel | 查询 | 类型 |
|-------|------|------|
| 活跃租户数 | `count(cloudflow_tenant_active_users > 0)` | Stat |
| 各租户 API QPS | `sum by(tenant_id) (rate(...))` | Graph |
| 配额使用率 Top 10 | `topk(10, ...)` | Bar Gauge |
| 告警数量分布 | `sum by(tenant_id, severity) (...)` | Stacked Bar |
| 数据摄入速率 | `sum by(tenant_id) (...)` | Graph |

### 看板 2：用户行为分析

| Panel | 查询 | 类型 |
|-------|------|------|
| 日活用户趋势 | `sum by(tenant_id) (cloudflow_user_active_sessions)` | Graph |
| 登录成功率 | `rate(success) / rate(total)` | Graph |
| 操作类型分布 | `sum by(action_type) (...)` | Pie Chart |
| 平均会话时长 | `histogram_quantile(0.5, ...)` | Graph |

### 看板 3：业务流程性能

| Panel | 查询 | 类型 |
|-------|------|------|
| 各流程 P99 延迟 | `histogram_quantile(0.99, ...)` | Graph |
| 流程失败率 | `rate(failure) / rate(total)` | Graph |
| 阶段延迟瀑布图 | `sum by(stage) (avg latency)` | Bar Gauge |
| 吞吐趋势 | `rate(total)` | Graph |

---

## 七、告警规则

见 `docs/metrics/business-rules.yml`：

```yaml
groups:
  - name: cloudflow_business_alerts
    rules:
      # 租户配额使用率过高
      - alert: TenantQuotaHighUsage
        expr: cloudflow_tenant_quota_usage_ratio > 0.85
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "租户 {{ $labels.tenant_id }} 配额使用率过高"
          description: "配额类型 {{ $labels.quota_type }} 使用率 {{ $value | humanizePercentage }}"

      # 业务流程失败率升高
      - alert: BusinessOperationHighFailureRate
        expr: |
          sum by(operation) (rate(cloudflow_business_operation_total{status="failure"}[5m]))
          /
          sum by(operation) (rate(cloudflow_business_operation_total[5m])) > 0.05
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "业务操作 {{ $labels.operation }} 失败率过高"

      # 数据摄入延迟升高
      - alert: FlowIngestHighLatency
        expr: |
          histogram_quantile(0.95,
            sum by(le) (rate(cloudflow_business_operation_duration_seconds{operation="flow_ingest"}[5m]))
          ) > 1
        for: 3m
        labels:
          severity: warning
        annotations:
          summary: "数据摄入 P95 延迟超过 1 秒"
```

---

## 八、注意事项

1. **高基数防护**：endpoint 已自动规范化（UUID/数字替换为 `{id}`），但自定义 label 仍需注意基数控制
2. **tenant_id 为空**：所有函数自动将空 `tenant_id` 转为 `"unknown"`，避免指标丢失
3. **Histogram 内存**：业务操作 histogram 使用 14 个 bucket，注意内存开销
4. **Gauge 更新频率**：租户资源使用 Gauge 建议每 30 秒更新一次，避免过于频繁
5. **Context 传递**：HTTP 中间件和 gRPC 拦截器会自动注入 `tenant_id`/`user_id` 到 context，后续 handler 无需手动处理
6. **Tracer 并发安全**：`BusinessTracer` 内部使用 `sync.Mutex` 保护 stages，可安全用于并发场景（但通常建议每个请求独立创建一个 tracer）
