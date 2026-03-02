import type { ReceivedMessage, MessageHandler } from "../types";
import { getClient } from "../client";
import { forwardToAgent } from "../services/agent-client";
import { getChatInfo } from "../services/message";
import { feishuConfig } from "../config";

/**
 * 消息事件处理器注册表
 * 允许外部注册自定义的消息处理逻辑
 */
const messageHandlers: MessageHandler[] = [];

const CACHE_TTL_MS = 10 * 60 * 1000;
const chatNameCache = new Map<string, { name: string; expiresAt: number }>();
const userNameCache = new Map<string, { name: string; expiresAt: number }>();

let tenantToken: { token: string; expiresAt: number } | null = null;

function getCachedChatName(chatId: string): string | undefined {
  const cached = chatNameCache.get(chatId);
  if (!cached) return undefined;
  if (cached.expiresAt < Date.now()) {
    chatNameCache.delete(chatId);
    return undefined;
  }
  return cached.name;
}

async function refreshChatName(chatId: string): Promise<void> {
  try {
    const chatInfo = await getChatInfo(chatId);
    const name = (chatInfo as any)?.data?.name;
    if (typeof name === "string" && name.trim()) {
      chatNameCache.set(chatId, { name: name.trim(), expiresAt: Date.now() + CACHE_TTL_MS });
    }
  } catch {
    // 群名刷新失败不影响主流程
  }
}

async function getTenantAccessToken(): Promise<string> {
  if (tenantToken && tenantToken.expiresAt > Date.now()) {
    return tenantToken.token;
  }
  const res = await fetch("https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      app_id: feishuConfig.appId,
      app_secret: feishuConfig.appSecret,
    }),
  });
  const data = await res.json() as { code?: number; tenant_access_token?: string; expire?: number };
  if (data.code !== 0 || !data.tenant_access_token) {
    throw new Error(`获取 tenant_access_token 失败: ${JSON.stringify(data)}`);
  }
  tenantToken = {
    token: data.tenant_access_token,
    expiresAt: Date.now() + (data.expire ?? 7200) * 1000 - 60_000,
  };
  return tenantToken.token;
}

/**
 * 用原生 fetch 查询飞书用户昵称（绕过 SDK 的 axios，避免 Bun 兼容性问题）。
 * 带本地缓存 + 重试。
 */
async function resolveUserName(openId: string): Promise<string | undefined> {
  const cached = userNameCache.get(openId);
  if (cached && cached.expiresAt > Date.now()) {
    return cached.name;
  }

  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const token = await getTenantAccessToken();
      const res = await fetch(
        `https://open.feishu.cn/open-apis/contact/v3/users/${openId}?user_id_type=open_id`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (!res.ok) {
        console.warn(`⚠️ 获取用户昵称 HTTP ${res.status} (${openId}), attempt=${attempt}`);
        if (attempt === 0) continue;
        return undefined;
      }
      const data = await res.json() as { code?: number; data?: { user?: { name?: string } } };
      if (data.code !== 0) {
        console.warn(`⚠️ 获取用户昵称 API 错误 code=${data.code} (${openId})`);
        return undefined;
      }
      const name = data.data?.user?.name;
      if (typeof name === "string" && name.trim()) {
        userNameCache.set(openId, { name: name.trim(), expiresAt: Date.now() + CACHE_TTL_MS });
        return name.trim();
      }
      return undefined;
    } catch (err) {
      if (attempt === 0) {
        console.warn(`⚠️ 获取用户昵称失败 (${openId}), 重试中...`);
        continue;
      }
      console.warn(`⚠️ 获取用户昵称最终失败 (${openId}):`, (err as Error).message);
    }
  }
  return undefined;
}

/** 注册消息处理器 */
export function onMessage(handler: MessageHandler): void {
  messageHandlers.push(handler);
}

/** 默认消息处理函数：收到消息后的处理逻辑 */
async function handleMessageReceive(data: any): Promise<void> {
  const { message, sender } = data;

  // 构造标准化消息对象
  const received: ReceivedMessage = {
    messageId: message.message_id,
    chatId: message.chat_id,
    messageType: message.message_type,
    content: message.content,
    sender: {
      senderId: sender.sender_id,
      senderType: sender.sender_type,
      tenantKey: sender.tenant_key,
    },
    createTime: message.create_time,
    mentionedBot:
      message.mentions?.some(
        (m: any) => m.key === "@_all" || m.id?.open_id
      ) ?? false,
  };

  console.log(
    `📨 收到消息 [${received.messageType}] from ${received.sender.senderId.open_id} in chat ${received.chatId}`
  );

  // 调用所有注册的处理器
  for (const handler of messageHandlers) {
    try {
      await handler(received);
    } catch (err) {
      console.error("❌ 消息处理器执行失败:", err);
    }
  }

  // 如果没有注册自定义处理器，使用 Agent 处理（开启时）或 echo 兜底
  if (messageHandlers.length === 0) {
    if (feishuConfig.agentEnabled) {
      await agentHandler(received);
    } else {
      await defaultEchoHandler(received);
    }
  }
}

