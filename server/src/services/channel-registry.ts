/**
 * 渠道适配器注册表
 * 管理所有渠道适配器的注册、查找和消息路由
 */

import type { ChannelType, ChannelAdapter, OutgoingMessage } from "./channel-types";
import { settingsDb, messagesDb } from "./database";
import { logger } from "./logger";
import { EventEmitter } from "events";

// 适配器注册表
const adapters = new Map<ChannelType, ChannelAdapter>();

// WebUI 用的内存事件总线（SSE push）
export const webuiEventBus = new EventEmitter();
webuiEventBus.setMaxListeners(100); // 允许多个 WebUI 客户端连接

/**
 * 注册一个渠道适配器
 */
export function registerAdapter(adapter: ChannelAdapter): void {
  adapters.set(adapter.type, adapter);
  logger.info(`Channel adapter registered: ${adapter.type}`);
}

/**
 * 获取一个渠道适配器
 */
export function getAdapter(channel: ChannelType): ChannelAdapter | undefined {
  return adapters.get(channel);
}

/**
 * 发送消息到指定渠道（先存后发）
 *
 * 1. 写入 messages 表 (status='sending')
 * 2. 调用渠道适配器发送
 * 3. 更新 status='sent' (成功) / 'failed' (失败)
 *
 * 返回存储的消息ID，便于调用方追踪
 */
export async function sendToChannel(message: OutgoingMessage): Promise<string | undefined> {
  const adapter = adapters.get(message.channel);
  if (!adapter) {
    logger.warn(`No adapter registered for channel: ${message.channel}`);
    return undefined;
  }

  // 1. 写入 messages 表
  const messageId = crypto.randomUUID();
  try {
    messagesDb.insert({
      id: messageId,
      sessionId: message.sessionId || "",
      role: "assistant",
      content: message.content,
      messageType: message.messageType,
      channel: message.channel,
      replyToMessageId: message.replyToChannelMessageId,
      mentions: message.mentions,
      channelMeta: message.channelMeta,
      traceId: message.traceId,
      initiator: "agent",
      status: "sending",
    });
  } catch (dbErr) {
    logger.error(`Failed to store outgoing message`, { error: dbErr });
    // 存储失败不阻塞发送
  }

  // 2. 调用渠道适配器发送
  try {
    await adapter.send(message);

    // 3a. 发送成功，更新状态
    messagesDb.updateStatus(messageId, "sent");

    logger.debug(`Message sent to ${message.channel}`, {
      messageId,
      channelUserId: message.channelUserId,
      messageType: message.messageType,
    });
    return messageId;
  } catch (error) {
    // 3b. 发送失败，更新状态
    messagesDb.updateStatus(messageId, "failed");

    logger.error(`Failed to send message to ${message.channel}`, { error });
    throw error;
  }
}

/**
 * 检查所有渠道的健康状态
 */
export async function healthCheckAll(): Promise<Record<ChannelType, boolean>> {
  const results: Record<string, boolean> = {};
  for (const [type, adapter] of adapters) {
    try {
      results[type] = adapter.healthCheck ? await adapter.healthCheck() : true;
    } catch {
      results[type] = false;
    }
  }
  return results as Record<ChannelType, boolean>;
}

/**
 * 获取所有已注册的渠道类型
 */
export function getRegisteredChannels(): ChannelType[] {
  return Array.from(adapters.keys());
}

// ==================== 内置适配器实现 ====================

/**
 * 飞书适配器：通过 HTTP 调用 channel-feishu 的 /api/feishu/send 端点
 */
function createFeishuAdapter(): ChannelAdapter {
  const port = settingsDb.get("general.feishu_port") || "1999";
  const baseUrl = `http://localhost:${port}`;

  return {
    type: "feishu",
    async send(message: OutgoingMessage): Promise<void> {
      const response = await fetch(`${baseUrl}/api/feishu/send`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(message),
      });
      if (!response.ok) {
        const error = await response.text();
        throw new Error(`Feishu send error: ${response.status} - ${error}`);
      }
    },
    async healthCheck(): Promise<boolean> {
      try {
        const response = await fetch(`${baseUrl}/api/health`);
        return response.ok;
      } catch {
        return false;
      }
    },
  };
}

/**
 * 企微适配器：通过 HTTP 调用 channel-qiwei 的 /api/qiwei/send 端点
 */
function createQiweiAdapter(): ChannelAdapter {
  const port = settingsDb.get("general.qiwei_port") || "2000";
  const baseUrl = `http://localhost:${port}`;

  return {
    type: "qiwei",
    async send(message: OutgoingMessage): Promise<void> {
      const response = await fetch(`${baseUrl}/api/qiwei/send`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(message),
      });
      if (!response.ok) {
        const error = await response.text();
        throw new Error(`QiWei send error: ${response.status} - ${error}`);
      }
    },
    async healthCheck(): Promise<boolean> {
      try {
        const response = await fetch(`${baseUrl}/api/health`);
        return response.ok;
      } catch {
        return false;
      }
    },
  };
}

/**
 * WebUI 适配器：通过内存事件总线推送 SSE 消息
 */
function createWebuiAdapter(): ChannelAdapter {
  return {
    type: "webui",
    async send(message: OutgoingMessage): Promise<void> {
      // 通过事件总线发送，WebUI SSE 端点会监听并推送给客户端
      webuiEventBus.emit(`message:${message.channelUserId}`, message);
    },
    async healthCheck(): Promise<boolean> {
      return true; // WebUI 适配器总是可用的
    },
  };
}

/**
 * 初始化所有内置适配器
 */
export function initializeAdapters(): void {
  registerAdapter(createFeishuAdapter());
  registerAdapter(createQiweiAdapter());
  registerAdapter(createWebuiAdapter());
  logger.info(`Initialized ${adapters.size} channel adapters`);
}
