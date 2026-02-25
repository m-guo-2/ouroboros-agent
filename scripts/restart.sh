#!/usr/bin/env bash
#
# restart.sh — 快速关闭所有 cc_code 相关进程并重启服务
#
# 用法:
#   ./scripts/restart.sh          # 关闭全部进程 + 重启 dev:all
#   ./scripts/restart.sh stop     # 仅关闭，不重启
#   ./scripts/restart.sh start    # 仅启动（不先 kill）
#   ./scripts/restart.sh status   # 查看当前运行状态
#

set -euo pipefail

# ── 项目根目录 ──
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ── 颜色 ──
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# ── 服务端口映射 ──
declare -A SERVICE_PORTS=(
  [server]=1997
  [agent]=1996
  [admin]=5173
  [feishu]=1999
  [qiwei]=2000
)

# ── 工具函数 ──

info()  { echo -e "${CYAN}ℹ${NC}  $*"; }
ok()    { echo -e "${GREEN}✓${NC}  $*"; }
warn()  { echo -e "${YELLOW}⚠${NC}  $*"; }
err()   { echo -e "${RED}✗${NC}  $*"; }
header(){ echo -e "\n${BOLD}$*${NC}"; }

# 获取所有与项目相关的进程 PID（排除自身和 grep）
get_project_pids() {
  local pids=""

  # 1) 通过端口查找（最可靠）
  for port in "${SERVICE_PORTS[@]}"; do
    local port_pids
    port_pids=$(lsof -ti :"$port" 2>/dev/null || true)
    if [[ -n "$port_pids" ]]; then
      pids="$pids $port_pids"
    fi
  done

  # 2) 通过进程名/路径匹配 cc_code 相关进程
  local path_pids
  path_pids=$(ps aux | grep -E "$PROJECT_ROOT" | grep -v grep | grep -v "restart.sh" | awk '{print $2}' || true)
  if [[ -n "$path_pids" ]]; then
    pids="$pids $path_pids"
  fi

  # 3) 查找 claude-agent-sdk 子进程（agent spawn 的 SDK runner）
  local sdk_pids
  sdk_pids=$(ps aux | grep "claude-agent-sdk" | grep -v grep | awk '{print $2}' || true)
  if [[ -n "$sdk_pids" ]]; then
    pids="$pids $sdk_pids"
  fi

  # 去重 + 排除自身
  echo "$pids" | tr ' ' '\n' | sort -u | grep -v "^$$\$" | grep -v "^$" || true
}

# 显示进程状态
show_status() {
  header "📊 服务状态"
  echo ""

  local any_running=false
  for name in server admin agent feishu qiwei; do
    local port="${SERVICE_PORTS[$name]}"
    local pid
    pid=$(lsof -ti :"$port" 2>/dev/null | head -1 || true)
    if [[ -n "$pid" ]]; then
      local cmd
      cmd=$(ps -p "$pid" -o comm= 2>/dev/null || echo "unknown")
      echo -e "  ${GREEN}●${NC}  ${BOLD}${name}${NC}\t:${port}\tPID ${pid} (${cmd})"
      any_running=true
    else
      echo -e "  ${RED}○${NC}  ${BOLD}${name}${NC}\t:${port}\t-"
    fi
  done

  # 检查 SDK runner 子进程
  local sdk_count
  sdk_count=$(ps aux | grep "claude-agent-sdk" | grep -v grep | wc -l | tr -d ' ')
  if [[ "$sdk_count" -gt 0 ]]; then
    echo -e "\n  ${YELLOW}⚡${NC} ${BOLD}SDK runners${NC}: ${sdk_count} 个活跃"
    any_running=true
  fi

  echo ""
  if ! $any_running; then
    info "没有运行中的服务"
  fi
}

# 停止所有进程
do_stop() {
  header "🛑 停止所有服务"

  local pids
  pids=$(get_project_pids)

  if [[ -z "$pids" ]]; then
    ok "没有需要停止的进程"
    return 0
  fi

  local count
  count=$(echo "$pids" | wc -w | tr -d ' ')
  info "发现 ${count} 个相关进程，正在停止..."

  # 第一轮: SIGTERM（优雅关闭）
  echo "$pids" | xargs kill 2>/dev/null || true
  
  # 等待进程退出（最多 3 秒）
  local waited=0
  while [[ $waited -lt 3 ]]; do
    sleep 1
    waited=$((waited + 1))
    local remaining
    remaining=$(get_project_pids)
    if [[ -z "$remaining" ]]; then
      ok "所有进程已优雅退出"
      return 0
    fi
  done

  # 第二轮: SIGKILL（强制终止残留进程）
  local remaining
  remaining=$(get_project_pids)
  if [[ -n "$remaining" ]]; then
    local remain_count
    remain_count=$(echo "$remaining" | wc -w | tr -d ' ')
    warn "${remain_count} 个进程未响应 SIGTERM，强制终止..."
    echo "$remaining" | xargs kill -9 2>/dev/null || true
    sleep 1
  fi

  # 最终确认
  remaining=$(get_project_pids)
  if [[ -z "$remaining" ]]; then
    ok "所有进程已停止"
  else
    err "部分进程可能仍在运行:"
    echo "$remaining" | while read -r pid; do
      ps -p "$pid" -o pid=,comm=,args= 2>/dev/null || true
    done
  fi
}

# 启动服务
do_start() {
  header "🚀 启动所有服务"
  
  cd "$PROJECT_ROOT"
  
  info "运行 bun run dev:all ..."
  echo ""

  # 使用 exec 替换当前 shell，让用户可以直接 Ctrl+C 停止
  exec bun run dev:all
}

# ── 主逻辑 ──

ACTION="${1:-restart}"

case "$ACTION" in
  stop)
    do_stop
    echo ""
    show_status
    ;;
  start)
    do_start
    ;;
  status|st)
    show_status
    ;;
  restart|"")
    do_stop
    echo ""
    do_start
    ;;
  -h|--help|help)
    echo "用法: $0 [stop|start|restart|status]"
    echo ""
    echo "  restart  (默认) 关闭所有进程并重启 dev:all"
    echo "  stop     仅关闭所有相关进程"
    echo "  start    仅启动 dev:all"
    echo "  status   查看当前运行状态"
    echo ""
    echo "也可以使用 bun run 启动各种组合:"
    echo "  bun run dev          server + admin"
    echo "  bun run dev:all      全部 5 个服务"
    echo "  bun run dev:core     server + admin + agent"
    echo "  bun run dev:channels feishu + qiwei"
    echo "  bun run dev:server   仅 server (:1997)"
    echo "  bun run dev:admin    仅 admin  (:5173)"
    echo "  bun run dev:agent    仅 agent  (:1996)"
    echo "  bun run dev:feishu   仅 feishu (:1999)"
    echo "  bun run dev:qiwei    仅 qiwei  (:2000)"
    ;;
  *)
    err "未知命令: $ACTION"
    echo "用法: $0 [stop|start|restart|status]"
    exit 1
    ;;
esac
