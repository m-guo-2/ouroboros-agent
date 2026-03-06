#!/usr/bin/env bash

set -euo pipefail

APP_NAME="${APP_NAME:-moli}"
CONFIG_DIR="${CONFIG_DIR:-/etc/$APP_NAME}"
AGENT_ENV_FILE="${AGENT_ENV_FILE:-$CONFIG_DIR/agent.env}"
BOOTSTRAP_ENV_FILE="${BOOTSTRAP_ENV_FILE:-$CONFIG_DIR/bootstrap.env}"
WAIT_SECONDS="${WAIT_SECONDS:-20}"

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

load_env_file() {
  local file="$1"
  if [[ ! -f "$file" ]]; then
    warn "env file not found: $file"
    return
  fi
  set -a
  # shellcheck disable=SC1090
  source "$file"
  set +a
}

bool_enabled() {
  case "${1:-0}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

wait_for_db() {
  local waited=0
  while [[ ! -f "$DB_PATH" ]]; do
    if (( waited >= WAIT_SECONDS )); then
      die "database not found at $DB_PATH"
    fi
    sleep 1
    waited=$((waited + 1))
  done
}

main() {
  load_env_file "$AGENT_ENV_FILE"
  load_env_file "$BOOTSTRAP_ENV_FILE"

  : "${DB_PATH:?DB_PATH is required in $AGENT_ENV_FILE}"
  command -v python3 >/dev/null 2>&1 || die "python3 is required"

  wait_for_db

  info "bootstrapping sqlite settings in $DB_PATH"
  python3 - "$DB_PATH" <<'PY'
import json
import os
import sqlite3
import sys

db_path = sys.argv[1]
conn = sqlite3.connect(db_path)
cur = conn.cursor()


def env(name, default=""):
    return os.environ.get(name, default).strip()


def upsert_setting(key, value):
    cur.execute(
        """
        INSERT INTO settings (key, value, updated_at)
        VALUES (?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(key) DO UPDATE SET
          value = excluded.value,
          updated_at = CURRENT_TIMESTAMP
        """,
        (key, value),
    )


providers = [
    ("api_key.anthropic", env("ANTHROPIC_API_KEY")),
    ("base_url.anthropic", env("ANTHROPIC_BASE_URL")),
    ("api_key.openai", env("OPENAI_API_KEY")),
    ("base_url.openai", env("OPENAI_BASE_URL")),
    ("api_key.moonshot", env("MOONSHOT_API_KEY")),
    ("base_url.moonshot", env("MOONSHOT_BASE_URL")),
    ("api_key.zhipu", env("ZHIPU_API_KEY")),
    ("base_url.zhipu", env("ZHIPU_BASE_URL")),
]

written_settings = []
for key, value in providers:
    if value:
        upsert_setting(key, value)
        written_settings.append(key)


def read_prompt():
    inline = os.environ.get("DEFAULT_AGENT_SYSTEM_PROMPT", "")
    if inline:
        return inline

    prompt_file = os.environ.get("DEFAULT_AGENT_SYSTEM_PROMPT_FILE", "").strip()
    if prompt_file and os.path.isfile(prompt_file):
        with open(prompt_file, "r", encoding="utf-8") as f:
            return f.read()
    return ""


default_agent_enabled = env("DEFAULT_AGENT_ENABLED", "true").lower() in {
    "1",
    "true",
    "yes",
    "on",
}
created_agent = False
updated_agent = False

if default_agent_enabled:
    agent_id = env("DEFAULT_AGENT_ID", "agent-prod")
    display_name = env("DEFAULT_AGENT_NAME", "Production Agent")
    provider = env("DEFAULT_AGENT_PROVIDER", "")
    model = env("DEFAULT_AGENT_MODEL", "")
    prompt = read_prompt()
    is_active = 1 if env("DEFAULT_AGENT_ACTIVE", "true").lower() in {"1", "true", "yes", "on"} else 0

    if not provider or not model:
        raise SystemExit("DEFAULT_AGENT_PROVIDER and DEFAULT_AGENT_MODEL are required when DEFAULT_AGENT_ENABLED=true")

    cur.execute("SELECT 1 FROM agent_configs WHERE id = ?", (agent_id,))
    exists = cur.fetchone() is not None

    cur.execute(
        """
        INSERT INTO agent_configs (
          id, user_id, model_id, display_name, system_prompt,
          provider, model, skills, channels, is_active, created_at, updated_at
        ) VALUES (?, '', '', ?, ?, ?, ?, '[]', '[]', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        ON CONFLICT(id) DO UPDATE SET
          display_name = excluded.display_name,
          system_prompt = excluded.system_prompt,
          provider = excluded.provider,
          model = excluded.model,
          is_active = excluded.is_active,
          updated_at = CURRENT_TIMESTAMP
        """,
        (agent_id, display_name, prompt, provider, model, is_active),
    )
    created_agent = not exists
    updated_agent = exists

conn.commit()
conn.close()

result = {
    "settings_written": written_settings,
    "default_agent_created": created_agent,
    "default_agent_updated": updated_agent,
    "default_agent_enabled": default_agent_enabled,
}
print(json.dumps(result, ensure_ascii=False))
PY
}

main "$@"
