/**
 * 执行链路追踪 API
 *
 * POST /api/traces/events           — Agent 上报执行事件（实时推送 + 持久化）
 * GET  /api/traces/active           — 查询当前活跃（running）的链路（含步骤摘要）
 * GET  /api/traces/recent-summaries — 查询最近的链路（含步骤摘要）
 * GET  /api/traces/:id              — 查询完整链路（含所有步骤）
 * GET  /api/traces                  — 查询链路列表（按 session 或最近）
 */

import { Router, type Request, type Response } from "express";
import {
  handleTraceEvent,
  getTrace,
  getTracesBySessionId,
  getRecentTracesList,
  getActiveTraces,
  getRecentTraceSummaries,
  type TraceEvent,
} from "../services/execution-trace";

const router = Router();

/**
 * POST /api/traces/events
 *
 * Agent 上报执行事件。支持单条和批量。
 * Agent 在 processMessage() 每一步都 fire-and-forget POST 到此端点。
 *
 * Body: TraceEvent | TraceEvent[]
 */
router.post("/events", (req: Request, res: Response) => {
  const body = req.body;

  try {
    if (Array.isArray(body)) {
      for (const event of body) {
        handleTraceEvent(event as TraceEvent);
      }
    } else {
      handleTraceEvent(body as TraceEvent);
    }

    res.json({ success: true });
  } catch (err) {
    const error = err instanceof Error ? err.message : "Unknown error";
    console.error("[traces] Failed to handle event:", error);
    res.status(500).json({ success: false, error });
  }
});

/**
 * GET /api/traces/active
 *
 * 查询当前所有 running 状态的链路（含步骤摘要统计）
 * 用于全局 Trace 列表展示实时执行中的任务
 */
router.get("/active", (_req: Request, res: Response) => {
  try {
    const traces = getActiveTraces();
    res.json({ success: true, data: traces });
  } catch (err) {
    console.error("[traces] Failed to get active traces:", err);
    res.json({ success: true, data: [] });
  }
});

/**
 * GET /api/traces/recent-summaries
 *
 * 查询最近的链路（含步骤摘要统计：thinking/tool_call 次数、工具名列表等）
 * 用于全局 Trace 列表展示历史执行记录
 *
 * Query params:
 *   - limit: 最大返回数（默认 30）
 */
router.get("/recent-summaries", (req: Request, res: Response) => {
  try {
    const limit = parseInt(req.query.limit as string) || 30;
    const traces = getRecentTraceSummaries(limit);
    res.json({ success: true, data: traces });
  } catch (err) {
    console.error("[traces] Failed to get recent summaries:", err);
    res.json({ success: true, data: [] });
  }
});

/**
 * GET /api/traces/:id
 *
 * 查询完整链路（含所有步骤）
 * 用于 MessageTraceDetail 展示历史执行详情
 */
router.get("/:id", (req: Request, res: Response) => {
  try {
    const trace = getTrace(req.params.id);
    if (!trace) {
      return res.status(404).json({ success: false, error: "Trace not found" });
    }
    res.json({ success: true, data: trace });
  } catch (err) {
    console.error("[traces] Failed to get trace:", err);
    res.status(500).json({ success: false, error: "Failed to get trace" });
  }
});

/**
 * GET /api/traces
 *
 * 查询链路列表
 * Query params:
 *   - sessionId: 按 session 过滤
 *   - limit: 最大返回数（默认 50）
 */
router.get("/", (req: Request, res: Response) => {
  try {
    const { sessionId, limit } = req.query;

    if (sessionId) {
      const traces = getTracesBySessionId(sessionId as string);
      return res.json({ success: true, data: traces });
    }

    const traces = getRecentTracesList(parseInt(limit as string) || 50);
    res.json({ success: true, data: traces });
  } catch (err) {
    console.error("[traces] Failed to get traces:", err);
    res.json({ success: true, data: [] });
  }
});

export default router;
