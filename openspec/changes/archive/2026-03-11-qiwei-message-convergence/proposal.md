## Why

channel-qiwei 适配器当前只能接收和发送有限的消息类型（文本/图片/文件/语音/视频），大量真人日常使用的消息类型（表情、链接、位置、名片、红包、小程序、视频号、图文混合）被静默丢弃，agent 完全看不见。发送侧同样缺乏链接卡片、定位、消息引用、撤回等能力。这导致 agent 无法像真人一样自然地参与微信对话——既听不全，也说不全。此外 msgType=0 的文本消息被遗漏是一个现有 bug。

## What Changes

### 接收侧（QiWei → adapter → agent）

- **修复** msgType=0 文本消息被丢弃的 bug
- **补齐** msgType=7 (企微图片另一编码) 接入现有 image 流程
- **新增** 表情/GIF (29/104) 接收：下载 → OSS → 作为 image 附件送给 agent（与图片同流程，agent 可用视觉模型理解）
- **新增** 链接消息 (13) 接收：Base64 解码 title/desc/linkUrl，收敛为人类可读文本
- **新增** 位置消息 (6) 接收：提取 address/latitude/longitude，收敛为文本
- **新增** 名片消息 (41) 接收：Base64 解码 nickname/corpName，收敛为文本
- **新增** 红包消息 (26) 接收：Base64 解码 wishingContent，收敛为只读文本通知
- **新增** 小程序消息 (78) 接收：Base64 解码 title/desc，收敛为文本 + 保留原始 msgData 到 channelMeta（供转发时使用）
- **新增** 视频号消息 (141) 接收：Base64 解码 channelName/channelUrl，收敛为文本
- **新增** 图文混合消息 (123) 接收：拆分子消息数组，文字拼入 content，图片走附件流程
- **新增** 系统事件路由：cmd=15500 中 msgType=1002（新成员入群）转为系统消息推送给 agent
- **新增** 好友申请 (2357/2132) 记录日志，暂不推送 agent

### 发送侧（agent → adapter → QiWei）

- **新增** 发送链接卡片：`/msg/sendLink` (title + desc + iconUrl + linkUrl)
- **新增** 发送定位消息：`/msg/sendLocation` (title + address + latitude + longitude)
- **新增** 发送小程序消息：`/msg/sendWeapp` (基于接收时保存的 msgData 参数)
- **新增** 消息引用回复：在 sendText/sendHyperText 的 params 中支持 reply 字段
- **新增** 撤回消息：`/msg/revokeMsg` (chatId + msgServerId)
- **扩展** 主回复路径 `toQiweiMessageRequest` 支持 video/link/location 等新类型

### Agent 侧工具更新

- 更新 `wecom_send_message` 工具描述，覆盖新支持的发送类型
- 新增 `wecom_revoke_message` 工具
- 更新 `send_channel_message` 中 messageType 的可选值说明

## Capabilities

### New Capabilities

- `receive-rich-messages`: 接收侧补齐所有常见消息类型的解析和收敛逻辑（表情、链接、位置、名片、红包、小程序、视频号、图文混合）
- `send-rich-messages`: 发送侧扩展支持链接卡片、定位、小程序、消息引用回复、撤回消息
- `system-event-routing`: 系统事件（cmd=15500）的路由和处理，包括群成员变动通知和好友申请记录

### Modified Capabilities

(无已有 spec 需要修改)

## Impact

- **channel-qiwei 服务**：`events.go`（回调路由 + 消息类型映射）、`media_pipeline.go`（mediaClassifications 扩展）、`api_handlers.go`（toQiweiMessageRequest 扩展）、`facade_handlers.go`（toFacadeQiweiMessageRequest 扩展）、`models.go`（qiweiCallbackMessage 补 cmd 字段）
- **agent 服务**：`runner/wecom_builtin_tools.go`（工具描述更新 + 新增撤回工具）、`runner/processor.go`（formatUserMessage 中对新 messageType 的处理）
- **API 契约**：adapter → agent 的 `incomingMessage.messageType` 新增枚举值；agent → adapter 的 `outgoingMessage.messageType` 新增枚举值
- **无破坏性变更**：所有新增类型对 agent 侧向后兼容，未识别的 messageType 会按 text 降级处理
