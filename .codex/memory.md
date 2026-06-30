# Project Memory

## Production Server

- SSH alias: `moli-prod`
- SSH target: `root@115.190.14.209`
- Local identity file: `~/.ssh/id_ed25519_moli_prod`
- Passwords, private keys, and API tokens are not stored here.

Observed on 2026-05-20:

- OS: Debian GNU/Linux 12 (bookworm), Linux 6.1 x86_64.
- Main install path: `/opt/moli`.
- Config path in use: `/opt/moli/conf`.
- Runtime manager: `supervisor`, not systemd, for the current `moli` services.
- Supervisor programs: `moli-agent`, `moli-qiwei`; group: `moli`.
- `agent` command: `/opt/moli/bin/agent -config /opt/moli/conf/agent.yaml`.
- `channel-qiwei` command: `/opt/moli/bin/channel-qiwei -config /opt/moli/conf/qiwei.yaml`.
- Health endpoints observed healthy: `http://127.0.0.1:2014/health` and `http://127.0.0.1:2013/health`.
- Public `agent` health was reachable at `http://115.190.14.209:2014/health`.
- MinIO is present on ports `2012` API and `2011` console.
- Nginx is active on port `80`.
- Other supervisor-managed services observed: `db_view`, `minio`, `my-doc`; `xhs-api` and `xhs-worker` were stopped.

Operational notes:

- Repository deploy defaults (`1997`/`2000`) differ from the current server ports (`2014`/`2013`).
- Disk usage on `/` was about 80% during the initial check; large areas included `/opt/moli/logs`, `/opt/moli/bin` backups, and systemd journal files.
- The server has no swap configured.
- Several service ports were publicly reachable/listening, including `2011`, `2012`, `2013`, `2014`, `2000`, and `3000`; review firewall/nginx exposure before treating the host as hardened.

## Server Inventory

Supervisor:

- Main config: `/etc/supervisor/supervisord.conf`.
- Program configs: `/etc/supervisor/conf.d/*.conf`.
- Active moli configs: `/etc/supervisor/conf.d/moli-agent.conf`, `/etc/supervisor/conf.d/moli-qiwei.conf`, `/etc/supervisor/conf.d/moli-group.conf`.
- Service control examples: `supervisorctl status`, `supervisorctl restart moli:moli-agent`, `supervisorctl restart moli:moli-qiwei`, `supervisorctl restart moli:*`.
- Other configured programs: `db_view`, `minio`, `my-doc`, `xhs-api`, `xhs-worker`.

Nginx:

- Main config: `/etc/nginx/nginx.conf`.
- Active sites: `/etc/nginx/sites-enabled/default`, `/etc/nginx/sites-enabled/db_view`, `/etc/nginx/sites-enabled/minio.conf`, `/etc/nginx/sites-enabled/xhs`.
- Available but inactive site snippets include `mp-weixin`, `xhs-api`, and `xhs-simple-service`.
- Public nginx routes verified on 2026-05-20: `/` works, `/db_view/` works, `/xhs/` works.
- `/minio/` and `/minio-api/` returned 404 through nginx because multiple active sites use `server_name _`; direct MinIO ports were still reachable.
- `/api/`, `/xhs/api/`, and `/xhs/docs` returned 502 because the xhs backend on `127.0.0.1:8080` was stopped.

Important directories:

- `/root/ouroboros-agent`: server-side source checkout; observed branch `feat/qiwei`, commit `014dda37d2a16f4710a048c99aa31164ee2ab53a`.
- `/opt/moli/bin`: deployed binaries and many timestamped binary backups.
- `/opt/moli/conf`: live moli YAML configs; contains secrets, only inspect with redaction.
- `/opt/moli/data`: SQLite runtime data, including `config.db` and `qiwei.db`.
- `/opt/moli/admin/dist`: deployed admin static files served by agent.
- `/opt/moli/logs`: moli logs, including `boundary`, `sqlite`, `business`, and `detail`.
- `/opt/moli/db_view`: db_view Node app served on local port `3000`.
- `/data/minio`: MinIO object data.
- `/data/weixin`: weixin file data area.
- `/var/www/html`, `/var/www/xhs`, `/var/www/mp-weixin`: nginx-served static trees.
- `/root/xhs-simple-service`: xhs backend source; supervisor programs were stopped during exploration.
- `/root/my-doc`: my-doc source used by supervisor program `my-doc`.

Deployment notes:

