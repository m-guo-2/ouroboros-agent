import { modelDb, settingsDb, type ModelRecord } from "../services/database";

// 导出 ModelConfig 类型（兼容现有代码）
export type ModelConfig = ModelRecord;

/**
 * 从 settings DB 获取配置，不再依赖 .env 文件
 */
function getSettingOrEnv(settingKey: string, envKey: string): string | undefined {
  return settingsDb.get(settingKey) || process.env[envKey] || undefined;
}

// 默认模型配置（首次启动时初始化到数据库）
const defaultModels: Omit<ModelConfig, "createdAt" | "updatedAt">[] = [
  {
    id: "claude-sonnet",
    name: "Claude 3.5 Sonnet",
    provider: "claude",
    enabled: true,
    apiKey: getSettingOrEnv("api_key.anthropic", "ANTHROPIC_API_KEY"),
    model: "claude-sonnet-4-20250514",
    maxTokens: 8192,
    temperature: 0.7,
  },
  {
    id: "gpt-4o",
    name: "GPT-4o",
    provider: "openai",
    enabled: true,
    apiKey: getSettingOrEnv("api_key.openai", "OPENAI_API_KEY"),
    baseUrl: getSettingOrEnv("base_url.openai", "OPENAI_BASE_URL") || "https://api.openai.com/v1",
    model: "gpt-4o",
    maxTokens: 4096,
    temperature: 0.7,
  },
  {
    id: "moonshot-v1",
    name: "Kimi (Moonshot)",
    provider: "kimi",
    enabled: true,
    apiKey: getSettingOrEnv("api_key.moonshot", "MOONSHOT_API_KEY"),
    baseUrl: getSettingOrEnv("base_url.moonshot", "MOONSHOT_BASE_URL") || "https://api.moonshot.cn/v1",
    model: "moonshot-v1-8k",
    maxTokens: 4096,
    temperature: 0.7,
  },
  {
    id: "glm-4",
    name: "GLM-4 (智谱)",
    provider: "glm",
    enabled: true,
    apiKey: getSettingOrEnv("api_key.zhipu", "ZHIPU_API_KEY"),
    baseUrl: getSettingOrEnv("base_url.zhipu", "ZHIPU_BASE_URL") || "https://open.bigmodel.cn/api/paas/v4",
    model: "glm-4",
    maxTokens: 4096,
    temperature: 0.7,
  },
];

// 初始化默认模型到数据库（如果数据库为空）
modelDb.initDefaults(defaultModels);

// ==================== Model API (使用数据库) ====================

export function getModels(): ModelConfig[] {
  return modelDb.getAll();
}

export function getModelById(id: string): ModelConfig | undefined {
  return modelDb.getById(id) || undefined;
}

export function updateModel(id: string, updates: Partial<ModelConfig>): ModelConfig | null {
  return modelDb.update(id, updates);
}

export function getEnabledModels(): ModelConfig[] {
  return modelDb.getAll().filter((m) => m.enabled && m.apiKey);
}

export function createModel(model: Omit<ModelConfig, "createdAt" | "updatedAt">): ModelConfig {
  return modelDb.create(model);
}

export function deleteModel(id: string): boolean {
  return modelDb.delete(id);
}

// ==================== App Config ====================

export const appConfig = {
  port: parseInt(settingsDb.get("general.server_port") || process.env.PORT || "1997", 10),
  isDev: process.env.NODE_ENV !== "production",
};
