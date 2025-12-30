import type { Model, Conversation, ApiResponse, StreamEvent } from "./types";

const API_BASE = "/api";

async function fetchApi<T>(
  path: string,
  options?: RequestInit
): Promise<ApiResponse<T>> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: "Request failed" }));
    return { success: false, error: error.error || response.statusText };
  }

  return response.json();
}

// 可用模型类型
export interface AvailableModel {
  id: string;
  name: string;
  provider: string;
  contextLength?: number;
  description?: string;
}

// 模型相关 API
export const modelsApi = {
  getAll: () => fetchApi<Model[]>("/models"),

  getEnabled: () => fetchApi<Model[]>("/models/enabled"),

  getById: (id: string) => fetchApi<Model>(`/models/${id}`),

  update: (id: string, data: Partial<Model> & { apiKey?: string }) =>
    fetchApi<Model>(`/models/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),

  // 获取某个 provider 可用的模型列表
  getAvailableModels: (id: string) =>
    fetchApi<AvailableModel[]>(`/models/${id}/available-models`),
};

// 会话相关 API
export const conversationsApi = {
  getAll: () => fetchApi<Conversation[]>("/conversations"),

  create: (modelId: string, title?: string) =>
    fetchApi<Conversation>("/conversations", {
      method: "POST",
      body: JSON.stringify({ modelId, title }),
    }),

  getById: (id: string) => fetchApi<Conversation>(`/conversations/${id}`),

  update: (id: string, title: string) =>
    fetchApi<Conversation>(`/conversations/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ title }),
    }),

  delete: (id: string) =>
    fetchApi<void>(`/conversations/${id}`, { method: "DELETE" }),

  switchModel: (id: string, modelId: string) =>
    fetchApi<{ modelId: string }>(`/conversations/${id}/model`, {
      method: "POST",
      body: JSON.stringify({ modelId }),
    }),
};

// 流式聊天 API
export async function streamChat(
  conversationId: string,
  message: string,
  onEvent: (event: StreamEvent) => void,
  onError?: (error: Error) => void
): Promise<void> {
  try {
    const response = await fetch(`${API_BASE}/conversations/${conversationId}/chat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message }),
    });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const reader = response.body?.getReader();
    if (!reader) {
      throw new Error("No response body");
    }

    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      let eventType = "";
      for (const line of lines) {
        if (line.startsWith("event: ")) {
          eventType = line.slice(7);
        } else if (line.startsWith("data: ")) {
          try {
            const data = JSON.parse(line.slice(6));
            onEvent({ type: eventType as StreamEvent["type"], ...data });
          } catch {
            // 忽略解析错误
          }
        }
      }
    }
  } catch (error) {
    onError?.(error instanceof Error ? error : new Error(String(error)));
  }
}
