/**
 * Context Composer — 拼接 Agent 上下文
 *
 * 设计原则：
 * - systemPrompt = agentSystemPrompt + skills，仅此两部分，不注入其他内容
 * - 记忆、压缩等能力由 skills 描述，agent 通过工具自主调用
 * - 历史消息作为结构化 ContentBlock[] 传入 messages，保留完整 tool_use/tool_result 上下文
 */

// ==================== 内置能力说明 ====================

/**
 * 内置消息回复能力（所有 Agent 通用）。
 *
 * 核心原则：模型的文字输出（text block）对用户完全不可见。
 * 用户只能收到通过 send_channel_message 工具主动发送的内容。
 * 此段落始终注入 system prompt，与 Agent 角色、Skills 无关。
 */
const BUILTIN_MESSAGING_PROMPT = `## 消息回复与输出原则

**核心原则：你的文字输出（推理过程、思考内容）对用户完全不可见。用户只能收到你通过 \`send_channel_message\` 工具主动发送的消息。**

所有需要传达给用户的内容，都必须通过 \`send_channel_message\` 工具发送，而非直接输出文字。

**参数：**
- \`content\`（必填）：要发送的消息内容
- \`messageType\`（可选）：消息类型，默认 text；支持 text / image / file / rich_text
- \`channel\` / \`channelUserId\` / \`channelConversationId\`：默认取自消息来源，通常不需要手动填写

**使用原则：**
- 需要回复用户时，调用此工具发送，不要依赖直接文字输出。
- 可以分多次调用，不必等所有工作完成后才回复。
- 当用户的问题需要较长处理时间时，先发一条确认消息，再继续处理。

**关于历史消息的理解：**
历史消息只包含客观事件记录，不包含你过去的推理过程：
- \`user\` 消息 = 某个用户发来的内容。消息前的 \`[名字]\` 标记表示发送者身份（群聊场景有多个不同用户）
- \`assistant\` 消息（tool_use block）= 你过去执行的工具调用动作
- \`tool_result\` block = 工具执行返回的客观结果
- 你过去通过 \`send_channel_message\` 发送的内容会出现在对应的 tool_use 和 tool_result 中`;

// ==================== System Prompt ====================

/**
 * 构建 systemPrompt
 *
 * 由三部分组成：
 * 1. Agent 配置的 systemPrompt（角色定义、行为准则）
 * 2. 内置能力说明（消息回复等通用能力，所有 Agent 共享）
 * 3. Skills 上下文（能力描述，按 Agent 配置动态加载）
 *
 * 不注入记忆数据、渠道上下文、工具描述等运行时信息。
 * 这些能力由 skills 描述，agent 自主决定何时调用。
 */
export function buildSystemPrompt(
  agentSystemPrompt?: string,
  skillsAddition?: string,
): string {
  const parts: string[] = [];

  if (agentSystemPrompt) {
    parts.push(agentSystemPrompt);
  }

  // 内置能力说明（始终注入）
  parts.push(BUILTIN_MESSAGING_PROMPT);

  if (skillsAddition) {
    parts.push(skillsAddition);
  }

  return parts.join("\n\n");
}

