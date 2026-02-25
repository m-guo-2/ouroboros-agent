/**
 * 消息链路追踪 (Message Trace)
 *
 * 设计原则：trace 的起点 = 消息的发起方，而不是 HTTP 请求。
 *
 * 一条 trace 覆盖一次完整的消息交互回合 (Message Round)：
 *   - 用户发起：用户消息 → Agent 思考 → 工具调用 → Agent 回复
 *   - Agent 发起：Agent 决定行动 → 思考 → 工具调用 → 发出消息
 *
 * 为什么不直接用 HTTP 中间件的 traceId？
 *   1. HTTP traceId 的边界是 "一次 HTTP 请求"，不是 "一次消息回合"
 *   2. 异步场景（POST /incoming 立即返回 202，后台处理）中，
 *      HTTP 请求的生命周期 < 消息处理的生命周期
 *   3. Agent 主动发起的消息没有入站 HTTP 请求
 *
 * 因此这里提供显式的 startMessageTrace()，在消息处理开始时创建独立的 trace，
 * 确保 traceId 的生命周期 = 消息处理的生命周期。
 */

import { generateTraceId, generateSpanId, runWithTrace, getCurrentContext } from "./logger/context";
import { logger } from "./logger";
import type { SpanHandle } from "./logger/types";

/** 消息发起方类型 */
export type MessageInitiator = "user" | "agent" | "system";

/** 消息链路上下文 */
export interface MessageTraceContext {
  /** 链路 ID：整个消息回合共享 */
  traceId: string;
  /** 操作段 ID：关联同一回合内的日志 */
  span: SpanHandle;
  /** 消息发起方 */
  initiator: MessageInitiator;
  /** 来源描述（如渠道名、Agent ID、定时任务名） */
  source: string;
}

/**
 * 开始一条消息链路
 *
 * 优先沿用当前 AsyncLocalStorage 中的 traceId（来自 HTTP 中间件），
 * 如果没有则生成新的。这样：
 *   - 用户通过 HTTP 发消息 → 沿用 HTTP traceId，HTTP 日志和消息日志关联
 *   - Agent 内部主动触发 → 生成新 traceId，不依赖 HTTP 上下文
 *
 * @param initiator 发起方：user | agent | system
 * @param source 来源描述（如 "feishu", "agent:default-agent-config", "cron:daily-report"）
 */
export function startMessageTrace(
  initiator: MessageInitiator,
  source: string
): MessageTraceContext {
  // 优先沿用已有的 HTTP trace（保持关联），否则生成新的
  const existingTraceId = getCurrentContext()?.traceId;
  const traceId = existingTraceId || generateTraceId();
  const span = logger.span();

  logger.business("decision", `Message trace started [${initiator}] from ${source}`, {
    initiator,
    source,
    traceId,
    inheritedFromHttp: !!existingTraceId,
  }, span);

  return { traceId, span, initiator, source };
}

/**
 * 在消息链路上下文中执行异步代码
 *
 * 确保闭包内所有日志自动带上该 trace 上下文。
 * 用于 Agent 主动发起的消息（没有 HTTP 上下文时）。
 */
export async function runInMessageTrace<T>(
  initiator: MessageInitiator,
  source: string,
  fn: (ctx: MessageTraceContext) => Promise<T>
): Promise<T> {
  const traceId = generateTraceId();
  const span: SpanHandle = { id: generateSpanId(), traceId };

  return runWithTrace(
    { traceId, service: "server" },
    async () => {
      logger.business("decision", `Message trace started [${initiator}] from ${source}`, {
        initiator,
        source,
        traceId,
      }, span);

      return fn({ traceId, span, initiator, source });
    }
  ) as T;
}
