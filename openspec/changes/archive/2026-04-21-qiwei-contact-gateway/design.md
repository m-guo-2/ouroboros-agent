## Context

`channel-qiwei`（截至 `feat/qiwei-multi-account`）已经完成 N:1 多账号改造，`qiwei.db` 里已经有 `qiwei_accounts` 和 `qiwei_known_rooms` 两张表，`accountRuntime` 持有每个账号的 `qiweiClient / roomStore / nameCache / dedupe`。

当前的联系人/群数据流：

- 每个 `accountRuntime` 有一个 `ttlCache`（5 分钟 TTL），靠 `loadContactsOnce / loadExternalContacts / loadInternalContacts / resolveGroupName / fetchUserName` 按需从 QiWe API 拉取，只存 `userId → name` 这一对；`externalUserId`、`openid`、头像、备注、corpId、群成员结构等字段**全都丢弃了**。
- 进程重启或 5 分钟后：再次回源 QiWe。
- 没有持久化，没有跨账号 / 跨服务复用。

小程序打通的核心需求是一把稳定的身份钥匙。在企微生态里，这把钥匙就是官方 `external_userid`（企业把外部微信用户 ID 化之后给出的值），它可以与小程序登录态里的 `openid / unionid` 做服务端绑定——一侧通过企微官方 API（或三方协议能返回的 `openid` 字段）获取，一侧通过小程序 `wx.login` 获得。没有持久化层的前提下，这个绑定只能每次对话前现拉，既慢又没有历史轨迹。

用户同时说明："**企微官方 API 后续会补充**"。当前只能用 QiWe 第三方协议的数据，但设计必须保证之后切换/并存官方 API 数据源时，表结构和对下游暴露的查询接口不变。

## Goals / Non-Goals

**Goals**

- 在 `channel-qiwei` 内建一层 contact gateway，对联系人、群、群成员、身份映射做持久化，且按账号完全隔离。
- 数据字段对齐企微官方术语（`external_user_id / corp_id / follow_user / remark` 等），当前 QiWe 协议无法提供的字段（如 `unionid`、`type=1|2`）留空但列先建出来。
- 定义 `ContactSource` interface，将"从哪个 API 拉数据"与"怎么存 / 怎么查"解耦；v1 实现是 `qiweProtoSource`，之后官方 API adapter 只是换一个 `ContactSource`。
- `events.go` 的 `resolveUserName / resolveGroupName / loadContactsOnce` 改为先读 DB、未命中再回源；`ttlCache` 降级成纯内存加速层。
- 对下游服务（小程序后端、CRM）提供只读 HTTP 查询 API，最小化耦合：下游只知道 `accountId + senderId` 或 `externalUserId`，不需要理解企微/QiWe 协议差异。
- 事件驱动为主：好友自动通过、群成员变动、群改名这些 webhook 事件进来时就 upsert；6 小时周期性全量兜底；admin API 可手动 resync。
- 不破坏 Agent 契约；`incomingMessage.channelIdentity` 等字段继续工作。

**Non-Goals**

- 不做"实时双向同步"：企微侧的数据有延迟，本层也接受最终一致。
- 不在本层做小程序登录流程本身；`POST /identity-links` 只做映射登记，登录验签在小程序后端完成。
- 不把联系人头像 / 群二维码图片代下载到 OSS；只存 URL。图片持久化走 `media_pipeline` 的现有路径，不在本 change 范围。
- 不替换 `nameCache`；热路径保持 L1 ttlCache、L2 DB、L3 API 三级结构，删掉 L1 会在消息抵达密集时无谓压 SQLite。
- 不做 PII 加密；和 `qiwei_accounts.token` 同级安全假设（SQLite 文件本身在受控机器上）。
- 不做跨 `account_id` 的联系人合并："同一个微信号同时加了两个企微号" 这种情形在 v1 下是两条记录，后续再设计 unionId-based dedupe。

## Decisions

### D1. 数据模型：4 张表，账号隔离，主键复合

