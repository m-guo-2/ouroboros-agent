/**
 * 日志上下文传播
 * 
 * 使用 AsyncLocalStorage 在异步调用链中传播 trace/span 上下文，
 * 业务代码写日志时无需手动传递 traceId。
 */

import { AsyncLocalStorage } from "node:async_hooks";
import type { TraceContext } from "./types";

/** 全局日志上下文存储 */
export const logContext = new AsyncLocalStorage<TraceContext>();

/** 生成短 ID（用于 trace 和 span） */
export function generateId(prefix: string = ""): string {
  // 使用 crypto.randomUUID 取前 10 位，足够唯一且简短
  const id = crypto.randomUUID().replace(/-/g, "").substring(0, 10);
  return prefix ? `${prefix}-${id}` : id;
}

/** 生成 trace ID */
export function generateTraceId(): string {
  return generateId("t");
}

/** 生成 span ID */
export function generateSpanId(): string {
  return generateId("s");
}

/** 获取当前 trace 上下文 */
export function getCurrentContext(): TraceContext | undefined {
  return logContext.getStore();
}

/**
 * 在指定 trace 上下文中运行代码
 * 闭包内所有日志自动带上该 trace 上下文
 */
export function runWithTrace<T>(
  context: TraceContext,
  fn: () => T
): T {
  return logContext.run(context, fn);
}

/**
 * 在指定 trace 上下文中运行异步代码
 */
export function runWithTraceAsync<T>(
  context: TraceContext,
  fn: () => Promise<T>
): Promise<T> {
  return logContext.run(context, fn);
}
