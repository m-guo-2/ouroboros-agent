/**
 * SDK Runner — 会话化执行引擎
 *
 * 目标：
 * 1. Session = channel:uniqueId（群聊 conversationId，私聊 channelUserId）
 * 2. 同一 session 单线程执行：运行中追加 event 入队
 * 3. 通过 Claude Agent SDK resume 维持长程 session
 * 4. 每个 session 独立工作目录 + 每次激活同步 skills
 * 5. 对外回复由 skills 工具决定（不自动回传 assistant 文本）
 * 6. 全链路 trace append-only 上报
 *
 * 持久 Session（LiveSession）机制：
 * - 基于 V1 query() + AsyncIterable<SDKUserMessage> 实现 send/stream 模式
 *   （等效 V2 unstable_v2_createSession，但保留完整配置能力）
 * - SDK 子进程在 session 生命周期内常驻内存，多条消息复用同一子进程
 * - 无在途任务且空闲超过 SESSION_IDLE_TIMEOUT_MS 后自动驱逐（关闭子进程、释放内存）
 * - 出错时自动销毁 LiveSession，下条消息重建
 */

import {
  query,
  createSdkMcpServer,
  tool,
  type SDKResultMessage,
  type SDKUserMessage,
  type SDKMessage,
} from "@anthropic-ai/claude-agent-sdk";
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { z, type ZodRawShape, type ZodTypeAny } from "zod";
import {
  ServerClient,
  type ModelConfig,
  type SkillContext,
  type SkillToolExecutor,
  type TraceEventPayload,
} from "./server-client";
import { buildSystemPrompt } from "./context-composer";
import { registerSessionProxy, unregisterSessionProxy, type SessionProxyConfig } from "./api-proxy";
import { FEISHU_TOOL_DEFS, executeFeishuTool } from "./feishu-tools";

const PROJECT_ROOT = resolve(import.meta.dir, "../../..");
const SESSION_WORK_ROOT = resolve(PROJECT_ROOT, ".agent-sessions");

/** SDK 单次 query 的最大工具循环轮次 */
const MAX_TURNS = 20;
/** 控制台输出 trace 过程（默认开启；AGENT_TRACE_CONSOLE=0 可关闭） */
const TRACE_CONSOLE_ENABLED =
  process.env.AGENT_TRACE_CONSOLE !== "0" &&
  process.env.AGENT_TRACE_CONSOLE !== "false";
const TRACE_CONSOLE_THINKING_MAX = Number(process.env.AGENT_TRACE_CONSOLE_THINKING_MAX || 120);
const TRACE_CONSOLE_PAYLOAD_MAX = Number(process.env.AGENT_TRACE_CONSOLE_PAYLOAD_MAX || 240);

/** Session 空闲多久后从内存中驱逐（默认 10 分钟） */
const SESSION_IDLE_TIMEOUT_MS = Number(process.env.AGENT_SESSION_IDLE_TIMEOUT_MS || 10 * 60 * 1000);

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

interface QueuedProcessRequest extends ProcessRequest {
  sessionId: string;
  traceId: string;
  traceStarted?: boolean;
}

interface Usage {
  inputTokens: number;
  outputTokens: number;
  totalCostUsd: number;
}

/**
 * LiveSession — 基于 V1 query() + AsyncIterable 实现的持久 session。
 *
 * 原理：V2 unstable_v2_createSession 内部就是 query() + streamInput()。
 * 但 V2 的 SDKSessionOptions 硬编码了 permissionMode=default / settingSources=[] / mcpServers={}，
 * 不满足我们的需求。所以直接用 V1 query() 的完整配置能力，复刻 V2 的 send/stream 模式。
 *
 * 生命周期：
 *   创建 → send(msg) → stream() → send(msg) → stream() → ... → close()
 * stream() 在收到 result 消息后 return，下次 send + stream 开始新一轮。
 * 底层 SDK 子进程在整个生命周期内常驻，不重启。
 */
class LiveSession {
  private queryRef: ReturnType<typeof query>;
  private queryIterator: AsyncIterator<SDKMessage> | null = null;
  private inputQueue: SDKUserMessage[] = [];
  private inputWaiter: ((result: IteratorResult<SDKUserMessage>) => void) | null = null;
  closed = false;
  readonly abortController: AbortController;

  constructor(options: Record<string, unknown>) {
    this.abortController = (options.abortController as AbortController) || new AbortController();
    options.abortController = this.abortController;

    const inputIterable = this.createInputIterable();
    this.queryRef = query({ prompt: inputIterable, options: options as any });
  }

  private createInputIterable(): AsyncIterable<SDKUserMessage> {
    const self = this;
    return {
      [Symbol.asyncIterator]() {
        return {
          next(): Promise<IteratorResult<SDKUserMessage>> {
            if (self.inputQueue.length > 0) {
              return Promise.resolve({ value: self.inputQueue.shift()!, done: false });
            }
            if (self.closed) {
              return Promise.resolve({ value: undefined as any, done: true });
            }
            return new Promise((resolve) => {
              self.inputWaiter = resolve;
            });
          },
        };
      },
    };
  }

  /** 向 session 推送一条用户消息 */
  send(content: string): void {
    if (this.closed) throw new Error("Cannot send to closed LiveSession");
    const userMessage: SDKUserMessage = {
      type: "user",
      session_id: "",
      message: { role: "user", content: [{ type: "text", text: content }] } as any,
      parent_tool_use_id: null,
    };
    if (this.inputWaiter) {
      const resolve = this.inputWaiter;
      this.inputWaiter = null;
      resolve({ value: userMessage, done: false });
    } else {
      this.inputQueue.push(userMessage);
    }
  }

  /** 读取当前轮次的流式响应，遇到 result 消息后 return */
  async *stream(): AsyncGenerator<SDKMessage, void> {
    if (!this.queryIterator) {
      this.queryIterator = this.queryRef[Symbol.asyncIterator]();
    }
    while (true) {
      const { value, done } = await this.queryIterator.next();
      if (done) return;
      yield value;
      if (value.type === "result") return;
    }
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    if (this.inputWaiter) {
      this.inputWaiter({ value: undefined as any, done: true });
      this.inputWaiter = null;
    }
    try {
      this.abortController.abort();
    } catch {
      // 忽略 abort 异常
    }
  }
}

interface SessionRuntime {
  sessionId: string;
  sessionKey: string;
  sdkSessionId?: string;
  workDir: string;
  queue: QueuedProcessRequest[];
  processing: boolean;
  /** 持久 SDK session（子进程常驻内存） */
  liveSession?: LiveSession;
  /** 最后一次活跃时间戳 */
  lastActivityAt: number;
  /** 空闲驱逐定时器 */
  idleTimer?: ReturnType<typeof setTimeout>;
  /** 缓存的 skill 工具运行时上下文（跟随 liveSession 生命周期） */
  cachedRuntimeContext?: SkillToolRuntimeContext;
  /** 缓存的 skill 工具名称集合 */
  cachedSkillToolNames?: Set<string>;
}

