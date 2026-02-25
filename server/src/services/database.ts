/**
 * 本地数据库服务
 * 使用 Bun 内置的 SQLite 存储配置信息
 */

import { Database } from "bun:sqlite";
import { resolve } from "path";

// 数据库文件路径
const DB_PATH = resolve(import.meta.dir, "../../data/config.db");

// 确保 data 目录存在
import { mkdirSync, existsSync } from "fs";
const dataDir = resolve(import.meta.dir, "../../data");
if (!existsSync(dataDir)) {
  mkdirSync(dataDir, { recursive: true });
}

// 创建数据库连接
const db = new Database(DB_PATH);

// 先设置 busy_timeout，避免后续 PRAGMA 遇锁时立即报 SQLITE_BUSY
db.run("PRAGMA busy_timeout = 5000");
// 启用 WAL 模式（支持并发读写，避免 "readonly database" 错误）
db.run("PRAGMA journal_mode = WAL");

// 启动时尝试 WAL checkpoint，防止 WAL 文件膨胀导致 disk I/O error
try {
  db.run("PRAGMA wal_checkpoint(TRUNCATE)");
} catch { /* checkpoint 失败不阻塞启动 */ }

// 定期 WAL checkpoint（每 60 秒），防止 WAL 累积过大
setInterval(() => {
  try {
    db.run("PRAGMA wal_checkpoint(PASSIVE)");
  } catch { /* 静默忽略 */ }
}, 60_000);

// 初始化数据库表
db.run(`
  CREATE TABLE IF NOT EXISTS models (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    api_key TEXT,
    base_url TEXT,
    model TEXT NOT NULL,
    max_tokens INTEGER DEFAULT 4096,
    temperature REAL DEFAULT 0.7,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
  )
`);

db.run(`
  CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    title TEXT,
    model_id TEXT NOT NULL,
    messages TEXT DEFAULT '[]',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (model_id) REFERENCES models(id)
  )
`);

db.run(`
  CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
  )
`);

// Agent 会话表 - 存储 Agent 模式的聊天记录
db.run(`
  CREATE TABLE IF NOT EXISTS agent_sessions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '新对话',
    sdk_session_id TEXT,
    user_id TEXT,
    agent_id TEXT,
    source_channel TEXT DEFAULT 'webui',
    execution_status TEXT DEFAULT 'idle',
    channel_name TEXT,
    channel_conversation_id TEXT,
    session_key TEXT,
    work_dir TEXT,
    messages TEXT DEFAULT '[]',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
  )
`);

// 迁移：给已有的 agent_sessions 表添加新列（如果缺失）
try {
  db.run(`ALTER TABLE agent_sessions ADD COLUMN user_id TEXT`);
} catch { /* column already exists */ }
try {
  db.run(`ALTER TABLE agent_sessions ADD COLUMN source_channel TEXT DEFAULT 'webui'`);
} catch { /* column already exists */ }
// 多 Agent 架构迁移：agent_sessions 增加 agent_id
try {
  db.run(`ALTER TABLE agent_sessions ADD COLUMN agent_id TEXT`);
} catch { /* column already exists */ }
db.run(`CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent ON agent_sessions(agent_id)`);

// Agent 重构迁移：agent_sessions 增加 execution_status（断点续传）
try {
  db.run(`ALTER TABLE agent_sessions ADD COLUMN execution_status TEXT DEFAULT 'idle'`);
} catch { /* column already exists */ }
db.run(`CREATE INDEX IF NOT EXISTS idx_agent_sessions_exec_status ON agent_sessions(execution_status)`);

// 迁移：agent_sessions 增加 channel_name（群名/私聊名称）
try {
  db.run(`ALTER TABLE agent_sessions ADD COLUMN channel_name TEXT`);
} catch { /* column already exists */ }

// 迁移：agent_sessions 增加 channel_conversation_id（渠道会话ID，如飞书chat_id）
try {
  db.run(`ALTER TABLE agent_sessions ADD COLUMN channel_conversation_id TEXT`);
} catch { /* column already exists */ }
db.run(`CREATE INDEX IF NOT EXISTS idx_agent_sessions_conv_id ON agent_sessions(channel_conversation_id)`);

// 迁移：agent_sessions 增加 session_key（channel:uniqueId）
try {
  db.run(`ALTER TABLE agent_sessions ADD COLUMN session_key TEXT`);
} catch { /* column already exists */ }

// 迁移：agent_sessions 增加 work_dir（每个 session 独立工作目录）
try {
  db.run(`ALTER TABLE agent_sessions ADD COLUMN work_dir TEXT`);
} catch { /* column already exists */ }

db.run(`CREATE INDEX IF NOT EXISTS idx_agent_sessions_session_key ON agent_sessions(session_key)`);
db.run(`CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent_key ON agent_sessions(agent_id, session_key)`);

// ==================== Unified Channel Tables ====================

// 统一参与者身份表（人类和 Agent 共享同一张表）
db.run(`
  CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'human',
    avatar_url TEXT,
    metadata TEXT DEFAULT '{}',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
  )
`);
// 迁移：给已有 users 表添加 type 字段
try {
  db.run(`ALTER TABLE users ADD COLUMN type TEXT NOT NULL DEFAULT 'human'`);
} catch { /* column already exists */ }

// Agent 配置表（Agent 的 systemPrompt、技能、模型等）
db.run(`
  CREATE TABLE IF NOT EXISTS agent_configs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    system_prompt TEXT DEFAULT '',
    model_id TEXT,
    skills TEXT DEFAULT '[]',
    channels TEXT DEFAULT '[]',
    is_active INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
  )
`);
db.run(`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_configs_user ON agent_configs(user_id)`);

// 迁移：为 agent_configs 添加 provider 和 model 列（直接指定 LLM 提供商和模型）
try {
  db.run(`ALTER TABLE agent_configs ADD COLUMN provider TEXT`);
} catch { /* column already exists */ }
try {
  db.run(`ALTER TABLE agent_configs ADD COLUMN model TEXT`);
} catch { /* column already exists */ }

// 用户渠道绑定表（一个用户可绑定多个渠道账号）
db.run(`
  CREATE TABLE IF NOT EXISTS user_channels (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    channel_type TEXT NOT NULL,
    channel_user_id TEXT NOT NULL,
    display_name TEXT,
    channel_meta TEXT DEFAULT '{}',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
  )
`);
db.run(`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_channels_unique ON user_channels(channel_type, channel_user_id)`);
db.run(`CREATE INDEX IF NOT EXISTS idx_user_channels_user ON user_channels(user_id)`);

// 用户记忆摘要（按 agent_id × user_id 隔离）
db.run(`
  CREATE TABLE IF NOT EXISTS user_memory (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    agent_id TEXT NOT NULL DEFAULT '',
    summary TEXT DEFAULT '',
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
  )
`);
db.run(`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_memory_agent_user ON user_memory(agent_id, user_id)`);

// 用户记忆事实（结构化，按 agent_id × user_id 隔离）
db.run(`
  CREATE TABLE IF NOT EXISTS user_memory_facts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    agent_id TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL,
    fact TEXT NOT NULL,
    source_channel TEXT,
    source_session_id TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    expires_at TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
  )
`);
db.run(`CREATE INDEX IF NOT EXISTS idx_memory_facts_user ON user_memory_facts(user_id)`);
db.run(`CREATE INDEX IF NOT EXISTS idx_memory_facts_agent_user ON user_memory_facts(agent_id, user_id)`);

// Note: agent_id 列已在 CREATE TABLE 中定义（user_memory 和 user_memory_facts）
// 以下迁移仅用于升级旧数据库（agent_id 列缺失的情况）
try {
  db.run("ALTER TABLE user_memory ADD COLUMN agent_id TEXT DEFAULT ''");
} catch { /* 列已存在 */ }
try {
  db.run("ALTER TABLE user_memory_facts ADD COLUMN agent_id TEXT DEFAULT ''");
} catch { /* 列已存在 */ }

// 用户跨渠道绑定码
db.run(`
  CREATE TABLE IF NOT EXISTS user_binding_codes (
    code TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    target_channel TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
  )
`);

// 消息去重表
db.run(`
  CREATE TABLE IF NOT EXISTS processed_messages (
    channel_message_id TEXT PRIMARY KEY,
    channel_type TEXT NOT NULL,
    processed_at TEXT DEFAULT CURRENT_TIMESTAMP
  )
`);

// Agent 工作记忆（短期工作笔记，独立于用户记忆）
db.run(`
  CREATE TABLE IF NOT EXISTS agent_notes (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'general',
    content TEXT NOT NULL,
    related_session_id TEXT,
    related_user_id TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agent_configs(id)
  )
`);
db.run(`CREATE INDEX IF NOT EXISTS idx_agent_notes_agent ON agent_notes(agent_id)`);

