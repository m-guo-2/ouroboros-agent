/**
 * Agent Workspace 路由 - 管理 Agent 的工作空间
 * 多 Agent 架构 Phase 5：笔记、任务、产出物
 *
 * 每个 Agent 有自己的工作空间：
 * - Notes（工作笔记）：短期工作记忆、观察、决策记录
 * - Tasks（任务记录）：Agent 认领/被分配的任务，及其状态流转
 * - Artifacts（产出物）：Agent 生成的文件、文档、代码等
 */

import { Router, Request, Response } from "express";
import { agentNoteDb, agentTaskDb, agentArtifactDb, agentConfigDb } from "../services/database";
import { logger } from "../services/logger";

const router = Router();

// ==================== Notes ====================

/**
 * GET /api/agents/:agentId/notes
 * 获取 Agent 的工作笔记
 */
router.get("/:agentId/notes", (req: Request, res: Response) => {
  try {
    const { agentId } = req.params;
    const { category } = req.query;

    const agent = agentConfigDb.getById(agentId);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    const notes = agentNoteDb.getByAgentId(agentId, category as string | undefined);
    res.json({ success: true, data: notes });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * POST /api/agents/:agentId/notes
 * 添加工作笔记
 */
router.post("/:agentId/notes", (req: Request, res: Response) => {
  try {
    const { agentId } = req.params;
    const { category, content, relatedSessionId, relatedUserId } = req.body;

    if (!content) {
      return res.status(400).json({ success: false, error: "content is required" });
    }

    const agent = agentConfigDb.getById(agentId);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    const note = agentNoteDb.create({
      id: crypto.randomUUID(),
      agentId,
      category: category || "general",
      content,
      relatedSessionId,
      relatedUserId,
    });

    logger.info(`Agent note created for [${agent.displayName}]`, { agentId, noteId: note.id, category: note.category });
    res.status(201).json({ success: true, data: note });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * PUT /api/agents/:agentId/notes/:noteId
 * 更新工作笔记
 */
router.put("/:agentId/notes/:noteId", (req: Request, res: Response) => {
  try {
    const { content, category } = req.body;
    const updated = agentNoteDb.update(req.params.noteId, { content, category });

    if (!updated) {
      return res.status(404).json({ success: false, error: "Note not found" });
    }

    res.json({ success: true, data: updated });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * DELETE /api/agents/:agentId/notes/:noteId
 */
router.delete("/:agentId/notes/:noteId", (req: Request, res: Response) => {
  try {
    const deleted = agentNoteDb.delete(req.params.noteId);
    if (!deleted) {
      return res.status(404).json({ success: false, error: "Note not found" });
    }
    res.json({ success: true });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

// ==================== Tasks ====================

/**
 * GET /api/agents/:agentId/tasks
 * 获取 Agent 的任务列表
 */
router.get("/:agentId/tasks", (req: Request, res: Response) => {
  try {
    const { agentId } = req.params;
    const { status } = req.query;

    const agent = agentConfigDb.getById(agentId);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    const tasks = agentTaskDb.getByAgentId(agentId, status as string | undefined);
    res.json({ success: true, data: tasks });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * GET /api/agents/:agentId/tasks/:taskId
 * 获取单个任务详情（含关联产出物）
 */
router.get("/:agentId/tasks/:taskId", (req: Request, res: Response) => {
  try {
    const task = agentTaskDb.getById(req.params.taskId);
    if (!task) {
      return res.status(404).json({ success: false, error: "Task not found" });
    }

    const artifacts = agentArtifactDb.getByTaskId(task.id);

    res.json({
      success: true,
      data: { ...task, artifacts },
    });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * POST /api/agents/:agentId/tasks
 * 创建任务
 */
router.post("/:agentId/tasks", (req: Request, res: Response) => {
  try {
    const { agentId } = req.params;
    const { title, description, priority, sourceChannel, sourceSessionId, assignedBy } = req.body;

    if (!title) {
      return res.status(400).json({ success: false, error: "title is required" });
    }

    const agent = agentConfigDb.getById(agentId);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    const task = agentTaskDb.create({
      id: crypto.randomUUID(),
      agentId,
      title,
      description: description || "",
      status: "pending",
      priority: priority || "normal",
      sourceChannel,
      sourceSessionId,
      assignedBy,
    });

    logger.info(`Task created for [${agent.displayName}]: ${title}`, { agentId, taskId: task.id });
    res.status(201).json({ success: true, data: task });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * PUT /api/agents/:agentId/tasks/:taskId
 * 更新任务状态/内容
 */
router.put("/:agentId/tasks/:taskId", (req: Request, res: Response) => {
  try {
    const { title, description, status, priority, result } = req.body;
    const updated = agentTaskDb.update(req.params.taskId, { title, description, status, priority, result });

    if (!updated) {
      return res.status(404).json({ success: false, error: "Task not found" });
    }

    if (status) {
      logger.info(`Task status updated: ${updated.title} → ${status}`, {
        agentId: req.params.agentId,
        taskId: req.params.taskId,
      });
    }

    res.json({ success: true, data: updated });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * DELETE /api/agents/:agentId/tasks/:taskId
 */
router.delete("/:agentId/tasks/:taskId", (req: Request, res: Response) => {
  try {
    const deleted = agentTaskDb.delete(req.params.taskId);
    if (!deleted) {
      return res.status(404).json({ success: false, error: "Task not found" });
    }
    res.json({ success: true });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

// ==================== Artifacts ====================

/**
 * GET /api/agents/:agentId/artifacts
 * 获取 Agent 的产出物列表
 */
router.get("/:agentId/artifacts", (req: Request, res: Response) => {
  try {
    const { agentId } = req.params;

    const agent = agentConfigDb.getById(agentId);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    const artifacts = agentArtifactDb.getByAgentId(agentId);
    res.json({ success: true, data: artifacts });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * POST /api/agents/:agentId/artifacts
 * 记录产出物
 */
router.post("/:agentId/artifacts", (req: Request, res: Response) => {
  try {
    const { agentId } = req.params;
    const { taskId, type, title, content, filePath, metadata } = req.body;

    if (!title) {
      return res.status(400).json({ success: false, error: "title is required" });
    }

    const agent = agentConfigDb.getById(agentId);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    const artifact = agentArtifactDb.create({
      id: crypto.randomUUID(),
      agentId,
      taskId,
      type: type || "file",
      title,
      content,
      filePath,
      metadata: metadata || {},
    });

    logger.info(`Artifact created for [${agent.displayName}]: ${title}`, { agentId, artifactId: artifact.id });
    res.status(201).json({ success: true, data: artifact });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

/**
 * DELETE /api/agents/:agentId/artifacts/:artifactId
 */
router.delete("/:agentId/artifacts/:artifactId", (req: Request, res: Response) => {
  try {
    const deleted = agentArtifactDb.delete(req.params.artifactId);
    if (!deleted) {
      return res.status(404).json({ success: false, error: "Artifact not found" });
    }
    res.json({ success: true });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

// ==================== Workspace Overview ====================

/**
 * GET /api/agents/:agentId/workspace
 * 获取 Agent 工作空间概览
 */
router.get("/:agentId/workspace", (req: Request, res: Response) => {
  try {
    const { agentId } = req.params;

    const agent = agentConfigDb.getById(agentId);
    if (!agent) {
      return res.status(404).json({ success: false, error: "Agent not found" });
    }

    const notes = agentNoteDb.getByAgentId(agentId);
    const pendingTasks = agentTaskDb.getByAgentId(agentId, "pending");
    const inProgressTasks = agentTaskDb.getByAgentId(agentId, "in_progress");
    const recentArtifacts = agentArtifactDb.getByAgentId(agentId).slice(0, 10);

    res.json({
      success: true,
      data: {
        agent: { id: agent.id, displayName: agent.displayName },
        summary: {
          noteCount: notes.length,
          pendingTaskCount: pendingTasks.length,
          inProgressTaskCount: inProgressTasks.length,
          artifactCount: recentArtifacts.length,
        },
        recentNotes: notes.slice(0, 5),
        activeTasks: [...inProgressTasks, ...pendingTasks].slice(0, 10),
        recentArtifacts,
      },
    });
  } catch (error) {
    const msg = error instanceof Error ? error.message : "Unknown error";
    res.status(500).json({ success: false, error: msg });
  }
});

export default router;
