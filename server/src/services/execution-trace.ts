/**
 * 执行链路追踪存储
 *
 * 持久化 Agent 执行过程中的每一步决策和工具调用，
 * 支持实时推送（通过 observation bus）和历史查询。
 *
 * 数据流：
 *   Agent processMessage() → POST /api/traces/events → handleTraceEvent()
 *     → 1) SQLite 持久化（execution_traces + execution_steps）
 *     → 2) observation bus 实时推送（MonitorView SSE 消费）
 *
 * 设计原则：
 *   - 事件驱动：Agent 上报，Server 只做存储 + 转发
 *   - 不阻塞 Agent：Agent fire-and-forget，Server 同步写入（SQLite 本地写入微秒级）
 *   - 双通道输出：实时 SSE + 历史 SQL 查询
 */

import { observationBus } from "./observation-bus";
import { db } from "./database";

// ==================== Schema ====================

db.run(`
  CREATE TABLE IF NOT EXISTS execution_traces (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    agent_id TEXT,
    user_id TEXT,
    channel TEXT,
    status TEXT DEFAULT 'running',
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    total_cost_usd REAL DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
  )
`);

db.run(`
  CREATE TABLE IF NOT EXISTS execution_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    iteration INTEGER DEFAULT 1,
    timestamp INTEGER NOT NULL,
    type TEXT NOT NULL,
    thinking TEXT,
    tool_call_id TEXT,
    tool_name TEXT,
    tool_input TEXT,
    tool_result TEXT,
    tool_duration INTEGER,
    tool_success INTEGER,
    content TEXT,
    error TEXT,
    FOREIGN KEY (trace_id) REFERENCES execution_traces(id)
  )
`);

try {
  db.run(`CREATE INDEX IF NOT EXISTS idx_execution_steps_trace ON execution_steps(trace_id)`);
  db.run(`CREATE INDEX IF NOT EXISTS idx_execution_traces_session ON execution_traces(session_id)`);
  db.run(`CREATE INDEX IF NOT EXISTS idx_execution_traces_started ON execution_traces(started_at)`);
  db.run(`CREATE INDEX IF NOT EXISTS idx_execution_traces_status ON execution_traces(status)`);
} catch { /* already exists */ }

// 迁移：新增 source 列（model/system 区分 thinking 来源）
try {
  db.run(`ALTER TABLE execution_steps ADD COLUMN source TEXT`);
} catch { /* 列已存在 */ }

// 迁移：新增 model_input / model_output 列（存储每次 LLM 调用的 I/O 摘要）
try {
  db.run(`ALTER TABLE execution_steps ADD COLUMN model_input TEXT`);
} catch { /* 列已存在 */ }
try {
  db.run(`ALTER TABLE execution_steps ADD COLUMN model_output TEXT`);
} catch { /* 列已存在 */ }

// 迁移：新增 model_input / model_output 列（LLM 调用完整 I/O 摘要）
try {
  db.run(`ALTER TABLE execution_steps ADD COLUMN model_input TEXT`);
} catch { /* 列已存在 */ }
try {
  db.run(`ALTER TABLE execution_steps ADD COLUMN model_output TEXT`);
} catch { /* 列已存在 */ }

// ==================== Types ====================

/** 完整的执行链路（含步骤） */
export interface ExecutionTrace {
  id: string;
  sessionId: string;
  agentId?: string;
  userId?: string;
  channel?: string;
  status: "running" | "completed" | "error";
  startedAt: number;
  completedAt?: number;
  inputTokens: number;
  outputTokens: number;
  totalCostUsd: number;
  steps: ExecutionStep[];
}

/** 单个执行步骤 */
export interface ExecutionStep {
  index: number;
  iteration: number;
  timestamp: number;
  type: "thinking" | "tool_call" | "tool_result" | "content" | "error" | "model_io";
  // thinking
  thinking?: string;
  /** thinking 来源：model = 模型推理, system = 系统状态日志 */
  source?: "model" | "system";
  // tool_call / tool_result
  toolCallId?: string;
  toolName?: string;
  toolInput?: unknown;
  toolResult?: unknown;
  toolDuration?: number;
  toolSuccess?: boolean;
  // content
  content?: string;
  // error
  error?: string;
  // model_io：每次 LLM 调用的完整输入/输出摘要
  modelInput?: unknown;
  modelOutput?: unknown;
}