// Agent 任务记录
db.run(`
  CREATE TABLE IF NOT EXISTS agent_tasks (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    priority TEXT DEFAULT 'normal',
    source_channel TEXT,
    source_session_id TEXT,
    assigned_by TEXT,
    result TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    completed_at TEXT,
    FOREIGN KEY (agent_id) REFERENCES agent_configs(id)
  )
`);
db.run(`CREATE INDEX IF NOT EXISTS idx_agent_tasks_agent ON agent_tasks(agent_id)`);
db.run(`CREATE INDEX IF NOT EXISTS idx_agent_tasks_status ON agent_tasks(agent_id, status)`);

// Agent 产出物（Agent 生成的文件、文档、代码等引用）
db.run(`
  CREATE TABLE IF NOT EXISTS agent_artifacts (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    task_id TEXT,
    type TEXT NOT NULL DEFAULT 'file',
    title TEXT NOT NULL,
    content TEXT,
    file_path TEXT,
    metadata TEXT DEFAULT '{}',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agent_configs(id),
    FOREIGN KEY (task_id) REFERENCES agent_tasks(id)
  )
`);
db.run(`CREATE INDEX IF NOT EXISTS idx_agent_artifacts_agent ON agent_artifacts(agent_id)`);

// 独立消息表（每条消息一行，支持分页查询和索引）
db.run(`
  CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    message_type TEXT DEFAULT 'text',
    channel TEXT,
    channel_message_id TEXT,
    reply_to_message_id TEXT,
    mentions TEXT,
    channel_meta TEXT,
    tool_calls TEXT,
    trace_id TEXT,
    initiator TEXT,
    status TEXT DEFAULT 'sent',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
  )
`);
db.run(`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id)`);
db.run(`CREATE INDEX IF NOT EXISTS idx_messages_created ON messages(created_at)`);
db.run(`CREATE INDEX IF NOT EXISTS idx_messages_channel_msg ON messages(channel_message_id)`);

// 迁移：给已有 messages 表添加 initiator 列（如果缺失）
try {
  db.run(`ALTER TABLE messages ADD COLUMN initiator TEXT`);
} catch { /* column already exists */ }

// 迁移：给已有 messages 表添加 sender_name / sender_id 列（群聊身份区分）
try {
  db.run(`ALTER TABLE messages ADD COLUMN sender_name TEXT`);
} catch { /* column already exists */ }
try {
  db.run(`ALTER TABLE messages ADD COLUMN sender_id TEXT`);
} catch { /* column already exists */ }

console.log(`📦 Database initialized: ${DB_PATH}`);

// ==================== Model CRUD ====================

export interface ModelRecord {
  id: string;
  name: string;
  provider: "claude" | "openai" | "kimi" | "glm";
  enabled: boolean;
  apiKey?: string;
  baseUrl?: string;
  model: string;
  maxTokens: number;
  temperature: number;
  createdAt?: string;
  updatedAt?: string;
}

// 数据库行类型
interface ModelRow {
  id: string;
  name: string;
  provider: string;
  enabled: number;
  api_key: string | null;
  base_url: string | null;
  model: string;
  max_tokens: number;
  temperature: number;
  created_at: string;
  updated_at: string;
}

