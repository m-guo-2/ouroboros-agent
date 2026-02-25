/**
 * 日志查询路由
 * 提供结构化 JSONL 日志的读取 API，用于 Agent 可观测性
 */
import { Router, Request, Response } from "express";
import { queryByTraceId, queryBySpanId, queryRecent } from "../services/logger/reader";
import type { LogLevel } from "../services/logger/types";

const router = Router();

/**
 * GET /api/logs/trace/:traceId
 * 按 traceId 查询所有相关日志条目（跨所有级别）
 */
router.get("/trace/:traceId", (req: Request, res: Response) => {
  try {
    const { traceId } = req.params;
    if (!traceId) {
      res.status(400).json({ success: false, error: "traceId is required" });
      return;
    }

    const entries = queryByTraceId(traceId);
    res.json({
      success: true,
      data: entries,
      count: entries.length,
    });
  } catch (error) {
    console.error("Failed to query logs by traceId:", error);
    res.status(500).json({ success: false, error: "Failed to query logs" });
  }
});

/**
 * GET /api/logs/span/:spanId
 * 按 spanId 查询所有相关日志条目
 */
router.get("/span/:spanId", (req: Request, res: Response) => {
  try {
    const { spanId } = req.params;
    if (!spanId) {
      res.status(400).json({ success: false, error: "spanId is required" });
      return;
    }

    const entries = queryBySpanId(spanId);
    res.json({
      success: true,
      data: entries,
      count: entries.length,
    });
  } catch (error) {
    console.error("Failed to query logs by spanId:", error);
    res.status(500).json({ success: false, error: "Failed to query logs" });
  }
});

/**
 * GET /api/logs/recent
 * 查询最近日志条目
 * Query params:
 *   - level: boundary | business | detail (可多选，逗号分隔)
 *   - op: 操作类型（可多选，逗号分隔）
 *   - limit: 最大条数（默认 100）
 *   - dateFrom: 起始日期
 *   - dateTo: 结束日期
 */
router.get("/recent", (req: Request, res: Response) => {
  try {
    const {
      level: levelStr,
      op: opStr,
      limit: limitStr,
      dateFrom,
      dateTo,
    } = req.query as Record<string, string | undefined>;

    const level = levelStr
      ? (levelStr.split(",") as LogLevel[])
      : undefined;
    const op = opStr ? opStr.split(",") : undefined;
    const limit = limitStr ? parseInt(limitStr, 10) : 100;

    const entries = queryRecent({
      level: level as LogLevel[] | undefined,
      op,
      limit,
      dateFrom,
      dateTo,
    });

    res.json({
      success: true,
      data: entries,
      count: entries.length,
    });
  } catch (error) {
    console.error("Failed to query recent logs:", error);
    res.status(500).json({ success: false, error: "Failed to query logs" });
  }
});

export default router;
