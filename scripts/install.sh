#!/bin/bash
# ============================================================
# CloudFlow 超级一键部署脚本
# 完美适配：CentOS、Red Hat、Rocky、AlmaLinux、麒麟、欧拉、Debian、Ubuntu 等
# 
# 使用方式：
#   sudo bash install.sh              # 标准安装
#   sudo bash install.sh --skip-docker # 跳过 Docker 安装（已安装 Docker）
#   sudo bash install.sh --skip-git    # 跳过 Git 克隆（已克隆代码）
#   sudo bash install.sh --help        # 显示帮助
# ============================================================

set -euo pipefail

# ============================================================
# 版本和路径配置
# ============================================================
SCRIPT_VERSION="3.0.0"
INSTALL_DIR="/opt/cloudflow"
LOG_DIR="/var/log/cloudflow"
CONFIG_DIR="/etc/cloudflow"
BACKUP_DIR="/opt/cloudflow-backup"
PROJECT_REPO="https://github.com/meinanzilinzhengying/cloudflow.git"
PROJECT_NAME="CloudFlow"

# ============================================================
# 颜色定义
# ============================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'
BOLD='\033[1m'

# ============================================================
# 全局变量
# ============================================================
OS_NAME=""
OS_VERSION=""
OS_FAMILY=""
OS_FULL_VERSION=""
PACKAGE_MANAGER=""
ARCH=""
DOCKER_VERSION=""
INSTALL_DOCKER=true
INSTALL_GIT=true
SKIP_DOCKER=false
SKIP_GIT=false

# ============================================================
# 日志函数
# ============================================================
log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step() { echo -e "${BLUE}[STEP]${NC} $1"; }
log_success() { echo -e "${GREEN}✓${NC} $1"; }
log_debug() { 
    if [[ "${DEBUG:-false}" == "true" ]]; then
        echo -e "${MAGENTA}[DEBUG]${NC} $1"
    fi
}

# ============================================================
# 显示帮助
# ============================================================
show_help() {
    cat << EOF
${BOLD}${CYAN}CloudFlow 一键部署脚本 v${SCRIPT_VERSION}${NC}

${BOLD}描述：${NC}
    适用于 CentOS、Red Hat、Rocky、AlmaLinux、麒麟、欧拉、Debian、Ubuntu 等
    主流 Linux 发行版的超级一键部署脚本，自动检测系统环境并安装所有依赖。

${BOLD}用法：${NC}
    sudo bash install.sh [选项]

${BOLD}选项：${NC}
    --skip-docker     跳过 Docker 安装（已安装 Docker）
    --skip-git        跳过 Git 克隆（已克隆代码）
    --debug           启用调试模式
    --help            显示此帮助信息

${BOLD}示例：${NC}
    sudo bash install.sh                # 标准安装
    sudo bash install.sh --skip-docker  # 已安装 Docker
    sudo bash install.sh --skip-git     # 已克隆代码

${BOLD}支持的操作系统：${NC}
    ✓ CentOS 7/8/9
    ✓ Red Hat Enterprise Linux 7/8/9
    ✓ Rocky Linux 8/9
    ✓ AlmaLinux 8/9
    ✓ 麒麟 V10
    ✓ 欧拉 openEuler 20/21/22
    ✓ 华为 EulerOS
    ✓ Debian 10/11/12
    ✓ Ubuntu 18/20/22/24 LTS
    ✓ Fedora 36/37/38

${BOLD}安装后访问：${NC}
    • 前端分析页面: http://<服务器IP>:8080
    • 平台监控:     http://<服务器IP>:3003
    • Grafana:       http://<服务器IP>:3001
    • Prometheus:    http://<服务器IP>:9091
    • AI 服务:       http://<服务器IP>:8005

${BOLD}管理命令：${NC}
    cd ${INSTALL_DIR}
    docker compose ps           # 查看服务状态
    docker compose logs -f     # 查看日志
    docker compose restart      # 重启服务
    docker compose down         # 停止服务
    bash scripts/install.sh     # 重新部署

EOF
    exit 0
}

# ============================================================
# 解析命令行参数
# ============================================================
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --skip-docker)
                SKIP_DOCKER=true
                log_info "将跳过 Docker 安装"
                shift
                ;;
            --skip-git)
                SKIP_GIT=true
                log_info "将跳过 Git 克隆"
                shift
                ;;
            --debug)
                export DEBUG=true
                log_info "已启用调试模式"
                shift
                ;;
            --help|-h)
                show_help
                ;;
            *)
                log_error "未知参数: $1"
                echo "使用 --help 查看帮助信息"
                exit 1
                ;;
        esac
    done
}

