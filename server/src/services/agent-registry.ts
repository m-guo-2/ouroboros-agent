/**
 * Agent 实例注册表
 * 管理 Agent App 实例的注册、心跳和路由
 *
 * 滚动更新：同一时间可有两个 endpoint（v1 draining + v2 ready）
 * Server 只向 status='ready' 的 endpoint 派发新请求
 */

import { db } from "./database";
import { logger } from "./logger";

// ==================== Schema ====================

// 初始化 agent_registry 表
db.run(`
  CREATE TABLE IF NOT EXISTS agent_registry (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    version TEXT,
    status TEXT DEFAULT 'ready',
    inflight_count INTEGER DEFAULT 0,
    registered_at TEXT DEFAULT CURRENT_TIMESTAMP,
    last_heartbeat TEXT DEFAULT CURRENT_TIMESTAMP
  )
`);

// ==================== Types ====================

export interface AgentEndpoint {
  id: string;
  url: string;
  version?: string;
  status: "ready" | "draining" | "down";
  inflightCount: number;
  registeredAt: string;
  lastHeartbeat: string;
}

interface AgentRegistryRow {
  id: string;
  url: string;
  version: string | null;
  status: string;
  inflight_count: number;
  registered_at: string;
  last_heartbeat: string;
}

function rowToEndpoint(row: AgentRegistryRow): AgentEndpoint {
  return {
    id: row.id,
    url: row.url,
    version: row.version || undefined,
    status: row.status as AgentEndpoint["status"],
    inflightCount: row.inflight_count,
    registeredAt: row.registered_at,
    lastHeartbeat: row.last_heartbeat,
  };
}

// ==================== Registry ====================

/** 默认 Agent 端口（兼容无注册场景） */
const DEFAULT_AGENT_URL = process.env.AGENT_APP_URL || "http://localhost:1996";

export const agentRegistry = {
  /**
   * 注册一个 Agent 实例
   * 如果同 id 已存在则更新
   */
  register(endpoint: { id: string; url: string; version?: string }): AgentEndpoint {
    db.run(`
      INSERT INTO agent_registry (id, url, version, status, inflight_count, registered_at, last_heartbeat)
      VALUES (?, ?, ?, 'ready', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
      ON CONFLICT(id) DO UPDATE SET
        url = ?, version = ?, status = 'ready',
        last_heartbeat = CURRENT_TIMESTAMP
    `, [
      endpoint.id, endpoint.url, endpoint.version || null,
      endpoint.url, endpoint.version || null,
    ]);

    logger.info(`Agent registered: ${endpoint.id} @ ${endpoint.url} (v${endpoint.version || "unknown"})`);
    return this.getById(endpoint.id)!;
  },

  /**
   * 更新心跳
   */
  heartbeat(id: string): void {
    db.run(
      "UPDATE agent_registry SET last_heartbeat = CURRENT_TIMESTAMP WHERE id = ?",
      [id]
    );
  },

  /**
   * 标记为 draining（不再接新请求）
   */
  markDraining(id: string): void {
    db.run(
      "UPDATE agent_registry SET status = 'draining' WHERE id = ?",
      [id]
    );
    logger.info(`Agent marked as draining: ${id}`);
  },

  /**
   * 标记为 down
   */
  markDown(id: string): void {
    db.run(
      "UPDATE agent_registry SET status = 'down' WHERE id = ?",
      [id]
    );
    logger.info(`Agent marked as down: ${id}`);
  },

  /**
   * 移除注册
   */
  unregister(id: string): void {
    db.run("DELETE FROM agent_registry WHERE id = ?", [id]);
    logger.info(`Agent unregistered: ${id}`);
  },

  /**
   * 获取单个 endpoint
   */
  getById(id: string): AgentEndpoint | null {
    const row = db.query("SELECT * FROM agent_registry WHERE id = ?").get(id) as AgentRegistryRow | null;
    return row ? rowToEndpoint(row) : null;
  },

  /**
   * 获取所有已注册的 endpoint
   */
  getAll(): AgentEndpoint[] {
    const rows = db.query("SELECT * FROM agent_registry ORDER BY registered_at DESC").all() as AgentRegistryRow[];
    return rows.map(rowToEndpoint);
  },

  /**
   * 获取一个可用的 Agent endpoint URL
   * 优先选择 status='ready' 的实例
   * 如果没有注册的实例，返回默认 URL（兼容旧架构）
   */
  getEndpoint(agentId?: string): string {
    // 先查 ready 的
    const ready = db.query(
      "SELECT * FROM agent_registry WHERE status = 'ready' ORDER BY last_heartbeat DESC LIMIT 1"
    ).get() as AgentRegistryRow | null;

    if (ready) return ready.url;

    // 没有注册的实例，返回默认（兼容旧架构过渡期）
    return DEFAULT_AGENT_URL;
  },

  /**
   * 增加 inflight 计数
   */
  incrementInflight(id: string): void {
    db.run(
      "UPDATE agent_registry SET inflight_count = inflight_count + 1 WHERE id = ?",
      [id]
    );
  },

  /**
   * 减少 inflight 计数
   */
  decrementInflight(id: string): void {
    db.run(
      "UPDATE agent_registry SET inflight_count = MAX(0, inflight_count - 1) WHERE id = ?",
      [id]
    );
  },

  /**
   * 清理过期的 endpoint（心跳超过 N 秒未更新）
   */
  cleanupStale(maxAgeSeconds: number = 60): number {
    const result = db.run(
      `DELETE FROM agent_registry WHERE last_heartbeat < datetime('now', '-${maxAgeSeconds} seconds')`
    );
    if (result.changes > 0) {
      logger.info(`Cleaned up ${result.changes} stale agent endpoints`);
    }
    return result.changes;
  },
};
