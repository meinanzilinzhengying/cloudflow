# CloudFlow API 参考文档

## 目录
1. [认证方式](#认证方式)
2. [REST API](#rest-api)
3. [gRPC API](#grpc-api)
4. [错误码说明](#错误码说明)

---

## 认证方式

### Bearer Token 认证

所有API请求都需要在Header中携带JWT Token：

```bash
curl -H "Authorization: Bearer <your-token>" \
     https://cloudflow.example.com/api/v1/flows
```

### 获取Token

**请求：**
```http
POST /api/v1/auth/login
Content-Type: application/json

{
    "username": "admin",
    "password": "your-password"
}
```

**响应：**
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
        "expire_at": "2024-01-01T00:00:00Z"
    }
}
```

### Token吊销

**请求：**
```http
POST /api/v1/auth/revoke
Authorization: Bearer <your-token>
Content-Type: application/json

{
    "token": "token-to-revoke"
}
```

**响应：**
```json
{
    "code": 0,
    "message": "Token revoked successfully"
}
```

---

## REST API

### 1. 流量查询 API

#### 获取流量列表

**请求：**
```http
GET /api/v1/flows
Authorization: Bearer <token>
```

**查询参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| limit | int | 否 | 返回条数，默认100 |
| offset | int | 否 | 偏移量，默认0 |
| start_time | string | 否 | 开始时间，ISO8601格式 |
| end_time | string | 否 | 结束时间，ISO8601格式 |
| src_ip | string | 否 | 源IP过滤 |
| dst_ip | string | 否 | 目的IP过滤 |
| src_port | int | 否 | 源端口过滤 |
| dst_port | int | 否 | 目的端口过滤 |
| protocol | string | 否 | 协议过滤：tcp/udp/icmp |
| vni | int | 否 | VXLAN VNI过滤 |
| tenant_id | string | 否 | 租户ID过滤 |

**响应：**
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "total": 1000,
        "flows": [
            {
                "id": "flow-xxx",
                "src_ip": "192.168.1.100",
                "dst_ip": "10.0.0.50",
                "src_port": 54321,
                "dst_port": 80,
                "protocol": "tcp",
                "bytes": 102400,
                "packets": 256,
                "vni": 100,
                "start_time": "2024-01-01T00:00:00Z",
                "end_time": "2024-01-01T00:00:10Z"
            }
        ]
    }
}
```

#### 获取流量聚合统计

**请求：**
```http
GET /api/v1/flows/aggregate
Authorization: Bearer <token>
```

**查询参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| group_by | string | 是 | 聚合维度：src_ip/dst_ip/protocol/vni |
| start_time | string | 是 | 开始时间 |
| end_time | string | 是 | 结束时间 |

**响应：**
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "aggregations": [
            {
                "group_key": "192.168.1.100",
                "total_bytes": 104857600,
                "total_packets": 262144,
                "flow_count": 1024
            }
        ]
    }
}
```

### 2. Agent管理 API

#### 获取Agent列表

**请求：**
```http
GET /api/v1/agents
Authorization: Bearer <token>
```

**响应：**
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "agents": [
            {
                "id": "agent-xxx",
                "hostname": "server-01",
                "ip": "192.168.1.10",
                "status": "online",
                "version": "1.0.0",
                "last_heartbeat": "2024-01-01T00:00:00Z",
                "interfaces": ["eth0", "eth1"]
            }
        ]
    }
}
```

#### 获取Agent详情

**请求：**
```http
GET /api/v1/agents/{agent_id}
Authorization: Bearer <token>
```

#### 更新Agent配置

**请求：**
```http
PUT /api/v1/agents/{agent_id}/config
Authorization: Bearer <token>
Content-Type: application/json

{
    "sample_rate": 80,
    "interfaces": ["eth0"],
    "report_interval": 2
}
```

### 3. 告警管理 API

#### 获取告警规则列表

**请求：**
```http
GET /api/v1/alerts/rules
Authorization: Bearer <token>
```

**响应：**
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "rules": [
            {
                "id": "rule-xxx",
                "name": "高流量告警",
                "description": "单流超过1GB触发告警",
                "condition": "bytes > 1073741824",
                "threshold": 1073741824,
                "enabled": true,
                "created_at": "2024-01-01T00:00:00Z"
            }
        ]
    }
}
```

#### 创建告警规则

**请求：**
```http
POST /api/v1/alerts/rules
Authorization: Bearer <token>
Content-Type: application/json

{
    "name": "高流量告警",
    "description": "单流超过1GB触发告警",
    "condition": "bytes > 1073741824",
    "threshold": 1073741824,
    "severity": "warning",
    "channels": ["webhook", "email"]
}
```

#### 获取告警历史

**请求：**
```http
GET /api/v1/alerts/history
Authorization: Bearer <token>
```

### 4. 租户管理 API

#### 获取租户列表

**请求：**
```http
GET /api/v1/tenants
Authorization: Bearer <token>
```

#### 创建租户

**请求：**
```http
POST /api/v1/tenants
Authorization: Bearer <token>
Content-Type: application/json

