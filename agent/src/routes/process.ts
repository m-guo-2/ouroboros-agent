/**
 * 消息处理路由
 *
 * POST /process — 异步处理（IM 渠道场景，立即返回 202）
 *
 * 仅支持 IM 渠道（飞书/企微等），不提供 SSE 流式接口。
 * Agent 后台执行，是否回复及回复内容由 skills 工具决定。
 */

import { Router, type Request, type Response } from "express";
import { enqueueProcessRequest, type ProcessRequest } from "../engine/runner";

const router = Router();

/**
 * POST /process
 *
 * 异步处理消息：
 * 1. Server dispatcher 发来请求
 * 2. 立即返回 202（不阻塞 dispatcher）
 * 3. 后台将 event 追加到 session worker 队列
 * 4. session worker 串行执行（运行中 session 会继续追加）
 */
router.post("/process", (req: Request, res: Response) => {
  const request = req.body as ProcessRequest;

  // 基本校验
  if (!request.userId || !request.agentId || !request.content) {
    return res.status(400).json({
      success: false,
      error: "Missing required fields: userId, agentId, content",
    });
  }

  if (!request.channel || !request.channelUserId) {
    return res.status(400).json({
      success: false,
      error: "Missing required fields: channel, channelUserId",
    });
  }

  // 立即返回 202
  res.status(202).json({ success: true, message: "Processing" });

  // 后台异步执行
  (async () => {
    try {
      await enqueueProcessRequest(request);
    } catch (err) {
      console.error(`[process] Unhandled error:`, err);
    }
  })();
});

export default router;
