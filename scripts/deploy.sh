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
NC='\033[0m' # No Color

# 日志函数
info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 检查命令是否存在
check_command() {
    if ! command -v "$1" &> /dev/null; then
        error "请先安装 $1"
    fi
}

# 检查 Docker 是否安装
check_docker() {
    if ! command -v docker &> /dev/null; then
        info "正在安装 Docker..."
        curl -fsSL https://get.docker.com | bash
        info "Docker 安装完成"
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        info "正在安装 Docker Compose..."
        curl -L "https://github.com/docker/compose/releases/download/v2.24.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
        chmod +x /usr/local/bin/docker-compose
        info "Docker Compose 安装完成"
    fi
}

# 主部署流程
main() {
    info "=========================================="
    info "    CloudFlow 一键部署脚本"
    info "=========================================="
    
    # 检查依赖
    check_command curl
    check_command git
    check_docker
    
    # 创建部署目录
    info "创建部署目录..."
    mkdir -p /opt/cloudflow && cd /opt/cloudflow
    
    # 克隆代码库
    info "克隆代码仓库..."
    if [ -d ".git" ]; then
        git pull origin main
    else
        git clone https://github.com/meinanzilinzhengying/cloudflow.git .
    fi
    
    # 创建环境变量文件
    info "配置环境变量..."
    if [ ! -f ".env" ]; then
        cp .env.example .env
        
        # 设置随机密码
        DB_PASSWORD=$(openssl rand -hex 16)
        GRAFANA_PASSWORD=$(openssl rand -hex 8)
        
        sed -i "s/CLOUD_FLOW_DB_PASSWORD=.*/CLOUD_FLOW_DB_PASSWORD=$DB_PASSWORD/" .env
        sed -i "s/GRAFANA_ADMIN_PASSWORD=.*/GRAFANA_ADMIN_PASSWORD=$GRAFANA_PASSWORD/" .env
        
        info "数据库密码已设置为: $DB_PASSWORD"
        info "Grafana 管理员密码已设置为: $GRAFANA_PASSWORD"
        info "请妥善保存以上密码！"
    fi
    
    # 启动服务
    info "启动 CloudFlow 服务..."
    docker compose up -d
    
    # 等待服务启动
    info "等待服务启动（约30秒）..."
    sleep 30
    
    # 检查服务状态
    info "检查服务状态..."
    docker compose ps
    
    # 输出访问信息
    info "=========================================="
    info "    部署完成！"
    info "=========================================="
    info ""
    info "服务访问地址："
    info "  Grafana:    http://$(hostname -I | awk '{print $1}'):3001"
    info "  Prometheus: http://$(hostname -I | awk '{print $1}'):9091"
    info "  Jaeger:     http://$(hostname -I | awk '{print $1}'):16686"
    info ""
    info "API 端点："
    info "  Auth:       http://$(hostname -I | awk '{print $1}'):8006"
    info "  Control:    http://$(hostname -I | awk '{print $1}'):8001"
    info "  Query:      http://$(hostname -I | awk '{print $1}'):8007"
    info ""
    info "默认凭证："
    info "  Grafana 用户名: admin"
    info "  Grafana 密码: $(grep GRAFANA_ADMIN_PASSWORD .env | cut -d= -f2)"
    info ""
    info "管理命令："
    info "  查看日志: docker compose logs -f"
    info "  停止服务: docker compose down"
    info "  重启服务: docker compose restart"
    info "=========================================="
}

# 执行主流程
main