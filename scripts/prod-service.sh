#!/usr/bin/env bash
#
# Production service helper for the systemd-based moli deployment.
#
# Usage:
#   ./scripts/prod-service.sh --host qny restart
#   ./scripts/prod-service.sh --host qny status
#   ./scripts/prod-service.sh --host qny health
#   ./scripts/prod-service.sh --host qny logs
#
# It can also run directly on the server without --host.

set -euo pipefail

ACTION="status"
HOST=""
SERVICE_PREFIX="${SERVICE_PREFIX:-moli}"
AGENT_PORT="${AGENT_PORT:-1997}"
QIWEI_PORT="${QIWEI_PORT:-2000}"
FEISHU_PORT="${FEISHU_PORT:-1999}"
INCLUDE_FEISHU="${INCLUDE_FEISHU:-0}"
LOG_LINES="${LOG_LINES:-80}"

usage() {
  cat <<EOF
Usage: $0 [--host HOST] [status|restart|health|logs]

Examples:
  $0 --host qny restart
  $0 --host qny status
  $0 health

Environment:
  SERVICE_PREFIX=$SERVICE_PREFIX
  AGENT_PORT=$AGENT_PORT
  QIWEI_PORT=$QIWEI_PORT
  FEISHU_PORT=$FEISHU_PORT
  INCLUDE_FEISHU=$INCLUDE_FEISHU
  LOG_LINES=$LOG_LINES
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --host)
        HOST="${2:-}"
        if [[ -z "$HOST" ]]; then
          echo "missing value for --host" >&2
          exit 2
        fi
        shift 2
        ;;
      -h|--help|help)
        usage
        exit 0
        ;;
      status|restart|health|logs)
        ACTION="$1"
        shift
        ;;
      *)
        echo "unknown argument: $1" >&2
        usage >&2
        exit 2
        ;;
    esac
  done
}

units() {
  printf '%s-agent.service\n' "$SERVICE_PREFIX"
  printf '%s-qiwei.service\n' "$SERVICE_PREFIX"
  if [[ "$INCLUDE_FEISHU" == "1" ]]; then
    printf '%s-feishu.service\n' "$SERVICE_PREFIX"
  fi
}

health_checks() {
  printf 'agent http://127.0.0.1:%s/health\n' "$AGENT_PORT"
  printf 'qiwei http://127.0.0.1:%s/health\n' "$QIWEI_PORT"
  if [[ "$INCLUDE_FEISHU" == "1" ]]; then
    printf 'feishu http://127.0.0.1:%s/health\n' "$FEISHU_PORT"
  fi
}

run_status() {
  systemctl --no-pager --full status $(units) || true
}

run_health() {
  local failed=0
  while read -r name url; do
    printf '==> %s %s\n' "$name" "$url"
    if ! curl -fsS --max-time 5 "$url"; then
      failed=1
      printf '\nhealth check failed: %s\n' "$name" >&2
    fi
    printf '\n'
  done < <(health_checks)
  return "$failed"
}

run_restart() {
  echo "==> restarting services"
  sudo systemctl restart $(units)

  echo "==> service status"
  systemctl --no-pager --full status $(units) || true

  echo "==> health checks"
  run_health
}

run_logs() {
  journalctl -u "$(printf "%s-agent.service" "$SERVICE_PREFIX")" \
    -u "$(printf "%s-qiwei.service" "$SERVICE_PREFIX")" \
    --since "30 min ago" -n "$LOG_LINES" --no-pager
}

run_local() {
  case "$ACTION" in
    status) run_status ;;
    restart) run_restart ;;
    health) run_health ;;
    logs) run_logs ;;
  esac
}

run_remote() {
  if [[ -z "$HOST" ]]; then
    run_local
    return
  fi

  ssh -t "$HOST" \
    "PROD_SERVICE_REMOTE=1 SERVICE_PREFIX='$SERVICE_PREFIX' AGENT_PORT='$AGENT_PORT' QIWEI_PORT='$QIWEI_PORT' FEISHU_PORT='$FEISHU_PORT' INCLUDE_FEISHU='$INCLUDE_FEISHU' LOG_LINES='$LOG_LINES' bash -s -- '$ACTION'" \
    < "$0"
}

parse_args "$@"

if [[ "${PROD_SERVICE_REMOTE:-0}" == "1" ]]; then
  HOST=""
fi

run_remote