/**
 * Skill 工具运行时上下文。
 * 在 LiveSession 模式下，request / traceId / report / seenToolCallIds 等字段
 * 会在每条消息处理前被更新（mutable），工具 handler 通过引用读取最新值。
 */
interface SkillToolRuntimeContext {
  server: ServerClient;
  request: ProcessRequest;
  sessionId: string;
  traceId: string;
  workDir: string;
  skillDocs: Record<string, string>;
  report: (event: Omit<TraceEventPayload, "traceId" | "sessionId" | "agentId" | "userId" | "channel">) => void;
  seenToolCallIds: Set<string>;
  finishedToolCallIds: Set<string>;
}

/** Agent 自身端口（API Proxy 挂在此端口） */
let agentAppPort = 1996;

/** sessionId -> 单线程 worker */
const sessionWorkers = new Map<string, SessionRuntime>();
/** enqueue 串行锁（按 session 标识） */
const enqueueLocks = new Map<string, Promise<void>>();

/** 活跃的 query AbortController，shutdown 时统一 abort */
const activeAbortControllers = new Set<AbortController>();
/** shutdown 标志：一旦置 true，不再接受新请求 */
let shuttingDown = false;

export function setAgentAppPort(port: number): void {
  agentAppPort = port;
}

// ==================== Session 空闲驱逐 ====================

function resetSessionIdleTimer(worker: SessionRuntime): void {
  if (worker.idleTimer) {
    clearTimeout(worker.idleTimer);
    worker.idleTimer = undefined;
  }
  worker.lastActivityAt = Date.now();
  worker.idleTimer = setTimeout(() => evictIdleSession(worker), SESSION_IDLE_TIMEOUT_MS);
}

function evictIdleSession(worker: SessionRuntime): void {
  // 正在处理中不驱逐
  if (worker.processing) {
    console.log(`[sdk-runner][idle-skip] sessionId=${worker.sessionId} reason=still_processing`);
    resetSessionIdleTimer(worker);
    return;
  }

  console.log(
    `[sdk-runner][idle-evict] sessionId=${worker.sessionId} ` +
    `idleMs=${Date.now() - worker.lastActivityAt} ` +
    `hadLiveSession=${!!worker.liveSession}`,
  );

  // 关闭持久 session（终止 SDK 子进程）
  if (worker.liveSession) {
    worker.liveSession.close();
    activeAbortControllers.delete(worker.liveSession.abortController);
    worker.liveSession = undefined;
  }
  worker.cachedRuntimeContext = undefined;
  worker.cachedSkillToolNames = undefined;

  if (worker.idleTimer) {
    clearTimeout(worker.idleTimer);
    worker.idleTimer = undefined;
  }

  sessionWorkers.delete(worker.sessionId);
}

function resolveSessionKey(
  channel: string,
  channelUserId: string,
  channelConversationId?: string,
): string {
  const uniqueId = channelConversationId || channelUserId;
  return `${channel}:${uniqueId}`;
}

function ensureDir(path: string): void {
  if (!existsSync(path)) {
    mkdirSync(path, { recursive: true });
  }
}

function isPathInsideProject(path: string): boolean {
  const rel = relative(PROJECT_ROOT, path);
  return rel === "" || (!rel.startsWith("..") && !rel.startsWith("../"));
}

