# Linux Production Deployment

This directory contains a production-oriented deployment bundle for running the project on a single Linux server.

## What It Installs

- `agent` as the main service
- `channel-qiwei` as an optional service
- `channel-feishu` as an optional service
- `admin/dist` served by `agent`
- `systemd` service units
- an optional `nginx` reverse-proxy config
- a bootstrap script to seed provider settings and a default agent into SQLite

## Recommended Topology

- `agent` listens on port `1997`
- `channel-feishu` listens on port `1999`
- `channel-qiwei` listens on port `2000`
- `nginx` handles public `80/443`

## Prerequisites

- Linux with `systemd`
- `go` 1.24+
- `bun`
- `python3`
- `nginx` if you want the proxy config installed
- root privileges for installation

## Files

- `install.sh`: build artifacts and install binaries, config, systemd, nginx templates
- `bootstrap.sh`: seed SQLite settings and create or update a default agent
- `env/*.example`: environment-file templates
- `systemd/*.service`: systemd unit templates
- `nginx/moli.conf`: nginx template

## Quick Start

Run on the Linux server from the repository root:

```bash
sudo env APP_NAME=moli \
  ENABLE_QIWEI=1 \
  ENABLE_FEISHU=0 \
  INSTALL_NGINX=1 \
  SERVER_NAME=example.com \
  bash ./deploy/install.sh
```

Then edit:

- `/etc/moli/agent.env`
- `/etc/moli/bootstrap.env`
- `/etc/moli/qiwei.env` if Qiwei is enabled
- `/etc/moli/feishu.env` if Feishu is enabled

Apply bootstrap data after editing:

```bash
sudo /opt/moli/bin/bootstrap
sudo systemctl restart moli-agent
sudo systemctl restart moli-qiwei
```

## Useful Environment Overrides

You can override these when running `install.sh`:

- `APP_NAME`: service prefix, default `moli`
- `INSTALL_PREFIX`: install root, default `/opt/$APP_NAME`
- `CONFIG_DIR`: config dir, default `/etc/$APP_NAME`
- `DATA_DIR`: sqlite dir, default `/var/lib/$APP_NAME`
- `LOG_DIR`: log dir, default `/var/log/$APP_NAME`
- `RUN_USER`: system user, default `$APP_NAME`
- `RUN_GROUP`: system group, default `$APP_NAME`
- `ENABLE_QIWEI`: `1` or `0`, default `1`
- `ENABLE_FEISHU`: `1` or `0`, default `0`
- `INSTALL_NGINX`: `1` or `0`, default `1`
- `SERVER_NAME`: nginx `server_name`, default `_`
- `BOOTSTRAP_AFTER_INSTALL`: `1` or `0`, default `1`

## Notes

- Existing env files are not overwritten by `install.sh`.
- `bootstrap.sh` writes provider credentials and the default agent directly into SQLite.
- The admin UI is served from `agent`; there is no production Vite process.
- If you expose the service publicly, put `/admin` and `/api` behind auth or a private network.
