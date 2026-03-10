#!/usr/bin/env bash
#
# CliRelay 一键部署脚本 (Docker 版)
#
# curl -fsSL https://raw.githubusercontent.com/kittors/CliRelay/main/install.sh | bash
#
set -euo pipefail

# ── 颜色 & 符号 ─────────────────────────────────────────────────────────────
C_RESET='\033[0m';  C_BOLD='\033[1m';  C_DIM='\033[2m'
C_RED='\033[0;31m'; C_GREEN='\033[0;32m'; C_YELLOW='\033[0;33m'
C_BLUE='\033[0;34m'; C_CYAN='\033[0;36m'; C_WHITE='\033[1;37m'
C_BG_BLUE='\033[44m'; C_BG_GREEN='\033[42m'; C_BG_RED='\033[41m'

SYM_OK="✓"; SYM_FAIL="✗"; SYM_ARROW="→"; SYM_DOT="·"; SYM_STAR="★"

# ── 配置 ─────────────────────────────────────────────────────────────────────
DOCKER_IMAGE="ghcr.io/kittors/clirelay:latest"
INSTALL_DIR="${CLIRELAY_DIR:-/opt/clirelay}"
CONTAINER_NAME="clirelay"
DEFAULT_PORT=8317

# ── TUI ──────────────────────────────────────────────────────────────────────

banner() {
    echo ""
    echo -e "${C_CYAN}${C_BOLD}"
    cat << 'EOF'
   ██████╗██╗     ██╗██████╗ ███████╗██╗      █████╗ ██╗   ██╗
  ██╔════╝██║     ██║██╔══██╗██╔════╝██║     ██╔══██╗╚██╗ ██╔╝
  ██║     ██║     ██║██████╔╝█████╗  ██║     ███████║ ╚████╔╝
  ██║     ██║     ██║██╔══██╗██╔══╝  ██║     ██╔══██║  ╚██╔╝
  ╚██████╗███████╗██║██║  ██║███████╗███████╗██║  ██║   ██║
   ╚═════╝╚══════╝╚═╝╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝   ╚═╝
EOF
    echo -e "${C_RESET}"
    echo -e "  ${C_DIM}AI Proxy Gateway ${SYM_DOT} Docker 一键部署${C_RESET}"
    echo -e "  ${C_DIM}────────────────────────────────────────────────${C_RESET}"
    echo ""
}

info()    { echo -e "  ${C_BLUE}${SYM_ARROW}${C_RESET} $*"; }
success() { echo -e "  ${C_GREEN}${SYM_OK}${C_RESET} $*"; }
warn()    { echo -e "  ${C_YELLOW}!${C_RESET} $*"; }
fail()    { echo -e "  ${C_RED}${SYM_FAIL}${C_RESET} $*"; exit 1; }
step()    { echo -e "\n${C_WHITE}${C_BOLD}  [$1/$TOTAL_STEPS] $2${C_RESET}"; }

progress_bar() {
    local current=$1 total=$2 width=40
    local filled=$((current * width / total))
    local empty=$((width - filled))
    local bar=""
    for ((i=0; i<filled; i++)); do bar+="█"; done
    for ((i=0; i<empty; i++)); do bar+="░"; done
    printf "\r  ${C_CYAN}[${bar}]${C_RESET} ${C_BOLD}%3d%%${C_RESET}" $((current * 100 / total))
}

spin_exec() {
    local msg="$1"; shift
    local spinchars='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    local tmplog; tmplog=$(mktemp)
    "$@" > "$tmplog" 2>&1 &
    local pid=$! i=0
    while kill -0 "$pid" 2>/dev/null; do
        printf "\r  ${C_CYAN}%s${C_RESET} %s" "${spinchars:i%${#spinchars}:1}" "$msg"
        i=$((i + 1)); sleep 0.1
    done
    wait "$pid"; local rc=$?
    printf '\r\033[K'
    if [[ $rc -eq 0 ]]; then success "$msg"
    else fail "$msg (查看日志: $(tail -3 "$tmplog"))"; fi
    rm -f "$tmplog"
}

rand_hex() { openssl rand -hex "$1" 2>/dev/null || head -c $(($1*2)) /dev/urandom | xxd -p | head -c $(($1*2)); }

