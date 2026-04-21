## ADDED Requirements

### Requirement: 账号注册表持久化于 SQLite
系统 SHALL 将企微账号注册表持久化到 channel-qiwei 独立的 SQLite 数据库文件，位置由 `cfg.DBPath` 决定，默认 `{data_dir}/qiwei.db`；表 `qiwei_accounts` 至少包含列 `id, guid, token, short_hash, display_name, agent_id, enabled, self_user_id, self_name, self_alias, self_avatar_url, self_corp_name, self_synced_at, meta_json, notes, created_at, updated_at`，其中 `guid` 和 `short_hash` 具有 UNIQUE 约束，`agent_id` 建立非唯一索引。

#### Scenario: 首次启动建表
- **WHEN** 进程首次启动且 `qiwei.db` 不存在
- **THEN** 系统在 `cfg.DataDir` 下创建 `qiwei.db`，并在其中建出 `qiwei_accounts` 与 `qiwei_known_rooms` 两张表，启用 WAL 模式，`busy_timeout` 至少 5000ms

#### Scenario: 启动时发现空表且 YAML 配有顶层账号
- **WHEN** `qiwei_accounts` 行数为 0，且 `config.yaml` 顶层配有非空 `guid` 与 `token`
- **THEN** 系统将该 YAML 账号作为首个默认账号 INSERT 到 `qiwei_accounts`，`id` 默认使用 `guid`，`display_name` 默认为 `"default"`，`short_hash` 由 guid 派生

#### Scenario: short_hash 碰撞检测
- **WHEN** 新增账号时生成的 `short_hash` 已被现有行占用
- **THEN** 数据库 UNIQUE 约束拒绝插入，HTTP 接口返回 409 Conflict，记录 `error` 日志并保留原账号不变

### Requirement: 已知群列表按账号维度持久化
系统 SHALL 用表 `qiwei_known_rooms(account_id, room_id, first_seen_at, PRIMARY KEY(account_id, room_id))` 替代原来的 `known_rooms.txt`；首启时若旧文件存在，应将其内容作为默认账号的已知群一次性导入该表，并将旧文件重命名为 `known_rooms.txt.migrated.{unix_timestamp}`。

#### Scenario: 首启完成 legacy 文件迁移
- **WHEN** 首启 seed 完成且 `{data_dir}/known_rooms.txt` 存在
- **THEN** 系统为默认账号批量 INSERT OR IGNORE 每条 room_id 到 `qiwei_known_rooms`，然后将旧文件重命名为 `known_rooms.txt.migrated.{unix_timestamp}`，并在日志中写入迁移条数

#### Scenario: 运行时新群首次出现
- **WHEN** 某账号收到 `member_joined` 回调，且 `(account_id, room_id)` 在 `qiwei_known_rooms` 中不存在
- **THEN** 系统向表中插入该行并把事件类型改写为 `group_joined` 上报给 Agent

### Requirement: accountRuntime 与全局状态隔离
系统 SHALL 为每个启用账号构造独立的 `accountRuntime`，持有账号私有的 `qiweiClient`、`roomStore`、`nameCache`、`selfUserID`、联系人同步状态和 `msgSvrId` 去重集合；跨账号的请求 MUST NOT 共用任何以上状态。

#### Scenario: 两账号收到相同 msgSvrId 的回调
- **WHEN** 账号 A 和账号 B 分别收到 `msgSvrId="m123"` 的回调
- **THEN** 两条消息各自被各自 runtime 的 dedupe 集合记录，各自都会被处理并转发给 Agent，不会因 dedupe 而丢弃其中任何一条

#### Scenario: 账号间联系人缓存不互相污染
- **WHEN** 账号 A 的 `nameCache` 缓存了 `userId="u1" → "张三"`，而账号 B 收到同一个 `u1` 的消息
- **THEN** 账号 B 使用自己的 `nameCache` 解析用户名，不会读取到账号 A 的 "张三"

### Requirement: 透明复合 conversationId
系统 SHALL 在上行到 Agent 的 `incomingMessage.channelConversationId` 中以 `{rawId}@{shortHash}` 形式编码账号归属；`rawId` 为企微原始 `roomId`（群）或 `userId`（单聊），`shortHash` 为 `qiwei_accounts.short_hash` 列。Agent 视之为不透明字符串；下行时系统 SHALL 从字符串末尾 `@` 之后解析出 `shortHash` 并据此定位目标 `accountRuntime`。

