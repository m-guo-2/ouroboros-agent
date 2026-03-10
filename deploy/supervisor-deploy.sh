#!/usr/bin/env bash
#
# supervisor-deploy.sh — 编译 + 安装产物到 /opt/moli，配合 supervisor 使用
#
# 用法:
#   ./deploy/supervisor-deploy.sh deploy   # 编译 + 安装 + 生成 .env 和 wrapper + 重启
#   ./deploy/supervisor-deploy.sh build    # 仅编译（产物留在 repo bin/）
#   ./deploy/supervisor-deploy.sh install  # 仅安装（不编译，把已有产物装进去）
#   ./deploy/supervisor-deploy.sh restart  # 编译 + 安装 + supervisorctl restart
#   ./deploy/supervisor-deploy.sh stop     # supervisorctl stop
#   ./deploy/supervisor-deploy.sh status   # supervisorctl status

set -euo pipefail

# ─── 路径 ───────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ─── 可覆盖参数 ─────────────────────────────────────────────────────────
APP_NAME="${APP_NAME:-moli}"
INSTALL_PREFIX="${INSTALL_PREFIX:-/opt/$APP_NAME}"
CONF_DIR="${CONF_DIR:-${INSTALL_PREFIX}/conf}"
DATA_DIR="${DATA_DIR:-${INSTALL_PREFIX}/data}"
LOG_DIR="${LOG_DIR:-${INSTALL_PREFIX}/logs}"

AGENT_PORT="${AGENT_PORT:-2014}"
QIWEI_PORT="${QIWEI_PORT:-2013}"

# ─── 工具函数 ───────────────────────────────────────────────────────────
info()  { printf '\033[32m[INFO]\033[0m  %s\n' "$*"; }
warn()  { printf '\033[33m[WARN]\033[0m  %s\n' "$*" >&2; }
die()   { printf '\033[31m[ERR]\033[0m  %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"
}

# ─── 编译（各项目独立） ─────────────────────────────────────────────────
build_admin() {
  info "[Admin] 安装依赖 + 编译 SPA..."
  (cd "$REPO_ROOT/admin" && bun install --frozen-lockfile && bun run build)
}

build_agent() {
  info "[Agent] 编译二进制..."
  mkdir -p "$REPO_ROOT/bin"
  (cd "$REPO_ROOT/agent" && CGO_ENABLED=1 go build -o "$REPO_ROOT/bin/agent" ./cmd/agent)
  info "[Agent] → bin/agent"
}

build_qiwei() {
  info "[Qiwei] 编译二进制..."
  mkdir -p "$REPO_ROOT/bin"
  (cd "$REPO_ROOT/channel-qiwei" && CGO_ENABLED=1 go build -o "$REPO_ROOT/bin/channel-qiwei" .)
  info "[Qiwei] → bin/channel-qiwei"
}

build_all() {
  require_cmd go
  require_cmd bun
  build_admin
  build_agent
  build_qiwei
  info "全部编译完成"
}

# ─── 安装 ───────────────────────────────────────────────────────────────
ensure_directories() {
  info "确保目录结构..."
  sudo mkdir -p \
    "$INSTALL_PREFIX/bin" \
    "$INSTALL_PREFIX/admin" \
    "$CONF_DIR" \
    "$DATA_DIR" \
    "$LOG_DIR"
}

install_binaries() {
  info "安装二进制..."
  for name in agent channel-qiwei; do
    local src="$REPO_ROOT/bin/$name"
    local dst="$INSTALL_PREFIX/bin/$name"
    [[ -f "$src" ]] || die "二进制不存在: $src — 请先 build"

    if [[ -f "$dst" ]]; then
      local bak="${dst}.bak.$(date +%Y%m%d%H%M%S)"
      sudo cp "$dst" "$bak"
      info "备份: $(basename "$dst") → $(basename "$bak")"
    fi

    sudo cp "$src" "$dst"
    sudo chmod 0755 "$dst"
    info "已安装: $dst"
  done
}

install_admin_dist() {
  [[ -d "$REPO_ROOT/admin/dist" ]] || die "admin/dist 不存在 — 请先 build"
  info "安装 Admin 静态文件..."
  sudo rm -rf "$INSTALL_PREFIX/admin/dist"
  sudo cp -R "$REPO_ROOT/admin/dist" "$INSTALL_PREFIX/admin/dist"
}

