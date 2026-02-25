/**
 * Settings API Routes
 * 统一配置管理 - 所有配置存储在 SQLite 中
 */

import { Router } from "express";
import { settingsDb } from "../services/database";
import { fetchAvailableModels } from "../services/models/registry";

const router = Router();

// provider 名称映射：settings 中使用的名称 → registry 中使用的名称
const PROVIDER_MAP: Record<string, { registryName: string; apiKeySettingKey: string; baseUrlSettingKey?: string }> = {
  anthropic: { registryName: "claude", apiKeySettingKey: "api_key.anthropic", baseUrlSettingKey: "base_url.anthropic" },
  openai:    { registryName: "openai", apiKeySettingKey: "api_key.openai", baseUrlSettingKey: "base_url.openai" },
  moonshot:  { registryName: "kimi",   apiKeySettingKey: "api_key.moonshot", baseUrlSettingKey: "base_url.moonshot" },
  zhipu:     { registryName: "glm",    apiKeySettingKey: "api_key.zhipu", baseUrlSettingKey: "base_url.zhipu" },
  // 也支持 registry 内部名称（兼容两种写法）
  claude:    { registryName: "claude", apiKeySettingKey: "api_key.anthropic", baseUrlSettingKey: "base_url.anthropic" },
  kimi:      { registryName: "kimi",   apiKeySettingKey: "api_key.moonshot", baseUrlSettingKey: "base_url.moonshot" },
  glm:       { registryName: "glm",    apiKeySettingKey: "api_key.zhipu", baseUrlSettingKey: "base_url.zhipu" },
};

// 预定义的配置项分组
const SETTING_GROUPS = {
  orchestrator: {
    label: "Orchestrator (LLM)",
    keys: [
      {
        key: "orchestrator.provider",
        label: "LLM 提供商",
        placeholder: "anthropic",
        description: "anthropic / moonshot / openai / zhipu",
        type: "provider-select" as const,
        options: [
          { value: "anthropic", label: "Anthropic (Claude)" },
          { value: "openai", label: "OpenAI (GPT)" },
          { value: "moonshot", label: "Moonshot (Kimi)" },
          { value: "zhipu", label: "智谱 (GLM)" },
        ],
      },
      {
        key: "orchestrator.model",
        label: "模型名称",
        placeholder: "claude-sonnet-4-20250514",
        description: "选择提供商后可自动查询可用模型",
        type: "model-select" as const,
        providerKey: "orchestrator.provider",
      },
    ],
  },
  apiKeys: {
    label: "API Keys",
    keys: [
      { key: "api_key.anthropic", label: "Anthropic API Key", secret: true },
      { key: "api_key.openai", label: "OpenAI API Key", secret: true },
      { key: "api_key.moonshot", label: "Moonshot API Key", secret: true },
      { key: "api_key.zhipu", label: "智谱 API Key", secret: true },
      { key: "base_url.anthropic", label: "Anthropic Base URL", placeholder: "https://api.anthropic.com" },
      { key: "base_url.openai", label: "OpenAI Base URL", placeholder: "https://api.openai.com/v1" },
      { key: "base_url.moonshot", label: "Moonshot Base URL", placeholder: "https://api.moonshot.cn/v1" },
      { key: "base_url.zhipu", label: "智谱 Base URL", placeholder: "https://open.bigmodel.cn/api/paas/v4" },
    ],
  },
  feishu: {
    label: "飞书配置",
    keys: [
      { key: "feishu.app_id", label: "App ID", placeholder: "cli_xxxxxxxxxxxxxxxx" },
      { key: "feishu.app_secret", label: "App Secret", secret: true },
      { key: "feishu.encrypt_key", label: "Encrypt Key (可选)", secret: true },
      { key: "feishu.verification_token", label: "Verification Token (可选)" },
    ],
  },
  qiwei: {
    label: "企微配置",
    keys: [
      { key: "qiwei.token", label: "QiWei Token", secret: true, placeholder: "X-QIWEI-TOKEN" },
      { key: "qiwei.guid", label: "设备 GUID", placeholder: "企微设备实例 GUID" },
      { key: "qiwei.api_base_url", label: "API Base URL", placeholder: "https://api.qiweapi.com" },
    ],
  },
  general: {
    label: "通用设置",
    keys: [
      { key: "general.server_port", label: "Server 端口", placeholder: "1997" },
      { key: "general.orchestrator_port", label: "Orchestrator 端口", placeholder: "1996" },
      { key: "general.feishu_port", label: "飞书 Bot 端口", placeholder: "1999" },
      { key: "general.qiwei_port", label: "企微 Bot 端口", placeholder: "2000" },
    ],
  },
};

/**
 * GET /api/settings/provider-models
 * 查询指定 LLM 提供商的可用模型列表
 *
 * Query params:
 *   - provider: 提供商名称（anthropic/openai/moonshot/zhipu 或 claude/kimi/glm）
 *
 * 自动从 settings 中读取对应的 API Key 和 Base URL
 */
