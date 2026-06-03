#!/bin/bash
# CloudFlow 安全密钥生成工具
# 用途: 生成强随机密钥用于生产环境

set -e

echo "======================================"
echo "CloudFlow 安全密钥生成工具"
echo "======================================"
echo ""

# 检查依赖
check_dependencies() {
    if ! command -v openssl &> /dev/null; then
        echo "❌ 错误: 需要安装 openssl"
        echo "   Ubuntu/Debian: sudo apt-get install openssl"
        echo "   CentOS/RHEL: sudo yum install openssl"
        exit 1
    fi
}

# 生成 API Key (32字节 hex)
generate_api_key() {
    echo "🔑 生成 API Key (32字节 hex):"
    API_KEY=$(openssl rand -hex 32)
    echo "   $API_KEY"
    echo ""
    echo "   使用方法:"
    echo "   export CLOUDFLOW_CENTER_API_KEY=$API_KEY"
    echo ""
}

# 生成 JWT Secret (64字节 base64)
generate_jwt_secret() {
    echo "🔐 生成 JWT Secret (64字节 base64):"
    JWT_SECRET=$(openssl rand -base64 64)
    echo "   $JWT_SECRET"
    echo ""
    echo "   使用方法:"
    echo "   export CLOUDFLOW_JWT_SECRET_KEY=\"$JWT_SECRET\""
    echo ""
}

# 生成 Webhook Secret (32字节 hex)
generate_webhook_secret() {
    echo "🌐 生成 Webhook Secret (32字节 hex):"
    WEBHOOK_SECRET=$(openssl rand -hex 32)
    echo "   $WEBHOOK_SECRET"
    echo ""
    echo "   使用方法:"
    echo "   在 Webhook 配置中设置: $WEBHOOK_SECRET"
    echo ""
}

# 生成所有密钥
generate_all() {
    echo "🎯 生成所有安全密钥..."
    echo ""
    
    generate_api_key
    generate_jwt_secret
    generate_webhook_secret
    
    echo "======================================"
    echo "⚠️  安全警告"
    echo "======================================"
    echo ""
    echo "1. 请立即将上述密钥保存到安全的位置"
    echo "2. 不要将密钥提交到版本控制系统"
    echo "3. 建议使用密钥管理服务 (如 Vault)"
    echo "4. 定期轮换密钥（建议每90天）"
    echo ""
    echo "📝 生成 .env 文件:"
    echo "   复制以下内容到 .env 文件："
    echo ""
    echo "   CLOUDFLOW_CENTER_API_KEY=$API_KEY"
    echo "   CLOUDFLOW_JWT_SECRET_KEY=\"$JWT_SECRET\""
    echo ""
}

# 生成 Kubernetes Secret
generate_k8s_secret() {
    echo "️  生成 Kubernetes Secret YAML:"
    echo ""
    
    API_KEY=$(openssl rand -hex 32)
    JWT_SECRET=$(openssl rand -base64 64)
    
    cat << EOF
apiVersion: v1
kind: Secret
metadata:
  name: cloudflow-secrets
  namespace: cloudflow
type: Opaque
stringData:
  api-key: $API_KEY
  jwt-secret: |
    $JWT_SECRET
EOF
    
    echo ""
    echo "   使用方法:"
    echo "   kubectl apply -f cloudflow-secrets.yaml"
    echo ""
}

# 生成 Docker Secret
generate_docker_secret() {
    echo "🐳 生成 Docker Secret 命令:"
    echo ""
    
    API_KEY=$(openssl rand -hex 32)
    
    echo "   docker secret create cloudflow-api-key <<< '$API_KEY'"
    echo ""
    echo "   然后在 docker-compose.yml 中使用:"
    echo "   secrets:"
    echo "     cloudflow-api-key:"
    echo "       external: true"
    echo ""
}

# 显示帮助
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  all          生成所有密钥（默认）"
    echo "  api-key      仅生成 API Key"
    echo "  jwt-secret   仅生成 JWT Secret"
    echo "  webhook      仅生成 Webhook Secret"
    echo "  k8s          生成 Kubernetes Secret YAML"
    echo "  docker       生成 Docker Secret 命令"
    echo "  help         显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 all           # 生成所有密钥"
    echo "  $0 api-key       # 仅生成 API Key"
    echo "  $0 k8s           # 生成 K8s Secret"
    echo ""
}

# 主函数
main() {
    check_dependencies
    
    case "${1:-all}" in
        all)
            generate_all
            ;;
        api-key)
            generate_api_key
            ;;
        jwt-secret)
            generate_jwt_secret
            ;;
        webhook)
            generate_webhook_secret
            ;;
        k8s)
            generate_k8s_secret
            ;;
        docker)
            generate_docker_secret
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            echo "❌ 未知选项: $1"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

main "$@"
