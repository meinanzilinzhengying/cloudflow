# CloudFlow eBPF Agent 部署指南

## 概述

CloudFlow eBPF Agent 是基于 eBPF 技术的高性能网络流量采集组件，支持无侵入式的网络数据包捕获、协议解析和性能指标采集。

## 架构特性

- **双后端支持**: 支持 `cilium/ebpf` (Go 原生) 和 `libbpf` (C 后端) 两种加载方式
- **CO-RE 兼容**: 支持编译一次运行到处 (Compile Once - Run Everywhere)
- **跨架构**: 支持 x86_64 和 ARM64 (鲲鹏920/海光C86)
- **容器化部署**: 完整支持 Docker 容器化运行

## 部署方式

### 方式一：Docker Compose（推荐）

```bash
# 启动 eBPF Agent
docker compose up -d cloud-flow-agent

# 查看日志
docker compose logs -f cloud-flow-agent
```

**注意**: eBPF Agent 需要以下特殊权限：
- `privileged: true` - 加载 eBPF 程序需要 root 权限
- `pid: host` - 访问主机进程命名空间
- `network_mode: host` - 访问主机网络命名空间
- 挂载 `/sys/fs/bpf`, `/lib/modules`, `/usr/src`

### 方式二：独立 Docker 运行

```bash
# 构建镜像
cd cloud-flow-agent
docker build -t cloud-flow-agent:latest .

# 运行容器
docker run -d \
  --name cloudflow-ebpf-agent \
  --privileged \
  --pid=host \
  --network=host \
  -v /sys/fs/bpf:/sys/fs/bpf \
  -v /lib/modules:/lib/modules:ro \
  -v /usr/src:/usr/src:ro \
  -e CONTROL_PLANE_ADDR="localhost:9001" \
  -e DATA_PLANE_ADDR="localhost:9002" \
  -e MGMT_IFACE="eth0" \
  cloud-flow-agent:latest
```

### 方式三：二进制部署

```bash
# 安装依赖
make deps

# 编译 eBPF 程序和 Go 二进制
make all

# 运行
./bin/cloud-flow-agent --config configs/config.yaml
```

## 配置说明

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `CONTROL_PLANE_ADDR` | `localhost:9001` | 控制面 gRPC 地址 |
| `DATA_PLANE_ADDR` | `localhost:9002` | 数据面 gRPC 地址 |
| `METRICS_ADDR` | `:9090` | Prometheus 指标端口 |
| `MGMT_IFACE` | `eth0` | 流量采集网卡接口 |
| `CLOUD_FLOW_BPF_BACKEND` | `auto` | eBPF 后端: libbpf/cilium/auto |
| `ENABLE_TCP_METRICS` | `true` | 启用 TCP 深度指标 |
| `ENABLE_HTTP_METRICS` | `true` | 启用 HTTP 请求指标 |
| `ENABLE_HTTP_FULL` | `false` | 启用 HTTP 全字段解析 |
| `ENABLE_DNS_FULL` | `false` | 启用 DNS 全字段解析 |
| `ENABLE_MYSQL_FULL` | `false` | 启用 MySQL 全字段解析 |
| `LOG_LEVEL` | `info` | 日志级别: debug/info/warn/error |

### eBPF 后端选择

1. **auto (默认)**: 自动检测，优先 libbpf，失败回退到 cilium
2. **libbpf**: C 语言后端，更好的国产芯片兼容性 (鲲鹏920/海光C86)
3. **cilium**: Go 原生后端，更好的开发体验

## 采集功能说明

### 基础流量采集 (必选)
- 网络流五元组统计 (源IP/目的IP/源端口/目的端口/协议)
- 字节数、数据包数统计
- 支持 TCP/UDP/ICMP 协议

### TCP 深度指标 (可选)
- 连接建立延迟
- 重传计数
- 零窗口事件
- 队列溢出
- 连接失败统计

### HTTP 指标 (可选)
- 请求方法、路径、状态码
- 请求/响应大小
- 延迟统计

### 协议全字段解析 (可选)
- HTTP: 完整请求头、响应头、请求体
- DNS: 查询域名、响应记录、TTL
- MySQL: SQL 语句、执行时间、结果集大小

## 故障排查

### 常见问题

**1. eBPF 程序加载失败**
```bash
# 检查内核版本 (需要 >= 4.15)
uname -r

# 检查 BPF 文件系统是否挂载
mount | grep bpf

# 检查内核头文件
ls /lib/modules/$(uname -r)/build
```

**2. 权限不足**
- 确保容器运行在 `--privileged` 模式
- 确保挂载了 `/sys/fs/bpf`

**3. 看不到流量数据**
- 检查 `MGMT_IFACE` 是否正确
- 检查控制面和数据面连接
- 查看 Agent 日志: `docker logs cloudflow-ebpf-agent`

### 健康检查

```bash
# 检查 Agent 健康状态
curl http://localhost:9090/healthz

# 查看采集指标
curl http://localhost:9090/metrics
```

## 性能调优

### 推荐配置

| 环境 | CPU | 内存 | 网卡 |
|------|-----|------|------|
| 生产环境 | 4核+ | 8GB+ | 10Gbps |
| 测试环境 | 2核 | 4GB | 1Gbps |

### 内核参数优化

```bash
# 增大 eBPF Map 内存限制
sysctl -w net.core.bpf_jit_enable=1
sysctl -w net.core.rmem_max=67108864
sysctl -w net.core.wmem_max=67108864
```

## 国产化适配

### 支持的国产芯片
- ✅ 鲲鹏 920 (ARM64)
- ✅ 海光 C86 (x86_64)
- ✅ 飞腾 FT-2000+/64
- ✅ 龙芯 3A5000

### 支持的国产操作系统
- ✅ 银河麒麟 Kylin OS V10
- ✅ 统信 UOS 20
- ✅ 中标麒麟 NeoKylin
- ✅ 深度 Deepin
