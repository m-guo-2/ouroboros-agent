/**
 * Agent 路由 - 通过 Orchestrator 执行 Agent 任务
 * 这是自举架构的核心：Server 下发指令给 Orchestrator 执行
 *
 * WebUI 的流式请求走此路由（保持 SSE 连接），
 * 飞书/企微的异步请求走 /api/channels/incoming 路由。
 */

import { Router, Request, Response } from "express";
import { orchestratorClient } from "../services/orchestrator-client";
import { dispatchIncomingMessageStream } from "../services/channel-dispatcher";
import type { IncomingMessage } from "../services/channel-types";
import { getModelById } from "../config";
import { settingsDb } from "../services/database";
import { logger } from "../services/logger";

/** Settings key for persisted agent model */
const AGENT_MODEL_KEY = "agent.default_model";

const router = Router();

/**
 * POST /api/agent/chat/stream
 * 通过 Channel Router 的流式路径处理 WebUI 请求
 * 保持 SSE 连接，实时推送事件
 */
router.post("/chat/stream", async (req: Request, res: Response) => {
  const {
    message,
    sessionId,
    channelUserId,
    agentId,
  } = req.body;

  if (!message) {
    return res.status(400).json({ success: false, error: "message is required" });
  }

  // 构造 IncomingMessage（WebUI 渠道）
  const incomingMsg: IncomingMessage = {
    channel: "webui",
    channelUserId: channelUserId || "webui-anonymous",
    channelMessageId: `webui-${Date.now()}-${Math.random().toString(36).substring(2, 8)}`,
    messageType: "text",
    content: message,
    timestamp: Date.now(),
    channelMeta: sessionId ? { requestedSessionId: sessionId } : undefined,
    agentId: agentId || undefined,
  };

  // 设置 SSE 头
  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");
  res.setHeader("Access-Control-Allow-Origin", "*");
  res.setHeader("X-Accel-Buffering", "no");

  try {
    const agentResponse = await dispatchIncomingMessageStream(incomingMsg);

    if (!agentResponse) {
      // 重复消息
      res.write(`data: ${JSON.stringify({ type: "done", success: true })}\n\n`);
      res.end();
      return;
    }

    // 透传 Agent App 的 SSE 流
    const reader = agentResponse.body?.getReader();
    if (!reader) {
      res.write(`data: ${JSON.stringify({ type: "error", error: "No response body from Agent" })}\n\n`);
      res.end();
      return;
    }

    const decoder = new TextDecoder();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      res.write(decoder.decode(value, { stream: true }));
    }
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    logger.error("agent_stream", `Agent streaming failed: ${errorMessage}`, error instanceof Error ? error : errorMessage);
    res.write(`data: ${JSON.stringify({ type: "error", error: errorMessage })}\n\n`);
  }

  res.end();
});

/**
 * POST /api/agent/chat
 * 通过 Orchestrator 执行 Agent 任务（非流式）
 */
router.post("/chat", async (req: Request, res: Response) => {
  const { message } = req.body;

  if (!message) {
    return res.status(400).json({ success: false, error: "message is required" });
  }

  try {
    const result = await orchestratorClient.execute(message);
    res.json(result);
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: errorMessage });
  }
});

/**
 * POST /api/agent/interrupt
 * 中断当前执行
 */
router.post("/interrupt", async (_req: Request, res: Response) => {
  try {
    await orchestratorClient.interrupt();
    res.json({ success: true });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: errorMessage });
  }
});

/**
 * POST /api/agent/reset
 * 重置 Agent 会话
 */
router.post("/reset", async (_req: Request, res: Response) => {
  try {
    await orchestratorClient.resetSession();
    res.json({ success: true });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: errorMessage });
  }
});

/**
 * GET /api/agent/model
 * 获取当前 Agent 使用的模型（从持久化配置读取）
 */
router.get("/model", (_req: Request, res: Response) => {
  const modelId = settingsDb.get(AGENT_MODEL_KEY);
  if (!modelId) {
    return res.json({ success: true, data: null });
  }

  const model = getModelById(modelId);
  if (!model) {
    return res.json({ success: true, data: { id: modelId, name: modelId } });
  }

  res.json({
    success: true,
    data: {
      id: model.id,
      name: model.name,
      provider: model.provider,
      model: model.model,
    },
  });
});

/**
 * POST /api/agent/use-model/:modelId
 * 使用指定的模型配置，并持久化到 settings
 */
router.post("/use-model/:modelId", async (req: Request, res: Response) => {
  const { modelId } = req.params;

  const model = getModelById(modelId);
  if (!model) {
    return res.status(404).json({
      success: false,
      error: `Model not found: ${modelId}`,
    });
  }

  if (!model.apiKey) {
    return res.status(400).json({
      success: false,
      error: `Model ${modelId} has no API key configured. Please configure it in settings.`,
    });
  }

  // 先持久化模型选择到 settings（不依赖 Orchestrator 是否运行）
  settingsDb.set(AGENT_MODEL_KEY, modelId);

  // 同步更新 Orchestrator 启动配置，确保下次启动时使用正确的模型
  // provider 映射：model config 的 provider → orchestrator settings 的 provider
  const providerMap: Record<string, string> = {
    claude: "anthropic",
    openai: "openai",
    kimi: "moonshot",
    glm: "zhipu",
  };
  const orchestratorProvider = providerMap[model.provider] || model.provider;
  settingsDb.set("orchestrator.provider", orchestratorProvider);
  settingsDb.set("orchestrator.model", model.model);
  console.log(`🔄 Agent model saved: ${model.name} (provider=${orchestratorProvider}, model=${model.model})`);

  // 尝试实时通知 Orchestrator（如果它正在运行）
  let orchestratorConfigured = false;
  try {
    await orchestratorClient.configureModel({
      baseUrl: model.baseUrl,
      apiKey: model.apiKey,
      model: model.model,
    });
    orchestratorConfigured = true;
  } catch {
    // Orchestrator 未运行或不可达——不影响模型保存
    // 模型配置会在 Orchestrator 下次启动时通过环境变量生效
    console.log(`⚠️ Orchestrator 未运行，模型配置将在下次启动时生效`);
  }

  res.json({
    success: true,
    message: orchestratorConfigured
      ? `Agent now using ${model.name}`
      : `模型已保存为 ${model.name}，将在 Orchestrator 启动后生效`,
    model: {
      id: model.id,
      name: model.name,
      provider: model.provider,
      model: model.model,
    },
  });
});

/**
 * POST /api/agent/self-update
 * 请求自我更新（重启）
 */
router.post("/self-update", async (_req: Request, res: Response) => {
  res.json({ success: true, message: "Self-restart requested" });

  setTimeout(async () => {
    try {
      await orchestratorClient.restartSelf();
    } catch (error) {
      console.error("Self-restart failed:", error);
    }
  }, 100);
});

/**
 * GET /api/agent/status
 * 获取 Agent 系统状态
 */
router.get("/status", async (_req: Request, res: Response) => {
  try {
    const processStatus = await orchestratorClient.getProcessStatus();
    const orchestratorHealthy = await orchestratorClient.healthCheck();

    res.json({
      success: true,
      orchestrator: {
        healthy: orchestratorHealthy,
        ...processStatus.orchestrator,
      },
      services: processStatus.services,
    });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    res.json({
      success: false,
      orchestrator: { healthy: false },
      error: errorMessage,
    });
  }
});

export default router;