- The current repo has `deploy/supervisor-deploy.sh`, a wrapper around Makefile targets: `deploy`, `build`, `install`, `restart`, `stop`, `status`.
- `make deploy` runs `stop build install restart status` for the `moli` supervisor group.
- `make build` builds `admin/dist`, `bin/agent`, and `bin/channel-qiwei`.
- `make install` copies binaries to `/opt/moli/bin` and admin assets to `/opt/moli/admin/dist`; existing binaries are backed up as `*.bak.YYYYMMDDHHMMSS`.
- `make restart` runs `sudo supervisorctl restart "moli:*"`.
- `make status` runs `sudo supervisorctl status "moli:*"`.
- Server-side source checkout had untracked `agent/vendor/`, `channel-qiwei/vendor/`, and `go1.24.0.linux-amd64.tar.gz` during exploration.
- Current deployed binaries in `/opt/moli/bin` were modified on 2026-04-21 19:14.

## Development And Debugging Runbook

Common server entry:

- SSH into production: `ssh moli-prod`.
- Production source checkout: `ssh moli-prod 'cd /root/ouroboros-agent && git status --short && git log -1 --oneline'`.
- Production service status: `ssh moli-prod 'supervisorctl status'`.
- Core service status: `ssh moli-prod 'supervisorctl status moli:*'`.

Health checks:

- Agent health: `ssh moli-prod 'curl -fsS http://127.0.0.1:2014/health'`.
- Qiwei health: `ssh moli-prod 'curl -fsS http://127.0.0.1:2013/health'`.
- Public agent health was reachable during exploration: `curl -fsS http://115.190.14.209:2014/health`.
- Repo deployment docs may mention `1997`/`2000`; current production uses `2014` for agent and `2013` for qiwei.

Deploy flow:

- Preferred server-side deployment entry: `ssh moli-prod 'cd /root/ouroboros-agent && ./deploy/supervisor-deploy.sh deploy'`.
- Equivalent direct command: `ssh moli-prod 'cd /root/ouroboros-agent && make deploy'`.
- Restart without rebuilding: `ssh moli-prod 'cd /root/ouroboros-agent && make restart'`.
- Check after deploy: `ssh moli-prod 'supervisorctl status moli:* && curl -fsS http://127.0.0.1:2014/health && curl -fsS http://127.0.0.1:2013/health'`.
- Be careful: server checkout may be behind local development branches; compare branches and commits before deploying.

Log analysis:

- Main agent log: `/opt/moli/logs/moli-agent.log`.
- Qiwei channel log: `/opt/moli/logs/moli-qiwei.log`.
- Agent rotated logs: `/opt/moli/logs/moli-agent.log.1`, `.2`, `.3`; supervisor config allows up to 10 backups.
- Structured/business logs: `/opt/moli/logs/business/*.jsonl`.
- Detail logs: `/opt/moli/logs/detail/*.jsonl`.
- Boundary/API logs: `/opt/moli/logs/boundary/*.jsonl`.
- SQLite-style daily logs: `/opt/moli/logs/sqlite/*.db`.
- Nginx access log: `/var/log/nginx/access.log`.
- Nginx error log: `/var/log/nginx/error.log`.
- Supervisor daemon log: `/var/log/supervisor/supervisord.log`.
- db_view logs: `/var/log/db_view.out.log`, `/var/log/db_view.err.log`.
- MinIO logs: `/var/log/minio.stdout.log`, `/var/log/minio.stderr.log`.

Useful log commands:

- Recent agent errors: `ssh moli-prod 'tail -n 500 /opt/moli/logs/moli-agent.log | grep -Ei "error|panic|fatal|failed|warn|denied" | tail -n 80'`.
- Recent qiwei errors: `ssh moli-prod 'tail -n 500 /opt/moli/logs/moli-qiwei.log | grep -Ei "error|panic|fatal|failed|warn|denied" | tail -n 80'`.
- Follow core logs live: `ssh moli-prod 'tail -f /opt/moli/logs/moli-agent.log /opt/moli/logs/moli-qiwei.log'`.
- Nginx upstream errors: `ssh moli-prod 'tail -n 200 /var/log/nginx/error.log'`.
- Supervisor restart history: `ssh moli-prod 'tail -n 200 /var/log/supervisor/supervisord.log'`.

Config and data cautions:

- Live configs under `/opt/moli/conf` contain secrets; inspect with redaction and do not copy raw contents into chat or docs.
- Main SQLite DB is `/opt/moli/data/config.db`; qiwei DB is `/opt/moli/data/qiwei.db`.
- Before changing binaries or DB files, preserve current backups and confirm disk space.
- `/opt/moli/bin` contains many historical binary backups and can grow quickly after deployments.
