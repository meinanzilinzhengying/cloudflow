#!/bin/bash
set -e

# CloudFlow 探针安装脚本
# 用法: curl -sSL https://install.cloudflow.io/probe.sh | sh -s -- --name my-probe --type agent

DEFAULT_INSTALL_DIR="/usr/local/bin"
DEFAULT_SERVICE_NAME="cloudflow-agent"
DEFAULT_API_ENDPOINT="http://control-plane:8001"
DEFAULT_EDGE_ADDR="edge:50051"

# 显示帮助信息
show_help() {
    cat <<EOF
CloudFlow 探针安装脚本

用法:
  $0 [选项]

选项:
  --name NAME       探针名称 (默认: 主机名)
  --type TYPE       探针类型: agent, center, edge (默认: agent)
  --group GROUP     分组名称 (默认: default)
  --api-endpoint   控制面地址 (默认: http://control-plane:8001)
  --edge-addr      边缘节点地址 (默认: edge:50051)
  --install-dir    安装目录 (默认: /usr/local/bin)
  --help           显示帮助信息

示例:
  # 安装 Agent 类型探针
  curl -sSL https://install.cloudflow.io/probe.sh | sh -s -- --name agent-01 --type agent --group beijing

  # 本地安装
  bash probe.sh --name agent-01 --type agent
EOF
}

# 解析命令行参数
parse_args() {
    while [ $# -gt 0 ]; do
        case $1 in
            --name)
                PROBE_NAME="$2"
                shift 2
                ;;
            --type)
                PROBE_TYPE="$2"
                shift 2
                ;;
            --group)
                PROBE_GROUP="$2"
                shift 2
                ;;
            --api-endpoint)
                API_ENDPOINT="$2"
                shift 2
                ;;
            --edge-addr)
                EDGE_ADDR="$2"
                shift 2
                ;;
            --install-dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            --help)
                show_help
                exit 0
                ;;
            *)
                echo "未知选项: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# 检测操作系统和架构
detect_os() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case $ARCH in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            echo "不支持的架构: $ARCH"
            exit 1
            ;;
    esac
    
    echo "检测到操作系统: $OS, 架构: $ARCH"
}

# 下载探针可执行文件
download_probe() {
    local version="latest"
    local download_url="https://github.com/meinanzilinzhengying/cloudflow/releases/${version}/download/cloudflow-agent-${OS}-${ARCH}"
    
    echo "正在下载探针可执行文件..."
    echo "下载地址: <ADDRESS_REMOVED>"
    
    # 如果 GitHub Releases 没有预编译文件，尝试从源码构建
    if ! curl -fSL -o "$INSTALL_DIR/cloudflow-agent" "$download_url"; then
        echo "预编译文件不存在，尝试从源码构建..."
        build_from_source
    fi
    
    chmod +x "$INSTALL_DIR/cloudflow-agent"
    echo "探针可执行文件已安装到: $INSTALL_DIR/cloudflow-agent"
}

# 从源码构建（备用方案）
build_from_source() {
    echo "从源码构建需要 Go 1.22+ 环境"
    echo "请先安装 Go，然后运行: go build -o cloudflow-agent ./cmd/agent"
    echo "或者从 GitHub Releases 下载预编译文件"
    exit 1
}

# 创建配置文件
create_config() {
    local config_dir="/etc/cloudflow"
    local config_file="$config_dir/agent.yaml"
    
    mkdir -p "$config_dir"
    
    cat > "$config_file" <<EOF
# CloudFlow 探针配置文件
probe:
  name: "${PROBE_NAME}"
  type: "${PROBE_TYPE}"
  group: "${PROBE_GROUP}"

control-plane:
  endpoint: "${API_ENDPOINT}"

edge:
  addr: "${EDGE_ADDR}"

# 数据采集配置
采集:
  interfaces:
    - eth0
    - ens*
  sample-rate: 100
  enable-metrics: true
  enable-logs: true
EOF
    
    echo "配置文件已创建: $config_file"
}

# 创建 systemd 服务
create_systemd_service() {
    if ! command -v systemctl &> /dev/null; then
        echo "systemd 不可用，跳过服务创建"
        return
    fi
    
    cat > "/etc/systemd/system/${DEFAULT_SERVICE_NAME}.service" <<EOF
[Unit]
Description=CloudFlow Agent
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/cloudflow-agent --config /etc/cloudflow/agent.yaml
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    systemctl enable "$DEFAULT_SERVICE_NAME"
    systemctl start "$DEFAULT_SERVICE_NAME"
    
    echo "systemd 服务已创建并启动: $DEFAULT_SERVICE_NAME"
}

# 创建 upstart 服务（适用于旧版 Linux）
create_upstart_service() {
    if [ -d "/etc/init" ] && ! command -v systemctl &> /dev/null; then
        cat > "/etc/init/${DEFAULT_SERVICE_NAME}.conf" <<EOF
description "CloudFlow Agent"

start on runlevel [2345]
stop on runlevel [!2345]

exec $INSTALL_DIR/cloudflow-agent --config /etc/cloudflow/agent.yaml
respawn
EOF
        
        echo "upstart 服务已创建: $DEFAULT_SERVICE_NAME"
    fi
}

# 主函数
main() {
    # 设置默认值
    PROBE_NAME=$(hostname)
    PROBE_TYPE="agent"
    PROBE_GROUP="default"
    API_ENDPOINT="$DEFAULT_API_ENDPOINT"
    EDGE_ADDR="$DEFAULT_EDGE_ADDR"
    INSTALL_DIR="$DEFAULT_INSTALL_DIR"
    
    # 解析命令行参数
    parse_args "$@"
    
    echo "========================================="
    echo "CloudFlow 探针安装脚本"
    echo "========================================="
    echo "探针名称: $PROBE_NAME"
    echo "探针类型: $PROBE_TYPE"
    echo "分组: $PROBE_GROUP"
    echo "控制面地址: <ADDRESS_REMOVED>
    echo "边缘节点地址: <ADDRESS_REMOVED>
    echo "安装目录: $INSTALL_DIR"
    echo "========================================="
    
    # 检测操作系统
    detect_os
    
    # 检查是否以 root 身份运行
    if [ "$EUID" -ne 0 ]; then
        echo "请以 root 身份运行此脚本"
        exit 1
    fi
    
    # 下载探针可执行文件
    download_probe
    
    # 创建配置文件
    create_config
    
    # 创建系统服务
    create_systemd_service
    create_upstart_service
    
    echo "========================================="
    echo "安装完成！"
    echo "查看服务状态: systemctl status $DEFAULT_SERVICE_NAME"
    echo "查看日志: journalctl -u $DEFAULT_SERVICE_NAME -f"
    echo "========================================="
}

# 运行主函数
main "$@"
