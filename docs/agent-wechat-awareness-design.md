# Agent 微信感知能力设计

> 目标：让 agent 像人一样使用微信 —— 知道谁在说话、在哪个群、谁加入/离开了、被 @ 了、有好友申请，并能主动查询群信息、联系人详情，做出真正有上下文的回应。

---

## 1. 现状与差距

### 1.1 当前 agent 能感知到什么

| 维度 | 现状 | 来源 |
|------|------|------|
| 消息内容 | 文本/图片/文件/语音转写/视频/链接/位置/名片/小程序/混合消息 | channel→agent incoming |
| 发送者 | `senderName` + `channelUserId` | incoming |
| 会话 ID | `channelConversationId`（私聊=userId，群聊=roomId） | incoming |
| 会话名称 | `channelConversationName`（私聊=昵称，群聊=群名） | incoming |
| 引用消息 | `channelMeta.quotedMessage`（被引用消息内容+发送者） | incoming |
| 附件 | OSS 地址 + 元数据 | incoming attachments |
| 群事件 | group_name_changed / member_joined / member_removed / member_quit / group_dissolved / group_joined | group-event |
| 主动查询 | 搜索联系人/群、会话列表/历史、解析消息、发送消息、撤回消息 | agent tools |

### 1.2 差距：平台有但 agent 看不到的信息

| 编号 | 缺失信息 | 平台来源 | 影响 |
|------|----------|---------|------|
| G1 | **会话类型**（p2p vs group） | 回调 `fromRoomId` 推导，channel 发送了 `conversationType` 但 agent 端结构体未接收 | agent 无法区分私聊和群聊，影响行为策略 |
| G2 | **@提及信息** | 回调 `msgData.atList[]`（userId + nickname） | agent 不知道自己是否被 @ 或谁被 @，群聊响应策略受限 |
| G3 | **群事件涉及的成员身份** | 回调 `msgData.changedMemberList`（base64 编码的 userId 列表） | agent 只知道"有人加入/离开"，不知道是谁 |
| G4 | **群创建事件** | 回调 `msgType=1006` | agent 看不到新群创建 |
| G5 | **群主转让事件** | 回调 `msgType=1022` | agent 不知道群主变了 |
| G6 | **群管理员变动事件** | 回调 `msgType=1043` | agent 不知道管理员变了 |
| G7 | **好友申请事件** | 回调 `msgType=2357`，含 contactNickname/contactId/contactType | agent 无法自动处理好友申请 |
| G8 | **联系人变动事件** | 回调 `msgType=2131/2188` | agent 不知道联系人信息更新了 |
| G9 | **群详情查询**（公告、成员列表、管理员、创建者） | API `/room/batchGetRoomDetail` | agent 无法主动了解群的完整信息 |
| G10 | **联系人详情查询**（昵称、真实姓名、企业、职位、手机号） | API `/contact/batchGetUserinfo` | agent 无法了解发送者的详细信息 |
| G11 | **群成员增量变动查询** | API `/room/getRoomIncrSync` | agent 无法跟踪大群的成员变化 |
| G12 | **群管理操作** | API 群改名/群公告/邀请/移除/管理员等 | agent 无法执行群管理操作 |
| G13 | **好友申请处理** | API `/contact/agreeContact` | agent 无法同意好友申请 |
| G14 | **联系人详细信息**（含标签、来源、好友状态） | API `/contact/getWxContactList` | agent 无法深度了解客户画像 |

---

## 2. 能力设计

按"感知 → 理解 → 行动"三层组织。

### 2.1 感知层：被动接收的信息增强

#### 2.1.1 消息入站增强

**目标**：agent 收到每条消息时，能区分场景并知道完整上下文。

| 改造项 | 字段 | 改动位置 | 说明 |
|--------|------|---------|------|
| 传递会话类型 | `conversationType: "p2p" \| "group"` | agent `IncomingMessage` 结构体增加该字段 | channel 已发送，agent 端需接收 |
| 传递 @提及列表 | `channelMeta.atList: [{userId, nickname}]` | channel events.go 提取 `msgData.atList`，写入 channelMeta | agent 可判断是否被 @ 以决定是否响应 |
| 标记是否 @自己 | `channelMeta.mentionedSelf: bool` | channel 比对 atList 中是否包含自身 userId | 群聊响应的核心判据 |

**agent 端视角**（改造后的 incoming message 新增字段）：