function resolveSessionWorkDir(sessionId: string, existingWorkDir?: string): string {
  if (existingWorkDir && isPathInsideProject(existingWorkDir)) {
    return existingWorkDir;
  }
  return resolve(SESSION_WORK_ROOT, sessionId);
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function safeSkillName(name: string): string {
  return name.replace(/[^a-zA-Z0-9._-]/g, "-");
}

function parseMaybeJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function toToolText(result: unknown): string {
  if (typeof result === "string") return result;
  try {
    return JSON.stringify(result, null, 2);
  } catch {
    return String(result);
  }
}

function normalizeTextChunk(text: string): string {
  return text.replace(/\r/g, "").replace(/\s+/g, " ").trim();
}

function trimPreview(text: string, maxLength: number): string {
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength)}...`;
}

function toOneLineText(value: unknown): string {
  if (typeof value === "string") {
    return normalizeTextChunk(value);
  }
  try {
    return normalizeTextChunk(JSON.stringify(value));
  } catch {
    return normalizeTextChunk(String(value));
  }
}

function formatTraceEventLog(
  base: Pick<TraceEventPayload, "traceId" | "sessionId">,
  event: Omit<TraceEventPayload, "traceId" | "sessionId" | "agentId" | "userId" | "channel">,
): string {
  const head = `[sdk-trace] sessionId=${base.sessionId} traceId=${base.traceId} type=${event.type}`;

  if (event.type === "start") {
    return `${head} initiator=${event.initiator || "user"}`;
  }
  if (event.type === "thinking") {
    return `${head} text="${trimPreview(toOneLineText(event.thinking || ""), TRACE_CONSOLE_THINKING_MAX)}"`;
  }
  if (event.type === "tool_call") {
    const inputPreview = trimPreview(toOneLineText(event.toolInput), TRACE_CONSOLE_PAYLOAD_MAX);
    return `${head} tool=${event.toolName || "unknown"} callId=${event.toolCallId || ""} input=${inputPreview}`;
  }
  if (event.type === "tool_result") {
    const resultPreview = trimPreview(toOneLineText(event.toolResult), TRACE_CONSOLE_PAYLOAD_MAX);
    const success = event.toolSuccess === false ? "false" : "true";
    return `${head} tool=${event.toolName || "unknown"} callId=${event.toolCallId || ""} success=${success} durationMs=${event.toolDuration ?? 0} result=${resultPreview}`;
  }
  if (event.type === "content") {
    return `${head} content="${trimPreview(toOneLineText(event.content || ""), TRACE_CONSOLE_THINKING_MAX)}"`;
  }
  if (event.type === "error") {
    return `${head} error="${trimPreview(toOneLineText(event.error || ""), TRACE_CONSOLE_PAYLOAD_MAX)}"`;
  }
  if (event.type === "done") {
    const usage = event.usage
      ? `usage(in=${event.usage.inputTokens}, out=${event.usage.outputTokens}, cost=${event.usage.totalCostUsd})`
      : "usage(n/a)";
    const errorTag = event.error ? ` error="${trimPreview(toOneLineText(event.error), TRACE_CONSOLE_PAYLOAD_MAX)}"` : "";
    return `${head} ${usage}${errorTag}`;
  }
  return head;
}

function extractStreamThinkingText(event: unknown): string | null {
  if (!event || typeof event !== "object") return null;
  const raw = event as Record<string, unknown>;
  const eventType = asString(raw.type);

  if (eventType === "content_block_delta") {
    const delta = raw.delta && typeof raw.delta === "object"
      ? raw.delta as Record<string, unknown>
      : null;
    if (!delta) return null;

    const deltaType = asString(delta.type);
    if (deltaType === "thinking_delta") {
      return asString(delta.thinking) || null;
    }
    if (deltaType === "text_delta") {
      return asString(delta.text) || null;
    }
    return null;
  }

  if (eventType === "content_block_start") {
    const block = raw.content_block && typeof raw.content_block === "object"
      ? raw.content_block as Record<string, unknown>
      : null;
    if (!block) return null;

    if (asString(block.type) === "thinking") {
      return asString(block.thinking) || asString(block.text) || null;
    }
  }

  return null;
}

/**
 * 构建 PreToolUse hooks。
 * 接收 getReport 间接引用（而非直接 report），使得 LiveSession 多轮复用时
 * hooks 始终调用当前消息的 report（traceId 正确）。
 */
function buildPreToolUseGuards(
  getReport: () => (event: Omit<TraceEventPayload, "traceId" | "sessionId" | "agentId" | "userId" | "channel">) => void,
) {
  return {
    PreToolUse: [{
      hooks: [async (input: unknown) => {
        const raw = (input && typeof input === "object")
          ? input as Record<string, unknown>
          : {};
        const hookEventName = asString(raw.hook_event_name);
        const toolName = asString(raw.tool_name);
        if (hookEventName !== "PreToolUse" || toolName !== "Bash") {
          return { continue: true };
        }

        const toolInput = (raw.tool_input && typeof raw.tool_input === "object")
          ? raw.tool_input as Record<string, unknown>
          : {};
        const command = asString(toolInput.command) || asString(toolInput.cmd) || "";
        const normalized = command.toLowerCase();

        const tryingChannelSend =
          normalized.includes("/api/data/channels/send") ||
          normalized.includes("/api/channels/send") ||
          normalized.includes("channels/send");

        if (!tryingChannelSend) {
          return { continue: true };
        }

        getReport()({
          type: "thinking",
          timestamp: Date.now(),
          thinking: "已拦截 Bash 直接调用 channels/send，要求改用 send_channel_message 工具。",
          source: "system",
        });

        return {
          continue: true,
          hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "deny",
            permissionDecisionReason:
              "Do not call channels/send via Bash/curl. Use send_channel_message tool only.",
          },
        };
      }],
    }],
  };
}

function resolveToolUseIdFromExtra(extra: unknown): string | undefined {
  if (!extra || typeof extra !== "object") return undefined;
  const raw = extra as Record<string, unknown>;
  const candidate = [
    asString(raw.tool_use_id),
    asString(raw.toolUseID),
    asString(raw.toolUseId),
    asString(raw.tool_call_id),
    asString(raw.toolCallId),
    asString(raw.id),
  ].find((v) => !!v);
  return candidate || undefined;
}

function buildEventPrompt(request: ProcessRequest): string {
  const meta: string[] = [
    `channel=${request.channel}`,
    `channelUserId=${request.channelUserId}`,
  ];
  if (request.channelConversationId) meta.push(`channelConversationId=${request.channelConversationId}`);
  if (request.channelMessageId) meta.push(`channelMessageId=${request.channelMessageId}`);
  if (request.senderName) meta.push(`senderName=${request.senderName}`);

  return [
    "[消息来源] " + meta.join(" "),
    "[用户消息]",
    request.content,
  ].join("\n");
}

function syncSkillsToWorkDir(workDir: string, skillDocs: Record<string, string>): number {
  const skillsRoot = join(workDir, ".claude", "skills");
  ensureDir(skillsRoot);

  let changed = 0;
  const expectedDirs = new Set<string>();

  for (const [name, doc] of Object.entries(skillDocs)) {
    const dirName = safeSkillName(name);
    expectedDirs.add(dirName);

    const targetDir = join(skillsRoot, dirName);
    const targetFile = join(targetDir, "SKILL.md");
    ensureDir(targetDir);

    const nextContent = doc || "";
    const prevContent = existsSync(targetFile) ? readFileSync(targetFile, "utf-8") : null;
    if (prevContent !== nextContent) {
      writeFileSync(targetFile, nextContent, "utf-8");
      changed++;
    }
  }

  const entries = readdirSync(skillsRoot, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    if (expectedDirs.has(entry.name)) continue;
    rmSync(join(skillsRoot, entry.name), { recursive: true, force: true });
    changed++;
  }

  return changed;
}

function schemaToZod(schema: unknown): ZodTypeAny {
  const def = (schema && typeof schema === "object")
    ? schema as Record<string, unknown>
    : {};

  const oneOf = Array.isArray(def.oneOf) ? def.oneOf : null;
  if (oneOf && oneOf.length >= 2) {
    const variants = oneOf.map((s) => schemaToZod(s));
    return z.union(variants as [ZodTypeAny, ZodTypeAny, ...ZodTypeAny[]]);
  }

  const anyOf = Array.isArray(def.anyOf) ? def.anyOf : null;
  if (anyOf && anyOf.length >= 2) {
    const variants = anyOf.map((s) => schemaToZod(s));
    return z.union(variants as [ZodTypeAny, ZodTypeAny, ...ZodTypeAny[]]);
  }

  const enumValues = Array.isArray(def.enum) ? def.enum : null;
  if (enumValues && enumValues.length > 0) {
    const allStrings = enumValues.every((v) => typeof v === "string");
    if (allStrings) {
      return z.enum(enumValues as [string, ...string[]]);
    }
    return z.any().refine((v) => enumValues.includes(v), "Invalid enum value");
  }

  const type = typeof def.type === "string" ? def.type : undefined;
  switch (type) {
    case "string":
      return z.string();
    case "integer":
    case "number":
      return z.number();
    case "boolean":
      return z.boolean();
    case "array": {
      const itemSchema = schemaToZod(def.items);
      return z.array(itemSchema);
    }
    case "object": {
      const properties = (def.properties && typeof def.properties === "object")
        ? def.properties as Record<string, unknown>
        : {};
      const requiredList = Array.isArray(def.required)
        ? def.required.filter((x): x is string => typeof x === "string")
        : [];
      const required = new Set(requiredList);
      const shape: Record<string, ZodTypeAny> = {};

      for (const [key, value] of Object.entries(properties)) {
        let field = schemaToZod(value);
        if (!required.has(key)) {
          field = field.optional();
        }
        shape[key] = field;
      }

      return z.object(shape).passthrough();
    }
    default:
      return z.any();
  }
}

function inputSchemaToShape(inputSchema: SkillContext["tools"][number]["input_schema"]): ZodRawShape {
  const properties = inputSchema?.properties || {};
  const required = new Set(inputSchema?.required || []);
  const shape: Record<string, ZodTypeAny> = {};

  for (const [key, prop] of Object.entries(properties)) {
    const propSchema = (prop && typeof prop === "object") ? prop as Record<string, unknown> : {};
    let field = schemaToZod(propSchema);
    const description = typeof propSchema.description === "string" ? propSchema.description : undefined;
    if (description) {
      field = field.describe(description);
    }
    if (!required.has(key)) {
      field = field.optional();
    }
    shape[key] = field;
  }

  return shape as ZodRawShape;
}

async function executeInternalTool(
  handler: string | undefined,
  input: Record<string, unknown>,
  runtime: SkillToolRuntimeContext,
): Promise<unknown> {
  if (handler === "get_skill_doc") {
    const skillName = asString(input.skill_name);
    if (!skillName) {
      throw new Error("get_skill_doc requires skill_name");
    }
    const doc = runtime.skillDocs[skillName];
    if (!doc) {
      throw new Error(`Skill doc not found: ${skillName}`);
    }
    return { skill: skillName, doc };
  }

  if (handler === "send_channel_message") {
    const channel = asString(input.channel) || runtime.request.channel;
    const channelUserId = asString(input.channelUserId) || runtime.request.channelUserId;
    const content = asString(input.content);

    if (!channel || !channelUserId || !content) {
      throw new Error("send_channel_message requires channel, channelUserId, content");
    }

    await runtime.server.sendToChannel({
      channel,
      channelUserId,
      content,
      messageType: asString(input.messageType),
      channelConversationId: asString(input.channelConversationId) || runtime.request.channelConversationId,
      sessionId: runtime.sessionId,
      traceId: runtime.traceId,
    });

    return {
      success: true,
      channel,
      channelUserId,
      sessionId: runtime.sessionId,
    };
  }

  throw new Error(`Unsupported internal handler: ${handler || "<empty>"}`);
}

async function executeSkillTool(
  toolName: string,
  executor: SkillToolExecutor | undefined,
  input: Record<string, unknown>,
  runtime: SkillToolRuntimeContext,
): Promise<unknown> {
  if (!executor) {
    throw new Error(`Tool executor not found: ${toolName}`);
  }

  if (executor.type === "internal") {
    return executeInternalTool(executor.handler, input, runtime);
  }

  if (executor.type === "http") {
    if (!executor.url) {
      throw new Error(`HTTP executor missing url: ${toolName}`);
    }

    const method = (executor.method || "POST").toUpperCase();
    const response = await fetch(executor.url, {
      method,
      headers: { "Content-Type": "application/json" },
      body: method === "GET" ? undefined : JSON.stringify(input),
    });

    const text = await response.text();
    if (!response.ok) {
      throw new Error(`HTTP tool ${toolName} failed: ${response.status} ${text}`);
    }

    return parseMaybeJson(text);
  }

  if (executor.type === "script") {
    throw new Error(`Script executor is not enabled in minimal mode: ${toolName}`);
  }

  throw new Error(`Unsupported executor type: ${(executor as { type?: string }).type || "unknown"}`);
}

/**
 * 创建 SDK 工具用的 trace 包装回调
 */
function wrapToolWithTrace(
  name: string,
  description: string,
  shape: ZodRawShape,
  execute: (args: Record<string, unknown>) => Promise<unknown>,
  runtime: SkillToolRuntimeContext,
) {
  return tool(name, description, shape, async (args, extra) => {
    const callId = resolveToolUseIdFromExtra(extra) || `local-${crypto.randomUUID()}`;
    const startedAt = Date.now();

    if (!runtime.seenToolCallIds.has(callId)) {
      runtime.seenToolCallIds.add(callId);
      runtime.report({
        type: "tool_call",
        timestamp: startedAt,
        toolCallId: callId,
        toolName: name,
        toolInput: args as Record<string, unknown>,
      });
    }

    try {
      const result = await execute(args as Record<string, unknown>);

      if (!runtime.finishedToolCallIds.has(callId)) {
        runtime.finishedToolCallIds.add(callId);
        runtime.report({
          type: "tool_result",
          timestamp: Date.now(),
          toolCallId: callId,
          toolName: name,
          toolResult: result,
          toolDuration: Date.now() - startedAt,
          toolSuccess: true,
        });
      }

      return {
        content: [{ type: "text", text: toToolText(result) }],
      };
    } catch (err) {
      const error = err instanceof Error ? err.message : String(err);

      if (!runtime.finishedToolCallIds.has(callId)) {
        runtime.finishedToolCallIds.add(callId);
        runtime.report({
          type: "tool_result",
          timestamp: Date.now(),
          toolCallId: callId,
          toolName: name,
          toolResult: error,
          toolDuration: Date.now() - startedAt,
          toolSuccess: false,
        });
      }

      return {
        isError: true,
        content: [{ type: "text", text: error }],
      };
    }
  });
}

function buildSkillMcpServer(skillsCtx: SkillContext, runtime: SkillToolRuntimeContext) {
  // Skill 工具
  const sdkTools = skillsCtx.tools.map((def) => {
    const shape = inputSchemaToShape(def.input_schema);
    return wrapToolWithTrace(
      def.name,
      def.description,
      shape,
      (args) => executeSkillTool(def.name, skillsCtx.toolExecutors[def.name], args, runtime),
      runtime,
    );
  });

  // 内置工具：send_channel_message（不依赖 skill 注册）
  const sendMessageTool = wrapToolWithTrace(
    "send_channel_message",
    "向当前渠道发送消息。content 填要发出去的话。",
    {
      content: z.string().describe("消息内容"),
      channel: z.string().optional().describe("目标渠道（默认取消息来源渠道）"),
      channelUserId: z.string().optional().describe("渠道用户 ID（默认取消息来源用户）"),
      channelConversationId: z.string().optional().describe("群聊 ID（群聊时需要，默认取消息来源）"),
      messageType: z.string().optional().describe("消息类型：text / image / file / rich_text，默认 text"),
    },
    (args) => executeInternalTool("send_channel_message", args, runtime),
    runtime,
  );

  // 去重：如果 skill 已注册了同名工具，内置工具不重复添加
  const skillToolNames = new Set(sdkTools.map((t) => (t as any).name || ""));

  // 内置工具：send_channel_message
  if (!skillToolNames.has("send_channel_message")) {
    sdkTools.push(sendMessageTool);
  }

  // 飞书工具：仅当 skill 配置中包含 feishu-operator 时加载
  const hasFeishuSkill = Object.keys(runtime.skillDocs).some(
    (name) => name === "feishu-operator" || name.toLowerCase().includes("feishu"),
  );
  if (hasFeishuSkill) {
    for (const def of FEISHU_TOOL_DEFS) {
      if (skillToolNames.has(def.name)) continue; // skill 定义优先
      sdkTools.push(
        wrapToolWithTrace(
          def.name,
          def.description,
          def.shape,
          (args) => executeFeishuTool(def, args),
          runtime,
        ),
      );
    }
  }

  return createSdkMcpServer({
    name: `session-tools-${runtime.sessionId}`,
    tools: sdkTools,
  });
}

function buildSdkEnv(modelConfig: ModelConfig): Record<string, string> {
  const env: Record<string, string> = {};

  const systemKeys = [
    "PATH", "HOME", "TMPDIR", "SHELL", "LANG", "LC_ALL", "USER", "LOGNAME",
    "TERM", "TERM_PROGRAM", "XDG_RUNTIME_DIR", "XDG_DATA_HOME", "XDG_CONFIG_HOME",
    "NODE_PATH", "BUN_INSTALL",
  ];
  for (const key of systemKeys) {
    if (process.env[key]) env[key] = process.env[key]!;
  }

  const isAnthropicCompatible =
    modelConfig.provider === "claude" ||
    (modelConfig.baseUrl || "").includes("/anthropic");

  if (isAnthropicCompatible && modelConfig.apiKey) {
    env.ANTHROPIC_API_KEY = modelConfig.apiKey;
    env.ANTHROPIC_AUTH_TOKEN = modelConfig.apiKey;
    if (modelConfig.baseUrl) {
      env.ANTHROPIC_BASE_URL = modelConfig.baseUrl;
    }
  } else if (modelConfig.apiKey && modelConfig.baseUrl) {
    updateProxyTarget({
      baseUrl: modelConfig.baseUrl,
      apiKey: modelConfig.apiKey,
      model: modelConfig.model,
    });
    env.ANTHROPIC_BASE_URL = `http://localhost:${agentAppPort}`;
    env.ANTHROPIC_API_KEY = "sk-proxy-local";
  } else {
    throw new Error(
      `Model \"${modelConfig.name}\" config incomplete: ` +
      `provider=${modelConfig.provider}, apiKey=${modelConfig.apiKey ? "set" : "missing"}, ` +
      `baseUrl=${modelConfig.baseUrl || "missing"}`,
    );
  }

  if (modelConfig.model) {
    const modelEnvVars = [
      "ANTHROPIC_MODEL",
      "ANTHROPIC_DEFAULT_OPUS_MODEL",
      "ANTHROPIC_DEFAULT_SONNET_MODEL",
      "ANTHROPIC_DEFAULT_HAIKU_MODEL",
      "CLAUDE_CODE_SUBAGENT_MODEL",
    ];
    for (const key of modelEnvVars) {
      env[key] = modelConfig.model;
    }
  }

  return env;
}

