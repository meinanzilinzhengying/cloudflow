# CloudFlow API 参考文档

## 目录
1. [认证方式](#认证方式)
2. [REST API](#rest-api)
3. [gRPC API](#grpc-api)
4. [错误码说明](#错误码说明)

---

## 认证方式

### Bearer Token认证
所有API请求需要在Header中携带JWT Token：

```http
Authorization: Bearer <your-jwt-token>
```

### 获取Token
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
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": "2026-06-17T00:00:00Z",
  "user": {
    "id": "1",
    "username": "admin",
    "role": "admin"
  }
}
```

---

## REST API

### 1. 认证接口

#### 1.1 登录
```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "string",
  "password": "string"
}
```

#### 1.2 登出
```http
POST /api/v1/auth/logout
Authorization: Bearer <token>
```

#### 1.3 吊销Token
```http
POST /api/v1/auth/revoke
Authorization: Bearer <token>
Content-Type: application/json

{
  "token": "string"
}
```

#### 1.4 刷新Token
```http
POST /api/v1/auth/refresh
Authorization: Bearer <token>
```

---

### 2. 流量查询接口

#### 2.1 查询流量数据
```http
GET /api/v1/flows
Authorization: Bearer <token>
```

**查询参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| start_time | string | 否 | 开始时间 (RFC3339) |
| end_time | string | 否 | 结束时间 (RFC3339) |
| src_ip | string | 否 | 源IP |
| dst_ip | string | 否 | 目的IP |
| src_port | int | 否 | 源端口 |
| dst_port | int | 否 | 目的端口 |
| protocol | string | 否 | 协议: TCP/UDP/ICMP |
| vni | int | 否 | VXLAN VNI |
| limit | int | 否 | 返回数量限制，默认100 |
| offset | int | 否 | 偏移量 |

**响应：**
```json
{
  "total": 1000,
  "flows": [
    {
      "id": "flow-001",
      "timestamp": "2026-06-16T12:00:00Z",
      "src_ip": "192.168.1.100",
      "dst_ip": "10.0.0.1",
      "src_port": 54321,
      "dst_port": 80,
      "protocol": "TCP",
      "bytes": 102400,
      "packets": 256,
      "vni": 100,
      "duration_ms": 1500
    }
  ]
}
```

#### 2.2 流量聚合统计
```http
GET /api/v1/flows/aggregate
Authorization: Bearer <token>
```

**查询参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| start_time | string | 是 | 开始时间 |
| end_time | string | 是 | 结束时间 |
| group_by | string | 是 | 聚合维度: src_ip/dst_ip/protocol/vni |

**响应：**
```json
{
  "aggregations": [
    {
      "group_key": "192.168.1.100",
      "total_bytes": 104857600,
      "total_packets": 262144,
      "flow_count": 1024
    }
  ]
}
```

---

### 3. Agent管理接口

#### 3.1 获取Agent列表
```http
GET /api/v1/agents
Authorization: Bearer <token>
```

**响应：**
```json
{
  "agents": [
    {
      "id": "agent-001",
      "hostname": "server-01",
      "ip": "192.168.1.10",
      "status": "online",
      "version": "1.0.0",
      "last_heartbeat": "2026-06-16T12:00:00Z",
      "interfaces": ["eth0", "eth1"]
    }
  ]
}
```

#### 3.2 获取Agent详情
```http
GET /api/v1/agents/{agent_id}
Authorization: Bearer <token>
```

#### 3.3 Agent配置下发
```http
PUT /api/v1/agents/{agent_id}/config
Authorization: Bearer <token>
Content-Type: application/json

{
  "sample_rate": 1,
  "interfaces": ["eth0"],
  "batch_size": 100
}
```

---

### 4. 告警规则接口

#### 4.1 获取告警规则列表
```http
GET /api/v1/alerts/rules
Authorization: Bearer <token>
```

#### 4.2 创建告警规则
```http
POST /api/v1/alerts/rules
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "高CPU告警",
  "expr": "cpu_usage > 80",
  "duration": 300,
  "severity": "warning",
  "enabled": true
}
```

#### 4.3 获取告警历史
```http
GET /api/v1/alerts/history
Authorization: Bearer <token>
```

---

### 5. 健康检查接口

#### 5.1 服务健康检查
```http
GET /health
```

**响应（200 OK）：**
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
      "name": "etcd",
      "status": "healthy",
      "latency_ms": 3
    }
  ],
  "timestamp": "2026-06-16T12:00:00Z"
}
```

**响应（503 Service Unavailable）：**
```json
{
  "status": "unhealthy",
  "components": [
    {
      "name": "mysql",
      "status": "unhealthy",
      "error": "connection refused"
    }
  ]
}
```

---

## gRPC API

### Protobuf定义

#### ProbeService（Agent上报）
```protobuf
service ProbeService {
  // 注册Agent
  rpc RegisterProbe(RegisterProbeRequest) returns (RegisterProbeResponse);
  
  // 心跳上报
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  
  // 获取配置
  rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);
  
  // 上报指标
  rpc SendMetrics(SendMetricsRequest) returns (SendMetricsResponse);
  
  // 上报追踪
  rpc SendTraces(SendTracesRequest) returns (SendTracesResponse);
  
  // 流式数据上报
  rpc StreamData(stream StreamDataRequest) returns (stream StreamDataResponse);
}
```

#### 消息定义
```protobuf
message Flow {
  uint64 timestamp = 1;
  string src_ip = 2;
  string dst_ip = 3;
  uint32 src_port = 4;
  uint32 dst_port = 5;
  uint32 protocol = 6;
  uint64 bytes = 7;
  uint64 packets = 8;
  uint32 vni = 9;
}

message FlowBatch {
  repeated Flow flows = 1;
}

message GetConfigResponse {
  string config_version = 1;
  string config_yaml = 2;
  string sha256_checksum = 3;
}
```

---

## 错误码说明

### HTTP状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 成功 |
| 201 | 创建成功 |
| 400 | 请求参数错误 |
| 401 | 未认证 / Token无效 |
| 403 | 权限不足 |
| 404 | 资源不存在 |
| 429 | 请求过于频繁 |
| 500 | 服务器内部错误 |
| 503 | 服务不可用 |

### 业务错误码

| 错误码 | 说明 |
|--------|------|
| 10001 | 用户名或密码错误 |
| 10002 | Token已过期 |
| 10003 | Token已被吊销 |
| 10004 | 权限不足 |
| 20001 | Agent不存在 |
| 20002 | Agent已离线 |
| 30001 | 数据库连接失败 |
| 30002 | 查询参数错误 |
| 40001 | 告警规则不存在 |
| 40002 | 告警规则已启用 |

### 错误响应格式
```json
{
  "error": {
    "code": 10002,
    "message": "Token已过期",
    "details": "token expired at 2026-06-16T12:00:00Z"
  }
}
```
