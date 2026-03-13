#!/usr/bin/env bash
#
# supervisor-deploy.sh — 薄包装，实际逻辑在根 Makefile
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

die() { printf '\033[31m[ERR]\033[0m  %s\n' "$*" >&2; exit 1; }

command -v make >/dev/null 2>&1 || die "缺少命令: make"

# 映射子命令到 make 目标
case "${1:-}" in
  deploy)   exec make -C "$REPO_ROOT" -j3 deploy  ;;
  build)    exec make -C "$REPO_ROOT" -j3 build    ;;
  install)  exec make -C "$REPO_ROOT" install      ;;
  restart)  exec make -C "$REPO_ROOT" restart      ;;
  stop)     exec make -C "$REPO_ROOT" stop         ;;
  status)   exec make -C "$REPO_ROOT" status       ;;
  -h|--help|help|"")
    echo "用法: $0 <command>"
    echo ""
    echo "命令:"
    echo "  deploy    编译 + 安装 + 重启  (make -j3 deploy)"
    echo "  build     仅编译              (make -j3 build)"
    echo "  install   仅安装              (make install)"
    echo "  restart   重启服务            (make restart)"
    echo "  stop      停止服务            (make stop)"
    echo "  status    查看状态            (make status)"
    echo ""
    echo "等价于在项目根目录执行 make <command>。推荐直接使用 make。"
    ;;
  *)  die "未知命令: $1" ;;
esac
