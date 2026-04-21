## 1. Schema 与 Repo

- [x] 1.1 在 `channel-qiwei/db.go` 追加 `qiwei_contacts / qiwei_rooms / qiwei_room_members / qiwei_identity_links` 四张表及索引，`account_id` 外键 CASCADE；所有 `CREATE TABLE IF NOT EXISTS` 保持幂等
- [x] 1.2 新增 `channel-qiwei/contact_repo.go`：封装 `UpsertContact / UpsertRoom / ReplaceRoomMembers / GetContact / GetRoom / SearchContacts / CountByAccount / UpsertIdentityLink / ResolveIdentityLink` 等方法，`raw_json` 写入时截断 > 64KB 并打 warn
- [x] 1.3 为 `contact_repo` 写单元测试：`upsertContact` 的 insert/update 分支、`identity_links` 的三种覆盖路径（新建/幂等/覆盖）、`raw_json` 截断 guard

## 2. ContactSource 抽象与 QiWe 实现

- [x] 2.1 在 `channel-qiwei/contact_source.go` 定义 `ContactSource` interface 与 `ContactSnapshot / RoomSnapshot / RoomMemberSnapshot` 结构体，字段命名对齐企微官方术语
- [x] 2.2 实现 `qiweProtoSource`：包装既有 `qiweiClient`，把 QiWe 协议字段（如 `openid`、`nickname`、`avatarUrl`）按固定映射翻译成 `ContactSnapshot`
- [x] 2.3 为 `qiweProtoSource` 写测试：mock `doAPIRaw` 返回样例 payload，断言映射结果（特别是 `openid → ExternalUserID`、`Raw` 保留原字段）

## 3. ContactGateway 与 Runtime 集成

- [x] 3.1 新增 `channel-qiwei/contact_gateway.go`：实现 `ResolveName / ResolveContact / ResolveRoom / ResolveExternalUserID`，内部顺序为 ttlCache → repo → src，每级命中后回写上层
- [x] 3.2 在 `accountRuntime` 上挂一个 `*ContactGateway`，`newAccountRuntime` 构造时注入
- [x] 3.3 改写 `events.go` 里 `resolveUserName / resolveUserNameInRoom / resolveGroupName / fetchUserName / loadContactsOnce` 走 `rt.gateway`，保留旧函数签名减少改动面
- [ ] 3.4 人工回归：本地起服务，给一条已知联系人发消息，确认 `qiwei_contacts` 被写入且 senderName 解析正确

## 4. 同步器

- [x] 4.1 新增 `channel-qiwei/contact_sync.go`：实现 `contactSyncer`，结构参考 `profileSyncer`（per-account backoff + ring buffer recentRuns）
- [x] 4.2 实现事件驱动 worker：per-account channel（容量 64）、满则丢弃最旧 + warn + 计数指标
- [x] 4.3 实现 `runFull(rt)`：`ListExternalContacts + ListInternalContacts + ListRooms`，对 room 变化集合调 `BatchGetRoomDetail`，写入 repo
- [x] 4.4 在 `main.go` 启动序列加 `contactSyncer.Start(ctx)`，读取 `contact_sync_interval / contact_sync_enabled`
- [x] 4.5 在 `events.go` 的 `autoAcceptFriendRequest / handleGroupEvent / handleNormalMessage`（未知 senderId 分支）插入 `syncer.Enqueue(...)`

## 5. 配置与启动

- [x] 5.1 在 `config.go` 增加 `ContactSyncInterval / ContactSyncEnabled / GatewayToken` 字段，含默认值与 `flatten()` 兼容
- [x] 5.2 更新 `.env.example` 与 `README.md` 的配置项清单
- [x] 5.3 `LoadConfig` 的 `Validate` 不强制 `gateway_token`，为空时后续路由注册会自行跳过

## 6. 下游 HTTP API

- [x] 6.1 新增 `channel-qiwei/gateway_handlers.go`：实现 `/contacts/resolve / /contacts/search / /rooms/{roomId} / /rooms/{roomId}/members / /identity-links (GET+POST)`
- [x] 6.2 在 `server.go` 的 `routes()` 里按 `gateway_token` 非空条件注册 `/api/qiwei/gateway/` 前缀路由；注意注册顺序在 `/api/qiwei/` 通配之前
- [x] 6.3 实现 `X-Gateway-Token` 校验中间件，对 token 空/错返回 401；路由未注册时自然 404
- [x] 6.4 实现 `refresh=1` 限流：per-`(account_id, user_id)` 30s 令牌桶（内存 map + TTL 即可），超限 429
- [x] 6.5 为每个 handler 写一个集成测试，覆盖成功 / 未命中 404 / 缺 token 401 / identity-links 幂等+覆盖两条路径

## 7. Admin 扩展

- [x] 7.1 在 `admin_handlers.go` 的 `GET /accounts/{id}` 返回值追加 `contact_count / room_count / room_member_count / contact_sync_last_at / contact_sync_last_error / identity_link_count`，数据源走 repo 与 syncer
- [x] 7.2 新增 `POST /accounts/{id}/resync`：调用 `syncer.Enqueue` 返回 202
- [x] 7.3 新增 `GET /accounts/{id}/sync_status`：返回 syncer ring buffer

## 8. 迁移、测试与灰度

- [ ] 8.1 本地起空库 + 老库两种场景，确认启动迁移幂等、旧行为不破坏
- [ ] 8.2 跑 `go test ./channel-qiwei/...`，补充必要的 race test (`-race`)
- [ ] 8.3 灰度计划：在单个非生产企微号上跑 12 小时，观察两轮周期同步的 `contact_sync_last_error` 均为空、`contact_count` 符合预期
- [ ] 8.4 与小程序后端联调 `POST /identity-links`，确认 `UNIQUE(downstream_system, downstream_id)` + 覆盖 audit 行为
- [x] 8.5 更新 `channel-qiwei/README.md`：新增 gateway 端点清单、`gateway_token` 说明、与官方 API adapter 的接入点
- [x] 8.6 准备回滚脚本/文档：四张新表的 DROP 语句，以便需要时清空重来

## 9. 未来接入企微官方 API 的预留点（本次不实现，仅落文档）

- [x] 9.1 在 `contact_source.go` 文件头注释里列出预计新增的 `officialAPISource` 需要覆盖的方法与切换策略（`GetOpenID` 优先走官方）
- [x] 9.2 在 `README.md` 里写清官方 API adapter 接入时需要修改的文件清单（不改表，不改 gateway HTTP 契约）
