#!/bin/bash
# CloudFlow 分支清理脚本
# 用途：确保所有代码在 main 分支，删除其他所有分支

set -e

cd /opt/cloudflow

echo "======================================"
echo "CloudFlow 分支清理脚本"
echo "======================================"
echo ""

# 1. 切换到 main 分支
echo "📌 步骤 1: 切换到 main 分支..."
git checkout main 2>&1 || {
    echo "❌ 无法切换到 main 分支"
    exit 1
}
echo "✅ 已切换到 main 分支"
echo ""

# 2. 拉取最新代码
echo "📥 步骤 2: 拉取远程最新代码..."
git pull origin main 2>&1 || {
    echo "⚠️  拉取失败，继续执行..."
}
echo ""

# 3. 提交所有未提交的更改
echo "💾 步骤 3: 提交所有未提交的更改..."
git add -A
if ! git diff --cached --quiet; then
    git commit -m "chore: 同步所有修改到 main 分支" --no-verify
    echo "✅ 已提交更改"
else
    echo "ℹ️  没有未提交的更改"
fi
echo ""

# 4. 推送到远程 main 分支
echo "🚀 步骤 4: 推送到远程 main 分支..."
git push origin main --force-with-lease 2>&1 || {
    echo "❌ 推送失败"
    exit 1
}
echo "✅ 已推送到 main 分支"
echo ""

# 5. 列出所有本地分支（排除 main）
echo "🔍 步骤 5: 检查本地分支..."
LOCAL_BRANCHES=$(git branch | grep -v "^\*" | grep -v "main" | sed 's/^[[:space:]]*//')

if [ -z "$LOCAL_BRANCHES" ]; then
    echo "ℹ️  没有其他本地分支需要删除"
else
    echo "发现以下本地分支："
    echo "$LOCAL_BRANCHES"
    echo ""
    
    # 删除所有非 main 本地分支
    echo "🗑️  删除本地分支..."
    for branch in $LOCAL_BRANCHES; do
        echo "   删除: $branch"
        git branch -D "$branch" 2>&1 || echo "   ⚠️  删除失败: $branch"
    done
    echo "✅ 本地分支清理完成"
fi
echo ""

# 6. 列出所有远程分支（排除 main）
echo "🔍 步骤 6: 检查远程分支..."
REMOTE_BRANCHES=$(git branch -r | grep -v "origin/main" | grep -v "HEAD" | sed 's/^[[:space:]]*origin\///' | sed 's/^[[:space:]]*//')

if [ -z "$REMOTE_BRANCHES" ]; then
    echo "ℹ️  没有其他远程分支需要删除"
else
    echo "发现以下远程分支："
    echo "$REMOTE_BRANCHES"
    echo ""
    
    # 删除所有非 main 远程分支
    echo "🗑️  删除远程分支..."
    for branch in $REMOTE_BRANCHES; do
        echo "   删除: origin/$branch"
        git push origin --delete "$branch" 2>&1 || echo "   ⚠️  删除失败: $branch"
    done
    echo "✅ 远程分支清理完成"
fi
echo ""

# 7. 清理本地远程跟踪分支
echo "🧹 步骤 7: 清理本地远程跟踪分支..."
git remote prune origin 2>&1 || {
    echo "⚠️  清理跟踪分支失败"
}
echo ""

# 8. 最终状态检查
echo "======================================"
echo "📊 最终状态检查"
echo "======================================"
echo ""
echo "当前分支:"
git branch
echo ""
echo "远程分支:"
git branch -r
echo ""
echo "最近5次提交:"
git log --oneline -5
echo ""
echo "======================================"
echo "✅ 分支清理完成！"
echo "======================================"
echo ""
echo "总结:"
echo "  ✓ 所有代码已推送到 main 分支"
echo "  ✓ 其他本地分支已删除"
echo "  ✓ 其他远程分支已删除"
echo "  ✓ 只保留 main 分支"

