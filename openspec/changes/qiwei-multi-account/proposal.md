## Why

`channel-qiwei` 当前只支持单企微账号：`guid/token/agent_id` 写死在 `config.yaml`，`app` 里一套 `qiweiClient / roomStore / nameCache / selfUserID`，新号要上线就得再起一个进程。运营侧已经出现"一个 Agent 管多个企微号"的需求（N:1），继续走多进程/多副本的路子会带来回调地址管理、资源浪费、运维成本、账号状态可观测性等一系列问题。

本次改造把 channel-qiwei 从"单账号进程"升级为"多账号网关"：账号配置迁到 SQLite，运行时按 guid 路由，Agent 侧对账号完全无感知，同时把每个企微号的自身身份（昵称/头像/self userId）作为可查询的快照沉淀到库里。

## What Changes

- **新增** SQLite 存储（独立于 agent 的 `config.db`），用于持有企微账号注册表和已知群列表
- **新增** 账号注册表 `accountRegistry` 和 `accountRuntime`：每个账号独立持有 qiweiClient、roomStore、selfUserID、联系人缓存、msgSvrId 去重
- **新增** 透明复合 conversationId 策略：对 Agent 暴露的 `channelConversationId = {rawId}@{shortHash(guid)}`，Agent 当不透明字符串原样回传；Channel 按末尾后缀反查账号
- **新增** 企微自身身份快照：启动与周期性（默认 6h）调用 `/user/getProfile` 拉取昵称/头像/alias/corpName 存库，并通过 `incomingMessage.channelIdentity` 透传给 Agent 做展示
- **新增** 账号管理 HTTP API（完整 CRUD + 手动刷新 + 热 reload），受 `admin_token` 鉴权保护
- **新增** 启动时自动 seed：当库为空且 YAML 顶层仍配了 `guid/token` 时，作为首个默认账号写入库；同时迁移 `known_rooms.txt`
- **修改** `/webhook/callback` 路由：按 `msg.GUID` 分派到对应账号 runtime；未注册 guid 记录并丢弃
- **修改** `/api/qiwei/send` 等下行接口：按 conversationId 末尾后缀路由账号；无后缀且仅一个账号时走默认账号（兼容老 Agent）
- **修改** `incomingMessage.agentId` 改为由账号绑定决定（N:1），同时新增 `channelIdentity` 字段
- **BREAKING** `channelConversationId` 对 Agent 呈现的值格式改变：历史 session 的 conversationId 与新消息不再匹配，首日可能出现新建 session 的过渡现象；admin/agent 两侧数据表不做迁移，自然收敛

## Capabilities

### New Capabilities

- `qiwei-multi-account`: 企微渠道多账号注册、运行时路由、身份快照同步、管理 API、以及对 Agent 透明的复合 conversation 策略

### Modified Capabilities

<!-- 不改动其它 capability 的 spec-level 行为。Agent 侧继续按现有 channel-incoming 契约消费，只是多出可选字段 channelIdentity 和变更了 channelConversationId 的字符串形态。 -->

## Impact

- 代码：`channel-qiwei/*`（新增 `account_*.go`、`db.go`、`admin_handlers.go`；大幅改 `server.go / events.go / api_handlers.go / facade_handlers.go / room_store.go / config.go`）
- 数据：新增 `channel-qiwei/data/qiwei.db`（表 `qiwei_accounts`、`qiwei_known_rooms`）；遗留 `known_rooms.txt` 在首启时迁入
- 配置：`config.yaml` 顶层 `token/guid/agent_id` 降级为"首启 seed"语义；新增 `db_path`、`admin_token`、`profile_sync_interval` 字段
- 契约：`incomingMessage` 新增 `channelIdentity` 字段；`channelConversationId` 值形态变化
- 部署：systemd 单元不变；首次升级要确保 `data_dir` 可写（SQLite 文件 + WAL）
- Agent 侧：零改动；但需要知道 conversationId 语义变化，老 session 自然过期
- 运维：新增一组 `/api/qiwei/_admin/accounts` 端点，要在网关或内部网络层限制访问
