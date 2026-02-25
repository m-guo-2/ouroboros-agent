/**
 * Agent Runner — 将 ReAct Engine 接入现有基础设施
 *
 * 职责：
 *   1. 接收 ProcessRequest（来自消息路由）
 *   2. 从 ServerClient 加载配置（Agent 配置、Skills、模型凭据）
 *   3. 构建 ToolRegistry（注册内置工具 + Skill 工具）
 *   4. 创建 LLMClient（根据 provider 选择 Anthropic 或 OpenAI 兼容）
 *   5. 运行 Agent Loop，将事件上报到 Trace 系统
 *   6. 管理 Session 生命周期（创建、更新、队列串行化）
 *
 * 与旧版 sdk-runner.ts 的区别：
 *   - 不再依赖 @anthropic-ai/claude-agent-sdk
 *   - 不再启动 SDK 子进程
 *   - 100% 透明：每一步 Thought/Action/Observation 都通过 onEvent 上报
 */

import {
  ServerClient,
  type AgentConfig,
  type MessageData,
  type ModelConfig,
  type SkillContext,
  type TraceEventPayload,
} from "../services/server-client";
import { buildSystemPrompt } from "../services/context-composer";
import { runAgentLoop } from "./loop";
import { ToolRegistry } from "./tool-registry";
import { AnthropicClient } from "./llm-client";
import { updateProxyTarget } from "../services/api-proxy";
import { ensureSandboxDir } from "./sandbox";
import { FEISHU_TOOL_DEFS, executeFeishuTool } from "../services/feishu-tools";
import type {
  AgentEvent,
  AgentMessage,
  ContentBlock,
  LLMClient,
  ToolExecutor,
} from "./types";
import { resolve } from "node:path";
import { existsSync, mkdirSync } from "node:fs";

const PROJECT_ROOT = resolve(import.meta.dir, "../../..");
const SESSION_WORK_ROOT = resolve(PROJECT_ROOT, ".agent-sessions");

/** Agent 进程端口（proxy 挂在此端口，非 Anthropic provider 通过它转换格式） */
let agentPort = 1996;
export function setAgentPort(port: number): void {
  agentPort = port;
}

/** 最大工具调用轮次 */
const MAX_ITERATIONS = Number(process.env.AGENT_MAX_ITERATIONS || 25);

/** 历史消息加载条数上限 */
const HISTORY_LIMIT = Number(process.env.AGENT_HISTORY_LIMIT || 50);

/** 历史消息中 tool_result 内容截断长度 */
const TOOL_RESULT_TRUNCATE_LEN = 800;

// ==================== Request / Session Types ====================

export interface ProcessRequest {
  userId: string;
  agentId: string;
  content: string;
  channel: string;
  channelUserId: string;
  channelConversationId?: string;
  channelMessageId?: string;
  senderName?: string;
  messageId: string;
  sessionId?: string;
  traceId?: string;
}

interface SessionWorker {
  sessionId: string;
  sessionKey: string;
  workDir: string;
  queue: QueuedRequest[];
  processing: boolean;
  abortController?: AbortController;
  lastActivityAt: number;
  idleTimer?: ReturnType<typeof setTimeout>;
}

interface QueuedRequest extends ProcessRequest {
  sessionId: string;
  traceId: string;
  traceStarted?: boolean;
}

/** Session 空闲多久后从内存中驱逐 */
const SESSION_IDLE_TIMEOUT_MS = Number(
  process.env.AGENT_SESSION_IDLE_TIMEOUT_MS || 10 * 60 * 1000,
);

const sessionWorkers = new Map<string, SessionWorker>();
const enqueueLocks = new Map<string, Promise<void>>();
let shuttingDown = false;

// ==================== Trace Reporter ====================