/** Agent 上报的事件格式 */
export interface TraceEvent {
  traceId: string;
  sessionId: string;
  agentId?: string;
  userId?: string;
  channel?: string;
  type: "start" | "thinking" | "tool_call" | "tool_result" | "content" | "error" | "done" | "model_io";
  timestamp: number;
  /** Agent Loop 的迭代轮次（从 1 开始，Agent 直报） */
  iteration?: number;
  // start
  initiator?: "user" | "agent" | "system";
  // thinking
  thinking?: string;
  /** thinking 来源：model = 模型推理, system = 系统状态日志 */
  source?: "model" | "system";
  // tool_call
  toolCallId?: string;
  toolName?: string;
  toolInput?: unknown;
  // tool_result
  toolResult?: unknown;
  toolDuration?: number;
  toolSuccess?: boolean;
  // content
  content?: string;
  // error
  error?: string;
  // done
  usage?: { inputTokens: number; outputTokens: number; totalCostUsd: number };
  // model_io
  modelInput?: unknown;
  modelOutput?: unknown;
}

// ==================== Prepared Statements ====================

const insertTrace = db.prepare(`
  INSERT OR IGNORE INTO execution_traces (id, session_id, agent_id, user_id, channel, status, started_at)
  VALUES (?, ?, ?, ?, ?, 'running', ?)
`);

const insertStep = db.prepare(`
  INSERT INTO execution_steps (trace_id, step_index, iteration, timestamp, type, thinking, source, tool_call_id, tool_name, tool_input, tool_result, tool_duration, tool_success, content, error, model_input, model_output)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`);

const updateTraceCompleted = db.prepare(`
  UPDATE execution_traces SET status = ?, completed_at = ?, input_tokens = ?, output_tokens = ?, total_cost_usd = ? WHERE id = ?
`);

const selectTraceById = db.prepare(`
  SELECT * FROM execution_traces WHERE id = ?
`);

const selectStepsByTrace = db.prepare(`
  SELECT * FROM execution_steps WHERE trace_id = ? ORDER BY step_index ASC
`);

const selectTracesBySession = db.prepare(`
  SELECT * FROM execution_traces WHERE session_id = ? ORDER BY started_at DESC
`);

const selectRecentTraces = db.prepare(`
  SELECT * FROM execution_traces ORDER BY started_at DESC LIMIT ?
`);

// ==================== Per-Trace State ====================

interface TraceState {
  stepIndex: number;
  iteration: number;
  lastPhase: "" | "think" | "act" | "observe";
}

const traceStates = new Map<string, TraceState>();

// ==================== Event Handler ====================

/**
 * 处理来自 Agent 的事件上报
 *
 * 同时执行两件事：
 * 1. 写入 SQLite（持久化）
 * 2. 推送 observation bus（实时 SSE）
 */
