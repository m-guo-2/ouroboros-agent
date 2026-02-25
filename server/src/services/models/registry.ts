import OpenAI from "openai";

export interface AvailableModel {
  id: string;
  name: string;
  provider: string;
  contextLength?: number;
  description?: string;
}

/**
 * 获取 OpenAI 可用模型列表
 */
export async function fetchOpenAIModels(
  apiKey: string,
  baseUrl?: string
): Promise<AvailableModel[]> {
  try {
    const client = new OpenAI({
      apiKey,
      baseURL: baseUrl || "https://api.openai.com/v1",
    });

    const response = await client.models.list();
    const models: AvailableModel[] = [];

    for await (const model of response) {
      // 过滤出聊天模型
      if (
        model.id.includes("gpt") ||
        model.id.includes("o1") ||
        model.id.includes("o3")
      ) {
        models.push({
          id: model.id,
          name: model.id,
          provider: "openai",
        });
      }
    }

    // 按名称排序，优先显示最新模型
    return models.sort((a, b) => {
      // gpt-4o 优先
      if (a.id.includes("gpt-4o") && !b.id.includes("gpt-4o")) return -1;
      if (!a.id.includes("gpt-4o") && b.id.includes("gpt-4o")) return 1;
      return a.id.localeCompare(b.id);
    });
  } catch (error) {
    console.error("Failed to fetch OpenAI models:", error);
    return [];
  }
}

/**
 * 获取 Anthropic (Claude) 可用模型列表
 * 使用 Anthropic Models API: GET /v1/models
 */