# ── 依赖检查 ─────────────────────────────────────────────────────────────────

check_docker() {
    if ! command -v docker &>/dev/null; then
        warn "Docker 未安装，正在自动安装..."
        if command -v curl &>/dev/null; then
            curl -fsSL https://get.docker.com | sh
        elif command -v wget &>/dev/null; then
            wget -qO- https://get.docker.com | sh
        else
            fail "请先安装 curl 或 wget"
        fi
        systemctl enable docker 2>/dev/null || true
        systemctl start docker 2>/dev/null || true
        success "Docker 安装完成"
    else
        success "Docker $(docker --version | awk '{print $3}' | tr -d ',') 已安装"
    fi

    if ! command -v docker compose &>/dev/null && ! docker compose version &>/dev/null 2>&1; then
        warn "Docker Compose 未安装，正在安装 plugin..."
        mkdir -p ~/.docker/cli-plugins
        local arch; arch=$(uname -m)
        case "$arch" in aarch64) arch="aarch64" ;; x86_64) arch="x86_64" ;; esac
        curl -fsSL "https://github.com/docker/compose/releases/latest/download/docker-compose-linux-${arch}" \
            -o ~/.docker/cli-plugins/docker-compose
        chmod +x ~/.docker/cli-plugins/docker-compose
        success "Docker Compose 安装完成"
    else
        success "Docker Compose 已安装"
    fi
}

# ── 交互式配置 ───────────────────────────────────────────────────────────────

prompt_config() {
    echo ""
    echo -e "  ${C_BG_BLUE}${C_WHITE}${C_BOLD} 服务配置 ${C_RESET}"
    echo ""

    read -r -p "  $(echo -e "${C_CYAN}?${C_RESET}") 服务端口 [${DEFAULT_PORT}]: " CFG_PORT
    CFG_PORT="${CFG_PORT:-$DEFAULT_PORT}"

    read -r -p "  $(echo -e "${C_CYAN}?${C_RESET}") 管理面板密钥 (留空自动生成): " CFG_SECRET
    if [[ -z "$CFG_SECRET" ]]; then
        CFG_SECRET="$(rand_hex 16)"
        echo -e "  ${C_DIM}  已生成: ${CFG_SECRET}${C_RESET}"
    fi

    read -r -p "  $(echo -e "${C_CYAN}?${C_RESET}") 客户端 API Key (留空自动生成): " CFG_API_KEY
    if [[ -z "$CFG_API_KEY" ]]; then
        CFG_API_KEY="sk-$(rand_hex 16)"
        echo -e "  ${C_DIM}  已生成: ${CFG_API_KEY}${C_RESET}"
    fi

    read -r -p "  $(echo -e "${C_CYAN}?${C_RESET}") 允许远程管理? [Y/n]: " yn
    case "$yn" in [Nn]*) CFG_REMOTE="false" ;; *) CFG_REMOTE="true" ;; esac

    echo ""
    echo -e "  ${C_GREEN}${SYM_OK}${C_RESET} 配置确认:"
    echo -e "    ${C_DIM}端口:${C_RESET}     ${CFG_PORT}"
    echo -e "    ${C_DIM}管理密钥:${C_RESET} ${CFG_SECRET:0:8}****"
    echo -e "    ${C_DIM}API Key:${C_RESET}  ${CFG_API_KEY:0:10}****"
    echo -e "    ${C_DIM}远程管理:${C_RESET} ${CFG_REMOTE}"
}

write_config() {
    cat > "${INSTALL_DIR}/config.yaml" << YAML
# CliRelay 配置文件 - 由部署脚本自动生成
host: ""
port: ${CFG_PORT}

redis:
  enable: false

remote-management:
  allow-remote: ${CFG_REMOTE}
  secret-key: "${CFG_SECRET}"
  disable-control-panel: false

auth-dir: "/root/.cli-proxy-api"

api-keys:
  - "${CFG_API_KEY}"

debug: false
logging-to-file: true
logs-max-total-size-mb: 100
usage-statistics-enabled: true
request-retry: 3
max-retry-interval: 30
routing:
  strategy: "round-robin"
YAML
}

