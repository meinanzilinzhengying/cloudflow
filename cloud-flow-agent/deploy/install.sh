#!/bin/bash
# CloudFlow eBPF Agent 一键安装脚本
# 支持: CentOS 7/8, Ubuntu 20.04+, Debian 11, 银河麒麟V10, 统信UOS

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# 检查root权限
check_root() {
    if [ "$(id -u)" != "0" ]; then
        error "此脚本需要 root 权限运行，请使用 sudo"
    fi
    info "权限检查通过"
}

# 检测内核版本
check_kernel() {
    KERNEL_VERSION=$(uname -r | cut -d'-' -f1)
    KERNEL_MAJOR=$(echo "$KERNEL_VERSION" | cut -d'.' -f1)
    KERNEL_MINOR=$(echo "$KERNEL_VERSION" | cut -d'.' -f2)
    
    info "检测到内核版本: $KERNEL_MAJOR.$KERNEL_MINOR"
    
    if [ "$KERNEL_MAJOR" -lt 5 ] || ([ "$KERNEL_MAJOR" -eq 5 ] && [ "$KERNEL_MINOR" -lt 4 ]); then
        error "eBPF 需要内核版本 >= 5.4，当前版本: $KERNEL_MAJOR.$KERNEL_MINOR"
    fi
    
    info "内核版本检查通过"
}

# 检测操作系统
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_NAME=$ID
        OS_VERSION=$VERSION_ID
        OS_PRETTY=$PRETTY_NAME
    elif [ -f /etc/redhat-release ]; then
        OS_NAME="centos"
        OS_VERSION=$(cat /etc/redhat-release | grep -oE '[0-9]+\.[0-9]+' | head -1)
        OS_PRETTY=$(cat /etc/redhat-release)
    else
        error "无法检测操作系统类型"
    fi
    
    info "检测到操作系统: $OS_PRETTY"
}

# 安装依赖
install_dependencies() {
    info "正在安装依赖包..."
    
    case $OS_NAME in
        ubuntu|debian)
            apt-get update -y
            apt-get install -y \
                clang llvm libbpf-dev \
                linux-headers-$(uname -r) \
                libelf-dev zlib1g-dev \
                curl wget
            ;;
        centos|rhel|rocky|almalinux)
            yum install -y epel-release
            yum install -y \
                clang llvm libbpf-devel \
                kernel-devel-$(uname -r) \
                elfutils-libelf-devel zlib-devel \
                curl wget
            ;;
        kylin|uos)
            # 银河麒麟/统信UOS - 基于Debian
            apt-get update -y
            apt-get install -y \
                clang llvm libbpf-dev \
                linux-headers-$(uname -r) \
                libelf-dev zlib1g-dev \
                curl wget
            ;;
        *)
            warn "未知操作系统，尝试通用安装方式"
            ;;
    esac
    
    info "依赖安装完成"
}

# 创建目录结构
create_directories() {
    info "创建目录结构..."
    
    mkdir -p /opt/cloudflow/agent/bin
    mkdir -p /opt/cloudflow/agent/config
    mkdir -p /opt/cloudflow/agent/logs
    mkdir -p /opt/cloudflow/agent/bpf
    
    info "目录创建完成: /opt/cloudflow/agent/"
}

