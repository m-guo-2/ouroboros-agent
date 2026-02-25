/**
 * 飞书统一消息发送端点
 *
 * POST /api/feishu/send
 * 统一的消息发送接口，支持多种消息类型、@用户、引用回复
 * 同时向后兼容 server 的 OutgoingMessage 格式
 */

import { Router, type Request, type Response } from "express";
import { getClient } from "../client";
import type { SendRequest, RichTextContent, ApiResponse } from "../types";

const router = Router();

// ==================== 旧格式兼容（server OutgoingMessage） ====================

interface OutgoingMessage {
  channel: "feishu";
  channelUserId: string;
  replyToChannelMessageId?: string;
  channelConversationId?: string;
  messageType: "text" | "image" | "file" | "rich_text";
  content: string;
  channelMeta?: Record<string, unknown>;
}

// ==================== 统一发送端点 ====================

/**
 * POST /api/feishu/send
 *
 * 新格式（SendRequest）：content 为 JSON 对象，按 content.type 区分消息类型
 * 旧格式（OutgoingMessage）：content 为字符串，自动检测并走兼容逻辑
 */
router.post("/", async (req: Request, res: Response) => {
  try {
    // 向后兼容：content 为字符串时走旧 OutgoingMessage 逻辑
    if (typeof req.body.content === "string") {
      return await handleLegacyMessage(req.body as OutgoingMessage, res);
    }

    // 新格式
    return await handleSendRequest(req.body as SendRequest, res);
  } catch (err: any) {
    console.error("❌ 飞书消息发送失败:", err.message || err);
    res.status(500).json({
      success: false,
      error: err.message || "Failed to send message via Feishu",
    } as ApiResponse);
  }
});

// ==================== 新格式处理 ====================

async function handleSendRequest(req: SendRequest, res: Response) {
  // 参数校验
  if (!req.receiveId) {
    return res.status(400).json({ success: false, error: "receiveId is required" } as ApiResponse);
  }
  if (!req.content) {
    return res.status(400).json({ success: false, error: "content is required" } as ApiResponse);
  }

  const receiveIdType = req.receiveIdType || "chat_id";
  const { content, replyToMessageId, mentions } = req;

  // 根据 content.type 构建飞书消息
  const feishuMsg = buildFeishuMessage(content, mentions);

  const client = getClient();

  // 引用回复
  if (replyToMessageId) {
    try {
      await client.im.message.reply({
        path: { message_id: replyToMessageId },
        data: {
          content: feishuMsg.content,
          msg_type: feishuMsg.msgType,
        },
      });
      console.log(`  ✅ 飞书回复已发送 (reply to ${replyToMessageId})`);
    } catch (replyErr: any) {
      // 引用回复失败，降级为直接发送
      console.warn(`  ⚠️ 引用回复失败 (${replyErr.message || replyErr})，降级为直接发送`);
      await client.im.message.create({
        params: { receive_id_type: receiveIdType },
        data: {
          receive_id: req.receiveId,
          content: feishuMsg.content,
          msg_type: feishuMsg.msgType,
        },
      });
      console.log(`  ✅ 飞书消息已发送 (fallback to ${receiveIdType} ${req.receiveId})`);
    }
  }
  // 直接发送
  else {
    await client.im.message.create({
      params: { receive_id_type: receiveIdType },
      data: {
        receive_id: req.receiveId,
        content: feishuMsg.content,
        msg_type: feishuMsg.msgType,
      },
    });
    console.log(`  ✅ 飞书消息已发送 (to ${receiveIdType} ${req.receiveId})`);
  }

  res.json({ success: true } as ApiResponse);
}

// ==================== 消息构建 ====================

/**
 * 根据 content.type 构建飞书 SDK 需要的消息格式
 */
