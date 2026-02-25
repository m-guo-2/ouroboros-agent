/**
 * ReAct Agent Loop — 核心执行循环
 *
 * 纯手写的 while 循环，100% 透明，每一步都可观测。
 *
 * 流程：
 *   1. 将 messages + tools 发给 LLM
 *   2. LLM 返回 text（思考）和/或 tool_use（调用工具）
 *   3. 如果有 tool_use → 执行工具 → 将结果追加到 messages → 继续循环
 *   4. 如果只有 text 且 stopReason=end_turn → 结束循环
 *
 * 设计原则：
 *   - 引擎不持有状态，每次调用都是无副作用的纯函数
 *   - 可观测性通过 onEvent 回调暴露，引擎不关心日志存到哪里
 *   - 工具执行通过 ToolRegistry 路由，引擎不关心工具怎么实现
 */

import type {
  AgentLoopConfig,
  AgentLoopResult,
  AgentMessage,
  ContentBlock,
  TextBlock,
  ToolUseBlock,
  TokenUsage,
  ToolDefinition,
  LLMResponse,
} from "./types";

const DEFAULT_MAX_ITERATIONS = 25;

// ==================== 模型 I/O 摘要构建 ====================

function buildModelInputSummary(
  systemPrompt: string,
  messages: AgentMessage[],
  tools: ToolDefinition[],
  model?: string,
): unknown {
  return {
    model: model || "unknown",
    systemPromptPreview: systemPrompt.substring(0, 500),
    messageCount: messages.length,
    messages: messages.map((m) => {
      const content = typeof m.content === "string"
        ? m.content.substring(0, 300)
        : m.content.map((b) => {
            if (b.type === "text") return `[text] ${b.text.substring(0, 200)}`;
            if (b.type === "tool_use") return `[tool_use:${b.name}]`;
            if (b.type === "tool_result") return `[tool_result:${b.tool_use_id}] ${b.content.substring(0, 150)}`;
            return "[unknown]";
          }).join(" | ").substring(0, 400);
      return { role: m.role, content };
    }),
    toolNames: tools.map((t) => t.name),
  };
}

function buildModelOutputSummary(response: LLMResponse): unknown {
  const textContent = response.content
    .filter((b): b is Extract<ContentBlock, { type: "text" }> => b.type === "text")
    .map((b) => b.text)
    .join("")
    .substring(0, 1500);
  const toolCalls = response.content
    .filter((b): b is Extract<ContentBlock, { type: "tool_use" }> => b.type === "tool_use")
    .map((b) => ({
      name: b.name,
      id: b.id,
      inputPreview: JSON.stringify(b.input).substring(0, 200),
    }));
  return {
    content: textContent,
    toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
    stopReason: response.stopReason,
    inputTokens: response.usage.inputTokens,
    outputTokens: response.usage.outputTokens,
  };
}

