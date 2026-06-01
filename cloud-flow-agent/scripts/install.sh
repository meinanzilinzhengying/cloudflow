#!/bin/bash
# cloud-flow-agent 安装脚本
# 支持架构: x86_64, aarch64
# 支持芯片: Intel, AMD, 海光 (Hygon), 鲲鹏 (Kunpeng)
# 用法: sudo ./install.sh [选项]
#   --prefix=PREFIX    安装前缀 (默认: /opt/cloud-flow-agent)
#   --config=CONFIG    配置文件路径 (默认: /etc/cloud-flow-agent/config.yaml)
#   --binary=BINARY    二进制文件路径 (默认: 自动检测)

set -euo pipefail

# ============================================================================
# 配置
# ============================================================================

INSTALL_PREFIX="${INSTALL_PREFIX:-/opt/cloud-flow-agent}"
CONFIG_DIR="/etc/cloud-flow-agent"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
BINARY_NAME="cloud-flow-agent"
SERVICE_NAME="cloud-flow-agent"
USER_NAME="cloud-flow"
GROUP_NAME="cloud-flow"
LOG_DIR="/var/log/cloud-flow-agent"
DATA_DIR="/var/lib/cloud-flow-agent"
SYSTEMD_DIR="/etc/systemd/system"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ============================================================================
# 工具函数
# ============================================================================

info()    { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }
step()    { echo -e "${BLUE}[STEP]${NC} $*"; }

check_root() {
    if [[ $EUID -ne 0 ]]; then
        error "此脚本需要 root 权限运行，请使用 sudo"
        exit 1
    fi
}

# ============================================================================
# 系统检测
# ============================================================================

detect_arch() {
    local arch
    arch=$(uname -m)

    case "${arch}" in
        x86_64|amd64)
            echo "x86_64"
            ;;
        aarch64|arm64)
            echo "aarch64"
            ;;
        *)
            error "不支持的架构: ${arch}"
            exit 1
            ;;
    esac
}

detect_vendor() {
    local arch="$1"
    local vendor="unknown"

    if [[ "${arch}" == "aarch64" ]]; then
        # 检测鲲鹏
        if grep -qi "kunpeng\|鲲鹏\|hisilicon\|hi silicon" /proc/cpuinfo 2>/dev/null; then
            vendor="kunpeng"
        fi
    elif [[ "${arch}" == "x86_64" ]]; then
        # 检测海光
        if grep -qi "hygon\|dhyana\|海光" /proc/cpuinfo 2>/dev/null; then
            vendor="hygon"
        elif grep -q "GenuineIntel" /proc/cpuinfo 2>/dev/null; then
            vendor="intel"
        elif grep -q "AuthenticAMD" /proc/cpuinfo 2>/dev/null; then
            vendor="amd"
        fi
    fi

    echo "${vendor}"
}

detect_kernel_version() {
    uname -r
}

check_kernel_version() {
    local kernel_version="$1"
    local major minor

    # 提取主次版本号
    major=$(echo "${kernel_version}" | cut -d. -f1)
    minor=$(echo "${kernel_version}" | cut -d. -f2)

    # 最低内核版本要求: 4.14
    if [[ "${major}" -lt 4 ]] || [[ "${major}" -eq 4 && "${minor}" -lt 14 ]]; then
        warn "内核版本 ${kernel_version} 低于推荐版本 4.14+"
        warn "eBPF 功能可能不可用，将使用传统采集模式"
        return 1
    fi

    return 0
}

check_systemd() {
    if ! command -v systemctl &>/dev/null; then
        error "未检测到 systemd，无法注册系统服务"
        error "请手动管理 cloud-flow-agent 进程"
        return 1
    fi
    return 0
}

# ============================================================================
# 安装步骤
# ============================================================================

create_user() {
    step "创建系统用户和组"

    if id "${USER_NAME}" &>/dev/null; then
        info "用户 ${USER_NAME} 已存在"
    else
        groupadd -r "${GROUP_NAME}" 2>/dev/null || true
        useradd -r -g "${GROUP_NAME}" -d "${INSTALL_PREFIX}" -s /usr/sbin/nologin \
            -c "Cloud Flow Agent" "${USER_NAME}" 2>/dev/null || true
        info "已创建用户 ${USER_NAME} 和组 ${GROUP_NAME}"
    fi
}

