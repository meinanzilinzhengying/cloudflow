#!/bin/bash
# 验证安全修复是否已推送到远程仓库

echo "======================================"
echo "CloudFlow 安全修复验证"
echo "======================================"
echo ""

cd /opt/cloudflow

# 1. 检查本地配置文件
echo "📋 步骤 1: 检查本地配置文件..."
echo ""

echo "检查 cloud-flow-center/configs/config.yaml:"
if grep -q "ef1a6aca1e43ff5d1c4e85845281153f" cloud-flow-center/configs/config.yaml; then
    echo "  ❌ 仍包含硬编码的 API Key"
else
    echo "  ✅ 硬编码 API Key 已移除"
fi

if grep -q "test-secret-1234567890123456" cloud-flow-center/configs/config.yaml; then
    echo "  ❌ 仍包含硬编码的 JWT Secret"
else
    echo "  ✅ 硬编码 JWT Secret 已移除"
fi

if grep -q 'CLOUDFLOW_CENTER_API_KEY' cloud-flow-center/configs/config.yaml; then
    echo "  ✅ 已使用环境变量占位符"
else
    echo "  ❌ 未找到环境变量占位符"
fi

echo ""

# 2. 检查新增文件
echo "📦 步骤 2: 检查新增的安全文件..."
echo ""

files=(
    ".env.example"
    "scripts/generate-secrets.sh"
    "SECURITY_CONFIG.md"
)

for file in "${files[@]}"; do
    if [ -f "$file" ]; then
        echo "  ✅ $file 存在"
    else
        echo "  ❌ $file 不存在"
    fi
done

echo ""

# 3. 检查 Git 提交
echo "📝 步骤 3: 检查 Git 提交..."
echo ""

if git log --oneline -10 | grep -q "security: 修复硬编码密钥安全问题"; then
    echo "  ✅ 安全修复已提交"
else
    echo "   未找到安全修复提交"
fi

echo ""

# 4. 检查远程状态
echo "🚀 步骤 4: 检查远程同步状态..."
echo ""

LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse @{u} 2>/dev/null || echo "unknown")

if [ "$LOCAL" = "$REMOTE" ]; then
    echo "  ✅ 本地与远程同步"
else
    echo "  ⚠️  本地与远程不同步"
    echo "     本地: $LOCAL"
    echo "     远程: $REMOTE"
fi

echo ""

# 5. 检查远程分支
echo "🔀 步骤 5: 检查远程分支..."
echo ""

REMOTE_BRANCHES=$(git branch -r | grep -v "origin/main$" | grep -v "origin/HEAD$" | sed 's/^[[:space:]]*origin\///')

if [ -z "$REMOTE_BRANCHES" ]; then
    echo "  ✅ 远程只有 main 分支"
else
    echo "  ⚠️  远程还有其他分支："
    echo "$REMOTE_BRANCHES"
    echo ""
    echo "  建议执行合并和清理："
    echo "  bash scripts/merge_and_cleanup.sh"
fi

echo ""
echo "======================================"
echo "✅ 验证完成"
echo "======================================"
