## Context

channel-qiwei 是 agent 与企微/微信的协议适配器。当前架构：

- **接收链路**: QiWei 平台 → HTTP callback → `events.go` 按 msgType 映射 → 媒体下载/转写 → 构造 `incomingMessage` → POST 到 agent `/api/channels/incoming`
- **发送链路**: agent `send_channel_message` / `wecom_send_message` → POST 到 adapter `/api/qiwei/send` 或 `/api/qiwei/send_message` → `toQiweiMessageRequest` 映射为 QiWei API 调用
- **回调过滤**: 当前只处理 `cmd=15000`（普通消息），系统消息 (`cmd=15500`) 和状态消息 (`cmd=11016`) 被完全忽略

关键约束：
- adapter 层负责所有协议脏活，agent 只看到语义清晰的统一类型
- 所有非文本消息最终都必须有人类可读的 `content` 文本，确保 LLM 即使不处理附件也能理解上下文
- QiWei 平台的字段值很多是 Base64 编码的（title/desc/nickname/address 等），adapter 必须解码
- 企微(QW)和个微(GW)同类消息的 msgType 编码不同，msgData 结构也不同，adapter 必须统一

## Goals / Non-Goals

**Goals:**

- agent 能"看见"所有常见人类消息类型，不再有感知盲区
- agent 能发送链接卡片、定位、小程序消息，能引用回复和撤回消息
- 新成员入群事件能通知到 agent
- 好友申请能被记录（为后续自动处理打基础）
- 所有新消息类型对 agent 侧向后兼容，不破坏现有流程

**Non-Goals:**

- 发送 GIF 表情（流程复杂，需先上传 CDN 或从收藏获取，暂不做）
- 发送视频（需先上传到 QiWei CDN 获取 fileId 等参数，流程重，暂不做）
- 自动处理好友申请（仅记录日志，不推送 agent）
- 群管理操作（踢人/改名/转让群主等，不在本次范围）
- 消息已读/未读通知处理（2001/2005）

## Decisions

### D1: 接收侧收敛策略——"一切皆文本"

所有新增消息类型的核心处理逻辑是：**从 msgData 提取关键信息 → Base64 解码 → 拼成人类可读的 content 字符串**。

不同类型的具体策略：

| 类型 | 策略 | 是否有附件 |
|------|------|-----------|
| 表情 (29/104) | 和图片统一，下载 → OSS → image 附件 | 是 |
| 链接 (13) | 解码 title/desc/linkUrl → 拼文本 | 否 |
| 位置 (6) | 解码 address + title + 经纬度 → 拼文本 | 否 |
| 名片 (41) | 解码 nickname/corpName + shared_id → 拼文本 | 否 |
| 红包 (26) | 解码 wishingContent → 拼文本通知 | 否 |
| 小程序 (78) | 解码 title/desc → 拼文本 + 原始 msgData 保存到 channelMeta | 否 |
| 视频号 (141) | 解码 channelName + channelUrl → 拼文本 | 否 |
| 图文混合 (123) | 遍历子消息：文字拼 content，图片走附件流程 | 是(图片子消息) |

**为什么不给每种类型各建一套独立的处理流程？** 因为 agent 是 LLM，它理解自然语言文本的能力远强于理解结构化字段。统一输出为"人类可读文本 + 可选附件"是最简洁且对 LLM 最友好的方式。

### D2: 新增 contentFromMsgData 函数族

在 `events.go` 中新增一组函数，每种非媒体消息类型一个函数：

```
contentFromLink(msgData) → string
contentFromLocation(msgData) → string
contentFromCard(msgData) → string
contentFromRedPacket(msgData) → string
contentFromMiniapp(msgData) → string
contentFromChannelMsg(msgData) → string
contentFromMixed(msgData) → (string, []incomingAttachment)
```

这些函数统一签名，内部处理 Base64 解码和字段提取。

