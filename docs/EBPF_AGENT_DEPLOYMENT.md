# CloudFlow eBPF Agent 部署指南

## 一、部署架构

### 1.1 架构说明

```
┌─────────────────────────────────────────────────────────────────┐
│                     CloudFlow 平台服务（容器化）                   │
│  ┌──────────┐  ┌──────────┐  ┌────────────┐  ┌──────────────┐  │
│  │ Control  │  │   Data   │  │  Query     │  │   Kafka      │  │
│  │  Plane   │  │  Plane   │  │  Service   │  │              │  │
│  └────┬─────┘  └────┬─────┘  └─────┬──────┘  └──────┬───────┘  │
│       │              │               │                 │         │
│       └──────────────┴───────────────┴─────────────────┘         │
│                              │                                    │
└──────────────────────────────┼────────────────────────────────────┘
                               │ gRPC
                               │
        ┌──────────────────────┼──────────────────────┐
        │                      │                      │
        ▼                      ▼                      ▼
┌───────────────┐    ┌───────────────┐    ┌───────────────┐
│  业务服务器 A  │    │  业务服务器 B  │    │  业务服务器 C  │
│  (裸金属部署)  │    │  (裸金属部署)  │    │  (裸金属部署)  │
│               │    │               │    │               │
│  CloudFlow    │    │  CloudFlow    │    │  CloudFlow    │
│  eBPF Agent   │    │  eBPF Agent   │    │  eBPF Agent   │
└───────────────┘    └───────────────┘    └───────────────┘
```

### 1.2 为什么 Agent 不容器化？

| 维度 | 容器化部署 | 裸金属部署 | 结论 |
|------|-----------|-----------|------|
| **性能** | 有网络namespace隔离开销 | 直接访问主机网络栈，性能提升 15-30% | ✅ 裸金属 |
| **内核依赖** | 需要挂载内核头文件，跨版本兼容性差 | 使用主机内核，兼容性好 | ✅ 裸金属 |
| **权限管理** | 需要 --privileged，安全风险高 | systemd 精确控制 capabilities | ✅ 裸金属 |
| **运维复杂度** | 每台业务机都要跑Docker | 标准 systemd 服务，运维简单 | ✅ 裸金属 |
| **启动速度** | 容器启动慢 | 进程级启动，秒级就绪 | ✅ 裸金属 |
| **资源占用** | Docker 运行时额外开销 | 无额外开销 | ✅ 裸金属 |

**结论**: eBPF Agent 必须裸金属部署在业务服务器上。

---

## 二、系统要求

### 2.1 操作系统支持

| 操作系统 | 版本 | 状态 |
|---------|------|------|
| **CentOS** | 7.x / 8.x | ✅ 支持 |
| **Ubuntu** | 20.04 LTS / 22.04 LTS | ✅ 支持 |
| **Debian** | 11 / 12 | ✅ 支持 |
| **银河麒麟 Kylin** | V10 | ✅ 支持 |
| **统信 UOS** | 20 | ✅ 支持 |
| **中标麒麟 NeoKylin** | 7.0 | ✅ 支持 |
| **深度 Deepin** | 20.x | ✅ 支持 |

### 2.2 内核要求

- **最低版本**: Linux Kernel >= 5.4
- **推荐版本**: Linux Kernel >= 5.10
- **必需配置**:
  - CONFIG_BPF=y
  - CONFIG_BPF_SYSCALL=y
  - CONFIG_BPF_JIT=y
  - CONFIG_HAVE_EBPF_JIT=y
  - CONFIG_BPF_EVENTS=y

### 2.3 硬件要求

| 环境 | CPU | 内存 | 网卡 |
|------|-----|------|------|
| **生产环境** | 4核+ | 8GB+ | 10Gbps |
| **测试环境** | 2核 | 4GB | 1Gbps |

**支持架构**: x86_64 (Intel/AMD/海光)、ARM64 (鲲鹏/飞腾)

---

## 三、一键部署（推荐）

### 3.1 在线安装

```bash
# 下载并执行安装脚本
curl -sSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/cloud-flow-agent/deploy/install.sh | sudo bash
```

### 3.2 离线安装

```bash
# 1. 下载部署包到业务服务器
wget https://github.com/meinanzilinzhengying/cloudflow/archive/refs/heads/main.tar.gz
tar -xzf main.tar.gz

# 2. 执行安装
cd cloudflow-main/cloud-flow-agent/deploy
sudo bash install.sh
```

### 3.3 安装完成验证