function createTraceReporter(
  server: ServerClient,
  base: Pick<TraceEventPayload, "traceId" | "sessionId" | "agentId" | "userId" | "channel">,
) {
  return (
    event: Omit<TraceEventPayload, "traceId" | "sessionId" | "agentId" | "userId" | "channel">,
  ) => {
    if (TRACE_CONSOLE_ENABLED) {
      console.log(formatTraceEventLog(base, event));
    }
    server.reportTraceEvent({ ...base, ...event });
  };
}

async function resolveOrCreateSession(
  server: ServerClient,
  request: ProcessRequest,
): Promise<{ sessionId: string; sessionKey: string; sdkSessionId?: string; workDir: string }> {
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
    const workDir = resolveSessionWorkDir(sessionId);
    ensureDir(workDir);

    const title = request.content.substring(0, 30) + (request.content.length > 30 ? "..." : "");
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
  const workDir = resolveSessionWorkDir(sessionId, session.workDir);
  ensureDir(workDir);

  const updates: Record<string, string> = {};
  if (!session.sessionKey || session.sessionKey !== sessionKey) {
    updates.sessionKey = sessionKey;
  }
  if (!session.workDir || session.workDir !== workDir) {
    updates.workDir = workDir;
  }
  if (request.channelConversationId && !session.channelConversationId) {
    updates.channelConversationId = request.channelConversationId;
  }

  if (Object.keys(updates).length > 0) {
    await server.updateSession(sessionId, updates);
  }

  return {
    sessionId,
    sessionKey,
    sdkSessionId: session.sdkSessionId,
    workDir,
  };
}

