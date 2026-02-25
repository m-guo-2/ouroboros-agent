/**
 * 统一渠道路由
 * POST /api/channels/incoming -- 接收所有渠道的归一化消息
 */

import { Router, type Request, type Response } from "express";
import type { IncomingMessage } from "../services/channel-types";
import { dispatchIncomingMessage } from "../services/channel-dispatcher";
import { healthCheckAll, getRegisteredChannels, sendToChannel } from "../services/channel-registry";
import type { OutgoingMessage } from "../services/channel-types";
import { logger } from "../services/logger";

const router = Router();
const LEGACY_CHANNEL_SEND_ENABLED =
  process.env.ALLOW_LEGACY_CHANNEL_SEND === "1" ||
  process.env.ALLOW_LEGACY_CHANNEL_SEND === "true";

/**
 * POST /api/channels/incoming
 * 接收来自所有渠道（飞书、企微、WebUI）的归一化消息
 * 立即返回 202 Accepted，异步处理消息
 */
router.post("/incoming", (req: Request, res: Response) => {
  const msg = req.body as IncomingMessage;

  // 基本校验
  if (!msg.channel || !msg.channelUserId || !msg.channelMessageId || !msg.content) {
    return res.status(400).json({
      success: false,
      error: "Missing required fields: channel, channelUserId, channelMessageId, content",
    });
  }

  const validChannels = ["feishu", "qiwei", "webui"];
  if (!validChannels.includes(msg.channel)) {
    return res.status(400).json({
      success: false,
      error: `Invalid channel: ${msg.channel}. Must be one of: ${validChannels.join(", ")}`,
    });
  }

  // 补充默认值
  if (!msg.timestamp) {
    msg.timestamp = Date.now();
  }
  if (!msg.messageType) {
    msg.messageType = "text";
  }

  logger.info(`Incoming message from ${msg.channel}`, {
    channelUserId: msg.channelUserId,
    channelMessageId: msg.channelMessageId,
    messageType: msg.messageType,
    contentLength: msg.content.length,
    agentId: msg.agentId || "default",
  });

  // 立即返回 202，异步处理
  res.status(202).json({ success: true, message: "Message accepted for processing" });

  // 后台异步处理：派发给 Agent App
  dispatchIncomingMessage(msg).catch((error) => {
    logger.error("channel_incoming", `Failed to dispatch incoming message: ${error.message}`, error);
  });
});

/**
 * POST /api/channels/send
 * 统一出站消息接口：先存储后分发
 * 供 orchestrator skill、外部服务、手动调试使用
 */
router.post("/send", async (req: Request, res: Response) => {
  if (!LEGACY_CHANNEL_SEND_ENABLED) {
    const userAgent = req.headers["user-agent"] || "unknown";
    const ip = req.ip || "unknown";
    logger.warn("[channel-send-legacy] blocked legacy send endpoint", {
      path: req.originalUrl || req.path,
      ip,
      userAgent,
    });
    console.warn(
      `[channel-send-legacy] BLOCKED path=${req.originalUrl || req.path} ip=${ip} ua=${String(userAgent)}`,
    );
    return res.status(410).json({
      success: false,
      error: "legacy endpoint disabled; use /api/data/channels/send",
    });
  }

  const {
    channel,
    channelUserId,
    content,
    messageType,
    channelConversationId,
    replyToChannelMessageId,
    sessionId,
    mentions,
  } = req.body;

  if (!channel || !channelUserId || !content) {
    return res.status(400).json({
      success: false,
      error: "Missing required fields: channel, channelUserId, content",
    });
  }

  const outgoing: OutgoingMessage = {
    channel,
    channelUserId,
    content,
    messageType: messageType || "text",
    channelConversationId,
    replyToChannelMessageId,
    sessionId,
    mentions,
  };

  try {
    const messageId = await sendToChannel(outgoing);
    logger.info(`Message sent to ${channel}`, {
      messageId,
      channelUserId,
      messageType: outgoing.messageType,
      sessionId,
    });
    res.json({ success: true, message: "Message sent", messageId });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    logger.error("channel_send", `Failed to send message: ${errorMessage}`, error instanceof Error ? error : undefined);
    res.status(500).json({ success: false, error: errorMessage });
  }
});

/**
 * GET /api/channels/health
 * 检查所有渠道适配器的健康状态
 */
router.get("/health", async (_req: Request, res: Response) => {
  try {
    const health = await healthCheckAll();
    const registered = getRegisteredChannels();

    res.json({
      success: true,
      channels: registered,
      health,
    });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: errorMessage });
  }
});

export default router;