```json
{
  "conversationType": "group",
  "channelMeta": {
    "atList": [
      {"userId": "168885...", "nickname": "Bot"}
    ],
    "mentionedSelf": true,
    "quotedMessage": { ... }
  }
}
```

#### 2.1.2 群事件增强

**目标**：每个群生命周期事件都携带"是谁""影响了谁"的信息。

| 改造项 | 改动 | 说明 |
|--------|------|------|
| 群事件携带成员 ID | `GroupEvent.payload.memberIds: []string` | 从 `changedMemberList` base64 解码提取 userId 列表 |
| 群事件携带操作者 | `GroupEvent.payload.operatorId: string` | 从回调 `senderId` 提取 |
| 新增 group_created | eventType `group_created`（msgType=1006） | 首次被拉入新群时触发 |
| 新增 owner_transferred | eventType `owner_transferred`（msgType=1022） | 群主转让 |
| 新增 admin_changed | eventType `admin_changed`（msgType=1043） | 管理员变动 |

**agent 端视角**（改造后的 group event）：

```json
{
  "channel": "qiwei",
  "agentId": "...",
  "channelGroupId": "10791082...",
  "eventType": "member_joined",
  "groupName": "测试群",
  "payload": {
    "memberIds": ["168885598964248"],
    "operatorId": "168885763165180"
  }
}
```

#### 2.1.3 新增系统事件 → agent

| 事件 | 平台 msgType | agent eventType | payload | 说明 |
|------|-------------|----------------|---------|------|
| 好友申请 | 2357 | `friend_request` | `{contactId, contactNickname, contactType, applyTime}` | agent 可决定是否自动通过 |
| 联系人变动 | 2131/2188 | `contact_changed` | `{contactType: "external"\|"internal"}` | agent 可刷新联系人缓存 |

---

### 2.2 理解层：主动查询的工具增强

agent 需要能主动"看一眼"群信息、联系人信息，就像人打开微信看群详情页一样。

#### 2.2.1 新增 agent 工具

| 工具名 | 能力 | 调用链 | 核心参数 | 返回 |
|--------|------|--------|---------|------|
| `wecom_get_group_detail` | 查看群详情（群名、公告、成员列表+角色、创建时间、群主） | agent → channel `/api/qiwei/group/batch-detail` | `roomIds: []string` | 群名、公告、成员列表（userId+昵称+角色+入群时间） |
| `wecom_get_contact_detail` | 查看联系人详情（昵称、真实姓名、企业、头像、性别） | agent → channel `/api/qiwei/contact/batch-detail` | `userIds: []string` | 昵称、别名、真实姓名、企业ID、性别、手机号、头像 |
| `wecom_accept_friend` | 同意好友申请 | agent → channel `/api/qiwei/contact/approve-request` | `userId, corpId` | 成功/失败 |
| `wecom_manage_group` | 群管理操作（改名、改公告、加人、踢人、设管理员） | agent → channel `/api/qiwei/group/{action}` | action + 对应参数 | 操作结果 |

#### 2.2.2 现有工具增强

| 工具 | 改造 | 说明 |
|------|------|------|
| `wecom_list_or_get_conversations` | 返回值增加 `sessionType`（0=好友, 1=群聊, 3=系统） | 让 agent 能区分会话类型 |
| `wecom_search_targets` | 返回值增加更多联系人信息（企业名、职位等） | 搜索结果更有用 |

---

### 2.3 行动层：出站能力增强

当前 agent 已有 `wecom_send_message`（文本/图片/文件/视频/链接/小程序/位置）和 `wecom_revoke_message`。增量：

| 新能力 | 说明 | 优先级 |
|--------|------|--------|
| 发送 @提及消息 | 通过 `wecom_send_message` 支持 mention 参数，生成混合文本 | P1 |
| 好友申请处理 | 通过 `wecom_accept_friend` 同意/拒绝 | P1 |
| 群管理操作 | 通过 `wecom_manage_group` 执行 | P2 |

---

## 3. 改造清单

### 3.1 channel-qiwei 侧

