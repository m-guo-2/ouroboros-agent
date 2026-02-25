/**
 * LLM Client 实现
 *
 * 提供两种 LLM 调用方式：
 *   1. AnthropicClient: 直接调用 Anthropic Messages API（原生格式，零转换）
 *   2. OpenAICompatibleClient: 调用 OpenAI 兼容 API，做格式转换
 *
 * 两者都实现 LLMClient 接口，对 Agent Loop 完全透明。
 */

import type {
  LLMClient,
  LLMResponse,
  AgentMessage,
  ToolDefinition,
  ContentBlock,
} from "./types";

// ==================== Helpers ====================

/**
 * 从消息文本中提取 `[senderName]` 前缀。
 * dbMessagesToAgentMessages() 将群聊身份以 `[张三] 内容` 格式嵌入 Anthropic 消息的 content 中。
 * OpenAI 兼容 API 原生支持 `name` 字段，所以在转换时将前缀提取出来。
 */
function extractSenderName(text: string): { name: string | undefined; text: string } {
  const match = text.match(/^\[([^\]]+)\]\s*/);
  if (match) {
    return { name: match[1], text: text.slice(match[0].length) };
  }
  return { name: undefined, text };
}

// ==================== Anthropic Native Client ====================

export interface AnthropicClientConfig {
  apiKey: string;
  baseUrl?: string;
  maxTokens?: number;
}

export class AnthropicClient implements LLMClient {
  private apiKey: string;
  private baseUrl: string;
  private maxTokens: number;

  constructor(config: AnthropicClientConfig) {
    this.apiKey = config.apiKey;
    this.baseUrl = (config.baseUrl || "https://api.anthropic.com").replace(/\/$/, "");
    this.maxTokens = config.maxTokens || 8192;
  }