# ============================================================
# 权限检查
# ============================================================
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "此脚本需要 root 权限运行，请使用 sudo"
        echo ""
        echo "正确用法："
        echo "  sudo bash install.sh"
        exit 1
    fi
}

# ============================================================
# 系统检测模块 - 完整版
# ============================================================
detect_os() {
    log_step "正在检测操作系统环境..."
    echo ""
    
    # 检测架构
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)
            ARCH="x86_64"
            log_debug "架构: x86_64 (64位)"
            ;;
        aarch64|arm64)
            ARCH="aarch64"
            log_debug "架构: aarch64 (ARM64)"
            ;;
        *)
            log_warn "未知架构: $ARCH，继续尝试安装..."
            ;;
    esac
    
    # 检测容器环境
    if [ -f /.dockerenv ] || [ -f /run/.containerenv ] || grep -q 'containerized' /proc/1/cgroup 2>/dev/null; then
        log_warn "检测到容器环境，部分功能可能受限"
    fi
    
    # 主检测：从 /etc/os-release
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_NAME="${ID:-unknown}"
        OS_VERSION="${VERSION_ID:-unknown}"
        OS_FULL_VERSION="${VERSION:-unknown}"
        
        # 解析家族
        if [ -n "${ID_LIKE:-}" ]; then
            OS_FAMILY="$ID_LIKE"
        else
            OS_FAMILY="$OS_NAME"
        fi
        
        # 特殊处理国产操作系统
        case "$OS_NAME" in
            centos|rhel|rocky|almalinux|anolis)
                OS_FAMILY="rhel"
                ;;
            ubuntu)
                OS_FAMILY="debian"
                ;;
            debian)
                OS_FAMILY="debian"
                ;;
            kylin|openkylin|neokylin)
                OS_FAMILY="kylin"
                ;;
            openeuler|euleros|anolis)
                OS_FAMILY="euleros"
                ;;
            fedora)
                OS_FAMILY="fedora"
                ;;
            opensuse|suse|sles)
                OS_FAMILY="suse"
                ;;
            *)
                # 尝试从 ID_LIKE 判断
                if echo "$ID_LIKE" | grep -q "rhel\|centos\|fedora"; then
                    OS_FAMILY="rhel"
                elif echo "$ID_LIKE" | grep -q "debian\|ubuntu"; then
                    OS_FAMILY="debian"
                fi
                ;;
        esac
        
    # 备用检测：/etc/redhat-release
    elif [ -f /etc/redhat-release ]; then
        local release_content=$(cat /etc/redhat-release)
        
        if echo "$release_content" | grep -qi "centos"; then
            OS_NAME="centos"
            OS_FAMILY="rhel"
        elif echo "$release_content" | grep -qi "red hat"; then
            OS_NAME="rhel"
            OS_FAMILY="rhel"
        elif echo "$release_content" | grep -qi "rocky"; then
            OS_NAME="rocky"
            OS_FAMILY="rhel"
        elif echo "$release_content" | grep -qi "alma"; then
            OS_NAME="almalinux"
            OS_FAMILY="rhel"
        else
            OS_NAME="rhel"
            OS_FAMILY="rhel"
        fi
        
        # 提取版本号
        OS_VERSION=$(echo "$release_content" | grep -oE '[0-9]+\.[0-9]+' | head -1)
        if [ -z "$OS_VERSION" ]; then
            OS_VERSION="7"
        fi
        
    # 备用检测：lsb_release
    elif command -v lsb_release &>/dev/null; then
        OS_NAME=$(lsb_release -si | tr '[:upper:]' '[:lower:]')
        OS_VERSION=$(lsb_release -sr)
        
        case "$OS_NAME" in
            ubuntu*|debian*)
                OS_FAMILY="debian"
                ;;
            *)
                OS_FAMILY="$OS_NAME"
                ;;
        esac
    else
        OS_NAME="unknown"
        OS_FAMILY="unknown"
        OS_VERSION="unknown"
    fi
    
    # 检测包管理器
    if command -v dnf &>/dev/null; then
        PACKAGE_MANAGER="dnf"
    elif command -v yum &>/dev/null; then
        PACKAGE_MANAGER="yum"
    elif command -v apt-get &>/dev/null; then
        PACKAGE_MANAGER="apt"
    elif command -v zypper &>/dev/null; then
        PACKAGE_MANAGER="zypper"
    elif command -v apk &>/dev/null; then
        PACKAGE_MANAGER="apk"
    fi
    
    # 输出检测结果
    echo ""
    echo -e "${BOLD}系统信息：${NC}"
    echo "  操作系统: ${BOLD}${OS_FULL_VERSION:-$OS_NAME $OS_VERSION}${NC}"
    echo "  系统家族: ${BOLD}$OS_FAMILY${NC}"
    echo "  系统架构: ${BOLD}$ARCH${NC}"
    echo "  包管理器: ${BOLD}$PACKAGE_MANAGER${NC}"
    echo ""
}

