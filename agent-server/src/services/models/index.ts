import { BaseModelAdapter } from "./base";
import { ClaudeAdapter } from "./claude";
import { OpenAIAdapter } from "./openai";
import { KimiAdapter } from "./kimi";
import { GLMAdapter } from "./glm";
import { getModelById, type ModelConfig } from "../../config";

export * from "./base";

const adapterCache: Map<string, BaseModelAdapter> = new Map();

export function createAdapter(config: ModelConfig): BaseModelAdapter {
  switch (config.provider) {
    case "claude":
      return new ClaudeAdapter(config);
    case "openai":
      return new OpenAIAdapter(config);
    case "kimi":
      return new KimiAdapter(config);
    case "glm":
      return new GLMAdapter(config);
    default:
      throw new Error(`Unknown provider: ${config.provider}`);
  }
}

export function getAdapter(modelId: string): BaseModelAdapter {
  // 检查缓存
  if (adapterCache.has(modelId)) {
    return adapterCache.get(modelId)!;
  }

  // 获取模型配置
  const config = getModelById(modelId);
  if (!config) {
    throw new Error(`Model not found: ${modelId}`);
  }

  if (!config.apiKey) {
    throw new Error(`API key not configured for model: ${modelId}`);
  }

  // 创建适配器并缓存
  const adapter = createAdapter(config);
  adapterCache.set(modelId, adapter);
  return adapter;
}

export function clearAdapterCache(modelId?: string): void {
  if (modelId) {
    adapterCache.delete(modelId);
  } else {
    adapterCache.clear();
  }
}