/**
 * 确保 worker 持有一个活跃的 LiveSession。
 * 首次调用时加载配置、创建 MCP server、启动 SDK 子进程。
 * 后续调用直接复用（子进程常驻）。
 */
async function ensureLiveSession(
  worker: SessionRuntime,
  server: ServerClient,
  request: QueuedProcessRequest,
  report: (event: Omit<TraceEventPayload, "traceId" | "sessionId" | "agentId" | "userId" | "channel">) => void,
): Promise<void> {
  if (worker.liveSession && !worker.liveSession.closed) {
    report({ type: "thinking", timestamp: Date.now(), thinking: "复用已有 session，跳过配置加载。", source: "system" });
    return;
  }

  report({ type: "thinking", timestamp: Date.now(), thinking: "加载 Agent 配置与技能，创建持久 session。", source: "system" });

  const agentConfig = await server.getAgentConfig(request.agentId);
  if (!agentConfig) {
    throw new Error(`Agent not found: ${request.agentId}`);
  }

  // 从 agent_configs 的 provider/model 字段 + settings 表的 API Key 构建模型配置
  const provider = agentConfig.provider;
  const modelName = agentConfig.model;
  if (!provider || !modelName) {
    throw new Error(
      `Agent "${request.agentId}" has no provider/model configured ` +
      `(provider=${provider || "missing"}, model=${modelName || "missing"})`,
    );
  }

  const [credentials, skillsCtx] = await Promise.all([
    server.getProviderCredentials(provider),
    server.getSkillsContext(request.agentId),
  ]);

  if (!credentials || !credentials.apiKey) {
    throw new Error(
      `No API key configured for provider "${provider}". ` +
      `Please set it in Admin → Settings.`,
    );
  }

  const modelConfig: ModelConfig = {
    id: `${provider}:${modelName}`,
    name: `${provider}/${modelName}`,
    provider: provider as ModelConfig["provider"],
    baseUrl: credentials.baseUrl || undefined,
    apiKey: credentials.apiKey,
    model: modelName,
    maxTokens: 4096,
    temperature: 0.7,
  };

  const skillContext: SkillContext = skillsCtx || {
    systemPromptAddition: "",
    tools: [],
    toolExecutors: {},
    skillDocs: {},
  };

  const replaced = syncSkillsToWorkDir(worker.workDir, skillContext.skillDocs);
  if (replaced > 0) {
    console.log(`[sdk-runner] Skills synced for ${worker.sessionId}: ${replaced} file/dir changed`);
  }

  // 创建 mutable 运行时上下文（后续每条消息会更新 request/traceId/report/seen* 字段）
  const runtimeContext: SkillToolRuntimeContext = {
    server,
    request,
    sessionId: worker.sessionId,
    traceId: request.traceId,
    workDir: worker.workDir,
    skillDocs: skillContext.skillDocs,
    report,
    seenToolCallIds: new Set<string>(),
    finishedToolCallIds: new Set<string>(),
  };

  const skillMcpServer = buildSkillMcpServer(skillContext, runtimeContext);
  const sdkEnv = buildSdkEnv(modelConfig);
  const systemPrompt = buildSystemPrompt(
    agentConfig.systemPrompt,
    skillContext.systemPromptAddition,
  );

  // 上报加载完成的详细信息
  const skillNames = Object.keys(skillContext.skillDocs);
  const toolNames = skillContext.tools.map((t) => t.name);
  const systemPromptPreview = systemPrompt.length > 200
    ? systemPrompt.slice(0, 200) + "..."
    : systemPrompt;
  const detailLines: string[] = [
    `Agent: ${agentConfig.displayName} (${request.agentId})`,
    `Model: ${provider}/${modelName}${credentials.baseUrl ? ` @ ${credentials.baseUrl}` : ""}`,
    `Skills: ${skillNames.length > 0 ? skillNames.join(", ") : "（无）"}`,
    `Tools: ${toolNames.length > 0 ? toolNames.join(", ") : "（无）"}`,
    `Session: ${worker.sdkSessionId ? `resume ${worker.sdkSessionId}` : "新建"}`,
    `WorkDir: ${worker.workDir}`,
    "",
    "--- System Prompt ---",
    systemPromptPreview,
  ];
  report({
    type: "thinking",
    timestamp: Date.now(),
    thinking: detailLines.join("\n"),
    source: "system",
  });

  const abortController = new AbortController();
  activeAbortControllers.add(abortController);

  const queryOptions: Record<string, unknown> = {
    abortController,
    tools: { type: "preset", preset: "claude_code" },
    mcpServers: { skills: skillMcpServer },
    systemPrompt,
    permissionMode: "bypassPermissions",
    allowDangerouslySkipPermissions: true,
    includePartialMessages: true,
    cwd: worker.workDir,
    additionalDirectories: [PROJECT_ROOT],
    settingSources: ["project"],
    maxTurns: MAX_TURNS,
    env: sdkEnv,
    hooks: buildPreToolUseGuards(() => runtimeContext.report),
    ...(worker.sdkSessionId ? { resume: worker.sdkSessionId } : {}),
    ...(modelConfig.model ? { model: modelConfig.model } : {}),
  };

  const liveSession = new LiveSession(queryOptions);
  worker.liveSession = liveSession;
  worker.cachedRuntimeContext = runtimeContext;
  // 缓存工具名（trace 去重用）
  const allManagedToolNames = new Set(skillContext.tools.map((t) => t.name));
  const hasFeishuSkill = Object.keys(skillContext.skillDocs).some(
    (name) => name === "feishu-operator" || name.toLowerCase().includes("feishu"),
  );
  if (hasFeishuSkill) {
    for (const def of FEISHU_TOOL_DEFS) allManagedToolNames.add(def.name);
  }
  allManagedToolNames.add("send_channel_message");
  worker.cachedSkillToolNames = allManagedToolNames;

  console.log(
    `[sdk-runner][live-session-created] sessionId=${worker.sessionId} ` +
    `model=${modelConfig.model} sdkResume=${!!worker.sdkSessionId}`,
  );
}

