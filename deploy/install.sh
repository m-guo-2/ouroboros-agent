#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

APP_NAME="${APP_NAME:-moli}"
RUN_USER="${RUN_USER:-$APP_NAME}"
RUN_GROUP="${RUN_GROUP:-$APP_NAME}"
INSTALL_PREFIX="${INSTALL_PREFIX:-/opt/$APP_NAME}"
CONFIG_DIR="${CONFIG_DIR:-/etc/$APP_NAME}"
DATA_DIR="${DATA_DIR:-/var/lib/$APP_NAME}"
LOG_DIR="${LOG_DIR:-/var/log/$APP_NAME}"
SYSTEMD_DIR="${SYSTEMD_DIR:-/etc/systemd/system}"
NGINX_SITES_AVAILABLE="${NGINX_SITES_AVAILABLE:-/etc/nginx/sites-available}"
NGINX_SITES_ENABLED="${NGINX_SITES_ENABLED:-/etc/nginx/sites-enabled}"
SERVER_NAME="${SERVER_NAME:-_}"
ENABLE_QIWEI="${ENABLE_QIWEI:-1}"
ENABLE_FEISHU="${ENABLE_FEISHU:-0}"
INSTALL_NGINX="${INSTALL_NGINX:-1}"
RELOAD_NGINX="${RELOAD_NGINX:-1}"
BOOTSTRAP_AFTER_INSTALL="${BOOTSTRAP_AFTER_INSTALL:-1}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

info() {
  printf '[INFO] %s\n' "$*"
}

warn() {
  printf '[WARN] %s\n' "$*" >&2
}

die() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    die "run install.sh as root"
  fi
}

require_cmd() {
  local cmd="$1"
  command -v "$cmd" >/dev/null 2>&1 || die "missing required command: $cmd"
}

