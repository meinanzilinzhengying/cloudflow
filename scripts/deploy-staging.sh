#!/bin/bash
# CloudFlow Staging 环境部署脚本
# 一键部署所有服务到 Staging 环境

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
COMPOSE_FILE="$PROJECT_DIR/docker-compose-microservices.yml"

echo "🚀 CloudFlow Staging Deployment"
echo "================================"
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

ERRORS=0
WARNINGS=0

# ============================================================================
# 1. 前置检查
# ============================================================================

echo "📋 Step 1: Pre-deployment Checks"
echo "--------------------------------"

# 检查 Docker
echo -n "  Docker ... "
if docker --version > /dev/null 2>&1; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${RED}FAIL${NC}"
    echo "  Docker is not installed or not running"
    exit 1
fi

# 检查 Docker Compose
echo -n "  Docker Compose ... "
if docker-compose --version > /dev/null 2>&1 || docker compose version > /dev/null 2>&1; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${RED}FAIL${NC}"
    exit 1
fi

# 检查磁盘空间
echo -n "  Disk space ... "
AVAILABLE=$(df / | tail -1 | awk '{print $4}')
if [ "$AVAILABLE" -gt 10485760 ]; then  # > 10GB
    echo -e "${GREEN}OK${NC} ($(echo $AVAILABLE / 1024 / 1024 | bc)GB available)"
else
    echo -e "${YELLOW}WARN${NC} (only $(echo $AVAILABLE / 1024 / 1024 | bc)GB available)"
    WARNINGS=$((WARNINGS + 1))
fi

# 检查内存
echo -n "  Memory ... "
MEM_TOTAL=$(free -m | awk '/^Mem:/{print $2}')
if [ "$MEM_TOTAL" -gt 8192 ]; then  # > 8GB
    echo -e "${GREEN}OK${NC} (${MEM_TOTAL}MB)"
else
    echo -e "${YELLOW}WARN${NC} (${MEM_TOTAL}MB, recommend 16GB+)"
    WARNINGS=$((WARNINGS + 1))
fi

# 检查端口占用
echo -n "  Port check (3306, 6379, 9000, 9090) ... "
PORTS_OK=true
for port in 3306 6379 9000 9090; do
    if lsof -Pi :$port -sTCP:LISTEN > /dev/null 2>&1; then
        echo -e "\n    ${YELLOW}WARN${NC}: Port $port is already in use"
        PORTS_OK=false
    fi
done
if $PORTS_OK; then
    echo -e "${GREEN}OK${NC}"
else
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 2. 代码质量检查
# ============================================================================

echo ""
echo "📋 Step 2: Code Quality Checks"
echo "--------------------------------"

cd "$PROJECT_DIR"

# gofmt
echo -n "  gofmt ... "
if [ "$(gofmt -l . | grep -v '^vendor/' | grep -v '^cloud-flow-frontend/' | wc -l)" -eq 0 ]; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${YELLOW}WARN${NC} (some files not formatted)"
    WARNINGS=$((WARNINGS + 1))
fi

# go vet (pkg only, quick check)
echo -n "  go vet (pkg) ... "
if go vet ./pkg/... > /dev/null 2>&1; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${YELLOW}WARN${NC}"
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 3. 构建服务
# ============================================================================

echo ""
echo "📋 Step 3: Building Services"
echo "--------------------------------"

SERVICES=(
    "cloud-flow-center"
    "cloud-flow-agent"
    "services/alert-engine"
    "services/data-plane"
    "services/control-plane"
    "services/query-service"
    "services/tenant-service"
    "services/auth-service"
)

for svc in "${SERVICES[@]}"; do
    svc_name=$(basename "$svc")
    echo -n "  Building $svc_name ... "
    if go build -o "bin/staging-$svc_name" "./$svc" > /dev/null 2>&1; then
        echo -e "${GREEN}OK${NC}"
    else
        echo -e "${YELLOW}SKIP${NC} (may need dependencies)"
        WARNINGS=$((WARNINGS + 1))
    fi
done

# ============================================================================
# 4. 启动基础设施
# ============================================================================

echo ""
echo "📋 Step 4: Starting Infrastructure"
echo "--------------------------------"