function rowToModel(row: ModelRow): ModelRecord {
  return {
    id: row.id,
    name: row.name,
    provider: row.provider as ModelRecord["provider"],
    enabled: row.enabled === 1,
    apiKey: row.api_key || undefined,
    baseUrl: row.base_url || undefined,
    model: row.model,
    maxTokens: row.max_tokens,
    temperature: row.temperature,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export const modelDb = {
  // 获取所有模型
  getAll(): ModelRecord[] {
    const rows = db.query("SELECT * FROM models ORDER BY created_at").all() as ModelRow[];
    return rows.map(rowToModel);
  },

  // 获取单个模型
  getById(id: string): ModelRecord | null {
    const row = db.query("SELECT * FROM models WHERE id = ?").get(id) as ModelRow | null;
    return row ? rowToModel(row) : null;
  },

  // 创建模型
  create(model: Omit<ModelRecord, "createdAt" | "updatedAt">): ModelRecord {
    const stmt = db.prepare(`
      INSERT INTO models (id, name, provider, enabled, api_key, base_url, model, max_tokens, temperature)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `);
    stmt.run(
      model.id,
      model.name,
      model.provider,
      model.enabled ? 1 : 0,
      model.apiKey || null,
      model.baseUrl || null,
      model.model,
      model.maxTokens,
      model.temperature
    );
    return this.getById(model.id)!;
  },

  // 更新模型
  update(id: string, updates: Partial<ModelRecord>): ModelRecord | null {
    const existing = this.getById(id);
    if (!existing) return null;

    const fields: string[] = [];
    const values: (string | number | null)[] = [];

    if (updates.name !== undefined) {
      fields.push("name = ?");
      values.push(updates.name);
    }
    if (updates.provider !== undefined) {
      fields.push("provider = ?");
      values.push(updates.provider);
    }
    if (updates.enabled !== undefined) {
      fields.push("enabled = ?");
      values.push(updates.enabled ? 1 : 0);
    }
    if (updates.apiKey !== undefined) {
      fields.push("api_key = ?");
      values.push(updates.apiKey || null);
    }
    if (updates.baseUrl !== undefined) {
      fields.push("base_url = ?");
      values.push(updates.baseUrl || null);
    }
    if (updates.model !== undefined) {
      fields.push("model = ?");
      values.push(updates.model);
    }
    if (updates.maxTokens !== undefined) {
      fields.push("max_tokens = ?");
      values.push(updates.maxTokens);
    }
    if (updates.temperature !== undefined) {
      fields.push("temperature = ?");
      values.push(updates.temperature);
    }

    if (fields.length > 0) {
      fields.push("updated_at = CURRENT_TIMESTAMP");
      values.push(id);
      db.run(`UPDATE models SET ${fields.join(", ")} WHERE id = ?`, values);
    }

    return this.getById(id);
  },

  // 删除模型
  delete(id: string): boolean {
    const result = db.run("DELETE FROM models WHERE id = ?", [id]);
    return result.changes > 0;
  },

  // 检查模型是否存在
  exists(id: string): boolean {
    const row = db.query("SELECT 1 FROM models WHERE id = ?").get(id);
    return !!row;
  },

  // 初始化默认模型（如果表为空）
  initDefaults(defaults: Omit<ModelRecord, "createdAt" | "updatedAt">[]): void {
    const count = db.query("SELECT COUNT(*) as count FROM models").get() as { count: number };
    if (count.count === 0) {
      console.log("📝 Initializing default models...");
      for (const model of defaults) {
        this.create(model);
      }
      console.log(`✅ Created ${defaults.length} default models`);
    }
  },
};

// ==================== Conversation CRUD ====================

export interface ConversationRecord {
  id: string;
  title: string;
  modelId: string;
  messages: Array<{ role: string; content: string }>;
  createdAt?: string;
  updatedAt?: string;
}

interface ConversationRow {
  id: string;
  title: string;
  model_id: string;
  messages: string;
  created_at: string;
  updated_at: string;
}

function rowToConversation(row: ConversationRow): ConversationRecord {
  return {
    id: row.id,
    title: row.title,
    modelId: row.model_id,
    messages: JSON.parse(row.messages || "[]"),
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export const conversationDb = {
  getAll(): ConversationRecord[] {
    const rows = db.query("SELECT * FROM conversations ORDER BY updated_at DESC").all() as ConversationRow[];
    return rows.map(rowToConversation);
  },

  getById(id: string): ConversationRecord | null {
    const row = db.query("SELECT * FROM conversations WHERE id = ?").get(id) as ConversationRow | null;
    return row ? rowToConversation(row) : null;
  },

  create(conv: Omit<ConversationRecord, "createdAt" | "updatedAt">): ConversationRecord {
    const stmt = db.prepare(`
      INSERT INTO conversations (id, title, model_id, messages)
      VALUES (?, ?, ?, ?)
    `);
    stmt.run(conv.id, conv.title, conv.modelId, JSON.stringify(conv.messages));
    return this.getById(conv.id)!;
  },

  update(id: string, updates: Partial<ConversationRecord>): ConversationRecord | null {
    const existing = this.getById(id);
    if (!existing) return null;

    const fields: string[] = [];
    const values: (string | null)[] = [];

    if (updates.title !== undefined) {
      fields.push("title = ?");
      values.push(updates.title);
    }
    if (updates.modelId !== undefined) {
      fields.push("model_id = ?");
      values.push(updates.modelId);
    }
    if (updates.messages !== undefined) {
      fields.push("messages = ?");
      values.push(JSON.stringify(updates.messages));
    }

    if (fields.length > 0) {
      fields.push("updated_at = CURRENT_TIMESTAMP");
      values.push(id);
      db.run(`UPDATE conversations SET ${fields.join(", ")} WHERE id = ?`, values);
    }

    return this.getById(id);
  },

  delete(id: string): boolean {
    const result = db.run("DELETE FROM conversations WHERE id = ?", [id]);
    return result.changes > 0;
  },

  // 添加消息到对话
  addMessage(id: string, message: { role: string; content: string }): ConversationRecord | null {
    const conv = this.getById(id);
    if (!conv) return null;

    conv.messages.push(message);
    return this.update(id, { messages: conv.messages });
  },
};

// ==================== Settings CRUD ====================

export const settingsDb = {
  get(key: string): string | null {
    const row = db.query("SELECT value FROM settings WHERE key = ?").get(key) as { value: string } | null;
    return row?.value || null;
  },

  set(key: string, value: string): void {
    db.run(`
      INSERT INTO settings (key, value, updated_at) 
      VALUES (?, ?, CURRENT_TIMESTAMP)
      ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = CURRENT_TIMESTAMP
    `, [key, value, value]);
  },

  delete(key: string): boolean {
    const result = db.run("DELETE FROM settings WHERE key = ?", [key]);
    return result.changes > 0;
  },

  getAll(): Record<string, string> {
    const rows = db.query("SELECT key, value FROM settings").all() as { key: string; value: string }[];
    return Object.fromEntries(rows.map(r => [r.key, r.value]));
  },
};

// ==================== Agent Session CRUD ====================

export interface AgentMessageRecord {
  role: "user" | "assistant";
  content: string;
  channel?: "feishu" | "qiwei" | "webui";
  toolCalls?: Array<{
    id: string;
    tool: string;
    input: unknown;
    result?: unknown;
    status: "pending" | "running" | "success" | "error";
  }>;
  timestamp?: string;
  traceId?: string;     // 关联到 trace 日志，用于可观测性
  initiator?: "user" | "agent" | "system";  // 消息发起方（谁触发了这次交互）
}

export interface AgentSessionRecord {
  id: string;
  title: string;
  sdkSessionId?: string;
  userId?: string;
  agentId?: string;
  sourceChannel?: string;
  /** 会话唯一键：channel:uniqueId */
  sessionKey?: string;
  /** 群名/私聊对方昵称（来自渠道平台） */
  channelName?: string;
  /** 渠道会话ID（如飞书chat_id），用于区分同一用户的不同群聊/私聊 */
  channelConversationId?: string;
  /** Session 独立工作目录 */
  workDir?: string;
  /** 会话执行状态 */
  executionStatus?: string;
  // messages 已从 session 移除，统一使用 messagesDb.getBySession() 读取
  createdAt?: string;
  updatedAt?: string;
}

interface AgentSessionRow {
  id: string;
  title: string;
  sdk_session_id: string | null;
  user_id: string | null;
  agent_id: string | null;
  source_channel: string | null;
  session_key: string | null;
  channel_name: string | null;
  channel_conversation_id: string | null;
  work_dir: string | null;
  execution_status: string | null;
  messages: string;
  created_at: string;
  updated_at: string;
}

function rowToAgentSession(row: AgentSessionRow): AgentSessionRecord {
  return {
    id: row.id,
    title: row.title,
    sdkSessionId: row.sdk_session_id || undefined,
    userId: row.user_id || undefined,
    agentId: row.agent_id || undefined,
    sourceChannel: row.source_channel || undefined,
    sessionKey: row.session_key || undefined,
    channelName: row.channel_name || undefined,
    channelConversationId: row.channel_conversation_id || undefined,
    workDir: row.work_dir || undefined,
    executionStatus: row.execution_status || undefined,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export const agentSessionDb = {
  // 获取所有会话（按更新时间倒序）
  getAll(): AgentSessionRecord[] {
    const rows = db.query("SELECT * FROM agent_sessions ORDER BY updated_at DESC").all() as AgentSessionRow[];
    return rows.map(rowToAgentSession);
  },

  // 获取单个会话
  getById(id: string): AgentSessionRecord | null {
    const row = db.query("SELECT * FROM agent_sessions WHERE id = ?").get(id) as AgentSessionRow | null;
    return row ? rowToAgentSession(row) : null;
  },

  // 创建会话（消息不再存储在 session 中，统一使用 messages 表）
  create(session: Pick<AgentSessionRecord, "id" | "title"> & Partial<AgentSessionRecord>): AgentSessionRecord {
    const stmt = db.prepare(`
      INSERT INTO agent_sessions (
        id, title, sdk_session_id, user_id, agent_id, source_channel,
        session_key, channel_name, channel_conversation_id, work_dir, execution_status, messages
      )
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '[]')
    `);
    stmt.run(
      session.id,
      session.title || "新对话",
      session.sdkSessionId || null,
      session.userId || null,
      session.agentId || null,
      session.sourceChannel || "webui",
      session.sessionKey || null,
      session.channelName || null,
      session.channelConversationId || null,
      session.workDir || null,
      session.executionStatus || "idle",
    );
    return this.getById(session.id)!;
  },

  // 更新会话
  update(id: string, updates: Partial<AgentSessionRecord>): AgentSessionRecord | null {
    const existing = this.getById(id);
    if (!existing) return null;

    const fields: string[] = [];
    const values: (string | null)[] = [];

    if (updates.title !== undefined) {
      fields.push("title = ?");
      values.push(updates.title);
    }
    if (updates.sdkSessionId !== undefined) {
      fields.push("sdk_session_id = ?");
      values.push(updates.sdkSessionId || null);
    }
    if (updates.userId !== undefined) {
      fields.push("user_id = ?");
      values.push(updates.userId || null);
    }
    if (updates.agentId !== undefined) {
      fields.push("agent_id = ?");
      values.push(updates.agentId || null);
    }
    if (updates.sourceChannel !== undefined) {
      fields.push("source_channel = ?");
      values.push(updates.sourceChannel || null);
    }
    if (updates.sessionKey !== undefined) {
      fields.push("session_key = ?");
      values.push(updates.sessionKey || null);
    }
    if (updates.channelName !== undefined) {
      fields.push("channel_name = ?");
      values.push(updates.channelName || null);
    }
    if (updates.channelConversationId !== undefined) {
      fields.push("channel_conversation_id = ?");
      values.push(updates.channelConversationId || null);
    }
    if (updates.workDir !== undefined) {
      fields.push("work_dir = ?");
      values.push(updates.workDir || null);
    }
    if (updates.executionStatus !== undefined) {
      fields.push("execution_status = ?");
      values.push(updates.executionStatus || null);
    }
    // messages 不再写入 session，统一使用 messages 表

    // 即使没有显式字段更新，也刷新 updated_at
    fields.push("updated_at = CURRENT_TIMESTAMP");
    values.push(id);
    db.run(`UPDATE agent_sessions SET ${fields.join(", ")} WHERE id = ?`, values);

    return this.getById(id);
  },

  // 删除会话
  delete(id: string): boolean {
    const result = db.run("DELETE FROM agent_sessions WHERE id = ?", [id]);
    return result.changes > 0;
  },

  // [已废弃] addMessage 已移除，消息统一写入 messages 表
  // 使用 messagesDb.insert() 代替

  // 更新会话的 SDK session ID
  updateSdkSessionId(id: string, sdkSessionId: string): AgentSessionRecord | null {
    return this.update(id, { sdkSessionId });
  },

  // 按用户ID获取所有会话
  getByUserId(userId: string): AgentSessionRecord[] {
    const rows = db.query("SELECT * FROM agent_sessions WHERE user_id = ? ORDER BY updated_at DESC").all(userId) as AgentSessionRow[];
    return rows.map(rowToAgentSession);
  },

  // 按 sessionKey 查询会话（支持群聊共享会话：channel:conversationId）
  findBySessionKey(sessionKey: string, agentId?: string): AgentSessionRecord | null {
    if (!sessionKey) return null;
    if (agentId) {
      const row = db.query(
        "SELECT * FROM agent_sessions WHERE session_key = ? AND agent_id = ? ORDER BY updated_at DESC LIMIT 1"
      ).get(sessionKey, agentId) as AgentSessionRow | null;
      return row ? rowToAgentSession(row) : null;
    }
    const row = db.query(
      "SELECT * FROM agent_sessions WHERE session_key = ? ORDER BY updated_at DESC LIMIT 1"
    ).get(sessionKey) as AgentSessionRow | null;
    return row ? rowToAgentSession(row) : null;
  },

  // 按 channelConversationId + agentId 查找会话（迁移兜底：处理旧的无 session_key 的 session）
  findByConversationId(channelConversationId: string, agentId?: string): AgentSessionRecord | null {
    if (!channelConversationId) return null;
    if (agentId) {
      const row = db.query(
        "SELECT * FROM agent_sessions WHERE channel_conversation_id = ? AND agent_id = ? ORDER BY updated_at DESC LIMIT 1"
      ).get(channelConversationId, agentId) as AgentSessionRow | null;
      return row ? rowToAgentSession(row) : null;
    }
    const row = db.query(
      "SELECT * FROM agent_sessions WHERE channel_conversation_id = ? ORDER BY updated_at DESC LIMIT 1"
    ).get(channelConversationId) as AgentSessionRow | null;
    return row ? rowToAgentSession(row) : null;
  },

  // 按 agentId 获取所有会话
  getByAgentId(agentId: string): AgentSessionRecord[] {
    const rows = db.query("SELECT * FROM agent_sessions WHERE agent_id = ? ORDER BY updated_at DESC").all(agentId) as AgentSessionRow[];
    return rows.map(rowToAgentSession);
  },

  // 按过滤条件查询会话列表
  getFiltered(filters: { agentId?: string; channel?: string; userId?: string; limit?: number }): AgentSessionRecord[] {
    const conditions: string[] = [];
    const values: (string | number)[] = [];

    if (filters.agentId) {
      conditions.push("agent_id = ?");
      values.push(filters.agentId);
    }
    if (filters.channel) {
      conditions.push("source_channel = ?");
      values.push(filters.channel);
    }
    if (filters.userId) {
      conditions.push("user_id = ?");
      values.push(filters.userId);
    }

    const where = conditions.length > 0 ? `WHERE ${conditions.join(" AND ")}` : "";
    const limit = filters.limit || 50;
    const sql = `SELECT * FROM agent_sessions ${where} ORDER BY updated_at DESC LIMIT ?`;
    values.push(limit);

    const rows = db.query(sql).all(...values) as AgentSessionRow[];
    return rows.map(rowToAgentSession);
  },
};

// ==================== User CRUD ====================

export type ParticipantType = "human" | "agent";

export interface UserRecord {
  id: string;
  name: string;
  type: ParticipantType;
  avatarUrl?: string;
  metadata?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
}

interface UserRow {
  id: string;
  name: string;
  type: string;
  avatar_url: string | null;
  metadata: string;
  created_at: string;
  updated_at: string;
}

function rowToUser(row: UserRow): UserRecord {
  return {
    id: row.id,
    name: row.name,
    type: (row.type || "human") as ParticipantType,
    avatarUrl: row.avatar_url || undefined,
    metadata: JSON.parse(row.metadata || "{}"),
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export const userDb = {
  getAll(): UserRecord[] {
    const rows = db.query("SELECT * FROM users ORDER BY created_at DESC").all() as UserRow[];
    return rows.map(rowToUser);
  },

  getById(id: string): UserRecord | null {
    const row = db.query("SELECT * FROM users WHERE id = ?").get(id) as UserRow | null;
    return row ? rowToUser(row) : null;
  },

  create(user: Omit<UserRecord, "createdAt" | "updatedAt">): UserRecord {
    db.prepare(`
      INSERT INTO users (id, name, type, avatar_url, metadata)
      VALUES (?, ?, ?, ?, ?)
    `).run(user.id, user.name, user.type || "human", user.avatarUrl || null, JSON.stringify(user.metadata || {}));
    return this.getById(user.id)!;
  },

  // 按类型查询
  getByType(type: ParticipantType): UserRecord[] {
    const rows = db.query("SELECT * FROM users WHERE type = ? ORDER BY created_at DESC").all(type) as UserRow[];
    return rows.map(rowToUser);
  },

  update(id: string, updates: Partial<UserRecord>): UserRecord | null {
    const existing = this.getById(id);
    if (!existing) return null;

    const fields: string[] = [];
    const values: (string | null)[] = [];

    if (updates.name !== undefined) { fields.push("name = ?"); values.push(updates.name); }
    if (updates.type !== undefined) { fields.push("type = ?"); values.push(updates.type); }
    if (updates.avatarUrl !== undefined) { fields.push("avatar_url = ?"); values.push(updates.avatarUrl || null); }
    if (updates.metadata !== undefined) { fields.push("metadata = ?"); values.push(JSON.stringify(updates.metadata)); }

    if (fields.length > 0) {
      fields.push("updated_at = CURRENT_TIMESTAMP");
      values.push(id);
      db.run(`UPDATE users SET ${fields.join(", ")} WHERE id = ?`, values);
    }
    return this.getById(id);
  },

  delete(id: string): boolean {
    // Also clean up related data
    db.run("DELETE FROM user_channels WHERE user_id = ?", [id]);
    db.run("DELETE FROM user_memory WHERE user_id = ?", [id]);
    db.run("DELETE FROM user_memory_facts WHERE user_id = ?", [id]);
    db.run("DELETE FROM user_binding_codes WHERE user_id = ?", [id]);
    const result = db.run("DELETE FROM users WHERE id = ?", [id]);
    return result.changes > 0;
  },
};

// ==================== User Channel CRUD ====================

export interface UserChannelRecord {
  id: string;
  userId: string;
  channelType: string;
  channelUserId: string;
  displayName?: string;
  channelMeta?: Record<string, unknown>;
  createdAt?: string;
}

interface UserChannelRow {
  id: string;
  user_id: string;
  channel_type: string;
  channel_user_id: string;
  display_name: string | null;
  channel_meta: string;
  created_at: string;
}

function rowToUserChannel(row: UserChannelRow): UserChannelRecord {
  return {
    id: row.id,
    userId: row.user_id,
    channelType: row.channel_type,
    channelUserId: row.channel_user_id,
    displayName: row.display_name || undefined,
    channelMeta: JSON.parse(row.channel_meta || "{}"),
    createdAt: row.created_at,
  };
}

export const userChannelDb = {
  getByUserId(userId: string): UserChannelRecord[] {
    const rows = db.query("SELECT * FROM user_channels WHERE user_id = ?").all(userId) as UserChannelRow[];
    return rows.map(rowToUserChannel);
  },

  findByChannelUser(channelType: string, channelUserId: string): UserChannelRecord | null {
    const row = db.query(
      "SELECT * FROM user_channels WHERE channel_type = ? AND channel_user_id = ?"
    ).get(channelType, channelUserId) as UserChannelRow | null;
    return row ? rowToUserChannel(row) : null;
  },

  create(record: Omit<UserChannelRecord, "createdAt">): UserChannelRecord {
    db.prepare(`
      INSERT INTO user_channels (id, user_id, channel_type, channel_user_id, display_name, channel_meta)
      VALUES (?, ?, ?, ?, ?, ?)
    `).run(
      record.id, record.userId, record.channelType, record.channelUserId,
      record.displayName || null, JSON.stringify(record.channelMeta || {})
    );
    return this.findByChannelUser(record.channelType, record.channelUserId)!;
  },

  delete(id: string): boolean {
    const result = db.run("DELETE FROM user_channels WHERE id = ?", [id]);
    return result.changes > 0;
  },

  // 将渠道绑定从一个用户迁移到另一个用户（用于合并影子用户）
  transferToUser(fromUserId: string, toUserId: string): number {
    const result = db.run(
      "UPDATE user_channels SET user_id = ? WHERE user_id = ?",
      [toUserId, fromUserId]
    );
    return result.changes;
  },
};

// ==================== User Memory CRUD ====================

export interface UserMemoryRecord {
  id: string;
  userId: string;
  agentId?: string;
  summary: string;
  updatedAt?: string;
}

interface UserMemoryRow {
  id: string;
  user_id: string;
  agent_id: string;
  summary: string;
  updated_at: string;
}

function rowToUserMemory(row: UserMemoryRow): UserMemoryRecord {
  return { id: row.id, userId: row.user_id, agentId: row.agent_id || undefined, summary: row.summary, updatedAt: row.updated_at };
}

export const userMemoryDb = {
  /**
   * 按 agentId × userId 获取记忆摘要
   * agentId 为空时查全局记忆（兼容旧数据）
   */
  getByUserId(userId: string, agentId?: string): UserMemoryRecord | null {
    const aid = agentId || "";
    const row = db.query("SELECT * FROM user_memory WHERE user_id = ? AND agent_id = ?").get(userId, aid) as UserMemoryRow | null;
    return row ? rowToUserMemory(row) : null;
  },

  /**
   * 按 agentId × userId upsert 记忆摘要
   */
  upsert(userId: string, summary: string, agentId?: string): UserMemoryRecord {
    const aid = agentId || "";
    const id = crypto.randomUUID();
    db.run(`
      INSERT INTO user_memory (id, user_id, agent_id, summary, updated_at)
      VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
      ON CONFLICT(agent_id, user_id) DO UPDATE SET summary = ?, updated_at = CURRENT_TIMESTAMP
    `, [id, userId, aid, summary, summary]);
    return this.getByUserId(userId, agentId)!;
  },
};

// ==================== User Memory Facts CRUD ====================

export interface UserMemoryFactRecord {
  id: string;
  userId: string;
  agentId?: string;
  category: "preference" | "context" | "relationship" | "skill";
  fact: string;
  sourceChannel?: string;
  sourceSessionId?: string;
  createdAt?: string;
  expiresAt?: string;
}

interface UserMemoryFactRow {
  id: string;
  user_id: string;
  agent_id: string;
  category: string;
  fact: string;
  source_channel: string | null;
  source_session_id: string | null;
  created_at: string;
  expires_at: string | null;
}

function rowToMemoryFact(row: UserMemoryFactRow): UserMemoryFactRecord {
  return {
    id: row.id,
    userId: row.user_id,
    agentId: row.agent_id || undefined,
    category: row.category as UserMemoryFactRecord["category"],
    fact: row.fact,
    sourceChannel: row.source_channel || undefined,
    sourceSessionId: row.source_session_id || undefined,
    createdAt: row.created_at,
    expiresAt: row.expires_at || undefined,
  };
}

export const userMemoryFactDb = {
  /**
   * 按 agentId × userId 获取事实
   * agentId 为空时查全局事实（兼容旧数据）
   */
  getByUserId(userId: string, agentId?: string): UserMemoryFactRecord[] {
    const aid = agentId || "";
    const rows = db.query(
      "SELECT * FROM user_memory_facts WHERE user_id = ? AND agent_id = ? AND (expires_at IS NULL OR expires_at > datetime('now')) ORDER BY created_at DESC"
    ).all(userId, aid) as UserMemoryFactRow[];
    return rows.map(rowToMemoryFact);
  },

  getByCategory(userId: string, category: string, agentId?: string): UserMemoryFactRecord[] {
    const aid = agentId || "";
    const rows = db.query(
      "SELECT * FROM user_memory_facts WHERE user_id = ? AND agent_id = ? AND category = ? AND (expires_at IS NULL OR expires_at > datetime('now')) ORDER BY created_at DESC"
    ).all(userId, aid, category) as UserMemoryFactRow[];
    return rows.map(rowToMemoryFact);
  },

  create(fact: Omit<UserMemoryFactRecord, "createdAt">): UserMemoryFactRecord {
    const aid = fact.agentId || "";
    db.prepare(`
      INSERT INTO user_memory_facts (id, user_id, agent_id, category, fact, source_channel, source_session_id, expires_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      fact.id, fact.userId, aid, fact.category, fact.fact,
      fact.sourceChannel || null, fact.sourceSessionId || null, fact.expiresAt || null
    );
    return rowToMemoryFact(
      db.query("SELECT * FROM user_memory_facts WHERE id = ?").get(fact.id) as UserMemoryFactRow
    );
  },

  delete(id: string): boolean {
    const result = db.run("DELETE FROM user_memory_facts WHERE id = ?", [id]);
    return result.changes > 0;
  },

  // 将一个用户的事实迁移到另一个用户（合并时用）
  transferToUser(fromUserId: string, toUserId: string): number {
    const result = db.run(
      "UPDATE user_memory_facts SET user_id = ? WHERE user_id = ?",
      [toUserId, fromUserId]
    );
    return result.changes;
  },
};

// ==================== Binding Code CRUD ====================

export interface BindingCodeRecord {
  code: string;
  userId: string;
  targetChannel: string;
  expiresAt: string;
  usedAt?: string;
}

interface BindingCodeRow {
  code: string;
  user_id: string;
  target_channel: string;
  expires_at: string;
  used_at: string | null;
}

function rowToBindingCode(row: BindingCodeRow): BindingCodeRecord {
  return {
    code: row.code,
    userId: row.user_id,
    targetChannel: row.target_channel,
    expiresAt: row.expires_at,
    usedAt: row.used_at || undefined,
  };
}

export const bindingCodeDb = {
  create(record: Omit<BindingCodeRecord, "usedAt">): BindingCodeRecord {
    db.prepare(`
      INSERT INTO user_binding_codes (code, user_id, target_channel, expires_at)
      VALUES (?, ?, ?, ?)
    `).run(record.code, record.userId, record.targetChannel, record.expiresAt);
    return this.getByCode(record.code)!;
  },

  getByCode(code: string): BindingCodeRecord | null {
    const row = db.query("SELECT * FROM user_binding_codes WHERE code = ?").get(code) as BindingCodeRow | null;
    return row ? rowToBindingCode(row) : null;
  },

  markUsed(code: string): void {
    db.run("UPDATE user_binding_codes SET used_at = CURRENT_TIMESTAMP WHERE code = ?", [code]);
  },

  // 获取未使用且未过期的有效绑定码
  getValidCode(code: string): BindingCodeRecord | null {
    const row = db.query(
      "SELECT * FROM user_binding_codes WHERE code = ? AND used_at IS NULL AND expires_at > datetime('now')"
    ).get(code) as BindingCodeRow | null;
    return row ? rowToBindingCode(row) : null;
  },
};

// ==================== Messages CRUD ====================

export type MessageRole = "user" | "assistant" | "tool_result" | "system";
export type MessageStatus = "sending" | "sent" | "failed";

export interface MessageRecord {
  id: string;
  sessionId: string;
  role: MessageRole;
  content: string;
  messageType?: string;
  channel?: string;
  channelMessageId?: string;
  replyToMessageId?: string;
  mentions?: string[];
  channelMeta?: Record<string, unknown>;
  toolCalls?: Array<{
    id: string;
    tool: string;
    input: unknown;
    result?: unknown;
    status: "pending" | "running" | "success" | "error";
  }>;
  traceId?: string;
  initiator?: "user" | "agent" | "system";
  status?: MessageStatus;
  senderName?: string;
  senderId?: string;
  createdAt?: string;
}

interface MessageRow {
  id: string;
  session_id: string;
  role: string;
  content: string;
  message_type: string | null;
  channel: string | null;
  channel_message_id: string | null;
  reply_to_message_id: string | null;
  mentions: string | null;
  channel_meta: string | null;
  tool_calls: string | null;
  trace_id: string | null;
  initiator: string | null;
  status: string | null;
  sender_name: string | null;
  sender_id: string | null;
  created_at: string;
}

function rowToMessage(row: MessageRow): MessageRecord {
  return {
    id: row.id,
    sessionId: row.session_id,
    role: row.role as MessageRole,
    content: row.content,
    messageType: row.message_type || "text",
    channel: row.channel || undefined,
    channelMessageId: row.channel_message_id || undefined,
    replyToMessageId: row.reply_to_message_id || undefined,
    mentions: row.mentions ? JSON.parse(row.mentions) : undefined,
    channelMeta: row.channel_meta ? JSON.parse(row.channel_meta) : undefined,
    toolCalls: row.tool_calls ? JSON.parse(row.tool_calls) : undefined,
    traceId: row.trace_id || undefined,
    initiator: (row.initiator || undefined) as MessageRecord["initiator"],
    status: (row.status || "sent") as MessageStatus,
    senderName: row.sender_name || undefined,
    senderId: row.sender_id || undefined,
    createdAt: row.created_at,
  };
}

export const messagesDb = {
  /** 插入一条消息 */
  insert(msg: Omit<MessageRecord, "createdAt">): MessageRecord {
    db.prepare(`
      INSERT INTO messages (id, session_id, role, content, message_type, channel, channel_message_id, reply_to_message_id, mentions, channel_meta, tool_calls, trace_id, initiator, status, sender_name, sender_id)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      msg.id,
      msg.sessionId,
      msg.role,
      msg.content,
      msg.messageType || "text",
      msg.channel || null,
      msg.channelMessageId || null,
      msg.replyToMessageId || null,
      msg.mentions ? JSON.stringify(msg.mentions) : null,
      msg.channelMeta ? JSON.stringify(msg.channelMeta) : null,
      msg.toolCalls ? JSON.stringify(msg.toolCalls) : null,
      msg.traceId || null,
      msg.initiator || null,
      msg.status || "sent",
      msg.senderName || null,
      msg.senderId || null,
    );
    return this.getById(msg.id)!;
  },

  /** 获取单条消息 */
  getById(id: string): MessageRecord | null {
    const row = db.query("SELECT * FROM messages WHERE id = ?").get(id) as MessageRow | null;
    return row ? rowToMessage(row) : null;
  },

  /**
   * 按会话查询消息（分页，按时间正序）。limit=0 表示不限制条数。
   *
   * 当指定 limit > 0 时，返回的是最新的 limit 条消息（而非最旧的）。
   * 具体做法：先 ORDER BY DESC 取最近 N 条，再 reverse 还原时间正序，
   * 保证输入给模型的历史上下文始终是最近的对话，而非最早的。
   */
  getBySession(sessionId: string, options?: { limit?: number; before?: string }): MessageRecord[] {
    const limit = options?.limit ?? 50;
    if (options?.before) {
      const baseQuery = "SELECT * FROM messages WHERE session_id = ? AND created_at < ? ORDER BY created_at DESC";
      const rows = (limit > 0
        ? db.query(baseQuery + " LIMIT ?").all(sessionId, options.before, limit)
        : db.query(baseQuery).all(sessionId, options.before)
      ) as MessageRow[];
      return rows.reverse().map(rowToMessage);
    }
    if (limit > 0) {
      // 取最近 limit 条（DESC 取最新），再 reverse 为时间正序
      const rows = db.query(
        "SELECT * FROM messages WHERE session_id = ? ORDER BY created_at DESC LIMIT ?"
      ).all(sessionId, limit) as MessageRow[];
      return rows.reverse().map(rowToMessage);
    }
    // limit = 0：返回全部，时间正序
    const rows = db.query(
      "SELECT * FROM messages WHERE session_id = ? ORDER BY created_at ASC"
    ).all(sessionId) as MessageRow[];
    return rows.map(rowToMessage);
  },

  /** 按会话统计消息数（用于会话列表，避免加载大量正文） */
  countBySession(sessionId: string): number {
    const row = db.query("SELECT COUNT(*) as count FROM messages WHERE session_id = ?").get(sessionId) as { count: number } | null;
    return row?.count || 0;
  },

  /** 更新消息状态 */
  updateStatus(id: string, status: MessageStatus): void {
    db.run("UPDATE messages SET status = ? WHERE id = ?", [status, id]);
  },

  /** 更新消息的 toolCalls 字段 */
  updateToolCalls(id: string, toolCalls: MessageRecord["toolCalls"]): void {
    db.run("UPDATE messages SET tool_calls = ? WHERE id = ?", [
      toolCalls ? JSON.stringify(toolCalls) : null,
      id,
    ]);
  },

  /** 按渠道消息ID查找 */
  getByChannelMessageId(channelMessageId: string): MessageRecord | null {
    const row = db.query(
      "SELECT * FROM messages WHERE channel_message_id = ? LIMIT 1"
    ).get(channelMessageId) as MessageRow | null;
    return row ? rowToMessage(row) : null;
  },

  /** 按会话删除所有消息 */
  deleteBySession(sessionId: string): number {
    const result = db.run("DELETE FROM messages WHERE session_id = ?", [sessionId]);
    return result.changes;
  },
};

// ==================== Processed Messages (Dedup) ====================

export const processedMessageDb = {
  exists(channelMessageId: string): boolean {
    const row = db.query("SELECT 1 FROM processed_messages WHERE channel_message_id = ?").get(channelMessageId);
    return !!row;
  },

  mark(channelMessageId: string, channelType: string): void {
    db.run(
      "INSERT OR IGNORE INTO processed_messages (channel_message_id, channel_type) VALUES (?, ?)",
      [channelMessageId, channelType]
    );
  },

  // 清理旧记录（保留最近7天）
  cleanup(): number {
    const result = db.run(
      "DELETE FROM processed_messages WHERE processed_at < datetime('now', '-7 days')"
    );
    return result.changes;
  },
};

// ==================== Agent Config CRUD ====================

export interface AgentConfigRecord {
  id: string;
  userId: string;
  displayName: string;
  systemPrompt: string;
  modelId?: string;
  provider?: string;  // 直接指定 LLM 提供商（anthropic/openai/moonshot/zhipu）
  model?: string;     // 直接指定模型 ID（如 claude-sonnet-4-20250514）
  skills: string[];
  channels: Array<{ channelType: string; channelIdentifier: string }>;
  isActive: boolean;
  createdAt?: string;
  updatedAt?: string;
}

interface AgentConfigRow {
  id: string;
  user_id: string;
  display_name: string;
  system_prompt: string;
  model_id: string | null;
  provider: string | null;
  model: string | null;
  skills: string;
  channels: string;
  is_active: number;
  created_at: string;
  updated_at: string;
}

function rowToAgentConfig(row: AgentConfigRow): AgentConfigRecord {
  return {
    id: row.id,
    userId: row.user_id,
    displayName: row.display_name,
    systemPrompt: row.system_prompt || "",
    modelId: row.model_id || undefined,
    provider: row.provider || undefined,
    model: row.model || undefined,
    skills: JSON.parse(row.skills || "[]"),
    channels: JSON.parse(row.channels || "[]"),
    isActive: row.is_active === 1,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export const agentConfigDb = {
  getAll(): AgentConfigRecord[] {
    const rows = db.query("SELECT * FROM agent_configs ORDER BY created_at DESC").all() as AgentConfigRow[];
    return rows.map(rowToAgentConfig);
  },

  getActive(): AgentConfigRecord[] {
    const rows = db.query("SELECT * FROM agent_configs WHERE is_active = 1 ORDER BY created_at DESC").all() as AgentConfigRow[];
    return rows.map(rowToAgentConfig);
  },

  getById(id: string): AgentConfigRecord | null {
    const row = db.query("SELECT * FROM agent_configs WHERE id = ?").get(id) as AgentConfigRow | null;
    return row ? rowToAgentConfig(row) : null;
  },

  getByUserId(userId: string): AgentConfigRecord | null {
    const row = db.query("SELECT * FROM agent_configs WHERE user_id = ?").get(userId) as AgentConfigRow | null;
    return row ? rowToAgentConfig(row) : null;
  },

  create(config: Omit<AgentConfigRecord, "createdAt" | "updatedAt">): AgentConfigRecord {
    db.prepare(`
      INSERT INTO agent_configs (id, user_id, display_name, system_prompt, model_id, provider, model, skills, channels, is_active)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      config.id,
      config.userId,
      config.displayName,
      config.systemPrompt || "",
      config.modelId || null,
      config.provider || null,
      config.model || null,
      JSON.stringify(config.skills || []),
      JSON.stringify(config.channels || []),
      config.isActive ? 1 : 0
    );
    return this.getById(config.id)!;
  },

  update(id: string, updates: Partial<AgentConfigRecord>): AgentConfigRecord | null {
    const existing = this.getById(id);
    if (!existing) return null;

    const fields: string[] = [];
    const values: (string | number | null)[] = [];

    if (updates.displayName !== undefined) { fields.push("display_name = ?"); values.push(updates.displayName); }
    if (updates.systemPrompt !== undefined) { fields.push("system_prompt = ?"); values.push(updates.systemPrompt); }
    if (updates.modelId !== undefined) { fields.push("model_id = ?"); values.push(updates.modelId || null); }
    if (updates.provider !== undefined) { fields.push("provider = ?"); values.push(updates.provider || null); }
    if (updates.model !== undefined) { fields.push("model = ?"); values.push(updates.model || null); }
    if (updates.skills !== undefined) { fields.push("skills = ?"); values.push(JSON.stringify(updates.skills)); }
    if (updates.channels !== undefined) { fields.push("channels = ?"); values.push(JSON.stringify(updates.channels)); }
    if (updates.isActive !== undefined) { fields.push("is_active = ?"); values.push(updates.isActive ? 1 : 0); }

    if (fields.length > 0) {
      fields.push("updated_at = CURRENT_TIMESTAMP");
      values.push(id);
      db.run(`UPDATE agent_configs SET ${fields.join(", ")} WHERE id = ?`, values);
    }
    return this.getById(id);
  },

  delete(id: string): boolean {
    const config = this.getById(id);
    if (!config) return false;

    // 删除关联的 user（agent 身份）
    db.run("DELETE FROM agent_configs WHERE id = ?", [id]);
    userDb.delete(config.userId);
    return true;
  },

  // 按渠道查找绑定的 Agent
  getByChannel(channelType: string, channelIdentifier?: string): AgentConfigRecord[] {
    const allActive = this.getActive();
    return allActive.filter(agent => {
      return agent.channels.some(ch => {
        if (ch.channelType !== channelType) return false;
        if (channelIdentifier && ch.channelIdentifier !== channelIdentifier) return false;
        return true;
      });
    });
  },
};

// ==================== Agent Notes CRUD ====================

export interface AgentNoteRecord {
  id: string;
  agentId: string;
  category: "general" | "observation" | "plan" | "decision" | "learning";
  content: string;
  relatedSessionId?: string;
  relatedUserId?: string;
  createdAt?: string;
  updatedAt?: string;
}

interface AgentNoteRow {
  id: string;
  agent_id: string;
  category: string;
  content: string;
  related_session_id: string | null;
  related_user_id: string | null;
  created_at: string;
  updated_at: string;
}

function rowToAgentNote(row: AgentNoteRow): AgentNoteRecord {
  return {
    id: row.id,
    agentId: row.agent_id,
    category: row.category as AgentNoteRecord["category"],
    content: row.content,
    relatedSessionId: row.related_session_id || undefined,
    relatedUserId: row.related_user_id || undefined,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export const agentNoteDb = {
  getByAgentId(agentId: string, category?: string): AgentNoteRecord[] {
    if (category) {
      const rows = db.query(
        "SELECT * FROM agent_notes WHERE agent_id = ? AND category = ? ORDER BY updated_at DESC"
      ).all(agentId, category) as AgentNoteRow[];
      return rows.map(rowToAgentNote);
    }
    const rows = db.query(
      "SELECT * FROM agent_notes WHERE agent_id = ? ORDER BY updated_at DESC"
    ).all(agentId) as AgentNoteRow[];
    return rows.map(rowToAgentNote);
  },

  create(note: Omit<AgentNoteRecord, "createdAt" | "updatedAt">): AgentNoteRecord {
    db.prepare(`
      INSERT INTO agent_notes (id, agent_id, category, content, related_session_id, related_user_id)
      VALUES (?, ?, ?, ?, ?, ?)
    `).run(
      note.id, note.agentId, note.category, note.content,
      note.relatedSessionId || null, note.relatedUserId || null
    );
    return rowToAgentNote(
      db.query("SELECT * FROM agent_notes WHERE id = ?").get(note.id) as AgentNoteRow
    );
  },

  update(id: string, updates: Partial<Pick<AgentNoteRecord, "content" | "category">>): AgentNoteRecord | null {
    const fields: string[] = ["updated_at = CURRENT_TIMESTAMP"];
    const values: unknown[] = [];
    if (updates.content !== undefined) { fields.push("content = ?"); values.push(updates.content); }
    if (updates.category !== undefined) { fields.push("category = ?"); values.push(updates.category); }
    values.push(id);
    db.run(`UPDATE agent_notes SET ${fields.join(", ")} WHERE id = ?`, values);
    const row = db.query("SELECT * FROM agent_notes WHERE id = ?").get(id) as AgentNoteRow | null;
    return row ? rowToAgentNote(row) : null;
  },

  delete(id: string): boolean {
    const result = db.run("DELETE FROM agent_notes WHERE id = ?", [id]);
    return result.changes > 0;
  },
};

// ==================== Agent Tasks CRUD ====================

export type TaskStatus = "pending" | "in_progress" | "completed" | "failed" | "cancelled";
export type TaskPriority = "low" | "normal" | "high" | "urgent";

export interface AgentTaskRecord {
  id: string;
  agentId: string;
  title: string;
  description: string;
  status: TaskStatus;
  priority: TaskPriority;
  sourceChannel?: string;
  sourceSessionId?: string;
  assignedBy?: string;
  result?: string;
  createdAt?: string;
  updatedAt?: string;
  completedAt?: string;
}

interface AgentTaskRow {
  id: string;
  agent_id: string;
  title: string;
  description: string;
  status: string;
  priority: string;
  source_channel: string | null;
  source_session_id: string | null;
  assigned_by: string | null;
  result: string | null;
  created_at: string;
  updated_at: string;
  completed_at: string | null;
}

function rowToAgentTask(row: AgentTaskRow): AgentTaskRecord {
  return {
    id: row.id,
    agentId: row.agent_id,
    title: row.title,
    description: row.description,
    status: row.status as TaskStatus,
    priority: row.priority as TaskPriority,
    sourceChannel: row.source_channel || undefined,
    sourceSessionId: row.source_session_id || undefined,
    assignedBy: row.assigned_by || undefined,
    result: row.result || undefined,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
    completedAt: row.completed_at || undefined,
  };
}

export const agentTaskDb = {
  getByAgentId(agentId: string, status?: TaskStatus): AgentTaskRecord[] {
    if (status) {
      const rows = db.query(
        "SELECT * FROM agent_tasks WHERE agent_id = ? AND status = ? ORDER BY created_at DESC"
      ).all(agentId, status) as AgentTaskRow[];
      return rows.map(rowToAgentTask);
    }
    const rows = db.query(
      "SELECT * FROM agent_tasks WHERE agent_id = ? ORDER BY created_at DESC"
    ).all(agentId) as AgentTaskRow[];
    return rows.map(rowToAgentTask);
  },

  getById(id: string): AgentTaskRecord | null {
    const row = db.query("SELECT * FROM agent_tasks WHERE id = ?").get(id) as AgentTaskRow | null;
    return row ? rowToAgentTask(row) : null;
  },

  create(task: Omit<AgentTaskRecord, "createdAt" | "updatedAt" | "completedAt">): AgentTaskRecord {
    db.prepare(`
      INSERT INTO agent_tasks (id, agent_id, title, description, status, priority, source_channel, source_session_id, assigned_by, result)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      task.id, task.agentId, task.title, task.description || "",
      task.status || "pending", task.priority || "normal",
      task.sourceChannel || null, task.sourceSessionId || null,
      task.assignedBy || null, task.result || null
    );
    return rowToAgentTask(
      db.query("SELECT * FROM agent_tasks WHERE id = ?").get(task.id) as AgentTaskRow
    );
  },

  update(id: string, updates: Partial<Pick<AgentTaskRecord, "title" | "description" | "status" | "priority" | "result">>): AgentTaskRecord | null {
    const fields: string[] = ["updated_at = CURRENT_TIMESTAMP"];
    const values: unknown[] = [];
    if (updates.title !== undefined) { fields.push("title = ?"); values.push(updates.title); }
    if (updates.description !== undefined) { fields.push("description = ?"); values.push(updates.description); }
    if (updates.status !== undefined) {
      fields.push("status = ?"); values.push(updates.status);
      if (updates.status === "completed" || updates.status === "failed") {
        fields.push("completed_at = CURRENT_TIMESTAMP");
      }
    }
    if (updates.priority !== undefined) { fields.push("priority = ?"); values.push(updates.priority); }
    if (updates.result !== undefined) { fields.push("result = ?"); values.push(updates.result); }
    values.push(id);
    db.run(`UPDATE agent_tasks SET ${fields.join(", ")} WHERE id = ?`, values);
    const row = db.query("SELECT * FROM agent_tasks WHERE id = ?").get(id) as AgentTaskRow | null;
    return row ? rowToAgentTask(row) : null;
  },

  delete(id: string): boolean {
    db.run("DELETE FROM agent_artifacts WHERE task_id = ?", [id]);
    const result = db.run("DELETE FROM agent_tasks WHERE id = ?", [id]);
    return result.changes > 0;
  },
};

// ==================== Agent Artifacts CRUD ====================

export type ArtifactType = "file" | "code" | "document" | "image" | "data" | "other";

export interface AgentArtifactRecord {
  id: string;
  agentId: string;
  taskId?: string;
  type: ArtifactType;
  title: string;
  content?: string;
  filePath?: string;
  metadata: Record<string, unknown>;
  createdAt?: string;
}

interface AgentArtifactRow {
  id: string;
  agent_id: string;
  task_id: string | null;
  type: string;
  title: string;
  content: string | null;
  file_path: string | null;
  metadata: string;
  created_at: string;
}

function rowToAgentArtifact(row: AgentArtifactRow): AgentArtifactRecord {
  return {
    id: row.id,
    agentId: row.agent_id,
    taskId: row.task_id || undefined,
    type: row.type as ArtifactType,
    title: row.title,
    content: row.content || undefined,
    filePath: row.file_path || undefined,
    metadata: JSON.parse(row.metadata || "{}"),
    createdAt: row.created_at,
  };
}

export const agentArtifactDb = {
  getByAgentId(agentId: string): AgentArtifactRecord[] {
    const rows = db.query(
      "SELECT * FROM agent_artifacts WHERE agent_id = ? ORDER BY created_at DESC"
    ).all(agentId) as AgentArtifactRow[];
    return rows.map(rowToAgentArtifact);
  },

  getByTaskId(taskId: string): AgentArtifactRecord[] {
    const rows = db.query(
      "SELECT * FROM agent_artifacts WHERE task_id = ? ORDER BY created_at DESC"
    ).all(taskId) as AgentArtifactRow[];
    return rows.map(rowToAgentArtifact);
  },

  getById(id: string): AgentArtifactRecord | null {
    const row = db.query("SELECT * FROM agent_artifacts WHERE id = ?").get(id) as AgentArtifactRow | null;
    return row ? rowToAgentArtifact(row) : null;
  },

  create(artifact: Omit<AgentArtifactRecord, "createdAt">): AgentArtifactRecord {
    db.prepare(`
      INSERT INTO agent_artifacts (id, agent_id, task_id, type, title, content, file_path, metadata)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      artifact.id, artifact.agentId, artifact.taskId || null,
      artifact.type, artifact.title, artifact.content || null,
      artifact.filePath || null, JSON.stringify(artifact.metadata || {})
    );
    return rowToAgentArtifact(
      db.query("SELECT * FROM agent_artifacts WHERE id = ?").get(artifact.id) as AgentArtifactRow
    );
  },

  delete(id: string): boolean {
    const result = db.run("DELETE FROM agent_artifacts WHERE id = ?", [id]);
    return result.changes > 0;
  },
};

// ==================== Skills CRUD ====================

db.run(`
  CREATE TABLE IF NOT EXISTS skills (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    version INTEGER DEFAULT 1,
    type TEXT DEFAULT 'knowledge',
    enabled INTEGER DEFAULT 1,
    triggers TEXT DEFAULT '[]',
    tools TEXT DEFAULT '[]',
    readme TEXT DEFAULT '',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
  )
`);

db.run(`
  CREATE TABLE IF NOT EXISTS skill_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    skill_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    type TEXT DEFAULT 'knowledge',
    enabled INTEGER DEFAULT 1,
    triggers TEXT DEFAULT '[]',
    tools TEXT DEFAULT '[]',
    readme TEXT DEFAULT '',
    change_summary TEXT DEFAULT '',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(skill_id, version)
  )
`);
db.run(`CREATE INDEX IF NOT EXISTS idx_skill_versions_skill_id ON skill_versions(skill_id)`);

export type SkillType = "knowledge" | "action" | "hybrid";

export interface SkillToolDefinition {
  name: string;
  description: string;
  inputSchema: { type: "object"; properties: Record<string, unknown>; required?: string[] };
  executor: { type: "http" | "script" | "internal"; url?: string; method?: string; command?: string; handler?: string };
}

export interface SkillRecord {
  id: string;
  name: string;
  description: string;
  version: number;
  type: SkillType;
  enabled: boolean;
  triggers: string[];
  tools: SkillToolDefinition[];
  readme: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface SkillVersionRecord {
  id: number;
  skillId: string;
  version: number;
  name: string;
  description: string;
  type: SkillType;
  enabled: boolean;
  triggers: string[];
  tools: SkillToolDefinition[];
  readme: string;
  changeSummary: string;
  createdAt?: string;
}

interface SkillRow {
  id: string;
  name: string;
  description: string;
  version: number;
  type: string;
  enabled: number;
  triggers: string;
  tools: string;
  readme: string;
  created_at: string;
  updated_at: string;
}

interface SkillVersionRow {
  id: number;
  skill_id: string;
  version: number;
  name: string;
  description: string;
  type: string;
  enabled: number;
  triggers: string;
  tools: string;
  readme: string;
  change_summary: string;
  created_at: string;
}

function rowToSkill(row: SkillRow): SkillRecord {
  return {
    id: row.id,
    name: row.name,
    description: row.description,
    version: row.version,
    type: row.type as SkillType,
    enabled: row.enabled === 1,
    triggers: JSON.parse(row.triggers || "[]"),
    tools: JSON.parse(row.tools || "[]"),
    readme: row.readme || "",
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

function rowToSkillVersion(row: SkillVersionRow): SkillVersionRecord {
  return {
    id: row.id,
    skillId: row.skill_id,
    version: row.version,
    name: row.name,
    description: row.description,
    type: row.type as SkillType,
    enabled: row.enabled === 1,
    triggers: JSON.parse(row.triggers || "[]"),
    tools: JSON.parse(row.tools || "[]"),
    readme: row.readme || "",
    changeSummary: row.change_summary || "",
    createdAt: row.created_at,
  };
}

export const skillDb = {
  getAll(): SkillRecord[] {
    const rows = db.query("SELECT * FROM skills ORDER BY created_at").all() as SkillRow[];
    return rows.map(rowToSkill);
  },

  getEnabled(): SkillRecord[] {
    const rows = db.query("SELECT * FROM skills WHERE enabled = 1 ORDER BY created_at").all() as SkillRow[];
    return rows.map(rowToSkill);
  },

  getById(id: string): SkillRecord | null {
    const row = db.query("SELECT * FROM skills WHERE id = ?").get(id) as SkillRow | null;
    return row ? rowToSkill(row) : null;
  },

  create(skill: { id: string; name: string; description: string; type: SkillType; enabled?: boolean; triggers?: string[]; tools?: SkillToolDefinition[]; readme?: string }): SkillRecord {
    db.prepare(`
      INSERT INTO skills (id, name, description, version, type, enabled, triggers, tools, readme)
      VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?)
    `).run(
      skill.id,
      skill.name,
      skill.description,
      skill.type,
      (skill.enabled ?? true) ? 1 : 0,
      JSON.stringify(skill.triggers || []),
      JSON.stringify(skill.tools || []),
      skill.readme || "",
    );

    // Save as version 1
    db.prepare(`
      INSERT INTO skill_versions (skill_id, version, name, description, type, enabled, triggers, tools, readme, change_summary)
      VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, '初始版本')
    `).run(
      skill.id, skill.name, skill.description, skill.type,
      (skill.enabled ?? true) ? 1 : 0,
      JSON.stringify(skill.triggers || []),
      JSON.stringify(skill.tools || []),
      skill.readme || "",
    );

    return this.getById(skill.id)!;
  },

  update(id: string, updates: Partial<Pick<SkillRecord, "name" | "description" | "type" | "triggers" | "tools" | "readme">>, changeSummary?: string): SkillRecord | null {
    const existing = this.getById(id);
    if (!existing) return null;

    const newVersion = existing.version + 1;
    const fields: string[] = ["version = ?", "updated_at = CURRENT_TIMESTAMP"];
    const values: (string | number | null)[] = [newVersion];

    if (updates.name !== undefined) { fields.push("name = ?"); values.push(updates.name); }
    if (updates.description !== undefined) { fields.push("description = ?"); values.push(updates.description); }
    if (updates.type !== undefined) { fields.push("type = ?"); values.push(updates.type); }
    if (updates.triggers !== undefined) { fields.push("triggers = ?"); values.push(JSON.stringify(updates.triggers)); }
    if (updates.tools !== undefined) { fields.push("tools = ?"); values.push(JSON.stringify(updates.tools)); }
    if (updates.readme !== undefined) { fields.push("readme = ?"); values.push(updates.readme); }

    values.push(id);
    db.run(`UPDATE skills SET ${fields.join(", ")} WHERE id = ?`, values);

    // Snapshot the new state into skill_versions
    const updated = this.getById(id)!;
    db.prepare(`
      INSERT INTO skill_versions (skill_id, version, name, description, type, enabled, triggers, tools, readme, change_summary)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      id, newVersion, updated.name, updated.description, updated.type,
      updated.enabled ? 1 : 0,
      JSON.stringify(updated.triggers),
      JSON.stringify(updated.tools),
      updated.readme,
      changeSummary || "",
    );

    return updated;
  },

  delete(id: string): boolean {
    db.run("DELETE FROM skill_versions WHERE skill_id = ?", [id]);
    const result = db.run("DELETE FROM skills WHERE id = ?", [id]);
    return result.changes > 0;
  },

  setEnabled(id: string, enabled: boolean): SkillRecord | null {
    const existing = this.getById(id);
    if (!existing) return null;
    db.run("UPDATE skills SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", [enabled ? 1 : 0, id]);
    return this.getById(id);
  },

  getVersions(skillId: string): SkillVersionRecord[] {
    const rows = db.query(
      "SELECT * FROM skill_versions WHERE skill_id = ? ORDER BY version DESC"
    ).all(skillId) as SkillVersionRow[];
    return rows.map(rowToSkillVersion);
  },

  getVersion(skillId: string, version: number): SkillVersionRecord | null {
    const row = db.query(
      "SELECT * FROM skill_versions WHERE skill_id = ? AND version = ?"
    ).get(skillId, version) as SkillVersionRow | null;
    return row ? rowToSkillVersion(row) : null;
  },

  restoreVersion(skillId: string, version: number): SkillRecord | null {
    const ver = this.getVersion(skillId, version);
    if (!ver) return null;
    return this.update(skillId, {
      name: ver.name,
      description: ver.description,
      type: ver.type,
      triggers: ver.triggers,
      tools: ver.tools,
      readme: ver.readme,
    }, `回滚到版本 ${version}`);
  },

  /** Bulk insert for migration (skips version auto-increment) */
  bulkImport(skill: { id: string; name: string; description: string; version: number; type: SkillType; enabled: boolean; triggers: string[]; tools: SkillToolDefinition[]; readme: string }): SkillRecord {
    db.prepare(`
      INSERT OR REPLACE INTO skills (id, name, description, version, type, enabled, triggers, tools, readme)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      skill.id, skill.name, skill.description, skill.version, skill.type,
      skill.enabled ? 1 : 0,
      JSON.stringify(skill.triggers),
      JSON.stringify(skill.tools),
      skill.readme,
    );
    db.prepare(`
      INSERT OR REPLACE INTO skill_versions (skill_id, version, name, description, type, enabled, triggers, tools, readme, change_summary)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '从文件系统迁移')
    `).run(
      skill.id, skill.version, skill.name, skill.description, skill.type,
      skill.enabled ? 1 : 0,
      JSON.stringify(skill.triggers),
      JSON.stringify(skill.tools),
      skill.readme,
    );
    return this.getById(skill.id)!;
  },
};

// ==================== Default Agent Initialization ====================

/**
 * 确保系统中存在至少一个默认 Agent
 * 在首次启动或迁移时自动创建
 */
export function initializeDefaultAgent(): AgentConfigRecord {
  const DEFAULT_AGENT_USER_ID = "default-agent";
  const DEFAULT_AGENT_CONFIG_ID = "default-agent-config";

  // 检查是否已存在
  const existing = agentConfigDb.getById(DEFAULT_AGENT_CONFIG_ID);
  if (existing) {
    // 迁移：如果旧的默认 Agent 没有 provider/model，补上默认值
    if (!existing.provider || !existing.model) {
      agentConfigDb.update(DEFAULT_AGENT_CONFIG_ID, {
        provider: existing.provider || "anthropic",
        model: existing.model || "claude-sonnet-4-20250514",
      });
      return agentConfigDb.getById(DEFAULT_AGENT_CONFIG_ID)!;
    }
    return existing;
  }

  // 创建 Agent 的 user 身份（如果不存在）
  if (!userDb.getById(DEFAULT_AGENT_USER_ID)) {
    userDb.create({
      id: DEFAULT_AGENT_USER_ID,
      name: "Ouroboros",
      type: "agent",
      avatarUrl: undefined,
      metadata: { isDefault: true },
    });
  }

  // 创建 Agent 配置（直接指定 provider + model）
  const config = agentConfigDb.create({
    id: DEFAULT_AGENT_CONFIG_ID,
    userId: DEFAULT_AGENT_USER_ID,
    displayName: "Ouroboros",
    systemPrompt: "你是 Ouroboros，一个友好、聪明、高效的 AI 助手。你善于理解用户意图，提供有用的回答。",
    provider: "anthropic",
    model: "claude-sonnet-4-20250514",
    skills: [],
    channels: [
      { channelType: "webui", channelIdentifier: "*" },
      { channelType: "feishu", channelIdentifier: "*" },
      { channelType: "qiwei", channelIdentifier: "*" },
    ],
    isActive: true,
  });

  console.log("🤖 Default agent initialized: Ouroboros");
  return config;
}

// 导出数据库实例（用于高级操作）
export { db };
