#!/bin/bash
# ===============================================================
# CloudFlow Agent Kubernetes 一键部署脚本
# 支持三种部署方式选择
# ===============================================================

set -euo pipefail

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[1;34m'
PURPLE='\033[1;35m'
NC='\033[0m'

# 日志函数
log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_title() { echo -e "${BLUE}============ $1 ============${NC}"; }

# 脚本目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"

# 默认配置
NAMESPACE="cloudflow"
DEPLOY_MODE="daemonset"
REGISTRY="registry.cloudflow.io"
IMAGE_TAG="latest"
DRY_RUN=false

# 帮助信息
show_help() {
    cat <<EOF
CloudFlow Agent Kubernetes 部署脚本

用法: $0 [选项]

选项:
  -h, --help              显示帮助信息
  -n, --namespace <name>  指定命名空间 (默认: cloudflow)
  -m, --mode <mode>       部署模式: daemonset/node/ecs (默认: daemonset)
  -r, --registry <url>    镜像仓库 (默认: registry.cloudflow.io)
  -t, --tag <tag>         镜像标签 (默认: latest)
  -k, --api-key <key>     CloudFlow API Key
  -e, --edge-addr <addr>  Edge服务地址 (默认: cloudflow-edge.cloudflow.svc.cluster.local:50051)
  -d, --dry-run           仅打印配置，不执行部署
  -u, --uninstall         卸载现有部署

示例:
  # 标准DaemonSet部署
  $0 -k your-api-key
  
  # 自定义命名空间
  $0 -n cloudflow-prod -k your-api-key
  
  # 仅生成配置
  $0 -k your-api-key -d > my-config.yaml
  
  # 卸载
  $0 -u
EOF
}

# 检查命令
check_command() {
    if ! command -v "$1" &>/dev/null; then
        log_error "命令 $1 未找到，请先安装"
        return 1
    fi
}

# 检查环境
check_environment() {
    log_title "环境检查"
    
    check_command kubectl || exit 1
    
    if ! kubectl cluster-info &>/dev/null; then
        log_error "无法连接到Kubernetes集群"
        log_info "请确保kubeconfig已正确配置"
        exit 1
    fi
    
    log_info "✓ Kubernetes集群连接正常"
    log_info "✓ 集群版本: $(kubectl version --short -o=yaml 2>/dev/null | grep serverVersion -A 5 | grep gitVersion | cut -d' ' -f4 || echo 'unknown')"
}

# 显示部署选项菜单
show_deployment_menu() {
    log_title "选择部署方式"
    echo "请选择CloudFlow Agent部署方式："
    echo ""
    echo "1) DaemonSet部署 - 在集群每个节点上部署一个Pod（推荐）"
    echo "2) 节点直接安装 - 在K8s节点上直接安装（需要SSH访问）"
    echo "3) ECS独立部署 - 在单个ECS上安装"
    echo ""
    read -p "请输入选择 [1]: " choice
    
    case "$choice" in
        1|"") DEPLOY_MODE="daemonset" ;;
        2) DEPLOY_MODE="node" ;;
        3) DEPLOY_MODE="ecs" ;;
        *) log_error "无效选择"; exit 1 ;;
    esac
}

# 获取API Key
get_api_key() {
    if [ -z "${API_KEY:-}" ]; then
        echo ""
        read -p "请输入CloudFlow API Key: " API_KEY
        if [ -z "$API_KEY" ]; then
            log_error "API Key不能为空"
            exit 1
        fi
    fi
}

# 生成部署配置
generate_config() {
    log_title "生成部署配置"
    
    local output_dir="${SCRIPT_DIR}/output"
    mkdir -p "$output_dir"
    
    # 替换变量
    sed "s#registry.cloudflow.io#${REGISTRY}#g" \
        "${SCRIPT_DIR}/05-daemonset.yaml" | \
    sed "s#cloudflow-edge.cloudflow.svc.cluster.local:50051#${EDGE_ADDR}#g" | \
    sed "s#latest#${IMAGE_TAG}#g" > "$output_dir/05-daemonset.yaml"
    
    # 复制其他配置
    cp "${SCRIPT_DIR}/00-namespace.yaml" "$output_dir/"
    cp "${SCRIPT_DIR}/01-serviceaccount.yaml" "$output_dir/"
    cp "${SCRIPT_DIR}/02-rbac.yaml" "$output_dir/"
    cp "${SCRIPT_DIR}/03-configmap.yaml" "$output_dir/"
    
    # 生成Secret
    cat > "$output_dir/04-secret.yaml" <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: cloudflow-agent-secrets
  namespace: ${NAMESPACE}
  labels:
    app: cloudflow-agent
type: Opaque
data:
  api-key: $(echo -n "$API_KEY" | base64 -w 0)
EOF
    
    log_info "配置文件已生成到: ${output_dir}"
}

