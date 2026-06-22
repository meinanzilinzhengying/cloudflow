#!/bin/bash
# stability-test.sh - 长时间稳定性测试
# 运行方式: ./scripts/stability-test.sh [duration_hours] [report_file]
# 示例: ./scripts/stability-test.sh 24 /tmp/stability-report.json

set -euo pipefail

DURATION_HOURS=${1:-24}
REPORT_FILE=${2:-"/tmp/stability-report-$(date +%Y%m%d-%H%M%S).json"}
DURATION_SECONDS=$((DURATION_HOURS * 3600))

echo "=========================================="
echo "CloudFlow 稳定性测试"
echo "持续时间: ${DURATION_HOURS} 小时"
echo "报告文件: ${REPORT_FILE}"
echo "=========================================="

# 检查依赖
for cmd in docker docker-compose curl jq; do
    if ! command -v $cmd &> /dev/null; then
        echo "错误: 缺少依赖 $cmd"
        exit 1
    fi
done

# 启动测试环境
echo "[1/5] 启动测试环境..."
cd /opt/cloudflow
if docker-compose -f docker-compose.staging.yml ps -q | grep -q .; then
    echo "测试环境已在运行，跳过启动"
else
    docker-compose -f docker-compose.staging.yml up -d
    echo "等待 30 秒让服务完全启动..."
    sleep 30
fi

# 预热检查
echo "[2/5] 预热检查..."
HEALTH_URLS=(
    "http://localhost:8009/healthz"
    "http://localhost:8001/healthz"
    "http://localhost:8007/healthz"
    "http://localhost:8003/healthz"
    "http://localhost:8006/healthz"
    "http://localhost:8002/healthz"
)
for url in "${HEALTH_URLS[@]}"; do
    for i in {1..10}; do
        if curl -sf "$url" &>/dev/null; then
            echo "  ✓ $url 健康"
            break
        fi
        if [ $i -eq 10 ]; then
            echo "  ✗ $url 未就绪"
            exit 1
        fi
        sleep 2
    done
done

# 运行负载测试
echo "[3/5] 运行负载测试 (${DURATION_HOURS}h)..."
export GOWORK=/opt/cloudflow/go.work
export GOROOT=/usr/local/go
export PATH=/usr/local/go/bin:$PATH

# 在后台运行负载测试
go run /opt/cloudflow/scripts/load-test.go \
    -target "http://localhost:8009" \
    -concurrency 50 \
    -rate 5 \
    -duration "${DURATION_SECONDS}s" \
    -http-timeout 30s \
    > /tmp/stability-load-test.log 2>&1 &
LOAD_TEST_PID=$!

# 收集指标
echo "[4/5] 收集指标..."
METRICS_FILE="/tmp/stability-metrics.log"
echo "timestamp,total_reqs,errors,error_rate,p50,p95,p99,goroutines,heap_mb" > "$METRICS_FILE"

START_TIME=$(date +%s)
END_TIME=$((START_TIME + DURATION_SECONDS))

while [ $(date +%s) -lt $END_TIME ]; do
    # 检查负载测试是否还在运行
    if ! kill -0 $LOAD_TEST_PID 2>/dev/null; then
        echo "警告: 负载测试提前退出"
        break
    fi

    NOW=$(date +%s)
    ELAPSED=$((NOW - START_TIME))
    REMAINING=$((DURATION_SECONDS - ELAPSED))
    PROGRESS=$((ELAPSED * 100 / DURATION_SECONDS))

    # 从日志提取指标
    if [ -f /tmp/stability-load-test.log ]; then
        LAST_LINE=$(tail -1 /tmp/stability-load-test.log 2>/dev/null || echo "")
        # 解析日志中的指标
        TOTAL_REQS=$(echo "$LAST_LINE" | grep -oP 'Total: \K[0-9]+' || echo "0")
        ERRORS=$(echo "$LAST_LINE" | grep -oP 'Errors: \K[0-9]+' || echo "0")
        P50=$(echo "$LAST_LINE" | grep -oP 'P50=\K[0-9]+' || echo "0")
        P95=$(echo "$LAST_LINE" | grep -oP 'P95=\K[0-9]+' || echo "0")
        P99=$(echo "$LAST_LINE" | grep -oP 'P99=\K[0-9]+' || echo "0")
    else
        TOTAL_REQS=0; ERRORS=0; P50=0; P95=0; P99=0
    fi

    # 获取 Go runtime 指标（如果服务暴露 /debug/pprof）
    GOROUTINES="N/A"
    HEAP_MB="N/A"

    # 计算错误率
    if [ "$TOTAL_REQS" -gt 0 ]; then
        ERROR_RATE=$(echo "scale=4; $ERRORS / $TOTAL_REQS * 100" | bc 2>/dev/null || echo "0")
    else
        ERROR_RATE=0
    fi

    echo "$(date '+%Y-%m-%d %H:%M:%S'),$TOTAL_REQS,$ERRORS,$ERROR_RATE,$P50,$P95,$P99,$GOROUTINES,$HEAP_MB" >> "$METRICS_FILE"

    # 每 5 分钟输出进度
    if [ $((ELAPSED % 300)) -eq 0 ]; then
        echo "  进度: ${PROGRESS}% | 已运行: ${ELAPSED}s | 剩余: ${REMAINING}s | 错误率: ${ERROR_RATE}%"
    fi

    sleep 10
