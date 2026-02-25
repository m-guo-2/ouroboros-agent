/**
 * Server Client — Agent 调用 Server Data API 的 HTTP 客户端
 *
 * Agent 通过这些方法读写数据，自身不持有任何状态。
 */

const SERVER_URL = process.env.AGENT_SERVER_URL || "http://localhost:1997";
const CHANNEL_SEND_TOKEN =
  process.env.AGENT_CHANNEL_SEND_TOKEN ||
  process.env.AGENT_SEND_TOKEN ||
  "local-agent-send-token";
const CHANNEL_SEND_SOURCE = process.env.AGENT_SEND_SOURCE || "agent-sdk-runner";

/** Agent 配置 */
export interface AgentConfig {
  id: string;
  displayName: string;
  systemPrompt: string;
  /** @deprecated 使用 provider + model 代替 */
  modelId?: string;
  /** LLM 提供商：moonshot / anthropic / openai / zhipu 等 */
  provider?: string;
  /** 模型名称：kimi-k2.5 / claude-sonnet-4-20250514 等 */
  model?: string;
  skills: string[];
  channels: Array<{ channelType: string; channelIdentifier: string }>;
  isActive: boolean;
}

/** 模型配置（从 Server 获取，通过参数传递，不走 env） */
export interface ModelConfig {
  id: string;
  name: string;
  provider: "claude" | "openai" | "kimi" | "glm";
  baseUrl?: string;
  apiKey?: string;
  model: string;
  maxTokens: number;
  temperature: number;
}

/** 记忆数据 */
export interface MemoryData {
  summary: string;
  facts: Array<{
    id: string;
    category: string;
    fact: string;
    createdAt?: string;
  }>;
}

/** Session 数据 */
export interface SessionData {
  id: string;
  title: string;
  sdkSessionId?: string;
  userId?: string;
  agentId?: string;
  sourceChannel?: string;
  sessionKey?: string;
  channelConversationId?: string;
  workDir?: string;
  executionStatus?: string;
  messages: unknown[];
  createdAt?: string;
  updatedAt?: string;
}

/** 消息数据 */
export interface MessageData {
  id: string;
  sessionId: string;
  role: string;
  content: string;
  messageType?: string;
  channel?: string;
  toolCalls?: unknown[];
  senderName?: string;
  senderId?: string;
  createdAt?: string;
}

/** Skill 上下文 */
export interface SkillToolDefinition {
  name: string;
  description: string;
  input_schema: {
    type: "object";
    properties: Record<string, unknown>;
    required?: string[];
  };
}

export interface SkillToolExecutor {
  type: "http" | "script" | "internal";
  url?: string;
  method?: string;
  command?: string;
  handler?: string;
}

export interface SkillContext {
  systemPromptAddition: string;
  tools: SkillToolDefinition[];
  toolExecutors: Record<string, SkillToolExecutor>;
  skillDocs: Record<string, string>;
}

/** Agent 上报给 Server 的执行事件 */
export interface TraceEventPayload {
  traceId: string;
  sessionId: string;
  agentId?: string;
  userId?: string;
  channel?: string;
  type: "start" | "thinking" | "tool_call" | "tool_result" | "content" | "error" | "done" | "model_io";
  timestamp: number;
  /** Agent Loop 的迭代轮次（从 1 开始） */
  iteration?: number;
  initiator?: "user" | "agent" | "system";
  thinking?: string;
  /** thinking 来源：model = 模型推理, system = 系统状态日志 */
  source?: "model" | "system";
  toolCallId?: string;
  toolName?: string;
  toolInput?: unknown;
  toolResult?: unknown;
  toolDuration?: number;
  toolSuccess?: boolean;
  content?: string;
  error?: string;
  usage?: { inputTokens: number; outputTokens: number; totalCostUsd: number };
  /** 模型 I/O 观测：每次 LLM 调用的完整输入/输出摘要 */
  modelInput?: unknown;
  modelOutput?: unknown;
}

export class ServerClient {
  readonly baseUrl: string;

  constructor(baseUrl?: string) {
    this.baseUrl = baseUrl || SERVER_URL;
  }

  // ==================== 读操作 ====================

  /** 获取 Agent 配置 */
  async getAgentConfig(agentId: string): Promise<AgentConfig | null> {
    const res = await this.get(`/api/data/agents/${agentId}`);
    return res?.data || null;
  }

  /** @deprecated 使用 getProviderCredentials 代替 */
  async getModelConfig(modelId: string): Promise<ModelConfig | null> {
    const res = await this.get(`/api/data/models/${modelId}`);
    return res?.data || null;
  }

  /** 根据 provider 名称获取 API Key 和 Base URL（从 settings 表） */
  async getProviderCredentials(provider: string): Promise<{ provider: string; apiKey: string; baseUrl: string } | null> {
    const res = await this.get(`/api/data/provider-credentials/${provider}`);
    return res?.data || null;
  }

  /** 获取编译后的 skill 上下文 */
  async getSkillsContext(agentId: string): Promise<SkillContext | null> {
    const res = await this.get(`/api/data/agents/${agentId}/skills-context`);
    return res?.data || null;
  }

  /** 按 session key 查找 session（群聊用 conversationId，私聊用 userId） */
  async findSessionByKey(agentId: string, sessionKey: string): Promise<SessionData | null> {
    const params = new URLSearchParams({ agentId, sessionKey });
    const res = await this.get(`/api/data/sessions/by-key?${params}`);
    return res?.data || null;
  }

