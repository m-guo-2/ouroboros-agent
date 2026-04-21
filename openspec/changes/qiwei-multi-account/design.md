## Context

`channel-qiwei` 是 Moli 平台连接企微（QiWe 第三方协议）的单向渠道服务。当前架构（截至 `feat/qiwei`）：

- `app` 结构全局持有 `qiweiClient / roomStore / nameCache / selfUserID / dedupe`，硬编码"一个进程 = 一个企微号"。
- 配置来源是 `config.yaml`（顶层 `token / guid / agent_id`）。
- `known_rooms.txt` 纯文件落盘，没有账号维度。
- 对 Agent 暴露的 `incomingMessage.channelConversationId` 是原始企微 `roomId / userId`；`agentId` 是全局常量。
- 仓库整体已经深度使用 SQLite（`modernc.org/sqlite`，无 CGO），`agent/internal/storage/db.go` 提供了可直接类比的范式。

运营诉求（用户给出）：

1. 一个 Agent 要能挂 N 个企微号（N:1）。
2. Agent 代码不要感知"当前是哪个企微号"的路由分支。
3. 每个企微号的身份信息（昵称/头像/corp）要能在系统里被查到。

N:1 + Agent 不区分账号这两条叠加，产生一个关键约束：**同一个 Agent 下的两个企微号可能被拉进同一个客户群**，此时两个 guid 各自会收到同一条用户消息的回调。如果 `channelConversationId` 维持原值，Agent 按 `(agent_id, conversationId)` 建 session 会把两条消息合并，下行回复也无法判断走哪个号。本文档给出的方案必须在这种场景下保持行为确定。

## Goals / Non-Goals

**Goals**

- 一个 channel-qiwei 进程承载 N 个企微账号；每账号独立 client、状态、去重。
- 账号生命周期（新增/禁用/换 token/删除）通过 HTTP API 完成，支持运行时热 reload。
- 账号身份信息（self profile）定期同步并持久化；对 Agent 以展示字段呈现，不作路由依据。
- Agent 代码零改动即可工作；新旧 Agent 客户端兼容（单账号 deployment 兼容回退）。
- 企微号↔Agent 是 N:1：多行 `qiwei_accounts` 可指向同一个 `agent_id`；按 agent 反查账号列表可用。

**Non-Goals**

- 不做 Agent 层面的"多号聚合策略"（比如跨号消息合流、"总机"视角）；Agent 仍以会话为独立单元。
- 不做企微号的自动登录/扫码/托管（token/guid 的获取仍在 QiWe 第三方平台完成，本服务只消费）。
- 不把 OSS/火山/AgentServer 等基础设施配置挪进数据库（维持 YAML）。
- 不在 agent 侧改动 schema、session 路由、历史数据迁移。
- 不为已有 Agent session 做 conversationId 向后兼容——承担一次性"session 重建"的代价。

## Decisions

### D1. 路由 key：透明复合 conversationId（`{rawId}@{shortHash}`）

对 Agent 暴露的 `channelConversationId` = `原 roomId 或 userId` + `@` + `guid 的 8 位短哈希`。Agent 当不透明字符串原样回存、原样回传；Channel 在下行路径上按后缀反查 `accountRuntime`。

替代方案及放弃理由：

- **映射表（conversationId → accountId）**：同群多号会冲突，冲突修复等价于再把 accountId 拼进 key，逻辑复杂度反超 D1；另外引入一致性问题（该表必须在上行前写、下行时查，崩溃恢复麻烦）。
- **URL 路径携带 accountId**（`/api/qiwei/{accountId}/send`）：需要改 Agent 下行规则、每个企微回调地址单独配置，迁移代价高。
- **Agent 侧显式带 accountId**：违反"Agent 不区分企微号"原则。

短哈希取自 guid 的 SHA-256 前 8 字节的 base32（去掉 `=` 填充），碰撞概率远低于一个部署里可能的账号数量；且 `short_hash` 作为列存进 `qiwei_accounts`，插入账号时就算出来并保证 UNIQUE。

### D2. 存储：独立 SQLite 文件 `{data_dir}/qiwei.db`

不与 `agent/config.db` 合用。原因：

- `channel-qiwei` 有独立 `go.mod`，跨模块共享 SQLite 文件会在部署/打包/备份上耦合。
- 两个进程同时写同一个 WAL 文件没有收益，只有复杂度。
- schema 演进节奏不同。