```bash
# 查看服务状态
systemctl status cloudflow-agent

# 查看日志
journalctl -u cloudflow-agent -f

# 健康检查
curl http://localhost:9090/healthz

# 查看指标
curl http://localhost:9090/metrics
```

---

## 四、手动部署步骤

### 4.1 环境准备

```bash
# ========== Ubuntu/Debian/麒麟/UOS ==========
sudo apt-get update
sudo apt-get install -y clang llvm libbpf-dev linux-headers-$(uname -r) libelf-dev

# ========== CentOS/RHEL ==========
sudo yum install -y epel-release
sudo yum install -y clang llvm libbpf-devel kernel-devel-$(uname -r) elfutils-libelf-devel
```

### 4.2 编译二进制

```bash
# 克隆代码
git clone https://github.com/meinanzilinzhengying/cloudflow.git
cd cloudflow/cloud-flow-agent

# 编译
make build-go

# 验证
./cloud-flow-agent --version
```

### 4.3 部署文件

```bash
# 创建目录
sudo mkdir -p /opt/cloudflow/agent/{bin,config,logs,bpf}

# 复制二进制
sudo cp cloud-flow-agent /opt/cloudflow/agent/bin/
sudo chmod +x /opt/cloudflow/agent/bin/cloud-flow-agent

# 复制 eBPF 字节码
sudo cp internal/ebpfcollector/bpf/*.bpf.o /opt/cloudflow/agent/bpf/

# 复制配置文件
sudo cp deploy/config.yaml /opt/cloudflow/agent/config/
```

### 4.4 修改配置

编辑 `/opt/cloudflow/agent/config/config.yaml`:

```yaml
# 修改为实际的平台服务地址
control_plane_addr: "192.168.1.100:9001"
data_plane_addr: "192.168.1.100:9002"

# 指定采集网卡（留空采集所有）
bpf:
  mgmt_iface: "eth0"
```

### 4.5 安装 systemd 服务

```bash
# 复制服务文件
sudo cp deploy/cloudflow-agent.service /etc/systemd/system/

# 重载配置
sudo systemctl daemon-reload

# 启用开机自启
sudo systemctl enable cloudflow-agent

# 启动服务
sudo systemctl start cloudflow-agent
```

---

## 五、批量部署方案

### 5.1 Ansible Playbook

创建 `deploy-cloudflow-agent.yml`:

```yaml
---
- name: 部署 CloudFlow eBPF Agent
  hosts: all
  become: yes
  tasks:
    - name: 检查内核版本
      assert:
        that: ansible_kernel is version('5.4', '>=')
        fail_msg: "内核版本必须 >= 5.4"

    - name: 安装依赖（Ubuntu/Debian）
      apt:
        name:
          - clang
          - llvm
          - libbpf-dev
          - linux-headers-{{ ansible_kernel }}
        state: present
      when: ansible_os_family == 'Debian'

    - name: 安装依赖（CentOS/RHEL）
      yum:
        name:
          - clang
          - llvm
          - libbpf-devel
          - kernel-devel-{{ ansible_kernel }}
        state: present
      when: ansible_os_family == 'RedHat'

    - name: 创建目录
      file:
        path: /opt/cloudflow/agent/{{ item }}
        state: directory
      loop: [bin, config, logs, bpf]

    - name: 复制二进制文件
      copy:
        src: ./cloud-flow-agent
        dest: /opt/cloudflow/agent/bin/
        mode: '0755'

    - name: 复制配置文件
      template:
        src: ./config.yaml.j2
        dest: /opt/cloudflow/agent/config/config.yaml

    - name: 复制 systemd 服务
      copy:
        src: ./cloudflow-agent.service
        dest: /etc/systemd/system/

    - name: 启动服务
      systemd:
        name: cloudflow-agent
        state: started
        enabled: yes
        daemon_reload: yes
```

执行部署:

```bash
ansible-playbook -i inventory.ini deploy-cloudflow-agent.yml
```

### 5.2 SaltStack / Puppet

参考 Ansible 方案，使用对应配置管理工具实现。

---

## 六、运维指南

### 6.1 服务管理

```bash
# 启动服务
sudo systemctl start cloudflow-agent

# 停止服务
sudo systemctl stop cloudflow-agent

# 重启服务
sudo systemctl restart cloudflow-agent

# 查看状态
sudo systemctl status cloudflow-agent

# 开机自启
sudo systemctl enable cloudflow-agent

# 取消开机自启
sudo systemctl disable cloudflow-agent
```