if [ -f "$COMPOSE_FILE" ]; then
    echo -n "  Starting databases ... "
    docker-compose -f "$COMPOSE_FILE" up -d mysql redis clickhouse > /dev/null 2>&1
    echo -e "${GREEN}OK${NC}"
    
    echo -n "  Waiting for services to be ready (15s) ... "
    sleep 15
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${YELLOW}WARN${NC}: docker-compose-microservices.yml not found"
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 5. 启动核心服务
# ============================================================================

echo ""
echo "📋 Step 5: Starting Core Services"
echo "--------------------------------"

if [ -f "$COMPOSE_FILE" ]; then
    echo -n "  Starting services ... "
    docker-compose -f "$COMPOSE_FILE" up -d center agent alert-engine data-plane control-plane query-service tenant-service auth-service > /dev/null 2>&1
    echo -e "${GREEN}OK${NC}"
    
    echo -n "  Waiting for services to be ready (20s) ... "
    sleep 20
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${YELLOW}WARN${NC}: Skipping service startup (compose file not found)"
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 6. 健康检查
# ============================================================================

echo ""
echo "📋 Step 6: Health Checks"
echo "--------------------------------"

HEALTH_PORTS=(
    "9001:control-plane"
    "9002:data-plane"
    "9007:query-service"
    "9009:alert-engine"
    "9010:tenant-service"
)

for item in "${HEALTH_PORTS[@]}"; do
    port="${item%%:*}"
    name="${item##*:}"
    echo -n "  $name (:$port) ... "
    if curl -s http://localhost:$port/healthz > /dev/null 2>&1; then
        echo -e "${GREEN}HEALTHY${NC}"
    else
        echo -e "${YELLOW}UNKNOWN${NC} (service may not be ready yet)"
        WARNINGS=$((WARNINGS + 1))
    fi
done

# ============================================================================
# 7. 前端部署
# ============================================================================

echo ""
echo "📋 Step 7: Frontend Deployment"
echo "--------------------------------"

if [ -d "$PROJECT_DIR/cloud-flow-frontend" ]; then
    cd "$PROJECT_DIR/cloud-flow-frontend"
    echo -n "  Installing dependencies ... "
    if npm install > /dev/null 2>&1; then
        echo -e "${GREEN}OK${NC}"
    else
        echo -e "${YELLOW}WARN${NC}"
        WARNINGS=$((WARNINGS + 1))
    fi
    
    echo -n "  Building frontend ... "
    if npm run build > /dev/null 2>&1; then
        echo -e "${GREEN}OK${NC}"
    else
        echo -e "${YELLOW}WARN${NC}"
        WARNINGS=$((WARNINGS + 1))
    fi
    
    echo -n "  Deploying to Nginx ... "
    if [ -d "dist" ]; then
        # 复制到 Nginx 目录（需要 sudo）
        if sudo cp -r dist/* /usr/share/nginx/html/ > /dev/null 2>&1; then
            echo -e "${GREEN}OK${NC}"
            sudo systemctl reload nginx > /dev/null 2>&1
        else
            echo -e "${YELLOW}WARN${NC} (may need sudo)"
            WARNINGS=$((WARNINGS + 1))
        fi
    else
        echo -e "${YELLOW}WARN${NC} (dist not found)"
        WARNINGS=$((WARNINGS + 1))
    fi
    cd "$PROJECT_DIR"
else
    echo -e "${YELLOW}WARN${NC}: Frontend directory not found"
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 8. 汇总
# ============================================================================

echo ""
echo "================================"
echo "🎉 Staging Deployment Complete"
echo "================================"

if [ "$ERRORS" -eq 0 ]; then
    echo -e "${GREEN}✅ Deployment succeeded!${NC}"
    if [ "$WARNINGS" -gt 0 ]; then
        echo -e "${YELLOW}⚠️  $WARNINGS warning(s) - review above${NC}"
    fi
    echo ""
    echo "📌 Access Points:"
    echo "  - Frontend:    http://localhost"
    echo "  - Center API:  http://localhost:9090"
    echo "  - Prometheus:  http://localhost:9090/metrics"
    echo ""
    echo "📌 Next Steps:"
    echo "  1. Run health check: ./scripts/health-check.sh"
    echo "  2. Run integration tests: ./tests/integration/run.sh"
    echo "  3. Check logs: docker-compose -f docker-compose-microservices.yml logs -f"
    exit 0
else
    echo -e "${RED}❌ $ERRORS error(s) occurred${NC}"
    exit 1
fi