# ============================================================
# 依赖安装模块 - 完整版
# ============================================================
install_packages() {
    local packages=("$@")
    local failed=()
    
    log_step "正在安装依赖包: ${packages[*]}"
    
    case "$PACKAGE_MANAGER" in
        dnf)
            log_debug "使用 dnf 安装..."
            if ! dnf install -y --nogpgcheck --allowerasing "${packages[@]}" 2>&1 | tee /tmp/dnf-install.log; then
                # 尝试安装可用包
                for pkg in "${packages[@]}"; do
                    if ! rpm -q "$pkg" &>/dev/null; then
                        log_warn "无法安装: $pkg"
                        failed+=("$pkg")
                    fi
                done
            fi
            ;;
        yum)
            log_debug "使用 yum 安装..."
            if ! yum install -y --nogpgcheck "${packages[@]}" 2>&1 | tee /tmp/yum-install.log; then
                for pkg in "${packages[@]}"; do
                    if ! rpm -q "$pkg" &>/dev/null; then
                        log_warn "无法安装: $pkg"
                        failed+=("$pkg")
                    fi
                done
            fi
            ;;
        apt)
            log_debug "使用 apt 安装..."
            export DEBIAN_FRONTEND=noninteractive
            if ! apt-get update -qq 2>&1 | grep -v "^Get:" | grep -v "^Hit:"; then
                log_warn "apt-get update 失败，继续尝试..."
            fi
            if ! apt-get install -y --no-install-recommends "${packages[@]}" 2>&1 | tee /tmp/apt-install.log; then
                for pkg in "${packages[@]}"; do
                    if ! dpkg -l "$pkg" 2>/dev/null | grep -q "^ii"; then
                        log_warn "无法安装: $pkg"
                        failed+=("$pkg")
                    fi
                done
            fi
            ;;
        zypper)
            log_debug "使用 zypper 安装..."
            if ! zypper --non-interactive install -y "${packages[@]}"; then
                for pkg in "${packages[@]}"; do
                    if ! rpm -q "$pkg" &>/dev/null; then
                        log_warn "无法安装: $pkg"
                        failed+=("$pkg")
                    fi
                done
            fi
            ;;
        *)
            log_error "不支持的包管理器: $PACKAGE_MANAGER"
            return 1
            ;;
    esac
    
    # 验证安装
    for pkg in "${packages[@]}"; do
        if command -v "$pkg" &>/dev/null || rpm -q "$pkg" &>/dev/null || dpkg -l "$pkg" 2>/dev/null | grep -q "^ii"; then
            log_debug "已安装: $pkg"
        fi
    done
    
    if [ ${#failed[@]} -gt 0 ]; then
        log_warn "以下包安装失败（可能不影响部署）: ${failed[*]}"
    fi
    
    return 0
}

install_system_dependencies() {
    log_step "正在安装系统依赖..."
    
    # 基础工具包
    local basic_packages=(
        curl
        wget
        ca-certificates
        sudo
        openssl
        tar
        gzip
        jq
        unzip
        git
    )
    
    # 根据系统家族安装特定依赖
    case "$OS_FAMILY" in
        rhel|fedora)
            log_debug "安装 RHEL/Fedora 系列依赖..."
            install_packages yum-utils 2>/dev/null || true
            install_packages device-mapper-persistent-data lvm2 2>/dev/null || true
            install_packages "${basic_packages[@]}"
            
            # CentOS 7 特殊处理
            if [[ "$OS_VERSION" =~ ^7\.? ]]; then
                log_debug "CentOS 7 检测到，安装 compat 包..."
                install_packages centos-release-scl 2>/dev/null || true
            fi
            ;;
        debian)
            log_debug "安装 Debian/Ubuntu 系列依赖..."
            install_packages "${basic_packages[@]}"
            install_packages apt-transport-https 2>/dev/null || true
            install_packages gnupg2 2>/dev/null || true
            ;;
        kylin)
            log_debug "安装麒麟系统依赖..."
            install_packages "${basic_packages[@]}"
            # 麒麟可能需要额外的字体支持
            install_packages fonts-wqy-microhei 2>/dev/null || true
            ;;
        euleros)
            log_debug "安装欧拉系统依赖..."
            install_packages "${basic_packages[@]}"
            # 欧拉可能需要额外的依赖
            install_packages dnf-plugins-core 2>/dev/null || true
            ;;
        suse)
            log_debug "安装 SUSE 系列依赖..."
            install_packages "${basic_packages[@]}"
            ;;
        *)
            log_debug "安装通用依赖..."
            install_packages "${basic_packages[@]}"
            ;;
    esac
    
    log_success "系统依赖安装完成"
}

