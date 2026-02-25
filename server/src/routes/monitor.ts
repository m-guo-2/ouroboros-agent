/**
 * Monitor 路由
 * 提供 Agent 可观测性的 SSE 实时流端点
 */
import { Router, Request, Response } from "express";
import { observationBus, type ObservationFilter } from "../services/observation-bus";

const router = Router();

/**
 * GET /api/monitor/stream
 * SSE 实时观测流
 * 
 * Query params:
 *   - agentId: 过滤特定 Agent
 *   - channel: 过滤特定渠道
 *   - sessionId: 过滤特定会话
 */
router.get("/stream", (req: Request, res: Response) => {
  const { agentId, channel, sessionId } = req.query as Record<string, string | undefined>;

  // 禁用响应压缩
  req.headers["accept-encoding"] = "identity";

  // SSE headers
  res.writeHead(200, {
    "Content-Type": "text/event-stream",
    "Cache-Control": "no-cache, no-transform",
    "Connection": "keep-alive",
    "X-Accel-Buffering": "no",
  });
  res.flushHeaders();

  /** 安全写入 + flush */
  const sseWrite = (data: string) => {
    try {
      res.write(data);
      if (typeof (res as any).flush === "function") {
        (res as any).flush();
      }
    } catch {
      // 连接可能已关闭
    }
  };

  // 发送连接确认
  sseWrite(`data: ${JSON.stringify({ type: "connected", timestamp: Date.now(), filters: { agentId, channel, sessionId } })}\n\n`);

  // 构建过滤条件
  const filter: ObservationFilter = {};
  if (agentId) filter.agentId = agentId;
  if (channel) filter.channel = channel;
  if (sessionId) filter.sessionId = sessionId;

  // 订阅观测事件
  const unsubscribe = observationBus.subscribe((event) => {
    sseWrite(`data: ${JSON.stringify(event)}\n\n`);
  }, Object.keys(filter).length > 0 ? filter : undefined);

  // 心跳保活
  const heartbeat = setInterval(() => {
    try {
      res.write(`: heartbeat\n\n`);
      if (typeof (res as any).flush === "function") {
        (res as any).flush();
      }
    } catch {
      cleanup();
    }
  }, 15000);

  const cleanup = () => {
    unsubscribe();
    clearInterval(heartbeat);
  };

  req.on("close", cleanup);
  req.on("error", cleanup);
});

/**
 * GET /api/monitor/status
 * 获取 Monitor 连接状态
 */
router.get("/status", (_req: Request, res: Response) => {
  res.json({
    success: true,
    data: {
      activeListeners: observationBus.listenerCount(),
      timestamp: Date.now(),
    },
  });
});

export default router;