  /** 按 ID 获取 session */
  async getSession(sessionId: string): Promise<SessionData | null> {
    const res = await this.get(`/api/data/sessions/${sessionId}`);
    return res?.data || null;
  }

  /** 获取 session 消息历史 */
  async getSessionMessages(sessionId: string, limit = 20): Promise<MessageData[]> {
    const res = await this.get(`/api/data/sessions/${sessionId}/messages?limit=${limit}`);
    return res?.data || [];
  }

  /** 获取记忆 */
  async getMemory(agentId: string, userId: string): Promise<MemoryData> {
    const res = await this.get(`/api/data/memory/${agentId}/${userId}`);
    return res?.data || { summary: "", facts: [] };
  }

  /** 获取中断的 session 列表（断点续传） */
  async getInterruptedSessions(): Promise<SessionData[]> {
    const res = await this.get("/api/data/sessions-interrupted");
    return res?.data || [];
  }

  // ==================== 写操作 ====================

  /** 创建 session */
  async createSession(session: {
    id: string;
    agentId: string;
    userId: string;
    channel: string;
    /** session 唯一标识 = channel:uniqueId */
    sessionKey?: string;
    channelConversationId?: string;
    workDir?: string;
    sdkSessionId?: string;
    title?: string;
  }): Promise<SessionData> {
    const res = await this.post("/api/data/sessions", session);
    return res.data;
  }

  /** 更新 session */
  async updateSession(sessionId: string, updates: {
    sdkSessionId?: string;
    title?: string;
    sessionKey?: string;
    channelConversationId?: string;
    workDir?: string;
    executionStatus?: string;
  }): Promise<void> {
    await this.put(`/api/data/sessions/${sessionId}`, updates);
  }

  /** 保存消息 */
  async saveMessage(message: {
    sessionId: string;
    role: string;
    content: string;
    messageType?: string;
    channel?: string;
    toolCalls?: unknown[];
    traceId?: string;
    initiator?: string;
    senderName?: string;
    senderId?: string;
  }): Promise<MessageData> {
    const res = await this.post("/api/data/messages", {
      id: crypto.randomUUID(),
      ...message,
    });
    return res.data;
  }

  /** 更新记忆摘要 */
  async updateMemorySummary(agentId: string, userId: string, summary: string): Promise<void> {
    await this.put(`/api/data/memory/${agentId}/${userId}/summary`, { summary });
  }

  /** 添加记忆事实 */
  async addMemoryFact(agentId: string, userId: string, fact: {
    category: string;
    fact: string;
    sourceChannel?: string;
    sourceSessionId?: string;
  }): Promise<void> {
    await this.post(`/api/data/memory/${agentId}/${userId}/facts`, fact);
  }

  /** 发送消息到渠道 */
  async sendToChannel(message: {
    channel: string;
    channelUserId: string;
    content: string;
    messageType?: string;
    channelConversationId?: string;
    sessionId: string;
    traceId: string;
  }): Promise<void> {
    await this.post("/api/data/channels/send", message, {
      "x-agent-send-token": CHANNEL_SEND_TOKEN,
      "x-agent-source": CHANNEL_SEND_SOURCE,
    });
  }

  // ==================== 执行链路上报 ====================

  /**
   * 上报执行事件到 Server（fire-and-forget）
   *
   * Agent 在 processMessage 每一步都调用此方法，
   * Server 收到后：1) 持久化到 execution_traces/steps 表
   *                2) 推送到 observation bus（实时 SSE → MonitorView）
   *
   * 设计：不等待响应，不阻塞 Agent 主循环。
   * 异常静默吞掉（上报失败不影响 Agent 执行）。
   */
  reportTraceEvent(event: TraceEventPayload): void {
    this.post("/api/traces/events", event).catch(() => {
      // 静默：上报失败不影响 Agent 执行
    });
  }

  /**
   * 同步上报执行事件（关键节点用：例如消息入队时的 start）
   */
  async reportTraceEventSync(event: TraceEventPayload): Promise<void> {
    await this.post("/api/traces/events", event);
  }

  /**
   * 批量上报执行事件
   */
  reportTraceEvents(events: TraceEventPayload[]): void {
    if (events.length === 0) return;
    this.post("/api/traces/events", events).catch(() => {});
  }

  // ==================== Agent 生命周期 ====================

  /** 注册 Agent 实例 */
  async register(id: string, url: string, version?: string): Promise<void> {
    await this.post("/api/lifecycle/register", { id, url, version });
  }

  /** 发送心跳 */
  async heartbeat(id: string): Promise<void> {
    await this.post("/api/lifecycle/heartbeat", { id });
  }

  // ==================== HTTP 基础方法 ====================

  private async get(path: string): Promise<any> {
    try {
      const response = await fetch(`${this.baseUrl}${path}`);
      if (!response.ok) return null;
      return response.json();
    } catch {
      return null;
    }
  }

  private async post(path: string, body: unknown, extraHeaders?: Record<string, string>): Promise<any> {
    const response = await fetch(`${this.baseUrl}${path}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(extraHeaders || {}),
      },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Server API error: ${response.status} - ${error}`);
    }
    return response.json();
  }

  private async put(path: string, body: unknown): Promise<any> {
    const response = await fetch(`${this.baseUrl}${path}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Server API error: ${response.status} - ${error}`);
    }
    return response.json();
  }
}