router.get("/provider-models", async (req, res) => {
  const provider = (req.query.provider as string || "").toLowerCase().trim();

  if (!provider) {
    res.status(400).json({ success: false, error: "请指定 provider 参数" });
    return;
  }

  const mapping = PROVIDER_MAP[provider];
  if (!mapping) {
    res.status(400).json({
      success: false,
      error: `不支持的 provider: ${provider}，可选：anthropic / openai / moonshot / zhipu`,
    });
    return;
  }

  const apiKey = settingsDb.get(mapping.apiKeySettingKey);
  if (!apiKey) {
    res.status(400).json({
      success: false,
      error: `请先在设置中配置 ${mapping.apiKeySettingKey} 后再获取模型列表`,
    });
    return;
  }

  const baseUrl = mapping.baseUrlSettingKey ? settingsDb.get(mapping.baseUrlSettingKey) : undefined;

  try {
    const models = await fetchAvailableModels(mapping.registryName, apiKey, baseUrl || undefined);
    res.json({ success: true, data: models });
  } catch (error) {
    console.error(`[settings] Failed to fetch models for ${provider}:`, error);
    res.status(500).json({ success: false, error: "获取模型列表失败，请检查 API Key 是否正确" });
  }
});

/**
 * GET /api/settings
 * 获取所有配置（敏感值脱敏）
 */
router.get("/", (_req, res) => {
  const allSettings = settingsDb.getAll();

  // 对敏感字段脱敏
  const masked: Record<string, string> = {};
  const secretKeys = new Set<string>();

  // 收集所有 secret 字段
  for (const group of Object.values(SETTING_GROUPS)) {
    for (const item of group.keys) {
      if (item.secret) {
        secretKeys.add(item.key);
      }
    }
  }

  for (const [key, value] of Object.entries(allSettings)) {
    if (secretKeys.has(key) && value) {
      // 脱敏：显示前4位和后4位
      if (value.length > 12) {
        masked[key] = value.substring(0, 4) + "****" + value.substring(value.length - 4);
      } else if (value.length > 0) {
        masked[key] = "****";
      } else {
        masked[key] = "";
      }
    } else {
      masked[key] = value;
    }
  }

  res.json({
    success: true,
    data: masked,
    groups: SETTING_GROUPS,
  });
});

/**
 * GET /api/settings/raw
 * 获取所有配置（包含原始值，用于内部调用）
 */
router.get("/raw", (_req, res) => {
  const allSettings = settingsDb.getAll();
  res.json({ success: true, data: allSettings });
});

/**
 * GET /api/settings/:key
 * 获取单个配置
 */
router.get("/:key", (req, res) => {
  const { key } = req.params;
  const value = settingsDb.get(key);
  res.json({ success: true, data: { key, value } });
});

/**
 * PUT /api/settings
 * 批量更新配置
 */
router.put("/", (req, res) => {
  const settings = req.body as Record<string, string>;

  if (!settings || typeof settings !== "object") {
    res.status(400).json({ success: false, error: "请提供配置对象" });
    return;
  }

  let count = 0;
  for (const [key, value] of Object.entries(settings)) {
    if (typeof key === "string" && typeof value === "string") {
      settingsDb.set(key, value);
      count++;
    }
  }

  // 同步更新 model 表中的 API Key（保持兼容）
  syncApiKeysToModels(settings);

  res.json({ success: true, message: `已更新 ${count} 项配置` });
});

/**
 * PUT /api/settings/:key
 * 更新单个配置
 */
router.put("/:key", (req, res) => {
  const { key } = req.params;
  const { value } = req.body;

  if (typeof value !== "string") {
    res.status(400).json({ success: false, error: "value 必须是字符串" });
    return;
  }

  settingsDb.set(key, value);

  // 同步到 models 表
  syncApiKeysToModels({ [key]: value });

  res.json({ success: true, data: { key, value: "updated" } });
});

/**
 * DELETE /api/settings/:key
 * 删除配置
 */
router.delete("/:key", (req, res) => {
  const { key } = req.params;
  const deleted = settingsDb.delete(key);
  res.json({ success: true, deleted });
});

/**
 * 将 settings 中的 API Key 同步到 models 表
 * 这样模型配置和全局配置保持一致
 */
function syncApiKeysToModels(settings: Record<string, string>) {
  const { modelDb } = require("../services/database");

  const providerMap: Record<string, { apiKeyField: string; baseUrlField?: string; provider: string }> = {
    "api_key.anthropic": { apiKeyField: "api_key.anthropic", provider: "claude" },
    "api_key.openai": { apiKeyField: "api_key.openai", baseUrlField: "base_url.openai", provider: "openai" },
    "api_key.moonshot": { apiKeyField: "api_key.moonshot", baseUrlField: "base_url.moonshot", provider: "kimi" },
    "api_key.zhipu": { apiKeyField: "api_key.zhipu", baseUrlField: "base_url.zhipu", provider: "glm" },
  };

  for (const [settingKey, value] of Object.entries(settings)) {
    const mapping = providerMap[settingKey];
    if (mapping && value) {
      // 找到该 provider 的所有模型，更新 API Key
      const models = modelDb.getAll();
      for (const model of models) {
        if (model.provider === mapping.provider) {
          modelDb.update(model.id, { apiKey: value });
        }
      }
    }

    // 同步 base URL
    const baseUrlMappings: Record<string, string> = {
      "base_url.openai": "openai",
      "base_url.moonshot": "kimi",
      "base_url.zhipu": "glm",
    };

    const provider = baseUrlMappings[settingKey];
    if (provider && value) {
      const models = modelDb.getAll();
      for (const model of models) {
        if (model.provider === provider) {
          modelDb.update(model.id, { baseUrl: value });
        }
      }
    }
  }
}

export default router;
