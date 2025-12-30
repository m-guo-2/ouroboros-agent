import { config } from "dotenv";
import { resolve } from "path";

// 加载 .env 文件
config({ path: resolve(__dirname, "../../../.env") });

export interface ModelConfig {
  id: string;
  name: string;
  provider: "claude" | "openai" | "kimi" | "glm";
  enabled: boolean;
  apiKey?: string;
  baseUrl?: string;
  model: string;
  maxTokens: number;
  temperature: number;
}

// 默认模型配置
const defaultModels: ModelConfig[] = [
  {
    id: "claude-sonnet",
    name: "Claude 3.5 Sonnet",
    provider: "claude",
    enabled: true,
    apiKey: process.env.ANTHROPIC_API_KEY,
    model: "claude-sonnet-4-20250514",
    maxTokens: 8192,
    temperature: 0.7,
  },
  {
    id: "gpt-4o",
    name: "GPT-4o",
    provider: "openai",
    enabled: true,
    apiKey: process.env.OPENAI_API_KEY,
    baseUrl: process.env.OPENAI_BASE_URL || "https://api.openai.com/v1",
    model: "gpt-4o",
    maxTokens: 4096,
    temperature: 0.7,
  },
  {
    id: "moonshot-v1",
    name: "Kimi (Moonshot)",
    provider: "kimi",
    enabled: true,
    apiKey: process.env.MOONSHOT_API_KEY,
    baseUrl: "https://api.moonshot.cn/v1",
    model: "moonshot-v1-8k",
    maxTokens: 4096,
    temperature: 0.7,
  },
  {
    id: "glm-4",
    name: "GLM-4 (智谱)",
    provider: "glm",
    enabled: true,
    apiKey: process.env.ZHIPU_API_KEY,
    baseUrl: "https://open.bigmodel.cn/api/paas/v4",
    model: "glm-4",
    maxTokens: 4096,
    temperature: 0.7,
  },
];

// 运行时模型配置存储（内存）
let runtimeModels: ModelConfig[] = [...defaultModels];

export function getModels(): ModelConfig[] {
  return runtimeModels;
}

export function getModelById(id: string): ModelConfig | undefined {
  return runtimeModels.find((m) => m.id === id);
}

export function updateModel(id: string, updates: Partial<ModelConfig>): ModelConfig | null {
  const index = runtimeModels.findIndex((m) => m.id === id);
  if (index === -1) return null;
  
  runtimeModels[index] = { ...runtimeModels[index], ...updates };
  return runtimeModels[index];
}

export function getEnabledModels(): ModelConfig[] {
  return runtimeModels.filter((m) => m.enabled && m.apiKey);
}

export const appConfig = {
  port: parseInt(process.env.PORT || "1997", 10),
  isDev: process.env.NODE_ENV !== "production",
};
