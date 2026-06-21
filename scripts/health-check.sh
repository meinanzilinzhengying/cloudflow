#!/bin/bash
# CloudFlow 服务健康检查脚本
# 检查所有核心服务的健康状态

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

ERRORS=0
WARNINGS=0
HEALTHY=0

echo "🏥 CloudFlow Health Check"
echo "=========================="
echo ""

# ============================================================================
# 1. 基础设施检查
# ============================================================================

echo "📋 Infrastructure"
echo "----------------"

# Docker
echo -n "  Docker daemon ... "
if docker info > /dev/null 2>&1; then
    echo -e "${GREEN}HEALTHY${NC}"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${RED}FAIL${NC}"
    ERRORS=$((ERRORS + 1))
fi

# Docker Compose containers
echo -n "  Running containers ... "
RUNNING=$(docker ps --format '{{.Names}}' 2>/dev/null | wc -l)
if [ "$RUNNING" -gt 0 ]; then
    echo -e "${GREEN}OK${NC} ($RUNNING containers)"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${YELLOW}WARN${NC} (no containers running)"
    WARNINGS=$((WARNINGS + 1))
fi

# 磁盘空间
echo -n "  Disk space ... "
USAGE=$(df / | tail -1 | awk '{print $5}' | tr -d '%')
if [ "$USAGE" -lt 90 ]; then
    echo -e "${GREEN}OK${NC} ($USAGE% used)"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${RED}FAIL${NC} ($USAGE% used, >90%)"
    ERRORS=$((ERRORS + 1))
fi

# 内存
echo -n "  Memory usage ... "
MEM_USED=$(free | awk '/^Mem:/{printf "%.0f", ($3/$2)*100}')
if [ "$MEM_USED" -lt 90 ]; then
    echo -e "${GREEN}OK${NC} ($MEM_USED% used)"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${RED}FAIL${NC} ($MEM_USED% used, >90%)"
    ERRORS=$((ERRORS + 1))
fi

# ============================================================================
# 2. 数据库检查
# ============================================================================

echo ""
echo "📋 Databases"
echo "------------"

# MySQL
echo -n "  MySQL (3306) ... "
if mysqladmin -h localhost -P 3306 ping > /dev/null 2>&1; then
    echo -e "${GREEN}HEALTHY${NC}"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${YELLOW}WARN${NC} (not reachable)"
    WARNINGS=$((WARNINGS + 1))
fi

# Redis
echo -n "  Redis (6379) ... "
if redis-cli -h localhost ping > /dev/null 2>&1; then
    echo -e "${GREEN}HEALTHY${NC}"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${YELLOW}WARN${NC} (not reachable)"
    WARNINGS=$((WARNINGS + 1))
fi

# ClickHouse
echo -n "  ClickHouse (8123) ... "
if curl -s http://localhost:8123/ping > /dev/null 2>&1; then
    echo -e "${GREEN}HEALTHY${NC}"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${YELLOW}WARN${NC} (not reachable)"
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 3. 核心服务 HTTP 健康检查
# ============================================================================

echo ""
echo "📋 Core Services (HTTP)"
echo "----------------------"

HTTP_SERVICES=(
    "8001:control-plane"
    "9102:data-plane-metrics"
    "8007:query-service"
    "8009:alert-engine"
    "8010:tenant-service"
    "9090:center"
)

for item in "${HTTP_SERVICES[@]}"; do
    port="${item%%:*}"
    name="${item##*:}"
    echo -n "  $name (:$port) ... "
    
    if curl -sf http://localhost:$port/healthz > /dev/null 2>&1; then
        echo -e "${GREEN}HEALTHY${NC}"
        HEALTHY=$((HEALTHY + 1))
    elif curl -sf http://localhost:$port/health > /dev/null 2>&1; then
        echo -e "${GREEN}HEALTHY${NC} (via /health)"
        HEALTHY=$((HEALTHY + 1))
    else
        echo -e "${YELLOW}WARN${NC} (no response)"
        WARNINGS=$((WARNINGS + 1))
    fi
