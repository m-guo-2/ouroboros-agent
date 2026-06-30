#!/usr/bin/env bash
set -euo pipefail

HOST="${1:-moli-prod}"
TOKEN_NAME="${2:-moli}"

ssh "$HOST" "TOKEN_NAME='$TOKEN_NAME' python3 - <<'PY'
import os
import re
import subprocess
from pathlib import Path

token_name = os.environ['TOKEN_NAME']
env_text = Path('/opt/new-api/.env').read_text()
dsn_line = next((line for line in env_text.splitlines() if line.startswith('SQL_DSN=')), None)
if not dsn_line:
    raise SystemExit('SQL_DSN missing in /opt/new-api/.env')

dsn = dsn_line.split('=', 1)[1].strip().strip('\"').strip(\"'\")
match = re.match(r'([^:]+):([^@]+)@tcp\\(([^:)]+)(?::([0-9]+))?\\)/([^?]+)', dsn)
if not match:
    raise SystemExit('Unrecognized SQL_DSN format')

user, password, host, port, db = match.groups()
port = port or '3306'
env = os.environ.copy()
env['MYSQL_PWD'] = password
sql = \"SELECT tokens.key FROM tokens WHERE name=%s AND status=1 LIMIT 1;\"
cmd = [
    'mysql', '-h', host, '-P', port, '-u', user, '-D', db,
    '-N', '-s', '-e', sql % repr(token_name)
]
result = subprocess.run(cmd, env=env, text=True, capture_output=True, check=True)
token = result.stdout.strip()
if not token:
    raise SystemExit(f'No active token named {token_name!r}')
print(token)
PY"
