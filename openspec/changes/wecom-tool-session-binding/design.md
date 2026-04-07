## Context

当前 `registerWecomBuiltinTools(registry, request ProcessRequest)` 已经接收 `sessionReq`，其中包含 `ChannelConversationID` 和 `ChannelUserID`。但除 `inspect_attachment` 外，其余工具都没有使用这个上下文——它们要求 LLM 在每次调用时显式提供会话 ID。

Agent 始终在单一会话中运行，不存在跨会话操作的需求。会话级参数对 LLM 是不可知的（它没有记忆这些 ID），暴露在 schema 中只会引发幻觉或无效调用。

已有先例：`send_channel_message`（非 wecom 工具）的 executor 从 `sessionReq` 闭包注入目标 ID，schema 中不暴露任何会话参数。

## Goals / Non-Goals

**Goals:**

- 从 wecom 工具的 JSON Schema 中移除所有会话级参数，LLM 无感知
- 在 executor 层通过闭包从 `ProcessRequest` 自动注入会话 ID
- 保持 `ToolExecutor` 函数签名 `(ctx, input) → (result, error)` 不变
- 保持 channel-qiwei facade 层不变（它已经能正确接收这些参数）

**Non-Goals:**

- 跨会话操作（给别的群/人发消息）不在此次范围
- 不改造 `ToolExecutor` 接口或引入通用中间件层
- 不改造 channel-qiwei 侧的任何代码

## Decisions

### D1: 用闭包捕获 sessionReq，不引入新抽象

**选择**：每个需要会话参数的 executor 通过闭包捕获 `ProcessRequest`，在执行时将会话 ID 写入 `input` map。

**替代方案**：
- 改 `ToolExecutor` 签名为 `(ctx, ToolContext, input) → result`：影响面太大，所有工具都要改
- 通用 middleware/interceptor 层：过度抽象，当前只有 4-5 个工具需要注入

**理由**：闭包模式已经是代码库的既有模式（`inspect_attachment`、`send_channel_message` 都用了），零接口变更，逐工具改造。

### D2: session_fixed 策略——从 schema 完全移除，不提供覆盖入口

**选择**：会话参数从 schema 中删除，executor 无条件从 session 注入。不提供"LLM 可选覆盖"的机制。

**替代方案**：
- `session_default`（schema 保留，LLM 不填时从 session 填充）：给 LLM 增加认知负担，它可能纠结要不要填或填错

**理由**：当前不需要跨会话操作。如果未来需要，通过新增独立工具（如 `wecom_send_to_target`）实现，职责更清晰。

### D3: 改造 createWecomSendMessageExecutor 使其接收 ProcessRequest

**选择**：`createWecomSendMessageExecutor(request ProcessRequest)` 接收 session 上下文，在构建每条消息的 payload 时从 `request` 注入 `channelConversationId` / `channelUserId`。

现有代码从 `input` 读取：
```go
convID, _ := input["channelConversationId"].(string)
userID, _ := input["channelUserId"].(string)
```

改为从 `request` 读取：
```go
convID := request.ChannelConversationID
userID := request.ChannelUserID
```

### D4: wecom_revoke_message 的 chatId 注入

`chatId` 语义等于 `channelConversationId`（私聊时是 userId，群聊时是 roomId）。注入方式：

```go
input["chatId"] = resolveSessionConversationID(request)
```

其中 `resolveSessionConversationID` 优先取 `ChannelConversationID`，为空则取 `ChannelUserID`（与 dispatcher 的 session key 逻辑一致）。

### D5: wecom_get_group_detail 仅在群聊 session 中自动注入

当 session 的 `ChannelConversationID` 存在（群聊场景）时，executor 自动将 `roomIds` 设为 `[ChannelConversationID]`。

私聊场景下 `ChannelConversationID` 等于 `ChannelUserID`，不是有效的 roomId——此时工具应返回明确错误（"当前不在群聊中"），而非传入无效 ID。

Schema 改为无参数。

### D6: wecom_list_or_get_conversations 拆分行为

该工具有两种模式：
1. 不给 `conversationId` → 列最近会话（不需要注入）
2. 给 `conversationId` → 读指定会话历史

注入策略：executor 注入当前会话 ID 作为默认值，LLM 调用时默认读当前会话历史。`conversationId` 从 schema 移除。

如果 LLM 需要列最近会话（模式 1），通过一个布尔参数 `listRecent: true` 触发，此时忽略注入的 conversationId。

## Risks / Trade-offs

- **[风险] 私聊场景下 wecom_get_group_detail 无效** → 缓解：executor 检测 session 类型，非群聊时返回清晰错误而非静默失败
- **[风险] 未来需要跨会话操作** → 缓解：通过新增独立工具实现，不影响当前设计
- **[取舍] wecom_list_or_get_conversations 语义变化** → 原先一个参数控制两种模式，现在需要 `listRecent` 布尔参数区分。多了一个参数，但语义更清晰