**为什么不复用 media_pipeline.go 的 prepareMediaForAgent？** 因为这些类型不是"媒体"，不需要下载/OSS/转写流程。它们只需要字段提取和文本拼接，逻辑完全不同。混入 media_pipeline 会破坏其单一职责。

### D3: 回调路由——按 cmd 分流

当前 `handleCallbackMessage` 假定所有消息都是 `cmd=15000`。改为先按 cmd 分流：

- `cmd=15000`：走现有的普通消息处理逻辑（扩展 msgType 覆盖）
- `cmd=15500`：新增 `handleSystemEvent` 函数，按 msgType 分流
  - `1002`（新成员入群）→ 构造 system 类型消息推送 agent
  - `2357/2132`（好友申请）→ 记录日志
  - 其他 → 仅日志
- `cmd=11016`/`cmd=20000`：仅日志（已有行为不变）

**为什么 qiweiCallbackMessage 需要新增 cmd 字段？** 因为当前 `decodeOneMessage` 不解析 cmd，导致系统消息和普通消息无法区分。需要在 `qiweiCallbackMessage` 结构体和 `decodeOneMessage` 中补上 cmd 字段。

### D4: 发送侧——扩展 toQiweiMessageRequest

在 `api_handlers.go` 的 `toQiweiMessageRequest` 和 `facade_handlers.go` 的 `toFacadeQiweiMessageRequest` 中新增 case：

| messageType | QiWei method | 参数来源 |
|-------------|-------------|---------|
| link | `/msg/sendLink` | channelMeta 中取 title/desc/iconUrl/linkUrl |
| location | `/msg/sendLocation` | channelMeta 中取 title/address/latitude/longitude |
| miniapp | `/msg/sendWeapp` | channelMeta 中透传完整参数（来自接收时保存的 msgData） |

**为什么用 channelMeta 而不是解析 content？** 因为这些类型需要结构化参数（经纬度、URL 等），放在 content 字符串里再解析不可靠。channelMeta 是现有的 `map[string]any` 扩展字段，正好用于传递渠道特有的结构化数据。

### D5: 消息引用回复

在 outgoingMessage 的 channelMeta 中支持 `reply` 字段：

```json
{
  "channelMeta": {
    "reply": {
      "type": 0,
      "msgServerId": 1003054,
      "userId": "168...",
      "timeStamp": 1625957403,
      "msgUniqueIdentifier": "...",
      "msgData": { "content": "被引用的原始消息" }
    }
  }
}
```

adapter 在发送 sendText/sendHyperText 时，如果 channelMeta 中有 reply 字段，则透传到 QiWei API params 中。

### D6: 撤回消息

新增独立的 API 端点或复用 module action 机制。在 agent 侧新增 `wecom_revoke_message` 工具，调用 adapter 的 `/api/qiwei/message/revoke` (module action)。

参数：chatId (会话 ID) + msgServerId (消息服务端 ID)。`internal/modules/message.go` 已注册了 `revoke → /msg/revokeMsg`，所以 agent 可以直接通过 `wecom_send_message` 之外的 module action 路径调用。但为了工具语义清晰，仍建议注册一个专用工具。

## Risks / Trade-offs

- **[Base64 解码可能失败]** → 所有 `decodeMaybeBase64` 调用失败时降级返回原始值，不中断流程
- **[新 msgType 的 msgData 结构不稳定]** → 每个 contentFrom* 函数内部用 `firstNonEmpty` + `anyToString` 做防御式提取，字段缺失时给合理的 placeholder
- **[图文混合消息子消息处理复杂]** → 先做简单版：文字子消息拼接 content，图片子消息只取第一张做附件。完整多图支持后续迭代
- **[小程序转发参数依赖原始 msgData]** → 接收时必须完整保留 msgData 到 channelMeta，任何字段丢失都会导致转发失败。这是上游 API 的约束，无法规避
- **[撤回消息需要 msgServerId]** → 发送成功后返回的 response 中包含此值，需要在发送时解析并返回给 agent。当前 `handleSend` 已返回 response data，agent 需从中提取
