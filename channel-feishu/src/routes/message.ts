import { Router } from "express";
import * as messageService from "../services/message";
import type { ApiResponse } from "../types";

const router = Router();

// ==================== 消息查询与管理 ====================
// 发送/回复消息已统一到 POST /api/feishu/send

/** DELETE /api/feishu/message/:messageId - 撤回消息 */
router.delete("/:messageId", async (req, res) => {
  try {
    const { messageId } = req.params;
    const result = await messageService.recallMessage(messageId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("撤回消息失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** GET /api/feishu/message/:messageId - 获取消息详情 */
router.get("/:messageId", async (req, res) => {
  try {
    const { messageId } = req.params;
    const result = await messageService.getMessage(messageId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取消息失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** GET /api/feishu/message/list/:chatId - 获取会话消息列表 */
router.get("/list/:chatId", async (req, res) => {
  try {
    const { chatId } = req.params;
    const { pageSize, pageToken } = req.query;
    const result = await messageService.getMessageList(
      chatId,
      Number(pageSize) || 20,
      pageToken as string | undefined
    );
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取消息列表失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

// ==================== 群组管理 ====================

/** GET /api/feishu/message/chat/:chatId - 获取群信息 */
router.get("/chat/:chatId", async (req, res) => {
  try {
    const { chatId } = req.params;
    const result = await messageService.getChatInfo(chatId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取群信息失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** GET /api/feishu/message/chat/:chatId/members - 获取群成员列表 */
router.get("/chat/:chatId/members", async (req, res) => {
  try {
    const { chatId } = req.params;
    const result = await messageService.getChatMembers(chatId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取群成员失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** POST /api/feishu/message/chat - 创建群组 */
router.post("/chat", async (req, res) => {
  try {
    const { name, description, userIdList } = req.body;
    if (!name) {
      res.status(400).json({ success: false, error: "name is required" } as ApiResponse);
      return;
    }

    const result = await messageService.createChat({ name, description, userIdList });
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("创建群组失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

export default router;