```
qiwei_contacts
  (account_id, user_id) PK          -- user_id 是 QiWe/企微返回的内部 userId（senderId 就是它）
  external_user_id TEXT   INDEX     -- 企微官方 external_userid；QiWe 协议的 openid 字段放这里
  source           TEXT             -- 'external' | 'internal' | 'unknown'
  nickname / real_name / alias / remark / avatar_url / gender / corp_id / corp_name
  follow_user_json TEXT             -- 预留：企微官方 API 的 follow_user 数组
  raw_json         TEXT             -- 最近一次上游返回的完整 payload（方便未来 schema 演进）
  first_seen_at / last_synced_at / updated_at

qiwei_rooms
  (account_id, room_id) PK
  name / announcement / notice / owner_user_id / member_count / qr_code_url / raw_json
  first_seen_at / last_synced_at / updated_at

qiwei_room_members
  (account_id, room_id, user_id) PK
  display_name / role ('owner'|'admin'|'member') / joined_at / last_seen_at

qiwei_identity_links
  id INTEGER PK AUTOINCREMENT
  account_id / user_id / external_user_id          -- 三者都是弱引用，允许 NULL
  downstream_system TEXT NOT NULL                  -- 'weapp' | 'web' | 'crm' ...
  downstream_id     TEXT NOT NULL                  -- 小程序 openid 或 unionid 或其它
  downstream_meta_json TEXT                        -- 小程序侧附带的 appid/unionid/openid map
  created_at / updated_at
  UNIQUE (downstream_system, downstream_id)
  INDEX (account_id, external_user_id), (account_id, user_id)
```

放弃的替代：

- **合并成一张 `qiwei_identities` 大宽表**：身份信息和小程序映射是两种生命周期（一侧由企微驱动、一侧由小程序登录驱动），合表会让写路径互相干扰。
- **用 `user_id` 作为 identity_links 外键**：QiWe 协议下联系人首次出现在群里时可能还没 `external_user_id`；强外键会让映射写入被卡住。改为三列都是弱引用，`UNIQUE(downstream_system, downstream_id)` 保证对下游侧不重复。
- **不落 `raw_json`**：现实中 QiWe / 官方 API 的字段经常新增；存原始 payload 让我们后续加字段时不用把所有账号都重新拉一遍。付出的代价是每行大一些，可接受。

`account_id` 外键到 `qiwei_accounts(id)`，`ON DELETE CASCADE`：`qiwei_multi_account` 里软删走 `enabled=0`，真要 `hard=1` 时一并清空该账号的联系人/群/映射是正确行为。

### D2. ContactSource interface：为官方 API 切换预留

```go
type ContactSource interface {
    ListExternalContacts(ctx context.Context) ([]ContactSnapshot, error)
    ListInternalContacts(ctx context.Context) ([]ContactSnapshot, error)
    ListRooms(ctx context.Context) ([]RoomSnapshot, error)
    BatchGetRoomDetail(ctx context.Context, roomIDs []string) ([]RoomSnapshot, error)
    BatchGetUserInfo(ctx context.Context, userIDs []string) ([]ContactSnapshot, error)
    GetOpenID(ctx context.Context, userID string) (openid, unionid string, err error)
}

type ContactSnapshot struct {
    UserID         string
    ExternalUserID string            // 企微官方 external_userid（QiWe `openid` 字段映到这里）
    Source         string            // "external" | "internal"
    Nickname, RealName, Alias, Remark, AvatarURL, CorpID, CorpName, Gender string
    FollowUser     any               // 预留：官方 API 的 follow_user 数组
    Raw            map[string]any    // 原样落 raw_json
}
```

- v1 的实现 `qiweProtoSource` 直接复用 `rt.client.doAPIRaw`；映射规则是一次性的字段翻译。
- 之后接入官方 API 时新增 `officialAPISource`；可以在 `accountRuntime` 上并行保留两个 source，默认走 `qiweProtoSource`，某些查询（如 `GetOpenID`）转接到官方 source（官方 API 对 `external_userid ↔ openid` 的转换最权威）。策略放在 `ContactGateway`，source 本身无状态。