done

# ============================================================================
# 4. 核心服务 gRPC 健康检查
# ============================================================================

echo ""
echo "📋 Core Services (gRPC)"
echo "-----------------------"

GRPC_PORTS=(
    "9001:control-plane"
    "9002:data-plane"
    "9007:query-service"
    "9009:alert-engine"
    "9010:tenant-service"
)

for item in "${GRPC_PORTS[@]}"; do
    port="${item%%:*}"
    name="${item##*:}"
    echo -n "  $name (:$port) ... "
    
    # 使用 grpc_health_probe 或简单 TCP 检查
    if timeout 2 bash -c "cat < /dev/null > /dev/tcp/localhost/$port" > /dev/null 2>&1; then
        echo -e "${GREEN}HEALTHY${NC} (TCP open)"
        HEALTHY=$((HEALTHY + 1))
    else
        echo -e "${YELLOW}WARN${NC} (port closed)"
        WARNINGS=$((WARNINGS + 1))
    fi
done

# ============================================================================
# 5. 前端检查
# ============================================================================

echo ""
echo "📋 Frontend"
echo "-----------"

echo -n "  Nginx (80) ... "
if curl -sf http://localhost/ > /dev/null 2>&1; then
    echo -e "${GREEN}HEALTHY${NC}"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${YELLOW}WARN${NC} (not responding)"
    WARNINGS=$((WARNINGS + 1))
fi

echo -n "  Frontend assets ... "
if curl -sf http://localhost/ | grep -q "html\|vue\|script" > /dev/null 2>&1; then
    echo -e "${GREEN}OK${NC}"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${YELLOW}WARN${NC} (unexpected response)"
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 6. 监控检查
# ============================================================================

echo ""
echo "📋 Observability"
echo "----------------"

echo -n "  Prometheus (9090) ... "
if curl -sf http://localhost:9090/-/healthy > /dev/null 2>&1; then
    echo -e "${GREEN}HEALTHY${NC}"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${YELLOW}WARN${NC}"
    WARNINGS=$((WARNINGS + 1))
fi

echo -n "  Grafana (3000) ... "
if curl -sf http://localhost:3000/api/health > /dev/null 2>&1; then
    echo -e "${GREEN}HEALTHY${NC}"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "${YELLOW}WARN${NC}"
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 7. 日志检查
# ============================================================================

echo ""
echo "📋 Recent Errors (last 5 min)"
echo "------------------------------"

ERROR_COUNT=$(journalctl --since "5 minutes ago" -u cloudflow* 2>/dev/null | grep -i "error\|fatal\|panic" | wc -l)
if [ "$ERROR_COUNT" -eq 0 ]; then
    echo -e "  ${GREEN}✅ No errors found${NC}"
    HEALTHY=$((HEALTHY + 1))
else
    echo -e "  ${YELLOW}⚠️  $ERROR_COUNT error(s) found${NC}"
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 8. 汇总
# ============================================================================

echo ""
echo "=========================="
echo "📊 Health Check Summary"
echo "=========================="

echo -e "  ${GREEN}✅ Healthy: $HEALTHY${NC}"
echo -e "  ${YELLOW}⚠️  Warnings: $WARNINGS${NC}"
echo -e "  ${RED}❌ Errors: $ERRORS${NC}"

if [ "$ERRORS" -eq 0 ] && [ "$WARNINGS" -eq 0 ]; then
    echo ""
    echo -e "${GREEN}🎉 All systems operational!${NC}"
    exit 0
elif [ "$ERRORS" -eq 0 ]; then
    echo ""
    echo -e "${YELLOW}⚠️  System functional but has warnings${NC}"
    exit 0
else
    echo ""
    echo -e "${RED}❌ System has errors that need attention${NC}"
    echo ""
    echo "💡 Troubleshooting:"
    echo "   - Check service logs: docker-compose logs -f <service>"
    echo "   - Restart failed services: docker-compose restart <service>"
    echo "   - Check disk space: df -h"
    echo "   - Check memory: free -h"
    exit 1
fi
