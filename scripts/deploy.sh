#!/bin/bash
# ============================================================
# CloudFlow 一键部署脚本
# 适用环境：Linux 服务器（Ubuntu/Debian/CentOS）
# 使用方式：curl -sSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/scripts/deploy.sh | bash
# ============================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[1;34m'
NC='\033[0m' # No Color

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
        
        # 启动 Docker 服务
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
    local max_attempts=30
    local attempt=0
    
    info "等待 $service_name 服务就绪..."
    while [ $attempt -lt $max_attempts ]; do
        if docker compose ps "$service_name" --format json | grep -q '"Health":\s*"healthy"'; then
            info "$service_name 服务已就绪"
            return 0
        fi
        sleep 2
        attempt=$((attempt + 1))
    done
    warn "$service_name 服务可能未完全就绪，但继续部署"
}

# 主部署流程
main() {
    section "CloudFlow 一键部署脚本"
    
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
        DB_PASSWORD=$(openssl rand -hex 16)
        GRAFANA_PASSWORD=$(openssl rand -hex 8)
        
        sed -i "s/CLOUD_FLOW_DB_PASSWORD=.*/CLOUD_FLOW_DB_PASSWORD=$DB_PASSWORD/" .env
        sed -i "s/GRAFANA_ADMIN_PASSWORD=.*/GRAFANA_ADMIN_PASSWORD=$GRAFANA_PASSWORD/" .env
        
        info "数据库密码已设置"
        info "Grafana 管理员密码已设置"
    fi
    
    # 启动服务
    section "启动服务"
    info "构建并启动所有服务（这可能需要 5-10 分钟）..."
    docker compose up -d --build
    
    # 等待服务启动
    section "服务就绪检查"
    info "等待基础设施服务启动..."
    sleep 30
    
    # 检查关键服务
    check_service_health tidb || true
    check_service_health clickhouse || true
    check_service_health control-plane || true
    check_service_health query-service || true
    
    # 检查服务状态
    section "服务状态"
    docker compose ps --format table
    
    # 输出访问信息
    section "部署完成"
    info ""
    info "服务访问地址："
    info "  📊 前端分析页面: http://$HOST_IP:8080"
    info "  🔧 平台自监控前端: http://$HOST_IP:3003"
    info "  📈 Grafana:    http://$HOST_IP:3001"
    info "  📉 Prometheus: http://$HOST_IP:9091"
    info "  🔍 Jaeger:     http://$HOST_IP:16686"
    info ""
    info "API 端点："
    info "  🔐 Auth:       http://$HOST_IP:8006"
    info "  🎛️  Control:    http://$HOST_IP:8001"
    info "  📥 Query:      http://$HOST_IP:8007"
    info ""
    info "默认凭证："
    info "  Grafana 用户名: admin"
    info "  Grafana 密码: $(grep GRAFANA_ADMIN_PASSWORD .env | cut -d= -f2)"
    info ""
    info "关于自监控："
    info "  ✅ 平台内置了完整的自监控功能"
    info "  ✅ 通过 Prometheus + Grafana 监控系统状态"
    info "  ✅ 通过 Loki 收集和查询日志"
    info "  ✅ 通过 Jaeger 进行分布式追踪"
    info "  ✅ 无需额外安装探针即可监控自身"
    info ""
    info "管理命令（在 $DEPLOY_DIR 目录执行）："
    info "  查看所有日志: docker compose logs -f"
    info "  查看特定服务: docker compose logs -f [service-name]"
    info "  停止服务:    docker compose down"
    info "  重启服务:    docker compose restart"
    info "  更新服务:    docker compose up -d --build"
    info "  查看服务状态: docker compose ps"
    info ""
    info "服务架构："
    info "  ┌─────────────────────────────────────────────────────┐"
    info "  │                    用户访问层                         │"
    info "  │  Business Frontend    │    Platform Frontend         │"
    info "  │  (业务监控)            │    (平台自监控)               │"
    info "  └─────────────────────────────────────────────────────┘"
    info "  ┌─────────────────────────────────────────────────────┐"
    info "  │                   微服务层                           │"
    info "  │  Auth | Control | Query | Alert | Tenant | ...     │"
    info "  └─────────────────────────────────────────────────────┘"
    info "  ┌─────────────────────────────────────────────────────┐"
    info "  │                   基础设施层                          │"
    info "  │  TiDB | ClickHouse | Kafka | Redis | ...            │"
    info "  └─────────────────────────────────────────────────────┘"
    info "  ┌─────────────────────────────────────────────────────┐"
    info "  │                   可观测性层                          │"
    info "  │  Prometheus | Grafana | Loki | Jaeger | ...         │"
    info "  └─────────────────────────────────────────────────────┘"
    info ""
    info "下一步建议："
    info "  1. 访问 Platform Frontend (http://$HOST_IP:3003) 查看平台状态"
    info "  2. 通过 SSH 安装功能部署 Agent 到目标机器"
    info "  3. 查看 Grafana 仪表板了解系统运行指标"
    info ""
    info "=========================================="
}

# 执行主流程
main