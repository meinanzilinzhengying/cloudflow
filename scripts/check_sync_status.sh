#!/bin/bash
# CloudFlow 远程仓库同步验证脚本

echo "======================================"
echo "CloudFlow 远程仓库同步状态检查"
echo "======================================"
echo ""

cd /opt/cloudflow || exit 1

# 检查本地状态
echo "📋 本地 Git 状态:"
git status --short
echo ""

# 检查最近的提交
echo "📝 最近5次提交:"
git log --oneline -5
echo ""

# 检查是否有未推送的提交
echo "🚀 远程同步状态:"
git fetch origin 2>&1
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse @{u})

if [ "$LOCAL" = "$REMOTE" ]; then
    echo "✅ 本地分支与远程分支同步"
else
    echo "⚠️  本地分支与远程分支不同步"
    echo "   本地: $LOCAL"
    echo "   远程: $REMOTE"
    
    # 显示差异
    echo ""
    echo "📊 未推送的提交:"
    git log --oneline @{u}..HEAD
fi

echo ""
echo "📦 阶段1新增文件统计:"
echo "   测试文件: $(find . -name "*_test.go" -newer PRODUCTION_READINESS_REPORT.md 2>/dev/null | wc -l)"
echo "   脚本文件: $(find scripts/ -name "*.sh" 2>/dev/null | wc -l)"
echo "   核心功能: $(find internal/ -name "*.go" ! -name "*_test.go" -newer PRODUCTION_READINESS_REPORT.md 2>/dev/null | wc -l)"
echo ""

echo "======================================"
echo "检查完成"
echo "======================================"