放弃的替代：

- **直接在 `qiweiClient` 上加方法**：`qiweiClient` 的职责是"协议级 HTTP 客户端"，加高层语义会让之后接官方 API（不同鉴权模型）难以复用。
- **把 adapter 做成 module + action 字符串查表**：已有 `internal/modules` 这样做了，但那是"通用代理"层，不包含字段映射；gateway 关心的是字段语义，不是 HTTP path。

### D3. 同步调度：`contactSyncer`，事件驱动 + 6h 周期性 + admin 手动

- `contactSyncer` 和 `profileSyncer` 同级，per-app 一个实例，按账号维度维护 `lastFullSyncAt / failureBackoff / resyncRequested`。
- 触发点：
  1. **事件驱动**（低延迟，最常命中）：
     - `autoAcceptFriendRequest` 成功后 → `syncer.upsertContact(rt, contactID)`。
     - `handleGroupEvent`（`member_joined / member_removed / group_name_changed / group_joined`）→ `syncer.upsertRoom(rt, roomID)` + 差异刷成员。
     - 任一消息的 `senderId` 在 `qiwei_contacts` 未出现过 → 异步 `syncer.upsertContact`，不阻塞消息转发。
  2. **周期性**（兜底，默认 6h，可配 `contact_sync_interval`）：
     - `runFull(rt)`：`ListExternalContacts + ListInternalContacts` → upsert；`ListRooms` → 对 `(account_id, room_id)` diff，只对变化集合调 `BatchGetRoomDetail`。
     - 失败指数退避，复用 `profileSyncer` 里的 `backoff` 实现。
  3. **手动**：`POST /api/qiwei/_admin/accounts/{id}/resync` → 立即把该账号加入下一轮运行队列。
- 写入路径通过 `contactRepo`，所有 upsert 都是 `INSERT ... ON CONFLICT DO UPDATE`，`first_seen_at` 只在插入时写，`last_synced_at / updated_at` 每次刷新。
- 并发模型：周期性 sync 在后台 goroutine，事件驱动 upsert 走一个 per-account 的轻量 worker channel（size 64，满则丢弃最旧的，带 warn log 和计数）；避免突发群活动把 DB 写锁打挂。

放弃的替代：

- **不分 worker channel，直接 go-routine 每次 spawn**：突发时会 spawn 成百上千个 goroutine 争写锁。
- **所有同步都放主线程**：消息转发路径对延迟敏感，DB round-trip 不能插在热路径。

### D4. 读路径：`ContactGateway`（runtime 内 + HTTP 外）

```go
type ContactGateway struct {
    repo  *contactRepo
    cache *ttlCache          // 仍做 L1，5 分钟
    src   ContactSource
}

func (g *ContactGateway) ResolveName(ctx, userID, roomID string) (string, error)
func (g *ContactGateway) ResolveContact(ctx, userID string) (Contact, error)
func (g *ContactGateway) ResolveRoom(ctx, roomID string) (Room, error)
func (g *ContactGateway) ResolveExternalUserID(ctx, userID string) (string, error)
```

调用顺序：`cache → repo → src`，命中任何一级即短路；命中 `src` 之后异步写回 repo 和 cache。

`events.go` 里：

- `resolveUserName(ctx, rt, userID)` → `rt.gateway.ResolveName(...)`
- `resolveGroupName(ctx, rt, roomID)` → `rt.gateway.ResolveRoom(...).Name`
- `loadContactsOnce` 语义保留，但实际变成"距离上次全量同步超过 5 分钟才触发一次 src 拉取"——避免群活动把 API quota 打满。

放弃的替代：

