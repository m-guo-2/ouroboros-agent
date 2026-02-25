/**
 * Agent 客户端（统一渠道版）
 * channel-feishu 将飞书消息归一化后，POST 到 server 的 /api/channels/incoming
 * server 异步处理后，通过 POST /api/feishu/send 回调发送回复
 */

import { feishuConfig } from "../config";

const AGENT_SERVER_URL = feishuConfig.agentServerUrl;

/**
 * 统一入站消息格式（与 server 的 IncomingMessage 一致）
 * messageType 支持所有用户消息类型：text/image/audio/video/file/sticker/post/...
 */
interface IncomingMessage {
  channel: "feishu";
  channelUserId: string;
  channelMessageId: string;
  channelConversationId?: string;
  /** 渠道平台内的会话名称（群名/私聊对方昵称） */
  channelConversationName?: string;
  conversationType?: "p2p" | "group";
  messageType: string;
  content: string;
  senderName?: string;
  timestamp: number;
  channelMeta?: Record<string, unknown>;
  /** 目标 Agent ID — 标识本 bot 对应哪个 Agent */
  agentId?: string;
}

/**
 * 将飞书消息转发到 server 统一渠道入口
 * 立即返回（server 返回 202 Accepted），不等待 AI 处理结果
 * 处理结果会通过 server → POST /api/feishu/send 回调返回
 */
export async function forwardToAgent(msg: IncomingMessage): Promise<{ success: boolean; error?: string }> {
  try {
    const response = await fetch(`${AGENT_SERVER_URL}/api/channels/incoming`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(msg),
    });

    if (!response.ok) {
      const errText = await response.text();
      console.error(`❌ 转发消息到 server 失败: ${response.status} - ${errText}`);
      return { success: false, error: `Agent server error: ${response.status}` };
    }

    return { success: true };
  } catch (err) {
    const error = err as Error;

    if (
      error.message.includes("ECONNREFUSED") ||
      error.message.includes("fetch failed")
    ) {
      console.error("❌ Agent 服务未启动，消息转发失败");
      return { success: false, error: "Agent 服务未启动" };
    }

    console.error(`❌ 转发消息到 server 失败: ${error.message}`);
    return { success: false, error: error.message };
  }
}

/**
 * 检查 Agent 服务是否可用
 */
export async function checkAgentHealth(): Promise<boolean> {
  try {
    const response = await fetch(`${AGENT_SERVER_URL}/health`);
    return response.ok;
  } catch {
    return false;
  }
}
