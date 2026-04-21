## ADDED Requirements

### Requirement: 联系人持久化

系统 SHALL 在 `qiwei.db` 中维护 `qiwei_contacts` 表，按 `(account_id, user_id)` 主键存储每个企微账号视角下的联系人主数据，字段至少包含 `external_user_id / source / nickname / real_name / alias / remark / avatar_url / gender / corp_id / corp_name / follow_user_json / raw_json / first_seen_at / last_synced_at / updated_at`。`external_user_id` 列 MUST 建索引。

#### Scenario: 首次出现的联系人被写入

- **WHEN** QiWe 回调或接口返回一条此前未在 `qiwei_contacts` 出现的 `user_id`
- **THEN** 系统 SHALL 插入一行，`first_seen_at / last_synced_at / updated_at` 置为当前时间，`external_user_id` 若上游提供则落库

#### Scenario: 已存在联系人的字段刷新

- **WHEN** 同一 `(account_id, user_id)` 再次被同步并且任一可变字段发生变化
- **THEN** 系统 SHALL 执行 upsert，`first_seen_at` 保持不变，`last_synced_at / updated_at` 更新为当前时间，变化字段被覆盖，`raw_json` 记录本次上游 payload

#### Scenario: 账号被硬删除

- **WHEN** `qiwei_accounts` 中某行被 `hard=1` 真删
- **THEN** 该 `account_id` 对应的 `qiwei_contacts` 行 SHALL 随之删除（外键 ON DELETE CASCADE）

---

### Requirement: 群与群成员持久化

系统 SHALL 维护 `qiwei_rooms`（按 `(account_id, room_id)`）与 `qiwei_room_members`（按 `(account_id, room_id, user_id)`）两张表。群表字段至少包含 `name / announcement / owner_user_id / member_count / qr_code_url / raw_json / first_seen_at / last_synced_at / updated_at`；成员表字段至少包含 `display_name / role / joined_at / last_seen_at`。

#### Scenario: 机器人新加入的群首次同步

- **WHEN** webhook 触发 `group_joined` 或 `member_joined`（首次见到该 room_id）
- **THEN** 系统 SHALL 异步调用上游 `BatchGetRoomDetail` 并 upsert `qiwei_rooms` 与对应 `qiwei_room_members`

#### Scenario: 群名变更事件

- **WHEN** webhook 触发 `group_name_changed`
- **THEN** 系统 SHALL 刷新 `qiwei_rooms.name` 与 `updated_at / last_synced_at`，并清空内存 `nameCache` 中 `room:<roomId>` 项

#### Scenario: 群成员离开

- **WHEN** webhook 触发 `member_removed` 或 `member_quit`
- **THEN** 系统 SHALL 从 `qiwei_room_members` 删除对应 `(account_id, room_id, user_id)` 行，`qiwei_rooms.member_count` 在下次全量同步时对齐（不要求事件内强一致）

---

### Requirement: 身份映射表

系统 SHALL 提供 `qiwei_identity_links` 表，记录企微 `(account_id, user_id, external_user_id)` 与下游系统（小程序、Web、CRM 等）唯一 ID 之间的多对一映射，按 `(downstream_system, downstream_id)` UNIQUE。

#### Scenario: 小程序侧首次登记映射

- **WHEN** 下游调用 `POST /api/qiwei/gateway/identity-links` 携带一个新的 `(downstream_system, downstream_id)`
- **THEN** 系统 SHALL 插入一行，`created_at / updated_at` 置为当前时间，返回 201 与写入后的 id

#### Scenario: 相同下游 ID 的幂等更新

- **WHEN** 同一个 `(downstream_system, downstream_id)` 携带与库内一致的 `(account_id, external_user_id)` 再次提交
- **THEN** 系统 SHALL 仅更新 `downstream_meta_json / updated_at`，返回 200 与既有 id

#### Scenario: 下游 ID 被指派到新的 external_user_id

- **WHEN** 同一个 `(downstream_system, downstream_id)` 提交时携带的 `external_user_id` 与库内不同
- **THEN** 系统 SHALL 覆盖 `account_id / user_id / external_user_id`，更新 `updated_at`，并通过 `logger.Business` 写入一条含旧值与新值的 audit 记录

---

### Requirement: 事件驱动同步

系统 SHALL 在 webhook 事件通路上对联系人和群数据做低延迟 upsert，不阻塞消息转发。

#### Scenario: 新好友通过后同步联系人

- **WHEN** `autoAcceptFriendRequest` 返回成功
- **THEN** 系统 SHALL 异步入队一次针对该 `(account_id, contact_id)` 的 `upsertContact`，调用上游 `BatchGetUserInfo` 并写入 `qiwei_contacts`

#### Scenario: 群事件后同步群主数据

- **WHEN** 处理 `member_joined / member_removed / group_name_changed / group_joined` 事件
- **THEN** 系统 SHALL 异步入队一次针对该 `(account_id, room_id)` 的 `upsertRoom`

#### Scenario: 消息发送者在库外

- **WHEN** 正常消息处理流程中发现 `sender_id` 未出现在 `qiwei_contacts`
- **THEN** 系统 SHALL 异步入队一次 `upsertContact`，当前消息不等待同步完成即继续转发

#### Scenario: 同步队列拥塞

- **WHEN** 某账号事件驱动队列（容量 64）已满
- **THEN** 系统 SHALL 丢弃最旧任务，以 `warn` 级别日志记录并计数，不阻塞入队侧

---

### Requirement: 周期性全量同步

系统 SHALL 以 `contact_sync_interval`（默认 6 小时）为周期，对每个 enabled 账号运行一次全量同步作为兜底；失败 SHALL 按指数退避，上限 1 小时；`contact_sync_enabled=false` 时 SHALL 跳过所有周期性同步。