迁移使用 `agent/internal/storage/db.go` 的同款模式：`modernc.org/sqlite` + WAL + `busy_timeout=5000` + `MaxOpenConns=1`，schema 用幂等 `CREATE TABLE IF NOT EXISTS` 数组，增量 migration 放 `ALTER`/`CREATE TABLE` 列表里用 `db.Exec` 忽略错误。

### D3. 账号注册表与运行时分离

- `Account`（数据层对象）：与表行一一对应，只是纯数据。
- `accountRuntime`（运行时对象）：基于 Account 构造，持有 `*qiweiClient / roomStore / nameCache / selfUserID / dedupe / contacts sync state`。
- `accountRegistry`：`{ byID, byGUID, byShortHash }` 三个 map + `sync.RWMutex`。
- Reload 时重新查全表，构建新的 registry 并原子替换；旧 runtime 不再接收新请求，已在飞的 context 自然完成。

### D4. 身份快照同步

- 启动时：并发遍历 enabled 账号，每个各跑一次 `/user/getProfile`（QiWe 标准接口），把 `selfUserID / selfName / selfAlias / selfAvatar / selfCorpName` 写回 `qiwei_accounts`。
- 周期性：后台 goroutine 每 `profile_sync_interval`（默认 6h）循环一次。
- 手动：`POST /api/qiwei/_admin/accounts/{id}/refresh` 触发单账号同步。
- 失败降级：同步失败只记 warn，不阻塞消息收发；`self_synced_at` 为 0 视作"未同步"，Agent 侧 `channelIdentity.self` 字段为空对象即可。

### D5. Agent 契约：新增 `channelIdentity` 字段，仅作展示

```json
"channelIdentity": {
  "displayName": "<accounts.display_name>",
  "self": {
    "userId": "<accounts.self_user_id>",
    "name":   "<accounts.self_name>",
    "alias":  "<accounts.self_alias>",
    "corpName": "<accounts.self_corp_name>"
  }
}
```

- Agent 未读时零影响。
- Agent 若要让机器人"自称 XXX"，读 `channelIdentity.self.name` 即可。
- 字段**不含** `accountId / guid`，避免 Agent 无意间用它做路由分支。

下行侧：`outgoingMessage` 不加 account 字段，路由只看 `channelConversationId` 后缀。缺后缀（老 Agent 或测试）时：

- 仅一个 enabled 账号 → 走唯一账号。
- 多账号 → 400。

### D6. 账号管理 HTTP API（完整版）

基准路径 `/api/qiwei/_admin/accounts`，统一 `X-Admin-Token` header 鉴权，token 来自 YAML `admin_token`（为空时禁用整个 admin 端点集而不是"任何人都能访问"）。

| Method | Path                                  | 说明                                         |
| ------ | ------------------------------------- | -------------------------------------------- |
| GET    | `/`                                   | 列表，可选 `?agent_id=xxx` / `?enabled=1`    |
| POST   | `/`                                   | 新建账号（`{guid, token, display_name, agent_id, enabled?, notes?}`） |
| GET    | `/{id}`                               | 详情                                         |
| PATCH  | `/{id}`                               | 更新（只允许 `token / display_name / agent_id / enabled / notes`） |
| DELETE | `/{id}`                               | 软删，置 `enabled=0`                          |
| POST   | `/{id}/refresh`                       | 立即拉取 self profile                         |
| POST   | `/reload`                             | 强制从数据库 reload registry（正常不需要，写接口已自动 reload） |

每个写操作执行后自动触发 registry reload。

### D7. 首启 seed 与迁移

启动序列：

1. 打开 DB、建表。
2. 查 `SELECT COUNT(*) FROM qiwei_accounts`。若为 0：
   - 读 YAML 顶层 `guid / token / agent_id`，存在则写入 `qiwei_accounts`，`id` 默认用 `guid`，`display_name` 默认 `"default"`。
   - 若 `{data_dir}/known_rooms.txt` 存在，把每行 room_id 写入 `qiwei_known_rooms` 归属到这个默认账号（一次性迁移），然后把旧文件 rename 成 `.migrated.{timestamp}` 归档。
3. 构建 registry、启动 profile sync goroutine。

旧部署升级无需手工干预；新部署用 admin API 创建账号。