- **L1 直接删掉**：热群场景下 L2（SQLite）每消息一次 `SELECT` 没必要；ttlCache O(1) 命中值当。
- **把 gateway 挂在 `app` 而不是 `accountRuntime`**：repo 要用 `account_id` 做 WHERE，放 runtime 里最自然；cache 也是账号隔离。共享 repo struct、共享 src 接口，只是每个 rt 各持有一个 `ContactGateway` 实例。

### D5. 对下游 HTTP API：`/api/qiwei/gateway/*`，只读 + 一个写入

基准路径 `/api/qiwei/gateway`，新增独立 `gateway_token`（YAML 配置），与 `admin_token` 分开——admin 能写账号 token，gateway 只读联系人 + 写 identity link，权限范围不一样。

| Method | Path | 说明 |
| --- | --- | --- |
| GET | `/contacts/resolve?accountId=&senderId=` | 单个联系人主数据 + 已知 identity links |
| GET | `/contacts/resolve?accountId=&externalUserId=` | 同上，按企微 external_user_id 查 |
| GET | `/contacts/search?accountId=&q=&limit=` | 名字模糊搜索（LIKE on nickname/real_name/alias/remark） |
| GET | `/rooms/{roomId}?accountId=` | 群主数据 |
| GET | `/rooms/{roomId}/members?accountId=` | 群成员列表 |
| POST | `/identity-links` | body: `{accountId, userId?, externalUserId?, downstreamSystem, downstreamId, meta?}` |
| GET | `/identity-links?accountId=&downstreamSystem=&downstreamId=` | 反查映射 |

鉴权：`X-Gateway-Token` header；空时不注册路由（同 admin 的做法）。

放弃的替代：

- **让下游直接读 `qiwei.db`**：跨进程锁、版本耦合、schema 变更扩散；明确不做。
- **复用 `admin_token`**：小程序后端拿到 admin token 就能改账号 token，权限过大。
- **gRPC / 私有 RPC**：当前生态全 HTTP+JSON，新增协议不值当。

### D6. `identity_links` 写入语义

`POST /identity-links` 是 **idempotent upsert**，按 `(downstream_system, downstream_id)` 唯一：

- 该对下游 ID 首次出现 → insert。
- 再次出现、`account_id + external_user_id` 与之前一致 → 仅更新 `updated_at` 和 `downstream_meta_json`。
- 再次出现但 `external_user_id` 不同（用户换了企微号）→ 覆盖 `account_id / user_id / external_user_id`，并写一条 audit log（logger.Business，级别足够，v1 不单独建 audit 表）。

这样小程序后端每次 `wx.login` 都可以无脑 POST；重复不会膨胀表。

### D7. 迁移与启动

- 启动序列在 `main.go` 里加 `contactSyncer.Start(ctx)`，与 `profileSyncer` 并列。
- 首启不做回填：不主动对历史 `qiwei_known_rooms` 里的群跑全量；这张表本来就只是"机器人进过的群"的去重集合，真正的群主数据会在下一条消息或下一轮 6h 同步时自然刷入。
- 升级风险低：新表都是 `CREATE TABLE IF NOT EXISTS`，旧部署多次重启无副作用。

### D8. 观测与 admin 可见性

- 在 `GET /api/qiwei/_admin/accounts/{id}` 的返回里扩展：
  - `contact_count / room_count / room_member_count`
  - `contact_sync_last_at / contact_sync_last_error`
  - `identity_link_count`
- 新增 `GET /api/qiwei/_admin/accounts/{id}/sync_status`：返回 syncer 的实时队列深度、最近几次运行的时间戳与结果。
- logger 沿用 `tag=contact-sync`、`tag=gateway`。

## Risks / Trade-offs