function createTraceReporter(
  server: ServerClient,
  base: Pick<TraceEventPayload, "traceId" | "sessionId" | "agentId" | "userId" | "channel">,
) {
  return (event: Omit<TraceEventPayload, "traceId" | "sessionId" | "agentId" | "userId" | "channel">) => {
    const fullEvent = { ...base, ...event };

    // 控制台简洁日志
    const tag = `[react-engine] sid=${base.sessionId.slice(0, 8)} tid=${base.traceId.slice(0, 8)}`;
    if (event.type === "thinking") {
      const preview = (event.thinking || "").replace(/\s+/g, " ").slice(0, 100);
      console.log(`${tag} think: "${preview}"`);
    } else if (event.type === "tool_call") {
      console.log(`${tag} call: ${event.toolName} id=${event.toolCallId}`);
    } else if (event.type === "tool_result") {
      const ok = event.toolSuccess !== false ? "ok" : "fail";
      console.log(`${tag} result: ${event.toolName} ${ok} ${event.toolDuration}ms`);
    } else if (event.type === "done") {
      const u = event.usage;
      console.log(`${tag} done: in=${u?.inputTokens || 0} out=${u?.outputTokens || 0}`);
    } else if (event.type === "error") {
      console.error(`${tag} error: ${event.error}`);
    }

    server.reportTraceEvent(fullEvent);
  };
}

/** 将 AgentEvent 转化为 TraceEventPayload 格式 */
function agentEventToTracePayload(
  event: AgentEvent,
): Omit<TraceEventPayload, "traceId" | "sessionId" | "agentId" | "userId" | "channel"> {
  return {
    type: event.type,
    timestamp: event.timestamp,
    iteration: event.iteration,
    thinking: event.thinking,
    source: event.source,
    toolCallId: event.toolCallId,
    toolName: event.toolName,
    toolInput: event.toolInput,
    toolResult: event.toolResult,
    toolDuration: event.toolDuration,
    toolSuccess: event.toolSuccess,
    error: event.error,
    usage: event.usage,
    modelInput: event.modelInput,
    modelOutput: event.modelOutput,
  };
}

// ==================== Session Resolution ====================

function resolveSessionKey(
  channel: string,
  channelUserId: string,
  channelConversationId?: string,
): string {
  return `${channel}:${channelConversationId || channelUserId}`;
}

async function resolveOrCreateSession(
  server: ServerClient,
  request: ProcessRequest,
): Promise<{ sessionId: string; sessionKey: string; workDir: string }> {
  const sessionKey = resolveSessionKey(
    request.channel,
    request.channelUserId,
    request.channelConversationId,
  );

  let session = request.sessionId
    ? await server.getSession(request.sessionId)
    : null;

  if (!session) {
    session = await server.findSessionByKey(request.agentId, sessionKey);
  }

  if (!session) {
    const sessionId = request.sessionId || crypto.randomUUID();
    const workDir = resolve(SESSION_WORK_ROOT, sessionId);
    ensureSandboxDir(workDir);

    const title = request.content.substring(0, 30) +
      (request.content.length > 30 ? "..." : "");
    await server.createSession({
      id: sessionId,
      agentId: request.agentId,
      userId: request.userId,
      channel: request.channel,
      sessionKey,
      channelConversationId: request.channelConversationId,
      workDir,
      title,
    });

    return { sessionId, sessionKey, workDir };
  }

  const sessionId = session.id;
  const workDir = session.workDir || resolve(SESSION_WORK_ROOT, sessionId);
  ensureSandboxDir(workDir);

  return { sessionId, sessionKey, workDir };
}

// ==================== Message History ====================

/**
 * 序列化 ContentBlock[] → DB 存储格式（JSON 字符串）。
 */
function serializeBlocks(blocks: ContentBlock[]): string {
  return JSON.stringify(blocks);
}

/**
 * 反序列化 DB 记录 → ContentBlock[]。
 * messageType="structured" → JSON parse；否则视为纯文本。
 */
function deserializeContent(content: string, messageType?: string): string | ContentBlock[] {
  if (messageType === "structured") {
    try { return JSON.parse(content); } catch { return content; }
  }
  return content;
}

function toContentBlocks(content: string | ContentBlock[]): ContentBlock[] {
  if (typeof content === "string") return [{ type: "text" as const, text: content }];
  return content;
}

// ── DB → LLM 消息转换 ──

interface DbMessage {
  role: string;
  content: string;
  messageType?: string;
  senderName?: string;
  senderId?: string;
}