### 6.2 日志查看

```bash
# 实时查看日志
journalctl -u cloudflow-agent -f

# 查看最近 100 行
journalctl -u cloudflow-agent -n 100

# 查看今天的日志
journalctl -u cloudflow-agent --since today

# 查看错误日志
journalctl -u cloudflow-agent -p err
```

### 6.3 配置修改

```bash
# 1. 修改配置
sudo vi /opt/cloudflow/agent/config/config.yaml

# 2. 重启服务生效
sudo systemctl restart cloudflow-agent
```

### 6.4 卸载 Agent

```bash
# 一键卸载
curl -sSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/cloud-flow-agent/deploy/uninstall.sh | sudo bash

# 或手动执行
cd cloud-flow-agent/deploy
sudo bash uninstall.sh
```

---

## 七、常见问题排查

### 7.1 eBPF 程序加载失败

**症状**: 日志显示 "failed to load eBPF program"

**排查步骤**:
```bash
# 1. 检查内核版本
uname -r

# 2. 检查 BPF 文件系统
mount | grep bpf

# 3. 检查内核头文件
ls /lib/modules/$(uname -r)/build

# 4. 检查内核配置
zcat /proc/config.gz | grep BPF
```

**解决方案**:
- 升级内核到 5.4+
- 安装对应版本的内核头文件
- 挂载 BPF 文件系统: `mount -t bpf none /sys/fs/bpf`

### 7.2 看不到流量数据

**症状**: 服务正常运行，但平台无数据

**排查步骤**:
```bash
# 1. 检查网络连通性
telnet <control-plane-ip> 9001

# 2. 检查采集网卡
ip link show

# 3. 检查防火墙
iptables -L -n

# 4. 查看 Agent 日志
journalctl -u cloudflow-agent | grep -i error
```

**解决方案**:
- 确认配置文件中平台服务地址正确
- 检查安全组/防火墙开放 9001/9002 端口
- 确认 mgmt_iface 配置正确

### 7.3 权限不足

**症状**: 日志显示 "operation not permitted"

**排查步骤**:
```bash
# 1. 确认以 root 运行
ps aux | grep cloudflow-agent

# 2. 检查 capabilities
grep Cap /proc/$(pidof cloudflow-agent)/status
```

**解决方案**:
- 确保服务以 root 用户运行
- 检查 systemd 服务文件中 AmbientCapabilities 配置

### 7.4 性能问题

**症状**: CPU/内存占用过高

**优化建议**:
```bash
# 1. 调整上报间隔（增大减少开销）
report:
  interval: "10s"

# 2. 关闭不需要的功能
bpf:
  enable_http_full: false
  enable_dns_full: false

# 3. 内核参数优化
sysctl -w net.core.bpf_jit_enable=1
sysctl -w net.core.rmem_max=67108864
```

---

## 八、国产化适配

### 8.1 支持的国产芯片

| 芯片 | 架构 | 状态 |
|------|------|------|
| **鲲鹏 920** | ARM64 | ✅ 完全支持 |
| **海光 C86** | x86_64 | ✅ 完全支持 |
| **飞腾 FT-2000+/64** | ARM64 | ✅ 完全支持 |
| **龙芯 3A5000** | LoongArch | ⚠️ 需要适配 |
| **申威 SW64** | Alpha | ⚠️ 需要适配 |

### 8.2 国产操作系统

| 系统 | 版本 | eBPF 后端推荐 |
|------|------|-------------|
| **银河麒麟 Kylin V10** | SP1/SP2 | libbpf |
| **统信 UOS 20** | 1040/1050 | libbpf |
| **中标麒麟 NeoKylin** | 7.0 | libbpf |

**注意**: 国产环境推荐使用 `libbpf` 后端，兼容性更好。

---

## 九、升级指南

### 9.1 在线升级

```bash
# 重新执行安装脚本即可升级
curl -sSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/cloud-flow-agent/deploy/install.sh | sudo bash
```

### 9.2 手动升级

```bash
# 1. 下载新版本二进制
# 2. 停止服务
sudo systemctl stop cloudflow-agent

# 3. 替换二进制
sudo cp new-cloud-flow-agent /opt/cloudflow/agent/bin/

# 4. 启动服务
sudo systemctl start cloudflow-agent
```

---

## 十、联系与支持

- 项目地址: https://github.com/meinanzilinzhengying/cloudflow
- Issue 反馈: https://github.com/meinanzilinzhengying/cloudflow/issues
- 文档中心: /docs/
