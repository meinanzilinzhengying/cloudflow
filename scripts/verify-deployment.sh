#!/bin/bash
# CloudFlow 部署验证脚本
# 验证部署后的业务功能是否正常

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASSED=0
FAILED=0
WARNINGS=0

echo "🧪 CloudFlow Deployment Verification"
echo "====================================="
echo ""

API_BASE="http://localhost:9090"
AUTH_TOKEN=""  # 将在登录后填充

# ============================================================================
# 辅助函数
# ============================================================================

http_get() {
    local url="$1"
    local expected_status="${2:-200}"
    local auth_header=""
    if [ -n "$AUTH_TOKEN" ]; then
        auth_header="-H Authorization: Bearer $AUTH_TOKEN"
    fi
    
    status=$(curl -s -o /dev/null -w "%{http_code}" $auth_header "$url" 2>/dev/null || echo "000")
    if [ "$status" = "$expected_status" ]; then
        echo -e "${GREEN}PASS${NC} ($status)"
        PASSED=$((PASSED + 1))
        return 0
    else
        echo -e "${RED}FAIL${NC} (expected $expected_status, got $status)"
        FAILED=$((FAILED + 1))
        return 1
    fi
}

http_post() {
    local url="$1"
    local data="$2"
    local expected_status="${3:-200}"
    
    status=$(curl -s -o /dev/null -w "%{http_code}" -X POST -H "Content-Type: application/json" -d "$data" "$url" 2>/dev/null || echo "000")
    if [ "$status" = "$expected_status" ]; then
        echo -e "${GREEN}PASS${NC} ($status)"
        PASSED=$((PASSED + 1))
        return 0
    else
        echo -e "${RED}FAIL${NC} (expected $expected_status, got $status)"
        FAILED=$((FAILED + 1))
        return 1
    fi
}

# ============================================================================
# 1. 基础连通性测试
# ============================================================================

echo "📋 Test Suite 1: Basic Connectivity"
echo "------------------------------------"

echo -n "  API server reachable ... "
http_get "$API_BASE/healthz" 200

echo -n "  Frontend reachable ... "
http_get "http://localhost/" 200

# ============================================================================
# 2. 认证测试
# ============================================================================

echo ""
echo "📋 Test Suite 2: Authentication"
echo "----------------------------------"

echo -n "  Login endpoint exists ... "
http_post "$API_BASE/api/auth/login" '{"username":"admin","password":"admin"}' 200

echo -n "  Invalid login rejected ... "
http_post "$API_BASE/api/auth/login" '{"username":"admin","password":"wrong"}' 401

# ============================================================================
# 3. 查询接口测试
# ============================================================================

echo ""
echo "📋 Test Suite 3: Query APIs"
echo "---------------------------"

echo -n "  Overview endpoint ... "
http_get "$API_BASE/api/overview" 200

echo -n "  Nodes endpoint ... "
http_get "$API_BASE/api/nodes" 200

echo -n "  Business endpoint ... "
http_get "$API_BASE/api/business" 200

echo -n "  Service endpoint ... "
http_get "$API_BASE/api/service" 200

echo -n "  Topology endpoint ... "
http_get "$API_BASE/api/topology" 200

echo -n "  Alert list endpoint ... "
http_get "$API_BASE/api/alert/list" 200

# ============================================================================
# 4. 数据写入测试
# ============================================================================

echo ""
echo "📋 Test Suite 4: Data Ingestion"
echo "-------------------------------"

echo -n "  Flow ingestion endpoint ... "
http_post "$API_BASE/api/v1/ingest/flows" '{"flows":[]}' 200

echo -n "  Metrics ingestion endpoint ... "
http_post "$API_BASE/api/v1/ingest/metrics" '{"metrics":[]}' 200

# ============================================================================
# 5. 管理接口测试
# ============================================================================

echo ""
echo "📋 Test Suite 5: Management APIs"
echo "--------------------------------"

echo -n "  Agent list endpoint ... "
http_get "$API_BASE/api/agents" 200

echo -n "  Config endpoint ... "
http_get "$API_BASE/api/configs" 200

# ============================================================================
# 6. 性能基线测试
# ============================================================================

echo ""
echo "📋 Test Suite 6: Performance Baseline"
echo "--------------------------------------"

