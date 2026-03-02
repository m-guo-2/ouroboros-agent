# 021 · Server + Agent 合并为 Go 单体二进制

**日期**：2026-02-27  
**状态**：已实施

## 背景

项目原来有两个运行时进程：

- `server`（TypeScript/Bun）：业务控制器，负责渠道路由、身份管理、会话、Agent Profile、技能管理
- `agent`（Go）：执行引擎，负责 LLM 推理、任务调度

这种分离带来了：
- 两套不同语言的数据库连接（都连同一个 SQLite 文件，存在 WAL 并发问题）
- 两个进程的运维负担（启动顺序、健康检查、日志分散）
- `serverclient` 包通过 HTTP 从 agent → server 做业务查询，增加了不必要的网络跳
- 单 Agent 场景（最常见场景）不需要这种分离

## 决策

将 `server` 和 `agent` 合并为单一 **Go 单体二进制**，监听 `:1997`。

具体变更：
1. 新建 `agent/internal/storage/` 包，移植所有 SQLite CRUD（原 `database.ts`）
2. 新建 `agent/internal/dispatcher/` 包，处理消息入向（去重、用户解析、会话管理、直接入队）
3. 新建 `agent/internal/channels/` 包，管理出向渠道适配器（飞书/企微/WebUI）
4. 新建 `agent/internal/api/` 包，移植所有管理 API（`/api/*`）
5. Go binary 静态托管 `admin/dist`，提供 SPA 服务
6. 删除 `agent/internal/serverclient/`、`agent/internal/handlers/`
7. 删除 `server/src/`、`server/scripts/` 等 TypeScript 源码

渠道适配器（`channel-feishu`、`channel-qiwei`）保持独立进程，通过 HTTP POST 到 `/api/channels/incoming`。

## 数据目录调整

| 旧路径 | 新路径 |
|--------|--------|
| `server/data/config.db` | `data/config.db` |
| `server/data/logs/` | `data/logs/`（已存在，Go agent 一直在写这里）|

## 结果

- 单一二进制（~15MB），无外部依赖，一条命令启动
- 消除了 agent→server 的 HTTP 内循环，所有业务查询变为直接函数调用
- 渠道适配器解耦保持不变，仍可独立部署和扩展
- 运维端口从 3 个（1996/1997/5173）减少到 1 个（1997）

## 权衡

- 合并后 Go binary 包含更多职责，但相比跨进程 HTTP 调用，单进程调用更简单、更快
- TypeScript server 的"热更新"能力丧失，但在实际使用中该能力从未被利用
- 渠道适配器仍是独立进程，保留了语言多样性和独立部署的灵活性
