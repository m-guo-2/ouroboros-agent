## Context

channel-qiwei 作为 QiWe 平台与 agent 之间的桥接层，负责接收平台回调、规范化消息、转发给 agent。当前实现已覆盖主要消息类型和基础群事件，但存在以下信息断层：

1. **agent 侧 `IncomingMessage` 缺少 `ConversationType` 字段** — channel 已发送 `conversationType`，但 agent 结构体未定义该字段，JSON 反序列化时直接丢弃。
2. **文本消息的 `atList` 未提取** — 平台回调 `msgData` 中包含 `atList`（被 @ 的用户列表），但 `events.go` 文本路径从未读取该字段。
3. **群事件 `changedMemberList` 未解码** — 回调携带了 base64 编码的受影响成员 ID 列表，`handleGroupEvent` 没有读取它；`reportGroupEvent` 也没有使用 `GroupEvent.Payload`（agent 端已定义但从未被填充）。
4. **好友申请仅日志** — `handleSystemEvent` 在 `case 2357` 仅 `logger.Business`，不转发也不自动处理。
5. **无群详情/联系人详情 facade** — agent 工具只能通过 `search_targets` 搜索，无法获取群成员列表、群公告、联系人详细资料。

## Goals / Non-Goals

**Goals:**

- agent 每条消息都携带 `conversationType`，明确区分 `p2p` / `group`
- agent 在群聊场景知道自己是否被 @（`mentionedSelf`），以及完整 @列表
- 群生命周期事件携带受影响成员身份（`memberIds`）和操作者（`operatorId`）
- 好友申请自动通过（channel 硬编码），通过后推送 `new_contact` 事件给 agent
- agent 可通过工具主动查询群详情（含成员列表、公告）和联系人详情

**Non-Goals:**

- 不新增群管理操作工具（改名/踢人/设管理员）— 留到后续迭代
- 不处理 1006/1022/1043 群事件 — 不推送也不缓存，后续按需加
- 不做联系人/群详情的本地缓存层 — 每次查询直接调平台 API
- 不修改 agent 的响应策略（如何决定是否回复 @ 消息）— 属于 prompt/策略层
- 不做 @mention 发送（出站消息 @ 某人）— 留到后续

## Decisions

### D1: `conversationType` 透传 — agent 结构体加字段

**方案**: 在 `agent/internal/dispatcher/dispatcher.go` 的 `IncomingMessage` 增加 `ConversationType string` 字段。

**理由**: channel 已经在发送该字段（`incomingMessage.ConversationType`），agent 端只需加字段就能自动反序列化。零侵入、零风险。

**替代方案**: 从 `channelMeta` 传递 → 不如直接加字段语义清晰。

### D2: `atList` / `mentionedSelf` 放入 `channelMeta`

**方案**: 在 `events.go` 文本消息路径中，从 `msg.MsgData["atList"]` 提取 @列表，写入 `channelMeta["atList"]`。同时比对列表中是否包含自身 userId，写入 `channelMeta["mentionedSelf"]`。

**理由**:
- `channelMeta` 是 `map[string]any`，已用于 `quotedMessage` / `miniappData` 等扩展信息，模式一致。
- 不需要改 agent 结构体，只需 agent 从 `channelMeta` 中读取。
- `mentionedSelf` 由 channel 计算，因为 channel 知道自身 userId（`cfg.SelfUserID` 或从登录态获取），agent 不知道。

**自身 userId 来源**: `events.go` 中回调消息的 `UserID` 字段（顶层，非 `senderId`）即为当前登录账号的 userId。每条回调都带这个字段。

### D3: 群事件成员身份 — 填充已有的 `GroupEvent.Payload`

**方案**: 在 `handleGroupEvent` 中解码 `msg.MsgData["changedMemberList"]`（base64 → UTF-8 → 分号分隔的 userId 列表），连同 `msg.SenderID` 作为 `operatorId`，一起写入 `reportGroupEvent` 的 payload。

