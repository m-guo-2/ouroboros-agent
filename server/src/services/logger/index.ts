/**
 * Logger 主模块
 * 
 * 使用方式：
 * 
 * ```typescript
 * import { logger } from './services/logger';
 * 
 * // 基本日志（自动从 AsyncLocalStorage 获取 trace 上下文）
 * logger.boundary('http_in', 'POST /api/agent/chat/stream ← user');
 * logger.business('llm_call', 'Claude: 分析任务', { model: 'claude-4-sonnet' });
 * logger.detail('tool_input', 'edit_file input', { path: '...', content: '...' });
 * 
 * // Span（关联同一操作的多条日志）
 * const span = logger.span();
 * logger.business('llm_call', '开始调用 Claude', {}, span);
 * logger.detail('llm_response', '响应内容', { tokens: 800 }, span);
 * 
 * // 在指定 trace 上下文中运行
 * logger.withTrace('t-abc123', () => {
 *   logger.boundary('http_in', '来自 server 的请求');
 * });
 * ```
 */

import { resolve } from "node:path";
import { LogWriter } from "./writer";
import {
  logContext,
  generateTraceId,
  generateSpanId,
  getCurrentContext,
  runWithTrace,
} from "./context";
import type {
  LogLevel,
  LogOp,
  LogStatus,
  LogEntry,
  TraceContext,
  SpanHandle,
} from "./types";

// 重新导出类型和工具函数
export type { LogLevel, LogOp, LogStatus, LogEntry, TraceContext, SpanHandle };
export {
  logContext,
  generateTraceId,
  generateSpanId,
  getCurrentContext,
  runWithTrace,
};

class Logger {
  private writer: LogWriter;
  private serviceName: string;

  constructor(serviceName: string, logDir: string) {
    this.serviceName = serviceName;
    this.writer = new LogWriter(logDir);
  }

  // ==================== 按级别写日志 ====================

  /** L1 服务边界日志 */
  boundary(
    op: LogOp,
    summary: string,
    meta?: Record<string, unknown>,
    spanOrStatus?: SpanHandle | LogStatus
  ): void {
    this.log("boundary", op, summary, { meta, spanOrStatus });
  }

  /** L2 业务逻辑日志 */
  business(
    op: LogOp,
    summary: string,
    meta?: Record<string, unknown>,
    spanOrStatus?: SpanHandle | LogStatus
  ): void {
    this.log("business", op, summary, { meta, spanOrStatus });
  }

  /** L3 执行细节日志 */
  detail(
    op: LogOp,
    summary: string,
    data?: Record<string, unknown>,
    spanOrStatus?: SpanHandle | LogStatus
  ): void {
    this.log("detail", op, summary, { data, spanOrStatus });
  }

  // ==================== Span 管理 ====================

  /** 创建一个新 span，用于关联同一操作的多条日志 */
  span(): SpanHandle {
    const ctx = getCurrentContext();
    return {
      id: generateSpanId(),
      traceId: ctx?.traceId || generateTraceId(),
    };
  }

  // ==================== Trace 上下文 ====================

  /**
   * 在指定 trace 上下文中运行代码
   * 闭包内所有日志自动带上该 trace
   */
  withTrace<T>(traceId: string, fn: () => T): T {
    return runWithTrace(
      { traceId, service: this.serviceName },
      fn
    );
  }

  /**
   * 在新 trace 上下文中运行代码
   * 自动生成新的 trace ID
   */
  withNewTrace<T>(fn: () => T): { traceId: string; result: T } {
    const traceId = generateTraceId();
    const result = runWithTrace(
      { traceId, service: this.serviceName },
      fn
    );
    return { traceId, result };
  }

  // ==================== 便捷方法（兼容传统 logger 调用） ====================

  /** 通用信息日志 → business 级别 */
  info(summary: string, meta?: Record<string, unknown>, spanOrStatus?: SpanHandle | LogStatus): void {
    this.business("state_change", summary, meta, spanOrStatus);
  }

  /** 警告日志 → business 级别 */
  warn(summary: string, meta?: Record<string, unknown>, spanOrStatus?: SpanHandle | LogStatus): void {
    this.business("decision", summary, meta, spanOrStatus);
  }

  /** 调试日志 → detail 级别 */
  debug(summary: string, data?: Record<string, unknown>, spanOrStatus?: SpanHandle | LogStatus): void {
    this.detail("tool_output", summary, data, spanOrStatus);
  }

  /** 记录错误（自动写入 business + detail） */
  error(op: LogOp, summary: string, error?: Error | string, span?: SpanHandle): void {
    const errorMsg = error instanceof Error ? error.message : error;
    const errorStack = error instanceof Error ? error.stack : undefined;

    this.business(op, summary, { error: errorMsg }, span);

    if (errorStack) {
      this.detail("error_stack", `${op} 错误堆栈`, {
        message: errorMsg,
        stack: errorStack,
      }, span);
    }
  }

  /** 获取当前 trace ID（如果有） */
  currentTraceId(): string | undefined {
    return getCurrentContext()?.traceId;
  }

  /** 立即 flush 所有缓冲日志到文件 */
  flush(): void {
    this.writer.flushAll();
  }

  /** 清理过期日志文件 */
  async cleanup(): Promise<{ deleted: string[]; errors: string[] }> {
    return this.writer.cleanup();
  }

  // ==================== 内部实现 ====================

  private log(
    level: LogLevel,
    op: LogOp,
    summary: string,
    options: {
      meta?: Record<string, unknown>;
      data?: Record<string, unknown>;
      spanOrStatus?: SpanHandle | LogStatus;
    } = {}
  ): void {
    const ctx = getCurrentContext();
    const { meta, data, spanOrStatus } = options;

    // 解析 spanOrStatus 参数
    let span: string | undefined;
    let status: LogStatus | undefined;
    if (typeof spanOrStatus === "string") {
      status = spanOrStatus as LogStatus;
    } else if (spanOrStatus && "id" in spanOrStatus) {
      span = spanOrStatus.id;
    }

    const entry: LogEntry = {
      ts: new Date().toISOString(),
      trace: ctx?.traceId || "no-trace",
      service: ctx?.service || this.serviceName,
      op,
      summary,
    };

    // 只添加非空字段，保持 JSONL 紧凑
    if (status) entry.status = status;
    if (span) entry.span = span;
    if (meta && Object.keys(meta).length > 0) entry.meta = meta;
    if (data && Object.keys(data).length > 0) entry.data = data;

    this.writer.write(level, entry);
  }
}

// ==================== 单例导出 ====================

/** 默认日志目录 */
const DEFAULT_LOG_DIR = resolve(import.meta.dir, "../../../../data/logs");

/** server 的 logger 单例 */
export const logger = new Logger("server", DEFAULT_LOG_DIR);

/**
 * 创建自定义 Logger 实例
 * 用于 orchestrator 等其他服务
 */
export function createLogger(serviceName: string, logDir: string): Logger {
  return new Logger(serviceName, logDir);
}
