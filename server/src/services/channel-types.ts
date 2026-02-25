/**
 * 统一渠道类型定义
 * 定义所有渠道（飞书、企微、WebUI）共享的消息接口和适配器接口
 */

// 支持的渠道类型
export type ChannelType = "feishu" | "qiwei" | "webui";

// 消息类型
export type MessageType = "text" | "image" | "file" | "rich_text";

// 会话类型
export type ConversationType = "p2p" | "group";

/**
 * 统一入站消息格式
 * 所有渠道接收到的消息都会被归一化为此格式，发送到 server
 *
 * 多 Agent 架构：每个 Agent 是群里的一个 bot，各自独立接收消息。
 * agentId 标识消息是哪个 bot/Agent 收到的，由渠道 bot 转发时填入。
 */
export interface IncomingMessage {
  /** 来源渠道 */
  channel: ChannelType;
  /** 渠道平台内的用户ID（如飞书open_id、企微toId） */
  channelUserId: string;
  /** 渠道平台内的消息ID，用于去重 */
  channelMessageId: string;
  /** 渠道平台内的会话ID（如飞书chat_id），可选 */
  channelConversationId?: string;
  /** 渠道平台内的会话名称（如飞书群名、企微群名、私聊对方昵称），可选 */
  channelConversationName?: string;
  /** 会话类型：单聊 or 群聊 */
  conversationType?: ConversationType;
  /** 消息类型 */
  messageType: MessageType;
  /** 消息内容（文本内容或资源URL） */
  content: string;
  /** 发送者显示名 */
  senderName?: string;
  /** 消息时间戳（毫秒） */
  timestamp: number;
  /** 渠道特定的额外元数据 */
  channelMeta?: Record<string, unknown>;
  /**
   * 目标 Agent ID（agent_configs.id）
   * 由渠道 bot 在转发时填入，标识消息是哪个 bot/Agent 收到的。
   * 群内多个 Agent 各自通过自己的 bot 收到同一条消息，各自携带不同的 agentId。
   * 若为空则使用默认 Agent。
   */
  agentId?: string;
}

/**
 * 统一出站消息格式
 * server 处理完毕后，通过此格式推送回复到对应渠道
 */
export interface OutgoingMessage {
  /** 目标渠道 */
  channel: ChannelType;
  /** 目标渠道用户ID */
  channelUserId: string;
  /** 回复的原始消息ID（可选，用于引用回复） */
  replyToChannelMessageId?: string;
  /** 目标渠道会话ID（如飞书chat_id）*/
  channelConversationId?: string;
  /** 消息类型 */
  messageType: MessageType;
  /** 消息内容 */
  content: string;
  /** 渠道特定的额外元数据 */
  channelMeta?: Record<string, unknown>;
  /** 关联的会话ID（可选，用于消息存储） */
  sessionId?: string;
  /** @的用户ID列表（可选） */
  mentions?: string[];
  /** 关联 traceId（可选，用于 Monitor 可观测关联） */
  traceId?: string;
}

/**
 * 渠道适配器接口
 * 每个渠道需要实现此接口来发送消息
 */
export interface ChannelAdapter {
  /** 渠道类型标识 */
  type: ChannelType;
  /** 发送消息到渠道端 */
  send(message: OutgoingMessage): Promise<void>;
  /** 健康检查（可选） */
  healthCheck?(): Promise<boolean>;
}

/**
 * 入站消息处理结果
 */
export interface ProcessingResult {
  /** 是否成功 */
  success: boolean;
  /** 会话ID */
  sessionId?: string;
  /** 用户ID */
  userId?: string;
  /** 错误信息 */
  error?: string;
  /** 是否为重复消息 */
  duplicate?: boolean;
}