async function processOneEvent(
  worker: SessionRuntime,
  request: QueuedProcessRequest,
  server: ServerClient,
): Promise<void> {
  const traceId = request.traceId;

  const report = createTraceReporter(server, {
    traceId,
    sessionId: worker.sessionId,
    agentId: request.agentId,
    userId: request.userId,
    channel: request.channel,
  });
  if (!request.traceStarted) {
    report({ type: "start", timestamp: Date.now(), initiator: "user" });
  }
  report({ type: "thinking", timestamp: Date.now(), thinking: "已接收事件，准备处理。", source: "system" });

  try {
    // 确保持久 session 存在（首条消息创建，后续复用）
    await ensureLiveSession(worker, server, request, report);

    const live = worker.liveSession!;
    const runtimeCtx = worker.cachedRuntimeContext!;
    const skillToolNames = worker.cachedSkillToolNames!;

    // 更新 mutable 上下文：每条消息的 request/traceId/report/dedup sets
    runtimeCtx.request = request;
    runtimeCtx.traceId = traceId;
    runtimeCtx.report = report;
    runtimeCtx.seenToolCallIds = new Set<string>();
    runtimeCtx.finishedToolCallIds = new Set<string>();

    // 发送消息到持久 session
    live.send(buildEventPrompt(request));
    report({ type: "thinking", timestamp: Date.now(), thinking: "消息已发送，等待模型响应。", source: "system" });

    // 处理本轮流式响应
    let resultMessage: SDKResultMessage | null = null;
    let nextSdkSessionId = worker.sdkSessionId;

    const toolCalls: Array<{ id: string; tool: string; input: unknown; result?: unknown }> = [];
    const toolStartTimes = new Map<string, number>();
    let streamThinkingBuffer = "";
    let sawStreamThinking = false;

    const flushStreamThinking = () => {
      const text = normalizeTextChunk(streamThinkingBuffer);
      streamThinkingBuffer = "";
      if (!text) return;
      report({ type: "thinking", timestamp: Date.now(), thinking: text, source: "model" });
    };

    for await (const msg of live.stream()) {
      const msgSessionId = (msg as { session_id?: string }).session_id;
      if (msgSessionId) {
        nextSdkSessionId = msgSessionId;
      }

      if (msg.type === "stream_event") {
        const text = extractStreamThinkingText((msg as { event?: unknown }).event);
        if (text) {
          sawStreamThinking = true;
          streamThinkingBuffer += text;
          if (text.includes("\n") || /[.!?。！？]$/.test(text) || streamThinkingBuffer.length >= 160) {
            flushStreamThinking();
          }
        }
      }

      if (msg.type === "tool_progress") {
        flushStreamThinking();
        report({
          type: "thinking",
          timestamp: Date.now(),
          thinking: `工具 ${(msg as any).tool_name} 执行中（${(msg as any).elapsed_time_seconds}s）`,
          source: "system",
        });
      }

      if (msg.type === "assistant") {
        for (const block of (msg as any).message.content) {
          if (block.type === "text" && block.text) {
            if (!sawStreamThinking) {
              report({ type: "thinking", timestamp: Date.now(), thinking: block.text, source: "model" });
            }
          }
          if (block.type === "tool_use") {
            flushStreamThinking();
            toolCalls.push({ id: block.id, tool: block.name, input: block.input });
            toolStartTimes.set(block.id, Date.now());
            if (skillToolNames.has(block.name)) {
              continue;
            }
            if (!runtimeCtx.seenToolCallIds.has(block.id)) {
              runtimeCtx.seenToolCallIds.add(block.id);
              report({
                type: "tool_call",
                timestamp: Date.now(),
                toolCallId: block.id,
                toolName: block.name,
                toolInput: block.input,
              });
            }
          }
        }
      }

      if (msg.type === "user" && (msg as { tool_use_result?: unknown }).tool_use_result !== undefined) {
        const parentId = (msg as { parent_tool_use_id?: string }).parent_tool_use_id || "";
        const result = (msg as { tool_use_result?: unknown }).tool_use_result;
        const tc = toolCalls.find((t) => t.id === parentId);
        if (tc) tc.result = result;

        const started = toolStartTimes.get(parentId);
        const duration = started ? Date.now() - started : undefined;
        toolStartTimes.delete(parentId);
        if (tc?.tool && skillToolNames.has(tc.tool)) {
          continue;
        }

        if (!runtimeCtx.finishedToolCallIds.has(parentId)) {
          runtimeCtx.finishedToolCallIds.add(parentId);
          report({
            type: "tool_result",
            timestamp: Date.now(),
            toolCallId: parentId,
            toolName: tc?.tool || "unknown",
            toolResult: result,
            toolDuration: duration,
            toolSuccess: true,
          });
        }
      }

      if (msg.type === "result") {
        flushStreamThinking();
        resultMessage = msg as SDKResultMessage;
      }
    }
    flushStreamThinking();

    // 更新 sdkSessionId
    if (nextSdkSessionId && nextSdkSessionId !== worker.sdkSessionId) {
      worker.sdkSessionId = nextSdkSessionId;
      await server.updateSession(worker.sessionId, { sdkSessionId: worker.sdkSessionId });
      console.log(`[sdk-runner] Session updated sdkSessionId=${worker.sdkSessionId}`);
    }

    const usage: Usage | undefined = resultMessage
      ? {
          inputTokens: resultMessage.usage?.input_tokens || 0,
          outputTokens: resultMessage.usage?.output_tokens || 0,
          totalCostUsd: resultMessage.total_cost_usd || 0,
        }
      : undefined;

    report({ type: "done", timestamp: Date.now(), usage });
  } catch (err) {
    // 出错时关闭 live session，下条消息会重建
    if (worker.liveSession) {
      activeAbortControllers.delete(worker.liveSession.abortController);
      worker.liveSession.close();
      worker.liveSession = undefined;
      worker.cachedRuntimeContext = undefined;
      worker.cachedSkillToolNames = undefined;
    }
    const error = err instanceof Error ? err.message : "Unknown error";
    report({ type: "error", timestamp: Date.now(), error });
    report({ type: "done", timestamp: Date.now(), error });
    throw err;
  }
}