  async chat(params: {
    messages: AgentMessage[];
    tools: ToolDefinition[];
    systemPrompt: string;
    model?: string;
    signal?: AbortSignal;
  }): Promise<LLMResponse> {
    const body: Record<string, unknown> = {
      model: params.model || "claude-sonnet-4-20250514",
      max_tokens: this.maxTokens,
      system: params.systemPrompt,
      messages: params.messages,
    };

    if (params.tools.length > 0) {
      body.tools = params.tools;
    }

    const response = await fetch(`${this.baseUrl}/v1/messages`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "x-api-key": this.apiKey,
        "anthropic-version": "2023-06-01",
      },
      body: JSON.stringify(body),
      signal: params.signal,
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Anthropic API ${response.status}: ${errorText}`);
    }

    const data = await response.json() as {
      content: ContentBlock[];
      stop_reason: string;
      usage: { input_tokens: number; output_tokens: number };
    };

    return {
      content: data.content,
      stopReason: data.stop_reason,
      usage: {
        inputTokens: data.usage.input_tokens,
        outputTokens: data.usage.output_tokens,
      },
    };
  }
}

// ==================== OpenAI Compatible Client ====================

export interface OpenAICompatibleClientConfig {
  apiKey: string;
  baseUrl: string;
  maxTokens?: number;
}

export class OpenAICompatibleClient implements LLMClient {
  private apiKey: string;
  private baseUrl: string;
  private maxTokens: number;

  constructor(config: OpenAICompatibleClientConfig) {
    this.apiKey = config.apiKey;
    this.baseUrl = config.baseUrl.replace(/\/$/, "");
    this.maxTokens = config.maxTokens || 8192;
  }

  async chat(params: {
    messages: AgentMessage[];
    tools: ToolDefinition[];
    systemPrompt: string;
    model?: string;
    signal?: AbortSignal;
  }): Promise<LLMResponse> {
    // ── 转换消息格式：Anthropic → OpenAI ──
    const openAIMessages = this.convertMessages(params.messages, params.systemPrompt);

    const body: Record<string, unknown> = {
      model: params.model || "gpt-4",
      messages: openAIMessages,
      max_tokens: this.maxTokens,
    };

    if (params.tools.length > 0) {
      body.tools = params.tools.map((t) => ({
        type: "function",
        function: {
          name: t.name,
          description: t.description,
          parameters: t.input_schema,
        },
      }));
    }

    const response = await fetch(`${this.baseUrl}/v1/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${this.apiKey}`,
      },
      body: JSON.stringify(body),
      signal: params.signal,
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`OpenAI API ${response.status}: ${errorText}`);
    }

    const data = await response.json() as {
      choices: Array<{
        message: {
          content?: string;
          tool_calls?: Array<{
            id: string;
            function: { name: string; arguments: string };
          }>;
        };
        finish_reason: string;
      }>;
      usage: { prompt_tokens: number; completion_tokens: number };
    };

    // ── 转换响应格式：OpenAI → Anthropic ──
    return this.convertResponse(data);
  }

  /**
   * Anthropic 内部格式 → OpenAI 兼容格式。
   *
   * 转换规则：
   *   - user + string content → role:"user", 提取 [senderName] 前缀到 name 字段
   *   - user + tool_result blocks → role:"tool" (每个 block 一条消息)
   *   - user + mixed (text + tool_result) → 分别拆为 user 和 tool 消息
   *   - assistant → 提取 tool_use 为 tool_calls，text 为 content
   */
  private convertMessages(
    messages: AgentMessage[],
    systemPrompt: string,
  ): Array<Record<string, unknown>> {
    const result: Array<Record<string, unknown>> = [];

    if (systemPrompt) {
      result.push({ role: "system", content: systemPrompt });
    }

    for (const msg of messages) {
      if (typeof msg.content === "string") {
        // 纯文本 user 消息：提取 [senderName] 前缀作为 name 字段（OpenAI 原生支持）
        if (msg.role === "user") {
          const { name, text } = extractSenderName(msg.content);
          const userMsg: Record<string, unknown> = { role: "user", content: text };
          if (name) userMsg.name = name;
          result.push(userMsg);
        } else {
          result.push({ role: msg.role, content: msg.content });
        }
        continue;
      }

      if (msg.role === "user") {
        for (const block of msg.content) {
          if (block.type === "tool_result") {
            result.push({
              role: "tool",
              tool_call_id: block.tool_use_id,
              content: typeof block.content === "string"
                ? block.content
                : JSON.stringify(block.content),
            });
          } else if (block.type === "text") {
            const { name, text } = extractSenderName(block.text);
            const userMsg: Record<string, unknown> = { role: "user", content: text };
            if (name) userMsg.name = name;
            result.push(userMsg);
          }
        }
      } else if (msg.role === "assistant") {
        const textParts: string[] = [];
        const toolCalls: Array<Record<string, unknown>> = [];

        for (const block of msg.content) {
          if (block.type === "text") {
            textParts.push(block.text);
          } else if (block.type === "tool_use") {
            toolCalls.push({
              id: block.id,
              type: "function",
              function: {
                name: block.name,
                arguments: JSON.stringify(block.input),
              },
            });
          }
        }

        const assistantMsg: Record<string, unknown> = {
          role: "assistant",
          content: textParts.length > 0 ? textParts.join("\n") : null,
        };
        if (toolCalls.length > 0) {
          assistantMsg.tool_calls = toolCalls;
        }
        result.push(assistantMsg);
      }
    }

    return result;
  }

  private convertResponse(data: {
    choices: Array<{
      message: {
        content?: string;
        tool_calls?: Array<{
          id: string;
          function: { name: string; arguments: string };
        }>;
      };
      finish_reason: string;
    }>;
    usage: { prompt_tokens: number; completion_tokens: number };
  }): LLMResponse {
    const choice = data.choices[0];
    const content: ContentBlock[] = [];

    if (choice?.message?.content) {
      content.push({ type: "text", text: choice.message.content });
    }

    if (choice?.message?.tool_calls) {
      for (const tc of choice.message.tool_calls) {
        let input: Record<string, unknown> = {};
        try {
          input = JSON.parse(tc.function.arguments);
        } catch {
          input = { raw: tc.function.arguments };
        }
        content.push({
          type: "tool_use",
          id: tc.id,
          name: tc.function.name,
          input,
        });
      }
    }

    if (content.length === 0) {
      content.push({ type: "text", text: "" });
    }

    const stopMap: Record<string, string> = {
      stop: "end_turn",
      tool_calls: "tool_use",
      length: "max_tokens",
    };

    return {
      content,
      stopReason: stopMap[choice?.finish_reason] || "end_turn",
      usage: {
        inputTokens: data.usage?.prompt_tokens || 0,
        outputTokens: data.usage?.completion_tokens || 0,
      },
    };
  }
}