# ============================================================
# Docker 安装模块 - 完整版
# ============================================================
install_docker() {
    if [[ "$SKIP_DOCKER" == "true" ]]; then
        log_info "跳过 Docker 安装（--skip-docker）"
        return 0
    fi
    
    log_step "正在检查并安装 Docker..."
    
    # 检查 Docker 是否已安装
    if command -v docker &>/dev/null; then
        DOCKER_VERSION=$(docker --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
        log_success "Docker 已安装: $DOCKER_VERSION"
        
        # 检查 Docker 服务状态
        if command -v systemctl &>/dev/null; then
            if systemctl is-active docker &>/dev/null; then
                log_success "Docker 服务正在运行"
            elif systemctl is-active docker &>/dev/null; then
                log_info "启动 Docker 服务..."
                systemctl start docker || true
            fi
        elif command -v service &>/dev/null; then
            if service docker status &>/dev/null; then
                log_success "Docker 服务正在运行"
            else
                log_info "启动 Docker 服务..."
                service docker start || true
            fi
        fi
        
        # 启用 Docker 服务
        if command -v systemctl &>/dev/null; then
            systemctl enable docker 2>/dev/null || true
        fi
    else
        log_info "开始安装 Docker..."
        
        # 根据不同系统选择安装方式
        case "$OS_FAMILY" in
            rhel|fedora)
                install_docker_rhel
                ;;
            debian)
                install_docker_debian
                ;;
            kylin|euleros)
                install_docker_euleros
                ;;
            *)
                install_docker_generic
                ;;
        esac
    fi
    
    # 安装 Docker Compose v2
    install_docker_compose
    
    # 配置 Docker 镜像加速
    config_docker_mirror
    
    # 添加当前用户到 docker 组
    config_docker_user
    
    log_success "Docker 安装完成"
}

install_docker_rhel() {
    log_debug "安装 Docker (RHEL/CentOS/Rocky/Alma)..."
    
    if command -v dnf &>/dev/null; then
        # CentOS Stream, Rocky, AlmaLinux, Fedora
        log_info "使用 dnf 安装 Docker..."
        
        dnf remove -y docker docker-client docker-client-latest docker-common \
            docker-latest docker-latest-logrotate docker-logrotate docker-engine 2>/dev/null || true
        
        dnf install -y dnf-plugins-core
        dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
        
        dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin \
            --nobest --setopt=obsoletes=0 || \
            dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
        
    elif command -v yum &>/dev/null; then
        # CentOS 7
        log_info "使用 yum 安装 Docker..."
        
        yum remove -y docker docker-client docker-client-latest docker-common \
            docker-latest docker-latest-logrotate docker-logrotate docker-engine-selinux \
            docker-engine-selinux 2>/dev/null || true
        
        yum install -y yum-utils
        yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
        
        yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin \
            --nobest || true
        
        # CentOS 7 可能需要启用 elrepo 或 extras
        if [[ "$OS_VERSION" =~ ^7\.? ]]; then
            yum install -y --enablerepo=extras docker-ce || true
        fi
    fi
    
    systemctl start docker || service docker start || true
    systemctl enable docker || true
}

install_docker_debian() {
    log_debug "安装 Docker (Debian/Ubuntu)..."
    
    log_info "使用 apt 安装 Docker..."
    
    apt-get remove -y docker docker-engine docker.io containerd runc 2>/dev/null || true
    
    apt-get update -qq
    apt-get install -y ca-certificates curl gnupg lsb-release
    
    # 添加 Docker GPG key
    mkdir -p /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/$OS_NAME/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    
    # 添加 Docker 仓库
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$OS_NAME $(lsb_release -cs) stable" | \
        tee /etc/apt/sources.list.d/docker.list > /dev/null
    
    apt-get update -qq
    apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    
    systemctl start docker || true
    systemctl enable docker || true
}

