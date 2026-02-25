/**
 * ReAct Agent 引擎 — 核心类型定义
 *
 * 所有类型遵循 Anthropic Messages API 的原生格式，
 * 减少中间转换层，保持最大透明度。
 */

// ==================== LLM 消息格式 ====================

export interface TextBlock {
  type: "text";
  text: string;
}

export interface ToolUseBlock {
  type: "tool_use";
  id: string;
  name: string;
  input: Record<string, unknown>;
}

export interface ToolResultBlock {
  type: "tool_result";
  tool_use_id: string;
  content: string;
  is_error?: boolean;
}

export type ContentBlock = TextBlock | ToolUseBlock | ToolResultBlock;

export interface AgentMessage {
  role: "user" | "assistant";
  content: string | ContentBlock[];
}

// ==================== 工具定义 ====================

export interface ToolDefinition {
  name: string;
  description: string;
  input_schema: {
    type: "object";
    properties: Record<string, unknown>;
    required?: string[];
  };
}

/**
 * 工具执行函数签名。
 * 接收工具参数，返回结果字符串或对象。
 * 抛出异常表示执行失败。
 */
export type ToolExecutor = (input: Record<string, unknown>) => Promise<unknown>;

/** 已注册的工具 = 定义 + 执行器 */
export interface RegisteredTool {
  definition: ToolDefinition;
  execute: ToolExecutor;
  /** 来源标识：skill / mcp / builtin */
  source: "skill" | "mcp" | "builtin";
  /** 来源名称（skill 名、MCP server 名等） */
  sourceName: string;
}

// ==================== Agent 事件（可观测性） ====================

export interface AgentEvent {
  type: "thinking" | "tool_call" | "tool_result" | "error" | "done" | "model_io";
  timestamp: number;
  /** 当前迭代轮次（从 1 开始） */
  iteration?: number;
  /** 模型的文本推理（Thought） */
  thinking?: string;
  source?: "model" | "system";
  /** 工具调用 */
  toolCallId?: string;
  toolName?: string;
  toolInput?: unknown;
  /** 工具结果 */
  toolResult?: unknown;
  toolDuration?: number;
  toolSuccess?: boolean;
  /** 错误信息 */
  error?: string;
  /** 完成时的 token 使用统计 */
  usage?: TokenUsage;
  /** 模型 I/O 观测（每次 LLM 调用的完整输入/输出摘要） */
  modelInput?: unknown;
  modelOutput?: unknown;
}

export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  totalCostUsd: number;
}

/** 事件回调：引擎每产生一个事件就调用一次 */
export type AgentEventHandler = (event: AgentEvent) => void;

// ==================== Agent Loop 配置 ====================

export interface AgentLoopConfig {
  /** LLM 调用器 */
  llmClient: LLMClient;
  /** System Prompt */
  systemPrompt: string;
  /** 历史消息 */
  messages: AgentMessage[];
  /** 已注册的工具列表 */
  tools: RegisteredTool[];
  /** 事件回调（可观测性挂载点） */
  onEvent: AgentEventHandler;
  /**
   * 每轮工具调用完成后的持久化回调。
   *
   * 引擎在每个 iteration 的 tool_use + tool_result 都产生后调用此回调，
   * 使调用方可以增量持久化，而非等整个 loop 结束后批量写入。
   * 这样即使进程崩溃，已完成的工具调用结果不会丢失。
   *
   * 参数为本轮新增的消息对（assistant tool_use + user tool_result）。
   * 回调失败不中断 loop，仅通过 onEvent 上报错误。
   */
  onNewMessages?: (messages: AgentMessage[]) => Promise<void>;
  /** 最大工具调用轮次（防止无限循环） */
  maxIterations?: number;
  /** 模型 ID（用于传给 LLM Client） */
  model?: string;
  /** AbortSignal（用于外部中断） */
  signal?: AbortSignal;
}

export interface AgentLoopResult {
  /** 最终的 assistant 文本回复（如果有） */
  finalText?: string;
  /** 完整的消息历史（含工具调用过程） */
  messages: AgentMessage[];
  /** Token 使用统计 */
  usage: TokenUsage;
  /** 是否因达到 maxIterations 而终止 */
  hitMaxIterations: boolean;
}

// ==================== LLM Client 抽象 ====================

export interface LLMResponse {
  content: ContentBlock[];
  stopReason: "end_turn" | "tool_use" | "max_tokens" | string;
  usage: {
    inputTokens: number;
    outputTokens: number;
  };
}

/**
 * LLM 调用抽象。
 * 引擎不关心底层是 Anthropic 原生还是 OpenAI 兼容层，
 * 只需要一个能发消息、收回复的接口。
 */
export interface LLMClient {
  /**
   * 发送消息给 LLM，获取回复。
   * @param messages 完整的对话历史
   * @param tools 可用的工具定义
   * @param systemPrompt system prompt
   * @param model 模型标识
   * @param signal 中断信号
   */
  chat(params: {
    messages: AgentMessage[];
    tools: ToolDefinition[];
    systemPrompt: string;
    model?: string;
    signal?: AbortSignal;
  }): Promise<LLMResponse>;
}
