import OpenAI from "openai";
import Anthropic from "@anthropic-ai/sdk";

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
 * 注意：Anthropic API 没有 list models 端点，使用预定义列表
 */
export async function fetchClaudeModels(apiKey: string): Promise<AvailableModel[]> {
  // Anthropic 没有公开的模型列表 API，返回已知模型
  // 可以尝试调用一个简单请求来验证 API Key
  try {
    const client = new Anthropic({ apiKey });
    
    // 验证 API Key 是否有效（发送一个最小请求）
    await client.messages.create({
      model: "claude-sonnet-4-20250514",
      max_tokens: 1,
      messages: [{ role: "user", content: "hi" }],
    });

    // API Key 有效，返回可用模型列表
    return [
      {
        id: "claude-sonnet-4-20250514",
        name: "Claude Sonnet 4",
        provider: "claude",
        description: "最新的 Claude Sonnet 模型",
      },
      {
        id: "claude-3-5-sonnet-20241022",
        name: "Claude 3.5 Sonnet",
        provider: "claude",
        description: "高性能平衡模型",
      },
      {
        id: "claude-3-5-haiku-20241022",
        name: "Claude 3.5 Haiku",
        provider: "claude",
        description: "快速响应模型",
      },
      {
        id: "claude-3-opus-20240229",
        name: "Claude 3 Opus",
        provider: "claude",
        description: "最强推理能力",
      },
    ];
  } catch (error) {
    console.error("Failed to verify Claude API key:", error);
    return [];
  }
}

/**
 * 获取 Kimi (Moonshot) 可用模型列表
 * Moonshot 兼容 OpenAI 接口
 */
export async function fetchKimiModels(apiKey: string): Promise<AvailableModel[]> {
  try {
    const client = new OpenAI({
      apiKey,
      baseURL: "https://api.moonshot.cn/v1",
    });

    const response = await client.models.list();
    const models: AvailableModel[] = [];

    for await (const model of response) {
      models.push({
        id: model.id,
        name: formatKimiModelName(model.id),
        provider: "kimi",
      });
    }

    return models;
  } catch (error) {
    console.error("Failed to fetch Kimi models:", error);
    return [];
  }
}

function formatKimiModelName(id: string): string {
  const map: Record<string, string> = {
    "moonshot-v1-8k": "Moonshot V1 (8K)",
    "moonshot-v1-32k": "Moonshot V1 (32K)",
    "moonshot-v1-128k": "Moonshot V1 (128K)",
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
      return fetchClaudeModels(apiKey);
    case "openai":
      return fetchOpenAIModels(apiKey, baseUrl);
    case "kimi":
      return fetchKimiModels(apiKey);
    case "glm":
      return fetchGLMModels(apiKey);
    default:
      return [];
  }
}
