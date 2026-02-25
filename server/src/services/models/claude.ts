import Anthropic from "@anthropic-ai/sdk";
import { BaseModelAdapter, ChatMessage, StreamChunk, ToolDefinition } from "./base";
import type { ModelConfig } from "../../config";

export class ClaudeAdapter extends BaseModelAdapter {
  private client: Anthropic;

  constructor(config: ModelConfig) {
    super(config);
    this.client = new Anthropic({
      apiKey: config.apiKey,
    });
  }

  private convertMessages(messages: ChatMessage[]): Anthropic.MessageParam[] {
    return messages
      .filter((m) => m.role !== "system")
      .map((m) => ({
        role: m.role as "user" | "assistant",
        content: m.content,
      }));
  }

  private convertTools(tools?: ToolDefinition[]): Anthropic.Tool[] | undefined {
    if (!tools || tools.length === 0) return undefined;
    return tools.map((t) => ({
      name: t.name,
      description: t.description,
      input_schema: t.parameters as Anthropic.Tool["input_schema"],
    }));
  }

  async chat(
    messages: ChatMessage[],
    tools?: ToolDefinition[],
    onChunk?: (chunk: StreamChunk) => void
  ): Promise<string> {
    const systemMessage = messages.find((m) => m.role === "system");
    
    const response = await this.client.messages.create({
      model: this.config.model,
      max_tokens: this.config.maxTokens,
      temperature: this.config.temperature,
      system: systemMessage?.content,
      messages: this.convertMessages(messages),
      tools: this.convertTools(tools),
    });

    let result = "";
    for (const block of response.content) {
      if (block.type === "text") {
        result += block.text;
        onChunk?.({ type: "text", content: block.text });
      } else if (block.type === "tool_use") {
        onChunk?.({
          type: "tool_use",
          toolName: block.name,
          toolInput: block.input as Record<string, unknown>,
          toolId: block.id,
        });
      }
    }

    onChunk?.({ type: "done" });
    return result;
  }

  async stream(
    messages: ChatMessage[],
    tools?: ToolDefinition[],
    onChunk: (chunk: StreamChunk) => void
  ): Promise<void> {
    const systemMessage = messages.find((m) => m.role === "system");

    const stream = this.client.messages.stream({
      model: this.config.model,
      max_tokens: this.config.maxTokens,
      temperature: this.config.temperature,
      system: systemMessage?.content,
      messages: this.convertMessages(messages),
      tools: this.convertTools(tools),
    });

    let currentToolUse: { id: string; name: string; input: string } | null = null;

    for await (const event of stream) {
      if (event.type === "content_block_start") {
        if (event.content_block.type === "tool_use") {
          currentToolUse = {
            id: event.content_block.id,
            name: event.content_block.name,
            input: "",
          };
        }
      } else if (event.type === "content_block_delta") {
        if (event.delta.type === "text_delta") {
          onChunk({ type: "text", content: event.delta.text });
        } else if (event.delta.type === "input_json_delta" && currentToolUse) {
          currentToolUse.input += event.delta.partial_json;
        }
      } else if (event.type === "content_block_stop" && currentToolUse) {
        try {
          const toolInput = JSON.parse(currentToolUse.input || "{}");
          onChunk({
            type: "tool_use",
            toolName: currentToolUse.name,
            toolInput,
            toolId: currentToolUse.id,
          });
        } catch {
          // JSON 解析失败，忽略
        }
        currentToolUse = null;
      } else if (event.type === "message_stop") {
        onChunk({ type: "done" });
      }
    }
  }
}