# 安装二进制文件
install_binary() {
    info "安装二进制文件..."
    
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    AGENT_DIR="$(dirname "$SCRIPT_DIR")"
    
    # 检查是否有预编译的二进制
    if [ -f "$AGENT_DIR/cloud-flow-agent" ]; then
        cp "$AGENT_DIR/cloud-flow-agent" /opt/cloudflow/agent/bin/
    elif [ -f "$AGENT_DIR/bin/cloud-flow-agent" ]; then
        cp "$AGENT_DIR/bin/cloud-flow-agent" /opt/cloudflow/agent/bin/
    else
        warn "未找到预编译二进制，将从源码编译..."
        cd "$AGENT_DIR"
        make build-go
        cp cloud-flow-agent /opt/cloudflow/agent/bin/
    fi
    
    chmod +x /opt/cloudflow/agent/bin/cloud-flow-agent
    
    # 复制eBPF对象文件
    if [ -d "$AGENT_DIR/internal/ebpfcollector/bpf" ]; then
        cp "$AGENT_DIR"/internal/ebpfcollector/bpf/*.bpf.o /opt/cloudflow/agent/bpf/ 2>/dev/null || true
    fi
    
    info "二进制安装完成"
}

# 安装配置文件
install_config() {
    info "安装配置文件..."
    
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    
    if [ -f "$SCRIPT_DIR/config.yaml" ]; then
        cp "$SCRIPT_DIR/config.yaml" /opt/cloudflow/agent/config/
    else
        # 生成默认配置
        cat > /opt/cloudflow/agent/config/config.yaml << 'EOF'
# CloudFlow eBPF Agent 配置文件

# 平台服务地址
control_plane_addr: "127.0.0.1:9001"
data_plane_addr: "127.0.0.1:9002"
kafka_addr: "127.0.0.1:9092"

# eBPF 配置
bpf:
  backend: "auto"           # libbpf / cilium / auto
  mgmt_iface: "eth0"        # 采集网卡
  enable_tcp_metrics: true
  enable_http_metrics: true
  enable_http_full: false
  enable_dns_full: false
  enable_mysql_full: false

# 上报配置
report:
  interval: "5s"
  batch_size: 1000
  queue_size: 10000

# 指标服务
metrics:
  addr: ":9090"
  enabled: true

# 日志配置
logging:
  level: "info"            # debug / info / warn / error
  format: "json"
  output: "/opt/cloudflow/agent/logs/agent.log"
EOF
    fi
    
    info "配置文件安装完成: /opt/cloudflow/agent/config/config.yaml"
}

# 安装systemd服务
install_systemd() {
    info "安装 systemd 服务..."
    
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    
    if [ -f "$SCRIPT_DIR/cloudflow-agent.service" ]; then
        cp "$SCRIPT_DIR/cloudflow-agent.service" /etc/systemd/system/
    else
        cat > /etc/systemd/system/cloudflow-agent.service << 'EOF'
[Unit]
Description=CloudFlow eBPF Network Agent
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/cloudflow/agent
ExecStart=/opt/cloudflow/agent/bin/cloud-flow-agent --config /opt/cloudflow/agent/config/config.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536
LimitMEMLOCK=infinity

# 日志配置
StandardOutput=journal+console
StandardError=journal+console
SyslogIdentifier=cloudflow-agent

[Install]
WantedBy=multi-user.target
EOF
    fi
    
    systemctl daemon-reload
    systemctl enable cloudflow-agent
    
    info "systemd 服务安装完成"
}

# 启动服务
start_service() {
    info "启动 CloudFlow eBPF Agent 服务..."
    
    systemctl start cloudflow-agent
    
    # 等待服务启动
    sleep 3
    
    if systemctl is-active --quiet cloudflow-agent; then
        info "服务启动成功"
    else
        error "服务启动失败，请查看日志: journalctl -u cloudflow-agent"
    fi
}

# 健康检查
health_check() {
    info "执行健康检查..."
    
    sleep 2
    
    if curl -s -f http://localhost:9090/healthz > /dev/null 2>&1; then
        info "健康检查通过"
    else
        warn "健康检查端点未响应，服务可能仍在启动中"
    fi
}

# 输出安装成功信息
print_success() {
    echo ""
    echo "================================================"
    echo -e "${GREEN}CloudFlow eBPF Agent 安装成功!${NC}"
    echo "================================================"
    echo ""
    echo "安装目录: /opt/cloudflow/agent/"
    echo "配置文件: /opt/cloudflow/agent/config/config.yaml"
    echo "日志文件: /opt/cloudflow/agent/logs/agent.log"
    echo ""
    echo "管理命令:"
    echo "  systemctl start cloudflow-agent    # 启动服务"
    echo "  systemctl stop cloudflow-agent     # 停止服务"
    echo "  systemctl restart cloudflow-agent  # 重启服务"
    echo "  systemctl status cloudflow-agent   # 查看状态"
    echo "  journalctl -u cloudflow-agent -f   # 查看日志"
    echo ""
    echo "验证命令:"
    echo "  curl http://localhost:9090/healthz"
    echo "  curl http://localhost:9090/metrics"
    echo ""
    echo "请根据实际环境修改配置文件中的平台服务地址!"
    echo "================================================"
}

# 主流程
main() {
    echo "================================================"
    echo "  CloudFlow eBPF Agent 安装脚本"
    echo "================================================"
    echo ""
    
    check_root
    check_kernel
    detect_os
    install_dependencies
    create_directories
    install_binary
    install_config
    install_systemd
    start_service
    health_check
    print_success
}

main "$@"
