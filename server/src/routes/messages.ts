/**
 * 消息查询路由
 *
 * GET /api/messages?sessionId=xxx&limit=50&before=timestamp  -- 按会话分页查消息
 * GET /api/messages/:id                                       -- 获取单条消息详情
 */

import { Router, type Request, type Response } from "express";
import { messagesDb } from "../services/database";

const router = Router();

/**
 * GET /api/messages
 * 按会话查询消息（分页）
 *
 * Query params:
 *   sessionId (必填) - 会话ID
 *   limit     (可选) - 返回条数，默认 50
 *   before    (可选) - 返回此时间戳之前的消息（ISO string），用于向前翻页
 */
router.get("/", (req: Request, res: Response) => {
  const { sessionId, limit, before } = req.query;

  if (!sessionId) {
    return res.status(400).json({
      success: false,
      error: "Missing required query param: sessionId",
    });
  }

  try {
    const messages = messagesDb.getBySession(sessionId as string, {
      limit: limit ? Number(limit) : 50,
      before: before as string | undefined,
    });

    res.json({
      success: true,
      data: messages,
      count: messages.length,
    });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: errorMessage });
  }
});

/**
 * GET /api/messages/:id
 * 获取单条消息详情
 */
router.get("/:id", (req: Request, res: Response) => {
  const { id } = req.params;

  try {
    const message = messagesDb.getById(id);
    if (!message) {
      return res.status(404).json({ success: false, error: "Message not found" });
    }

    res.json({ success: true, data: message });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: errorMessage });
  }
});

export default router;
