/**
 * 用户管理路由
 * 用户 CRUD、渠道绑定、记忆查询
 */

import { Router, Request, Response } from "express";
import { userDb, userChannelDb, agentSessionDb } from "../services/database";
import {
  getUserWithChannels,
  generateBindingCode,
  redeemBindingCode,
  unbindChannel,
} from "../services/user-resolver";
import { getUserMemory } from "../services/memory-manager";
import type { ChannelType } from "../services/channel-types";

const router = Router();

// ==================== 用户 CRUD ====================

/**
 * GET /api/users
 * 获取所有用户列表
 * 支持按渠道查询: ?channelType=webui&channelUserId=xxx
 */
router.get("/", (req: Request, res: Response) => {
  const { channelType, channelUserId } = req.query;

  // 如果提供了渠道过滤参数，通过渠道查找用户
  if (channelType && channelUserId) {
    const binding = userChannelDb.findByChannelUser(
      channelType as string,
      channelUserId as string
    );
    if (binding) {
      const user = getUserWithChannels(binding.userId);
      return res.json({ success: true, data: user ? [user] : [] });
    }
    return res.json({ success: true, data: [] });
  }

  const users = userDb.getAll();
  res.json({ success: true, data: users });
});

/**
 * GET /api/users/:id
 * 获取用户详情（含渠道绑定信息）
 */
router.get("/:id", (req: Request, res: Response) => {
  const user = getUserWithChannels(req.params.id);
  if (!user) {
    return res.status(404).json({ success: false, error: "User not found" });
  }
  res.json({ success: true, data: user });
});

/**
 * PATCH /api/users/:id
 * 更新用户信息
 */
router.patch("/:id", (req: Request, res: Response) => {
  const { name, avatarUrl, metadata } = req.body;
  const updated = userDb.update(req.params.id, { name, avatarUrl, metadata });
  if (!updated) {
    return res.status(404).json({ success: false, error: "User not found" });
  }
  res.json({ success: true, data: updated });
});

/**
 * DELETE /api/users/:id
 * 删除用户
 */
router.delete("/:id", (req: Request, res: Response) => {
  const deleted = userDb.delete(req.params.id);
  if (!deleted) {
    return res.status(404).json({ success: false, error: "User not found" });
  }
  res.json({ success: true });
});

// ==================== 用户会话 ====================

/**
 * GET /api/users/:id/sessions
 * 获取用户的所有会话（跨渠道）
 */
router.get("/:id/sessions", (req: Request, res: Response) => {
  const user = userDb.getById(req.params.id);
  if (!user) {
    return res.status(404).json({ success: false, error: "User not found" });
  }
  const sessions = agentSessionDb.getByUserId(req.params.id);
  res.json({ success: true, data: sessions });
});

// ==================== 用户记忆 ====================

/**
 * GET /api/users/:id/memory
 * 获取用户的记忆摘要和事实
 */
router.get("/:id/memory", (req: Request, res: Response) => {
  const user = userDb.getById(req.params.id);
  if (!user) {
    return res.status(404).json({ success: false, error: "User not found" });
  }
  const memory = getUserMemory(req.params.id);
  res.json({ success: true, data: memory });
});

// ==================== 跨渠道绑定 ====================

/**
 * POST /api/users/:id/binding-code
 * 生成绑定码（用于在另一个渠道中绑定此用户）
 */
router.post("/:id/binding-code", (req: Request, res: Response) => {
  const { targetChannel } = req.body as { targetChannel: ChannelType };

  if (!targetChannel) {
    return res.status(400).json({ success: false, error: "targetChannel is required" });
  }

  const validChannels = ["feishu", "qiwei", "webui"];
  if (!validChannels.includes(targetChannel)) {
    return res.status(400).json({
      success: false,
      error: `Invalid channel: ${targetChannel}`,
    });
  }

  const user = userDb.getById(req.params.id);
  if (!user) {
    return res.status(404).json({ success: false, error: "User not found" });
  }

  const code = generateBindingCode(req.params.id, targetChannel);

  res.json({
    success: true,
    data: {
      code,
      expiresIn: "5 minutes",
      targetChannel,
      instruction: `请在 ${targetChannel} 渠道中发送 "绑定 ${code}" 来完成绑定。`,
    },
  });
});

/**
 * POST /api/users/bind
 * 使用绑定码绑定渠道账号
 */
router.post("/bind", (req: Request, res: Response) => {
  const { code, channelType, channelUserId, displayName } = req.body;

  if (!code || !channelType || !channelUserId) {
    return res.status(400).json({
      success: false,
      error: "code, channelType, channelUserId are required",
    });
  }

  const result = redeemBindingCode(code, channelType, channelUserId, displayName);

  if (!result.success) {
    return res.status(400).json({ success: false, error: result.error });
  }

  res.json({
    success: true,
    data: {
      userId: result.userId,
      message: "绑定成功",
    },
  });
});

/**
 * DELETE /api/users/:id/channels/:channelId
 * 解绑渠道账号
 */
router.delete("/:id/channels/:channelId", (req: Request, res: Response) => {
  const success = unbindChannel(req.params.id, req.params.channelId);
  if (!success) {
    return res.status(400).json({
      success: false,
      error: "无法解绑。可能是绑定不存在，或这是用户唯一的渠道绑定。",
    });
  }
  res.json({ success: true });
});

export default router;