echo -n "  API response time (< 500ms) ... "
RESPONSE_TIME=$(curl -s -o /dev/null -w "%{time_total}" "$API_BASE/api/overview" 2>/dev/null || echo "999")
RESPONSE_MS=$(echo "$RESPONSE_TIME * 1000" | bc | cut -d. -f1)
if [ "$RESPONSE_MS" -lt 500 ]; then
    echo -e "${GREEN}PASS${NC} (${RESPONSE_MS}ms)"
    PASSED=$((PASSED + 1))
else
    echo -e "${YELLOW}WARN${NC} (${RESPONSE_MS}ms, > 500ms)"
    WARNINGS=$((WARNINGS + 1))
fi

echo -n "  Concurrent requests (10 parallel) ... "
if command -v parallel > /dev/null 2>&1; then
    SUCCESS_COUNT=$(seq 1 10 | parallel -j10 "curl -s -o /dev/null -w '%{http_code}' $API_BASE/healthz" 2>/dev/null | grep -c "200" || echo "0")
    if [ "$SUCCESS_COUNT" -eq 10 ]; then
        echo -e "${GREEN}PASS${NC} (10/10)"
        PASSED=$((PASSED + 1))
    else
        echo -e "${YELLOW}WARN${NC} ($SUCCESS_COUNT/10)"
        WARNINGS=$((WARNINGS + 1))
    fi
else
    SUCCESS_COUNT=0
    for i in {1..10}; do
        status=$(curl -s -o /dev/null -w "%{http_code}" "$API_BASE/healthz" 2>/dev/null || echo "000")
        if [ "$status" = "200" ]; then
            SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        fi
    done
    if [ "$SUCCESS_COUNT" -eq 10 ]; then
        echo -e "${GREEN}PASS${NC} (10/10)"
        PASSED=$((PASSED + 1))
    else
        echo -e "${YELLOW}WARN${NC} ($SUCCESS_COUNT/10)"
        WARNINGS=$((WARNINGS + 1))
    fi
fi

# ============================================================================
# 7. 前端功能测试
# ============================================================================

echo ""
echo "📋 Test Suite 7: Frontend Verification"
echo "---------------------------------------"

echo -n "  HTML page loads ... "
if curl -s http://localhost/ | grep -q "html\|body\|div" > /dev/null 2>&1; then
    echo -e "${GREEN}PASS${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}FAIL${NC} (unexpected content)"
    FAILED=$((FAILED + 1))
fi

echo -n "  Static assets (JS/CSS) ... "
JS_COUNT=$(curl -s http://localhost/ | grep -o 'src="[^"]*\.js"' | wc -l)
CSS_COUNT=$(curl -s http://localhost/ | grep -o 'href="[^"]*\.css"' | wc -l)
if [ "$JS_COUNT" -gt 0 ] || [ "$CSS_COUNT" -gt 0 ]; then
    echo -e "${GREEN}PASS${NC} ($JS_COUNT JS, $CSS_COUNT CSS)"
    PASSED=$((PASSED + 1))
else
    echo -e "${YELLOW}WARN${NC} (no JS/CSS found)"
    WARNINGS=$((WARNINGS + 1))
fi

# ============================================================================
# 8. 汇总
# ============================================================================

echo ""
echo "====================================="
echo "📊 Verification Summary"
echo "====================================="

echo -e "  ${GREEN}✅ Passed: $PASSED${NC}"
echo -e "  ${YELLOW}⚠️  Warnings: $WARNINGS${NC}"
echo -e "  ${RED}❌ Failed: $FAILED${NC}"

TOTAL=$((PASSED + FAILED + WARNINGS))
PASS_RATE=$(echo "scale=1; $PASSED * 100 / $TOTAL" | bc 2>/dev/null || echo "0")

echo ""
echo "  Pass Rate: $PASS_RATE%"

if [ "$FAILED" -eq 0 ]; then
    echo ""
    echo -e "${GREEN}🎉 Deployment verification passed!${NC}"
    echo ""
    echo "📌 The system is ready for production use."
    echo "📌 Remember to run the full DEPLOYMENT_CHECKLIST.md before going live."
    exit 0
else
    echo ""
    echo -e "${RED}❌ Deployment verification failed with $FAILED error(s)${NC}"
    echo ""
    echo "💡 Please fix the failing tests before proceeding to production."
    exit 1
fi
