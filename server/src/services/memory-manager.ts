/**
 * 用户记忆管理服务
 * 负责加载用户记忆上下文、添加/更新事实、重新生成摘要
 *
 * 多 Agent 架构：记忆按 agentId × userId 隔离
 * 每个 Agent 对同一个用户有独立的记忆空间
 */

import { userMemoryDb, userMemoryFactDb } from "./database";
import type { UserMemoryFactRecord } from "./database";
import type { ChannelType } from "./channel-types";

/**
 * 记忆上下文，用于注入到 agent 的 instruction 中
 */
export interface MemoryContext {
  /** 用户全局记忆摘要 */
  summary: string;
  /** 结构化的用户事实 */
  facts: UserMemoryFactRecord[];
  /** 格式化后的 prompt 片段 */
  prompt: string;
}

/**
 * 加载用户的记忆上下文（按 agentId × userId 隔离）
 * 返回格式化的 prompt 片段，可直接注入到 agent instruction 中
 */
export function loadMemoryContext(userId: string, agentId?: string): MemoryContext {
  const memory = userMemoryDb.getByUserId(userId, agentId);
  const facts = userMemoryFactDb.getByUserId(userId, agentId);

  const summary = memory?.summary || "";

  // 构建 prompt 片段
  const promptParts: string[] = [];

  if (summary) {
    promptParts.push(`[用户背景]\n${summary}`);
  }

  if (facts.length > 0) {
    const grouped = groupFactsByCategory(facts);
    const factLines: string[] = [];

    if (grouped.preference.length > 0) {
      factLines.push("偏好: " + grouped.preference.map(f => f.fact).join("; "));
    }
    if (grouped.context.length > 0) {
      factLines.push("上下文: " + grouped.context.map(f => f.fact).join("; "));
    }
    if (grouped.relationship.length > 0) {
      factLines.push("关系: " + grouped.relationship.map(f => f.fact).join("; "));
    }
    if (grouped.skill.length > 0) {
      factLines.push("技能: " + grouped.skill.map(f => f.fact).join("; "));
    }

    if (factLines.length > 0) {
      promptParts.push(`[已知信息]\n${factLines.join("\n")}`);
    }
  }

  return {
    summary,
    facts,
    prompt: promptParts.length > 0 ? promptParts.join("\n\n") : "",
  };
}

/**
 * 添加一个用户记忆事实（按 agentId × userId 隔离）
 */
export function addFact(
  userId: string,
  category: UserMemoryFactRecord["category"],
  fact: string,
  sourceChannel?: ChannelType,
  sourceSessionId?: string,
  expiresAt?: string,
  agentId?: string
): UserMemoryFactRecord {
  return userMemoryFactDb.create({
    id: crypto.randomUUID(),
    userId,
    agentId,
    category,
    fact,
    sourceChannel,
    sourceSessionId,
    expiresAt,
  });
}

/**
 * 更新用户记忆摘要（按 agentId × userId 隔离）
 */
export function updateSummary(userId: string, summary: string, agentId?: string): void {
  userMemoryDb.upsert(userId, summary, agentId);
}

/**
 * 获取用户的记忆摘要和所有事实（用于 API 返回）
 */
export function getUserMemory(userId: string, agentId?: string) {
  const memory = userMemoryDb.getByUserId(userId, agentId);
  const facts = userMemoryFactDb.getByUserId(userId, agentId);

  return {
    summary: memory?.summary || "",
    facts,
    factCount: facts.length,
    categories: groupFactsByCategory(facts),
  };
}

/**
 * 删除一个记忆事实
 */
export function deleteFact(factId: string): boolean {
  return userMemoryFactDb.delete(factId);
}

/**
 * 按类别分组事实
 */
function groupFactsByCategory(facts: UserMemoryFactRecord[]) {
  return {
    preference: facts.filter(f => f.category === "preference"),
    context: facts.filter(f => f.category === "context"),
    relationship: facts.filter(f => f.category === "relationship"),
    skill: facts.filter(f => f.category === "skill"),
  };
}