export async function fetchClaudeModels(
  apiKey: string,
  baseUrl?: string
): Promise<AvailableModel[]> {
  try {
    const base = (baseUrl || "https://api.anthropic.com").replace(/\/+$/, "");
    const response = await fetch(`${base}/v1/models?limit=100`, {
      headers: {
        "x-api-key": apiKey,
        "anthropic-version": "2023-06-01",
      },
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    const data = await response.json() as {
      data: Array<{ id: string; display_name: string; created_at: string }>;
    };

    if (!data.data || !Array.isArray(data.data)) {
      throw new Error("Unexpected response format");
    }

    return data.data
      .filter((m) => m.id.includes("claude"))
      .map((m) => ({
        id: m.id,
        name: m.display_name || m.id,
        provider: "claude",
      }))
      .sort((a, b) => {
        // 最新模型优先
        if (a.id.includes("claude-sonnet-4") && !b.id.includes("claude-sonnet-4")) return -1;
        if (!a.id.includes("claude-sonnet-4") && b.id.includes("claude-sonnet-4")) return 1;
        return a.name.localeCompare(b.name);
      });
  } catch (error) {
    console.error("Failed to fetch Claude models:", error);
    // 兜底：返回预定义的已知模型列表
    return [
      { id: "claude-sonnet-4-20250514", name: "Claude Sonnet 4", provider: "claude" },
      { id: "claude-3-5-sonnet-20241022", name: "Claude 3.5 Sonnet", provider: "claude" },
      { id: "claude-3-5-haiku-20241022", name: "Claude 3.5 Haiku", provider: "claude" },
      { id: "claude-3-opus-20240229", name: "Claude 3 Opus", provider: "claude" },
    ];
  }
}

/**
 * 获取 Kimi (Moonshot) 可用模型列表
 * Moonshot 兼容 OpenAI 格式，但使用 fetch 直接调用以避免 SDK 分页兼容问题
 * 官方文档: https://platform.moonshot.ai/docs/api/chat#list-models
 * 接口: GET {baseUrl}/models
 */
export async function fetchKimiModels(
  apiKey: string,
  baseUrl?: string
): Promise<AvailableModel[]> {
  const base = (baseUrl || "https://api.moonshot.cn/v1").replace(/\/+$/, "");
  try {
    const response = await fetch(`${base}/models`, {
      headers: {
        Authorization: `Bearer ${apiKey}`,
      },
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    const data = (await response.json()) as {
      data: Array<{ id: string; object?: string; created?: number; owned_by?: string }>;
      object?: string;
    };

    if (!data.data || !Array.isArray(data.data)) {
      throw new Error(`Unexpected response format: ${JSON.stringify(data).substring(0, 200)}`);
    }

    const models: AvailableModel[] = data.data.map((m) => ({
      id: m.id,
      name: formatKimiModelName(m.id),
      provider: "kimi",
    }));

    // 按模型系列排序：kimi-k2 系列优先，然后 moonshot-v1 系列
    return models.sort((a, b) => {
      const aIsK2 = a.id.startsWith("kimi-k2");
      const bIsK2 = b.id.startsWith("kimi-k2");
      if (aIsK2 && !bIsK2) return -1;
      if (!aIsK2 && bIsK2) return 1;
      return a.id.localeCompare(b.id);
    });
  } catch (error) {
    console.error("Failed to fetch Kimi models:", error);
    // 兜底：返回官方文档中列出的已知模型
    return [
      { id: "kimi-k2.5", name: "Kimi K2.5", provider: "kimi" },
      { id: "kimi-k2-turbo-preview", name: "Kimi K2 Turbo (Preview)", provider: "kimi" },
      { id: "kimi-k2-thinking", name: "Kimi K2 Thinking", provider: "kimi" },
      { id: "kimi-k2-thinking-turbo", name: "Kimi K2 Thinking Turbo", provider: "kimi" },
      { id: "kimi-k2-0905-preview", name: "Kimi K2 0905 (Preview)", provider: "kimi" },
      { id: "moonshot-v1-auto", name: "Moonshot V1 (Auto)", provider: "kimi" },
      { id: "moonshot-v1-8k", name: "Moonshot V1 (8K)", provider: "kimi" },
      { id: "moonshot-v1-32k", name: "Moonshot V1 (32K)", provider: "kimi" },
      { id: "moonshot-v1-128k", name: "Moonshot V1 (128K)", provider: "kimi" },
    ];
  }
}

function formatKimiModelName(id: string): string {
  const map: Record<string, string> = {
    "kimi-k2.5": "Kimi K2.5",
    "kimi-k2-0905-preview": "Kimi K2 0905 (Preview)",
    "kimi-k2-0711-preview": "Kimi K2 0711 (Preview)",
    "kimi-k2-turbo-preview": "Kimi K2 Turbo (Preview)",
    "kimi-k2-thinking-turbo": "Kimi K2 Thinking Turbo",
    "kimi-k2-thinking": "Kimi K2 Thinking",
    "moonshot-v1-auto": "Moonshot V1 (Auto)",
    "moonshot-v1-8k": "Moonshot V1 (8K)",
    "moonshot-v1-32k": "Moonshot V1 (32K)",
    "moonshot-v1-128k": "Moonshot V1 (128K)",
    "moonshot-v1-8k-vision-preview": "Moonshot V1 8K Vision (Preview)",
    "moonshot-v1-32k-vision-preview": "Moonshot V1 32K Vision (Preview)",
    "moonshot-v1-128k-vision-preview": "Moonshot V1 128K Vision (Preview)",
  };
  return map[id] || id;
}

/**
 * 获取 GLM (智谱) 可用模型列表
 * 智谱 API 兼容 OpenAI 格式
 */
export async function fetchGLMModels(apiKey: string): Promise<AvailableModel[]> {
  try {
    // 智谱的 models.list 可能不可用，使用预定义列表但验证 API Key
    const client = new OpenAI({
      apiKey,
      baseURL: "https://open.bigmodel.cn/api/paas/v4",
    });

    // 尝试获取模型列表
    try {
      const response = await client.models.list();
      const models: AvailableModel[] = [];

      for await (const model of response) {
        models.push({
          id: model.id,
          name: formatGLMModelName(model.id),
          provider: "glm",
        });
      }

      if (models.length > 0) {
        return models;
      }
    } catch {
      // models.list 不可用，继续使用预定义列表
    }

    // 返回已知的智谱模型列表
    return [
      { id: "glm-4-plus", name: "GLM-4 Plus", provider: "glm" },
      { id: "glm-4", name: "GLM-4", provider: "glm" },
      { id: "glm-4-long", name: "GLM-4 Long", provider: "glm" },
      { id: "glm-4-flash", name: "GLM-4 Flash", provider: "glm" },
      { id: "glm-4v-plus", name: "GLM-4V Plus (视觉)", provider: "glm" },
      { id: "glm-4v", name: "GLM-4V (视觉)", provider: "glm" },
    ];
  } catch (error) {
    console.error("Failed to fetch GLM models:", error);
    return [];
  }
}

function formatGLMModelName(id: string): string {
  const map: Record<string, string> = {
    "glm-4": "GLM-4",
    "glm-4-plus": "GLM-4 Plus",
    "glm-4-long": "GLM-4 Long",
    "glm-4-flash": "GLM-4 Flash",
    "glm-4v": "GLM-4V (视觉)",
    "glm-4v-plus": "GLM-4V Plus (视觉)",
  };
  return map[id] || id;
}

/**
 * 根据 provider 获取可用模型列表
 */
export async function fetchAvailableModels(
  provider: string,
  apiKey: string,
  baseUrl?: string
): Promise<AvailableModel[]> {
  switch (provider) {
    case "claude":
      return fetchClaudeModels(apiKey, baseUrl);
    case "openai":
      return fetchOpenAIModels(apiKey, baseUrl);
    case "kimi":
      return fetchKimiModels(apiKey, baseUrl);
    case "glm":
      return fetchGLMModels(apiKey);
    default:
      return [];
  }
}