# ─── 生成 .env 文件（仅首次，不覆盖已有） ───────────────────────────────
generate_env_files() {
  info "检查 .env 配置..."

  if [[ -f "$CONF_DIR/agent.env" ]]; then
    warn "已存在，跳过: $CONF_DIR/agent.env"
  else
    sudo tee "$CONF_DIR/agent.env" > /dev/null <<EOF
PORT=${AGENT_PORT}
AGENT_APP_VERSION=prod
AGENT_ID=agent-main
DB_PATH=${DATA_DIR}/config.db
LOG_DIR=${LOG_DIR}
ADMIN_DIST=${INSTALL_PREFIX}/admin/dist
EOF
    sudo chmod 0640 "$CONF_DIR/agent.env"
    info "已生成: $CONF_DIR/agent.env"
  fi

  if [[ -f "$CONF_DIR/qiwei.env" ]]; then
    warn "已存在，跳过: $CONF_DIR/qiwei.env"
  else
    sudo tee "$CONF_DIR/qiwei.env" > /dev/null <<EOF
QIWEI_API_BASE_URL=https://api.qiweapi.com
QIWEI_TOKEN=
QIWEI_GUID=

QIWEI_BOT_PORT=${QIWEI_PORT}
QIWEI_HTTP_TIMEOUT_SECONDS=25
QIWEI_LOG_LEVEL=info

AGENT_ENABLED=true
AGENT_SERVER_URL=http://127.0.0.1:${AGENT_PORT}
AGENT_ID=agent-main
EOF
    sudo chmod 0640 "$CONF_DIR/qiwei.env"
    info "已生成: $CONF_DIR/qiwei.env"
  fi
}

# ─── 生成 wrapper 启动脚本（每次覆盖，因为路径可能变） ─────────────────
generate_wrappers() {
  info "生成 wrapper 启动脚本..."

  sudo tee "$INSTALL_PREFIX/bin/run-agent.sh" > /dev/null <<EOF
#!/usr/bin/env bash
set -euo pipefail
CONF="${CONF_DIR}/agent.env"
if [[ -f "\$CONF" ]]; then set -a; source "\$CONF"; set +a; fi
exec "${INSTALL_PREFIX}/bin/agent"
EOF
  sudo chmod 0755 "$INSTALL_PREFIX/bin/run-agent.sh"

  sudo tee "$INSTALL_PREFIX/bin/run-qiwei.sh" > /dev/null <<EOF
#!/usr/bin/env bash
set -euo pipefail
CONF="${CONF_DIR}/qiwei.env"
if [[ -f "\$CONF" ]]; then set -a; source "\$CONF"; set +a; fi
exec "${INSTALL_PREFIX}/bin/channel-qiwei"
EOF
  sudo chmod 0755 "$INSTALL_PREFIX/bin/run-qiwei.sh"

  info "已生成: run-agent.sh, run-qiwei.sh"
}

install_all() {
  ensure_directories
  install_binaries
  install_admin_dist
  generate_env_files
  generate_wrappers
  info "安装完成"
}

# ─── Supervisor 快捷操作 ────────────────────────────────────────────────
sv_restart() {
  info "重启 ${APP_NAME} 服务组..."
  sudo supervisorctl restart "${APP_NAME}:*"
}

sv_stop() {
  info "停止 ${APP_NAME} 服务组..."
  sudo supervisorctl stop "${APP_NAME}:*"
}

sv_status() {
  sudo supervisorctl status "${APP_NAME}:*" 2>/dev/null || sudo supervisorctl status
}

# ─── 子命令 ─────────────────────────────────────────────────────────────
cmd_deploy() {
  build_all
  install_all
  sv_restart
  echo ""
  info "Agent:  http://127.0.0.1:${AGENT_PORT}  (含 Admin)"
  info "Qiwei:  http://127.0.0.1:${QIWEI_PORT}"
}

cmd_build() {
  build_all
  echo ""
  info "编译完成，产物在 bin/ 下。运行 '$0 install' 安装到 ${INSTALL_PREFIX}/"
}

cmd_install() {
  install_all
}

cmd_restart() {
  build_all
  install_all
  sv_restart
  echo ""
  sv_status
}

cmd_stop() {
  sv_stop
}

cmd_status() {
  sv_status
}

# ─── 入口 ───────────────────────────────────────────────────────────────
usage() {
  cat <<EOF
用法: $0 <command>

命令:
  deploy    编译 + 安装 + 重启 supervisor（首次部署用）
  build     仅编译（产物留在 repo bin/）
  install   仅安装到 ${INSTALL_PREFIX}/（不编译）
  restart   编译 + 安装 + 重启（日常迭代用）
  stop      停止服务
  status    查看服务状态

目录布局 (${INSTALL_PREFIX}/):
  bin/agent              Agent 二进制
  bin/channel-qiwei      Qiwei 二进制
  bin/run-agent.sh       Agent wrapper（source .env + exec）
  bin/run-qiwei.sh       Qiwei wrapper（source .env + exec）
  admin/dist/            Admin 前端静态文件
  conf/agent.env         Agent 环境变量
  conf/qiwei.env         Qiwei 环境变量
  data/                  数据（SQLite 等）
  logs/                  运行日志
EOF
}

main() {
  case "${1:-}" in
    deploy)   cmd_deploy ;;
    build)    cmd_build ;;
    install)  cmd_install ;;
    restart)  cmd_restart ;;
    stop)     cmd_stop ;;
    status)   cmd_status ;;
    -h|--help|help|"") usage ;;
    *)        die "未知命令: $1" ;;
  esac
}

main "$@"