/**
 * DB 消息记录 → AgentMessage[]（Anthropic 内部格式）。
 *
 * 三种 DB role 的转换规则：
 *   - user        → role:"user", content 前拼 [senderName]（群聊区分身份）
 *   - assistant   → role:"assistant", 反序列化为 tool_use ContentBlock[]
 *   - tool_result → role:"user", 反序列化为 tool_result ContentBlock[]（Anthropic 要求）
 *
 * 最终保证 user/assistant 严格交替（Anthropic API 要求）。
 */
function dbMessagesToAgentMessages(dbMessages: DbMessage[]): AgentMessage[] {
  const messages: AgentMessage[] = [];

  for (const msg of dbMessages) {
    if (msg.role === "user") {
      const sender = msg.senderName || msg.senderId;
      const prefix = sender ? `[${sender}] ` : "";
      messages.push({
        role: "user",
        content: prefix + msg.content,
      });
    } else if (msg.role === "assistant") {
      const blocks = deserializeContent(msg.content, msg.messageType);
      // 只保留 tool_use blocks；text block 是模型内部推理，不应进入历史上下文
      const toolUseBlocks = (typeof blocks === "string" ? [] : blocks).filter(
        (b) => b.type === "tool_use",
      );
      if (toolUseBlocks.length === 0) continue;
      messages.push({
        role: "assistant",
        content: toolUseBlocks,
      });
    } else if (msg.role === "tool_result") {
      // Anthropic: tool_result 放在 role:"user" 消息里
      const blocks = deserializeContent(msg.content, msg.messageType);
      messages.push({
        role: "user",
        content: typeof blocks === "string" ? [{ type: "tool_result" as const, tool_use_id: "unknown", content: blocks }] : blocks,
      });
    }
    // system role 不进入 messages（由 systemPrompt 参数处理）
  }

  return ensureAlternation(messages);
}

/**
 * 保证消息严格 user/assistant 交替（Anthropic / OpenAI API 要求）。
 *
 * 合并策略：
 *   - 连续 assistant 消息 → 合并 ContentBlock[]
 *   - 连续 user 消息 → 合并 ContentBlock[]，保留 tool_result 块结构
 *   - 开头不能是 assistant 消息（LLM 要求第一条是 user）
 */
function ensureAlternation(messages: AgentMessage[]): AgentMessage[] {
  if (messages.length <= 1) return messages;

  const result: AgentMessage[] = [messages[0]];
  for (let i = 1; i < messages.length; i++) {
    const prev = result[result.length - 1];
    const curr = messages[i];
    if (prev.role === curr.role) {
      // 无论是 user 还是 assistant，统一合并 ContentBlock[]，保留块结构
      result[result.length - 1] = {
        role: prev.role,
        content: [...toContentBlocks(prev.content), ...toContentBlocks(curr.content)],
      };
    } else {
      result.push(curr);
    }
  }

  while (result.length > 0 && result[0].role !== "user") {
    result.shift();
  }
  return result;
}

/**
 * 移除历史消息中"孤儿" tool_result 块。
 *
 * 当历史按 HISTORY_LIMIT 截断时，可能出现 tool_result 消息对应的
 * assistant tool_use 消息已被截掉，导致 tool_call_id 在上下文中找不到。
 * OpenAI 兼容 API（Kimi/GLM 等）会因此返回 400 tool_call_id is not found。
 *
 * 修复：收集所有 assistant 消息里的 tool_use.id，过滤掉没有配对的 tool_result 块；
 * 清理后如果某条 user 消息的 content 变为空数组则整条丢弃。
 */