#### Scenario: 单聊上行
- **WHEN** 账号 short_hash=`abc12345` 收到单聊消息，企微 `senderId="U:999"`，`fromRoomId="0"`
- **THEN** `incomingMessage.channelConversationId == "U:999@abc12345"`

#### Scenario: 群聊上行
- **WHEN** 账号 short_hash=`abc12345` 收到群 `R:123` 的消息
- **THEN** `incomingMessage.channelConversationId == "R:123@abc12345"`

#### Scenario: Agent 回复下行路由
- **WHEN** Agent 回发 `outgoingMessage.channelConversationId == "R:123@abc12345"`
- **THEN** 系统解析出 `shortHash="abc12345"`，查 registry 得到对应 `accountRuntime`，用其 client 以 `toId="R:123"` 调企微 API

#### Scenario: 下行后缀缺失且仅单账号
- **WHEN** Agent 回发的 `channelConversationId` 不含 `@{shortHash}` 后缀，且系统当前仅有一个 enabled 账号
- **THEN** 系统使用该唯一账号完成发送，并在日志中记录 `warn` 级提示"legacy conversationId format"

#### Scenario: 下行后缀缺失且多账号
- **WHEN** Agent 回发的 `channelConversationId` 不含 `@{shortHash}` 后缀，且系统启用的账号数量大于 1
- **THEN** 系统返回 HTTP 400，body 指出缺少账号标识

#### Scenario: 下行后缀无法匹配任何账号
- **WHEN** Agent 回发的 `channelConversationId` 含有不在 registry 的 `shortHash`
- **THEN** 系统返回 HTTP 400，日志记录 `warn` 级含原 conversationId 与解析出的 shortHash

### Requirement: 按 GUID 路由回调
系统 SHALL 在 `POST /webhook/callback` 入口按 payload 中每条消息的 `guid` 字段路由到对应 `accountRuntime`；对应 guid 未在 registry 注册或该账号 `enabled=0` 时，系统 SHALL 直接放弃该条消息，并记录到未知 guid 的审计缓冲。

#### Scenario: 已注册 guid 的回调
- **WHEN** 回调中 `msg.guid == "G1"` 且 registry 中存在启用态账号 `guid="G1"`
- **THEN** 该消息的全部下游处理（dedupe、解析、转发 Agent 等）均使用该账号的 `accountRuntime`

#### Scenario: 未注册 guid 的回调
- **WHEN** 回调中 `msg.guid == "Gunknown"` 且 registry 中无匹配账号
- **THEN** 系统以 `warn` 级记录 `"unregistered guid"` 日志，并将该事件（含 guid、msgType、createTime）写入容量受限的内存 ring buffer（最多保留最近 200 条），用于 admin 排障

#### Scenario: guid 匹配但账号被禁用
- **WHEN** 回调 `guid="G1"`，`qiwei_accounts.enabled=0`
- **THEN** 系统按"未注册 guid"语义丢弃，不触发任何下游；ring buffer 记录并标注 reason=`disabled`

### Requirement: 企微号↔Agent 是 N:1 关系
系统 SHALL 允许多行 `qiwei_accounts` 指向同一个 `agent_id`；`agent_id` 是该企微号上行给 Agent 时 `incomingMessage.agentId` 的取值来源，不要求在全局或 per-agent 范围内唯一。

#### Scenario: 同一 Agent 绑定两个企微号
- **WHEN** `qiwei_accounts` 中两行分别为 `(id=a1, agent_id="agent-x")` 和 `(id=a2, agent_id="agent-x")`
- **THEN** 这两个账号收到消息都会以 `agentId="agent-x"` 上行给 Agent，两者在 Agent 侧因 `channelConversationId` 后缀不同天然分成两个 session

#### Scenario: 按 agent_id 反查账号列表
- **WHEN** 管理端调用 `GET /api/qiwei/_admin/accounts?agent_id=agent-x`
- **THEN** 系统返回所有 `agent_id="agent-x"` 的账号，按 `created_at` 升序

