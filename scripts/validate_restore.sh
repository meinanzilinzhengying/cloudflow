#!/bin/bash
# CloudFlow 恢复验证脚本

echo "=== CloudFlow Restore Validation ==="

check_redis() {
    echo ""
    echo "1. Checking Redis..."
    REDIS_STATUS=$(redis-cli PING 2>/dev/null)
    if [ "$REDIS_STATUS" == "PONG" ]; then
        echo "   ✅ Redis is running"
        return 0
    else
        echo "   ❌ Redis is NOT running"
        return 1
    fi
}

check_clickhouse() {
    echo ""
    echo "2. Checking ClickHouse..."
    CLICKHOUSE_STATUS=$(clickhouse-client -q "SELECT 1" 2>/dev/null || echo "FAIL")
    if [ "$CLICKHOUSE_STATUS" == "1" ]; then
        echo "   ✅ ClickHouse is running"
        return 0
    else
        echo "   ❌ ClickHouse is NOT running"
        return 1
    fi
}

check_victoria() {
    echo ""
    echo "3. Checking VictoriaMetrics..."
    VM_STATUS=$(curl -s http://localhost:8428/health 2>/dev/null || echo "FAIL")
    if [ "$VM_STATUS" == "OK" ]; then
        echo "   ✅ VictoriaMetrics is running"
        return 0
    else
        echo "   ❌ VictoriaMetrics is NOT running"
        return 1
    fi
}

check_center() {
    echo ""
    echo "4. Checking Center API..."
    CENTER_STATUS=$(curl -s http://localhost:8080/api/healthz 2>/dev/null || echo "FAIL")
    if [ "$CENTER_STATUS" == "ok" ]; then
        echo "   ✅ Center API is running"
        return 0
    else
        echo "   ❌ Center API is NOT running"
        return 1
    fi
}

check_flow_data() {
    echo ""
    echo "5. Checking flow data..."
    FLOW_COUNT=$(clickhouse-client -q "SELECT COUNT(*) FROM flow_logs" 2>/dev/null || echo "0")
    echo "   Flow records: $FLOW_COUNT"
    if [ "$FLOW_COUNT" -gt 0 ]; then
        echo "   ✅ Flow data exists"
        return 0
    else
        echo "   ⚠️ No flow data found"
        return 0
    fi
}

check_dashboard_config() {
    echo ""
    echo "6. Checking dashboard configurations..."
    DASHBOARD_COUNT=$(redis-cli KEYS "dashboard:*" 2>/dev/null | wc -l)
    echo "   Dashboard configurations: $DASHBOARD_COUNT"
    if [ "$DASHBOARD_COUNT" -gt 0 ]; then
        echo "   ✅ Dashboard configurations exist"
        return 0
    else
        echo "   ⚠️ No dashboard configurations found"
        return 0
    fi
}

# Run all checks
ERRORS=0

check_redis || ERRORS=$((ERRORS + 1))
check_clickhouse || ERRORS=$((ERRORS + 1))
check_victoria || ERRORS=$((ERRORS + 1))
check_center || ERRORS=$((ERRORS + 1))
check_flow_data
check_dashboard_config

echo ""
echo "=== Validation Complete ==="

if [ $ERRORS -eq 0 ]; then
    echo "✅ All checks passed"
    exit 0
else
    echo "❌ $ERRORS check(s) failed"
    exit 1
fi