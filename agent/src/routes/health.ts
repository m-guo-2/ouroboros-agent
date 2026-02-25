/**
 * 健康检查路由
 * GET /health — 就绪探针（滚动更新时 Server 用来判断 Agent 是否 ready）
 */

import { Router, type Request, type Response } from "express";

const router = Router();

/** 当前是否正在 draining */
let isDraining = false;

/** 正在处理中的请求数 */
let inflightCount = 0;

/**
 * GET /health
 * 就绪探针
 */
router.get("/health", (_req: Request, res: Response) => {
  res.json({
    status: isDraining ? "draining" : "ready",
    inflight: inflightCount,
    version: process.env.AGENT_APP_VERSION || "unknown",
    uptime: process.uptime(),
  });
});

/**
 * POST /drain
 * 开始优雅退出：不再接新请求，等 inflight 完成后退出
 */
router.post("/drain", (_req: Request, res: Response) => {
  isDraining = true;
  console.log("[lifecycle] Drain started, no longer accepting new requests");

  res.json({ success: true, inflight: inflightCount });

  // 如果没有 inflight 请求，立即退出
  if (inflightCount === 0) {
    console.log("[lifecycle] No inflight requests, exiting...");
    setTimeout(() => process.exit(0), 1000);
  }
});

// 导出控制函数供其他模块使用
export function isDrainingState(): boolean {
  return isDraining;
}

export function incrementInflight(): void {
  inflightCount++;
}

export function decrementInflight(): void {
  inflightCount--;
  // draining 状态下，inflight 归零时退出
  if (isDraining && inflightCount <= 0) {
    console.log("[lifecycle] All inflight requests completed, exiting...");
    setTimeout(() => process.exit(0), 1000);
  }
}

export default router;
