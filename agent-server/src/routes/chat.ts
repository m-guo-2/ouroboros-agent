import { Router, Request, Response } from "express";
import {
  createConversation,
  getConversation,
  getAllConversations,
  deleteConversation,
  updateConversationTitle,
  chat,
  switchModel,
} from "../services/agent";
import { StreamChunk } from "../services/models";

const router = Router();

// 获取所有会话
router.get("/conversations", (_req: Request, res: Response) => {
  const conversations = getAllConversations();
  res.json({
    success: true,
    data: conversations.map((c) => ({
      id: c.id,
      title: c.title,
      modelId: c.modelId,
      messageCount: c.messages.length - 1, // 排除 system 消息
      createdAt: c.createdAt,
      updatedAt: c.updatedAt,
    })),
  });
});

// 创建新会话
router.post("/conversations", (req: Request, res: Response) => {
  const { modelId, title } = req.body;
  if (!modelId) {
    res.status(400).json({ success: false, error: "modelId is required" });
    return;
  }

  const conversation = createConversation(modelId, title);
  res.json({
    success: true,
    data: {
      id: conversation.id,
      title: conversation.title,
      modelId: conversation.modelId,
    },
  });
});

// 获取单个会话详情
router.get("/conversations/:id", (req: Request, res: Response) => {
  const conversation = getConversation(req.params.id);
  if (!conversation) {
    res.status(404).json({ success: false, error: "Conversation not found" });
    return;
  }

  res.json({
    success: true,
    data: {
      id: conversation.id,
      title: conversation.title,
      modelId: conversation.modelId,
      messages: conversation.messages.filter((m) => m.role !== "system"),
      createdAt: conversation.createdAt,
      updatedAt: conversation.updatedAt,
    },
  });
});

// 更新会话标题
router.patch("/conversations/:id", (req: Request, res: Response) => {
  const { title } = req.body;
  if (!title) {
    res.status(400).json({ success: false, error: "title is required" });
    return;
  }

  const conversation = updateConversationTitle(req.params.id, title);
  if (!conversation) {
    res.status(404).json({ success: false, error: "Conversation not found" });
    return;
  }

  res.json({ success: true, data: { id: conversation.id, title: conversation.title } });
});

// 删除会话
router.delete("/conversations/:id", (req: Request, res: Response) => {
  const deleted = deleteConversation(req.params.id);
  if (!deleted) {
    res.status(404).json({ success: false, error: "Conversation not found" });
    return;
  }
  res.json({ success: true });
});

// 切换会话模型
router.post("/conversations/:id/model", (req: Request, res: Response) => {
  const { modelId } = req.body;
  if (!modelId) {
    res.status(400).json({ success: false, error: "modelId is required" });
    return;
  }

  const conversation = switchModel(req.params.id, modelId);
  if (!conversation) {
    res.status(404).json({ success: false, error: "Conversation not found" });
    return;
  }

  res.json({ success: true, data: { modelId: conversation.modelId } });
});

// 发送消息（SSE 流式响应）
router.post("/conversations/:id/chat", async (req: Request, res: Response) => {
  const { message } = req.body;
  if (!message) {
    res.status(400).json({ success: false, error: "message is required" });
    return;
  }

  const conversation = getConversation(req.params.id);
  if (!conversation) {
    res.status(404).json({ success: false, error: "Conversation not found" });
    return;
  }

  // 设置 SSE 头
  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");
  res.setHeader("X-Accel-Buffering", "no");

  const sendEvent = (event: string, data: unknown) => {
    res.write(`event: ${event}\n`);
    res.write(`data: ${JSON.stringify(data)}\n\n`);
  };

  try {
    await chat(req.params.id, message, (chunk: StreamChunk) => {
      switch (chunk.type) {
        case "text":
          sendEvent("text", { content: chunk.content });
          break;
        case "tool_use":
          sendEvent("tool_use", {
            name: chunk.toolName,
            input: chunk.toolInput,
            id: chunk.toolId,
          });
          break;
        case "done":
          sendEvent("done", {});
          break;
        case "error":
          sendEvent("error", { message: chunk.content });
          break;
      }
    });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : "Unknown error";
    sendEvent("error", { message: errorMessage });
  }

  res.end();
});

export default router;
