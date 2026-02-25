/**
 * Agent 会话管理路由
 * 处理 Agent 模式的会话 CRUD
 * 消息存储统一使用 messages 表（不再使用 agent_sessions.messages JSON 列）
 */
import { Router, Request, Response } from "express";
import { agentSessionDb, agentConfigDb, messagesDb, db } from "../services/database";
import { randomUUID } from "crypto";
import { existsSync, rmSync } from "fs";
import { resolve } from "path";

/** 项目根目录 & Agent 会话工作目录根 */
const PROJECT_ROOT = resolve(import.meta.dir, "../../..");
const SESSION_WORK_ROOT = resolve(PROJECT_ROOT, ".agent-sessions");

const router = Router();

/**
 * GET /api/agent-sessions
 * 获取所有 Agent 会话列表
 * 支持过滤: ?agentId=&channel=&userId=&limit=
 */
router.get("/", (req: Request, res: Response) => {
  try {
    const { agentId, channel, userId, limit: limitStr } = req.query as Record<string, string | undefined>;
    const limit = limitStr ? parseInt(limitStr, 10) : undefined;

    const hasFilters = agentId || channel || userId;
    const sessions = hasFilters
      ? agentSessionDb.getFiltered({ agentId, channel, userId, limit })
      : agentSessionDb.getAll();

    // 批量查找 agent 显示名（缓存 map 避免重复查询）
    const agentNameCache = new Map<string, string>();
    const getAgentDisplayName = (aid: string | undefined): string | undefined => {
      if (!aid) return undefined;
      if (agentNameCache.has(aid)) return agentNameCache.get(aid);
      const config = agentConfigDb.getById(aid);
      const name = config?.displayName;
      if (name) agentNameCache.set(aid, name);
      return name;
    };

    res.json({
      success: true,
      data: sessions.map((s) => {
        return {
          id: s.id,
          title: s.title,
          agentId: s.agentId,
          agentDisplayName: getAgentDisplayName(s.agentId),
          userId: s.userId,
          sourceChannel: s.sourceChannel,
          executionStatus: s.executionStatus,
          channelName: s.channelName,
          messageCount: messagesDb.countBySession(s.id),
          createdAt: s.createdAt,
          updatedAt: s.updatedAt,
        };
      }),
    });
  } catch (error) {
    console.error("Failed to get agent sessions:", error);
    res.status(500).json({ success: false, error: "Failed to get sessions" });
  }
});

/**
 * POST /api/agent-sessions
 * 创建新的 Agent 会话
 */
router.post("/", (req: Request, res: Response) => {
  try {
    const { title } = req.body;
    const session = agentSessionDb.create({
      id: randomUUID(),
      title: title || "新对话",
    });
    res.json({
      success: true,
      data: {
        id: session.id,
        title: session.title,
        createdAt: session.createdAt,
      },
    });
  } catch (error) {
    console.error("Failed to create agent session:", error);
    res.status(500).json({ success: false, error: "Failed to create session" });
  }
});

/**
 * GET /api/agent-sessions/:id
 * 获取单个会话详情（消息从 messages 表读取）
 */
router.get("/:id", (req: Request, res: Response) => {
  try {
    const session = agentSessionDb.getById(req.params.id);
    if (!session) {
      res.status(404).json({ success: false, error: "Session not found" });
      return;
    }

    // 查找 agent 显示名
    let agentDisplayName: string | undefined;
    if (session.agentId) {
      const config = agentConfigDb.getById(session.agentId);
      agentDisplayName = config?.displayName;
    }

    // 从 messages 表读取消息（唯一数据源），会话详情不限条数
    const messages = messagesDb.getBySession(session.id, { limit: 0 });

    res.json({
      success: true,
      data: {
        id: session.id,
        title: session.title,
        sdkSessionId: session.sdkSessionId,
        userId: session.userId,
        agentId: session.agentId,
        agentDisplayName,
        sourceChannel: session.sourceChannel,
        executionStatus: session.executionStatus,
        channelName: session.channelName,
        messages: messages.map((m) => ({
          role: m.role,
          content: m.content,
          channel: m.channel,
          toolCalls: m.toolCalls,
          timestamp: m.createdAt,
          traceId: m.traceId,
          initiator: m.initiator,
          status: m.status,
          channelMeta: m.channelMeta,
        })),
        createdAt: session.createdAt,
        updatedAt: session.updatedAt,
      },
    });
  } catch (error) {
    console.error("Failed to get agent session:", error);
    res.status(500).json({ success: false, error: "Failed to get session" });
  }
});

/**
 * PATCH /api/agent-sessions/:id
 * 更新会话信息（标题等）
 */
router.patch("/:id", (req: Request, res: Response) => {
  try {
    const { title, sdkSessionId } = req.body;
    const updates: Partial<{ title: string; sdkSessionId: string }> = {};

    if (title !== undefined) updates.title = title;
    if (sdkSessionId !== undefined) updates.sdkSessionId = sdkSessionId;

    const session = agentSessionDb.update(req.params.id, updates);
    if (!session) {
      res.status(404).json({ success: false, error: "Session not found" });
      return;
    }
    res.json({
      success: true,
      data: { id: session.id, title: session.title },
    });
  } catch (error) {
    console.error("Failed to update agent session:", error);
    res.status(500).json({ success: false, error: "Failed to update session" });
  }
});

