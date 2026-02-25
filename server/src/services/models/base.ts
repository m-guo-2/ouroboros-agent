import type { ModelConfig } from "../../config";

export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: string;
}

export interface StreamChunk {
  type: "text" | "tool_use" | "tool_result" | "thinking" | "error" | "done";
  content?: string;
  toolName?: string;
  toolInput?: Record<string, unknown>;
  toolId?: string;
}

export interface ToolDefinition {
  name: string;
  description: string;
  parameters: {
    type: "object";
    properties: Record<string, unknown>;
    required?: string[];
  };
}

export abstract class BaseModelAdapter {
  protected config: ModelConfig;

  constructor(config: ModelConfig) {
    this.config = config;
  }

  abstract chat(
    messages: ChatMessage[],
    tools?: ToolDefinition[],
    onChunk?: (chunk: StreamChunk) => void
  ): Promise<string>;

  abstract stream(
    messages: ChatMessage[],
    tools?: ToolDefinition[],
    onChunk: (chunk: StreamChunk) => void
  ): Promise<void>;

  getConfig(): ModelConfig {
    return this.config;
  }

  updateConfig(updates: Partial<ModelConfig>): void {
    this.config = { ...this.config, ...updates };
  }
}
