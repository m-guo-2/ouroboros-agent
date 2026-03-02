# 飞书 reply_to 严格模式（模型显式控制）

- **日期**：2026-02-28
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

此前 `send_channel_message` 在未提供回复目标时，会自动回填当前入站消息的 `channelMessageId` 作为 `reply_to`。
该行为降低了模型对回复语义的控制力，不利于“回复当前消息 / 指定历史消息 / 发送非引用消息”的明确区分。

## 决策

改为 strict 模式：`reply_to` 仅在模型显式提供 `replyToChannelMessageId` 时生效，系统不再自动兜底回填。

## 变更内容

- `agent/internal/runner/processor.go`
  - 为 `send_channel_message` 增加可选参数 `replyToChannelMessageId`。
  - 删除默认使用 `request.ChannelMessageID` 的自动回填逻辑。
  - 在用户消息格式化头部新增 `msg_id=...`，让模型可见上游消息 ID。
- `agent/internal/storage/types.go`
  - 为 `MessageData` 增加 `ChannelMessageID` 字段。
- `agent/internal/storage/messages.go`
  - `SaveMessage` 补齐 `channel_message_id` 写入。
  - `GetMessageByID` / `GetSessionMessages` 补齐 `channel_message_id` 读取。

## 考虑过的替代方案

- 保留“模型可传 + 系统自动兜底”兼容模式。
  - 优点：降低模型出错概率。
  - 缺点：语义边界不清，仍会出现系统替模型做决策的问题。
  - 结论：不采用，直接切 strict。

## 影响

- 模型具备完整回复目标控制权，行为更可解释。
- 若模型未传 `replyToChannelMessageId`，飞书侧将发送普通消息而非引用回复。
- 历史消息中持久化 `channel_message_id` 后，后续可支持更稳定的跨轮次引用策略。
