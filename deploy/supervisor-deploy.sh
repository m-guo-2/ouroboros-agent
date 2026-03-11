#!/usr/bin/env bash
#
# supervisor-deploy.sh — 编译 + 安装产物到 /opt/moli
#
# 用法:
#   ./deploy/supervisor-deploy.sh deploy   # 编译 + 安装 + 重启
#   ./deploy/supervisor-deploy.sh build    # 仅编译
#   ./deploy/supervisor-deploy.sh install  # 仅安装（不编译）
#   ./deploy/supervisor-deploy.sh restart  # 编译 + 安装 + 重启
#   ./deploy/supervisor-deploy.sh stop     # 停止服务
#   ./deploy/supervisor-deploy.sh status   # 查看状态

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

APP_NAME="${APP_NAME:-moli}"
INSTALL_PREFIX="${INSTALL_PREFIX:-/opt/$APP_NAME}"

info()  { printf '\033[32m[INFO]\033[0m  %s\n' "$*"; }
warn()  { printf '\033[33m[WARN]\033[0m  %s\n' "$*" >&2; }
die()   { printf '\033[31m[ERR]\033[0m  %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"
}

# ─── 编译 ───────────────────────────────────────────────────────────────
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

# ─── 安装（只搬运二进制和前端静态文件） ─────────────────────────────────
install_all() {
  info "安装产物到 $INSTALL_PREFIX/ ..."
  sudo mkdir -p "$INSTALL_PREFIX/bin" "$INSTALL_PREFIX/admin"

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

  [[ -d "$REPO_ROOT/admin/dist" ]] || die "admin/dist 不存在 — 请先 build"
  sudo rm -rf "$INSTALL_PREFIX/admin/dist"
  sudo cp -R "$REPO_ROOT/admin/dist" "$INSTALL_PREFIX/admin/dist"
  info "已安装: $INSTALL_PREFIX/admin/dist/"
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
  info "部署完成"
  sv_status
}

cmd_build() {
  build_all
  echo ""
  info "编译完成，产物在 bin/。运行 '$0 install' 安装到 ${INSTALL_PREFIX}/"
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

cmd_stop() { sv_stop; }
cmd_status() { sv_status; }

# ─── 入口 ───────────────────────────────────────────────────────────────
usage() {
  cat <<EOF
用法: $0 <command>

命令:
  deploy    编译 + 安装 + 重启
  build     仅编译（产物留在 repo bin/）
  install   仅安装到 ${INSTALL_PREFIX}/（不编译）
  restart   编译 + 安装 + 重启（日常迭代用）
  stop      停止服务
  status    查看服务状态

安装产物 (${INSTALL_PREFIX}/):
  bin/agent              Agent 二进制
  bin/channel-qiwei      Qiwei 二进制
  admin/dist/            Admin 前端静态文件
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