export function handleTraceEvent(event: TraceEvent): void {
  const { traceId, sessionId, agentId, userId, channel, type, timestamp } = event;

  // ── start: 创建 trace 记录 ──
  if (type === "start") {
    const result = insertTrace.run(
      traceId,
      sessionId,
      agentId || null,
      userId || null,
      channel || null,
      timestamp,
    ) as { changes?: number };
    const inserted = typeof result.changes === "number" ? result.changes > 0 : true;

    // 同一个 traceId 的重复 start 只接收第一次，避免重置 step 状态和重复推送 execution_start。
    if (inserted) {
      traceStates.set(traceId, { stepIndex: 0, iteration: 0, lastPhase: "" });

      observationBus.emit({
        type: "execution_start",
        sessionId,
        agentId,
        userId,
        channel,
        traceId,
        initiator: event.initiator,
        timestamp,
      });
    }
    return;
  }

  // ── done: 更新 trace 完成状态 ──
  if (type === "done") {
    updateTraceCompleted.run(
      event.error ? "error" : "completed",
      timestamp,
      event.usage?.inputTokens || 0,
      event.usage?.outputTokens || 0,
      event.usage?.totalCostUsd || 0,
      traceId,
    );
    traceStates.delete(traceId);

    observationBus.emit({
      type: "execution_done",
      sessionId,
      agentId,
      userId,
      channel,
      traceId,
      timestamp,
      data: {
        usage: event.usage,
        error: event.error,
      },
    });
    return;
  }

  // ── 执行步骤：thinking / tool_call / tool_result / content / error ──

  const state = traceStates.get(traceId) || { stepIndex: 0, iteration: 0, lastPhase: "" as const };
  const stepIndex = state.stepIndex++;

  // 优先使用 Agent 直报的 iteration，回退到 Server 端启发式推断
  if (event.iteration != null) {
    state.iteration = event.iteration;
  } else {
    if (type === "thinking" || type === "content") {
      if (state.lastPhase === "observe" || state.lastPhase === "") {
        state.iteration++;
      }
    } else if (type === "tool_call") {
      if (state.iteration === 0) state.iteration = 1;
    } else if (type === "error") {
      if (state.iteration === 0) state.iteration = 1;
    }
  }

  if (type === "thinking" || type === "content") {
    state.lastPhase = "think";
  } else if (type === "tool_call") {
    state.lastPhase = "act";
  } else if (type === "tool_result") {
    state.lastPhase = "observe";
  }

  traceStates.set(traceId, state);

  // 写入 DB
  insertStep.run(
    traceId,
    stepIndex,
    state.iteration,
    timestamp,
    type,
    event.thinking || null,
    event.source || null,
    event.toolCallId || null,
    event.toolName || null,
    event.toolInput ? JSON.stringify(event.toolInput) : null,
    event.toolResult ? (typeof event.toolResult === "string" ? event.toolResult : JSON.stringify(event.toolResult)) : null,
    event.toolDuration ?? null,
    event.toolSuccess !== undefined ? (event.toolSuccess ? 1 : 0) : null,
    event.content || null,
    event.error || null,
    event.modelInput ? JSON.stringify(event.modelInput) : null,
    event.modelOutput ? JSON.stringify(event.modelOutput) : null,
  );

  // 推送 observation bus（实时 SSE）
  if (type === "thinking" || type === "content") {
    observationBus.emit({
      type: "reasoning",
      sessionId,
      agentId,
      userId,
      channel,
      traceId,
      timestamp,
      data: { text: event.thinking || event.content, source: event.source },
    });
  } else if (type === "tool_call") {
    observationBus.emit({
      type: "tool_call",
      sessionId,
      agentId,
      userId,
      channel,
      traceId,
      timestamp,
      data: { id: event.toolCallId, tool: event.toolName, input: event.toolInput },
    });
  } else if (type === "tool_result") {
    observationBus.emit({
      type: "tool_result",
      sessionId,
      agentId,
      userId,
      channel,
      traceId,
      timestamp,
      data: { id: event.toolCallId, tool: event.toolName, result: event.toolResult, duration: event.toolDuration, success: event.toolSuccess },
    });
  } else if (type === "error") {
    observationBus.emit({
      type: "error",
      sessionId,
      agentId,
      userId,
      channel,
      traceId,
      timestamp,
      data: { error: event.error },
    });
  }

  // 构建 decision_step（供 DecisionTimeline 前端组件使用）
  const decisionStep = buildDecisionStep(event, stepIndex, state.iteration);
  if (decisionStep) {
    observationBus.emit({
      type: "decision_step",
      sessionId,
      agentId,
      userId,
      channel,
      traceId,
      timestamp,
      data: { step: decisionStep },
    });
  }
}

// ==================== Decision Step Builder ====================

/**
 * 将 Agent 事件映射为 DecisionTimeline 需要的 DecisionStep 格式
 */
