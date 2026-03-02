# 历史消息加载兜底 + 用户消息格式优化

- **日期**：2025-02-25
- **类型**：代码变更
- **状态**：已实施

## 背景

Agent 存在两个问题：
1. 当 `session.Context`（JSON 序列化的完整对话历史）为空时（首次加载、context 丢失、crash 后恢复），Agent 丢失所有历史消息，只使用当前轮消息
2. 用户消息格式冗余且信息不佳：`[消息来源] channel=feishu channelUserId=ou_xxx` 元数据行噪音大，发送者标识只显示 channelUserID（技术 ID），不显示用户昵称

## 决策

1. 增加 messages 表兜底：当 `session.Context` 为空时，从 messages 表重建对话历史
2. 简化用户消息格式：去掉 `[消息来源]` 元数据行，改为 `[昵称 (渠道ID)]` 格式

## 变更内容

### Agent 侧 (`agent/internal/runner/processor.go`)
- 新增 `formatUserMessage(senderName, channelUserID, content)` 函数，统一格式化用户消息为 `[SenderName (channelUserID)]\ncontent`
- 新增 `reconstructHistoryFromMessages()` 函数，从 messages 表加载并重建 `[]AgentMessage` 格式的对话历史
- 删除 `[消息来源]` 元数据行的拼接逻辑
- 更新系统提示中的发送者格式说明

### Agent 侧 (`agent/internal/engine/llm.go`)
- 修改 `extractSenderName()` 适配新格式，从 `[SenderName (channelUserID)]` 中提取显示名

### Server 侧 (`server/src/services/channel-dispatcher.ts`)
- `dispatchIncomingMessage` 和 `dispatchIncomingMessageStream` 存用户消息时增加 `senderName` 和 `senderId` 字段

## 影响

- 消息格式变化：从 `[消息来源] channel=xxx channelUserId=xxx\n[sender]\ncontent` 变为 `[昵称 (渠道ID)]\ncontent`，已有 session 的 context 中存储的旧格式消息不受影响（JSON 原样保留）
- 历史消息重建是尽力而为：messages 表中的 `senderName`/`senderId` 在此变更之前未保存，旧消息重建时 sender 信息可能为空
