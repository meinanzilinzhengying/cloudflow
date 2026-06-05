# CloudFlow 云原生网络流量分析平台

<div align="center">

![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg?logo=go)
![GitHub release](https://img.shields.io/github/release/meinanzilinzhengying/cloudflow.svg)
![GitHub stars](https://img.shields.io/github/stars/meinanzilinzhengying/cloudflow.svg?style=social)
![GitHub forks](https://img.shields.io/github/forks/meinanzilinzhengying/cloudflow.svg?style=social)
![GitHub issues](https://img.shields.io/github/issues/meinanzilinzhengying/cloudflow.svg)
![GitHub pull requests](https://img.shields.io/github/prs/meinanzilinzhengying/cloudflow.svg)
![Contributors](https://img.shields.io/github/contributors/meinanzilinzhengying/cloudflow.svg)
![Last Commit](https://img.shields.io/github/last-commit/meinanzilinzhengying/cloudflow.svg)

**高性能云原生网络流量分析平台 | 基于 eBPF 实现 L3-L7 全栈可观测性**

[English](README.md) | [中文文档](README.md)

</div>

---

CloudFlow 是一个高性能的云原生网络流量分析平台，专注于 Kubernetes 环境下的网络可观测性。支持 L3-L7 全栈流量采集、分析和可视化。

## 🌟 核心优势

- 🚀 **超高性能**: 支持 100K flows/sec 采集，<1% CPU 开销
- 🔍 **全栈可视**: L3-L7 协议全覆盖，从网络层到应用层
- 🏗️ **分布式架构**: Agent → Edge → Center 三层设计，水平扩展
- 🛡️ **可靠保障**: 熔断器保护、断网缓存、自动重传
- 📊 **统一模型**: UnifiedFlow 整合 metrics/logs/traces
- 🔔 **智能告警**: 多通道通知，HMAC-SHA256 签名保证安全

## 📸 项目截图

<div align="center">

### All-in-One 仪表盘

![Dashboard](docs/images/dashboard-overview.png)
*统一的流量监控仪表盘，包含总览、流量监控、网络分析、告警中心等模块*

### 服务拓扑图

![Topology](docs/images/service-topology.png)
*自动发现的服务依赖关系图*

### L7 协议分析

![L7 Analysis](docs/images/l7-protocol-analysis.png)
*HTTP/gRPC/MySQL 等应用层协议详细分析*

### 告警管理

![Alerts](docs/images/alert-management.png)
*智能告警规则配置和通知管理*

</div>

> 💡 **提示**: 如果您想贡献截图，请参考 [CONTRIBUTING.md](CONTRIBUTING.md) 中的文档规范。

## 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                        CloudFlow 架构                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐    │
│  │   Agent      │     │    Edge      │     │   Center     │    │
│  │  (数据采集)   │────▶│  (数据聚合)   │────▶│  (数据存储)   │    │
│  │              │     │              │     │              │    │
│  │  • eBPF      │     │  • Kafka    │     │  • ClickHouse│    │
│  │  • L7 Parser │     │  • 负载均衡  │     │  • TiDB     │    │
│  │  • Metrics   │     │  • 熔断器   │     │  • Portal   │    │
│  └──────────────┘     └──────────────┘     └──────────────┘    │
│         │                    │                    │              │
│         └────────────────────┴────────────────────┘              │
│                              │                                   │
│                    ┌─────────┴─────────┐                         │
│                    │    Kafka 集群     │                         │
│                    │  • metrics.raw   │                         │
│                    │  • metrics.l4    │                         │
│                    │  • metrics.l7    │                         │
│                    │  • traces        │                         │
│                    └──────────────────┘                         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 核心特性

### 1. 全栈流量采集
- **eBPF 无侵入采集**: 基于 eBPF 技术，无需修改应用代码
- **L3-L7 全协议支持**: TCP/UDP/ICMP, HTTP/HTTPS, gRPC, MySQL, Redis, Kafka, DNS 等
- **零拷贝高性能**: 利用 eBPF map 和 ring buffer 实现高效数据传递
- **Kubernetes 元数据关联**: 自动关联 Pod、Service、Deployment 等 K8s 资源

### 2. 统一数据模型
- **UnifiedFlow**: 整合 metrics、logs、traces 的统一流量数据结构
- **高效序列化**: 二进制序列化格式，支持 presence bitmap 优化
- **强类型定义**: Protocol、Direction、FlowRoute 等强类型枚举

### 3. 分布式架构
- **Edge 节点**: 就近数据聚合，减少中心压力
- **一致性哈希**: 基于 hashring 的请求分发
- **熔断保护**: 防止级联故障
- **选举机制**: Raft 协议实现高可用

### 4. 存储与查询
- **ClickHouse**: 时序数据存储，支持 PB 级数据
- **TiDB**: 元数据和索引存储
- **Kafka**: 消息队列，支持实时流处理

## 目录结构

```
cloud-flow/
├── cloud-flow-agent/        # 数据采集 Agent (eBPF)
│   ├── cmd/                 # 入口点
│   ├── internal/            # 内部实现
│   │   ├── ebpfcollector/   # eBPF 数据采集
│   │   ├── l7parser/        # L7 协议解析
│   │   ├── collector/       # 指标收集器
│   │   └── config/          # 配置管理
│   └── pkg/                 # 公共包
│
├── cloud-flow-edge/         # 边缘聚合节点
│   ├── internal/            # 内部实现
│   │   ├── aggregator/      # 数据聚合器
│   │   ├── forwarder/        # 数据转发
│   │   ├── circuitbreaker/  # 熔断器
│   │   └── election/        # 选举机制
│   └── pkg/                 # 公共包
│
├── cloud-flow-center/       # 数据中心服务
│   ├── cmd/
│   │   ├── main.go          # 主服务入口
│   │   ├── portal/          # Web 门户
│   │   └── mixserver/       # MixServer
│   ├── internal/
│   │   ├── storage/         # 存储层
│   │   │   ├── clickhouse/  # ClickHouse 存储
│   │   │   ├── victoriametrics/
│   │   │   └── tidb/        # TiDB 存储
│   │   ├── grpcserver/      # gRPC 服务
│   │   ├── alerting/        # 告警引擎
│   │   └── portal/          # Portal API
│   └── deployments/         # 部署配置
│       └── migrations/      # 数据库迁移
│
├── pkg/                     # 共享公共包
│   ├── flow/                # UnifiedFlow 数据模型
│   ├── errors/              # 错误处理
│   ├── grpcutil/            # gRPC 工具
│   ├── hashring/            # 一致性哈希
│   ├── kafka/               # Kafka 生产者/消费者
│   ├── ratelimit/           # 限流器
│   ├── safety/              # 安全工具 (panic 恢复)
│   ├── storage/             # 存储接口
│   ├── trace/               # 链路追踪
│   └── utils/               # 通用工具
│
├── proto/                   # Protobuf 定义
│   └── edge.go              # Edge 服务接口定义
│
├── services/                # 微服务组件
│   ├── auth-service/        # 认证服务
│   ├── alert-engine/        # 告警引擎
│   ├── control-plane/       # 控制平面
│   ├── data-plane/          # 数据平面
│   ├── query-service/       # 查询服务
│   ├── tenant-service/      # 租户服务
│   ├── topology-engine/     # 拓扑引擎
│   └── shared/              # 共享库
│
├── monitoring/              # 监控配置
│   ├── prometheus/          # Prometheus 配置
│   ├── grafana/             # Grafana 仪表盘
│   └── alertmanager/        # AlertManager
│
├── cloud-flow-frontend/     # 前端 UI
└── docker-compose*.yml      # Docker Compose 配置
```

## 快速开始

### 环境要求

- Go 1.24+
- Docker & Docker Compose
- Kubernetes 1.24+ (可选，用于 K8s 部署)
- 8GB+ RAM
- 4+ CPU cores

### 本地开发

```bash
# 克隆仓库
git clone https://github.com/meinanzilinzhengying/cloudflow.git
cd cloudflow

# 启动依赖服务
docker-compose up -d zookeeper kafka clickhouse tidb

# 构建所有模块
make build

# 运行测试
make test

# 启动服务
make run-center
make run-edge
make run-agent
```

### Docker Compose 部署

```bash
# 启动完整集群
docker-compose -f docker-compose.yml up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f cloud-flow-center
```

### Kubernetes 部署

```bash
# 添加 Helm 仓库 (如果有)
helm repo add cloudflow https://charts.cloudflow.io

# 使用 Helm 安装
helm install cloudflow cloudflow/cloudflow \
  --set agent.enabled=true \
  --set edge.enabled=true \
  --set center.enabled=true
```

## 配置说明

### Agent 配置 (cloud-flow-agent/configs/config.yaml)

```yaml
collector:
  enabled: true
  sample_rate: 1.0
  max_flows_per_second: 100000

ebpf:
  buffer_size: 8388608
  perf_ring_size: 65536

l7:
  parsers:
    - http
    - grpc
    - mysql
    - redis
    - kafka
    - dns

kubernetes:
  enabled: true
  kubeconfig: ~/.kube/config
```

### Edge 配置 (cloud-flow-edge/configs/config.yaml)

```yaml
grpc:
  addr: ":8080"
  max_recv_msg_size: 104857600

kafka:
  brokers:
    - "localhost:9092"
  topics:
    raw: "metrics.raw"
    l4: "metrics.l4"
    l7: "metrics.l7"

aggregation:
  window_size: 10s
  flush_interval: 5s

circuit_breaker:
  failure_threshold: 5
  recovery_timeout: 30s
```

### Center 配置 (cloud-flow-center/configs/config.yaml)

```yaml
grpc:
  port: 50051
  max_concurrent_streams: 1000

storage:
  clickhouse:
    addrs:
      - "localhost:9000"
    database: "cloudflow"
    username: "default"
    password: ""

  tidb:
    dsn: "localhost:4000/cloudflow?parseTime=true"

portal:
  addr: ":8080"
  jwt_secret: "your-secret-key"

rate_limit:
  login:
    bucket_size: 5
    refill_rate: 1
  api:
    bucket_size: 100
    refill_rate: 50
```

## API 参考

### gRPC 接口

#### Edge Service

```protobuf
service EdgeService {
  // 上报指标数据
  rpc PushMetrics(stream MetricData) returns (PushResponse);
  
  // 上报链路追踪
  rpc PushTraces(stream TraceSpanData) returns (PushResponse);
  
  // 健康检查
  rpc HealthCheck(HealthRequest) returns (HealthResponse);
}
```

#### Center Service

```protobuf
service CenterService {
  // 查询指标
  rpc QueryMetrics(QueryRequest) returns (QueryResponse);
  
  // 订阅实时流
  rpc SubscribeStream(StreamRequest) returns (stream StreamData);
  
  // 告警规则管理
  rpc CreateAlertRule(AlertRule) returns (AlertRule);
  rpc ListAlertRules(ListRequest) returns (AlertRuleList);
}
```

### REST API (Portal)

| Method | Endpoint | 描述 |
|--------|----------|------|
| POST | /api/v1/login | 用户登录 |
| GET | /api/v1/metrics | 查询指标 |
| GET | /api/v1/traces | 查询链路 |
| POST | /api/v1/alerts | 创建告警 |
| GET | /api/v1/topology | 获取拓扑 |

## 数据模型

### UnifiedFlow

UnifiedFlow 是 CloudFlow 的核心数据结构，统一表示网络流量：

```go
type UnifiedFlow struct {
    // Header
    Timestamp     int64
    SchemaVersion uint32
    FlowID        uint32
    
    // L3: 网络层
    SrcIP     IP
    DstIP     IP
    IPVersion uint8
    
    // L4: 传输层
    SrcPort   uint16
    DstPort   uint16
    Protocol  Protocol  // TCP/UDP/ICMP
    TCPFlags  uint8
    
    // L7: 应用层
    L7Protocol Protocol  // HTTP/gRPC/MySQL/...
    Method     uint8
    Path       string
    StatusCode uint16
    
    // Metrics
    Bytes     uint64
    Packets   uint64
    LatencyNs uint64
    Direction Direction
    
    // Tags
    Tags map[string]string
}
```

### 协议类型

```go
type Protocol uint8

const (
    ProtoTCP   Protocol = iota + 1  // 6
    ProtoUDP                         // 17
    ProtoICMP                        // 1
    ProtoHTTP                        // L7
    ProtoHTTP2
    ProtoGRPC
    ProtoDNS
    ProtoMySQL
    ProtoRedis
    ProtoKafka
)
```

## 性能基准

测试环境: 4 核 8GB VM, ClickHouse 单节点

| 场景 | 吞吐量 | 延迟 P99 |
|------|--------|----------|
| 原始流采集 | 100K flows/s | < 5ms |
| L7 解析 | 50K flows/s | < 10ms |
| ClickHouse 写入 | 1M rows/s | < 100ms |
| Portal API 查询 | 5K QPS | < 500ms |

## 监控指标

CloudFlow 暴露以下 Prometheus 指标：

```promql
# Agent 指标
cloudflow_agent_flows_total
cloudflow_agent_bytes_total
cloudflow_agent_parse_errors_total

# Edge 指标
cloudflow_edge_aggregation_latency_seconds
cloudflow_edge_forward_errors_total
cloudflow_edge_circuit_breaker_state

# Center 指标
cloudflow_center_storage_write_latency_seconds
cloudflow_center_query_latency_seconds
cloudflow_center_active_connections
```

## 开发指南

### 添加新的 L7 协议解析器

1. 创建解析器文件:

```go
// internal/l7parser/parsers/myprotocol.go
package parsers

type MyProtocolParser struct{}

func (p *MyProtocolParser) Parse(data []byte) (*ParsedData, error) {
    // 实现解析逻辑
}
```

2. 注册解析器:

```go
// internal/l7parser/registry.go
func init() {
    RegisterParser("myprotocol", &MyProtocolParser{})
}
```

### 添加新的存储后端

1. 实现 Storage 接口:

```go
type Storage interface {
    Write(ctx context.Context, flows []*UnifiedFlow) error
    Query(ctx context.Context, req *QueryRequest) (*QueryResponse, error)
    Close() error
}
```

2. 在工厂中注册:

```go
// internal/storage/factory.go
func init() {
    RegisterStorage("mybackend", NewMyBackendStorage)
}
```

## 故障排查

### Agent 无法采集流量

```bash
# 检查 eBPF 权限
kubectl exec -it <agent-pod> -- cat /sys/kernel/debug/tracing/trace_pipe

# 检查内核版本 (需要 >= 4.14)
uname -r

# 检查 Cilium/其他 CNI 兼容性
kubectl get pods -o wide | grep <node>
```

### Edge 节点无法连接 Center

```bash
# 检查 gRPC 连接
kubectl exec -it <edge-pod> -- grpc_health_probe -addr=:8080

# 检查 Kafka 连通性
kubectl exec -it <edge-pod> -- kafka-broker-api-versions --bootstrap-server <center>:9092
```

### ClickHouse 查询慢

```sql
-- 检查活跃查询
SELECT query, elapsed, memory_usage FROM system.processes;

-- 检查表分区
SELECT table, partition, rows FROM system.parts WHERE table = 'flows';
```

## 🤝 贡献指南

我们欢迎所有形式的贡献！无论是 Bug 修复、功能增强、文档改进还是问题报告。

### 快速开始

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'feat: add amazing feature'`)
4. Push 到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

### 详细指南

请阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 了解：
- 📋 开发环境设置
- 🧪 测试要求和运行方法
- 📝 Commit 规范（Conventional Commits）
- 🔍 Code Review 流程
- 💬 社区行为准则

### 代码规范

- ✅ 使用 `gofmt` 格式化代码
- ✅ 遵循 Go 官方命名规范
- ✅ 所有公共 API 必须有文档注释
- ✅ 新功能需要添加单元测试
- ✅ 运行 `golangci-lint` 通过静态检查

```bash
# 运行测试
go test ./...

# 运行 linter
golangci-lint run

# 检查代码格式
gofmt -s -w .
```

## 📊 项目状态

<div align="center">

![GitHub stars](https://img.shields.io/github/stars/meinanzilinzhengying/cloudflow?style=social)
![GitHub forks](https://img.shields.io/github/forks/meinanzilinzhengying/cloudflow?style=social)
![GitHub watchers](https://img.shields.io/github/watchers/meinanzilinzhengying/cloudflow?style=social)

![GitHub issues](https://img.shields.io/github/issues/meinanzilinzhengying/cloudflow)
![GitHub closed issues](https://img.shields.io/github/issues-closed/meinanzilinzhengying/cloudflow)
![GitHub pull requests](https://img.shields.io/github/prs/meinanzilinzhengying/cloudflow)
![GitHub closed PRs](https://img.shields.io/github/issues-pr-closed/meinanzilinzhengying/cloudflow)

![GitHub contributors](https://img.shields.io/github/contributors/meinanzilinzhengying/cloudflow)
![GitHub last commit](https://img.shields.io/github/last-commit/meinanzilinzhengying/cloudflow)
![GitHub commit activity](https://img.shields.io/github/commit-activity/m/meinanzilinzhengying/cloudflow)

</div>

## 📜 许可证

本项目采用 **Apache License 2.0** 开源许可证。

- ✅ 允许商业使用
- ✅ 允许修改和分发
- ✅ 允许专利使用
- ⚠️ 需要保留版权声明
- ⚠️ 需要说明重大修改

详见 [LICENSE](LICENSE) 文件。

## 🙏 致谢

感谢以下开源项目的支持：

- [cilium/ebpf](https://github.com/cilium/ebpf) - eBPF Go 库
- [ClickHouse](https://clickhouse.com/) - 高性能列式数据库
- [TiDB](https://tidb.io/) - 分布式 NewSQL 数据库
- [Kafka](https://kafka.apache.org/) - 分布式流处理平台
- [Prometheus](https://prometheus.io/) - 监控系统
- [Grafana](https://grafana.com/) - 可视化平台

感谢所有为 CloudFlow 做出贡献的开发者！❤️

## 📞 联系我们

- 📧 邮件: [cloudflow@meinanzilinzhengying.com](mailto:cloudflow@meinanzilinzhengying.com)
- 💬 Issues: [GitHub Issues](https://github.com/meinanzilinzhengying/cloudflow/issues)
- 🌐 官网: https://cloudflow.io (建设中)
- 📖 文档: [Wiki](https://github.com/meinanzilinzhengying/cloudflow/wiki) (建设中)

---

<div align="center">

**如果 CloudFlow 对您有帮助，请给我们一个 ⭐ Star！**

Made with ❤️ by CloudFlow Team

</div>