function removeOrphanedToolResults(messages: AgentMessage[]): AgentMessage[] {
  const toolUseIds = new Set<string>();
  let hasToolResult = false;
  for (const msg of messages) {
    if (msg.role === "assistant" && Array.isArray(msg.content)) {
      for (const block of msg.content) {
        if (block.type === "tool_use") toolUseIds.add(block.id);
      }
    } else if (msg.role === "user" && Array.isArray(msg.content)) {
      if (msg.content.some((b) => b.type === "tool_result")) hasToolResult = true;
    }
  }
  if (!hasToolResult) return messages;

  const result: AgentMessage[] = [];
  for (const msg of messages) {
    if (msg.role === "user" && Array.isArray(msg.content)) {
      const filtered = msg.content.filter(
        (b) => b.type !== "tool_result" || toolUseIds.has(b.tool_use_id),
      );
      if (filtered.length === 0) continue;
      result.push({ ...msg, content: filtered });
    } else {
      result.push(msg);
    }
  }
  return result;
}

/**
 * 移除历史消息中"孤儿" tool_use 块（与 removeOrphanedToolResults 对称）。
 *
 * 两种触发场景：
 *   1. 前一次执行中断/崩溃：assistant 的 tool_use 已存入 DB，但 tool_result 未存入。
 *   2. ensureAlternation 合并破坏：tool_result（role:user）和下一条用户消息（role:user）
 *      被 flattenUserContent 合并为纯文本，tool_result 结构丢失。
 *
 * 后果：OpenAI 兼容 API 要求 tool_calls 必须紧跟 tool 响应，缺失则返回 400。
 *
 * 修复：收集所有 user 消息里的 tool_result.tool_use_id，
 * 过滤掉没有配对的 tool_use 块；清理后 assistant 消息变空则整条丢弃。
 */
function removeOrphanedToolUses(messages: AgentMessage[]): AgentMessage[] {
  const toolResultIds = new Set<string>();
  let hasToolUse = false;
  for (const msg of messages) {
    if (msg.role === "user" && Array.isArray(msg.content)) {
      for (const block of msg.content) {
        if (block.type === "tool_result") toolResultIds.add(block.tool_use_id);
      }
    } else if (msg.role === "assistant" && Array.isArray(msg.content)) {
      if (msg.content.some((b) => b.type === "tool_use")) hasToolUse = true;
    }
  }
  if (!hasToolUse) return messages;

  const result: AgentMessage[] = [];
  for (const msg of messages) {
    if (msg.role === "assistant" && Array.isArray(msg.content)) {
      const filtered = msg.content.filter(
        (b) => b.type !== "tool_use" || toolResultIds.has(b.id),
      );
      if (filtered.length === 0) continue;
      result.push({ ...msg, content: filtered });
    } else {
      result.push(msg);
    }
  }
  return result;
}

/**
 * 截断历史消息中过大的 tool_result 内容，控制 token 开销。
 * tool_use 的 name + input 保持完整（agent 需要知道调了什么），
 * tool_result 的大块返回值截断。
 */
function truncateToolResults(
  messages: AgentMessage[],
  maxLen = TOOL_RESULT_TRUNCATE_LEN,
): AgentMessage[] {
  return messages.map((msg) => {
    if (typeof msg.content === "string") return msg;
    const blocks = msg.content;
    const hasLarge = blocks.some(
      (b) => b.type === "tool_result" && b.content.length > maxLen,
    );
    if (!hasLarge) return msg;
    return {
      ...msg,
      content: blocks.map((block) => {
        if (block.type === "tool_result" && block.content.length > maxLen) {
          return { ...block, content: block.content.slice(0, maxLen) + "\n...(truncated)" };
        }
        return block;
      }),
    };
  });
}

// ── Loop 输出 → DB 存储转换 ──

interface PersistableMessage {
  role: "user" | "assistant" | "tool_result";
  content: string;
  messageType: string;
}

/**
 * 将 agent loop 产生的消息转换为可持久化格式。
 *
 * 核心原则：只存客观事实，剥离模型推理。
 *   - assistant + tool_use blocks → 剥离 text block，只保留 tool_use，role="assistant"
 *   - assistant + 纯 text（无 tool_use）→ 丢弃（用户不可见的推理）
 *   - user + tool_result blocks → role="tool_result"
 */