| # | 改造 | 文件 | 复杂度 |
|---|------|------|--------|
| C1 | 文本消息提取 `atList` 写入 channelMeta，计算 `mentionedSelf` | `events.go` | 低 |
| C2 | 群事件解码 `changedMemberList`（base64 → userId 列表），填充 payload | `events.go` | 中 |
| C3 | 群事件增加 1006/1022/1043 的映射和转发 | `events.go` | 低 |
| C4 | 好友申请事件(2357) 转发给 agent（新事件类型 or 新端点） | `events.go` | 中 |
| C5 | 联系人变动事件(2131/2188) 转发给 agent | `events.go` | 低 |
| C6 | facade_handlers 新增 `get_group_detail` 处理（调用 `/room/batchGetRoomDetail` 后加工返回） | `facade_handlers.go` | 中 |
| C7 | facade_handlers 新增 `get_contact_detail` 处理（调用 `/contact/batchGetUserinfo`） | `facade_handlers.go` | 中 |
| C8 | facade_handlers 新增 `accept_friend` 处理（调用 `/contact/agreeContact`） | `facade_handlers.go` | 低 |
| C9 | facade_handlers 新增 `manage_group` 处理（路由到群管理各操作） | `facade_handlers.go` | 中 |

### 3.2 agent 侧

| # | 改造 | 文件 | 复杂度 |
|---|------|------|--------|
| A1 | `IncomingMessage` 增加 `conversationType` 字段并使用 | `dispatcher/dispatcher.go` | 低 |
| A2 | dispatcher 解析并传递 `channelMeta.atList` + `mentionedSelf` | `dispatcher/dispatcher.go` | 低 |
| A3 | 群事件 `GroupEvent` 增加 `Payload` 实际使用（memberIds, operatorId） | `dispatcher/group_event.go` | 低 |
| A4 | 群事件处理增加 group_created/owner_transferred/admin_changed | `dispatcher/group_event.go` | 低 |
| A5 | 新增 `wecom_get_group_detail` 工具注册 | `runner/wecom_builtin_tools.go` | 中 |
| A6 | 新增 `wecom_get_contact_detail` 工具注册 | `runner/wecom_builtin_tools.go` | 中 |
| A7 | 新增 `wecom_accept_friend` 工具注册 | `runner/wecom_builtin_tools.go` | 低 |
| A8 | 新增 `wecom_manage_group` 工具注册 | `runner/wecom_builtin_tools.go` | 中 |
| A9 | 好友申请事件接收端点或复用 group-event 端点 | `dispatcher/` | 中 |
| A10 | system prompt 增加微信感知上下文描述（agent 要知道自己有这些能力） | `runner/` 或 prompt 模板 | 低 |

---

## 4. 信息流全景图

```
QiWe 平台回调
    │
    ├─ cmd=15000 普通消息
    │   ├─ msgType 0/2: 文本 ──→ 提取 atList/mentionedSelf ──→ agent incoming
    │   ├─ msgType 3/7/14/101: 图片 ──→ 媒体管道 ──→ agent incoming
    │   ├─ msgType 49.57: 引用 ──→ 提取 quotedMessage ──→ agent incoming
    │   ├─ msgType 1001: 群名变更 ──→ 查新群名 ──→ agent group-event ✓
    │   ├─ msgType 1002: 成员加入 ──→ [新] 解码成员列表 ──→ agent group-event
    │   ├─ msgType 1003: 成员移除 ──→ [新] 解码成员列表 ──→ agent group-event
    │   ├─ msgType 1005: 成员退出 ──→ agent group-event ✓
    │   ├─ msgType 1006: 群创建 ──→ [新] agent group-event
    │   ├─ msgType 1022: 群主转让 ──→ [新] agent group-event
    │   ├─ msgType 1023: 群解散 ──→ agent group-event ✓
    │   └─ msgType 1043: 管理员变动 ──→ [新] agent group-event
    │
    ├─ cmd=15500 系统事件
    │   ├─ 群事件 (同上) ──→ agent group-event
    │   ├─ msgType 2357: 好友申请 ──→ [新] agent system-event
    │   └─ msgType 2131/2188: 联系人变动 ──→ [新] agent system-event
    │
    └─ cmd=11016/20000: 账号/异步 ──→ 仅日志（不变）

Agent 主动查询
    ├─ wecom_search_targets ──→ 搜索联系人/群 ✓
    ├─ wecom_list_or_get_conversations ──→ 会话+历史 ✓（增强返回值）
    ├─ wecom_send_message ──→ 发消息 ✓（增加 @mention 支持）
    ├─ wecom_revoke_message ──→ 撤回 ✓
    ├─ wecom_get_group_detail ──→ [新] 群详情
    ├─ wecom_get_contact_detail ──→ [新] 联系人详情
    ├─ wecom_accept_friend ──→ [新] 同意好友
    └─ wecom_manage_group ──→ [新] 群管理
```

---

## 5. 分期计划

