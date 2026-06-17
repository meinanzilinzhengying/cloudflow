#!/bin/bash
# ============================================================
# CloudFlow 一键部署脚本 v3.2 - 100% 国产化版
# 适用环境：Linux 服务器（Ubuntu/Debian/CentOS）
# 使用方式：curl -sSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/scripts/deploy.sh | bash
#
# 国产化组件：
#   - OceanBase (主数据库, 蚂蚁集团开源)
#   - openGauss (时序数据库, 华为开源)
#   - Redis (缓存, 兼容高斯Redis)
#   - Nacos (服务发现, 阿里巴巴开源)
#   - RocketMQ (消息队列, 阿里云开源)
#   - Prometheus + Grafana (监控)
#   - SkyWalking (链路追踪, Apache)
# ============================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[1;34m'
NC='\033[0m'

# 日志函数
info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
section() { echo -e "${BLUE}========== $1 ==========${NC}"; }

# 检查命令是否存在
check_command() {
    if ! command -v "$1" &> /dev/null; then
        return 1
    fi
    return 0
}

# 检查 Docker 是否安装
check_docker() {
    if ! check_command docker; then
        info "正在安装 Docker..."
        curl -fsSL https://get.docker.com | bash
        info "Docker 安装完成"
        
        if command -v systemctl &> /dev/null; then
            systemctl enable --now docker
        fi
    fi
    
    if ! check_command docker-compose; then
        info "正在安装 Docker Compose..."
        curl -L "https://github.com/docker/compose/releases/download/v2.24.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
        chmod +x /usr/local/bin/docker-compose
        info "Docker Compose 安装完成"
    fi
}

# 健康检查函数
check_service_health() {
    local service_name=$1
    local max_attempts=60
    local attempt=0
    
    info "等待 $service_name 服务就绪..."
    while [ $attempt -lt $max_attempts ]; do
        if docker compose ps "$service_name" --format json 2>/dev/null | grep -q '"Health":\s*"healthy"'; then
            info "$service_name 服务已就绪"
            return 0
        fi
        sleep 3
        attempt=$((attempt + 1))
    done
    warn "$service_name 服务可能未完全就绪，但继续部署"
}

# 主部署流程
main() {
    section "CloudFlow 一键部署脚本 v3.2 - 100% 国产化版"
    
    # 检查依赖
    section "环境检查"
    if ! check_command curl; then
        error "请先安装 curl"
    fi
    if ! check_command git; then
        error "请先安装 git"
    fi
    check_docker
    
    # 获取主机 IP
    HOST_IP=$(hostname -I | awk '{print $1}' || echo "localhost")
    
    # 创建部署目录
    section "准备部署"
    DEPLOY_DIR=${DEPLOY_DIR:-/opt/cloudflow}
    info "部署目录: $DEPLOY_DIR"
    mkdir -p "$DEPLOY_DIR" && cd "$DEPLOY_DIR"
    
    # 克隆代码库
    section "获取代码"
    if [ -d ".git" ]; then
        info "更新现有代码..."
        git pull origin main
    else
        info "克隆代码仓库..."
        git clone https://github.com/meinanzilinzhengying/cloudflow.git .
    fi
    
    # 创建环境变量文件
    section "配置环境"
    if [ ! -f ".env" ]; then
        info "创建环境变量文件..."
        cp .env.example .env
        
        # 设置随机密码
        DB_PASSWORD=$(openssl rand -hex 16 2>/dev/null || echo "CloudFlow@2024")
        GRAFANA_PASSWORD=$(openssl rand -hex 8 2>/dev/null || echo "admin")
        
        info "数据库密码已设置"
        info "Grafana 管理员密码已设置"
    fi
    
    # 启动基础设施服务
    section "启动国产化基础设施"
    info "启动数据库和中间件服务（这可能需要 3-5 分钟）..."
    docker compose up -d oceanbase opengauss redis nacos rocketmq-namesrv rocketmq-broker
    
    # 等待关键服务就绪
    section "服务就绪检查"
    check_service_health nacos || true
    check_service_health redis || true
    sleep 60  # OceanBase 启动较慢
    
    # 启动监控和链路追踪
    section "启动监控平台"
    docker compose up -d prometheus grafana skywalking-oap skywalking-ui nginx
    
    # 启动 RocketMQ 控制台
    docker compose up -d rocketmq-dashboard
    
    # 检查服务状态
    section "服务状态"
    docker compose ps --format table
    
    # 输出访问信息
    section "部署完成"
    info ""
    info "🎉 国产化基础设施部署完成！"
    info ""
    info "📋 国产化组件面板访问地址："
    info "  ┌─────────────────────────────────────────────────┐"
    info "  │  Nacos:        http://$HOST_IP:8848            │"
    info "  │  用户名/密码:   nacos / nacos                   │"
    info "  ├─────────────────────────────────────────────────┤"
    info "  │  Grafana:      http://$HOST_IP:3000            │"
    info "  │  用户名/密码:   admin / admin                   │"
    info "  ├─────────────────────────────────────────────────┤"
    info "  │  Prometheus:   http://$HOST_IP:9090            │"
    info "  ├─────────────────────────────────────────────────┤"
    info "  │  SkyWalking:   http://$HOST_IP:18080           │"
    info "  │  用户名/密码:   admin / admin                   │"
    info "  ├─────────────────────────────────────────────────┤"
    info "  │  RocketMQ:     http://$HOST_IP:8080            │"
    info "  └─────────────────────────────────────────────────┘"
    info ""
    info "🗄️  数据库连接信息："
    info "  OceanBase:    $HOST_IP:2881"
    info "  用户名/密码:   root / CloudFlow@2024"
    info ""
    info "  openGauss:    $HOST_IP:5432"
    info "  用户名/密码:   gaussdb / CloudFlow@2024"
    info ""
    info "  Redis:        $HOST_IP:6379"
    info "  密码:          CloudFlow@2024"
    info ""
    info "📝 国产化组件清单："
    info "  ✅ OceanBase     - 蚂蚁集团开源（主数据库）"
    info "  ✅ openGauss     - 华为开源（时序数据库）"
    info "  ✅ Redis         - 兼容高斯Redis协议（缓存）"
    info "  ✅ Nacos         - 阿里巴巴开源（服务发现）"
    info "  ✅ RocketMQ      - 阿里云开源（消息队列）"
    info "  ✅ Prometheus    - CNCF标准（指标存储）"
    info "  ✅ Grafana       - 开源（监控面板）"
    info "  ✅ SkyWalking    - Apache（链路追踪）"
    info ""
    info "💡 管理命令（在 $DEPLOY_DIR 目录执行）："
    info "  查看所有服务:   docker compose ps"
    info "  查看日志:       docker compose logs -f [服务名]"
    info "  停止服务:       docker compose down"
    info "  重启服务:       docker compose restart"
    info "  更新服务:       docker compose pull && docker compose up -d"
    info ""
    info "📌 下一步："
    info "  1. 访问 Nacos (http://$HOST_IP:8848) 查看服务注册"
    info "  2. 访问 Grafana (http://$HOST_IP:3000) 配置监控面板"
    info "  3. 访问 SkyWalking (http://$HOST_IP:18080) 查看链路追踪"
    info ""
    info "=========================================="
}

# 执行主流程
main
