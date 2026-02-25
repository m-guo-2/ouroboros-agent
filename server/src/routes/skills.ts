/**
 * Skills API Routes
 *
 * Admin 端管理 skill（数据库存储），agent 通过 /api/skills/context 获取编译后的上下文
 */

import { Router } from "express";
import { skillManager } from "../services/skill-manager";
import type { SkillType, SkillToolDefinition } from "../services/skill-manager";

const router = Router();

// ==================== 固定路径先注册（避免被 /:name 拦截） ====================

/**
 * GET /api/skills/context
 * 获取编译后的 skill 上下文（供 agent 消费）
 */
router.get("/context", (_req, res) => {
  const context = skillManager.compileContext();
  res.json({ success: true, data: context });
});

// ==================== Admin CRUD 接口 ====================

/**
 * GET /api/skills
 * 获取所有 skill 列表
 */
router.get("/", (_req, res) => {
  const skills = skillManager.getAll();
  res.json({
    success: true,
    data: skills.map(s => ({
      name: s.id,
      description: s.description,
      version: s.version,
      type: s.type,
      enabled: s.enabled,
      triggers: s.triggers || [],
      toolCount: s.tools?.length || 0,
      tools: s.tools?.map(t => t.name) || [],
    })),
  });
});

/**
 * GET /api/skills/:name
 * 获取单个 skill 详情
 */
router.get("/:name", (req, res) => {
  const skill = skillManager.getByName(req.params.name);
  if (!skill) {
    res.status(404).json({ success: false, error: `Skill '${req.params.name}' not found` });
    return;
  }

  res.json({
    success: true,
    data: {
      name: skill.id,
      manifest: {
        name: skill.name,
        description: skill.description,
        version: skill.version,
        type: skill.type,
        enabled: skill.enabled,
        triggers: skill.triggers,
        tools: skill.tools,
      },
      readme: skill.readme,
    },
  });
});

/**
 * POST /api/skills
 * 创建新 skill
 */
router.post("/", (req, res) => {
  const { name, manifest, readme } = req.body as {
    name: string;
    manifest: { name: string; description: string; type: SkillType; enabled?: boolean; triggers?: string[]; tools?: SkillToolDefinition[] };
    readme?: string;
  };

  if (!name || !manifest) {
    res.status(400).json({ success: false, error: "需要 name 和 manifest" });
    return;
  }

  try {
    const skill = skillManager.create(name, {
      name: manifest.name || name,
      description: manifest.description || "",
      type: manifest.type || "knowledge",
      enabled: manifest.enabled,
      triggers: manifest.triggers,
      tools: manifest.tools,
      readme: readme || "",
    });
    res.json({
      success: true,
      data: {
        name: skill.id,
        manifest: {
          name: skill.name,
          description: skill.description,
          version: skill.version,
          type: skill.type,
          enabled: skill.enabled,
          triggers: skill.triggers,
          tools: skill.tools,
        },
      },
    });
  } catch (error) {
    res.status(400).json({ success: false, error: (error as Error).message });
  }
});

/**
 * PUT /api/skills/:name
 * 更新 skill
 */
router.put("/:name", (req, res) => {
  const { manifest, readme, changeSummary } = req.body as {
    manifest?: Partial<{ name: string; description: string; type: SkillType; triggers: string[]; tools: SkillToolDefinition[] }>;
    readme?: string;
    changeSummary?: string;
  };

  const updates: Parameters<typeof skillManager.update>[1] = {};
  if (manifest?.name !== undefined) updates.name = manifest.name;
  if (manifest?.description !== undefined) updates.description = manifest.description;
  if (manifest?.type !== undefined) updates.type = manifest.type;
  if (manifest?.triggers !== undefined) updates.triggers = manifest.triggers;
  if (manifest?.tools !== undefined) updates.tools = manifest.tools;
  if (readme !== undefined) updates.readme = readme;

  const skill = skillManager.update(req.params.name, updates, changeSummary);
  if (!skill) {
    res.status(404).json({ success: false, error: `Skill '${req.params.name}' not found` });
    return;
  }

  res.json({
    success: true,
    data: {
      name: skill.id,
      manifest: {
        name: skill.name,
        description: skill.description,
        version: skill.version,
        type: skill.type,
        enabled: skill.enabled,
        triggers: skill.triggers,
        tools: skill.tools,
      },
    },
  });
});

/**
 * PATCH /api/skills/:name/toggle
 * 启用/禁用 skill（不产生新版本）
 */
router.patch("/:name/toggle", (req, res) => {
  const { enabled } = req.body as { enabled: boolean };
  if (typeof enabled !== "boolean") {
    res.status(400).json({ success: false, error: "需要 boolean 类型的 enabled" });
    return;
  }

  const skill = skillManager.setEnabled(req.params.name, enabled);
  if (!skill) {
    res.status(404).json({ success: false, error: `Skill '${req.params.name}' not found` });
    return;
  }

  res.json({ success: true, data: { name: skill.id, enabled: skill.enabled } });
});

/**
 * DELETE /api/skills/:name
 * 删除 skill（及其版本历史）
 */
router.delete("/:name", (req, res) => {
  const deleted = skillManager.delete(req.params.name);
  if (!deleted) {
    res.status(404).json({ success: false, error: `Skill '${req.params.name}' not found` });
    return;
  }
  res.json({ success: true });
});

// ==================== 版本管理接口 ====================

/**
 * GET /api/skills/:name/versions
 * 获取 skill 的版本历史
 */
router.get("/:name/versions", (req, res) => {
  const skill = skillManager.getByName(req.params.name);
  if (!skill) {
    res.status(404).json({ success: false, error: `Skill '${req.params.name}' not found` });
    return;
  }

  const versions = skillManager.getVersions(req.params.name);
  res.json({
    success: true,
    data: versions.map(v => ({
      version: v.version,
      changeSummary: v.changeSummary,
      createdAt: v.createdAt,
    })),
  });
});

/**
 * GET /api/skills/:name/versions/:version
 * 获取特定版本详情
 */
router.get("/:name/versions/:version", (req, res) => {
  const version = parseInt(req.params.version, 10);
  if (isNaN(version)) {
    res.status(400).json({ success: false, error: "版本号必须是数字" });
    return;
  }

  const ver = skillManager.getVersion(req.params.name, version);
  if (!ver) {
    res.status(404).json({ success: false, error: `版本 ${version} 不存在` });
    return;
  }

  res.json({
    success: true,
    data: {
      version: ver.version,
      name: ver.name,
      description: ver.description,
      type: ver.type,
      triggers: ver.triggers,
      tools: ver.tools,
      readme: ver.readme,
      changeSummary: ver.changeSummary,
      createdAt: ver.createdAt,
    },
  });
});

/**
 * POST /api/skills/:name/versions/:version/restore
 * 回滚到指定版本（产生新版本号）
 */
router.post("/:name/versions/:version/restore", (req, res) => {
  const version = parseInt(req.params.version, 10);
  if (isNaN(version)) {
    res.status(400).json({ success: false, error: "版本号必须是数字" });
    return;
  }

  const skill = skillManager.restoreVersion(req.params.name, version);
  if (!skill) {
    res.status(404).json({ success: false, error: `回滚失败：skill 或版本不存在` });
    return;
  }

  res.json({
    success: true,
    data: {
      name: skill.id,
      version: skill.version,
      message: `已回滚到版本 ${version}，当前版本为 v${skill.version}`,
    },
  });
});

export default router;