### D8. 账号级 `dedupe` vs 全局

现在 `dedupe` 是全局 `ttlSet(5min)`，判重 key 是 `msgSvrId`。企微的 `msgSvrId` 在不同 guid 下有重叠可能性（不同实例各自生成），放全局可能误判。改为 per-account，简单且正确。代价：同群多号时同一事件的两条回调各自独立处理——这正是 D1 的预期行为。

### D9. `roomStore` 落盘改为 DB 背书

废弃 `known_rooms.txt` 的写路径，改为 `roomStore` 直接读写 `qiwei_known_rooms` 表。内存 map 仍在（热路径 O(1) 命中），启动时 bulk load。

## Risks / Trade-offs

- **[R1] conversationId 形态变化导致历史 session 断裂** → 本次就是 BREAKING；Agent 侧历史 session 在新消息进来时会被建成新 session，旧 session 自然沉默。运营提前周知；必要时 admin 在 agent 侧手动归档。
- **[R2] shortHash 碰撞** → 8 字节 base32 碰撞概率约 2^{-40}；`short_hash` 列带 UNIQUE 约束，插入时数据库阻止碰撞；万一发生，新账号创建失败并返回 409，运营改别名（本质是 guid 派生，所以实际路径是"建议更长 hash"—— v1 不做 over-engineering，出现再加）。
- **[R3] SQLite 单写锁下的 QPS 瓶颈** → channel-qiwei 的写路径主要是 `qiwei_known_rooms` 的 insert 和 profile 同步的 update，QPS 远小于 agent 侧。保持 `MaxOpenConns=1 + WAL` 即可；压力仍不够再换 pg。
- **[R4] profile 同步失败持续很久 → Agent 侧 self 信息永久为空** → 降级为空对象而非阻塞；admin 侧可监控 `self_synced_at`。同步 goroutine 带指数退避，max 1h。
- **[R5] admin API 误删 / 错改 token 导致整号失效** → 所有写接口审计日志（request body + before/after 差异 + 操作者 token 前缀）；删改走"软删 enabled=0"默认，彻底删除另起接口 `DELETE /{id}?hard=1`。
- **[R6] 未注册 guid 收到的回调被静默丢弃** → 记录 `warn` + 计数 metric；admin 端 `GET /api/qiwei/_admin/unknown_guids` 查看最近若干条未知 guid（仅保留内存 ring buffer，方便排障），避免"回调进来但行为消失"的哑故障。
- **[R7] registry reload 竞态** → `sync.RWMutex`；reload 先构建新 registry，再原子替换指针。in-flight callback handler 通过局部变量持有旧 runtime 直到完成。
- **[R8] admin API 未受权访问** → 强制 `admin_token` 非空才启用 admin 路由；`admin_token` 空 → admin 端点返回 404（不是 401，不暴露其存在）。

## Migration Plan

1. **代码**：在 `feat/qiwei-multi-account` 分支开发；合并前在本地跑 `go test ./...`、以及手动模拟两个账号的集成测试。
2. **数据库**：随进程首次启动自动建表。`qiwei.db` 放 `{data_dir}/qiwei.db`，部署脚本保证 `data_dir` 可写。
3. **配置**：老 `config.yaml` 不删字段，保留顶层 `guid/token/agent_id`；新增 `db_path` / `admin_token` / `profile_sync_interval` 有默认值。
4. **部署顺序**：agent 侧无需改动，先发 channel-qiwei。首启完成 seed 后，登录企微控制台确认消息通路仍正常（会产出新 conversationId，所以会新建 session）。
5. **Rollback**：若发现异常，回滚到上一个 release；`qiwei.db` 文件可删可留，再起老版本时被忽略。
6. **扩容**：后续要加第二个企微号，运营通过 admin API 新增，无需重启。

## Open Questions

- `admin_token` 是否考虑做成 per-user 审计？v1 决定先单 token；后续若接 Admin UI 再引入账户体系。
- 企微号接入失败（token 过期、guid 被 ban）是否需要自动禁用？v1 决定只做 warn + self_synced_at 观测；自动禁用逻辑放下一迭代，避免误伤。
- `channelIdentity.displayName` 与 `self.name` 同时出现时 Agent 该展示哪个？契约上两者都给，由 Agent 系统提示词决定；不在本层做取舍。
