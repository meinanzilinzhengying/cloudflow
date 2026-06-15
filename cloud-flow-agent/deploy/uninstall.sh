#!/bin/bash
# CloudFlow eBPF Agent 卸载脚本

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

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
}

# 停止服务
stop_service() {
    info "停止 CloudFlow eBPF Agent 服务..."
    
    if systemctl is-active --quiet cloudflow-agent; then
        systemctl stop cloudflow-agent
        info "服务已停止"
    else
        warn "服务未运行"
    fi
    
    if systemctl is-enabled --quiet cloudflow-agent; then
        systemctl disable cloudflow-agent
        info "服务已禁用开机自启"
    fi
}

# 删除systemd服务
remove_systemd() {
    info "删除 systemd 服务..."
    
    if [ -f /etc/systemd/system/cloudflow-agent.service ]; then
        rm -f /etc/systemd/system/cloudflow-agent.service
        systemctl daemon-reload
        info "systemd 服务已删除"
    else
        warn "未找到 systemd 服务文件"
    fi
}

# 删除安装目录
remove_files() {
    info "删除安装目录..."
    
    if [ -d /opt/cloudflow/agent ]; then
        rm -rf /opt/cloudflow/agent
        info "安装目录已删除: /opt/cloudflow/agent"
    else
        warn "未找到安装目录"
    fi
}

# 可选：清理依赖
clean_dependencies() {
    echo ""
    read -p "是否清理 eBPF 相关依赖包? [y/N]: " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        info "正在清理依赖包..."
        
        if [ -f /etc/debian_version ]; then
            apt-get remove -y clang llvm libbpf-dev libelf-dev || true
        elif [ -f /etc/redhat-release ]; then
            yum remove -y clang llvm libbpf-devel elfutils-libelf-devel || true
        fi
        
        info "依赖包已清理"
    fi
}

# 完成
print_complete() {
    echo ""
    echo "================================================"
    echo -e "${GREEN}CloudFlow eBPF Agent 卸载完成!${NC}"
    echo "================================================"
    echo ""
}

# 主流程
main() {
    echo "================================================"
    echo "  CloudFlow eBPF Agent 卸载脚本"
    echo "================================================"
    echo ""
    
    check_root
    stop_service
    remove_systemd
    remove_files
    clean_dependencies
    print_complete
}

main "$@"