install_binary() {
    step "安装二进制文件"

    # 查找二进制文件
    local binary_source=""

    # 优先使用命令行指定的二进制文件
    for arg in "$@"; do
        case "${arg}" in
            --binary=*)
                binary_source="${arg#*=}"
                ;;
        esac
    done

    # 自动检测：查找当前目录或构建目录中的二进制
    if [[ -z "${binary_source}" ]]; then
        local arch
        arch=$(detect_arch)

        # 按优先级查找二进制文件
        local search_paths=(
            "./${BINARY_NAME}"
            "./build/${BINARY_NAME}-${arch}"
            "./bin/${BINARY_NAME}"
            "./dist/${BINARY_NAME}-${arch}"
        )

        for path in "${search_paths[@]}"; do
            if [[ -x "${path}" ]]; then
                binary_source="${path}"
                break
            fi
        done
    fi

    if [[ -z "${binary_source}" || ! -f "${binary_source}" ]]; then
        error "未找到二进制文件，请使用 --binary=PATH 指定"
        exit 1
    fi

    # 创建安装目录
    mkdir -p "${INSTALL_PREFIX}/bin"
    mkdir -p "${INSTALL_PREFIX}/bpf"

    # 安装二进制文件
    cp -f "${binary_source}" "${INSTALL_PREFIX}/bin/${BINARY_NAME}"
    chmod 755 "${INSTALL_PREFIX}/bin/${BINARY_NAME}"

    info "二进制文件已安装到 ${INSTALL_PREFIX}/bin/${BINARY_NAME}"

    # 安装 eBPF 程序（如果存在）
    local bpf_search_paths=(
        "./bpf/tc.bpf.o"
        "./internal/ebpfcollector/bpf/tc.bpf.o"
    )

    for bpf_path in "${bpf_search_paths[@]}"; do
        if [[ -f "${bpf_path}" ]]; then
            cp -f "${bpf_path}" "${INSTALL_PREFIX}/bpf/tc.bpf.o"
            chmod 644 "${INSTALL_PREFIX}/bpf/tc.bpf.o"
            info "eBPF 程序已安装到 ${INSTALL_PREFIX}/bpf/tc.bpf.o"
            break
        fi
    done
}

install_config() {
    step "安装配置文件"

    mkdir -p "${CONFIG_DIR}"

    # 查找配置文件
    local config_source=""
    for arg in "$@"; do
        case "${arg}" in
            --config=*)
                config_source="${arg#*=}"
                ;;
        esac
    done

    if [[ -z "${config_source}" ]]; then
        local config_search_paths=(
            "./configs/config.yaml"
            "./config.yaml"
        )
        for path in "${config_search_paths[@]}"; do
            if [[ -f "${path}" ]]; then
                config_source="${path}"
                break
            fi
        done
    fi

    if [[ -f "${config_source}" ]]; then
        if [[ -f "${CONFIG_FILE}" ]]; then
            # 配置文件已存在，备份
            cp -f "${CONFIG_FILE}" "${CONFIG_FILE}.bak.$(date +%Y%m%d%H%M%S)"
            warn "已备份现有配置文件"
        fi
        cp -f "${config_source}" "${CONFIG_FILE}"
        info "配置文件已安装到 ${CONFIG_FILE}"
    else
        warn "未找到配置文件模板，请手动创建 ${CONFIG_FILE}"
        mkdir -p "${CONFIG_DIR}"
        cat > "${CONFIG_FILE}" << 'EOF'
# Cloud Flow Agent 配置文件
probe_id: ""
edge_addr: "edge:50051"
metrics_port: "9090"
health_port: "8080"
collect_interval: 10
batch_size: 10
log:
  level: "info"
  format: "json"
collect:
  cpu: true
  memory: true
  network: true
  disk: false
EOF
        info "已创建默认配置文件 ${CONFIG_FILE}"
    fi

    chmod 640 "${CONFIG_FILE}"
    chown "${USER_NAME}:${GROUP_NAME}" "${CONFIG_DIR}"
    chown "${USER_NAME}:${GROUP_NAME}" "${CONFIG_FILE}"
}

install_systemd() {
    step "安装 systemd 服务"

    if ! check_systemd; then
        return
    fi

    # 查找 systemd 服务文件
    local service_source=""
    local service_search_paths=(
        "./deployments/systemd/cloud-flow-agent.service"
        "./cloud-flow-agent.service"
    )

    for path in "${service_search_paths[@]}"; do
        if [[ -f "${path}" ]]; then
            service_source="${path}"
            break
        fi
    done

    if [[ -z "${service_source}" ]]; then
        # 使用内联的服务文件
        cat > "${SYSTEMD_DIR}/${SERVICE_NAME}.service" << EOF
[Unit]
Description=Cloud Flow Agent - 云内流量监测探针
Documentation=https://github.com/cloud-flow/cloud-flow-agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${USER_NAME}
Group=${GROUP_NAME}
WorkingDirectory=${INSTALL_PREFIX}
ExecStart=${INSTALL_PREFIX}/bin/${BINARY_NAME} -config ${CONFIG_FILE}
Restart=on-failure
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=60

# 资源限制
LimitNOFILE=65536
LimitNPROC=4096
MemoryMax=512M
CPUQuota=50%

# 安全加固
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${LOG_DIR} ${DATA_DIR} /sys/fs/bpf /proc
PrivateTmp=true
ReadOnlyPaths=${INSTALL_PREFIX}/bin ${INSTALL_PREFIX}/bpf ${CONFIG_DIR}

# 环境变量
Environment=HOME=${INSTALL_PREFIX}

# 日志
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

[Install]
WantedBy=multi-user.target
EOF
    else
        # 使用项目中的服务文件，替换变量
        sed -e "s|{{INSTALL_PREFIX}}|${INSTALL_PREFIX}|g" \
            -e "s|{{CONFIG_FILE}}|${CONFIG_FILE}|g" \
            -e "s|{{USER_NAME}}|${USER_NAME}|g" \
            -e "s|{{GROUP_NAME}}|${GROUP_NAME}|g" \
            -e "s|{{LOG_DIR}}|${LOG_DIR}|g" \
            -e "s|{{DATA_DIR}}|${DATA_DIR}|g" \
            "${service_source}" > "${SYSTEMD_DIR}/${SERVICE_NAME}.service"
    fi

    chmod 644 "${SYSTEMD_DIR}/${SERVICE_NAME}.service"

    # 重新加载 systemd
    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}" 2>/dev/null || true

    info "systemd 服务已安装并启用"
}