export async function runAgentLoop(config: AgentLoopConfig): Promise<AgentLoopResult> {
  const {
    llmClient,
    systemPrompt,
    tools,
    onEvent,
    signal,
    model,
    maxIterations = DEFAULT_MAX_ITERATIONS,
  } = config;

  const messages: AgentMessage[] = [...config.messages];
  const toolMap = new Map(tools.map((t) => [t.definition.name, t]));
  const toolDefinitions = tools.map((t) => t.definition);

  const cumulativeUsage: TokenUsage = {
    inputTokens: 0,
    outputTokens: 0,
    totalCostUsd: 0,
  };

  let iteration = 0;
  let finalText: string | undefined;
  let hitMaxIterations = false;

  while (iteration < maxIterations) {
    if (signal?.aborted) {
      onEvent({
        type: "error",
        timestamp: Date.now(),
        iteration,
        error: "Agent loop aborted by signal",
      });
      break;
    }

    iteration++;

    // ── Step 1: 调用 LLM ──
    let response;
    try {
      response = await llmClient.chat({
        messages,
        tools: toolDefinitions,
        systemPrompt,
        model,
        signal,
      });
    } catch (err) {
      const error = err instanceof Error ? err.message : String(err);
      onEvent({ type: "error", timestamp: Date.now(), iteration, error });
      break;
    }

    // 上报模型 I/O 摘要（每次 LLM 调用后）
    onEvent({
      type: "model_io",
      timestamp: Date.now(),
      iteration,
      modelInput: buildModelInputSummary(systemPrompt, messages, toolDefinitions, model),
      modelOutput: buildModelOutputSummary(response),
    });

    // 累加 token 使用
    cumulativeUsage.inputTokens += response.usage.inputTokens;
    cumulativeUsage.outputTokens += response.usage.outputTokens;

    // ── Step 2: 解析响应内容 ──
    const textBlocks: TextBlock[] = [];
    const toolUseBlocks: ToolUseBlock[] = [];

    for (const block of response.content) {
      if (block.type === "text") {
        textBlocks.push(block);
      } else if (block.type === "tool_use") {
        toolUseBlocks.push(block);
      }
    }

    // 上报 thinking（模型的文本推理）
    for (const block of textBlocks) {
      if (block.text.trim()) {
        onEvent({
          type: "thinking",
          timestamp: Date.now(),
          iteration,
          thinking: block.text,
          source: "model",
        });
      }
    }

    // ── Step 3: 如果没有工具调用，循环结束 ──
    if (toolUseBlocks.length === 0) {
      finalText = textBlocks.map((b) => b.text).join("\n");
      break;
    }

    // 将 assistant 消息追加到历史（只保留 tool_use blocks，剥离文本推理）
    // 设计原则：消息历史只记录客观动作，不记录模型推理过程
    messages.push({
      role: "assistant",
      content: toolUseBlocks,
    });

    // ── Step 4: 执行工具调用 ──
    const toolResults: ContentBlock[] = [];

    for (const toolUse of toolUseBlocks) {
      // 上报 tool_call
      onEvent({
        type: "tool_call",
        timestamp: Date.now(),
        iteration,
        toolCallId: toolUse.id,
        toolName: toolUse.name,
        toolInput: toolUse.input,
      });

      const startedAt = Date.now();
      const registeredTool = toolMap.get(toolUse.name);

      if (!registeredTool) {
        const errorMsg = `Tool not found: ${toolUse.name}. Available tools: ${Array.from(toolMap.keys()).join(", ")}`;
        onEvent({
          type: "tool_result",
          timestamp: Date.now(),
          iteration,
          toolCallId: toolUse.id,
          toolName: toolUse.name,
          toolResult: errorMsg,
          toolDuration: Date.now() - startedAt,
          toolSuccess: false,
        });
        toolResults.push({
          type: "tool_result",
          tool_use_id: toolUse.id,
          content: errorMsg,
          is_error: true,
        });
        continue;
      }

      try {
        const result = await registeredTool.execute(toolUse.input);
        const resultStr = typeof result === "string"
          ? result
          : JSON.stringify(result, null, 2);

        onEvent({
          type: "tool_result",
          timestamp: Date.now(),
          iteration,
          toolCallId: toolUse.id,
          toolName: toolUse.name,
          toolResult: result,
          toolDuration: Date.now() - startedAt,
          toolSuccess: true,
        });

        toolResults.push({
          type: "tool_result",
          tool_use_id: toolUse.id,
          content: resultStr,
        });
      } catch (err) {
        const errorMsg = err instanceof Error ? err.message : String(err);

        onEvent({
          type: "tool_result",
          timestamp: Date.now(),
          iteration,
          toolCallId: toolUse.id,
          toolName: toolUse.name,
          toolResult: errorMsg,
          toolDuration: Date.now() - startedAt,
          toolSuccess: false,
        });

        toolResults.push({
          type: "tool_result",
          tool_use_id: toolUse.id,
          content: errorMsg,
          is_error: true,
        });
      }
    }

    // ── Step 5: 将工具结果追加到消息，进入下一轮 ──
    const toolResultMessage: AgentMessage = { role: "user", content: toolResults };
    messages.push(toolResultMessage);

    // ── Step 6: 增量持久化本轮 tool_use + tool_result ──
    if (config.onNewMessages) {
      try {
        await config.onNewMessages([
          { role: "assistant", content: toolUseBlocks },
          toolResultMessage,
        ]);
      } catch (err) {
        onEvent({
          type: "error",
          timestamp: Date.now(),
          iteration,
          error: `Failed to persist iteration messages: ${err instanceof Error ? err.message : String(err)}`,
          source: "system",
        });
      }
    }

    // 检查是否达到最大迭代次数
    if (iteration >= maxIterations) {
      hitMaxIterations = true;
      onEvent({
        type: "error",
        timestamp: Date.now(),
        iteration,
        error: `Reached max iterations (${maxIterations}), stopping loop`,
        source: "system",
      });
    }
  }

  // ── 上报完成 ──
  onEvent({
    type: "done",
    timestamp: Date.now(),
    usage: cumulativeUsage,
  });

  return {
    finalText,
    messages,
    usage: cumulativeUsage,
    hitMaxIterations,
  };
}