write_compose() {
    cat > "${INSTALL_DIR}/docker-compose.yml" << YAML
services:
  clirelay:
    image: ${DOCKER_IMAGE}
    container_name: ${CONTAINER_NAME}
    ports:
      - "${CFG_PORT}:${CFG_PORT}"
    volumes:
      - ./config.yaml:/CLIProxyAPI/config.yaml
      - ./auths:/root/.cli-proxy-api
      - ./logs:/CLIProxyAPI/logs
      - ./data:/CLIProxyAPI/data
    restart: unless-stopped
    environment:
      TZ: Asia/Shanghai
YAML
}

# ── 显示结果 ─────────────────────────────────────────────────────────────────

show_result() {
    local public_ip
    public_ip="$(curl -s --max-time 5 https://api.ipify.org 2>/dev/null || curl -s --max-time 5 https://ifconfig.me 2>/dev/null || echo "YOUR_SERVER_IP")"

    echo ""
    echo -e "  ${C_BG_GREEN}${C_WHITE}${C_BOLD} 部署完成 ${C_RESET}"
    echo ""
    echo -e "  ${C_GREEN}${SYM_STAR}${C_RESET} ${C_BOLD}CliRelay 已成功部署！${C_RESET}"
    echo ""
    echo -e "  ${C_DIM}┌─────────────────────────────────────────────────────────────┐${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}                                                             ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}  ${C_CYAN}${C_BOLD}API 端点${C_RESET}                                                  ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}  http://${public_ip}:${CFG_PORT}/v1/chat/completions          ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}                                                             ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}  ${C_CYAN}${C_BOLD}管理面板${C_RESET}                                                  ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}  http://${public_ip}:${CFG_PORT}/manage                      ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}                                                             ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}  ${C_CYAN}管理密钥${C_RESET}  ${CFG_SECRET}  ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}  ${C_CYAN}API Key ${C_RESET}  ${CFG_API_KEY}  ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}│${C_RESET}                                                             ${C_DIM}│${C_RESET}"
    echo -e "  ${C_DIM}└─────────────────────────────────────────────────────────────┘${C_RESET}"
    echo ""
    echo -e "  ${C_YELLOW}${C_BOLD}建议下一步${C_RESET}: 配置反向代理 (Nginx/Caddy) 并绑定域名 + SSL"
    echo ""
    echo -e "  ${C_DIM}Nginx 反向代理示例:${C_RESET}"
    echo -e "    ${C_DIM}server {${C_RESET}"
    echo -e "    ${C_DIM}    listen 443 ssl;${C_RESET}"
    echo -e "    ${C_DIM}    server_name your-domain.com;${C_RESET}"
    echo -e "    ${C_DIM}    location / { proxy_pass http://127.0.0.1:${CFG_PORT}; }${C_RESET}"
    echo -e "    ${C_DIM}}${C_RESET}"
    echo ""
    echo -e "  ${C_DIM}常用命令:${C_RESET}"
    echo -e "    ${C_YELLOW}cd ${INSTALL_DIR} && docker compose logs -f${C_RESET}   查看日志"
    echo -e "    ${C_YELLOW}cd ${INSTALL_DIR} && docker compose restart${C_RESET}   重启服务"
    echo -e "    ${C_YELLOW}cd ${INSTALL_DIR} && docker compose pull && docker compose up -d${C_RESET}  更新"
    echo ""
}

# ── 主流程 ───────────────────────────────────────────────────────────────────

TOTAL_STEPS=5

