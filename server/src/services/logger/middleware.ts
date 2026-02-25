/**
 * Express 日志中间件
 * 
 * 自动为每个 HTTP 请求：
 * 1. 生成或沿用 trace ID
 * 2. 设置 AsyncLocalStorage 上下文
 * 3. 记录 boundary 级别的请求/响应日志
 * 
 * 使用：
 * ```typescript
 * app.use(traceMiddleware('server'));
 * ```
 */

import type { Request, Response, NextFunction } from "express";
import { logger } from "./index";
import { logContext, generateTraceId } from "./context";
import type { TraceContext } from "./types";

/** Trace ID 的 HTTP Header 名称 */
export const TRACE_HEADER = "x-trace-id";

/**
 * 创建 trace 中间件
 * 
 * @param serviceName - 当前服务名称（如 'server', 'agent'）
 * @param options - 配置选项
 */
export function traceMiddleware(
  serviceName: string,
  options: {
    /** 不记录日志的路径前缀 */
    ignorePaths?: string[];
  } = {}
) {
  const { ignorePaths = ["/api/health", "/health"] } = options;

  return (req: Request, res: Response, next: NextFunction) => {
    // 跳过健康检查等不需要追踪的路径
    if (ignorePaths.some((p) => req.path.startsWith(p))) {
      return next();
    }

    // 从 Header 获取或生成新 trace ID
    const traceId =
      (req.headers[TRACE_HEADER] as string) || generateTraceId();

    // 将 trace ID 放入响应 Header，便于调试
    res.setHeader(TRACE_HEADER, traceId);

    const context: TraceContext = {
      traceId,
      service: serviceName,
    };

    // 在 trace 上下文中执行后续中间件和路由
    logContext.run(context, () => {
      const startTime = Date.now();

      // 记录请求进入
      logger.boundary("http_in", `${req.method} ${req.path}`, {
        query: Object.keys(req.query).length > 0 ? req.query : undefined,
        ip: req.ip,
        userAgent: req.headers["user-agent"]?.substring(0, 80),
      });

      // 响应结束时记录
      res.on("finish", () => {
        const duration = Date.now() - startTime;
        const status = res.statusCode >= 400 ? "error" : "success";

        logger.boundary(
          "http_done",
          `${req.method} ${req.path} → ${res.statusCode} (${duration}ms)`,
          { statusCode: res.statusCode, duration_ms: duration },
          status
        );
      });

      next();
    });
  };
}
