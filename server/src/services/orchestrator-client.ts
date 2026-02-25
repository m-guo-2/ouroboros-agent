/**
 * Orchestrator 客户端
 * Server（业务控制器）通过这个客户端向 Orchestrator（执行引擎）下发指令
 */

import { getCurrentContext, logger } from "./logger";
import { TRACE_HEADER } from "./logger/middleware";

const ORCHESTRATOR_URL = process.env.ORCHESTRATOR_URL || "http://localhost:1996";

/** 构建带 trace 传播的 HTTP Headers */
function buildHeaders(extra?: Record<string, string>): Record<string, string> {
  const ctx = getCurrentContext();
  return {
    "Content-Type": "application/json",
    ...(ctx?.traceId ? { [TRACE_HEADER]: ctx.traceId } : {}),
    ...extra,
  };
}

/**
 * 决策步骤：Agent 决策链中的一个节点（从 orchestrator 传递）
 */
export interface DecisionStep {
  index: number;
  iteration: number;
  timestamp: number;
  phase: "think" | "act" | "observe";
  summary: string;
  reasoning?: string;
  tool?: string;
  toolInput?: unknown;
  toolId?: string;
  toolResult?: string;
  success?: boolean;
  duration?: number;
}

/**
 * Agent 流式事件类型
 * 设计原则：LLM text = 内部推理（reasoning），所有对外输出通过工具调用
 */
export type AgentStreamEvent =
  | { type: "session"; sessionId: string }
  | { type: "reasoning"; text: string }
  | { type: "tool_call"; id: string; tool: string; input: unknown }
  | { type: "tool_result"; id: string; tool: string; result: unknown }
  | { type: "decision_step"; step: DecisionStep }
  | { type: "sdk_log"; message: string; level: "info" | "warn" | "error" }
  | { type: "done"; success: boolean; thinking?: string; usage?: AgentUsage; decisionSteps?: DecisionStep[] }
  | { type: "error"; error: string };

export interface AgentUsage {
  inputTokens: number;
  outputTokens: number;
  totalCostUsd: number;
}

export interface AgentResult {
  success: boolean;
  /** Agent 的内部推理（LLM text），仅用于调试/审计 */
  thinking?: string;
  usage?: AgentUsage;
  error?: string;
}

/**
 * Agent 上下文：传递给 Orchestrator 的 Agent 特定配置
 * 多 Agent 架构下，每个 Agent 有自己的 systemPrompt 和 model
 */
export interface AgentContext {
  /** Agent 的 systemPrompt（完全从配置来） */
  systemPrompt?: string;
  /** Agent 配置的模型 ID */
  modelId?: string;
  /** Agent ID（用于日志追踪） */
  agentId?: string;
}

/**
 * Orchestrator 客户端
 * 用于向执行引擎下发指令
 */
export const orchestratorClient = {
  /**
   * 执行指令（非流式，等待完成）
   * @param instruction 用户消息 + 记忆上下文
   * @param agentContext 可选的 Agent 上下文（systemPrompt, model）
   */
  async execute(instruction: string, agentContext?: AgentContext): Promise<AgentResult> {
    logger.boundary("http_out", `POST ${ORCHESTRATOR_URL}/api/agent/chat`, {
      target: "orchestrator",
      agentId: agentContext?.agentId,
    });

    const response = await fetch(`${ORCHESTRATOR_URL}/api/agent/chat`, {
      method: "POST",
      headers: buildHeaders(),
      body: JSON.stringify({
        message: instruction,
        ...(agentContext?.systemPrompt && { systemPrompt: agentContext.systemPrompt }),
        ...(agentContext?.modelId && { modelId: agentContext.modelId }),
        ...(agentContext?.agentId && { agentId: agentContext.agentId }),
      }),
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Orchestrator error: ${response.status} - ${error}`);
    }

    return response.json();
  },

  /**
   * 执行指令（流式）
   * @param instruction 用户消息 + 记忆上下文
   * @param agentContext 可选的 Agent 上下文（systemPrompt, model）
   */
  async *executeStream(instruction: string, agentContext?: AgentContext): AsyncGenerator<AgentStreamEvent> {
    logger.boundary("http_out", `POST ${ORCHESTRATOR_URL}/api/agent/chat/stream`, {
      target: "orchestrator",
      streaming: true,
      agentId: agentContext?.agentId,
    });

    const response = await fetch(`${ORCHESTRATOR_URL}/api/agent/chat/stream`, {
      method: "POST",
      headers: buildHeaders(),
      body: JSON.stringify({
        message: instruction,
        ...(agentContext?.systemPrompt && { systemPrompt: agentContext.systemPrompt }),
        ...(agentContext?.modelId && { modelId: agentContext.modelId }),
        ...(agentContext?.agentId && { agentId: agentContext.agentId }),
      }),
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Orchestrator error: ${response.status} - ${error}`);
    }

    const reader = response.body?.getReader();
    if (!reader) throw new Error("No response body");

    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (line.startsWith("data: ")) {
          try {
            const data = JSON.parse(line.slice(6));
            yield data as AgentStreamEvent;
          } catch {
            // 忽略解析错误
          }
        }
      }
    }

    // 处理剩余的 buffer
    if (buffer.startsWith("data: ")) {
      try {
        const data = JSON.parse(buffer.slice(6));
        yield data as AgentStreamEvent;
      } catch {
        // 忽略解析错误
      }
    }
  },

  /**
   * 中断执行
   */
  async interrupt(): Promise<void> {
    await fetch(`${ORCHESTRATOR_URL}/api/agent/interrupt`, {
      method: "POST",
    });
  },

  /**
   * 重置会话
   */
  async resetSession(): Promise<void> {
    await fetch(`${ORCHESTRATOR_URL}/api/agent/reset`, {
      method: "POST",
    });
  },

  /**
   * 配置目标模型
   */
  async configureModel(config: {
    baseUrl?: string;
    apiKey: string;
    model?: string;
  }): Promise<void> {
    const response = await fetch(`${ORCHESTRATOR_URL}/api/agent/model`, {
      method: "POST",
      headers: buildHeaders(),
      body: JSON.stringify(config),
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Model config error: ${response.status} - ${error}`);
    }
  },

  /**
   * 请求重启自己（server）
   */
  async restartSelf(): Promise<void> {
    console.log("🔄 Requesting self-restart...");
    await fetch(`${ORCHESTRATOR_URL}/api/process/restart-server`, {
      method: "POST",
    });
  },

  /**
   * 获取进程状态
   */
  async getProcessStatus(): Promise<{
    orchestrator: { running: boolean; pid: number; uptime: number };
    services: Array<{ name: string; status: string; pid?: number }>;
  }> {
    const response = await fetch(`${ORCHESTRATOR_URL}/api/process/status`);
    return response.json();
  },

  /**
   * 健康检查
   */
  async healthCheck(): Promise<boolean> {
    try {
      const response = await fetch(`${ORCHESTRATOR_URL}/api/health`);
      return response.ok;
    } catch {
      return false;
    }
  },
};
