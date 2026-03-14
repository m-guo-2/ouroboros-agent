# Session 上下文连贯性：完整对话历史加载

- **日期**：2026-02-24
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

Agent 的每轮对话是孤立的——`processOneEvent()` 只构建当前一条用户消息传给 LLM，不加载任何历史。导致 Agent 无法记住之前聊了什么、做了什么，用户每轮都需要重复上下文。

基础设施（`getSessionMessages`、`saveMessage`、`buildContextPrompt`）全部就绪但未接入。

## 决策

1. **保存完整消息链**：每轮对话将 user 消息和 Agent Loop 产生的所有消息（包括 assistant 回复、tool_use、tool_result）持久化到 messages 表
2. **加载时原样恢复**：下一轮从 DB 加载历史，直接转为 `AgentMessage[]` 传给 LLM，保留完整的 tool_use/tool_result 结构
3. **截断控制 token**：历史中的 `tool_result` 内容超过 800 字符时截断，`tool_use` 的 name + input 保持完整

## 变更内容

**`agent/src/engine/runner.ts`**（唯一改动文件）：

- 新增 `serializeContent` / `deserializeContent`：AgentMessage ↔ DB 的序列化/反序列化，structured 消息用 JSON 存储
- 新增 `dbMessagesToAgentMessages`：DB 记录转 LLM 消息格式
- 新增 `ensureAlternation`：保证 user/assistant 严格交替（Anthropic API 要求），连续同 role 消息自动合并
- 新增 `truncateToolResults`：截断历史中过大的 tool_result
- 修改 `processOneEvent` 流程：加载历史 → 保存用户消息 → 组装完整序列 → 运行 Loop → 保存 Loop 产生的新消息

消息存储格式：
- `message_type="text"`：content 为纯文本
- `message_type="structured"`：content 为 JSON 序列化的 ContentBlock[]

## 考虑过的替代方案

**方案 A：只保存简化的 user/assistant 文本对**
- 否决原因：Agent 丢失工具调用上下文，不知道之前用了什么工具、得到什么结果，无法做出连续决策

**方案 B：用 `buildContextPrompt()` 将历史拼成文本注入 user message**
- 否决原因：丢失原生 role 标识，LLM 理解不如原生 user/assistant 消息序列准确

## 影响

- 每轮对话 token 开销增加（历史消息占用 input tokens），通过 `AGENT_HISTORY_LIMIT`（默认 50 条）和 tool_result 截断控制
- DB messages 表数据量增长（每轮存多条记录，含 tool 交互），需关注存储
- 后续可增加：按轮次分层压缩（近期完整、远期仅文本）、token 估算动态裁剪