/**
 * 飞书支持的用户消息类型
 * 只有用户消息事件才投递到 Agent，其他事件仅接收记录
 */
const USER_MESSAGE_TYPES = new Set([
  "text",       // 文本
  "image",      // 图片
  "audio",      // 语音
  "media",      // 视频
  "file",       // 文件
  "sticker",    // 表情包
  "post",       // 富文本
  "share_chat", // 分享群名片
  "share_user", // 分享个人名片
  "location",   // 位置
  "merge_forward", // 合并转发
]);

/**
 * Agent 消息处理器（统一渠道版）
 * 将飞书用户消息归一化为 IncomingMessage，转发到 server
 * 支持所有用户消息类型（文本 + 富媒体）
 * 不再等待 AI 回复——server 处理完毕后会通过 POST /api/feishu/send 回调
 */
async function agentHandler(msg: ReceivedMessage): Promise<void> {
  // 仅处理用户消息，避免机器人自己发出的消息再次回流造成“重复回复/多段回复”
  if (msg.sender.senderType && msg.sender.senderType !== "user") {
    console.log(`  ℹ️  跳过非用户发送者消息: senderType=${msg.sender.senderType}`);
    return;
  }

  // 判断聊天类型（chat_id 以 oc_ 开头是群聊）
  const chatType = msg.chatId.startsWith("oc_") ? "group" : "p2p";

  // 非用户消息类型：仅记录，不投递到 Agent
  if (!USER_MESSAGE_TYPES.has(msg.messageType)) {
    console.log(`  ℹ️  非用户消息类型 (${msg.messageType})，已接收但不投递到 Agent`);
    return;
  }

  // 多 Agent 架构：群内每个 bot 都收到所有消息，统一投递到各自的 Agent
  // 不再要求 @mention 才响应（Agent 自行决定是否回复）
  if (chatType === "group") {
    console.log(`  📢 群聊消息，mentionedBot=${msg.mentionedBot}，投递到 Agent`);
  }

  // 提取消息内容
  let content: string;
  if (msg.messageType === "text") {
    // 文本消息：提取纯文本并清理 @mention 标记
    try {
      const parsed = JSON.parse(msg.content);
      content = (parsed.text || "").replace(/@_user_\d+/g, "").trim();
    } catch {
      return;
    }
    if (!content) return;
  } else {
    // 富媒体消息（图片/语音/视频/文件/表情/富文本等）：直接透传原始 content
    content = msg.content;
  }

  console.log(`  🤖 转发到 Agent [${msg.messageType}]: "${content.substring(0, 80)}${content.length > 80 ? "..." : ""}"`);

  // 并行解析用户昵称和会话名称
  const openId = msg.sender.senderId.open_id;
  let conversationName: string | undefined;
  if (chatType === "group") {
    conversationName = getCachedChatName(msg.chatId);
    if (!conversationName) {
      void refreshChatName(msg.chatId);
    }
  }
  const senderName = await resolveUserName(openId);

  const result = await forwardToAgent({
    channel: "feishu",
    channelUserId: openId,
    channelMessageId: msg.messageId,
    channelConversationId: msg.chatId,
    channelConversationName: conversationName,
    conversationType: chatType,
    messageType: msg.messageType,
    content,
    senderName,
    timestamp: parseInt(msg.createTime, 10) || Date.now(),
    channelMeta: {
      chatType,
      tenantKey: msg.sender.tenantKey,
      senderType: msg.sender.senderType,
    },
    agentId: feishuConfig.agentId || undefined,
  });

  if (!result.success) {
    // 转发失败时，直接回复错误信息
    try {
      const client = getClient();
      await client.im.message.reply({
        path: { message_id: msg.messageId },
        data: {
          content: JSON.stringify({ text: `⚠️ ${result.error || "Agent 服务暂不可用，请稍后重试"}` }),
          msg_type: "text",
        },
      });
    } catch (err) {
      console.error(`  ❌ 发送错误提示失败:`, err);
    }
  } else {
    console.log(`  ✅ 消息已转发到 Agent（异步处理中）`);
  }
}

/** 默认 echo 处理器（Agent 未开启时的兜底） */
async function defaultEchoHandler(msg: ReceivedMessage): Promise<void> {
  const client = getClient();

  if (msg.messageType !== "text") {
    console.log(`  ℹ️  跳过非文本消息: ${msg.messageType}`);
    return;
  }

  try {
    const content = JSON.parse(msg.content);
    const text = content.text || "";

    await client.im.message.reply({
      path: { message_id: msg.messageId },
      data: {
        content: JSON.stringify({
          text: `🤖 收到你的消息: "${text}"`,
        }),
        msg_type: "text",
      },
    });

    console.log(`  ✅ 已回复消息`);
  } catch (err) {
    console.error(`  ❌ 回复失败:`, err);
  }
}

export { handleMessageReceive };