### Requirement: 企微号身份快照同步
系统 SHALL 在启动时以及每 `profile_sync_interval`（默认 6h）为每个启用账号调用一次 `/user/getProfile`，并把返回的 `userId / nickname / alias / avatarUrl / corpName` 写入 `qiwei_accounts` 对应列，`self_synced_at` 更新为当前 Unix 秒。失败 MUST 不阻塞消息收发，MUST 以 `warn` 记录，MUST 采用指数退避，最大间隔不超过 1 小时。

#### Scenario: 启动同步
- **WHEN** 服务完成 DB 初始化与 registry 构建
- **THEN** 系统并发对所有 `enabled=1` 账号执行一次 `/user/getProfile` 并持久化结果；同步失败的账号 `self_synced_at` 保持为 0

#### Scenario: 周期同步
- **WHEN** 距离上次同步成功已达 `profile_sync_interval`
- **THEN** 系统再次对所有启用账号拉取并更新快照，结果变更时 `updated_at` 也跟随刷新

#### Scenario: 同步失败退避
- **WHEN** 某账号连续 N 次同步失败
- **THEN** 该账号的下次尝试时间不早于 `min(base * 2^N, 1h)`；其它账号的节奏不受影响

#### Scenario: 手动触发
- **WHEN** 管理端调用 `POST /api/qiwei/_admin/accounts/{id}/refresh`
- **THEN** 系统立即对该账号执行一次同步，接口在同步完成后返回最新快照

### Requirement: 对 Agent 暴露 channelIdentity 展示字段
系统 SHALL 在上行到 Agent 的 `incomingMessage` 中新增 `channelIdentity` 字段，结构为 `{ displayName, self: { userId, name, alias, corpName } }`；字段值取自 `qiwei_accounts` 当前行。字段 MUST NOT 包含 `accountId`、`guid` 或 `short_hash`，以确保 Agent 不会据此做路由分支。

#### Scenario: 身份已同步
- **WHEN** 账号 `self_name="机器人小王"`、`display_name="运营小王"`、`self_alias="xiaowang"`
- **THEN** `incomingMessage.channelIdentity == { displayName: "运营小王", self: { userId: "<self_user_id>", name: "机器人小王", alias: "xiaowang", corpName: "<self_corp_name>" } }`

#### Scenario: 身份未同步
- **WHEN** 账号 `self_synced_at == 0`
- **THEN** `channelIdentity.self` 对应字段为空字符串（不得省略整个 `channelIdentity` 字段），`displayName` 取 `qiwei_accounts.display_name`

### Requirement: 账号管理 HTTP API（完整 CRUD）
系统 SHALL 暴露 `/api/qiwei/_admin/accounts` 下的完整管理接口，包括列表、新建、查询详情、更新、软删、硬删和手动刷新快照；每次成功写操作 MUST 在响应返回前完成 registry reload。

#### Scenario: 列表分页与过滤
- **WHEN** `GET /api/qiwei/_admin/accounts?agent_id=agent-x&enabled=1`
- **THEN** 响应 200，body 含满足条件的账号数组，每项字段为数据表列（不含 `token`，仅展示 `token_preview` 前 4 位）

#### Scenario: 新建账号
- **WHEN** `POST /api/qiwei/_admin/accounts`，body `{guid, token, display_name, agent_id}`
- **THEN** 系统生成 `short_hash` 与 `id`（若 body 未指定 `id` 则默认 `id=guid`），INSERT 成功后同步 self profile 一次，然后 reload registry，响应 201 含完整账号对象（`token_preview` 形式）

#### Scenario: 新建账号 guid 冲突
- **WHEN** `POST` 的 `guid` 已被现有账号占用
- **THEN** 响应 409，body 标注冲突字段；不触发 reload

#### Scenario: 更新字段白名单
- **WHEN** `PATCH /api/qiwei/_admin/accounts/{id}` body 携带 `{token, display_name, agent_id, enabled, notes}`
- **THEN** 系统只更新这五个字段，其它字段被忽略；`updated_at` 刷新；reload registry

#### Scenario: 软删
- **WHEN** `DELETE /api/qiwei/_admin/accounts/{id}`（未带 `hard=1`）
- **THEN** 系统将 `enabled` 置为 0，不删除行；reload registry，该账号从 byGUID/byShortHash 索引中移除

#### Scenario: 硬删
- **WHEN** `DELETE /api/qiwei/_admin/accounts/{id}?hard=1`
- **THEN** 系统删除 `qiwei_accounts` 对应行及其 `qiwei_known_rooms` 下所有行，reload registry