create_directories() {
    step "创建必要目录"

    mkdir -p "${LOG_DIR}"
    mkdir -p "${DATA_DIR}"
    mkdir -p /sys/fs/bpf 2>/dev/null || true

    chown -R "${USER_NAME}:${GROUP_NAME}" "${LOG_DIR}"
    chown -R "${USER_NAME}:${GROUP_NAME}" "${DATA_DIR}"
    chmod 750 "${LOG_DIR}"
    chmod 750 "${DATA_DIR}"

    info "目录创建完成"
}

set_permissions() {
    step "设置文件权限"

    chown -R root:root "${INSTALL_PREFIX}"
    chmod 755 "${INSTALL_PREFIX}/bin/${BINARY_NAME}"
    chmod 755 "${INSTALL_PREFIX}/bin"
    chmod 755 "${INSTALL_PREFIX}/bpf"
    chmod 644 "${INSTALL_PREFIX}/bpf/"* 2>/dev/null || true

    # 设置 capabilities（如果支持 eBPF）
    if command -v setcap &>/dev/null; then
        setcap cap_bpf+ep cap_perfmon+ep "${INSTALL_PREFIX}/bin/${BINARY_NAME}" 2>/dev/null || true
        info "已设置 eBPF capabilities"
    fi

    info "权限设置完成"
}

print_summary() {
    local arch vendor kernel_version

    arch=$(detect_arch)
    vendor=$(detect_vendor "${arch}")
    kernel_version=$(detect_kernel_version)

    echo ""
    echo "============================================"
    echo "  Cloud Flow Agent 安装完成"
    echo "============================================"
    echo "  架构:       ${arch}"
    echo "  芯片厂商:   ${vendor}"
    echo "  内核版本:   ${kernel_version}"
    echo "  安装路径:   ${INSTALL_PREFIX}"
    echo "  配置文件:   ${CONFIG_FILE}"
    echo "  日志目录:   ${LOG_DIR}"
    echo "  数据目录:   ${DATA_DIR}"
    echo "  服务名称:   ${SERVICE_NAME}"
    echo "============================================"
    echo ""
    echo "使用以下命令管理服务:"
    echo "  启动:   systemctl start ${SERVICE_NAME}"
    echo "  停止:   systemctl stop ${SERVICE_NAME}"
    echo "  状态:   systemctl status ${SERVICE_NAME}"
    echo "  日志:   journalctl -u ${SERVICE_NAME} -f"
    echo ""
    echo "或使用运维脚本:"
    echo "  ./scripts/agentctl.sh start"
    echo "  ./scripts/agentctl.sh status"
    echo ""
}

# ============================================================================
# 主流程
# ============================================================================

main() {
    # 解析命令行参数
    for arg in "$@"; do
        case "${arg}" in
            --prefix=*)
                INSTALL_PREFIX="${arg#*=}"
                ;;
            --help|-h)
                echo "用法: sudo $0 [选项]"
                echo "  --prefix=PREFIX    安装前缀 (默认: /opt/cloud-flow-agent)"
                echo "  --config=CONFIG    配置文件路径"
                echo "  --binary=BINARY    二进制文件路径"
                echo "  --help             显示帮助"
                exit 0
                ;;
        esac
    done

    echo "============================================"
    echo "  Cloud Flow Agent 安装程序"
    echo "============================================"
    echo ""

    check_root

    # 系统检测
    local arch vendor kernel_version
    arch=$(detect_arch)
    vendor=$(detect_vendor "${arch}")
    kernel_version=$(detect_kernel_version)

    info "系统架构: ${arch}"
    info "芯片厂商: ${vendor}"
    info "内核版本: ${kernel_version}"

    if [[ "${vendor}" == "hygon" ]]; then
        info "检测到海光 (Hygon) 芯片，将使用兼容模式"
    elif [[ "${vendor}" == "kunpeng" ]]; then
        info "检测到鲲鹏 (Kunpeng) 芯片，将使用 ARM64 优化"
    fi

    check_kernel_version "${kernel_version}" || true

    # 执行安装
    create_user
    create_directories
    install_binary "$@"
    install_config "$@"
    install_systemd
    set_permissions

    print_summary
}

main "$@"
