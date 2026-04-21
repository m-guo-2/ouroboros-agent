# channel-qiwei (Go)

QiWe 渠道服务（Go 版），负责：

1. 接收 QiWe 回调消息（`/webhook/callback`）
2. 转发标准化消息到 Agent（`/api/channels/incoming`）
3. 接收 Agent 回调并向企微发送消息（`/api/qiwei/send`）
4. 提供 QiWe 全模块 API 代理（`/api/qiwei/{module}/{action}`、`/api/qiwei/do`）

## 运行

在仓库根目录：

```bash
make qiwei
```

或在当前目录：

```bash
go run .
```

## 环境变量

参考 `.env.example`：

- `QIWEI_API_BASE_URL`：QiWe API 地址
- `QIWEI_TOKEN`：QiWe Token（仅首启时用于 seed 默认账号，之后以 DB 为准）
- `QIWEI_GUID`：实例 GUID（同上，仅 seed 用）
- `QIWEI_BOT_PORT`：服务端口（默认 `2000`）
- `QIWEI_HTTP_TIMEOUT_SECONDS`：HTTP 超时秒数（默认 `25`）
- `QIWEI_DB_PATH`：账号元数据 SQLite 路径（默认 `{data_dir}/qiwei.db`）
- `QIWEI_ADMIN_TOKEN`：管理接口鉴权 Token；空则 `/_admin/*` 返回 404
- `QIWEI_GATEWAY_TOKEN`：gateway 查询接口鉴权 Token；空则 `/api/qiwei/gateway/*` 返回 404
- `QIWEI_PROFILE_SYNC_INTERVAL`：自动同步企微账号资料的周期（默认 `6h`）
- `QIWEI_CONTACT_SYNC_INTERVAL`：联系人/群快照全量同步周期（默认 `6h`）
- `QIWEI_CONTACT_SYNC_ENABLED`：是否开启联系人/群快照后台全量同步（默认 `true`；事件驱动同步始终可用）
- `AGENT_ENABLED`：是否启用 Agent 转发（默认 `true`）
- `AGENT_SERVER_URL`：Agent 服务地址（默认 `http://localhost:1997`）
- `AGENT_ID`：全局默认 Agent ID（单个账号可在 DB 里覆盖）

## 多账号

`channel-qiwei` 进程可挂接多个企微账号，账号元数据存在 SQLite（`qiwei_accounts` 表）。路由策略：

- **入向（webhook → agent）**：按 `guid` 查账号 → 使用该账号的 client / roomStore / dedupe；向 Agent 投递的 `channelConversationId` 为 `{rawId}@{shortHash(guid)}`。Agent 不需要知道账号本身，透传回 Channel 即可正确定位。
- **出向（agent → webhook）**：按 `channelConversationId` 末尾的 `@shortHash` 反查账号；或显式在请求中带 `account_id`。单账号部署允许省略后缀，走默认账号。
- **账号<->Agent**：N:1。多个企微账号可以绑定同一个 Agent，`agent_id` 存在账号行上，Agent 不感知底下具体是哪个企微号。

### 账号管理 API（`X-Admin-Token` 鉴权）

```http
GET    /api/qiwei/_admin/accounts              # 列表（含 disabled）
POST   /api/qiwei/_admin/accounts               # 创建 {guid, token, displayName?, agentId?, ...}
GET    /api/qiwei/_admin/accounts/{id}          # 详情
PATCH  /api/qiwei/_admin/accounts/{id}          # 更新 token/displayName/agentId/enabled/metaJson/notes
DELETE /api/qiwei/_admin/accounts/{id}          # 软删（enabled=false，历史保留）
POST   /api/qiwei/_admin/accounts/{id}/hard_delete   # 硬删（连同该账号的 known_rooms 一并清理）
POST   /api/qiwei/_admin/accounts/{id}/refresh       # 立即拉取 /user/getProfile 写回 self_*
POST   /api/qiwei/_admin/accounts/{id}/resync        # 立即触发联系人/群快照全量同步
GET    /api/qiwei/_admin/accounts/{id}/sync_status   # 查看 contact sync 队列与最近运行结果
POST   /api/qiwei/_admin/reload                 # 重新从 DB 构建路由注册表
GET    /api/qiwei/_admin/unknown_guids          # 最近 200 条未注册 guid 的 webhook 事件
```

所有响应对 `token` 做掩码（`token_preview` 只保留前 4 位 + `***`）。

