## 1. 存储层

- [x] 1.1 在 `channel-qiwei/go.mod` 新增依赖 `modernc.org/sqlite`（与 agent 同版本）
- [x] 1.2 新建 `channel-qiwei/db.go`：`OpenDB(path string) (*sql.DB, error)`，设置 `WAL / busy_timeout=5000 / MaxOpenConns=1`，执行 schema
- [x] 1.3 在 `db.go` 定义 schema：`qiwei_accounts`（含所有列与 UNIQUE/索引）、`qiwei_known_rooms`；用幂等 `CREATE TABLE IF NOT EXISTS` 数组的写法
- [x] 1.4 新建 `channel-qiwei/account_repo.go`：`ListAccounts / GetAccount / CreateAccount / UpdateAccount / SoftDeleteAccount / HardDeleteAccount / UpdateProfile`，所有 SQL 集中放这里
- [x] 1.5 改造 `channel-qiwei/room_store.go`：去掉文件路径逻辑，构造器改为 `newRoomStore(db *sql.DB, accountID string)`；`Add / Merge` 改为 `INSERT OR IGNORE` + 内存 map 同步
- [x] 1.6 在 `db.go` 或 `account_repo.go` 实现 `seedFromYAML(db, cfg) error`：当 `qiwei_accounts` 为空且 YAML 顶层 `guid/token` 非空时 seed 一个默认账号
- [x] 1.7 实现 `migrateKnownRoomsFile(db, accountID, dataDir) error`：迁移旧 `known_rooms.txt` 到 `qiwei_known_rooms`，归档旧文件为 `.migrated.{ts}`
- [x] 1.8 为 `short_hash` 实现派生函数：`deriveShortHash(guid string) string`（SHA-256 前 8 字节 → base32 小写去填充）

## 2. 账号运行时

- [x] 2.1 新建 `channel-qiwei/account_runtime.go`：定义 `Account` struct（镜像表行）和 `accountRuntime` struct（含 `client / roomStore / nameCache / dedupe / selfUserID / contactsMu / contactsLoadedAt`）
- [x] 2.2 实现 `newAccountRuntime(cfg Config, db *sql.DB, acc Account) *accountRuntime`：基于账号参数构造 `qiweiClient` 和账号级 `roomStore / nameCache / dedupe`
- [x] 2.3 新建 `channel-qiwei/account_registry.go`：`accountRegistry` 结构含 `byID / byGUID / byShortHash` 三个 map 和 `sync.RWMutex`；提供 `Get / Lookup / Snapshot`
- [x] 2.4 实现 `loadRegistry(db) (*accountRegistry, error)`：从 DB 读所有账号，仅把 `enabled=1` 纳入索引，返回新实例
- [x] 2.5 `app` 结构中新增 `db *sql.DB` 和 `registry atomic.Pointer[accountRegistry]`；移除 `app` 上所有 per-account 字段（`client / selfUserID / roomStore / nameCache / dedupe / contactsMu / contactsLoadedAt`）
- [x] 2.6 实现 `(*app).currentRegistry() *accountRegistry` 和 `(*app).reloadRegistry(ctx) error`：reload 用"构建新实例 → 原子替换"
- [x] 2.7 每账号 `dedupe` 独立：改为 `accountRuntime` 字段，不再做全局 `a.dedupe`

## 3. 回调路由改造

- [x] 3.1 `events.go` 的 `handleWebhookCallback` / `handleCallbackMessage`：在外层按 `msg.GUID` 查 registry 得到 `runtime`；未知或禁用 → 写 unknown ring buffer 并丢弃
- [x] 3.2 所有下游 `handle*` 函数签名加一个 `rt *accountRuntime` 参数：`handleNormalMessage / handleAppMessage / handleMixedMessage / handleSystemEvent / handleGroupEvent / autoAcceptFriendRequest` 以及用到 self/dedupe/nameCache/roomStore 的助手
- [x] 3.3 `preloadKnownRooms / loadSelfUserID / loadContactsOnce` 改为 per-account，由外层遍历 registry 调用；`loadSelfUserID` 除了内存 cache 外还要 upsert `qiwei_accounts.self_*` 列
- [x] 3.4 `resolveUserName / resolveUserNameInRoom / resolveGroupName / fetchUserName / fetchMemberNameFromRoom` 全部改为 receiver 换成 `*accountRuntime`（或作为 free function 接 `rt`）
- [x] 3.5 新增复合 conversationId 组装：在上行点把 `replyToID` 或 `senderId` 拼上 `@shortHash`，然后设置到 `incomingMessage.ChannelConversationID`
- [x] 3.6 `incomingMessage.AgentID` 改为取 `rt.account.AgentID`；新增 `ChannelIdentity` 字段填充 `displayName / self{userId,name,alias,corpName}`
- [x] 3.7 `forwardToAgent` 和 `reportGroupEvent` 签名加 `rt`，url/body 中的 agentId 一并切换

## 4. 下行接口改造