bool_enabled() {
  case "${1:-0}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

render_template() {
  local src="$1"
  local dst="$2"
  sed \
    -e "s|__APP_NAME__|$APP_NAME|g" \
    -e "s|__RUN_USER__|$RUN_USER|g" \
    -e "s|__RUN_GROUP__|$RUN_GROUP|g" \
    -e "s|__INSTALL_PREFIX__|$INSTALL_PREFIX|g" \
    -e "s|__CONFIG_DIR__|$CONFIG_DIR|g" \
    -e "s|__DATA_DIR__|$DATA_DIR|g" \
    -e "s|__LOG_DIR__|$LOG_DIR|g" \
    -e "s|__SERVER_NAME__|$SERVER_NAME|g" \
    "$src" >"$dst"
}

install_env_if_missing() {
  local src="$1"
  local dst="$2"
  if [[ -f "$dst" ]]; then
    warn "keeping existing env file: $dst"
    return
  fi
  local tmp="$TMP_DIR/$(basename "$dst")"
  render_template "$src" "$tmp"
  install -m 0640 "$tmp" "$dst"
}

ensure_user_group() {
  if ! getent group "$RUN_GROUP" >/dev/null 2>&1; then
    info "creating group: $RUN_GROUP"
    groupadd --system "$RUN_GROUP"
  fi

  if ! id -u "$RUN_USER" >/dev/null 2>&1; then
    info "creating user: $RUN_USER"
    useradd \
      --system \
      --gid "$RUN_GROUP" \
      --home-dir "$INSTALL_PREFIX" \
      --create-home \
      --shell /usr/sbin/nologin \
      "$RUN_USER"
  fi
}

build_admin() {
  info "building admin dist"
  (
    cd "$REPO_ROOT/admin"
    bun install --frozen-lockfile
    bun run build
  )
}

build_agent() {
  info "building agent binary"
  (
    cd "$REPO_ROOT/agent"
    go build -o "$TMP_DIR/agent" ./cmd/agent
  )
}

build_qiwei() {
  if ! bool_enabled "$ENABLE_QIWEI"; then
    return
  fi
  info "building qiwei binary"
  (
    cd "$REPO_ROOT/channel-qiwei"
    go build -o "$TMP_DIR/channel-qiwei" .
  )
}

build_feishu() {
  if ! bool_enabled "$ENABLE_FEISHU"; then
    return
  fi
  info "building feishu binary"
  (
    cd "$REPO_ROOT/channel-feishu"
    go build -o "$TMP_DIR/channel-feishu" .
  )
}

install_directories() {
  info "creating install directories"
  install -d -m 0755 "$INSTALL_PREFIX/bin" "$INSTALL_PREFIX/admin" "$DATA_DIR" "$LOG_DIR"
  install -d -m 0750 "$CONFIG_DIR"
}

install_binaries() {
  info "installing binaries"
  install -m 0755 "$TMP_DIR/agent" "$INSTALL_PREFIX/bin/agent"
  install -m 0755 "$REPO_ROOT/deploy/bootstrap.sh" "$INSTALL_PREFIX/bin/bootstrap"

  if bool_enabled "$ENABLE_QIWEI"; then
    install -m 0755 "$TMP_DIR/channel-qiwei" "$INSTALL_PREFIX/bin/channel-qiwei"
  fi

  if bool_enabled "$ENABLE_FEISHU"; then
    install -m 0755 "$TMP_DIR/channel-feishu" "$INSTALL_PREFIX/bin/channel-feishu"
  fi
}

install_admin_dist() {
  info "installing admin dist"
  rm -rf "$INSTALL_PREFIX/admin/dist"
  cp -R "$REPO_ROOT/admin/dist" "$INSTALL_PREFIX/admin/dist"
}

install_default_prompt() {
  local src="$REPO_ROOT/agent/data/moli-system-prompt.md"
  local dst="$CONFIG_DIR/default-system-prompt.md"
  if [[ -f "$src" && ! -f "$dst" ]]; then
    install -m 0640 "$src" "$dst"
  fi
}

install_env_files() {
  info "installing env templates"
  install_env_if_missing "$REPO_ROOT/deploy/env/agent.env.example" "$CONFIG_DIR/agent.env"
  install_env_if_missing "$REPO_ROOT/deploy/env/bootstrap.env.example" "$CONFIG_DIR/bootstrap.env"

  if bool_enabled "$ENABLE_QIWEI"; then
    install_env_if_missing "$REPO_ROOT/deploy/env/qiwei.env.example" "$CONFIG_DIR/qiwei.env"
  fi

  if bool_enabled "$ENABLE_FEISHU"; then
    install_env_if_missing "$REPO_ROOT/deploy/env/feishu.env.example" "$CONFIG_DIR/feishu.env"
  fi
}

install_systemd_unit() {
  local template="$1"
  local output_name="$2"
  local tmp="$TMP_DIR/$output_name"
  render_template "$template" "$tmp"
  install -m 0644 "$tmp" "$SYSTEMD_DIR/$output_name"
}

install_systemd_units() {
  info "installing systemd units"
  install_systemd_unit "$REPO_ROOT/deploy/systemd/moli-agent.service" "$APP_NAME-agent.service"

  if bool_enabled "$ENABLE_QIWEI"; then
    install_systemd_unit "$REPO_ROOT/deploy/systemd/moli-qiwei.service" "$APP_NAME-qiwei.service"
  fi

  if bool_enabled "$ENABLE_FEISHU"; then
    install_systemd_unit "$REPO_ROOT/deploy/systemd/moli-feishu.service" "$APP_NAME-feishu.service"
  fi

  systemctl daemon-reload
}

install_nginx_config() {
  if ! bool_enabled "$INSTALL_NGINX"; then
    return
  fi
  command -v nginx >/dev/null 2>&1 || {
    warn "nginx not found; skipping nginx config installation"
    return
  }

  info "installing nginx site"
  install -d -m 0755 "$NGINX_SITES_AVAILABLE" "$NGINX_SITES_ENABLED"
  local tmp="$TMP_DIR/$APP_NAME-nginx.conf"
  render_template "$REPO_ROOT/deploy/nginx/moli.conf" "$tmp"
  install -m 0644 "$tmp" "$NGINX_SITES_AVAILABLE/$APP_NAME.conf"
  ln -sfn "$NGINX_SITES_AVAILABLE/$APP_NAME.conf" "$NGINX_SITES_ENABLED/$APP_NAME.conf"

  if bool_enabled "$RELOAD_NGINX"; then
    nginx -t
    systemctl reload nginx
  fi
}

fix_ownership() {
  info "setting directory ownership"
  chown -R "$RUN_USER:$RUN_GROUP" "$INSTALL_PREFIX" "$DATA_DIR" "$LOG_DIR"
}

enable_services() {
  info "enabling services"
  systemctl enable --now "$APP_NAME-agent.service"

  if bool_enabled "$ENABLE_QIWEI"; then
    systemctl enable --now "$APP_NAME-qiwei.service"
  fi

  if bool_enabled "$ENABLE_FEISHU"; then
    systemctl enable --now "$APP_NAME-feishu.service"
  fi
}

run_bootstrap() {
  if ! bool_enabled "$BOOTSTRAP_AFTER_INSTALL"; then
    warn "skipping bootstrap; run $INSTALL_PREFIX/bin/bootstrap after editing env files"
    return
  fi

  info "running bootstrap"
  AGENT_ENV_FILE="$CONFIG_DIR/agent.env" \
    BOOTSTRAP_ENV_FILE="$CONFIG_DIR/bootstrap.env" \
    "$INSTALL_PREFIX/bin/bootstrap"

  info "restarting agent to pick up bootstrapped settings"
  systemctl restart "$APP_NAME-agent.service"

  if bool_enabled "$ENABLE_QIWEI"; then
    systemctl restart "$APP_NAME-qiwei.service"
  fi

  if bool_enabled "$ENABLE_FEISHU"; then
    systemctl restart "$APP_NAME-feishu.service"
  fi
}

print_summary() {
  cat <<EOF

Installation complete.

Installed paths:
  prefix:      $INSTALL_PREFIX
  config:      $CONFIG_DIR
  data:        $DATA_DIR
  logs:        $LOG_DIR

Services:
  $APP_NAME-agent.service
EOF

  if bool_enabled "$ENABLE_QIWEI"; then
    printf '  %s\n' "$APP_NAME-qiwei.service"
  fi

  if bool_enabled "$ENABLE_FEISHU"; then
    printf '  %s\n' "$APP_NAME-feishu.service"
  fi

  cat <<EOF

Next checks:
  sudo systemctl status $APP_NAME-agent
  sudo journalctl -u $APP_NAME-agent -n 100 --no-pager
  curl http://127.0.0.1:1997/health
EOF
}

main() {
  require_root
  require_cmd go
  require_cmd bun
  require_cmd python3
  require_cmd systemctl
  require_cmd sed
  require_cmd install
  require_cmd cp

  ensure_user_group
  build_admin
  build_agent
  build_qiwei
  build_feishu
  install_directories
  install_binaries
  install_admin_dist
  install_default_prompt
  install_env_files
  install_systemd_units
  install_nginx_config
  fix_ownership
  enable_services
  run_bootstrap
  print_summary
}

main "$@"