#### Scenario: 周期性同步成功

- **WHEN** 周期触发对某账号运行全量同步，且所有上游调用成功
- **THEN** 系统 SHALL 依次刷新 `qiwei_contacts / qiwei_rooms / qiwei_room_members`，更新该账号的 `contact_sync_last_at`，清空该账号的失败计数

#### Scenario: 周期性同步失败

- **WHEN** 周期触发全量同步但上游调用失败
- **THEN** 系统 SHALL 记录 `contact_sync_last_error`，累加失败次数，下次触发时间按 `min(30s * 2^(fails-1), 1h)` 延后

#### Scenario: 管理员手动触发

- **WHEN** 调用 `POST /api/qiwei/_admin/accounts/{id}/resync`
- **THEN** 系统 SHALL 立即将该账号加入下一轮运行队列，响应 202，同步结果通过后续 `sync_status` 端点可见

---

### Requirement: ContactSource 抽象

系统 SHALL 在代码层定义 `ContactSource` interface，封装"从哪个数据源拉取联系人/群/身份信息"。v1 实现 SHALL 基于 QiWe 第三方协议；interface 签名与 `ContactSnapshot / RoomSnapshot` 结构体 SHALL 使用企微官方术语（`external_user_id / corp_id / follow_user` 等），为后续接入企微官方 API 预留切换点。

#### Scenario: 字段语义对齐企微官方

- **WHEN** `qiweProtoSource` 把 QiWe 协议返回的 `openid` 字段映射到 `ContactSnapshot`
- **THEN** 该值 SHALL 写入 `ExternalUserID` 字段并最终落到 `qiwei_contacts.external_user_id` 列

#### Scenario: 原始 payload 可追溯

- **WHEN** `ContactSource` 方法返回任何一条 `ContactSnapshot` 或 `RoomSnapshot`
- **THEN** 该记录 SHALL 附带上游原始 payload（以 `Raw map[string]any` 或等价结构），并由 repo 写入对应行的 `raw_json` 列（超过 64KB 时 SHALL 截断并打一条 warn 日志）

---

### Requirement: Runtime 读路径三级回源

系统 SHALL 以"内存 ttl 缓存 → SQLite → 上游 API"三级顺序解析联系人姓名与群名；任一级命中 SHALL 立即返回并把结果回写到更高速的层级；事件通路中的解析 MUST NOT 因 DB 或上游失败而阻塞消息转发。

#### Scenario: ttlCache 命中

- **WHEN** `ResolveName(userID)` 被调用且 `nameCache` 含该 userID
- **THEN** 系统 SHALL 直接返回缓存值，不访问 SQLite 或上游

#### Scenario: DB 命中

- **WHEN** `nameCache` 未命中但 `qiwei_contacts` 存在该行
- **THEN** 系统 SHALL 返回 DB 中的 `nickname / real_name / alias / remark` 的第一个非空值，并写回 `nameCache`

#### Scenario: 回源 API

- **WHEN** cache 与 DB 都未命中
- **THEN** 系统 SHALL 调用 `ContactSource.BatchGetUserInfo`，把结果 upsert 进 `qiwei_contacts` 并回填 cache，同步失败时返回空串但不产生 panic

---

### Requirement: 下游 HTTP 查询 API

系统 SHALL 在 `/api/qiwei/gateway` 下挂载一组只读（加一个 idempotent POST）接口，按 `X-Gateway-Token` header 鉴权；`gateway_token` 配置为空时 SHALL NOT 注册这组路由。

#### Scenario: 按 senderId 解析联系人

- **WHEN** `GET /api/qiwei/gateway/contacts/resolve?accountId=<id>&senderId=<uid>` 携带合法 token
- **THEN** 系统 SHALL 返回 `{contact, externalUserId, identityLinks[], lastSyncedAt}`，未命中时返回 404，token 缺失或错误时返回 401

#### Scenario: 按 externalUserId 反查

- **WHEN** `GET /api/qiwei/gateway/contacts/resolve?accountId=<id>&externalUserId=<euid>` 携带合法 token
- **THEN** 系统 SHALL 返回同样结构的 payload；同一 `external_user_id` 对应多条 `qiwei_contacts` 行时（历史残留），SHALL 返回 `last_synced_at` 最新的一条

#### Scenario: 强制刷新

- **WHEN** `GET /api/qiwei/gateway/contacts/resolve?...&refresh=1`
- **THEN** 系统 SHALL 同步调用 `ContactSource` 回源一次再返回；同一 `(account_id, user_id)` 的强制刷新 SHALL 被限流为每 30 秒最多一次，超限返回 429

#### Scenario: 未注册路由的安全默认

- **WHEN** 配置 `gateway_token` 为空而下游请求 `/api/qiwei/gateway/contacts/resolve`
- **THEN** 系统 SHALL 返回 404（不暴露该端点存在）

---

### Requirement: Admin 可观测性扩展

系统 SHALL 在既有 admin API 上扩展 contact gateway 的运行指标，使运营能够判断同步健康度。

#### Scenario: 账号详情扩展

- **WHEN** 调用 `GET /api/qiwei/_admin/accounts/{id}`
- **THEN** 返回 payload SHALL 额外包含 `contact_count / room_count / room_member_count / contact_sync_last_at / contact_sync_last_error / identity_link_count`

#### Scenario: 同步状态查看

- **WHEN** 调用 `GET /api/qiwei/_admin/accounts/{id}/sync_status`
- **THEN** 系统 SHALL 返回 `{queueDepth, recentRuns: [{at, result, error?}]}`，`recentRuns` 保留最近 10 次运行（内存 ring buffer 即可）
