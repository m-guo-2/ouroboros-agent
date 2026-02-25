/**
 * Data API — 供 Agent App 调用的数据读写接口
 *
 * 设计原则：Server 只暴露 CRUD，不含业务逻辑。
 * Agent App 自主决定何时读写、读写什么。
 */

import { Router, type Request, type Response } from "express";
import {
  agentConfigDb,
  agentSessionDb,
  messagesDb,
  userMemoryDb,
  userMemoryFactDb,
  modelDb,
  settingsDb,
  db,
} from "../services/database";
import { sendToChannel } from "../services/channel-registry";
import type { OutgoingMessage } from "../services/channel-types";
import { logger } from "../services/logger";

const router = Router();
const CHANNEL_SEND_TOKEN =
  process.env.AGENT_CHANNEL_SEND_TOKEN ||
  process.env.AGENT_SEND_TOKEN ||
  "local-agent-send-token";
const EXPECTED_AGENT_SEND_SOURCE =
  process.env.AGENT_SEND_SOURCE ||
  "agent-sdk-runner";
const ALLOW_CURL_CHANNEL_SEND =
  process.env.ALLOW_CURL_CHANNEL_SEND === "1" ||
  process.env.ALLOW_CURL_CHANNEL_SEND === "true";

function getHeaderValue(req: Request, key: string): string | undefined {
  const raw = req.headers[key];
  if (typeof raw === "string") return raw;
  if (Array.isArray(raw) && raw.length > 0) return raw[0];
  return undefined;
}

function resolveChannelSendToken(req: Request): string | undefined {
  const tokenFromHeader = getHeaderValue(req, "x-agent-send-token");
  if (tokenFromHeader) return tokenFromHeader;
  const authorization = getHeaderValue(req, "authorization");
  if (authorization?.startsWith("Bearer ")) {
    return authorization.slice("Bearer ".length);
  }
  return undefined;
}

function rejectUnauthorizedChannelSend(req: Request, res: Response): boolean {
  const token = resolveChannelSendToken(req);
  if (token === CHANNEL_SEND_TOKEN) return false;

  const userAgent = req.headers["user-agent"] || "unknown";
  const source = getHeaderValue(req, "x-agent-source") || "unknown";
  const ip = req.ip || "unknown";
  const path = req.originalUrl || req.path;

  logger.warn("[channel-send-auth] rejected unauthorized sender", {
    path,
    ip,
    userAgent,
    source,
  });
  console.warn(
    `[channel-send-auth] REJECT path=${path} ip=${ip} source=${source} ua=${String(userAgent)}`,
  );
  res.status(403).json({
    success: false,
    error: "unauthorized sender for channel send",
  });
  return true;
}

// ==================== Agent Config ====================

/**
 * GET /api/data/agents/:agentId
 * 获取 Agent 配置（systemPrompt, provider, model, skills, channels）
 */
router.get("/agents/:agentId", (req: Request, res: Response) => {
  const config = agentConfigDb.getById(req.params.agentId);
  if (!config) {
    return res.status(404).json({ success: false, error: "Agent not found" });
  }
  res.json({ success: true, data: config });
});

/**
 * GET /api/data/agents/:agentId/skills-context
 * 获取 Agent 编译后的 skill 上下文（systemPrompt 附加内容 + 工具定义）
 * 复用现有的 skill-manager 编译逻辑
 */
