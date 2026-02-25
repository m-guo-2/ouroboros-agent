/**
 * Agent 生命周期管理路由
 * Agent App 实例的注册、心跳、查看
 */

import { Router, type Request, type Response } from "express";
import { agentRegistry } from "../services/agent-registry";

const router = Router();

/**
 * POST /api/lifecycle/register
 * Agent 实例注册（启动时调用）
 */
router.post("/register", (req: Request, res: Response) => {
  const { id, url, version } = req.body;
  if (!id || !url) {
    return res.status(400).json({ success: false, error: "id and url are required" });
  }
  const endpoint = agentRegistry.register({ id, url, version });
  res.json({ success: true, data: endpoint });
});

/**
 * POST /api/lifecycle/heartbeat
 * Agent 心跳（定期调用）
 */
router.post("/heartbeat", (req: Request, res: Response) => {
  const { id } = req.body;
  if (!id) {
    return res.status(400).json({ success: false, error: "id is required" });
  }
  agentRegistry.heartbeat(id);
  res.json({ success: true });
});

/**
 * POST /api/lifecycle/drain/:id
 * 标记 Agent 实例为 draining（不再接新请求）
 */
router.post("/drain/:id", (req: Request, res: Response) => {
  agentRegistry.markDraining(req.params.id);
  res.json({ success: true });
});

/**
 * DELETE /api/lifecycle/agents/:id
 * 移除 Agent 注册
 */
router.delete("/agents/:id", (req: Request, res: Response) => {
  agentRegistry.unregister(req.params.id);
  res.json({ success: true });
});

/**
 * GET /api/lifecycle/agents
 * 查看所有注册的 Agent 实例
 */
router.get("/agents", (_req: Request, res: Response) => {
  const endpoints = agentRegistry.getAll();
  res.json({ success: true, data: endpoints });
});

export default router;