main() {
    banner

    info "系统: ${C_BOLD}$(uname -s)/$(uname -m)${C_RESET}"

    # 检查更新
    local is_update=false
    if [[ -d "$INSTALL_DIR" ]] && [[ -f "${INSTALL_DIR}/docker-compose.yml" ]]; then
        warn "检测到已有安装于 ${INSTALL_DIR}"
        read -r -p "  $(echo -e "${C_CYAN}?${C_RESET}") 选择操作 [1=更新 2=重装 3=取消]: " choice
        case "$choice" in
            1) is_update=true ;;
            3) echo "  取消。"; exit 0 ;;
            *) is_update=false ;;
        esac
    fi

    # ── Step 1 ──
    step 1 "检查 Docker 环境"
    check_docker

    # ── Step 2 ──
    step 2 "配置服务参数"
    if [[ "$is_update" == "true" ]]; then
        success "保留现有配置: ${INSTALL_DIR}/config.yaml"
        CFG_PORT="$(grep -E '^port:' "${INSTALL_DIR}/config.yaml" 2>/dev/null | awk '{print $2}' || echo "$DEFAULT_PORT")"
        CFG_SECRET="$(grep -E 'secret-key:' "${INSTALL_DIR}/config.yaml" 2>/dev/null | sed 's/.*: *"\{0,1\}\(.*\)"\{0,1\}/\1/' | tr -d '"' || echo "")"
        CFG_API_KEY="$(grep -E '^ *- "sk-' "${INSTALL_DIR}/config.yaml" 2>/dev/null | head -1 | sed 's/.*"\(.*\)".*/\1/' || echo "")"
        CFG_REMOTE="true"
    else
        prompt_config
        mkdir -p "$INSTALL_DIR"
        write_config
        success "配置已写入 ${INSTALL_DIR}/config.yaml"
    fi

    write_compose

    # ── Step 3 ──
    step 3 "拉取 Docker 镜像"
    spin_exec "拉取 ${DOCKER_IMAGE}" docker pull "$DOCKER_IMAGE"

    # ── Step 4 ──
    step 4 "启动服务"
    cd "$INSTALL_DIR"

    if [[ "$is_update" == "true" ]]; then
        info "停止旧容器..."
        docker compose down 2>/dev/null || true
    fi

    docker compose up -d

    # 等待就绪
    info "等待服务就绪..."
    local ready=false
    for i in $(seq 1 20); do
        progress_bar "$i" 20
        if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:${CFG_PORT}/" 2>/dev/null; then
            ready=true; break
        fi
        sleep 1
    done
    echo ""

    if [[ "$ready" == "true" ]]; then
        success "服务已启动并就绪"
    else
        warn "服务可能需要更多时间启动，请稍后检查: docker compose logs -f"
    fi

    # ── Step 5 ──
    step 5 "验证部署"
    local http_code
    http_code="$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "http://127.0.0.1:${CFG_PORT}/" 2>/dev/null || echo "000")"
    if [[ "$http_code" == "200" ]]; then
        success "API 服务响应正常 (HTTP ${http_code})"
    else
        warn "API 服务响应: HTTP ${http_code}，请检查日志"
    fi

    local panel_code
    panel_code="$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "http://127.0.0.1:${CFG_PORT}/manage" 2>/dev/null || echo "000")"
    if [[ "$panel_code" == "200" ]]; then
        success "管理面板响应正常 (HTTP ${panel_code})"
    else
        warn "管理面板响应: HTTP ${panel_code}"
    fi

    show_result
}

# ── 卸载 ─────────────────────────────────────────────────────────────────────

uninstall() {
    banner
    echo -e "  ${C_BG_RED}${C_WHITE}${C_BOLD} 卸载 CliRelay ${C_RESET}"
    echo ""
    read -r -p "  $(echo -e "${C_RED}?${C_RESET}") 确认卸载? 配置和数据将被删除 [y/N]: " yn
    case "$yn" in
        [Yy]*)
            if [[ -f "${INSTALL_DIR}/docker-compose.yml" ]]; then
                cd "$INSTALL_DIR" && docker compose down --rmi all 2>/dev/null || true
            fi
            rm -rf "$INSTALL_DIR"
            success "CliRelay 已完全卸载"
            ;;
        *) info "取消卸载" ;;
    esac
}

# ── 入口 ─────────────────────────────────────────────────────────────────────
case "${1:-}" in
    --uninstall|uninstall) uninstall ;;
    --help|-h)
        banner
        echo "  用法:"
        echo "    bash install.sh              安装或更新"
        echo "    bash install.sh --uninstall   卸载"
        echo ""
        echo "  一键安装:"
        echo "    curl -fsSL https://raw.githubusercontent.com/kittors/CliRelay/main/install.sh | bash"
        echo ""
        ;;
    *) main ;;
esac
