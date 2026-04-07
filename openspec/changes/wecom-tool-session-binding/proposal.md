## Why

Agent 在一个 session 中运行，session 绑定一个会话（私聊或群聊），但当前 wecom 工具的 schema 要求 LLM 显式提供 `channelConversationId`、`channelUserId`、`chatId` 等会话级参数。这些参数对 LLM 来说是冗余的——它不知道这些 ID 的值，只知道"在当前对话中操作"。结果是 LLM 可能产生幻觉填错 ID，或浪费 token 纠结要不要填。

`ProcessRequest`（sessionReq）已经携带了这些信息并传入 `registerWecomBuiltinTools`，但大部分工具没有使用它。需要将会话级参数从工具 schema 中移除，在 executor 层从 session 上下文自动注入。

## What Changes

- 从 `wecom_send_message` schema 移除 `channelConversationId` / `channelUserId`，executor 从 `sessionReq` 注入
- 从 `wecom_revoke_message` schema 移除 `chatId`，executor 从 `sessionReq` 注入
- `wecom_list_or_get_conversations` 读当前会话历史时，`conversationId` 从 `sessionReq` 注入
- `wecom_get_group_detail` 查当前群详情时，`roomIds` 从 `sessionReq` 注入
- 统一使用 wrapper 函数在 executor 层做 session parameter injection，保持 `ToolExecutor` 接口不变

## Capabilities

### New Capabilities
- `session-parameter-injection`: 定义 session-scoped parameter 的注入机制——从 `ProcessRequest` 提取会话级参数，在 tool executor 执行前自动填充到 input map 中，对 LLM 完全透明

### Modified Capabilities

## Impact

- `agent/internal/runner/wecom_builtin_tools.go`：修改工具注册，schema 移除会话参数，executor 改用 session-aware wrapper
- `channel-qiwei/facade_handlers.go`：无需改动，已支持接收这些参数
- LLM 行为变化：工具调用时不再需要提供会话 ID，降低幻觉风险和 token 消耗
- 跨会话操作当前不在范围内，后续如需支持可通过新增独立工具实现