function toPersistableMessages(loopMessages: AgentMessage[]): PersistableMessage[] {
  const result: PersistableMessage[] = [];

  for (const msg of loopMessages) {
    if (msg.role === "assistant") {
      if (typeof msg.content === "string") continue; // pure text thinking, skip
      const toolUseBlocks = msg.content.filter((b) => b.type === "tool_use");
      if (toolUseBlocks.length === 0) continue; // no tool_use = pure thinking, skip
      result.push({
        role: "assistant",
        content: serializeBlocks(toolUseBlocks),
        messageType: "structured",
      });
    } else if (msg.role === "user") {
      if (typeof msg.content === "string") continue; // shouldn't happen for loop-generated user msgs
      const toolResultBlocks = msg.content.filter((b) => b.type === "tool_result");
      if (toolResultBlocks.length === 0) continue;
      result.push({
        role: "tool_result",
        content: serializeBlocks(toolResultBlocks),
        messageType: "structured",
      });
    }
  }

  return result;
}

// ==================== LLM Client Factory ====================

/**
 * 创建 LLM Client。
 *
 * Agent 引擎始终使用 AnthropicClient（Anthropic Messages API 格式）。
 * 兼容性问题由 API Proxy 层（api-proxy.ts）解决：
 *   - Anthropic 原生 / 兼容代理 → 直连目标 URL
 *   - 其它 provider（OpenAI/Kimi/GLM 等）→ 指向本地 proxy，proxy 做格式转换
 */
function createLLMClient(
  modelConfig: ModelConfig,
): LLMClient {
  const isAnthropicCompatible =
    modelConfig.provider === "claude" ||
    (modelConfig.baseUrl || "").includes("/anthropic");

  if (isAnthropicCompatible) {
    return new AnthropicClient({
      apiKey: modelConfig.apiKey || "",
      baseUrl: modelConfig.baseUrl,
      maxTokens: modelConfig.maxTokens,
    });
  }

  // 非 Anthropic 兼容：配置本地 proxy 做格式转换，引擎仍用 AnthropicClient
  updateProxyTarget({
    baseUrl: modelConfig.baseUrl || "",
    apiKey: modelConfig.apiKey || "",
    model: modelConfig.model,
  });

  return new AnthropicClient({
    apiKey: "sk-proxy-local",
    baseUrl: `http://localhost:${agentPort}`,
    maxTokens: modelConfig.maxTokens,
  });
}

// ==================== Tool Registry Builder ====================

function buildToolRegistry(
  skillsCtx: SkillContext,
  request: ProcessRequest,
  sessionId: string,
  traceId: string,
  server: ServerClient,
): ToolRegistry {
  const registry = new ToolRegistry();

  // ── 内置工具：send_channel_message ──
  registry.registerBuiltin(
    "send_channel_message",
    "向当前渠道发送消息。content 填要发出去的话。",
    {
      type: "object",
      properties: {
        content: { type: "string", description: "消息内容" },
        channel: { type: "string", description: "目标渠道（默认取消息来源渠道）" },
        channelUserId: { type: "string", description: "渠道用户 ID" },
        channelConversationId: { type: "string", description: "群聊 ID" },
        messageType: { type: "string", description: "消息类型：text / image / file / rich_text，默认 text" },
      },
      required: ["content"],
    },
    async (input) => {
      const channel = (input.channel as string) || request.channel;
      const channelUserId = (input.channelUserId as string) || request.channelUserId;
      const content = input.content as string;
      if (!content) throw new Error("content is required");

      await server.sendToChannel({
        channel,
        channelUserId,
        content,
        messageType: input.messageType as string | undefined,
        channelConversationId:
          (input.channelConversationId as string) || request.channelConversationId,
        sessionId,
        traceId,
      });

      return { success: true, channel, channelUserId };
    },
  );

  // ── 内置工具：get_skill_doc ──
  const internalHandlers: Record<string, ToolExecutor> = {
    get_skill_doc: async (input) => {
      const skillName = input.skill_name as string;
      if (!skillName) throw new Error("skill_name is required");
      const doc = skillsCtx.skillDocs[skillName];
      if (!doc) throw new Error(`Skill doc not found: ${skillName}`);
      return { skill: skillName, doc };
    },
  };

  // ── Skill 工具 ──
  registry.registerSkills(skillsCtx, internalHandlers);

  // ── 飞书工具（仅当 feishu-operator skill 启用时） ──
  const hasFeishuSkill = Object.keys(skillsCtx.skillDocs).some(
    (name) => name === "feishu-operator" || name.toLowerCase().includes("feishu"),
  );
  if (hasFeishuSkill) {
    for (const def of FEISHU_TOOL_DEFS) {
      if (registry.has(def.name)) continue;
      registry.registerBuiltin(
        def.name,
        def.description,
        {
          type: "object",
          properties: Object.fromEntries(
            Object.entries(def.shape).map(([k, v]) => [k, { type: "string" }]),
          ),
        },
        (args) => executeFeishuTool(def, args),
      );
    }
  }

  return registry;
}

