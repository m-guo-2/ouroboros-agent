# 消息历史重构：设计文档

## 1. 设计原则

**历史消息 = 客观事件日志，不是模型的思维记录。**

DB 中只存储三种客观事实：

| DB Role | 语义 | 内容 |
|---------|------|------|
| `user` | 某个用户说了什么 | 裸文本 + `sender_name` / `sender_id` 字段 |
| `assistant` | Agent 调用了什么工具 | 仅 `tool_use` ContentBlock[]（剥离 text/thinking） |
| `tool_result` | 工具执行返回了什么 | `tool_result` ContentBlock[] |

**不存储的内容：**

- 模型的 text 输出（推理过程）—— 用户不可见，非客观事实，会造成锚定偏差
- 纯文本的 assistant end_turn 回复 —— 用户同样不可见
- system 消息 —— 由 `systemPrompt` 参数动态生成，不进入历史

## 2. 各厂商 API Role 调研

| 厂商 | 消息 Role | 工具结果 Role | `name` 字段 | System Prompt |
|------|----------|-------------|------------|---------------|
| Anthropic | `user`, `assistant` | `user` + tool_result block | 不支持 | 独立 `system` 参数 |
| OpenAI | `system`, `developer`, `user`, `assistant`, `tool` | `role: "tool"` | 支持 | `role: "system"` 或 `"developer"` |
| Kimi/Moonshot | `system`, `user`, `assistant`, `tool` | `role: "tool"` | 支持 | `role: "system"` |
| 智谱 GLM | `system`, `developer`, `user`, `assistant`, `tool` | `role: "tool"` | 支持 | `role: "system"` |
| Google Gemini | `user`, `model` | `functionResponse` part (in user Content) | 不支持 | 独立参数 |

**关键结论：**

- OpenAI 系（OpenAI/Kimi/GLM）统一支持 `role: "tool"` 和 `name` 字段
- Anthropic 是唯一需要把 tool_result 塞进 `role: "user"` 的，且不支持 `name` 字段
- 内部存储用 `tool_result` 是安全的 —— OpenAI 系直接映射为 `role: "tool"`，Anthropic 转为 `role: "user"` + content block
- 群聊身份：OpenAI 系用原生 `name` 字段，Anthropic 在 content 前拼 `[senderName]`

## 3. 数据流

```
                    ┌──────────────────────────────────┐
                    │        DB Storage (无关 Provider)  │
                    │                                    │
                    │  role=user     content + sender_*  │
                    │  role=assistant tool_use blocks     │
                    │  role=tool_result tool_result blocks│
                    └─────────┬───────────┬──────────────┘
                              │           │
                    ┌─────────▼──┐   ┌────▼──────────┐
                    │  Anthropic  │   │ OpenAI/Kimi/  │
                    │  转换       │   │ GLM 转换      │
                    ├────────────┤   ├───────────────┤
                    │ user →      │   │ user →        │
                    │  role:user  │   │  role:user    │
                    │  [name]前缀 │   │  name 字段    │
                    │            │   │               │
                    │ assistant → │   │ assistant →   │
                    │  role:asst  │   │  role:asst    │
                    │  tool_use   │   │  tool_calls   │
                    │            │   │               │
                    │ tool_result│   │ tool_result → │
                    │  → role:user│   │  role:tool    │
                    │  blocks    │   │  tool_call_id │
                    └────────────┘   └───────────────┘
```

## 4. 群聊场景身份区分

**问题：** 群聊中多个用户发消息，`role: "user"` 无法区分是谁说的。

**方案：**

- **DB 存储**：`sender_name` + `sender_id` 字段记录发送者身份
- **Anthropic**：不支持 `name` 字段，在 content 文本前拼 `[senderName] ` 前缀
- **OpenAI 系**：原生 `name` 字段传递身份，`extractSenderName()` 从 Anthropic 格式的前缀中提取

## 5. 改动文件清单

### `server/src/services/database.ts`

- `MessageRole` 类型扩展为 `"user" | "assistant" | "tool_result" | "system"`
- `MessageRecord` 增加 `senderName?: string` 和 `senderId?: string`
- `MessageRow` 增加 `sender_name` 和 `sender_id`
- `rowToMessage()` 读取新字段
- `insert()` 写入新字段
- ALTER TABLE 迁移添加 `sender_name` / `sender_id` 列

### `server/src/routes/data.ts`

- POST `/api/data/messages` 解构和传递 `senderName` / `senderId`

### `agent/src/services/server-client.ts`

- `MessageData` 接口增加 `senderName` / `senderId`
- `saveMessage()` 参数增加 `senderName` / `senderId`

### `agent/src/engine/runner.ts`

- **`toPersistableMessages()`**（新增）：将 loop 输出转为 DB 格式
  - assistant + tool_use → 剥离 text，只存 tool_use blocks，role="assistant"
  - assistant + 纯 text → 丢弃
  - user + tool_result → role="tool_result"
- **`dbMessagesToAgentMessages()`**（重写）：从三种 DB role 重建 Anthropic 内部格式
  - user → `[senderName]` 前缀 + content
  - assistant → tool_use ContentBlock[]
  - tool_result → role:"user" + tool_result ContentBlock[]（Anthropic 要求）
- **用户消息保存**：裸内容 + `senderName`/`senderId` 字段（不再拼元信息前缀）
- **loop 消息保存**：使用 `toPersistableMessages()` 过滤后存储

### `agent/src/services/context-composer.ts`

- system prompt "关于历史消息的理解" 段落更新：历史只含客观事件，不含推理过程

### `agent/src/engine/llm-client.ts`

- 新增 `extractSenderName()` 辅助函数：从 `[name] text` 格式提取 name
- `OpenAICompatibleClient.convertMessages()` 更新：
  - user 纯文本消息提取 `[senderName]` 前缀为 `name` 字段
  - user content block 中的 text 也提取 name
  - assistant 消息保持已有逻辑（text → content, tool_use → tool_calls）

## 6. 核心函数说明

### `toPersistableMessages(loopMessages)`

输入 agent loop 运行时产生的完整消息序列，输出可持久化的消息数组。核心过滤逻辑：

```
assistant msg:
  有 tool_use blocks → 保留 tool_use，剥离 text → role="assistant"
  无 tool_use（纯 text）→ 丢弃

user msg (loop 产生的都是 tool_result):
  有 tool_result blocks → role="tool_result"
  其他 → 丢弃
```

### `dbMessagesToAgentMessages(dbMessages)`

输入 DB 消息记录，输出 Anthropic 内部格式的 `AgentMessage[]`。这是 engine 的内部标准格式。OpenAI 兼容客户端在调用时再做二次转换。

### `extractSenderName(text)`

从 `[张三] 帮我查日程` 格式中提取 `{ name: "张三", text: "帮我查日程" }`。仅在 OpenAI 兼容客户端中使用（Anthropic 保留前缀不拆分）。

## 7. 后续迭代方向

- **长期记忆压缩**：超过 N 条的历史消息压缩为摘要，存入 memory 表
- **Gemini API 适配**：当前未接入，需要处理 `functionResponse` part 格式
- **token 预算管理**：根据模型 context window 动态调整历史消息条数
- **多模态消息支持**：图片、文件等非文本内容的历史存储与回放
