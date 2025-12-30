import OpenAI from "openai";
import { BaseModelAdapter, ChatMessage, StreamChunk, ToolDefinition } from "./base";
import type { ModelConfig } from "../../config";

/**
 * GLM (智谱清言) 适配器
 * 智谱 API 使用 OpenAI 兼容格式
 */
export class GLMAdapter extends BaseModelAdapter {
  private client: OpenAI;

  constructor(config: ModelConfig) {
    super(config);
    this.client = new OpenAI({
      apiKey: config.apiKey,
      baseURL: config.baseUrl || "https://open.bigmodel.cn/api/paas/v4",
    });
  }

  private convertMessages(messages: ChatMessage[]): OpenAI.ChatCompletionMessageParam[] {
    return messages.map((m) => ({
      role: m.role,
      content: m.content,
    }));
  }

  private convertTools(tools?: ToolDefinition[]): OpenAI.ChatCompletionTool[] | undefined {
    if (!tools || tools.length === 0) return undefined;
    return tools.map((t) => ({
      type: "function" as const,
      function: {
        name: t.name,
        description: t.description,
        parameters: t.parameters,
      },
    }));
  }

  async chat(
    messages: ChatMessage[],
    tools?: ToolDefinition[],
    onChunk?: (chunk: StreamChunk) => void
  ): Promise<string> {
    const response = await this.client.chat.completions.create({
      model: this.config.model,
      max_tokens: this.config.maxTokens,
      temperature: this.config.temperature,
      messages: this.convertMessages(messages),
      tools: this.convertTools(tools),
    });

    const choice = response.choices[0];
    const content = choice.message.content || "";

    if (content) {
      onChunk?.({ type: "text", content });
    }

    if (choice.message.tool_calls) {
      for (const toolCall of choice.message.tool_calls) {
        onChunk?.({
          type: "tool_use",
          toolName: toolCall.function.name,
          toolInput: JSON.parse(toolCall.function.arguments || "{}"),
          toolId: toolCall.id,
        });
      }
    }

    onChunk?.({ type: "done" });
    return content;
  }

  async stream(
    messages: ChatMessage[],
    tools?: ToolDefinition[],
    onChunk: (chunk: StreamChunk) => void
  ): Promise<void> {
    const stream = await this.client.chat.completions.create({
      model: this.config.model,
      max_tokens: this.config.maxTokens,
      temperature: this.config.temperature,
      messages: this.convertMessages(messages),
      tools: this.convertTools(tools),
      stream: true,
    });

    const toolCalls: Map<number, { id: string; name: string; arguments: string }> = new Map();

    for await (const chunk of stream) {
      const delta = chunk.choices[0]?.delta;

      if (delta?.content) {
        onChunk({ type: "text", content: delta.content });
      }

      if (delta?.tool_calls) {
        for (const tc of delta.tool_calls) {
          if (!toolCalls.has(tc.index)) {
            toolCalls.set(tc.index, {
              id: tc.id || "",
              name: tc.function?.name || "",
              arguments: "",
            });
          }
          const existing = toolCalls.get(tc.index)!;
          if (tc.id) existing.id = tc.id;
          if (tc.function?.name) existing.name = tc.function.name;
          if (tc.function?.arguments) existing.arguments += tc.function.arguments;
        }
      }

      if (chunk.choices[0]?.finish_reason === "tool_calls") {
        for (const [, tc] of toolCalls) {
          onChunk({
            type: "tool_use",
            toolName: tc.name,
            toolInput: JSON.parse(tc.arguments || "{}"),
            toolId: tc.id,
          });
        }
      }
    }

    onChunk({ type: "done" });
  }
}