async function drainSessionWorker(worker: SessionRuntime): Promise<void> {
  const server = new ServerClient();

  try {
    while (worker.queue.length > 0) {
      const request = worker.queue.shift()!;
      console.log(
        `[sdk-runner][dequeue] sessionId=${worker.sessionId} traceId=${request.traceId} queueRemaining=${worker.queue.length}`,
      );

      await server.updateSession(worker.sessionId, {
        executionStatus: "processing",
        sdkSessionId: worker.sdkSessionId,
        workDir: worker.workDir,
      });

      try {
        await processOneEvent(worker, request, server);
        console.log(
          `[sdk-runner][event-done] sessionId=${worker.sessionId} traceId=${request.traceId} queueRemaining=${worker.queue.length}`,
        );
      } catch (err) {
        const error = err instanceof Error ? err.message : "Unknown error";
        console.error(`[sdk-runner] Error in session ${worker.sessionId}:`, error);
        try {
          await server.updateSession(worker.sessionId, { executionStatus: "interrupted" });
        } catch {
          // 忽略上报失败
        }
      }
    }

    await server.updateSession(worker.sessionId, {
      executionStatus: "completed",
      sdkSessionId: worker.sdkSessionId,
      workDir: worker.workDir,
    });
  } finally {
    worker.processing = false;

    if (worker.queue.length > 0) {
      worker.processing = true;
      console.log(
        `[sdk-runner][drain-continue] sessionId=${worker.sessionId} queueRemaining=${worker.queue.length}`,
      );
      void drainSessionWorker(worker);
      return;
    }

    // 队列已空：不立即删除 worker，而是启动空闲计时器。
    // LiveSession（SDK 子进程）在空闲期间保持常驻，下条消息直接复用。
    const hasLive = !!worker.liveSession && !worker.liveSession.closed;
    console.log(
      `[sdk-runner][drain-idle] sessionId=${worker.sessionId} ` +
      `liveSession=${hasLive} idleTimeoutMs=${SESSION_IDLE_TIMEOUT_MS}`,
    );
    resetSessionIdleTimer(worker);
  }
}

