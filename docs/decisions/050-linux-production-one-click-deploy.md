# Linux 生产一键部署方案

- **日期**：2026-03-06
- **类型**：讨论结论
- **状态**：已决定

## 背景

当前仓库已有 `agent`、`channel-feishu`、`channel-qiwei` 三个可独立运行的进程，也已有 `admin` 前端构建产物挂载能力，但缺少适合 Linux 服务器直接落地的生产级部署封装。用户希望通过环境变量完成配置，并在单机上实现一键运行。

## 决策

单机生产环境采用“预构建二进制 + systemd 守护 + nginx 反向代理 + 独立 env 文件 + 启动后初始化配置”的部署模式，不使用开发态 `make dev` 或前端开发服务器。

## 变更内容

- 明确 `agent` 作为主服务，对外提供 API 与管理后台静态页面。
- 明确 `channel-feishu`、`channel-qiwei` 作为可选独立守护进程，分别使用独立环境变量文件。
- 明确生产启动流程应包含：
  - 构建 `admin/dist`
  - 构建三个 Go 二进制
  - 安装到固定目录
  - 写入 `systemd` service
  - 配置 `nginx`
  - 启动后向 SQLite 或 HTTP API 注入 provider 凭据与初始 agent 配置
- 明确模型 provider 凭据当前不直接从环境变量读取，而是存储在 `settings` 表中，因此部署脚本需要补一个 bootstrap 步骤。

## 考虑过的替代方案

- 直接使用 `make agent` / `make qiwei`：
  这是开发态运行方式，依赖源码目录和即时编译，不适合作为生产级部署入口。
- 只靠环境变量，不做初始化注入：
  当前主服务的模型凭据读取自 SQLite `settings` 表，单靠环境变量无法完整完成首启配置。
- Docker Compose：
  仓库当前没有现成 Dockerfile/compose 资产，短期内维护成本高于 `systemd` 方案。

## 影响

后续如果实现真正的一键脚本，应该优先补 `deploy/` 目录下的安装脚本、systemd 单元和 bootstrap 初始化脚本。该方案适合单机生产；若未来进入多实例或高可用部署，需要继续推进 Redis 队列与外部数据库方案。
