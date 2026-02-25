/**
 * Agent Profile 路由 - 管理 Agent 的 CRUD
 * 多 Agent 架构 Phase 1：Agent 配置管理
 *
 * Agent 在系统中是 "一等公民"，和人类共享 users 表（type='agent'）。
 * agent_configs 表存储 Agent 的 systemPrompt、技能、渠道绑定等。
 */

import { Router, Request, Response } from "express";
import { agentConfigDb, userDb } from "../services/database";
import type { AgentConfigRecord } from "../services/database";
import { logger } from "../services/logger";

const router = Router();

/**
 * GET /api/agents
 * 获取所有 Agent 列表
 */
router.get("/", (_req: Request, res: Response) => {
  try {
    const agents = agentConfigDb.getAll();

    // 附带 user 信息
    const result = agents.map(agent => {
      const user = userDb.getById(agent.userId);
      return {
        ...agent,
        user: user ? { id: user.id, name: user.name, avatarUrl: user.avatarUrl } : null,
      };
    });

    res.json({ success: true, data: result });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * GET /api/agents/active
 * 获取所有活跃的 Agent
 */
router.get("/active", (_req: Request, res: Response) => {
  try {
    const agents = agentConfigDb.getActive();
    res.json({ success: true, data: agents });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * GET /api/agents/:id
 * 获取单个 Agent 详情
 */
router.get("/:id", (req: Request, res: Response) => {
  try {
    const agent = agentConfigDb.getById(req.params.id);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    const user = userDb.getById(agent.userId);
    res.json({
      success: true,
      data: {
        ...agent,
        user: user ? { id: user.id, name: user.name, avatarUrl: user.avatarUrl } : null,
      },
    });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * POST /api/agents
 * 创建新 Agent
 * Body: { displayName, systemPrompt?, modelId?, skills?, channels? }
 */
router.post("/", (req: Request, res: Response) => {
  try {
    const { displayName, systemPrompt, modelId, provider, model, skills, channels, avatarUrl } = req.body;

    if (!displayName) {
      return res.status(400).json({ success: false, error: "displayName is required" });
    }

    // 1. 创建 Agent 的 user 身份
    const userId = crypto.randomUUID();
    userDb.create({
      id: userId,
      name: displayName,
      type: "agent",
      avatarUrl: avatarUrl || undefined,
      metadata: {},
    });

    // 2. 创建 Agent 配置
    const configId = crypto.randomUUID();
    const agent = agentConfigDb.create({
      id: configId,
      userId,
      displayName,
      systemPrompt: systemPrompt || "",
      modelId: modelId || undefined,
      provider: provider || undefined,
      model: model || undefined,
      skills: skills || [],
      channels: channels || [],
      isActive: true,
    });

    logger.info(`Agent created: ${displayName}`, { agentId: configId, userId });

    const user = userDb.getById(userId);
    res.status(201).json({
      success: true,
      data: {
        ...agent,
        user: user ? { id: user.id, name: user.name, avatarUrl: user.avatarUrl } : null,
      },
    });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    logger.error("agent_profiles", `Failed to create agent: ${msg}`);
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * PUT /api/agents/:id
 * 更新 Agent 配置
 */
router.put("/:id", (req: Request, res: Response) => {
  try {
    const agent = agentConfigDb.getById(req.params.id);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    const { displayName, systemPrompt, modelId, provider, model, skills, channels, isActive, avatarUrl } = req.body;

    // 更新 Agent 配置
    const updates: Partial<AgentConfigRecord> = {};
    if (displayName !== undefined) updates.displayName = displayName;
    if (systemPrompt !== undefined) updates.systemPrompt = systemPrompt;
    if (modelId !== undefined) updates.modelId = modelId;
    if (provider !== undefined) updates.provider = provider;
    if (model !== undefined) updates.model = model;
    if (skills !== undefined) updates.skills = skills;
    if (channels !== undefined) updates.channels = channels;
    if (isActive !== undefined) updates.isActive = isActive;

    const updated = agentConfigDb.update(req.params.id, updates);

    // 同步更新 user 名称和头像
    if (displayName !== undefined || avatarUrl !== undefined) {
      const userUpdates: Record<string, unknown> = {};
      if (displayName !== undefined) userUpdates.name = displayName;
      if (avatarUrl !== undefined) userUpdates.avatarUrl = avatarUrl;
      userDb.update(agent.userId, userUpdates as any);
    }

    logger.info(`Agent updated: ${agent.displayName}`, { agentId: req.params.id });

    const user = userDb.getById(agent.userId);
    res.json({
      success: true,
      data: {
        ...updated,
        user: user ? { id: user.id, name: user.name, avatarUrl: user.avatarUrl } : null,
      },
    });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * DELETE /api/agents/:id
 * 删除 Agent
 */
router.delete("/:id", (req: Request, res: Response) => {
  try {
    const agent = agentConfigDb.getById(req.params.id);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    // 检查是否是默认 Agent
    if (req.params.id === "default-agent-config") {
      return res.status(400).json({ success: false, error: "Cannot delete default agent" });
    }

    const deleted = agentConfigDb.delete(req.params.id);
    if (!deleted) {
      return res.status(500).json({ success: false, error: "Failed to delete agent" });
    }

    logger.info(`Agent deleted: ${agent.displayName}`, { agentId: req.params.id });
    res.json({ success: true });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * GET /api/agents/by-channel/:channelType
 * 按渠道查找绑定的 Agent
 */
router.get("/by-channel/:channelType", (req: Request, res: Response) => {
  try {
    const { channelType } = req.params;
    const { identifier } = req.query;

    const agents = agentConfigDb.getByChannel(channelType, identifier as string | undefined);
    res.json({ success: true, data: agents });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

export default router;
