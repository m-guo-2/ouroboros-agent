export interface Model {
  id: string;
  name: string;
  provider: "claude" | "openai" | "kimi" | "glm";
  enabled: boolean;
  configured: boolean;
  model: string;
  maxTokens: number;
  temperature: number;
  baseUrl?: string;
  hasApiKey?: boolean;
}

export interface Message {
  role: "user" | "assistant";
  content: string;
}

export interface Conversation {
  id: string;
  title: string;
  modelId: string;
  messageCount?: number;
  messages?: Message[];
  createdAt: string;
  updatedAt: string;
}

export interface ApiResponse<T> {
  success: boolean;
  data?: T;
  error?: string;
}

export interface StreamEvent {
  type: "text" | "tool_use" | "done" | "error";
  content?: string;
  name?: string;
  input?: Record<string, unknown>;
  id?: string;
  message?: string;
}