/**
 * DELETE /api/agent-sessions/:id
 * 一键删除会话：数据库记录 + 关联消息 + 执行链路 + Agent 工作目录
 */
router.delete("/:id", (req: Request, res: Response) => {
  try {
    const sessionId = req.params.id;

    // 1. 先查询 session 获取 workDir（删除前读取）
    const session = agentSessionDb.getById(sessionId);
    if (!session) {
      res.status(404).json({ success: false, error: "Session not found" });
      return;
    }

    // 2. 删除数据库记录
    agentSessionDb.delete(sessionId);
    messagesDb.deleteBySession(sessionId);

    // 3. 清理执行链路（execution_traces + execution_steps）
    try {
      const traces = db.prepare("SELECT id FROM execution_traces WHERE session_id = ?").all(sessionId) as { id: string }[];
      for (const trace of traces) {
        db.run("DELETE FROM execution_steps WHERE trace_id = ?", [trace.id]);
      }
      db.run("DELETE FROM execution_traces WHERE session_id = ?", [sessionId]);
    } catch (e) {
      console.warn("Failed to clean execution traces (table may not exist):", e);
    }

    // 4. 清理 Agent 工作目录
    const workDir = (session as any).work_dir || resolve(SESSION_WORK_ROOT, sessionId);
    if (workDir && existsSync(workDir)) {
      try {
        rmSync(workDir, { recursive: true, force: true });
        console.log(`Deleted agent work directory: ${workDir}`);
      } catch (e) {
        console.warn(`Failed to delete agent work directory ${workDir}:`, e);
      }
    }

    // 也尝试按 sessionId 命名的默认目录
    const defaultWorkDir = resolve(SESSION_WORK_ROOT, sessionId);
    if (defaultWorkDir !== workDir && existsSync(defaultWorkDir)) {
      try {
        rmSync(defaultWorkDir, { recursive: true, force: true });
        console.log(`Deleted default work directory: ${defaultWorkDir}`);
      } catch (e) {
        console.warn(`Failed to delete default work directory ${defaultWorkDir}:`, e);
      }
    }

    res.json({ success: true });
  } catch (error) {
    console.error("Failed to delete agent session:", error);
    res.status(500).json({ success: false, error: "Failed to delete session" });
  }
});

/**
 * POST /api/agent-sessions/:id/messages
 * 添加消息到会话（写入 messages 表）
 */
router.post("/:id/messages", (req: Request, res: Response) => {
  try {
    const { role, content, toolCalls, channel } = req.body;

    if (!role || !content) {
      res.status(400).json({ success: false, error: "role and content are required" });
      return;
    }

    // 确认 session 存在
    const session = agentSessionDb.getById(req.params.id);
    if (!session) {
      res.status(404).json({ success: false, error: "Session not found" });
      return;
    }

    // 写入 messages 表（唯一数据源）
    const message = messagesDb.insert({
      id: randomUUID(),
      sessionId: session.id,
      role,
      content,
      channel: channel || session.sourceChannel || "webui",
      toolCalls,
      status: "sent",
    });

    // 如果是第一条用户消息，自动更新标题
    if (role === "user") {
      const allMessages = messagesDb.getBySession(session.id, { limit: 5 });
      const userMessages = allMessages.filter(m => m.role === "user");
      if (userMessages.length === 1) {
        const autoTitle = content.substring(0, 30) + (content.length > 30 ? "..." : "");
        agentSessionDb.update(session.id, { title: autoTitle });
      }
    }

    // 更新 session 的 updated_at
    agentSessionDb.update(session.id, {});

    const messageCount = messagesDb.getBySession(session.id, { limit: 1000 }).length;

    res.json({
      success: true,
      data: {
        id: session.id,
        title: session.title,
        messageCount,
        messageId: message.id,
      },
    });
  } catch (error) {
    console.error("Failed to add message to agent session:", error);
    res.status(500).json({ success: false, error: "Failed to add message" });
  }
});

/**
 * PUT /api/agent-sessions/:id/messages
 * 清空会话消息（用于 reset 场景）
 */
router.put("/:id/messages", (req: Request, res: Response) => {
  try {
    const { messages } = req.body;

    if (!Array.isArray(messages)) {
      res.status(400).json({ success: false, error: "messages must be an array" });
      return;
    }

    const session = agentSessionDb.getById(req.params.id);
    if (!session) {
      res.status(404).json({ success: false, error: "Session not found" });
      return;
    }

    // 如果传入空数组，清空该 session 的所有消息
    if (messages.length === 0) {
      messagesDb.deleteBySession(req.params.id);
    }

    const messageCount = messagesDb.getBySession(session.id, { limit: 1000 }).length;

    res.json({
      success: true,
      data: {
        id: session.id,
        messageCount,
      },
    });
  } catch (error) {
    console.error("Failed to update agent session messages:", error);
    res.status(500).json({ success: false, error: "Failed to update messages" });
  }
});

export default router;