### Phase 1：核心信息补齐（让 agent "看得清"）

> 预期效果：agent 能区分私聊/群聊、知道自己是否被 @、知道谁加入/离开了群

| 项目 | 改造项 | 涉及编号 |
|------|--------|---------|
| conversationType 透传 | A1 | G1 |
| @mention 信息透传 | C1 + A2 | G2 |
| 群事件成员身份 | C2 + A3 | G3 |
| 好友申请转发 | C4 + A9 | G7 |

### Phase 2：主动查询能力（让 agent "查得到"）

> 预期效果：agent 能主动查看群详情、联系人信息、同意好友申请

| 项目 | 改造项 | 涉及编号 |
|------|--------|---------|
| 群详情查询工具 | C6 + A5 | G9 |
| 联系人详情查询工具 | C7 + A6 | G10 |
| 好友申请处理工具 | C8 + A7 | G13 |
| @mention 发送支持 | 出站消息增强 | — |

### Phase 3：完整群生命周期（让 agent "全知全觉"）

> 预期效果：agent 感知群的全部生命周期变化，并能执行群管理操作

| 项目 | 改造项 | 涉及编号 |
|------|--------|---------|
| 新群事件类型 | C3 + A4 | G4/G5/G6 |
| 联系人变动转发 | C5 | G8 |
| 群管理操作工具 | C9 + A8 | G12 |

---

## 6. 平台 API 参考（agent 工具需要调用的核心接口）

### 群详情-批量 `/room/batchGetRoomDetail`

```
请求: { guid, roomIdList: ["10723559966834914"] }
响应: {
  roomList: [{
    roomId, roomName, roomAnnouncement,
    roomCreateTime, roomCreateUserId,
    roomEnableInviteConfirm, roomIsForbidChangeName,
    memberList: [{
      userId, name(本群昵称), isAdmin, joinTime,
      inviterId, roomRemarkName
    }]
  }]
}
```

### 联系人详情-批量 `/contact/batchGetUserinfo`

```
请求: { guid, userIdList: ["1688854961262919"] }
响应: {
  contactList: [{
    userId, nickname, realName, alias,
    corpId, gender, mobile, avatarUrl,
    internationCode, groupId, acctid
  }]
}
```

### 群成员增量变动 `/room/getRoomIncrSync`

```
请求: { guid, roomId, ver(首次0), nowMemberCount }
响应: {
  roomList: [{
    roomId, roomName, roomAnnouncement,
    ver(下次查询用),
    memberList: [{
      userId, name, isAdmin, joinTime,
      memberFlag(0=正常, 1=移除),
      isCreator, joinScene
    }]
  }]
}
```

### 同意好友申请 `/contact/agreeContact`

```
请求: { guid, userId, corpId }
响应: { code: 0 }
```

### 回调 atList 结构（文本消息 msgData）

```json
{
  "atList": [
    { "userId": "788...", "nickname": "张三" },
    { "userId": "168...", "nickname": "Bot" }
  ],
  "content": "@张三 @Bot 你好"
}
```

### 回调群事件 changedMemberList（base64）

```json
{
  "msgType": 1002,
  "fromRoomId": 239655862281126,
  "msgData": {
    "changedMemberList": "MTY4ODg1NTk4OTY0MjQ4Nw=="
  },
  "senderId": 168885763165180
}
```

解码后 `changedMemberList` 为分号分隔的 userId 列表，如 `"1688855989642487;1688857631651804"`。

### 回调好友申请（msgType=2357）

```json
{
  "cmd": 15500,
  "msgType": 2357,
  "msgData": {
    "applyTime": 1759063191,
    "contactId": 78813****061361,
    "contactNickname": "nihao～",
    "contactType": "微信",
    "userId": 197032****006843
  }
}
```

---

## 7. 设计原则

1. **信息尽量透传，不丢弃**：平台回调的有价值字段都应传递到 agent，即使 agent 当前不处理也为未来预留。
2. **agent 工具命名统一**：`wecom_` 前缀 + 动词_名词，与现有 `wecom_search_targets` / `wecom_send_message` 风格一致。
3. **facade 模式不变**：新工具仍走 channel-qiwei facade → 平台 API 的架构，agent 不直接调平台。
4. **最小字段集返回**：agent 工具返回值只包含 agent 可能用到的字段，不 passthrough 原始平台响应的全量字段。
5. **事件 vs 工具**：被动感知走事件推送，主动查询走工具调用，两条路径职责分明。
