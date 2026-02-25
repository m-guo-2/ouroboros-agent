# Session Context & Logging 重构设计文档

- **日期**：2026-02-25
- **类型**：架构决策 / 代码重构
- **状态**：设计中

## 1. 背景与核心痛点

当前系统中，Agent 的消息历史和内部执行步骤在数据层混杂在一起，主要体现在 `messages` 表被赋予了多重职责：既要用于终端用户的 UI 展示，又要作为 LLM 的上下文记忆（Context）。这种设计导致了以下严重问题：

1. **强行篡改模型输出**：为了避免把 Agent 的内心独白（thinking）发给用户，`loop.ts` 中强行剥离了 `text` 块，导致模型“失忆”，在多轮工具调用中忘记上一步的推理逻辑。
2. **沉重的清洗管道**：为满足 LLM 厂商（如 Anthropic/OpenAI）严格的 `user -> assistant -> user` 角色交替和工具匹配格式，`runner.ts` 堆砌了 `ensureAlternation`、`removeOrphanedToolUses` 等大量补丁代码，逻辑脆弱且一团乱麻。
3. **容易产生反向幻觉**：如果把内部思考保留在历史中，模型会误以为这些内容用户已经看到了（因为它们都按顺序排列在上下文中），从而在后续对话中产生“反向幻觉”。
4. **数据库膨胀与性能问题**：将体积庞大的工具调用中间结果（如网页源码、API 返回）存在 SQLite 的 `execution_steps` 表中，造成不必要的读写压力。

## 2. 设计原则

本次重构的核心思想是**“读写分离与职责归位”**，将通信层（UI 呈现）与执行层（ReAct 中间状态）彻底解耦。

### 原则一：UI 消息与 LLM 上下文彻底分离
- **UI 消息（Chat DB）**：只存客观发生、用户可见的对话（人类提问与 Agent 的显式回复）。
- **LLM 上下文（Model Context）**：存储原汁原味、未经删改的完整交互序列（包含思考过程和详尽的工具输入输出）。

### 原则二：以“回合（Turn）”为原子单位管理状态
一个完整的回合定义为：`[1个用户输入] + [N次模型思考/工具调用] + [1个最终回复]`。
在截断上下文以控制 Token 时，必须以完整的“回合”为单位进行裁剪，绝不能在中间切断，从而彻底消灭“孤儿工具节点”问题。

### 原则三：强制工具回复模式（Tool-based Communication）
为了彻底解决大模型的“反向幻觉”，**剥夺模型直接回复用户的权利，强制它必须调用工具才能跟用户说话**。
- **Text = Internal Thought**：模型直接输出的纯文本，定义为绝对私密的内部日志，用于思维链（CoT），用户不可见。
- **Tool = Action & Communication**：模型必须调用 `send_channel_message` 工具才能将信息发送给用户。

## 3. 架构设计

### 3.1 存储层设计

数据流一分为三，各司其职：

1. **Model Context (SQLite: `session_context`)**
   - **职责**：作为大模型的“脑子”，存储严格符合 LLM API 标准的上下文历史。
   - **结构**：新增表或在 `agent_sessions` 中新增字段，以 JSON 格式存储未经篡改的完整 `AgentMessage[]` 数组。
   - **更新机制**：每次 ReAct Loop 结束后，整体覆盖回写。

2. **Chat UI Messages (SQLite: `messages`)**
   - **职责**：作为 C 端的展示板。
   - **结构**：返璞归真，去除 `tool_calls`、`message_type` 等复杂字段。
   - **写入机制**：仅在收到用户消息，或拦截到 `send_channel_message` 工具执行成功时写入。

3. **Execution Trace Logs (File System: `.jsonl`)**
   - **职责**：作为开发者/后台的调试与时间线展示（Timeline UI）。
   - **结构**：在 `.agent-sessions/{session_id}/` 目录下，按 `trace_id` 存储的 Append-only JSON Lines 文件。
   - **写入机制**：将原 `execution_steps` 中的高频、大体积数据（thinking、tool_call、tool_result）直接写入磁盘。

### 3.2 核心代码重构步骤

#### 第一步：重构存储层（`database.ts`）
- 新增 `session_context` 表或字段。
- 剥离 `messages` 表中不再需要的工具执行字段。
- 调整日志追踪服务，使其落盘 `.jsonl` 文件而非写入数据库。

#### 第二步：重构 LLM 引擎（`loop.ts`）
- 停止在 `loop.ts` 中剥离模型的 `text` 块。
- 将 `response.content`（包含 `text` 和 `tool_use`）原封不动地保留在 `messages` 数组中。

#### 第三步：清理管道与加载逻辑（`runner.ts`）
- **删除废弃代码**：移除 `dbMessagesToAgentMessages`、`ensureAlternation`、`removeOrphanedToolUses` 等所有补丁函数。
- **极简加载**：直接从 `session_context` 加载 JSON 反序列化为 `AgentMessage[]`。
- **回合制截断**：实现基于 Token 预算的 `truncateByFullTurns` 函数，安全地控制上下文长度。
- **全量保存**：Loop 结束后，将最终的 `messages` 数组 `JSON.stringify` 存回数据库。

#### 第四步：强化系统提示词（`context-composer.ts`）
在 System Prompt 中加入严厉的约束指令，明确声明信息不对称和工作流范式：

```markdown
# 沟通准则 (CRITICAL)
你是一个由 ReAct 引擎驱动的自主 Agent。
你必须遵守以下最高优先级规则：
1. **内部思考隔离**：你直接输出的任何普通文本内容，都属于你的【内部思考记录】，**用户是绝对看不到的！**
2. **通过工具发言**：如果你想回复用户、向用户提问、或报告进度，你**必须且只能**调用 `send_channel_message` 工具。
3. 典型工作流示例：
   - 收到用户请求。
   - 输出内部思考（分析需求，规划工具调用）。
   - 调用数据查询工具。
   - 获取工具结果。
   - 输出内部思考（分析结果，组织回复语言）。
   - 调用 `send_channel_message` 工具将最终结果发送给用户。
   - 输出内部思考（记录任务已完成），且不调用任何工具，结束当前回合。
```

## 4. 预期收益

1. **架构极致清晰**：底层大模型上下文数据结构彻底脱离上层业务逻辑，代码大幅精简，维护成本显著降低。
2. **免疫“反向幻觉”与“失忆”**：物理隔离机制让模型清晰分辨“所想”与“所说”，同时完整的思维链保留让模型能够处理复杂长程任务而不迷失。
3. **性能与可扩展性提升**：高频大体积日志下沉至文件系统，SQLite 减负，系统足以支撑更庞大的工具返回结果和并发请求。