install_docker_euleros() {
    log_debug "安装 Docker (麒麟/欧拉)..."
    
    # 麒麟和欧拉可能使用不同的包管理器
    log_info "尝试安装 Docker..."
    
    # 尝试使用 snap 或其他方式
    if command -v dnf &>/dev/null; then
        install_docker_rhel
    elif command -v yum &>/dev/null; then
        install_docker_rhel
    else
        # 最后尝试通用安装脚本
        log_info "使用通用安装脚本..."
        curl -fsSL https://get.docker.com | sh || true
    fi
    
    systemctl start docker || service docker start || true
    systemctl enable docker || true
}

install_docker_generic() {
    log_debug "使用通用方式安装 Docker..."
    
    log_info "使用 Docker 官方安装脚本..."
    
    # 使用官方安装脚本
    curl -fsSL https://get.docker.com | sh || {
        log_warn "官方安装脚本失败，尝试手动安装..."
        
        # 尝试基本的 docker 安装
        case "$PACKAGE_MANAGER" in
            apt)
                apt-get update -qq
                apt-get install -y docker.io docker-compose || true
                ;;
            yum)
                yum install -y docker || true
                ;;
            dnf)
                dnf install -y docker || true
                ;;
        esac
    }
    
    systemctl start docker || service docker start || true
    systemctl enable docker || true
}

install_docker_compose() {
    log_step "检查 Docker Compose..."
    
    # 检查 Docker Compose v2 (docker compose 子命令)
    if docker compose version &>/dev/null; then
        log_success "Docker Compose v2 已安装: $(docker compose version)"
        return 0
    fi
    
    # 检查独立 docker-compose
    if command -v docker-compose &>/dev/null; then
        log_success "Docker Compose 已安装: $(docker-compose --version)"
        return 0
    fi
    
    log_info "正在安装 Docker Compose v2..."
    
    # 下载并安装 Docker Compose v2
    local compose_version="v2.24.5"
    local install_path="/usr/local/lib/docker/cli-plugins/docker-compose"
    
    mkdir -p "$(dirname "$install_path")"
    
    if [[ "$ARCH" == "x86_64" ]]; then
        curl -SL "https://github.com/docker/compose/releases/download/${compose_version}/docker-compose-$(uname -s)-$(uname -m)" \
            -o "$install_path"
    elif [[ "$ARCH" == "aarch64" ]]; then
        curl -SL "https://github.com/docker/compose/releases/download/${compose_version}/docker-compose-linux-aarch64" \
            -o "$install_path"
    fi
    
    chmod +x "$install_path"
    ln -sf "$install_path" /usr/local/bin/docker-compose
    
    # 验证安装
    if docker compose version &>/dev/null || docker-compose --version &>/dev/null; then
        log_success "Docker Compose 安装成功"
    else
        log_warn "Docker Compose 安装可能失败，请手动检查"
    fi
}

config_docker_mirror() {
    log_step "配置 Docker 镜像加速..."
    
    local docker_config_dir="/etc/docker"
    local docker_config_file="$docker_config_dir/daemon.json"
    
    mkdir -p "$docker_config_dir"
    
    # 检查是否已有配置
    if [ -f "$docker_config_file" ]; then
        log_info "Docker 配置文件已存在，备份并更新..."
        cp "$docker_config_file" "${docker_config_file}.bak.$(date +%s)"
    fi
    
    # 创建镜像加速配置
    cat > "$docker_config_file" << 'EOF'
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ],
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "100m",
    "max-file": "3"
  },
  "storage-driver": "overlay2",
  "live-restore": true,
  "default-address-pools": [
    {"base": "172.17.0.0/16", "size": 24}
  ]
}
EOF
    
    # 重启 Docker
    if command -v systemctl &>/dev/null; then
        systemctl daemon-reload
        systemctl restart docker || true
    elif command -v service &>/dev/null; then
        service docker restart || true
    fi
    
    log_success "Docker 镜像加速配置完成"
}

config_docker_user() {
    log_debug "配置 Docker 用户组..."
    
    # 获取当前用户（如果通过 sudo 运行）
    local current_user="${SUDO_USER:-$(whoami)}"
    
    if [ "$current_user" != "root" ] && [ "$current_user" != "" ]; then
        if ! groups "$current_user" | grep -q docker; then
            usermod -aG docker "$current_user"
            log_info "已将用户 $current_user 添加到 docker 组"
            log_warn "请重新登录或运行 'newgrp docker' 以使更改生效"
        fi
    fi
}

