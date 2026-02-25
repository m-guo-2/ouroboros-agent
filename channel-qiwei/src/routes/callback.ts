/**
 * 企微消息回调路由
 *
 * POST /webhook/callback — 接收 QiWe 平台推送的消息事件
 *
 * 回调数据结构（QiWe 平台推送）:
 * {
 *   code: 200,
 *   msg: "success",
 *   data: [{
 *     guid: "设备GUID",
 *     msgType: 1,          // 1=文本, 3=图片, 43=视频, 49=文件/链接
 *     msgData: { content: "消息文本", ... },
 *     senderId: "发送者ID",
 *     senderNickname: "发送者昵称",
 *     fromRoomId: "0" | "群ID",  // "0" 表示私聊
 *     msgSvrId: "服务端消息ID",
 *     createTime: 1234567890,
 *   }]
 * }
 */

import { Router, type Request, type Response } from "express";
import { forwardToAgent } from "../services/agent-client";
import { sendText } from "../services/message";
import { qiweiConfig } from "../config";

const router = Router();

/** QiWe 回调消息中的单条消息 */
interface QiWeiCallbackMessage {
  guid: string;
  msgType: number;
  msgData: {
    content?: string;
    [key: string]: unknown;
  };
  senderId: string;
  senderNickname?: string;
  fromRoomId: string;
  msgSvrId: string;
  createTime: number;
}

/** QiWe 回调请求体 */
interface QiWeiCallbackBody {
  code: number;
  msg: string;
  data: QiWeiCallbackMessage[];
}

/**
 * POST /webhook/callback
 * QiWe 平台推送消息事件到此端点
 * 必须 3 秒内响应，消息异步处理
 */
router.post("/", (req: Request, res: Response) => {
  const body = req.body as QiWeiCallbackBody;

  // 立即返回 200，确保 3 秒内响应
  res.status(200).json({ code: 200, msg: "ok" });

  // 异步处理每条消息
  if (body.data && Array.isArray(body.data)) {
    for (const msg of body.data) {
      handleCallbackMessage(msg).catch((err) => {
        console.error("❌ 处理企微回调消息失败:", err);
      });
    }
  }
});

/**
 * 用户消息类型映射
 * 只有用户消息事件才投递到 Agent，其他事件仅接收记录
 */
const USER_MSG_TYPE_MAP: Record<number, string> = {
  1: "text",
  3: "image",
  34: "voice",
  43: "video",
  47: "sticker",
  49: "file", // 49 也包含链接/小程序等，统一归为 file
};

/**
 * 处理单条回调消息
 * 只处理用户消息事件（包含文本和富媒体），其他事件仅记录不投递
 */
async function handleCallbackMessage(msg: QiWeiCallbackMessage): Promise<void> {
  const messageType = USER_MSG_TYPE_MAP[msg.msgType];
  const isGroup = msg.fromRoomId !== "0" && msg.fromRoomId !== "";
  const conversationType = isGroup ? "group" : "p2p";

  console.log(
    `📨 收到企微消息 [${messageType || `unknown(${msg.msgType})`}] from ${msg.senderId}${isGroup ? ` in group ${msg.fromRoomId}` : ""}`
  );

  // 非用户消息类型：仅记录，不投递到 Agent
  if (!messageType) {
    console.log(`  ℹ️  非用户消息事件 (msgType=${msg.msgType})，已接收但不投递到 Agent`);
    return;
  }

  // 提取消息内容
  let content: string;
  if (msg.msgType === 1) {
    // 文本消息：提取纯文本
    content = (msg.msgData?.content || "").trim();
    if (!content) return;
  } else {
    // 富媒体消息（图片/语音/视频/表情/文件）：将原始 msgData 序列化传递
    if (!msg.msgData || Object.keys(msg.msgData).length === 0) {
      console.log(`  ℹ️  富媒体消息 [${messageType}] 无有效数据，跳过`);
      return;
    }
    content = JSON.stringify(msg.msgData);
  }

  // Agent 联动
  if (qiweiConfig.agentEnabled) {
    await agentHandler(msg, content, messageType, conversationType, isGroup);
  } else {
    // Echo 模式仅处理文本消息
    if (msg.msgType === 1) {
      await echoHandler(msg, content, isGroup);
    } else {
      console.log(`  ℹ️  Echo 模式下跳过非文本消息: ${messageType}`);
    }
  }
}

/**
 * Agent 消息处理器
 * 归一化为 IncomingMessage 并转发到 server
 * 支持所有用户消息类型（文本 + 富媒体）
 */
async function agentHandler(
  msg: QiWeiCallbackMessage,
  content: string,
  messageType: string,
  conversationType: "p2p" | "group",
  isGroup: boolean
): Promise<void> {
  console.log(`  🤖 转发到 Agent [${messageType}]: "${content.substring(0, 80)}${content.length > 80 ? "..." : ""}"`);

  // 回复目标：群消息发到群，私聊发到发送者
  const replyToId = isGroup ? msg.fromRoomId : msg.senderId;

  // 会话名称：私聊用对方昵称，群聊暂无 API 获取群名
  const conversationName = isGroup ? undefined : msg.senderNickname;

  // agentId: 标识本 bot 对应哪个 Agent（多 Agent 架构）
  const result = await forwardToAgent({
    channel: "qiwei",
    channelUserId: msg.senderId,
    channelMessageId: msg.msgSvrId || `qw-${Date.now()}`,
    channelConversationId: replyToId,
    channelConversationName: conversationName,
    conversationType,
    messageType,
    content,
    senderName: msg.senderNickname,
    timestamp: (msg.createTime || 0) * 1000 || Date.now(), // QiWe 时间戳为秒级
    channelMeta: {
      guid: msg.guid,
      fromRoomId: msg.fromRoomId,
      msgType: msg.msgType,
    },
    agentId: qiweiConfig.agentId || undefined,
  });

  if (!result.success) {
    // 转发失败时直接回复错误提示
    try {
      await sendText(replyToId, `⚠️ ${result.error || "Agent 服务暂不可用，请稍后重试"}`);
    } catch (err) {
      console.error(`  ❌ 发送错误提示失败:`, err);
    }
  } else {
    console.log(`  ✅ 消息已转发到 Agent（异步处理中）`);
  }
}

/**
 * Echo 兜底处理器
 */
async function echoHandler(
  msg: QiWeiCallbackMessage,
  text: string,
  isGroup: boolean
): Promise<void> {
  const replyToId = isGroup ? msg.fromRoomId : msg.senderId;

  try {
    await sendText(replyToId, `🤖 收到你的消息: "${text}"`);
    console.log(`  ✅ 已回复消息`);
  } catch (err) {
    console.error(`  ❌ 回复失败:`, err);
  }
}

export default router;
