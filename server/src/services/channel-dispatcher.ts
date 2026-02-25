/**
 * 渠道消息派发器
 *
 * Server 负责：
 * 1. 去重
 * 2. 用户解析
 * 3. 定位 Agent endpoint
 * 4. 会话管理（创建/获取 session）
 * 5. 消息追踪（traceId）
 * 6. 存用户消息（关联 session + trace）
 * 7. 派发给 Agent App（附带 sessionId + traceId）
 *
 * 所有"agent 怎么思考"的逻辑（记忆加载、指令构建、会话策略）
 * 已移到 agent 侧。
 */

import type { IncomingMessage, ProcessingResult } from "./channel-types";
import { processedMessageDb, agentConfigDb, messagesDb, agentSessionDb } from "./database";
import { resolveUser } from "./user-resolver";
import { agentRegistry } from "./agent-registry";
import { logger } from "./logger";
import { startMessageTrace } from "./message-trace";
import { handleTraceEvent } from "./execution-trace";

function resolveSessionKey(msg: IncomingMessage): string {
  const uniqueId = msg.channelConversationId || msg.channelUserId;
  return `${msg.channel}:${uniqueId}`;
}

/**
 * 迁移兜底：按 (channelConversationId, agentId) 查找旧 session
 * 处理无 session_key 的旧 session，找到后回填 session_key
 */
function findLegacySession(
  msg: IncomingMessage,
  agentId: string,
  sessionKey: string,
): ReturnType<typeof agentSessionDb.findBySessionKey> {
  if (!msg.channelConversationId) return null;

  // 按 channelConversationId + agentId 精确匹配（同一群 + 同一 Agent = 同一会话）
  const session = agentSessionDb.findByConversationId(msg.channelConversationId, agentId);
  if (!session) return null;

  // 回填 session_key，后续查询走 findBySessionKey 正常路径
  const updates: Partial<{ sessionKey: string; channelConversationId: string }> = {};
  if (!session.sessionKey) {
    updates.sessionKey = sessionKey;
  }
  if (!session.channelConversationId) {
    updates.channelConversationId = msg.channelConversationId;
  }
  if (Object.keys(updates).length > 0) {
    agentSessionDb.update(session.id, updates);
    logger.info(`Migrated legacy session: backfilled sessionKey`, {
      sessionId: session.id,
      sessionKey,
      channelConversationId: msg.channelConversationId,
    });
  }

  return session;
}

/**
 * 派发入站消息到 Agent App
 */