- **[R1] QiWe 协议返回的 `openid` 与企微官方 `external_userid` 不一定是同一串** → 在 `ContactSnapshot.ExternalUserID` 字段注释里明确"v1 由 QiWe 协议提供、语义上等价于官方 external_userid，但切到官方 API 后需要一次性 migration"；migration 走一次性 admin 接口，v1 暂不实现代码，只在 design 里写明处理路径。
- **[R2] 6h 全量同步对大账号的 API quota 压力** → 全量同步只拉 list（每条很轻）；仅对"群详情"走 `batchGetRoomDetail` 的增量集。QPS 基本可控；若仍打到限额，`contact_sync_enabled=false` 可以关掉整层，退化成事件驱动。
- **[R3] 事件驱动 upsert 在突发群活动下把 worker channel 塞满** → 丢弃最旧 + 计数 + warn log；被丢的那一条会在下一轮 6h 全量里兜底刷回，延迟但不丢数据。
- **[R4] 下游服务拿到 stale external_user_id** → 每条返回带 `lastSyncedAt` 字段，由下游自行决定是否可接受；`/contacts/resolve` 可选 `?refresh=1` 强制回源一次（rate-limited：每 account_id 每 user_id 每 30s 一次）。
- **[R5] `qiwei_identity_links` 被污染（小程序 bug 下把别人的 openid 写到你账下）** → `UNIQUE(downstream_system, downstream_id)` 保证 openid 不会同时挂两个 external_user_id；覆盖写时记 audit log，admin 可查。进一步的权限控制（比如签名验证请求来自哪个小程序 appid）是下一迭代。
- **[R6] 切官方 API 时表需要字段迁移** → 已在 schema 预留 `follow_user_json / corp_id / raw_json`，大概率不需要 ALTER TABLE；若需要也走 `qiwei-multi-account` 里的幂等 migration 风格。
- **[R7] SQLite 单写锁下，worker + profileSyncer + roomStore + gateway 共同写** → 所有写路径都走 `contactRepo`，批量写时用事务；`MaxOpenConns=1 + WAL` 的组合在当前量级够用，真要撑不住再考虑分库或 pg。
- **[R8] `ContactSource` 接口在官方 API 接入后可能需要变形** → v1 的接口只定义七个动词，且签名只接 `[]string` / `string` 这些原语；把字段映射放 `ContactSnapshot` 里，接口应该稳定。若官方 API 引入分页 cursor，可以加 `ListExternalContactsPaged` 而不破坏旧方法。

## Migration Plan

1. 在 `feat/qiwei-contact-gateway` 分支开发。
2. 本地 `go test ./channel-qiwei/...`；为 `contactRepo` 的 upsert 语义写单元测试（尤其 identity_links 的覆盖分支）。
3. `make qiwei` 起本地进程，跑两种场景：
   - 有历史 `qiwei.db` 的老库：验证新表迁移无误，旧行为不变。
   - 空库 + 新账号：验证首启 → 事件进来 → 数据落库 → HTTP 查询返回。
4. 先灰度到单个非生产的企微账号，观察 6h 周期运行两次后的 `contact_sync_last_error` 是否持续为空。
5. 小程序后端侧接 `/identity-links` 写入；观察 `identity_link_count` 上涨节奏符合用户登录量级。
6. 全量上线后，保留 1 周 `qiwei.db.bak`（部署脚本负责）以便回滚。
7. Rollback：回滚到上一个二进制；新表留空无副作用；若要重置，`DROP TABLE qiwei_contacts / qiwei_rooms / qiwei_room_members / qiwei_identity_links` 即可。

## Open Questions

- `gateway_token` 是否允许按 `downstream_system` 多 token？v1 单 token；等第 2 个下游接入再拆。
- `POST /identity-links` 覆盖写入时是否需要要求前端带上 `expected_external_user_id`（类似 CAS）？v1 直接覆盖 + audit，等出现实际安全事件再加。
- `raw_json` 是否加上大小上限（截断 64KB）？QiWe 单条 payload 不会超，但官方 API 若把大头像 base64 带进来会胀，写入前做一次 `len>65536 => 丢弃` 的 guard，由 repo 负责。
- 群成员的 `role` 字段 QiWe 协议直接给不给？若不给，v1 只写 owner（`owner_user_id == user_id` 时推断），其余都写 `member`；`admin` 这一类等官方 API 接入。
