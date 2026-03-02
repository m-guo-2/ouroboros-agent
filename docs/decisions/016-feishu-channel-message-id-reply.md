# 飞书消息 ID 传递与引用回复

- **日期**：2026-02-26
- **类型**：代码变更
- **状态**：已实施

## 背景

飞书给 Agent 发送 event 时应携带消息 ID，便于 Agent 执行后续操作（如引用回复）。此前 channelMessageId 已从飞书 → channel-feishu → server 传到 agent，但 agent 在调用 `send_channel_message` 时未将其作为 `replyToChannelMessageId` 传给渠道，导致回复无法以引用形式展示在用户消息下。

## 决策

1. Agent 在 `send_channel_message` 中，当存在 `ChannelMessageID` 时，将其作为 `replyToChannelMessageId` 传给 server。
2. Server 的 `/api/data/channels/send` 路由支持解析并转发 `replyToChannelMessageId` 到渠道适配器。
3. channel-feishu 的 legacy 格式已支持 `replyToChannelMessageId`，无需改动。

## 变更内容

- **agent/internal/runner/processor.go**：在 `send_channel_message` 工具构建 msg 时，当 `request.ChannelMessageID` 非空时添加 `replyToChannelMessageId`。
- **server/src/routes/data.ts**：在 `/channels/send` 解构 req.body 时增加 `replyToChannelMessageId`，并写入 `outgoing` 对象传给 `sendToChannel`。

## 影响

- Agent 回复飞书消息时，会自动以引用形式展示在用户原消息下，提升对话连贯性。
- 飞书等支持引用回复的渠道均可受益；WebUI 等不支持的渠道会忽略该字段，无副作用。