- [x] 4.1 抽出 helper `resolveOutgoingTarget(registry, channelConversationId) (rt *accountRuntime, rawTarget string, err error)`：解析末尾 `@shortHash`，包含"缺后缀且单账号走默认 / 多账号报错 / shortHash 未匹配报错"三条分支
- [x] 4.2 `api_handlers.go` 的 `handleSend`：使用 `resolveOutgoingTarget` 拿到 `rt`，后续 `a.client.doAPIRaw` 换成 `rt.client.doAPIRaw`；`toId` 用 `rawTarget`
- [x] 4.3 `handleDoAPI` / `handleModuleAction` / `handleModuleCall` 改造：如果 body 带了 `channelConversationId` 则走 `resolveOutgoingTarget`；否则从 body / query 中取 `account_id` 作为显式指定；仍不能确定时若仅单账号走默认，否则 400
- [x] 4.4 `facade_handlers.go` 所有对 `a.client` 的引用：审计一遍，换成按会话或账号参数拿到的 `rt.client`
- [x] 4.5 `handleSearchTargets / handleListOrGetConversations / handleParseMessage / handleFacadeSendMessage / handleGetGroupDetail / handleGetContactDetail`：新增 `account_id` 参数（优先）或复合 conversationId（其次），单账号时回退默认；更新 `docs/qiwei-platform.md` 与 `api.md`

## 5. 身份快照同步

- [x] 5.1 实现 `syncAccountProfile(ctx, rt, repo) error`：调 `/user/getProfile`，解析返回，UPDATE `qiwei_accounts` 的 self_* 列和 `self_synced_at`
- [x] 5.2 启动时并发同步所有启用账号（复用 `main.go` 启动 goroutine 模式）
- [x] 5.3 新增后台 goroutine `profileSyncLoop(ctx, a)`：每 `cfg.ProfileSyncInterval`（默认 6h）遍历所有账号同步一次
- [x] 5.4 实现每账号的指数退避失败记录：`map[accountID]struct{ fails int; nextAt time.Time }`，`min(base * 2^fails, 1h)`
- [x] 5.5 新增 `handleAdminRefreshAccount`：同步调用 `syncAccountProfile`，返回最新账号 JSON

## 6. Admin API

- [x] 6.1 新建 `channel-qiwei/admin_handlers.go`：实现 `listAccounts / getAccount / createAccount / updateAccount / deleteAccount / hardDeleteAccount / refreshAccount / reloadRegistry / listUnknownGuids`
- [x] 6.2 新建中间件 `adminAuthMiddleware`：读 `X-Admin-Token`，空 token 的部署路径下直接返回 404（对整个 `/api/qiwei/_admin/*` 路径）
- [x] 6.3 在 `server.go` 的 `routes()` 中挂载 admin 子路由；空 token 时由中间件返回 404，保证 `/api/qiwei/` 兜底不吞入口
- [x] 6.4 响应对象统一 hide `token` 字段，改为 `token_preview`（前 4 位 + `***`）
- [x] 6.5 新增 `unknownGuidBuffer`：`sync.Mutex + ring buffer cap=200`，提供 `Record(event)` 与 `Snapshot() []event`；在 3.1 的路由失败处调用

## 7. 配置与启动

- [x] 7.1 `config.go` 新增字段：`DBPath string`、`AdminToken string`、`ProfileSyncInterval time.Duration`；更新 `configDefaults` 提供默认值
- [x] 7.2 更新 `.env.example` 与 README 的"环境变量/配置"段落；新增 `admin_token / db_path / profile_sync_interval` 条目
- [x] 7.3 `main.go`：在 `newApp` 之前 `OpenDB`、跑 seed、迁移 legacy 文件；之后 `loadRegistry` 并启动 profile sync goroutine
- [x] 7.4 `app` 在 shutdown 时关闭 `*sql.DB`
- [x] 7.5 首启日志明确打印："已加载账号数量"、"首启 seed 条数"、"legacy known_rooms 迁移条数"

## 8. 契约与文档

- [x] 8.1 `models.go` 中 `incomingMessage`、`outgoingMessage`、`apiResponse` 增补字段：`ChannelIdentity`；确保 JSON tag 名与 spec 一致
- [x] 8.2 在 `channel-qiwei/api.md` 新增一节描述 `channelConversationId` 新形态 + `channelIdentity` 字段
- [x] 8.3 在 `channel-qiwei/README.md` 新增"多账号"段落：管理接口示例、seed 行为、admin_token 配置
- [ ] 8.4 在 `docs/qiwei-platform.md` 上补多账号部分（若该文档涉及 Channel 对外契约）  <!-- 仓库当前没有该文件，跳过 -->

## 9. 测试

- [x] 9.1 `account_repo_test.go`：CRUD、short_hash 冲突、软删/硬删路径
- [x] 9.2 `account_registry_test.go`：build / reload / Lookup / 原子替换（并发场景）
- [x] 9.3 `conversation_id_test.go`：组合与反向解析的往返；缺后缀时的单/多账号分支；非法后缀分支
- [x] 9.4 `admin_handlers_test.go`：鉴权路径（token 空/错/对）、CRUD happy path、guid 冲突、软删后注册表反映
- [x] 9.5 `dedupe_isolation_test.go`（替代原计划的 events 扩展）：两账号 `msgSvrId` 独立去重；资源隔离检查
- [ ] 9.6 `profile_sync_test.go`：mock qiweiClient，成功/失败/退避  <!-- 留待后续迭代：qiweiClient 目前未抽象为 interface -->
- [ ] 9.7 `main_integration_test.go`（可选）：端到端跑 seed + 一个模拟回调 + 一次下行 send，全部穿过 DB 与 registry  <!-- 标记为可选，后续迭代 -->

## 10. 收尾

- [x] 10.1 `go build ./...` 通过；`go test ./...` 除 2 个预先存在的 fixture 格式失败外其余全绿
- [ ] 10.2 运行 `openspec validate qiwei-multi-account --strict`
- [ ] 10.3 自测：单账号兼容回归（老 YAML seed）、多账号并发回调、admin CRUD 全流程、profile 同步失败退避
- [ ] 10.4 PR 描述引用 proposal/design，列出 BREAKING 注意事项与回滚方案