router.get("/agents/:agentId/skills-context", (req: Request, res: Response) => {
  try {
    const config = agentConfigDb.getById(req.params.agentId);
    if (!config) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    // 使用 skillManager 编译 skill 上下文
    const { skillManager } = require("../services/skill-manager");
    const context = skillManager.compileContext();
    res.json({ success: true, data: context });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

// ==================== Provider Credentials ====================

// provider → settings key 映射（与 settings.ts 中的 PROVIDER_MAP 保持一致）
const PROVIDER_CREDENTIALS: Record<string, { apiKeySettingKey: string; baseUrlSettingKey?: string }> = {
  anthropic: { apiKeySettingKey: "api_key.anthropic", baseUrlSettingKey: "base_url.anthropic" },
  claude:    { apiKeySettingKey: "api_key.anthropic", baseUrlSettingKey: "base_url.anthropic" },
  openai:    { apiKeySettingKey: "api_key.openai",    baseUrlSettingKey: "base_url.openai" },
  moonshot:  { apiKeySettingKey: "api_key.moonshot",   baseUrlSettingKey: "base_url.moonshot" },
  kimi:      { apiKeySettingKey: "api_key.moonshot",   baseUrlSettingKey: "base_url.moonshot" },
  zhipu:     { apiKeySettingKey: "api_key.zhipu",      baseUrlSettingKey: "base_url.zhipu" },
  glm:       { apiKeySettingKey: "api_key.zhipu",      baseUrlSettingKey: "base_url.zhipu" },
};

/**
 * GET /api/data/provider-credentials/:provider
 * 根据 provider 名称从 settings 表获取 apiKey 和 baseUrl
 * Agent App 用此接口获取 SDK 运行所需的凭证
 */
router.get("/provider-credentials/:provider", (req: Request, res: Response) => {
  const provider = req.params.provider.toLowerCase();
  const mapping = PROVIDER_CREDENTIALS[provider];
  if (!mapping) {
    return res.status(404).json({ success: false, error: `Unknown provider: ${provider}` });
  }
  const apiKey = settingsDb.get(mapping.apiKeySettingKey) || "";
  const baseUrl = mapping.baseUrlSettingKey ? (settingsDb.get(mapping.baseUrlSettingKey) || "") : "";
  res.json({ success: true, data: { provider, apiKey, baseUrl } });
});

// ==================== Models (legacy) ====================

/**
 * GET /api/data/models/:modelId
 * 获取模型完整配置（provider, baseUrl, apiKey, model name）
 * @deprecated 使用 /api/data/provider-credentials/:provider 代替
 */
router.get("/models/:modelId", (req: Request, res: Response) => {
  const model = modelDb.getById(req.params.modelId);
  if (!model) {
    return res.status(404).json({ success: false, error: "Model not found" });
  }
  res.json({
    success: true,
    data: {
      id: model.id,
      name: model.name,
      provider: model.provider,
      baseUrl: model.baseUrl,
      apiKey: model.apiKey,
      model: model.model,
      maxTokens: model.maxTokens,
      temperature: model.temperature,
    },
  });
});

// ==================== Sessions ====================

/**
 * GET /api/data/sessions/by-key
 * 查找会话（按 agentId × sessionKey）
 */
router.get("/sessions/by-key", (req: Request, res: Response) => {
  const { agentId, sessionKey } = req.query as Record<string, string>;
  if (!sessionKey) {
    return res.status(400).json({ success: false, error: "sessionKey is required" });
  }
  const session = agentSessionDb.findBySessionKey(sessionKey, agentId);
  res.json({ success: true, data: session || null });
});

/**
 * GET /api/data/sessions/:sessionId
 * 获取单个 session
 */
router.get("/sessions/:sessionId", (req: Request, res: Response) => {
  const session = agentSessionDb.getById(req.params.sessionId);
  if (!session) {
    return res.status(404).json({ success: false, error: "Session not found" });
  }
  res.json({ success: true, data: session });
});

/**
 * GET /api/data/sessions/:sessionId/messages
 * 获取 session 的消息历史（独立 messages 表）
 */
router.get("/sessions/:sessionId/messages", (req: Request, res: Response) => {
  const limit = parseInt(req.query.limit as string) || 50;
  const before = req.query.before as string | undefined;
  const messages = messagesDb.getBySession(req.params.sessionId, { limit, before });
  res.json({ success: true, data: messages });
});

/**
 * POST /api/data/sessions
 * 创建 session
 */
router.post("/sessions", (req: Request, res: Response) => {
  const {
    id,
    title,
    agentId,
    userId,
    channel,
    sdkSessionId,
    sessionKey,
    channelConversationId,
    workDir,
  } = req.body;
  if (!id) {
    return res.status(400).json({ success: false, error: "id is required" });
  }
  const session = agentSessionDb.create({
    id,
    title: title || "新对话",
    agentId,
    userId,
    sourceChannel: channel,
    sdkSessionId,
    sessionKey,
    channelConversationId,
    workDir,
  });
  res.json({ success: true, data: session });
});

/**
 * PUT /api/data/sessions/:sessionId
 * 更新 session（sdkSessionId, title, executionStatus）
 */
router.put("/sessions/:sessionId", (req: Request, res: Response) => {
  const updates: Partial<{
    sdkSessionId: string;
    title: string;
    executionStatus: string;
    sessionKey: string;
    channelConversationId: string;
    workDir: string;
  }> = {};
  const {
    sdkSessionId,
    title,
    executionStatus,
    sessionKey,
    channelConversationId,
    workDir,
  } = req.body;

  if (sdkSessionId !== undefined) updates.sdkSessionId = sdkSessionId;
  if (title !== undefined) updates.title = title;
  if (sessionKey !== undefined) updates.sessionKey = sessionKey;
  if (channelConversationId !== undefined) updates.channelConversationId = channelConversationId;
  if (workDir !== undefined) updates.workDir = workDir;
  if (executionStatus !== undefined) updates.executionStatus = executionStatus;

  const session = agentSessionDb.update(req.params.sessionId, updates);
  if (!session) {
    return res.status(404).json({ success: false, error: "Session not found" });
  }

  res.json({ success: true, data: agentSessionDb.getById(req.params.sessionId) });
});

/**
 * GET /api/data/sessions/interrupted
 * 查找所有被中断的 session（用于断点续传）
 */
router.get("/sessions-interrupted", (_req: Request, res: Response) => {
  const rows = db.query(
    "SELECT * FROM agent_sessions WHERE execution_status = 'interrupted' ORDER BY updated_at DESC"
  ).all();
  res.json({ success: true, data: rows });
});

// ==================== Messages ====================

/**
 * POST /api/data/messages
 * 存储消息（user 或 assistant）
 * 同时写入 session 的 messages JSON（供 MonitorView 读取）
 */
router.post("/messages", (req: Request, res: Response) => {
  const {
    id, sessionId, role, content, messageType, channel,
    channelMessageId, toolCalls, traceId, initiator, status,
    senderName, senderId,
  } = req.body;

  if (!sessionId || !role || !content) {
    return res.status(400).json({ success: false, error: "sessionId, role, content are required" });
  }

  const message = messagesDb.insert({
    id: id || crypto.randomUUID(),
    sessionId,
    role,
    content,
    messageType: messageType || "text",
    channel,
    channelMessageId,
    toolCalls,
    traceId,
    initiator,
    status: status || "sent",
    senderName,
    senderId,
  });

  res.json({ success: true, data: message });
});

// ==================== Memory ====================

/**
 * GET /api/data/memory/:agentId/:userId
 * 获取记忆（summary + facts 原始数据）
 */
router.get("/memory/:agentId/:userId", (req: Request, res: Response) => {
  const { agentId, userId } = req.params;
  const memory = userMemoryDb.getByUserId(userId, agentId);
  const facts = userMemoryFactDb.getByUserId(userId, agentId);

  res.json({
    success: true,
    data: {
      summary: memory?.summary || "",
      facts,
    },
  });
});

/**
 * PUT /api/data/memory/:agentId/:userId/summary
 * 更新记忆摘要
 */
router.put("/memory/:agentId/:userId/summary", (req: Request, res: Response) => {
  const { agentId, userId } = req.params;
  const { summary } = req.body;

  if (typeof summary !== "string") {
    return res.status(400).json({ success: false, error: "summary is required" });
  }

  userMemoryDb.upsert(userId, summary, agentId);
  res.json({ success: true });
});

/**
 * POST /api/data/memory/:agentId/:userId/facts
 * 添加记忆事实
 */
router.post("/memory/:agentId/:userId/facts", (req: Request, res: Response) => {
  const { agentId, userId } = req.params;
  const { category, fact, sourceChannel, sourceSessionId, expiresAt } = req.body;

  if (!category || !fact) {
    return res.status(400).json({ success: false, error: "category and fact are required" });
  }

  const record = userMemoryFactDb.create({
    id: crypto.randomUUID(),
    userId,
    agentId,
    category,
    fact,
    sourceChannel,
    sourceSessionId,
    expiresAt,
  });

  res.json({ success: true, data: record });
});

// ==================== Channel Send (proxy to existing) ====================

/**
 * POST /api/data/channels/send
 * 发消息到渠道（Agent 自主决策回复时调用）
 * 复用 channel-registry 的 sendToChannel
 * 同时写入 session JSON（供 MonitorView 读取）
 */
router.post("/channels/send", async (req: Request, res: Response) => {
  if (rejectUnauthorizedChannelSend(req, res)) return;
  const source = getHeaderValue(req, "x-agent-source") || "unknown";
  const userAgent = getHeaderValue(req, "user-agent") || "unknown";
  if (source !== EXPECTED_AGENT_SEND_SOURCE) {
    logger.warn("[channel-send-guard] unexpected sender source", {
      source,
      expectedSource: EXPECTED_AGENT_SEND_SOURCE,
      userAgent,
      path: req.originalUrl || req.path,
    });
    return res.status(403).json({
      success: false,
      error: "unexpected sender source",
    });
  }
  if (!ALLOW_CURL_CHANNEL_SEND && userAgent.startsWith("curl/")) {
    logger.warn("[channel-send-guard] curl sender blocked", {
      source,
      userAgent,
      path: req.originalUrl || req.path,
    });
    return res.status(403).json({
      success: false,
      error: "curl sender is blocked",
    });
  }

  const { channel, channelUserId, content, messageType, channelConversationId, sessionId, mentions, traceId } = req.body;

  if (!channel || !channelUserId || !content) {
    return res.status(400).json({ success: false, error: "channel, channelUserId, content are required" });
  }
  if (!sessionId || !traceId) {
    logger.warn("[channel-send-guard] missing sessionId or traceId", {
      source,
      userAgent,
      channel,
      channelUserId,
      hasSessionId: !!sessionId,
      hasTraceId: !!traceId,
    });
    return res.status(400).json({
      success: false,
      error: "sessionId and traceId are required",
    });
  }

  const outgoing: OutgoingMessage = {
    channel,
    channelUserId,
    content,
    messageType: messageType || "text",
    channelConversationId,
    sessionId,
    mentions,
    channelMeta: {
      source,
      userAgent,
    },
    traceId,
  };

  try {
    const messageId = await sendToChannel(outgoing);
    logger.info(`[Data API] Message sent to ${channel}`, { messageId, channelUserId, source });

    res.json({ success: true, messageId });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

export default router;