# 执行部署
execute_deploy() {
    log_title "开始部署"
    
    local output_dir="${SCRIPT_DIR}/output"
    
    if [ "$DRY_RUN" = true ]; then
        log_info "Dry Run模式，显示要部署的配置："
        echo "---"
        cat "$output_dir/00-namespace.yaml"
        echo "---"
        cat "$output_dir/01-serviceaccount.yaml"
        echo "---"
        cat "$output_dir/02-rbac.yaml"
        echo "---"
        cat "$output_dir/03-configmap.yaml"
        echo "---"
        cat "$output_dir/04-secret.yaml"
        echo "---"
        cat "$output_dir/05-daemonset.yaml"
        echo "---"
        log_info "Dry Run完成，不执行实际部署"
        return
    fi
    
    # 应用配置
    log_info "创建命名空间..."
    kubectl apply -f "$output_dir/00-namespace.yaml" || true
    
    log_info "创建ServiceAccount..."
    kubectl apply -f "$output_dir/01-serviceaccount.yaml"
    
    log_info "配置RBAC权限..."
    kubectl apply -f "$output_dir/02-rbac.yaml"
    
    log_info "创建ConfigMap..."
    kubectl apply -f "$output_dir/03-configmap.yaml"
    
    log_info "创建Secret..."
    kubectl apply -f "$output_dir/04-secret.yaml"
    
    log_info "部署DaemonSet..."
    kubectl apply -f "$output_dir/05-daemonset.yaml"
    
    # 等待Pod启动
    log_title "等待Pod就绪"
    echo "这可能需要1-5分钟，取决于拉取镜像速度..."
    
    local max_attempts=60
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        local ready_pods=$(kubectl -n "$NAMESPACE" get daemonset cloudflow-agent -o=jsonpath='{.status.numberReady}' 2>/dev/null || echo 0)
        local desired_pods=$(kubectl -n "$NAMESPACE" get daemonset cloudflow-agent -o=jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null || echo 0)
        
        if [ "$ready_pods" -gt 0 ] && [ "$ready_pods" = "$desired_pods" ]; then
            log_info "✓ 所有Pod已就绪 ($ready_pods/$desired_pods)"
            break
        fi
        
        echo -n "."
        sleep 3
        attempt=$((attempt+1))
    done
    
    if [ $attempt -eq $max_attempts ]; then
        log_warn "部分Pod可能还在启动中，请稍后检查状态"
    fi
}

# 显示部署结果
show_result() {
    log_title "部署完成"
    echo ""
    echo "🚀 CloudFlow Agent 已成功部署!"
    echo ""
    echo "📊 查看状态:"
    echo "   kubectl -n $NAMESPACE get pods -o wide -l app=cloudflow-agent"
    echo ""
    echo "📈 查看日志:"
    echo "   kubectl -n $NAMESPACE logs -f daemonset/cloudflow-agent"
    echo ""
    echo "🛠️  查看完整信息:"
    echo "   kubectl -n $NAMESPACE get all -l app=cloudflow-agent"
    echo ""
    echo "🔗 访问前端界面:"
    echo "   业务监控: http://<your-server-ip>:3002"
    echo "   平台监控: http://<your-server-ip>:3003"
    echo ""
    echo "📚 查看文档: docs/kubernetes-agent-deployment.md"
}

# 卸载
uninstall() {
    log_title "卸载CloudFlow Agent"
    
    read -p "确定要卸载吗？这将删除所有相关资源 [y/N]: " confirm
    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        log_info "取消卸载"
        return
    fi
    
    log_info "删除资源..."
    kubectl -n "$NAMESPACE" delete daemonset cloudflow-agent 2>/dev/null || true
    kubectl -n "$NAMESPACE" delete secret cloudflow-agent-secrets 2>/dev/null || true
    kubectl -n "$NAMESPACE" delete configmap cloudflow-agent-config 2>/dev/null || true
    kubectl delete clusterrolebinding cloudflow-agent 2>/dev/null || true
    kubectl delete clusterrole cloudflow-agent 2>/dev/null || true
    kubectl -n "$NAMESPACE" delete serviceaccount cloudflow-agent 2>/dev/null || true
    
    # 可选：删除命名空间
    read -p "是否删除命名空间 $NAMESPACE？[y/N]: " delete_ns
    if [ "$delete_ns" = "y" ] || [ "$delete_ns" = "Y" ]; then
        kubectl delete namespace "$NAMESPACE" || true
    fi
    
    log_info "卸载完成"
}

# 主函数
main() {
    # 解析命令行参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -n|--namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            -m|--mode)
                DEPLOY_MODE="$2"
                shift 2
                ;;
            -r|--registry)
                REGISTRY="$2"
                shift 2
                ;;
            -t|--tag)
                IMAGE_TAG="$2"
                shift 2
                ;;
            -k|--api-key)
                API_KEY="$2"
                shift 2
                ;;
            -e|--edge-addr)
                EDGE_ADDR="$2"
                shift 2
                ;;
            -d|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -u|--uninstall)
                uninstall
                exit 0
                ;;
            *)
                log_error "未知选项: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    log_title "CloudFlow Agent K8s 部署工具"
    echo ""
    
    # 检查环境
    check_environment
    
    # 交互式菜单
    if [ -z "${API_KEY:-}" ]; then
        show_deployment_menu
    fi
    
    # 根据模式选择
    case "$DEPLOY_MODE" in
        "daemonset")
            get_api_key
            EDGE_ADDR="${EDGE_ADDR:-cloudflow-edge.cloudflow.svc.cluster.local:50051}"
            generate_config
            execute_deploy
            show_result
            ;;
        "node"|"ecs")
            log_warn "该模式需要通过SSH连接到节点，正在使用备选方案..."
            log_info "请参考 docs/kubernetes-agent-deployment.md 的节点部署部分"
            ;;
    esac
}

# 执行主函数
main "$@"