export async function dispatchIncomingMessage(msg: IncomingMessage): Promise<ProcessingResult> {
  // 1. 去重（messageId + agentId 组合）
  const dedupeKey = msg.agentId
    ? `${msg.channelMessageId}:${msg.agentId}`
    : msg.channelMessageId;

  if (processedMessageDb.exists(dedupeKey)) {
    logger.debug(`Duplicate message skipped: ${dedupeKey}`);
    return { success: true, duplicate: true };
  }
  processedMessageDb.mark(dedupeKey, msg.channel);

  // 2. 用户解析
  const { userId, isNew } = resolveUser(msg.channel, msg.channelUserId, msg.senderName);
  if (isNew) {
    logger.info(`New shadow user created for ${msg.channel}:${msg.channelUserId}`, { userId });
  }

  // 3. 定位目标 Agent
  const agent = msg.agentId
    ? agentConfigDb.getById(msg.agentId)
    : agentConfigDb.getById("default-agent-config");

  if (!agent) {
    logger.warn(`No agent available for message from ${msg.channel}`);
    return { success: false, error: "No agent available" };
  }

  // 4. 消息追踪：创建 traceId
  const msgTrace = startMessageTrace("user", `${msg.channel}:${msg.channelUserId}`);
  const { traceId, span, initiator } = msgTrace;

  // 5. 会话管理：获取或创建 session
  //    优先按 sessionKey 查找；兜底按 channelConversationId 查找旧 session
  const sessionKey = resolveSessionKey(msg);
  let session = agentSessionDb.findBySessionKey(sessionKey, agent.id)
    || findLegacySession(msg, agent.id, sessionKey);
  if (!session) {
    const sessionId = crypto.randomUUID();
    const title = msg.content.substring(0, 30) + (msg.content.length > 30 ? "..." : "");
    session = agentSessionDb.create({
      id: sessionId,
      title,
      userId,
      agentId: agent.id,
      sourceChannel: msg.channel,
      sessionKey,
      channelName: msg.channelConversationName || undefined,
      channelConversationId: msg.channelConversationId,
    });
    logger.info(`New session created for agent [${agent.displayName}]`, { sessionId, userId, agentId: agent.id });
  } else {
    const updates: Partial<{ channelName: string; channelConversationId: string; sessionKey: string }> = {};
    if (msg.channelConversationName && !session.channelName) {
      updates.channelName = msg.channelConversationName;
      session.channelName = msg.channelConversationName;
    }
    if (msg.channelConversationId && !session.channelConversationId) {
      updates.channelConversationId = msg.channelConversationId;
      session.channelConversationId = msg.channelConversationId;
    }
    if (!session.sessionKey) {
      updates.sessionKey = sessionKey;
    }
    if (Object.keys(updates).length > 0) {
      agentSessionDb.update(session.id, updates);
    }
  }

  logger.business("decision", `Dispatching to Agent [${agent.displayName}]`, {
    userId,
    agentId: agent.id,
    channel: msg.channel,
    sessionId: session.id,
    traceId,
    messageLength: msg.content.length,
    initiator,
  }, span);

  // 6. 存用户消息（messages 表 = 唯一消息存储）
  const messageId = crypto.randomUUID();
  messagesDb.insert({
    id: messageId,
    sessionId: session.id,
    role: "user",
    content: msg.content,
    messageType: msg.messageType,
    channel: msg.channel,
    channelMessageId: msg.channelMessageId,
    traceId,
    initiator,
    status: "sent",
  });

  // 7. 消息到达即落库（trace start + session processing）
  const startTimestamp = Date.now();
  agentSessionDb.update(session.id, { executionStatus: "processing" });
  handleTraceEvent({
    traceId,
    sessionId: session.id,
    agentId: agent.id,
    userId,
    channel: msg.channel,
    type: "start",
    timestamp: startTimestamp,
    initiator,
  });

  // 8. 派发给 Agent App（附带 sessionId + traceId）
  const endpoint = agentRegistry.getEndpoint(agent.id);

  void (async () => {
    try {
      const response = await fetch(`${endpoint}/process`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          userId,
          agentId: agent.id,
          content: msg.content,
          channel: msg.channel,
          channelUserId: msg.channelUserId,
          channelConversationId: msg.channelConversationId,
          channelMessageId: msg.channelMessageId,
          senderName: msg.senderName,
          messageId,
          sessionId: session.id,
          traceId,
        }),
      });

      if (!response.ok) {
        const error = `Failed to dispatch to Agent: HTTP ${response.status}`;
        logger.error("channel_dispatcher", error);
        agentSessionDb.update(session.id, { executionStatus: "interrupted" });
        const errorTimestamp = Date.now();
        handleTraceEvent({
          traceId,
          sessionId: session.id,
          agentId: agent.id,
          userId,
          channel: msg.channel,
          type: "error",
          timestamp: errorTimestamp,
          error,
        });
        handleTraceEvent({
          traceId,
          sessionId: session.id,
          agentId: agent.id,
          userId,
          channel: msg.channel,
          type: "done",
          timestamp: errorTimestamp,
          error,
        });
      }
    } catch (err) {
      const error = err instanceof Error ? err.message : "Unknown error";
      logger.error("channel_dispatcher", `Failed to dispatch to Agent: ${error}`, err instanceof Error ? err : undefined);
      agentSessionDb.update(session.id, { executionStatus: "interrupted" });
      const errorTimestamp = Date.now();
      handleTraceEvent({
        traceId,
        sessionId: session.id,
        agentId: agent.id,
        userId,
        channel: msg.channel,
        type: "error",
        timestamp: errorTimestamp,
        error,
      });
      handleTraceEvent({
        traceId,
        sessionId: session.id,
        agentId: agent.id,
        userId,
        channel: msg.channel,
        type: "done",
        timestamp: errorTimestamp,
        error,
      });
    }
  })();

  return { success: true, sessionId: session.id, userId };
}

/**
 * 流式派发入站消息（WebUI 场景）
 * 返回 Agent App 的 SSE 流
 */