function buildDecisionStep(
  event: TraceEvent,
  stepIndex: number,
  iteration: number,
): Record<string, unknown> | null {
  switch (event.type) {
    case "thinking":
      return {
        index: stepIndex,
        iteration,
        timestamp: event.timestamp,
        phase: "think",
        summary: (event.thinking || "").substring(0, 120),
        reasoning: event.thinking,
        source: event.source || "model",
      };

    case "content":
      return {
        index: stepIndex,
        iteration,
        timestamp: event.timestamp,
        phase: "think",
        summary: (event.content || "").substring(0, 120),
        reasoning: event.content,
        source: event.source || "model",
      };

    case "tool_call":
      return {
        index: stepIndex,
        iteration,
        timestamp: event.timestamp,
        phase: "act",
        summary: `调用 ${event.toolName}`,
        tool: event.toolName,
        toolInput: event.toolInput,
        toolId: event.toolCallId,
      };

    case "tool_result": {
      const resultStr = typeof event.toolResult === "string"
        ? event.toolResult.substring(0, 500)
        : JSON.stringify(event.toolResult).substring(0, 500);
      return {
        index: stepIndex,
        iteration,
        timestamp: event.timestamp,
        phase: "observe",
        summary: `${event.toolName} ${event.toolSuccess !== false ? "完成" : "失败"}`,
        tool: event.toolName,
        toolResult: resultStr,
        success: event.toolSuccess !== false,
        duration: event.toolDuration,
        toolId: event.toolCallId,
      };
    }

    case "model_io":
      return {
        index: stepIndex,
        iteration,
        timestamp: event.timestamp,
        phase: "model_io",
        summary: `LLM 调用 #${iteration}`,
        modelInput: event.modelInput,
        modelOutput: event.modelOutput,
      };

    default:
      return null;
  }
}

// ==================== Query API ====================

/**
 * 查询完整的执行链路（含所有步骤）
 */
export function getTrace(traceId: string): ExecutionTrace | null {
  const trace = selectTraceById.get(traceId) as any;
  if (!trace) return null;

  const steps = (selectStepsByTrace.all(traceId) as any[]).map(rowToStep);

  return {
    id: trace.id,
    sessionId: trace.session_id,
    agentId: trace.agent_id || undefined,
    userId: trace.user_id || undefined,
    channel: trace.channel || undefined,
    status: trace.status,
    startedAt: trace.started_at,
    completedAt: trace.completed_at || undefined,
    inputTokens: trace.input_tokens || 0,
    outputTokens: trace.output_tokens || 0,
    totalCostUsd: trace.total_cost_usd || 0,
    steps,
  };
}

/**
 * 查询 session 的所有链路（不含步骤，需用 getTrace 单独查）
 */
export function getTracesBySessionId(sessionId: string): Omit<ExecutionTrace, "steps">[] {
  return (selectTracesBySession.all(sessionId) as any[]).map(rowToTrace);
}

/**
 * 查询最近的链路
 */
export function getRecentTracesList(limit = 50): Omit<ExecutionTrace, "steps">[] {
  return (selectRecentTraces.all(limit) as any[]).map(rowToTrace);
}

// ==================== 增强查询：带步骤摘要 ====================

/** Trace 摘要信息（含步骤统计，用于列表展示） */
export interface TraceSummary extends Omit<ExecutionTrace, "steps"> {
  thinkingCount: number;
  toolCallCount: number;
  toolErrorCount: number;
  /** 工具名列表（去重） */
  toolNames: string[];
  /** 最后一条 thinking 的摘要 */
  lastThinking?: string;
  /** 最后一个错误 */
  lastError?: string;
}

const selectActiveTraces = db.prepare(`
  SELECT * FROM execution_traces WHERE status = 'running' ORDER BY started_at DESC
`);

const selectRecentTracesWithLimit = db.prepare(`
  SELECT * FROM execution_traces ORDER BY started_at DESC LIMIT ?
`);