// ==================== Core: Process One Message ====================

async function processOneEvent(
  worker: SessionWorker,
  request: QueuedRequest,
  server: ServerClient,
): Promise<void> {
  const report = createTraceReporter(server, {
    traceId: request.traceId,
    sessionId: worker.sessionId,
    agentId: request.agentId,
    userId: request.userId,
    channel: request.channel,
  });

  if (!request.traceStarted) {
    report({ type: "start", timestamp: Date.now(), initiator: "user" });
  }

  report({
    type: "thinking",
    timestamp: Date.now(),
    thinking: "加载 Agent 配置与技能...",
    source: "system",
  });

  try {
    // ── 加载配置 ──
    const agentConfig = await server.getAgentConfig(request.agentId);
    if (!agentConfig) throw new Error(`Agent not found: ${request.agentId}`);

    const provider = agentConfig.provider;
    const modelName = agentConfig.model;
    if (!provider || !modelName) {
      throw new Error(
        `Agent "${request.agentId}" missing provider/model: ` +
        `provider=${provider || "missing"}, model=${modelName || "missing"}`,
      );
    }

    const [credentials, skillsCtx] = await Promise.all([
      server.getProviderCredentials(provider),
      server.getSkillsContext(request.agentId),
    ]);

    if (!credentials?.apiKey) {
      throw new Error(`No API key for provider "${provider}". Set it in Admin → Settings.`);
    }

    const modelConfig: ModelConfig = {
      id: `${provider}:${modelName}`,
      name: `${provider}/${modelName}`,
      provider: provider as ModelConfig["provider"],
      baseUrl: credentials.baseUrl || undefined,
      apiKey: credentials.apiKey,
      model: modelName,
      maxTokens: 8192,
      temperature: 0.7,
    };

    const skillContext: SkillContext = skillsCtx || {
      systemPromptAddition: "",
      tools: [],
      toolExecutors: {},
      skillDocs: {},
    };

    // ── 构建引擎组件 ──
    const llmClient = createLLMClient(modelConfig);
    const toolRegistry = buildToolRegistry(
      skillContext,
      request,
      worker.sessionId,
      request.traceId,
      server,
    );
    const systemPrompt = buildSystemPrompt(
      agentConfig.systemPrompt,
      skillContext.systemPromptAddition,
    );

    // ── 上报配置快照：skills + tools ──
    const loadedSkills = Object.keys(skillContext.skillDocs);
    const allToolNames = toolRegistry.getAll().map(t => t.definition.name);
    report({
      type: "thinking",
      timestamp: Date.now(),
      thinking: [
        `配置加载完成。模型: ${provider}/${modelName}`,
        `Skills (${loadedSkills.length}): ${loadedSkills.length > 0 ? loadedSkills.join(', ') : '无'}`,
        `Tools  (${allToolNames.length}): ${allToolNames.join(', ') || '无'}`,
      ].join('\n'),
      source: "system",
    });

    // ── 加载历史消息，过滤掉当前消息 ──
    // channel-dispatcher 在派发前就将当前 user 消息存入 DB，
    // 因此加载历史时必须按 messageId 排除当前消息，否则它会和上一轮的
    // tool_result 被 ensureAlternation 合并，导致 tool_call_id 上下文错乱。
    const allDbMessages = await server.getSessionMessages(worker.sessionId, HISTORY_LIMIT);
    const dbMessages = allDbMessages.filter((m) => m.id !== request.messageId);
    // 清洗管道：格式转换 → 清孤儿 result → 截断 → 清孤儿 use → 重新保证交替
    // removeOrphanedToolUses 必须在最后执行：ensureAlternation（在 dbMessagesToAgentMessages 内部）
    // 可能将 tool_result 块 flatten 为纯文本，导致对应的 tool_use 变成孤儿。
    const historyMessages = ensureAlternation(
      removeOrphanedToolUses(
        truncateToolResults(
          removeOrphanedToolResults(
            dbMessagesToAgentMessages(dbMessages),
          ),
        ),
      ),
    );

    // ── 构建当前轮给模型的用户消息（带渠道元信息，仅当前轮携带） ──
    const meta: string[] = [
      `channel=${request.channel}`,
      `channelUserId=${request.channelUserId}`,
    ];
    if (request.channelConversationId) meta.push(`channelConversationId=${request.channelConversationId}`);
    if (request.senderName) meta.push(`senderName=${request.senderName}`);

    const currentUserContent = [
      `[消息来源] ${meta.join(" ")}`,
      `[${request.senderName || request.channelUserId}]`,
      request.content,
    ].join("\n");

    // ── 组装完整消息序列：历史 + 当前 ──
    const messages: AgentMessage[] = [
      ...historyMessages,
      { role: "user", content: currentUserContent },
    ];

    // ── 创建 AbortController ──
    const abortController = new AbortController();
    worker.abortController = abortController;

    report({
      type: "thinking",
      timestamp: Date.now(),
      thinking: `准备就绪，历史消息 ${historyMessages.length} 条，开始执行思考与工具调用循环 (ReAct)...`,
      source: "system",
    });

    // ── 运行 Agent Loop ──
    const result = await runAgentLoop({
      llmClient,
      systemPrompt,
      messages,
      tools: toolRegistry.getAll(),
      model: modelConfig.model,
      maxIterations: MAX_ITERATIONS,
      signal: abortController.signal,
      onEvent: (event) => {
        report(agentEventToTracePayload(event));
      },
      // 每轮工具调用完成后立即持久化（tool_use + tool_result 是客观事实，必须完整记录）
      // 即使进程崩溃，已完成的迭代数据不会丢失；
      // 下一轮加载历史时 removeOrphanedToolUses 会兜底清理未完成的孤儿 tool_use。
      onNewMessages: async (iterationMessages) => {
        const persistable = toPersistableMessages(iterationMessages);
        for (const msg of persistable) {
          await server.saveMessage({
            sessionId: worker.sessionId,
            role: msg.role,
            content: msg.content,
            messageType: msg.messageType,
            channel: request.channel,
            traceId: request.traceId,
            initiator: msg.role === "assistant" ? "agent" : undefined,
          });
        }
      },
    });

  } catch (err) {
    const error = err instanceof Error ? err.message : String(err);
    report({ type: "error", timestamp: Date.now(), error });
    report({ type: "done", timestamp: Date.now(), error });
    throw err;
  } finally {
    worker.abortController = undefined;
  }
}

