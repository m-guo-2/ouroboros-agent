/**
 * Skill Manager - 基于数据库的 Skill 管理
 *
 * Skill 数据存储在 SQLite 数据库中（skills + skill_versions 表），
 * 通过 API 向 agent 分发编译后的上下文。
 * 每次读取直接查 DB，无内存缓存，确保 agent 始终获取最新配置。
 */

import { skillDb } from "./database";
import type { SkillRecord, SkillToolDefinition, SkillType } from "./database";
import { logger } from "./logger";

// ==================== 类型定义 ====================

export interface SkillToolExecutor {
  type: "http" | "script" | "internal";
  url?: string;
  method?: string;
  command?: string;
  handler?: string;
}

export interface SkillContext {
  systemPromptAddition: string;
  tools: Array<{
    name: string;
    description: string;
    input_schema: {
      type: "object";
      properties: Record<string, unknown>;
      required?: string[];
    };
  }>;
  toolExecutors: Record<string, SkillToolExecutor>;
  skillDocs: Record<string, string>;
}

// Re-export for backward compatibility
export type { SkillRecord, SkillToolDefinition, SkillType };

// ==================== Skill Manager ====================

export const skillManager = {
  getAll(): SkillRecord[] {
    return skillDb.getAll();
  },

  getByName(id: string): SkillRecord | null {
    return skillDb.getById(id);
  },

  create(id: string, data: { name: string; description: string; type: SkillType; enabled?: boolean; triggers?: string[]; tools?: SkillToolDefinition[]; readme?: string }): SkillRecord {
    const skill = skillDb.create({ id, ...data });
    logger.info(`Skill created: ${id}`);
    return skill;
  },

  update(id: string, updates: Partial<Pick<SkillRecord, "name" | "description" | "type" | "triggers" | "tools" | "readme">>, changeSummary?: string): SkillRecord | null {
    const skill = skillDb.update(id, updates, changeSummary);
    if (skill) logger.info(`Skill updated: ${id} → v${skill.version}`);
    return skill;
  },

  delete(id: string): boolean {
    const deleted = skillDb.delete(id);
    if (deleted) logger.info(`Skill deleted: ${id}`);
    return deleted;
  },

  setEnabled(id: string, enabled: boolean): SkillRecord | null {
    return skillDb.setEnabled(id, enabled);
  },

  getVersions(skillId: string) {
    return skillDb.getVersions(skillId);
  },

  getVersion(skillId: string, version: number) {
    return skillDb.getVersion(skillId, version);
  },

  restoreVersion(skillId: string, version: number) {
    const skill = skillDb.restoreVersion(skillId, version);
    if (skill) logger.info(`Skill restored: ${skillId} → v${skill.version} (from v${version})`);
    return skill;
  },

  /**
   * 编译所有启用的 skill 为上下文（供 agent 消费）
   * 每次直接查 DB，无缓存
   */
  compileContext(): SkillContext {
    const skills = skillDb.getEnabled();

    const summaryLines: string[] = [];
    const actionDocs: string[] = [];

    for (const s of skills) {
      const toolNames = s.tools?.map(t => t.name).join(", ") || "";
      const toolSuffix = toolNames ? ` [工具: ${toolNames}]` : "";
      summaryLines.push(`- **${s.name}**: ${s.description}${toolSuffix}`);

      if ((s.type === "action" || s.type === "hybrid") && s.readme) {
        actionDocs.push(`### Skill: ${s.name}\n\n${s.readme}`);
      }
    }

    let systemPromptAddition = "";
    if (skills.length > 0) {
      systemPromptAddition = `\n## 你拥有的 Skills\n以下是你已注册的技能，可以根据用户需求主动使用：\n${summaryLines.join("\n")}`;
    }
    if (actionDocs.length > 0) {
      systemPromptAddition += `\n\n## Skill 使用指南\n\n${actionDocs.join("\n\n---\n\n")}`;
    }

    const tools: SkillContext["tools"] = [];
    const toolExecutors: Record<string, SkillToolExecutor> = {};

    for (const skill of skills) {
      if (skill.tools) {
        for (const tool of skill.tools) {
          tools.push({
            name: tool.name,
            description: `[Skill: ${skill.name}] ${tool.description}`,
            input_schema: tool.inputSchema,
          });
          toolExecutors[tool.name] = tool.executor;
        }
      }
    }

    tools.push({
      name: "get_skill_doc",
      description: "查阅指定 skill 的详细文档。当你需要了解某个 skill 的具体用法时使用。",
      input_schema: {
        type: "object",
        properties: {
          skill_name: {
            type: "string",
            description: "skill 的 ID（如 'channel-reply'）",
          },
        },
        required: ["skill_name"],
      },
    });
    toolExecutors["get_skill_doc"] = { type: "internal", handler: "get_skill_doc" };

    const skillDocs: Record<string, string> = {};
    for (const skill of skills) {
      if (skill.readme) {
        skillDocs[skill.id] = skill.readme;
      }
    }

    return { systemPromptAddition, tools, toolExecutors, skillDocs };
  },
};
