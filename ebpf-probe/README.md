# CloudFlow eBPF Probe v3

生产级、全场景、自适应内核的通用 eBPF 采集探针。

## 特性

- **纯 eBPF 内核采集**: 内核态 100% 基于 eBPF 实现数据捕获
- **自适应内核版本**: 支持 Linux 4.15+，自动探测 BTF/CO-RE 能力
- **全维度采集**: 网络、进程、文件、系统调用、TCP 连接
- **Ring Buffer 投递**: 高性能事件通道，旧内核自动降级
- **多形态部署**: 支持 ECS、VM、机顶盒、K8s Node、容器

## 快速开始

### 手动安装

```bash
curl -fsSL https://raw.githubusercontent.com/meinanzilinzhengying/ebpf-probe/main/deploy/install/install.sh | bash
```

### K8s 部署

```bash
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/rbac.yaml
kubectl apply -f deploy/k8s/daemonset.yaml
```

### Docker 运行

```bash
docker run -d --privileged --net=host \
  -e CLICKHOUSE_ADDR=192.168.1.100 \
  cloudflow-ebpf-probe:3.0.0
```

## 编译

```bash
make all
```

## 内核兼容性

| 发行版 | 最低版本 | 状态 |
|--------|---------|------|
| CentOS 7 | 3.10 (部分功能) | 降级运行 |
| CentOS 8/9 | 4.18+ | 完整支持 |
| Ubuntu 20.04 | 5.4+ | 完整支持 |
| Ubuntu 22.04 | 5.15+ | 完整支持 |
| RHEL 8 | 4.18+ | 完整支持 |
| OpenEuler | 5.10+ | 完整支持 |

## 架构

```
cmd/probe          # 主程序
internal/          # 内部模块
  kernel/          # 内核检测
  collector/       # 采集器
  output/          # 输出层
  api/             # HTTP API
pkg/               # 公共库
  ebpf/            # eBPF 加载器
  platform/        # 平台检测
bpf/               # BPF C 代码
  network_flow.bpf.c
  process_exec.bpf.c
  file_open.bpf.c
  tcp_connect.bpf.c
  syscall.bpf.c
deploy/            # 部署产物
  docker/
  k8s/
  systemd/
  install/
config/            # 配置模板
```

## License

GPL-2.0