// ==================== Session Queue Management ====================

function resetIdleTimer(worker: SessionWorker): void {
  if (worker.idleTimer) {
    clearTimeout(worker.idleTimer);
    worker.idleTimer = undefined;
  }
  worker.lastActivityAt = Date.now();
  worker.idleTimer = setTimeout(() => evictSession(worker), SESSION_IDLE_TIMEOUT_MS);
}

function evictSession(worker: SessionWorker): void {
  if (worker.processing) {
    resetIdleTimer(worker);
    return;
  }
  console.log(`[runner] Evicting idle session: ${worker.sessionId}`);
  if (worker.idleTimer) clearTimeout(worker.idleTimer);
  sessionWorkers.delete(worker.sessionId);
}

async function drainWorker(worker: SessionWorker): Promise<void> {
  const server = new ServerClient();

  try {
    while (worker.queue.length > 0) {
      const request = worker.queue.shift()!;
      console.log(
        `[runner] Processing: sid=${worker.sessionId.slice(0, 8)} tid=${request.traceId.slice(0, 8)} ` +
        `remaining=${worker.queue.length}`,
      );

      await server.updateSession(worker.sessionId, { executionStatus: "processing" });

      try {
        await processOneEvent(worker, request, server);
      } catch (err) {
        const error = err instanceof Error ? err.message : String(err);
        console.error(`[runner] Error in session ${worker.sessionId}:`, error);
        try {
          await server.updateSession(worker.sessionId, { executionStatus: "interrupted" });
        } catch { /* ignore */ }
      }
    }

    await server.updateSession(worker.sessionId, { executionStatus: "completed" });
  } finally {
    worker.processing = false;

    if (worker.queue.length > 0) {
      worker.processing = true;
      void drainWorker(worker);
      return;
    }

    resetIdleTimer(worker);
  }
}

