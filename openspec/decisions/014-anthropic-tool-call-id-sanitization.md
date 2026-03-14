# Anthropic API tool_call_id 校验与消息清洗

- **日期**：2026-02-25
- **类型**：Bug 修复
- **状态**：已实施

## 背景

Agent 调用 Anthropic Messages API 时偶发 `tool_call_id is not found` 400 错误。原因可能包括：

1. 历史 truncation 后，tool_result 引用的 tool_use_id 在保留的消息中已不存在
2. Session 或 DB 中加载的旧数据，tool_result 的 tool_use_id 为空或损坏
3. JSON 序列化时 `omitempty` 导致空的 tool_use_id 被省略，API 收到无效结构

## 决策

在 AnthropicClient 发请求前对 messages 做清洗：只保留 `tool_use_id` 非空且能在前置 assistant 消息中找到对应 tool_use 的 tool_result；无效的用占位文本替代，保持 user/assistant 交替。

## 变更内容

- **agent/internal/engine/llm.go**
  - 新增 `sanitizeMessagesForAnthropic`：遍历消息，收集所有 tool_use 的 ID，过滤掉无效 tool_result
  - AnthropicClient.Chat 在构建请求体前调用 `sanitizeMessagesForAnthropic(params.Messages)`
  - 若 user 消息在过滤后为空，用 `[Tool results omitted – references invalid or truncated]` 占位，避免连续 assistant
- **agent/internal/engine/llm_test.go**
  - 新增 `TestSanitizeMessagesForAnthropic` 覆盖：有效保留、孤儿引用移除、空 ID 移除

## 后续：诊断日志（2026-02-25 追加）

为追踪「原始数据为什么会少」的根因，补充诊断日志：

- **processor**：历史加载后打 Debug 日志 `历史消息加载诊断`，含 source（session/db_reconstruct/empty）、messageCount、diagnostic（toolUseIDs、toolResultRefs、emptyToolUse、emptyToolResult）
- **llm.sanitizeMessagesForAnthropic**：当过滤掉 tool_result 时打 Warn 日志，含 dropped 详情与 validToolUseIDs
- **AnthropicClient.Chat**：当 API 返回含 tool_call_id 的错误时打 Error 日志，含已发送消息的 diagnostic

复现问题时可根据这些日志判断：数据来自 session 还是 DB  reconstructed、truncate 后是否有 orphan、是否存在空 ID 等。

## 后续：两遍清洗修复顺序敏感（2026-02-25 追加）

**问题**：trace t-d879cf739f 中，历史诊断显示 `toolUseIDs` 与 `toolResultRefs` 均为 `["send_channel_message_0"]`，匹配正常，但 sanitize 仍把 tool_result 判为 orphan 丢弃，导致 Anthropic 400：`send_channel_message:0 did not have response`。

**根因**：原实现是单遍遍历，`validToolUseIDs` 在遇到 assistant 时才累加。若消息顺序异常（如 DB reconstruct 或 session 上下文顺序与预期不同），user(tool_result) 可能先于对应 assistant 被处理，此时 `validToolUseIDs` 尚未包含该 ID，导致误判为 orphan。

**决策**：改为两遍处理——第一遍遍历所有 assistant 收集 `validToolUseIDs`，第二遍再过滤 tool_result。这样无论消息顺序如何，有效 tool_result 都不会被误删。

**变更**：`agent/internal/engine/llm.go` 中 `sanitizeMessagesForAnthropic` 先做一次纯收集 pass，再做一次过滤 pass。

## 后续：消息顺序确定性问题（2026-02-25 追加）

**问题**：DB 中 `assistant`（tool_use）与 `tool_result` 常为同一秒插入，`created_at` 相同。`getBySession` 仅按 `created_at` 排序时，二者相对顺序不确定，可能导致 reconstruct 产生 `tool_result` 先于 `assistant` 的序列，违反 Anthropic「assistant 先于 tool_result」要求。

**role=tool_result**：使用 `role=tool_result` 作为存储约定是正确的，用于在 DB 中区分工具结果与普通用户消息；reconstruct 时会正确转为 role=user 的 content blocks。

**决策**：在 `getBySession` 中增加 `rowid` 作为 `created_at` 相同时的稳定排序，利用 SQLite rowid 与插入顺序一致，保证 assistant 先于 tool_result。

**变更**：`server/src/services/database.ts` 中 `messagesDb.getBySession` 的 ORDER BY 增加 `, rowid ASC/DESC` 作为 tiebreaker。

**插入顺序**：agent 端 `OnNewMessages` 已按 `[assistant, tool_result]` 顺序调用 `SaveMessage`，无需改动。