{
    "name": "租户A",
    "description": "示例租户",
    "quota": {
        "max_flows_per_day": 1000000,
        "max_storage_gb": 100
    }
}
```

### 5. 健康检查 API

#### 服务健康检查

**请求：**
```http
GET /health
```

**响应（健康）：**
```json
{
    "status": "healthy",
    "components": [
        {
            "name": "mysql",
            "status": "healthy",
            "latency_ms": 5
        },
        {
            "name": "clickhouse",
            "status": "healthy",
            "latency_ms": 10
        },
        {
            "name": "redis",
            "status": "healthy",
            "latency_ms": 2
        },
        {
            "name": "kafka",
            "status": "healthy",
            "latency_ms": 8
        },
        {
            "name": "etcd",
            "status": "healthy",
            "latency_ms": 3
        }
    ],
    "timestamp": "2024-01-01T00:00:00Z"
}
```

**响应（不健康）：**
```json
{
    "status": "unhealthy",
    "components": [
        {
            "name": "mysql",
            "status": "unhealthy",
            "error": "connection refused",
            "latency_ms": 0
        }
    ],
    "timestamp": "2024-01-01T00:00:00Z"
}
```

---

## gRPC API

### Protobuf 定义

完整的Protobuf定义见 `proto/edge.proto`

### 1. ProbeService - Agent通信服务

#### RegisterProbe - Agent注册

```protobuf
rpc RegisterProbe(RegisterProbeRequest) returns (RegisterProbeResponse) {}

message RegisterProbeRequest {
    string probe_id = 1;
    string hostname = 2;
    string version = 3;
    repeated string interfaces = 4;
}

message RegisterProbeResponse {
    bool success = 1;
    string config_version = 2;
    string message = 3;
}
```

#### Heartbeat - 心跳上报

```protobuf
rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse) {}

message HeartbeatRequest {
    string probe_id = 1;
    uint64 timestamp = 2;
    AgentMetrics metrics = 3;
}

message HeartbeatResponse {
    bool success = 1;
    uint64 server_time = 2;
}
```

#### GetConfig - 获取配置

```protobuf
rpc GetConfig(GetConfigRequest) returns (GetConfigResponse) {}

message GetConfigRequest {
    string probe_id = 1;
    string current_version = 2;
}

message GetConfigResponse {
    string config_version = 1;
    string config_yaml = 2;
    string sha256_checksum = 3;
    bool need_update = 4;
}
```

#### SendMetrics - 指标上报

```protobuf
rpc SendMetrics(SendMetricsRequest) returns (SendMetricsResponse) {}

message SendMetricsRequest {
    string probe_id = 1;
    repeated Metric metrics = 2;
}

message Metric {
    string name = 1;
    map<string, string> labels = 2;
    double value = 3;
    uint64 timestamp = 4;
}
```

#### StreamData - 流式数据上报（双向流）

```protobuf
rpc StreamData(stream StreamDataRequest) returns (stream StreamDataResponse) {}

message StreamDataRequest {
    oneof data {
        FlowBatch flows = 1;
        MetricsBatch metrics = 2;
        TraceBatch traces = 3;
        CommandAck ack = 4;
    }
}

message StreamDataResponse {
    oneof data {
        Command command = 1;
        ConfigUpdate config = 2;
        Ack ack = 3;
    }
}
```

### 2. 支持的指令类型

```protobuf
enum CommandType {
    START_COLLECTION = 0;
    STOP_COLLECTION = 1;
    RELOAD_CONFIG = 2;
    UPDATE_FILTER = 3;
    SET_LOG_LEVEL = 4;
    HEARTBEAT_ACK = 5;
}
```

---

## 错误码说明

### HTTP 状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 请求成功 |
| 400 | 请求参数错误 |
| 401 | 未认证或Token无效 |
| 403 | 权限不足 |
| 404 | 资源不存在 |
| 429 | 请求过于频繁 |
| 500 | 服务器内部错误 |
| 503 | 服务不可用 |

### 业务错误码

| 错误码 | 说明 |
|--------|------|
| 0 | 成功 |
| 10001 | 参数错误 |
| 10002 | 认证失败 |
| 10003 | Token过期 |
| 10004 | Token已吊销 |
| 10005 | 权限不足 |
| 20001 | 资源不存在 |
| 20002 | 资源已存在 |
| 30001 | 数据库错误 |
| 30002 | 缓存错误 |
| 30003 | 消息队列错误 |
| 40001 | Agent离线 |
| 40002 | Agent注册失败 |
| 50001 | 内部错误 |

### 错误响应示例

```json
{
    "code": 10001,
    "message": "参数错误: limit必须大于0",
    "request_id": "req-xxx-xxx-xxx"
}
```

---

## 请求限制

### 限流规则

| API | 限制 |
|-----|------|
| 登录接口 | 10次/分钟/IP |
| 查询接口 | 100次/分钟/用户 |
| 写入接口 | 50次/分钟/用户 |
| 管理接口 | 20次/分钟/用户 |

### 响应头

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1704067200
```

超过限制时返回：
```json
{
    "code": 429,
    "message": "Too Many Requests",
    "retry_after": 60
}
```
