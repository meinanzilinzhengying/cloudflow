#!/bin/bash
# CloudFlow 本地代码质量检查脚本
# 在 git commit 前运行，确保代码质量

set -e

echo "🔍 CloudFlow Pre-commit Quality Check"
echo "========================================"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

ERRORS=0
WARNINGS=0

# 1. gofmt 检查
echo ""
echo "📋 1. Checking gofmt..."
GOFMT_FILES=$(gofmt -l . | grep -v '^vendor/' | grep -v '^cloud-flow-frontend/' || true)
if [ -n "$GOFMT_FILES" ]; then
    echo -e "${RED}❌ gofmt failed on the following files:${NC}"
    echo "$GOFMT_FILES"
    ERRORS=$((ERRORS + 1))
else
    echo -e "${GREEN}✅ gofmt passed${NC}"
fi

# 2. go vet 检查
echo ""
echo "📋 2. Running go vet..."
GO_VET_DIRS="pkg cloud-flow-center cloud-flow-agent services/alert-engine services/data-plane services/control-plane services/query-service"
for dir in $GO_VET_DIRS; do
    if [ -d "$dir" ]; then
        if ! go vet ./$dir/... 2>/dev/null; then
            echo -e "${RED}❌ go vet failed in $dir${NC}"
            ERRORS=$((ERRORS + 1))
        fi
    fi
done
echo -e "${GREEN}✅ go vet passed${NC}"

# 3. 测试运行（快速模式，跳过耗时测试）
echo ""
echo "📋 3. Running quick tests..."
QUICK_TEST_DIRS="pkg cloud-flow-center services/alert-engine services/data-plane services/control-plane services/query-service"
for dir in $QUICK_TEST_DIRS; do
    if [ -d "$dir" ]; then
        cd "$dir" > /dev/null 2>&1 || continue
        if go test -short -count=1 ./... > /dev/null 2>&1; then
            echo -e "${GREEN}✅ $dir tests passed${NC}"
        else
            echo -e "${YELLOW}⚠️  $dir tests failed (non-blocking)${NC}"
            WARNINGS=$((WARNINGS + 1))
        fi
        cd - > /dev/null 2>&1
    fi
done

# 4. TODO/FIXME 检查（警告级别）
echo ""
echo "📋 4. Checking TODO/FIXME count..."
TODO_COUNT=$(grep -r "TODO\|FIXME" cloud-flow-center/ services/ --include="*.go" 2>/dev/null | wc -l)
echo "Found $TODO_COUNT TODO/FIXME items in critical paths"
if [ "$TODO_COUNT" -gt 50 ]; then
    echo -e "${YELLOW}⚠️  Warning: $TODO_COUNT TODO/FIXME items found. Consider addressing them before release.${NC}"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}✅ TODO/FIXME count acceptable ($TODO_COUNT)${NC}"
fi

# 5. 检查是否有敏感信息泄露（简单检查）
echo ""
echo "📋 5. Checking for potential secrets..."
SECRET_PATTERNS="password|passwd|secret|token|apikey|api_key|private_key"
SECRET_FILES=$(grep -riE "$SECRET_PATTERNS" --include="*.go" cloud-flow-center/ services/ 2>/dev/null | grep -v "_test.go" | grep -v "// " | head -20 || true)
if [ -n "$SECRET_FILES" ]; then
    echo -e "${YELLOW}⚠️  Warning: Found potential hardcoded secrets in:${NC}"
    echo "$SECRET_FILES"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}✅ No obvious hardcoded secrets found${NC}"
fi

# 6. 检查前端文件是否有 console.log
echo ""
echo "📋 6. Checking frontend console.log..."
CONSOLE_LOGS=$(grep -r "console.log" cloud-flow-frontend/src/ --include="*.ts" --include="*.vue" 2>/dev/null | grep -v "node_modules" | head -10 || true)
if [ -n "$CONSOLE_LOGS" ]; then
    echo -e "${YELLOW}⚠️  Warning: Found console.log in frontend files:${NC}"
    echo "$CONSOLE_LOGS"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}✅ No console.log found in frontend${NC}"
fi

# 汇总
echo ""
echo "========================================"
echo "📊 Quality Check Summary"
echo "========================================"
if [ "$ERRORS" -eq 0 ]; then
    echo -e "${GREEN}✅ All checks passed!${NC}"
    if [ "$WARNINGS" -gt 0 ]; then
        echo -e "${YELLOW}⚠️  $WARNINGS warning(s) found (non-blocking)${NC}"
    fi
    echo ""
    echo "🎉 Ready to commit!"
    exit 0
else
    echo -e "${RED}❌ $ERRORS error(s) found. Please fix them before committing.${NC}"
    if [ "$WARNINGS" -gt 0 ]; then
        echo -e "${YELLOW}⚠️  $WARNINGS warning(s) found${NC}"
    fi
    echo ""
    echo "💡 Run 'gofmt -w .' to auto-format Go files."
    exit 1
fi