const selectStepSummary = db.prepare(`
  SELECT
    COUNT(CASE WHEN type = 'thinking' AND (source IS NULL OR source = 'model') THEN 1 END) as thinking_count,
    COUNT(CASE WHEN type = 'tool_call' THEN 1 END) as tool_call_count,
    COUNT(CASE WHEN type = 'tool_result' AND tool_success = 0 THEN 1 END) as tool_error_count,
    GROUP_CONCAT(DISTINCT CASE WHEN type = 'tool_call' THEN tool_name END) as tool_names
  FROM execution_steps WHERE trace_id = ?
`);

const selectLastThinking = db.prepare(`
  SELECT thinking FROM execution_steps
  WHERE trace_id = ? AND type = 'thinking' AND (source IS NULL OR source = 'model')
  ORDER BY step_index DESC LIMIT 1
`);

const selectLastError = db.prepare(`
  SELECT error, tool_result FROM execution_steps
  WHERE trace_id = ? AND (type = 'error' OR (type = 'tool_result' AND tool_success = 0))
  ORDER BY step_index DESC LIMIT 1
`);

/**
 * 查询当前所有活跃（running）的 trace，含步骤摘要
 */
export function getActiveTraces(): TraceSummary[] {
  const rows = selectActiveTraces.all() as any[];
  return rows.map(rowToTraceSummary);
}

/**
 * 查询最近的 trace，含步骤摘要
 */
export function getRecentTraceSummaries(limit = 30): TraceSummary[] {
  const rows = selectRecentTracesWithLimit.all(limit) as any[];
  return rows.map(rowToTraceSummary);
}

function rowToTraceSummary(row: any): TraceSummary {
  const base = rowToTrace(row);
  const summary = selectStepSummary.get(row.id) as any;
  const lastThinkRow = selectLastThinking.get(row.id) as any;
  const lastErrorRow = selectLastError.get(row.id) as any;

  return {
    ...base,
    thinkingCount: summary?.thinking_count || 0,
    toolCallCount: summary?.tool_call_count || 0,
    toolErrorCount: summary?.tool_error_count || 0,
    toolNames: summary?.tool_names ? summary.tool_names.split(",").filter(Boolean) : [],
    lastThinking: lastThinkRow?.thinking ? lastThinkRow.thinking.substring(0, 200) : undefined,
    lastError: lastErrorRow?.error || (lastErrorRow?.tool_result ? String(lastErrorRow.tool_result).substring(0, 200) : undefined),
  };
}

// ==================== Helpers ====================

function rowToTrace(row: any): Omit<ExecutionTrace, "steps"> {
  return {
    id: row.id,
    sessionId: row.session_id,
    agentId: row.agent_id || undefined,
    userId: row.user_id || undefined,
    channel: row.channel || undefined,
    status: row.status,
    startedAt: row.started_at,
    completedAt: row.completed_at || undefined,
    inputTokens: row.input_tokens || 0,
    outputTokens: row.output_tokens || 0,
    totalCostUsd: row.total_cost_usd || 0,
  };
}

function rowToStep(row: any): ExecutionStep {
  return {
    index: row.step_index,
    iteration: row.iteration || 1,
    timestamp: row.timestamp,
    type: row.type,
    thinking: row.thinking || undefined,
    source: row.source || undefined,
    toolCallId: row.tool_call_id || undefined,
    toolName: row.tool_name || undefined,
    toolInput: row.tool_input ? tryParseJSON(row.tool_input) : undefined,
    toolResult: row.tool_result ? tryParseJSON(row.tool_result) : undefined,
    toolDuration: row.tool_duration || undefined,
    toolSuccess: row.tool_success !== null ? !!row.tool_success : undefined,
    content: row.content || undefined,
    error: row.error || undefined,
    modelInput: row.model_input ? tryParseJSON(row.model_input) : undefined,
    modelOutput: row.model_output ? tryParseJSON(row.model_output) : undefined,
  };
}

function tryParseJSON(str: string): unknown {
  try {
    return JSON.parse(str);
  } catch {
    return str;
  }
}
