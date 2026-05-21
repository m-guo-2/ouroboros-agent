# Supervisor 发布流程

本文记录当前生产服务器的手工发布流程。目标是保证本地开发分支、GitHub 远端分支、服务器发布分支三者一致，避免用临时目录或未提交代码直接发布。

## 当前约定

- 本地开发目录：`/Users/gm/code/ouroboros-agent`
- 服务器 SSH：`moli-prod`
- 服务器仓库：`/root/ouroboros-agent`
- 服务安装目录：`/opt/moli`
- 服务管理：`supervisorctl`
- 服务组：`moli:*`
- 当前多账号开发分支：`feat/qiwei-multi-account`

发布必须从 GitHub 拉取代码。不要用本地 `rsync` 到 `/tmp` 后直接编译发布，除非是在抢修并且事后补提交、补发布记录。

## 本地发布前检查

确认当前分支就是要发布的分支：

```bash
git branch --show-current
git status --short
```

本地测试至少跑以下命令：

```bash
cd channel-qiwei && go test ./...
cd ../agent && go test ./...
```

如果只改了某个模块，可以先跑对应模块测试；涉及公共协议、数据库、消息路由或多账号隔离时，需要跑 `agent` 和 `channel-qiwei` 两边。

## 提交并推送到 GitHub

所有要发布的代码必须先提交到当前分支，并推送到 GitHub：

```bash
git add <changed-files>
git commit -m "fix: 描述本次发布内容"
git push origin "$(git branch --show-current)"
```

提交前不要包含临时文件、服务器配置、密钥、数据库文件、日志文件或 `.codex/`。

## 服务器拉取同名分支

在服务器仓库中切到和本地一致的分支：

```bash
ssh moli-prod
cd /root/ouroboros-agent

git fetch origin
git checkout feat/qiwei-multi-account
git pull --ff-only origin feat/qiwei-multi-account
```

如果服务器工作区有未跟踪的构建产物，例如 `agent/vendor/`、`channel-qiwei/vendor/` 或下载包，先确认不是人工修改，再清理：

```bash
git clean -fd agent/vendor channel-qiwei/vendor
```

不要在服务器上直接改业务代码。确需抢修时，先在本地分支提交并推送，再让服务器拉取。

## 编译并发布

当前服务器使用根目录 `Makefile` 的 supervisor 发布目标：

```bash
cd /root/ouroboros-agent
make deploy
```

`make deploy` 会执行：

- 停止 `moli:*`
- 编译 `admin`、`agent`、`channel-qiwei`
- 备份并安装二进制到 `/opt/moli/bin`
- 安装 `admin/dist`
- 重启 `moli:*`
- 输出 supervisor 状态

如果只改了企业微信通道，也可以使用更窄的发布方式：

```bash
make build-qiwei
make install-bins
sudo supervisorctl restart moli:moli-qiwei
sudo supervisorctl status moli:*
```

但多账号、协议字段、agent 路由或数据库相关改动，优先使用完整 `make deploy`。

## 发布后验证

发布后立即检查服务状态：

```bash
supervisorctl status moli:*
curl -fsS http://127.0.0.1:2013/health
curl -fsS http://127.0.0.1:2014/health
```

检查新日志中是否有明显错误：

```bash
grep -RaiE "panic|fatal|error|failed|Incorrect string|Error 1366|初始化失败|permission denied" /opt/moli/logs | tail -n 80
```

多账号相关发布还需要额外验证：

- `qiwei_accounts` 中账号数量、`agent_id`、`enabled` 正确。
- 新收到的企微消息 `channelConversationId` 带 `@shortHash`。
- agent 中用户、session、processed message 不跨账号误合并。
- 主动发送消息时能通过 `account_id` 或带后缀的 `channelConversationId` 命中目标账号。

## 回滚

`make install-bins` 会在 `/opt/moli/bin` 下留下带时间戳的 `.bak.*` 备份。需要回滚时：

```bash
sudo cp /opt/moli/bin/agent.bak.<timestamp> /opt/moli/bin/agent
sudo cp /opt/moli/bin/channel-qiwei.bak.<timestamp> /opt/moli/bin/channel-qiwei
sudo chmod 0755 /opt/moli/bin/agent /opt/moli/bin/channel-qiwei
sudo supervisorctl restart moli:*
sudo supervisorctl status moli:*
```

如果回滚代码，也要让服务器仓库 checkout 到对应提交，并记录当前运行的 commit：

```bash
cd /root/ouroboros-agent
git rev-parse --short HEAD
```