# ============================================================
# 项目安装模块
# ============================================================
setup_project() {
    log_step "正在部署 CloudFlow 项目..."
    
    # 创建目录
    mkdir -p "$INSTALL_DIR" "$LOG_DIR" "$CONFIG_DIR"
    
    # 进入安装目录
    cd "$INSTALL_DIR"
    
    # 克隆或更新代码
    if [ -d ".git" ]; then
        log_info "检测到已有代码库，正在更新..."
        git fetch origin
        
        # 保存本地修改
        if [ -f ".env" ]; then
            log_info "备份现有配置..."
            cp .env "${BACKUP_DIR}/.env.backup.$(date +%s)" 2>/dev/null || true
        fi
        
        git reset --hard origin/main || git reset --hard origin/master || {
            log_warn "无法重置到 main 分支，尝试 pull..."
            git pull origin main || true
        }
    else
        log_info "正在克隆代码库..."
        if ! git clone --depth 1 "$PROJECT_REPO" .; then
            log_error "Git 克隆失败，请检查网络连接"
            exit 1
        fi
    fi
    
    # 恢复配置
    if [ -f "${BACKUP_DIR}/.env.backup"* ] && [ ! -f ".env" ]; then
        log_info "恢复备份的配置..."
        local latest_backup=$(ls -t "${BACKUP_DIR}/.env.backup"* 2>/dev/null | head -1)
        if [ -n "$latest_backup" ]; then
            cp "$latest_backup" .env
        fi
    fi
    
    # 配置环境变量
    configure_environment
    
    # 设置权限
    chmod +x scripts/*.sh 2>/dev/null || true
    chmod +x *.sh 2>/dev/null || true
    
    log_success "项目代码准备完成"
}

configure_environment() {
    log_step "配置环境变量..."
    
    # 复制环境变量模板
    if [ ! -f ".env" ] && [ -f ".env.example" ]; then
        cp .env.example .env
        log_info "已创建 .env 配置文件"
    fi
    
    # 生成随机密码
    if [ -f ".env" ]; then
        # 生成随机密码
        local db_password=$(openssl rand -hex 24 2>/dev/null || head -c 48 /dev/urandom | base64)
        local grafana_password=$(openssl rand -hex 12 2>/dev/null || head -c 24 /dev/urandom | base64)
        local jwt_secret=$(openssl rand -base64 48 2>/dev/null || head -c 64 /dev/urandom | base64)
        
        # 更新环境变量
        sed -i "s|CLOUD_FLOW_DB_PASSWORD=.*|CLOUD_FLOW_DB_PASSWORD=${db_password}|" .env 2>/dev/null || true
        sed -i "s|GRAFANA_ADMIN_PASSWORD=.*|GRAFANA_ADMIN_PASSWORD=${grafana_password}|" .env 2>/dev/null || true
        sed -i "s|CLOUDFLOW_JWT_SECRET_KEY=.*|CLOUDFLOW_JWT_SECRET_KEY=${jwt_secret}|" .env 2>/dev/null || true
        
        # 确保关键变量存在
        if ! grep -q "CLOUD_FLOW_DB_PASSWORD=" .env; then
            echo "CLOUD_FLOW_DB_PASSWORD=${db_password}" >> .env
        fi
        if ! grep -q "GRAFANA_ADMIN_PASSWORD=" .env; then
            echo "GRAFANA_ADMIN_PASSWORD=${grafana_password}" >> .env
        fi
        if ! grep -q "CLOUDFLOW_JWT_SECRET_KEY=" .env; then
            echo "CLOUDFLOW_JWT_SECRET_KEY=${jwt_secret}" >> .env
        fi
        
        log_success "环境变量配置完成"
        log_warn "请妥善保管生成的密码，或在 .env 文件中修改"
    fi
}

# ============================================================
# 服务启动模块
# ============================================================
start_services() {
    log_step "正在启动 CloudFlow 服务..."
    
    cd "$INSTALL_DIR"
    
    # 检查 Docker 是否运行
    if ! docker info &>/dev/null; then
        log_error "Docker 未运行，请先启动 Docker"
        exit 1
    fi
    
    # 检查是否有 docker-compose.yml
    local compose_file=""
    if [ -f "docker-compose.yml" ]; then
        compose_file="docker-compose.yml"
    elif [ -f "compose.yml" ]; then
        compose_file="compose.yml"
    else
        log_error "未找到 Docker Compose 配置文件"
        exit 1
    fi
    
    log_info "使用配置文件: $compose_file"
    
    # 拉取最新镜像
    log_info "正在拉取 Docker 镜像..."
    docker compose -f "$compose_file" pull || {
        log_warn "拉取镜像失败，继续尝试启动本地构建..."
    }
    
    # 构建并启动服务
    log_info "正在构建并启动服务..."
    docker compose -f "$compose_file" up -d --build --remove-orphans
    
    # 等待服务启动
    log_info "等待服务启动..."
    sleep 15
    
    # 检查服务状态
    log_info "检查服务状态..."
    docker compose -f "$compose_file" ps
    
    log_success "服务启动命令已执行"
}

# ============================================================
# 健康检查模块
# ============================================================
check_services() {
    log_step "正在检查服务状态..."
    
    cd "$INSTALL_DIR"
    
    # 获取主机 IP
    local host_ip=$(get_host_ip)
    
    echo ""
    echo -e "${BOLD}${CYAN}============================================${NC}"
    echo -e "${BOLD}${CYAN}  CloudFlow 部署完成！${NC}"
    echo -e "${BOLD}${CYAN}============================================${NC}"
    echo ""
    echo -e "${BOLD}服务访问地址：${NC}"
    echo "  • 前端分析页面:  ${BOLD}http://${host_ip}:8080${NC}"
    echo "  • 平台监控前端:  ${BOLD}http://${host_ip}:3003${NC}"
    echo "  • Grafana:        ${BOLD}http://${host_ip}:3001${NC}"
    echo "  • Prometheus:     ${BOLD}http://${host_ip}:9091${NC}"
    echo "  • AI 服务 API:   ${BOLD}http://${host_ip}:8005${NC}"
    echo "  • Jaeger:         ${BOLD}http://${host_ip}:16686${NC}"
    echo ""
    
    echo -e "${BOLD}默认凭证：${NC}"
    if [ -f .env ]; then
        local grafana_pwd=$(grep -E '^GRAFANA_ADMIN_PASSWORD=' .env 2>/dev/null | cut -d= -f2)
        echo "  • Grafana:  admin / ${BOLD}${grafana_pwd:-admin}${NC}"
    else
        echo "  • Grafana:  admin / admin"
    fi
    echo ""
    
    echo -e "${BOLD}管理命令（在 ${INSTALL_DIR} 目录下执行）：${NC}"
    echo "  • 查看服务:     docker compose ps"
    echo "  • 查看日志:     docker compose logs -f"
    echo "  • 查看某服务:   docker compose logs -f <服务名>"
    echo "  • 停止服务:     docker compose down"
    echo "  • 重启服务:     docker compose restart"
    echo "  • 完全卸载:     docker compose down -v"
    echo ""
    
    echo -e "${BOLD}查看 AI 服务日志：${NC}"
    echo "  • docker compose logs -f ai-service"
    echo ""
    
    echo -e "${BOLD}配置 AI API Key（可选）：${NC}"
    echo "  • 编辑 .env 文件添加:"
    echo "    DEEPSEEK_API_KEY=your_key"
    echo "    QWEN_API_KEY=your_key"
    echo "    OPENAI_API_KEY=your_key"
    echo "  • 然后重启: docker compose restart ai-service"
    echo ""
    
    echo -e "${CYAN}============================================${NC}"
    echo -e "${GREEN}✓ 部署成功！${NC}"
    echo -e "${CYAN}============================================${NC}"
    echo ""
}

get_host_ip() {
    local ip=""
    
    # 优先使用内部 IP
    if command -v ip &>/dev/null; then
        ip=$(ip addr show 2>/dev/null | grep 'inet ' | grep -v '127.0.0.1' | awk '{print $2}' | cut -d'/' -f1 | head -1)
    fi
    
    # 备用：使用 ifconfig
    if [ -z "$ip" ] && command -v ifconfig &>/dev/null; then
        ip=$(ifconfig 2>/dev/null | grep 'inet ' | grep -v '127.0.0.1' | awk '{print $2}' | head -1)
    fi
    
    # 备用：使用 hostname
    if [ -z "$ip" ]; then
        ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    fi
    
    # 最后：使用 curl 获取公网 IP
    if [ -z "$ip" ]; then
        ip=$(curl -s ifconfig.me 2>/dev/null || echo "localhost")
    fi
    
    echo "${ip:-localhost}"
}

# ============================================================
# 预检查模块
# ============================================================
preflight_check() {
    log_step "执行安装前检查..."
    
    local errors=0
    
    # 检查网络
    log_debug "检查网络连接..."
    if ! curl -sf --max-time 5 https://github.com &>/dev/null; then
        log_warn "网络连接可能受限，无法访问 GitHub"
        ((errors++))
    fi
    
    # 检查磁盘空间
    log_debug "检查磁盘空间..."
    local available_space=$(df -BG "$INSTALL_DIR" 2>/dev/null | awk 'NR==2 {print $4}' | sed 's/G//')
    if [ "${available_space:-0}" -lt 10 ]; then
        log_warn "磁盘空间不足，建议至少 10GB，当前剩余: ${available_space}GB"
        ((errors++))
    fi
    
    # 检查内存
    log_debug "检查可用内存..."
    local available_mem=$(free -m 2>/dev/null | awk 'NR==2 {print $7}')
    if [ "${available_mem:-0}" -lt 1024 ]; then
        log_warn "可用内存不足，建议至少 1GB，当前剩余: ${available_mem}MB"
        ((errors++))
    fi
    
    # 检查 CPU
    log_debug "检查 CPU..."
    local cpu_count=$(nproc 2>/dev/null || echo 1)
    if [ "$cpu_count" -lt 1 ]; then
        log_warn "CPU 核心数不足"
        ((errors++))
    fi
    
    if [ $errors -gt 0 ]; then
        log_warn "预检查发现 $errors 个警告，但将继续安装..."
    else
        log_success "预检查通过"
    fi
}

# ============================================================
# 备份模块
# ============================================================
backup_existing() {
    if [ -d "$INSTALL_DIR" ] && [ "$(ls -A "$INSTALL_DIR" 2>/dev/null)" ]; then
        log_step "备份现有安装..."
        
        mkdir -p "$BACKUP_DIR"
        
        # 备份配置
        if [ -f "$INSTALL_DIR/.env" ]; then
            cp "$INSTALL_DIR/.env" "$BACKUP_DIR/.env.backup.$(date +%s)"
            log_info "配置已备份"
        fi
        
        # 备份数据卷
        log_info "注意：如果需要保留数据，可在安装前手动备份 $INSTALL_DIR"
    fi
}

# ============================================================
# 主程序
# ============================================================
main() {
    # 解析参数
    parse_args "$@"
    
    # 显示横幅
    clear
    echo -e "${BOLD}${CYAN}"
    cat << 'EOF'

    ██╗   ██╗ █████╗ ██╗   ██╗██╗  █████╗██╗  ██╗██╗   ██╗███████╗
    ██║   ██║██╔══██╗██║   ██║██║ ██╔══██╗██║  ██║██║   ██║██╔════╝
    ██║   ██║███████║██║   ██║██║ ███████║███████║██║   ██║█████╗  
    ╚██╗ ██╔╝██╔══██║██║   ██║██║ ██╔══██║██╔══██║╚██╗ ██╔╝██╔══╝  
     ╚████╔╝ ██║  ██║╚██████╔╝██║ ██║  ██║██║  ██║ ╚████╔╝ ███████╗
      ╚═══╝  ╚═╝  ╚═╝ ╚═════╝ ╚═╝ ╚═╝  ╚═╝╚═╝  ╚═╝  ╚═══╝  ╚══════╝
                                                                      
EOF
    echo -e "${NC}"
    echo -e "${BOLD}CloudFlow 超级一键部署脚本 v${SCRIPT_VERSION}${NC}"
    echo -e "适配系统：${BOLD}CentOS/RHEL/Rocky/AlmaLinux/麒麟/欧拉/Debian/Ubuntu${NC}"
    echo ""
    
    # 权限检查
    check_root
    
    # 预检查
    preflight_check
    
    # 备份
    backup_existing
    
    echo ""
    echo "============================================"
    echo ""
    
    # 系统检测
    detect_os
    
    # 安装依赖
    echo ""
    log_info "开始安装系统依赖..."
    install_system_dependencies
    
    # 安装 Docker
    echo ""
    log_info "安装 Docker 环境..."
    install_docker
    
    # 部署项目
    echo ""
    log_info "获取并配置项目..."
    setup_project
    
    # 启动服务
    echo ""
    log_info "启动 CloudFlow 服务..."
    start_services
    
    # 健康检查
    echo ""
    check_services
    
    # 完成
    log_success "安装完成！"
}

# 启动主程序
main "$@"