#### Scenario: 手动刷新
- **WHEN** `POST /api/qiwei/_admin/accounts/{id}/refresh`
- **THEN** 系统立即对该账号调用 `/user/getProfile`，持久化结果并返回 200 + 最新账号对象；若接口调用失败，响应 502 + 上游错误原因

#### Scenario: 显式 reload
- **WHEN** `POST /api/qiwei/_admin/accounts/reload`
- **THEN** 系统重新从 DB 加载所有账号并原子替换 registry，响应 200 + 现役账号数量

### Requirement: Admin 端点鉴权
系统 SHALL 用 `X-Admin-Token` HTTP 头对 `/api/qiwei/_admin/*` 下所有端点进行鉴权；token 取自 `cfg.AdminToken`（YAML 字段 `admin_token`）。当 `cfg.AdminToken` 为空字符串时，系统 MUST 禁用整个 admin 端点组并对其下任意路径返回 404。

#### Scenario: token 正确
- **WHEN** 请求携带 `X-Admin-Token` 与 `cfg.AdminToken` 相等且非空
- **THEN** 请求进入正常处理流程

#### Scenario: token 错误
- **WHEN** 请求携带 `X-Admin-Token` 与 `cfg.AdminToken` 不匹配
- **THEN** 响应 401 Unauthorized，不暴露 token 对比细节

#### Scenario: token 未配置
- **WHEN** `cfg.AdminToken == ""`，外部请求 `GET /api/qiwei/_admin/accounts`
- **THEN** 响应 404，与其它未注册路径一致，不暴露 admin 接口存在

### Requirement: Registry 热 reload 语义
系统 SHALL 提供对 `accountRegistry` 的原子替换能力：任意成功的账号写操作之后，系统 MUST 从 DB 重新查询所有账号并构建新的 registry 实例，一次性替换 app 持有的引用；已在飞的回调与请求 MUST 基于调用开始时的 registry 快照完成，MUST NOT 被替换打断。

#### Scenario: 写入后的新请求看到新 registry
- **WHEN** `PATCH` 成功修改某账号 `enabled` 为 0 后的下一个 `POST /webhook/callback` 带该账号 guid
- **THEN** 该回调按"未注册 guid"语义被丢弃

#### Scenario: 并发中的旧请求看到旧 registry
- **WHEN** 回调 handler 已开始处理、尚未走完完整上行流程期间发生 registry 替换
- **THEN** 该 handler 的后续步骤继续使用其开始时引用到的 `accountRuntime`，完成这次处理不受影响

### Requirement: 未知 guid 审计缓冲
系统 SHALL 维护一个容量上限 200 的内存 ring buffer 记录最近的未知/禁用 guid 事件，并通过 `GET /api/qiwei/_admin/unknown_guids` 暴露，用于运营排障。事件最少包含 `guid, msgType, createTime, reason("unregistered"|"disabled"), observed_at`。

#### Scenario: 缓冲写入
- **WHEN** 回调被"未注册 guid"或"guid 禁用"语义丢弃
- **THEN** 事件被追加到缓冲尾部；超出容量时最旧事件被淘汰

#### Scenario: 查询缓冲
- **WHEN** `GET /api/qiwei/_admin/unknown_guids` 通过鉴权
- **THEN** 响应 200，body 数组按 `observed_at` 倒序

### Requirement: 首启 seed 向后兼容旧 YAML
系统 SHALL 保留 `config.yaml` 顶层 `guid / token / agent_id` 字段的读取路径，但仅在 `qiwei_accounts` 行数为 0 时生效；生效时系统 MUST 以这些值构造首个默认账号持久化入库，之后即便 YAML 中这些字段发生变化也 MUST NOT 覆盖库中的值。

#### Scenario: 库非空时忽略 YAML 顶层账号字段
- **WHEN** `qiwei_accounts` 至少有 1 行，且 YAML 仍配有顶层 `guid/token`
- **THEN** 系统记录 `info` 日志提示 YAML 顶层账号字段已失效，不触发任何写库操作

#### Scenario: 库空且 YAML 无顶层账号字段
- **WHEN** `qiwei_accounts` 为空且 YAML 顶层 `guid` 与 `token` 之一为空
- **THEN** 系统不写入 seed，启动成功但打印 `warn` 提示"no accounts configured"，admin API 仍可工作以便 POST 新建账号