async function enqueueProcessRequestInner(request: ProcessRequest): Promise<void> {
  const server = new ServerClient();
  const resolved = await resolveOrCreateSession(server, request);
  const traceId = request.traceId || crypto.randomUUID();

  let worker = sessionWorkers.get(resolved.sessionId);
  if (!worker) {
    worker = {
      sessionId: resolved.sessionId,
      sessionKey: resolved.sessionKey,
      sdkSessionId: resolved.sdkSessionId,
      workDir: resolved.workDir,
      queue: [],
      processing: false,
      lastActivityAt: Date.now(),
    };
    sessionWorkers.set(resolved.sessionId, worker);
  } else {
    worker.sessionKey = resolved.sessionKey;
    worker.workDir = resolved.workDir;
    worker.lastActivityAt = Date.now();
    if (!worker.sdkSessionId && resolved.sdkSessionId) {
      worker.sdkSessionId = resolved.sdkSessionId;
    }
    // 新消息到达：清除空闲计时器（即将开始处理）
    if (worker.idleTimer) {
      clearTimeout(worker.idleTimer);
      worker.idleTimer = undefined;
    }
  }

  try {
    await server.updateSession(resolved.sessionId, {
      executionStatus: "processing",
      sdkSessionId: worker.sdkSessionId,
      workDir: worker.workDir,
    });
  } catch (err) {
    const error = err instanceof Error ? err.message : "Unknown error";
    console.warn(`[sdk-runner] Failed to mark session processing (${resolved.sessionId}): ${error}`);
  }

  const queuedRequest: QueuedProcessRequest = {
    ...request,
    sessionId: resolved.sessionId,
    traceId,
    traceStarted: false,
  };

  console.log(
    `[sdk-runner][enqueue] sessionId=${resolved.sessionId} traceId=${traceId} messageId=${request.messageId} queueBefore=${worker.queue.length}`,
  );

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
    const error = err instanceof Error ? err.message : "Unknown error";
    console.warn(`[sdk-runner] Failed to persist start trace (${traceId}): ${error}`);
  }

  worker.queue.push(queuedRequest);
  console.log(
    `[sdk-runner][queued] sessionId=${resolved.sessionId} traceId=${traceId} queueAfter=${worker.queue.length}`,
  );

  if (!worker.processing) {
    console.log(
      `[sdk-runner][drain-start] sessionId=${resolved.sessionId} traceId=${traceId} reason=idle_worker`,
    );
    worker.processing = true;
    void drainSessionWorker(worker);
  } else {
    console.log(
      `[sdk-runner][drain-pending] sessionId=${resolved.sessionId} traceId=${traceId} reason=worker_busy`,
    );
  }
}

/**
 * 入队处理 event。
 *
 * 规则：
 * - session 不存在：创建 session + worker
 * - session 存在但未激活：加载 sdkSessionId 后激活
 * - session 运行中：追加到队列
 */
export async function enqueueProcessRequest(request: ProcessRequest): Promise<void> {
  if (shuttingDown) {
    throw new Error("Agent is shutting down, not accepting new requests");
  }

  const key = request.sessionId || resolveSessionKey(
    request.channel,
    request.channelUserId,
    request.channelConversationId,
  );

  const prev = enqueueLocks.get(key)?.catch(() => {
    // 前序失败不阻塞后续请求
  }) || Promise.resolve();

  const current = prev.then(() => enqueueProcessRequestInner(request));
  enqueueLocks.set(key, current);

  try {
    await current;
  } finally {
    if (enqueueLocks.get(key) === current) {
      enqueueLocks.delete(key);
    }
  }
}

/**
 * 清理中断的 session（Agent 启动时调用）
 */
export async function cleanupInterruptedSessions(): Promise<number> {
  const server = new ServerClient();
  const interrupted = await server.getInterruptedSessions();

  if (interrupted.length === 0) return 0;

  let cleaned = 0;
  for (const session of interrupted) {
    try {
      await server.updateSession(session.id, { executionStatus: "completed" });
      cleaned++;
    } catch {
      // 忽略
    }
  }

  return cleaned;
}

/** 优雅终止超时（毫秒） */
const SHUTDOWN_TIMEOUT_MS = 15_000;

/**
 * 优雅终止：
 * 1. 拒绝新请求
 * 2. abort 所有活跃的 SDK 子进程
 * 3. 将运行中的 session 标记为 interrupted
 * 4. 等待活跃 worker 处理完毕（带超时）
 */
export async function gracefulShutdown(): Promise<void> {
  if (shuttingDown) return;
  shuttingDown = true;

  const liveCount = [...sessionWorkers.values()].filter((w) => w.liveSession && !w.liveSession.closed).length;
  console.log(
    `[sdk-runner][shutdown] Starting graceful shutdown... ` +
    `activeQueries=${activeAbortControllers.size} activeWorkers=${sessionWorkers.size} ` +
    `liveSessions=${liveCount}`,
  );

  // 1. 关闭所有空闲计时器 + 持久 LiveSession
  for (const [sessionId, worker] of sessionWorkers) {
    if (worker.idleTimer) {
      clearTimeout(worker.idleTimer);
      worker.idleTimer = undefined;
    }
    if (worker.liveSession && !worker.liveSession.closed) {
      worker.liveSession.close();
      console.log(`[sdk-runner][shutdown] Closed live session for ${sessionId}`);
    }
  }

  // 2. Abort 所有活跃的 SDK query（终止子进程）
  for (const ac of activeAbortControllers) {
    try {
      ac.abort();
    } catch {
      // 忽略 abort 异常
    }
  }

  if (activeAbortControllers.size > 0) {
    console.log(`[sdk-runner][shutdown] Aborted ${activeAbortControllers.size} active query(s)`);
  }

  // 3. 清空所有 worker 的待处理队列（不再执行）
  for (const [sessionId, worker] of sessionWorkers) {
    const dropped = worker.queue.length;
    worker.queue.length = 0;
    if (dropped > 0) {
      console.log(`[sdk-runner][shutdown] Dropped ${dropped} queued event(s) for session ${sessionId}`);
    }
  }

  // 4. 将所有运行中的 session 标记为 interrupted
  const server = new ServerClient();
  const markPromises: Promise<void>[] = [];
  for (const [sessionId, worker] of sessionWorkers) {
    if (worker.processing) {
      markPromises.push(
        server
          .updateSession(sessionId, { executionStatus: "interrupted" })
          .then(() => {
            console.log(`[sdk-runner][shutdown] Marked session ${sessionId} as interrupted`);
          })
          .catch((err) => {
            console.warn(`[sdk-runner][shutdown] Failed to mark session ${sessionId}:`, err);
          }),
      );
    }
  }
  await Promise.allSettled(markPromises);

  // 5. 等待所有 worker 停止（带超时）
  if (sessionWorkers.size > 0) {
    console.log(`[sdk-runner][shutdown] Waiting for ${sessionWorkers.size} worker(s) to stop (timeout=${SHUTDOWN_TIMEOUT_MS}ms)...`);

    await Promise.race([
      new Promise<void>((resolve) => {
        const check = () => {
          if (sessionWorkers.size === 0 && activeAbortControllers.size === 0) {
            resolve();
          } else {
            setTimeout(check, 200);
          }
        };
        check();
      }),
      new Promise<void>((resolve) => {
        setTimeout(() => {
          if (sessionWorkers.size > 0 || activeAbortControllers.size > 0) {
            console.warn(
              `[sdk-runner][shutdown] Timeout reached. ` +
              `Remaining: workers=${sessionWorkers.size} queries=${activeAbortControllers.size}`,
            );
          }
          // 强制清理残留
          sessionWorkers.clear();
          activeAbortControllers.clear();
          resolve();
        }, SHUTDOWN_TIMEOUT_MS);
      }),
    ]);
  }

  console.log("[sdk-runner][shutdown] Graceful shutdown complete");
}
