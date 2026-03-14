## 1. 基础设施：回调路由与数据模型

- [x] 1.1 `models.go`: 给 `qiweiCallbackMessage` 结构体新增 `Cmd int` 字段
- [x] 1.2 `events.go`: `decodeOneMessage` 中解析 cmd 字段，默认值 15000
- [x] 1.3 `events.go`: `handleCallbackMessage` 开头按 cmd 分流 — cmd=15000 走现有逻辑，cmd=15500 走新增的 `handleSystemEvent`，其他 cmd 仅日志
- [x] 1.4 `events.go`: `userMessageTypeMap` 补齐 0→"text"、7→"image"，修复现有 bug

## 2. 接收侧：纯文本收敛类型

- [x] 2.1 `events.go`: 新增 `contentFromLink(msgData) string` — 处理 msgType=13，Base64 解码 title/desc/linkUrl，拼接为 `"[链接] 标题：...\n描述：...\n地址：..."`
- [x] 2.2 `events.go`: 新增 `contentFromLocation(msgData) string` — 处理 msgType=6，Base64 解码 address/title，拼接为 `"[位置] {title} {address} (纬度:..., 经度:...)"`
- [x] 2.3 `events.go`: 新增 `contentFromCard(msgData) string` — 处理 msgType=41，Base64 解码 nickname/corpName，拼接为 `"[名片] {nickname} 企业：{corpName}"`，channelMeta 保存 shared_id
- [x] 2.4 `events.go`: 新增 `contentFromRedPacket(msgData) string` — 处理 msgType=26，Base64 解码 wishingContent，拼接为 `"[红包] {wishingContent}"`
- [x] 2.5 `events.go`: 新增 `contentFromMiniapp(msgData) (string, map[string]any)` — 处理 msgType=78，Base64 解码 title/desc，返回文本和原始 msgData（保存到 channelMeta）
- [x] 2.6 `events.go`: 新增 `contentFromChannelMsg(msgData) string` — 处理 msgType=141，Base64 解码 channelName/channelUrl，拼接为 `"[视频号] {channelName}\n链接：..."`

## 3. 接收侧：需媒体处理的类型

- [x] 3.1 `events.go` + `media_pipeline.go`: 将 msgType 29/104 (GIF/表情) 接入 `userMessageTypeMap` 映射为 "sticker"，走 image 下载→OSS 流程。mediaClassifications 中已有 29/104 条目（Kind=Unknown），改为 Kind=Image
- [x] 3.2 `events.go`: 新增 `contentFromMixed(msgData) (string, []incomingAttachment)` — 处理 msgType=123，遍历子消息数组：subMsgType=2 的文字（Base64 解码）拼入 content，subMsgType=14 的图片走 `prepareMediaForAgent` 生成附件
- [x] 3.3 `events.go`: `handleCallbackMessage` 中对上述纯文本类型和媒体类型的分支处理——纯文本类型直接调用 contentFrom* 构造 content 和可选 channelMeta，不经过 `prepareMediaForAgent`

## 4. 系统事件处理

- [x] 4.1 `events.go`: 新增 `handleSystemEvent(ctx, msg qiweiCallbackMessage) error`，按 msgType 分流
- [x] 4.2 `events.go`: msgType=1002（新成员入群）— 构造 messageType="system" 的 incomingMessage，channelConversationID=fromRoomId，content="[群事件] 新成员加入了群聊"，转发给 agent
- [x] 4.3 `events.go`: msgType=2357/2132（好友申请）— 记录 info 日志（含 contactNickname/contactId），不转发
- [x] 4.4 `events.go`: 其他 msgType — 记录 info 日志，不转发

## 5. 发送侧：adapter 路由扩展

- [x] 5.1 `api_handlers.go`: `toQiweiMessageRequest` 新增 case "link" → `/msg/sendLink`，从 channelMeta 取 title/desc/iconUrl/linkUrl
- [x] 5.2 `api_handlers.go`: `toQiweiMessageRequest` 新增 case "location" → `/msg/sendLocation`，从 channelMeta 取 title/address/latitude/longitude
- [x] 5.3 `api_handlers.go`: `toQiweiMessageRequest` 新增 case "miniapp" → `/msg/sendWeapp`，从 channelMeta 透传完整参数
- [x] 5.4 `api_handlers.go`: `toQiweiMessageRequest` 中 text/rich_text case 支持 channelMeta.reply 透传到 params
- [x] 5.5 `facade_handlers.go`: `toFacadeQiweiMessageRequest` 对齐上述所有新增 case（link/location/miniapp/reply）

## 6. 发送侧：撤回消息

- [x] 6.1 `internal/modules/message.go` 确认已注册 revoke → `/msg/revokeMsg`（已有则跳过）
- [x] 6.2 agent 侧 `runner/wecom_builtin_tools.go`: 新增 `wecom_revoke_message` 工具注册，参数为 chatId + msgServerId，通过 adapter module action 路径调用

## 7. Agent 侧工具描述更新

- [x] 7.1 `runner/wecom_builtin_tools.go`: 更新 `wecom_send_message` 工具的 messageType 描述，新增 link/location/miniapp 类型说明
- [x] 7.2 `runner/wecom_builtin_tools.go`: 更新 `wecom_send_message` 工具的 channelMeta 描述，说明各类型需要的字段
- [x] 7.3 `runner/processor.go`: `send_channel_message` 工具的 messageType 描述中补充新支持的类型

## 8. 验证与编译

- [x] 8.1 `go build ./...` 编译 channel-qiwei 通过
- [x] 8.2 `go build ./...` 编译 agent 通过
- [ ] 8.3 手动验证：向机器人发送链接/位置/名片/红包/小程序/表情消息，检查 agent 收到的 content 是否可读
