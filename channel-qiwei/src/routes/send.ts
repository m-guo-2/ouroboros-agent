/**
 * 企微消息发送端点（供 server 的 QiWei 适配器回调）
 *
 * POST /api/qiwei/send
 * 接收 server 的 OutgoingMessage 格式，通过 QiWe API 发送消息
 */

import { Router, type Request, type Response } from "express";
import * as messageService from "../services/message";

const router = Router();

/**
 * OutgoingMessage 格式（与 server 的 channel-types.ts 一致）
 */
interface OutgoingMessage {
  channel: "qiwei";
  channelUserId: string;
  replyToChannelMessageId?: string;
  channelConversationId?: string;
  messageType: "text" | "image" | "file" | "rich_text";
  content: string;
  channelMeta?: Record<string, unknown>;
}

/**
 * POST /api/qiwei/send
 * server 处理完消息后，通过此端点将回复发回企微
 */
router.post("/", async (req: Request, res: Response) => {
  const msg = req.body as OutgoingMessage;

  // 基本校验
  if (!msg.content) {
    return res.status(400).json({ success: false, error: "content is required" });
  }

  // 目标 ID：优先 channelConversationId（群/会话），否则 channelUserId（个人）
  const toId = msg.channelConversationId || msg.channelUserId;
  if (!toId) {
    return res.status(400).json({
      success: false,
      error: "channelConversationId or channelUserId is required",
    });
  }

  try {
    switch (msg.messageType) {
      case "text":
        await messageService.sendText(toId, msg.content);
        break;

      case "image":
        // content 为图片 URL
        await messageService.sendImage(toId, msg.content);
        break;

      case "file":
        // content 为文件 URL，meta 中可包含 fileName
        const fileName = (msg.channelMeta?.fileName as string) || "file";
        await messageService.sendFile(toId, msg.content, fileName);
        break;

      case "rich_text":
        // rich_text 退化为纯文本发送
        await messageService.sendText(toId, msg.content);
        break;

      default:
        await messageService.sendText(toId, msg.content);
    }

    console.log(`  ✅ 企微消息已发送 (to ${toId})`);
    res.json({ success: true });
  } catch (err: any) {
    console.error(`  ❌ 企微消息发送失败:`, err.message || err);
    res.status(500).json({
      success: false,
      error: err.message || "Failed to send message via QiWei",
    });
  }
});

export default router;