function buildFeishuMessage(
  content: SendRequest["content"],
  mentions?: string[]
): { content: string; msgType: string } {
  switch (content.type) {
    case "text": {
      let text = content.text;

      // 拼接 @用户 标签
      if (mentions?.length) {
        const atTags = mentions.map((uid) => `<at user_id="${uid}"></at>`).join(" ");
        text = `${atTags} ${text}`;
      }

      // 长文本自动转富文本（post），保留换行
      if (text.length >= 800) {
        return formatLongText(text);
      }

      return {
        content: JSON.stringify({ text }),
        msgType: "text",
      };
    }

    case "rich_text": {
      const richContent = content.content;

      // 如果有 mentions，在首行前插入 @标签
      if (mentions?.length) {
        const atElements: RichTextContent[] = mentions.map((uid) => ({
          tag: "at" as const,
          user_id: uid,
        }));
        // 在第一行头部插入 at 元素
        if (richContent.length > 0) {
          richContent[0] = [...atElements, ...richContent[0]];
        } else {
          richContent.push(atElements);
        }
      }

      return {
        content: JSON.stringify({
          zh_cn: {
            title: content.title || "",
            content: richContent,
          },
        }),
        msgType: "post",
      };
    }

    case "card": {
      if ("templateId" in content && content.templateId) {
        return {
          content: JSON.stringify({
            type: "template",
            data: {
              template_id: content.templateId,
              template_variable: content.templateVariable || {},
            },
          }),
          msgType: "interactive",
        };
      }

      if ("cardContent" in content && content.cardContent) {
        return {
          content: JSON.stringify(content.cardContent),
          msgType: "interactive",
        };
      }

      throw new Error("Card message requires either templateId or cardContent");
    }

    case "image": {
      return {
        content: JSON.stringify({ image_key: content.imageKey }),
        msgType: "image",
      };
    }

    case "file": {
      return {
        content: JSON.stringify({ file_key: content.fileKey }),
        msgType: "file",
      };
    }

    case "audio": {
      return {
        content: JSON.stringify({ file_key: content.fileKey }),
        msgType: "audio",
      };
    }

    case "video": {
      return {
        content: JSON.stringify({
          file_key: content.fileKey,
          image_key: content.imageKey,
        }),
        msgType: "media",
      };
    }

    default:
      throw new Error(`Unsupported content type: ${(content as any).type}`);
  }
}

/**
 * 长文本转飞书富文本（post）格式
 */
function formatLongText(text: string): { content: string; msgType: string } {
  const lines = text.split("\n");
  const postContent = lines.map((line) => [{ tag: "text" as const, text: line }]);

  return {
    content: JSON.stringify({
      zh_cn: {
        title: "",
        content: postContent,
      },
    }),
    msgType: "post",
  };
}

// ==================== 旧格式处理（向后兼容） ====================

/**
 * 处理旧 OutgoingMessage 格式（server 回调）
 * content 为纯字符串，messageType 为顶层字段
 */
async function handleLegacyMessage(msg: OutgoingMessage, res: Response) {
  if (!msg.content) {
    return res.status(400).json({ success: false, error: "content is required" } as ApiResponse);
  }

  if (!msg.channelConversationId && !msg.channelUserId) {
    return res.status(400).json({
      success: false,
      error: "channelConversationId or channelUserId is required",
    } as ApiResponse);
  }

  const client = getClient();
  const replyContent = formatLegacyMessage(msg.content, msg.messageType);

  // 优先引用回复
  if (msg.replyToChannelMessageId) {
    try {
      await client.im.message.reply({
        path: { message_id: msg.replyToChannelMessageId },
        data: {
          content: replyContent.content,
          msg_type: replyContent.msgType,
        },
      });
      console.log(`  ✅ [legacy] 飞书回复已发送 (reply to ${msg.replyToChannelMessageId})`);
    } catch (replyErr: any) {
      console.warn(`  ⚠️ [legacy] 引用回复失败 (${replyErr.message || replyErr})，降级为直接发送`);

      const target = msg.channelConversationId || msg.channelUserId;
      const idType = msg.channelConversationId ? "chat_id" : "open_id";

      await client.im.message.create({
        params: { receive_id_type: idType },
        data: {
          receive_id: target,
          content: replyContent.content,
          msg_type: replyContent.msgType,
        },
      });
      console.log(`  ✅ [legacy] 飞书消息已发送 (fallback to ${idType} ${target})`);
    }
  }
  // 发送到会话
  else if (msg.channelConversationId) {
    await client.im.message.create({
      params: { receive_id_type: "chat_id" },
      data: {
        receive_id: msg.channelConversationId,
        content: replyContent.content,
        msg_type: replyContent.msgType,
      },
    });
    console.log(`  ✅ [legacy] 飞书消息已发送 (to chat ${msg.channelConversationId})`);
  }
  // 发送给用户
  else {
    await client.im.message.create({
      params: { receive_id_type: "open_id" },
      data: {
        receive_id: msg.channelUserId,
        content: replyContent.content,
        msg_type: replyContent.msgType,
      },
    });
    console.log(`  ✅ [legacy] 飞书消息已发送 (to user ${msg.channelUserId})`);
  }

  res.json({ success: true } as ApiResponse);
}

/**
 * 旧格式消息格式化
 */
function formatLegacyMessage(
  text: string,
  messageType: string
): { content: string; msgType: string } {
  if (messageType !== "text") {
    return {
      content: JSON.stringify({ text }),
      msgType: "text",
    };
  }

  if (text.length < 800) {
    return {
      content: JSON.stringify({ text }),
      msgType: "text",
    };
  }

  return formatLongText(text);
}

export default router;