export async function dispatchIncomingMessageStream(msg: IncomingMessage): Promise<Response | null> {
  // 去重
  const dedupeKey = msg.agentId
    ? `${msg.channelMessageId}:${msg.agentId}`
    : msg.channelMessageId;

  if (processedMessageDb.exists(dedupeKey)) {
    return null;
  }
  processedMessageDb.mark(dedupeKey, msg.channel);

  // 用户解析
  const { userId } = resolveUser(msg.channel, msg.channelUserId, msg.senderName);

  // 定位 Agent
  const agent = msg.agentId
    ? agentConfigDb.getById(msg.agentId)
    : agentConfigDb.getById("default-agent-config");

  // 消息追踪
  const msgTrace = startMessageTrace("user", `${msg.channel}:${msg.channelUserId}`);
  const { traceId, initiator } = msgTrace;

  // 会话管理（同 dispatchIncomingMessage 逻辑）
  const sessionKey = resolveSessionKey(msg);
  let session = agentSessionDb.findBySessionKey(sessionKey, agent?.id)
    || (agent?.id ? findLegacySession(msg, agent.id, sessionKey) : null);
  if (!session) {
    const sessionId = crypto.randomUUID();
    const title = msg.content.substring(0, 30) + (msg.content.length > 30 ? "..." : "");
    session = agentSessionDb.create({
      id: sessionId,
      title,
      userId,
      agentId: agent?.id,
      sourceChannel: msg.channel,
      sessionKey,
      channelName: msg.channelConversationName || undefined,
      channelConversationId: msg.channelConversationId,
    });
  } else {
    const updates: Partial<{ channelName: string; channelConversationId: string; sessionKey: string }> = {};
    if (msg.channelConversationName && !session.channelName) {
      updates.channelName = msg.channelConversationName;
      session.channelName = msg.channelConversationName;
    }
    if (msg.channelConversationId && !session.channelConversationId) {
      updates.channelConversationId = msg.channelConversationId;
      session.channelConversationId = msg.channelConversationId;
    }
    if (!session.sessionKey) {
      updates.sessionKey = sessionKey;
    }
    if (Object.keys(updates).length > 0) {
      agentSessionDb.update(session.id, updates);
    }
  }

  // 存用户消息（messages 表 = 唯一消息存储）
  const messageId = crypto.randomUUID();
  messagesDb.insert({
    id: messageId,
    sessionId: session.id,
    role: "user",
    content: msg.content,
    messageType: msg.messageType,
    channel: msg.channel,
    channelMessageId: msg.channelMessageId,
    traceId,
    initiator,
    status: "sent",
  });

  // 观测起点：消息到达即写入 trace 与会话状态
  const startTimestamp = Date.now();
  agentSessionDb.update(session.id, { executionStatus: "processing" });
  handleTraceEvent({
    traceId,
    sessionId: session.id,
    agentId: agent?.id,
    userId,
    channel: msg.channel,
    type: "start",
    timestamp: startTimestamp,
    initiator,
  });

  // 流式请求 Agent App（附带 sessionId + traceId）
  const endpoint = agentRegistry.getEndpoint(agent?.id);
  try {
    const response = await fetch(`${endpoint}/process/stream`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        userId,
        agentId: agent?.id || "default-agent-config",
        content: msg.content,
        channel: msg.channel,
        channelUserId: msg.channelUserId,
        channelConversationId: msg.channelConversationId,
        channelMessageId: msg.channelMessageId,
        senderName: msg.senderName,
        messageId,
        sessionId: session.id,
        traceId,
      }),
    });

    if (!response.ok) {
      const error = `Failed to dispatch stream to Agent: HTTP ${response.status}`;
      logger.error("channel_dispatcher", error);
      agentSessionDb.update(session.id, { executionStatus: "interrupted" });
      const errorTimestamp = Date.now();
      handleTraceEvent({
        traceId,
        sessionId: session.id,
        agentId: agent?.id,
        userId,
        channel: msg.channel,
        type: "error",
        timestamp: errorTimestamp,
        error,
      });
      handleTraceEvent({
        traceId,
        sessionId: session.id,
        agentId: agent?.id,
        userId,
        channel: msg.channel,
        type: "done",
        timestamp: errorTimestamp,
        error,
      });
    }

    return response as unknown as Response;
  } catch (err) {
    const error = err instanceof Error ? err.message : "Unknown error";
    logger.error("channel_dispatcher", `Failed to dispatch stream to Agent: ${error}`, err instanceof Error ? err : undefined);
    agentSessionDb.update(session.id, { executionStatus: "interrupted" });
    const errorTimestamp = Date.now();
    handleTraceEvent({
      traceId,
      sessionId: session.id,
      agentId: agent?.id,
      userId,
      channel: msg.channel,
      type: "error",
      timestamp: errorTimestamp,
      error,
    });
    handleTraceEvent({
      traceId,
      sessionId: session.id,
      agentId: agent?.id,
      userId,
      channel: msg.channel,
      type: "done",
      timestamp: errorTimestamp,
      error,
    });
    return null;
  }
}