async function enqueueInner(request: ProcessRequest): Promise<void> {
  const server = new ServerClient();
  const resolved = await resolveOrCreateSession(server, request);
  const traceId = request.traceId || crypto.randomUUID();

  let worker = sessionWorkers.get(resolved.sessionId);
  if (!worker) {
    worker = {
      sessionId: resolved.sessionId,
      sessionKey: resolved.sessionKey,
      workDir: resolved.workDir,
      queue: [],
      processing: false,
      lastActivityAt: Date.now(),
    };
    sessionWorkers.set(resolved.sessionId, worker);
  } else {
    worker.lastActivityAt = Date.now();
    if (worker.idleTimer) {
      clearTimeout(worker.idleTimer);
      worker.idleTimer = undefined;
    }
  }

  // 上报 start trace
  const queuedRequest: QueuedRequest = {
    ...request,
    sessionId: resolved.sessionId,
    traceId,
    traceStarted: false,
  };

  try {
    await server.reportTraceEventSync({
      traceId,
      sessionId: resolved.sessionId,
      agentId: request.agentId,
      userId: request.userId,
      channel: request.channel,
      type: "start",
      timestamp: Date.now(),
      initiator: "user",
    });
    queuedRequest.traceStarted = true;
  } catch (err) {
    console.warn(`[runner] Failed to persist start trace: ${err}`);
  }

  worker.queue.push(queuedRequest);

  if (!worker.processing) {
    worker.processing = true;
    void drainWorker(worker);
  }
}

// ==================== Public API ====================

export async function enqueueProcessRequest(request: ProcessRequest): Promise<void> {
  if (shuttingDown) {
    throw new Error("Agent is shutting down");
  }

  const key = request.sessionId || resolveSessionKey(
    request.channel,
    request.channelUserId,
    request.channelConversationId,
  );

  const prev = enqueueLocks.get(key)?.catch(() => {}) || Promise.resolve();
  const current = prev.then(() => enqueueInner(request));
  enqueueLocks.set(key, current);

  try {
    await current;
  } finally {
    if (enqueueLocks.get(key) === current) {
      enqueueLocks.delete(key);
    }
  }
}

export async function cleanupInterruptedSessions(): Promise<number> {
  const server = new ServerClient();
  const interrupted = await server.getInterruptedSessions();
  let cleaned = 0;
  for (const session of interrupted) {
    try {
      await server.updateSession(session.id, { executionStatus: "completed" });
      cleaned++;
    } catch { /* ignore */ }
  }
  return cleaned;
}

export async function gracefulShutdown(): Promise<void> {
  if (shuttingDown) return;
  shuttingDown = true;
  console.log("[runner] Starting graceful shutdown...");

  // Abort all active loops
  for (const [, worker] of sessionWorkers) {
    if (worker.idleTimer) clearTimeout(worker.idleTimer);
    if (worker.abortController) {
      try { worker.abortController.abort(); } catch { /* ignore */ }
    }
    worker.queue.length = 0;
  }

  // Mark all processing sessions as interrupted
  const server = new ServerClient();
  for (const [sessionId, worker] of sessionWorkers) {
    if (worker.processing) {
      try {
        await server.updateSession(sessionId, { executionStatus: "interrupted" });
      } catch { /* ignore */ }
    }
  }

  sessionWorkers.clear();
  console.log("[runner] Shutdown complete");
}