**理由**:
- agent 端 `GroupEvent.Payload` 字段已存在（`map[string]any`），只是从未被填充。
- 不需要改 agent 数据模型，只需 channel 端填充数据。

**数据格式**: `changedMemberList` base64 解码后是分号分隔的 userId 字符串，如 `"1688855989642487;1688857631651804"`。空值或解码失败时 `memberIds` 为空数组。

### D4: 好友申请 — channel 硬编码自动通过 + 推送 `new_contact`

**方案**:
1. `handleSystemEvent` 的 `case 2357` 中，提取 `contactId` 和回调的 `userId`（corpId），调用平台 API `/contact/agreeContact` 自动通过。
2. 通过后，复用 `reportGroupEvent` 的模式，POST 到 agent 的 `/api/channels/group-event` 端点，`eventType = "new_contact"`。

**理由**:
- 好友申请不需要 agent 决策，直接通过是业务需求。
- 复用 group-event 端点而非新增端点，减少 agent 侧改动。只需在 `validGroupEventTypes` 中加入 `"new_contact"`。
- `GroupEvent.Payload` 携带 `contactId`、`contactNickname`、`contactType`。

**替代方案**: 新增 `/api/channels/system-event` 端点 → 过度设计，当前只有一个系统事件需要转发。

### D5: 新增 facade 端点 — `get_group_detail` / `get_contact_detail`

**方案**: 在 `facade_handlers.go` 新增两个 handler，在 `server.go` 注册路由。遵循现有 facade 模式（POST → 校验 → 调平台 API → 加工返回）。

- `POST /api/qiwei/get_group_detail` → 调 `/room/batchGetRoomDetail`
- `POST /api/qiwei/get_contact_detail` → 调 `/contact/batchGetUserinfo`

**返回值精简**: 不 passthrough 平台原始响应，只返回 agent 需要的字段子集：
- 群详情: `roomId`, `roomName`, `roomAnnouncement`, `roomCreateUserId`, `memberCount`, `members[{userId, nickname, isAdmin, joinTime}]`
- 联系人: `userId`, `nickname`, `realName`, `alias`, `corpId`, `gender`, `avatarUrl`

**agent 工具注册**: 遵循 `createWecomHTTPToolExecutor(path)` 模式，注册 `wecom_get_group_detail` 和 `wecom_get_contact_detail`。

### D6: 自身 userId 获取

**方案**: channel-qiwei 启动时调用 `/user/getProfile` 获取自身 userId 并缓存到 `app` 结构体。用于 `mentionedSelf` 判断。

**替代方案**: 从每条回调的顶层 `userId` 字段获取 → 也可行，但不如启动时一次性获取稳定。两种方式可以结合：启动时获取，回调时校验。

## Risks / Trade-offs

- **[好友申请自动通过的风险]** → 所有好友申请无差别通过，可能引入垃圾联系人。**缓解**: 这是当前业务需求；后续可加黑名单/限流机制。
- **[changedMemberList 解码可能失败]** → base64 格式不保证始终一致。**缓解**: 解码失败时 `memberIds` 为空数组，不影响事件本身的投递，仅降级为无成员身份信息。
- **[自身 userId 获取时机]** → 启动时实例可能未登录。**缓解**: 启动获取失败不阻塞，首次收到回调时从顶层 `userId` 字段补充。
- **[facade 直接调平台 API 无缓存]** → 高频查询可能触发平台限流。**缓解**: 当前 agent 工具调用频率很低（仅在 LLM 决定调用时触发），暂不需要缓存；后续可加 TTL cache。
- **[new_contact 复用 group-event 端点]** → 语义上 `new_contact` 不是"群事件"。**缓解**: 端点更准确地理解为"渠道生命周期事件"，避免为一个事件新增端点的复杂度。agent 端 `validGroupEventTypes` map 只是命名问题，不影响逻辑。