done

# 等待负载测试完成
if kill -0 $LOAD_TEST_PID 2>/dev/null; then
    wait $LOAD_TEST_PID || true
fi

echo "[5/5] 生成报告..."

# 解析最终结果
FINAL_LOG="/tmp/stability-load-test.log"
if [ -f "$FINAL_LOG" ]; then
    TOTAL_REQS=$(grep -oP 'Total: \K[0-9]+' "$FINAL_LOG" | tail -1 || echo "0")
    ERRORS=$(grep -oP 'Errors: \K[0-9]+' "$FINAL_LOG" | tail -1 || echo "0")
    P50=$(grep -oP 'P50=\K[0-9]+' "$FINAL_LOG" | tail -1 || echo "0")
    P95=$(grep -oP 'P95=\K[0-9]+' "$FINAL_LOG" | tail -1 || echo "0")
    P99=$(grep -oP 'P99=\K[0-9]+' "$FINAL_LOG" | tail -1 || echo "0")
else
    TOTAL_REQS=0; ERRORS=0; P50=0; P95=0; P99=0
fi

if [ "$TOTAL_REQS" -gt 0 ]; then
    ERROR_RATE=$(echo "scale=4; $ERRORS / $TOTAL_REQS * 100" | bc 2>/dev/null || echo "0")
else
    ERROR_RATE=0
fi

# 稳定性判断
STABLE="true"
REASONS=()
if (( $(echo "$ERROR_RATE > 1" | bc -l) )); then
    STABLE="false"
    REASONS+=("错误率 ${ERROR_RATE}% > 1%")
fi
if [ "$P99" -gt 200 ]; then
    STABLE="false"
    REASONS+=("P99 延迟 ${P99}ms > 200ms")
fi

# 生成 JSON 报告
cat > "$REPORT_FILE" << JSONEOF
{
  "test_type": "stability",
  "duration_hours": ${DURATION_HOURS},
  "start_time": "$(date -d @$START_TIME '+%Y-%m-%d %H:%M:%S')",
  "end_time": "$(date '+%Y-%m-%d %H:%M:%S')",
  "total_requests": ${TOTAL_REQS},
  "errors": ${ERRORS},
  "error_rate_percent": ${ERROR_RATE},
  "latency_ms": {
    "p50": ${P50},
    "p95": ${P95},
    "p99": ${P99}
  },
  "stable": ${STABLE},
  "reasons": [$(printf '"%s",' "${REASONS[@]}" | sed 's/,$//')],
  "metrics_file": "${METRICS_FILE}"
}
JSONEOF

echo "=========================================="
echo "稳定性测试完成"
echo "总请求: ${TOTAL_REQS} | 错误: ${ERRORS} | 错误率: ${ERROR_RATE}%"
echo "P50: ${P50}ms | P95: ${P95}ms | P99: ${P99}ms"
echo "稳定: ${STABLE}"
if [ ${#REASONS[@]} -gt 0 ]; then
    echo "问题: ${REASONS[*]}"
fi
echo "报告: ${REPORT_FILE}"
echo "=========================================="

# 清理
# docker-compose -f /opt/cloudflow/docker-compose.staging.yml down

exit 0