### Contact Gateway API（`X-Gateway-Token` 鉴权）

`channel-qiwei` 会把联系人、群、群成员和下游身份映射持久化到同一个 `qiwei.db`（表：`qiwei_contacts / qiwei_rooms / qiwei_room_members / qiwei_identity_links`），并通过以下只读/幂等接口暴露给小程序后端、CRM 等下游：

```http
GET  /api/qiwei/gateway/contacts/resolve?accountId=&senderId=
GET  /api/qiwei/gateway/contacts/resolve?accountId=&externalUserId=
GET  /api/qiwei/gateway/contacts/search?accountId=&q=&limit=
GET  /api/qiwei/gateway/rooms/{roomId}?accountId=
GET  /api/qiwei/gateway/rooms/{roomId}/members?accountId=
GET  /api/qiwei/gateway/identity-links?downstreamSystem=&downstreamId=
POST /api/qiwei/gateway/identity-links
```

说明：

- `contacts/resolve` 返回 `{contact, externalUserId, identityLinks, lastSyncedAt}`。
- `refresh=1` 会强制回源同步一次；同一 `(account_id, user_id)` 30 秒内最多一次，超限返回 `429`。
- `identity-links` 是幂等 upsert，唯一键是 `(downstream_system, downstream_id)`；若同一个下游 ID 改绑到新的 `externalUserId`，服务会写一条 audit log。

### Contact Gateway 数据流

- **事件驱动**：好友通过、群事件、消息命中未知 `senderId` 时，会异步入队增量同步。
- **周期性全量**：`contact_sync_enabled=true` 时，每 `contact_sync_interval` 对每个启用账号跑一轮全量兜底同步。
- **读路径**：消息链路上的姓名/群名解析顺序是 `ttlCache -> SQLite -> 上游 API`。

### 首启兼容

- 若 `qiwei_accounts` 为空且配置里提供了 `QIWEI_GUID / QIWEI_TOKEN`，启动时会 seed 一个默认账号。
- 旧 `known_rooms.txt` 会一次性迁移到 `qiwei_known_rooms`，原文件归档为 `.migrated.{timestamp}`。

## 核心接口

- `POST /webhook/callback`
  - 接收企微回调并异步处理，快速返回 `{ code: 200, msg: "ok" }`
- `POST /api/qiwei/send`
  - 接收主服务 `OutgoingMessage`，发送回企微
- `POST /api/qiwei/do`
  - 直接调用任意 QiWe method，格式 `{ method, params }`
- `POST /api/qiwei/{module}/{action}`
  - 模块化调用（instance/login/user/contact/group/message/cdn/moment/tag/session）

## 后续接入企微官方 API

当前 `channel-qiwei` 的 contact gateway 数据源是 QiWe 第三方协议，但代码已经预留了 `ContactSource` 抽象。后续切到或并存企微官方 API 时，目标是：

- 不改 SQLite 表结构
- 不改 `/api/qiwei/gateway/*` HTTP 契约
- 不改 Agent 收发消息契约

预计只需要补一套 `officialAPISource` adapter，并在 `ContactGateway` 里决定哪些查询优先走官方源（例如 `GetOpenID` / `external_userid` 相关查询）。

预计涉及的文件：

- `channel-qiwei/contact_source.go`：新增 `officialAPISource` 实现
- `channel-qiwei/contact_gateway.go`：决定 source 选择策略
- `channel-qiwei/config.go`：若官方 API 需要独立凭证，则新增配置项
- `channel-qiwei/gateway_handlers.go`：通常无需改动，只在返回字段扩展时调整

## 回滚 / 清理

若需要回退 contact gateway 的持久化层，可先回滚二进制，再按需清空新表：

```sql
DROP TABLE IF EXISTS qiwei_identity_links;
DROP TABLE IF EXISTS qiwei_room_members;
DROP TABLE IF EXISTS qiwei_rooms;
DROP TABLE IF EXISTS qiwei_contacts;
```

`qiwei_accounts` 与 `qiwei_known_rooms` 是多账号改造留下的基础表，不在这里删除。

## 模块映射

模块 action 映射定义在：

- `internal/modules/instance.go`
- `internal/modules/login.go`
- `internal/modules/user.go`
- `internal/modules/contact.go`
- `internal/modules/group.go`
- `internal/modules/message.go`
- `internal/modules/cdn.go`
- `internal/modules/moment.go`
- `internal/modules/tag.go`
- `internal/modules/session.go`
