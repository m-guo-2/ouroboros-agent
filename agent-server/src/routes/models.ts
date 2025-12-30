import { Router, Request, Response } from "express";
import { getModels, getModelById, updateModel, getEnabledModels } from "../config";
import { clearAdapterCache } from "../services/models";
import { fetchAvailableModels } from "../services/models/registry";

const router = Router();

// 获取所有模型
router.get("/", (_req: Request, res: Response) => {
  const models = getModels();
  res.json({
    success: true,
    data: models.map((m) => ({
      id: m.id,
      name: m.name,
      provider: m.provider,
      enabled: m.enabled,
      configured: !!m.apiKey,
      model: m.model,
      maxTokens: m.maxTokens,
      temperature: m.temperature,
      baseUrl: m.baseUrl,
    })),
  });
});

// 获取已启用且已配置的模型
router.get("/enabled", (_req: Request, res: Response) => {
  const models = getEnabledModels();
  res.json({
    success: true,
    data: models.map((m) => ({
      id: m.id,
      name: m.name,
      provider: m.provider,
    })),
  });
});

// 获取单个模型详情
router.get("/:id", (req: Request, res: Response) => {
  const model = getModelById(req.params.id);
  if (!model) {
    res.status(404).json({ success: false, error: "Model not found" });
    return;
  }

  res.json({
    success: true,
    data: {
      id: model.id,
      name: model.name,
      provider: model.provider,
      enabled: model.enabled,
      configured: !!model.apiKey,
      model: model.model,
      maxTokens: model.maxTokens,
      temperature: model.temperature,
      baseUrl: model.baseUrl,
      // 不返回 apiKey 明文
      hasApiKey: !!model.apiKey,
    },
  });
});

// 更新模型配置
router.patch("/:id", (req: Request, res: Response) => {
  const { apiKey, baseUrl, model, maxTokens, temperature, enabled } = req.body;

  const updates: Record<string, unknown> = {};
  if (apiKey !== undefined) updates.apiKey = apiKey;
  if (baseUrl !== undefined) updates.baseUrl = baseUrl;
  if (model !== undefined) updates.model = model;
  if (maxTokens !== undefined) updates.maxTokens = maxTokens;
  if (temperature !== undefined) updates.temperature = temperature;
  if (enabled !== undefined) updates.enabled = enabled;

  const updated = updateModel(req.params.id, updates);
  if (!updated) {
    res.status(404).json({ success: false, error: "Model not found" });
    return;
  }

  // 清除适配器缓存，使新配置生效
  clearAdapterCache(req.params.id);

  res.json({
    success: true,
    data: {
      id: updated.id,
      name: updated.name,
      provider: updated.provider,
      enabled: updated.enabled,
      configured: !!updated.apiKey,
      model: updated.model,
      maxTokens: updated.maxTokens,
      temperature: updated.temperature,
      baseUrl: updated.baseUrl,
    },
  });
});

// 获取某个 provider 可用的模型列表（需要先配置 API Key）
router.get("/:id/available-models", async (req: Request, res: Response) => {
  const modelConfig = getModelById(req.params.id);
  if (!modelConfig) {
    res.status(404).json({ success: false, error: "Model not found" });
    return;
  }

  if (!modelConfig.apiKey) {
    res.status(400).json({
      success: false,
      error: "请先配置 API Key 后再获取模型列表",
    });
    return;
  }

  try {
    const models = await fetchAvailableModels(
      modelConfig.provider,
      modelConfig.apiKey,
      modelConfig.baseUrl
    );

    res.json({
      success: true,
      data: models,
    });
  } catch (error) {
    console.error("Failed to fetch available models:", error);
    res.status(500).json({
      success: false,
      error: "获取模型列表失败，请检查 API Key 是否正确",
    });
  }
});

export default router;
