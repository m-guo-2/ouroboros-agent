## Why

`channel-qiwei` 目前只作为企微消息的收发通道：回调进来 → 转发 Agent；Agent 下行 → 调 QiWe API。联系人、群、群成员、`externalUserId/openid` 等身份数据全部停留在第三方协议的运行时内存里（`ttlCache` 5 分钟），进程重启即失效，也无法被小程序、数据分析、CRM 等下游系统复用。

运营和小程序侧已经出现打通诉求：当一个用户既在企微和 bot 对话、又在小程序里下单，我们需要一把稳定的"身份钥匙"——企微官方 `externalUserId`（对 WeApp 就是 `external_userid`，对小程序就是 `openid`/`unionid`）——把两端串起来。现有架构里没有地方沉淀这把钥匙，也没有一层抽象可以在后续接入企微官方 API 时平滑替换数据源。

## What Changes

- 在 `channel-qiwei` 里新增一层 **contact gateway**：
  - 负责从 QiWe 第三方协议拉取 / 监听联系人、群、群成员数据，并在 SQLite 里建立账号维度的持久化快照。
  - 抽象成 `ContactSource` interface，当前实现为 QiWe 第三方协议 adapter；后续企微官方 API 只要加一个新的 adapter 即可挂入，上层表结构和查询接口不变。
- 新增四张表（`qiwei.db`，账号隔离）：
  - `qiwei_contacts`：外部 / 内部联系人主数据，含 `external_user_id`、`openid`、昵称、备注、头像、`corp_id` 等。
  - `qiwei_rooms`：群主数据，含名称、公告、群主、成员数、群二维码链接（如有）。
  - `qiwei_room_members`：群 ↔ 联系人多对多，含群内昵称、加入时间、是否群主/管理员。
  - `qiwei_identity_links`：`(account_id, sender_id) ↔ external_user_id ↔ downstream (小程序 openid / unionid / userId)` 的多向映射表，显式为小程序打通而生，不跟联系人表耦合。
- 数据维护策略：**事件驱动 + 周期性全量兜底**
  - 事件驱动：`autoAcceptFriendRequest`、`handleGroupEvent`（joined/removed/name_changed）、群消息首次出现某个新 `senderId` 时触发增量 upsert。
  - 周期性：默认 6h 全量拉一次 `getWxContactList / getWxWorkContactList / getRoomList / batchGetRoomDetail`，带 per-account 退避；通过 `contact_sync_interval` 配置。
  - 手动：admin API `POST /api/qiwei/_admin/accounts/{id}/resync` 触发一次全量。
- 对下游服务暴露只读 HTTP 查询 API（基准路径 `/api/qiwei/gateway`）：
  - `GET /contacts/resolve?accountId=&senderId=` → 返回联系人主数据 + externalUserId + 已知 openid 映射。
  - `GET /contacts/search?accountId=&q=`、`GET /rooms/{roomId}?accountId=`、`GET /rooms/{roomId}/members?accountId=`。
  - `POST /identity-links` 由小程序登录流程调用，把 `external_user_id ↔ 小程序 openid/unionid` 写进 `qiwei_identity_links`。
- **BREAKING**（仅运行侧，不破坏 Agent 契约）：原本 `events.go` 里的 `loadContactsOnce / loadExternalContacts / loadInternalContacts / resolveUserName / resolveGroupName` 改为先读 DB、命中不到再回源；`ttlCache` 保留为热路径 L1，但数据所有权归 DB。
- 预留"企微官方 API adapter"接入点：interface 定义、表 schema 不绑死 QiWe 字段命名；与企微官方术语对齐（用 `external_user_id` / `corp_id` / `follow_user` 等标准词）。

## Capabilities

### New Capabilities

- `qiwei-contact-gateway`：`channel-qiwei` 内的联系人 / 群 / 身份映射持久化层，包括数据模型、同步策略、对内（runtime 读路径）与对外（HTTP 查询）的接口契约。

### Modified Capabilities

<!-- 无。`qiwei-multi-account` 当前尚未 archive，本 change 在其数据层之上扩展，不改动其既有 requirements。 -->

## Impact

- **代码**：`channel-qiwei` 新增 `contact_gateway.go / contact_repo.go / contact_source.go / gateway_handlers.go`；`db.go` schema 新增 4 张表；`events.go` 读路径改走 gateway；`server.go` 注册 `/api/qiwei/gateway/*` 路由。
- **数据库**：`{data_dir}/qiwei.db` 新增 4 张表，首启自动迁移；schema 增量上线，不影响 `qiwei_accounts / qiwei_known_rooms`。
- **配置**：`config.yaml` 新增 `contact_sync_interval`（默认 6h）、`contact_sync_enabled`（默认 true）。
- **API 契约**：
  - Agent 侧不感知本次改动；`incomingMessage` 字段不变。
  - 新增只读 `/api/qiwei/gateway/*` 供下游（小程序后端、CRM）调用，走现有 `admin_token` 或独立 `gateway_token`（二选一，design 阶段定）。
- **外部依赖**：v1 只依赖现有 QiWe 第三方协议 API；`ContactSource` interface 为后续企微官方 API 接入预留。
- **运营**：运营可通过 admin API 手动触发单账号 resync；新增 metric：`contact_sync_last_at / contact_sync_errors / identity_link_count`（通过 admin GET `/accounts/{id}` 暴露）。
