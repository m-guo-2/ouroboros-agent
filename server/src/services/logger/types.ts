/**
 * 日志系统类型定义
 * 
 * 三层渐进式日志：
 * - boundary: 服务边界（HTTP 出入、跨服务调用）
 * - business: 业务逻辑（LLM 决策、Tool 调用概要、状态变更）
 * - detail:   执行细节（Tool 完整 I/O、LLM 原始数据、错误堆栈）
 */

/** 日志级别 */
export type LogLevel = "boundary" | "business" | "detail";

/** 操作类型 */
export type LogOp =
  // boundary 级别
  | "http_in"           // 收到 HTTP 请求
  | "http_out"          // 发出 HTTP 请求（调用下游服务）
  | "http_done"         // HTTP 请求完成（带状态码和耗时）
  | "service_start"     // 服务启动
  | "service_stop"      // 服务停止
  | "service_restart"   // 服务重启
  // business 级别
  | "llm_call"          // LLM 调用（模型、token 用量）
  | "tool_call"         // Tool 调用概要（工具名、成功/失败）
  | "task_done"         // 任务完成
  | "state_change"      // 状态变更
  | "decision"          // 业务决策
  // detail 级别
  | "tool_input"        // Tool 完整输入
  | "tool_output"       // Tool 完整输出
  | "llm_request"       // LLM 请求详情
  | "llm_response"      // LLM 响应详情
  | "error_stack"       // 错误堆栈
  // 通用
  | string;             // 允许自定义

/** 日志条目状态 */
export type LogStatus = "success" | "error" | "running";

/**
 * 日志条目 - 写入 JSONL 文件的单行 JSON
 * 
 * boundary/business 级别使用 meta 字段存放结构化元数据
 * detail 级别使用 data 字段存放完整数据载荷
 */
export interface LogEntry {
  /** ISO 8601 时间戳 */
  ts: string;
  /** 链路 ID，格式 t-{随机字符串}，跨服务一致 */
  trace: string;
  /** 产生日志的服务名 */
  service: string;
  /** 操作类型 */
  op: LogOp;
  /** 一行摘要，给 LLM 快速扫描 */
  summary: string;
  /** 操作状态（仅完结类日志需要） */
  status?: LogStatus;
  /** 操作段 ID，关联同一操作的多条 detail 日志 */
  span?: string;
  /** 结构化元数据（boundary/business） */
  meta?: Record<string, unknown>;
  /** 完整数据载荷（detail） */
  data?: Record<string, unknown>;
  /** 错误信息 */
  error?: string;
}

/**
 * Trace 上下文 - 通过 AsyncLocalStorage 在异步链中传播
 */
export interface TraceContext {
  /** 链路 ID */
  traceId: string;
  /** 当前操作段 ID */
  spanId?: string;
  /** 当前服务名 */
  service: string;
}

/**
 * Span 句柄 - 用于关联同一操作的多条日志
 */
export interface SpanHandle {
  /** span ID */
  id: string;
  /** 关联的 trace ID */
  traceId: string;
}
