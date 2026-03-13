package storage

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// Init opens (or creates) the SQLite database and runs schema migrations.
func Init(dbPath string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	// SQLite is single-writer; WAL mode allows concurrent readers.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("exec %q: %w", p, err)
		}
	}

	if err := runSchema(db); err != nil {
		return fmt.Errorf("run schema: %w", err)
	}

	DB = db
	log.Printf("📦 Database initialized: %s", dbPath)
	return nil
}

func runSchema(db *sql.DB) error {
	stmts := []string{
		// Settings k/v store
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,

		// Agent configs
		`CREATE TABLE IF NOT EXISTS agent_configs (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			system_prompt TEXT DEFAULT '',
			model_id TEXT,
			provider TEXT,
			model TEXT,
			skills TEXT DEFAULT '[]',
			channels TEXT DEFAULT '[]',
			is_active INTEGER DEFAULT 1,
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,

		// Agent sessions
		`CREATE TABLE IF NOT EXISTS agent_sessions (
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
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,

		// Indexes for agent_sessions
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent ON agent_sessions(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_exec_status ON agent_sessions(execution_status)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_conv_id ON agent_sessions(channel_conversation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_session_key ON agent_sessions(session_key)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent_key ON agent_sessions(agent_id, session_key)`,

		// Users
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT 'human',
			avatar_url TEXT,
			metadata TEXT DEFAULT '{}',
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,

		// User-channel bindings
		`CREATE TABLE IF NOT EXISTS user_channels (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			channel_type TEXT NOT NULL,
			channel_user_id TEXT NOT NULL,
			display_name TEXT,
			channel_meta TEXT DEFAULT '{}',
			created_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_channels_unique ON user_channels(channel_type, channel_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_channels_user ON user_channels(user_id)`,

		// User memory
		`CREATE TABLE IF NOT EXISTS user_memory (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			summary TEXT DEFAULT '',
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_memory_agent_user ON user_memory(agent_id, user_id)`,

		// User memory facts
		`CREATE TABLE IF NOT EXISTS user_memory_facts (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL,
			fact TEXT NOT NULL,
			source_channel TEXT,
			source_session_id TEXT,
			created_at INTEGER NOT NULL DEFAULT 0,
			expires_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_facts_agent_user ON user_memory_facts(agent_id, user_id)`,

		// Message deduplication
		`CREATE TABLE IF NOT EXISTS processed_messages (
			channel_message_id TEXT PRIMARY KEY,
			channel_type TEXT NOT NULL,
			processed_at INTEGER NOT NULL DEFAULT 0
		)`,

		// Messages (per-session, append-only)
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			message_type TEXT DEFAULT 'text',
			channel TEXT,
			channel_message_id TEXT,
			reply_to_message_id TEXT,
			tool_calls TEXT,
			trace_id TEXT,
			initiator TEXT,
			sender_name TEXT,
			sender_id TEXT,
			attachments_json TEXT DEFAULT '[]',
			status TEXT DEFAULT 'sent',
			created_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_created ON messages(created_at)`,

		// LLM models (admin config)
		`CREATE TABLE IF NOT EXISTS models (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			enabled INTEGER DEFAULT 1,
			api_key TEXT,
			base_url TEXT,
			model TEXT NOT NULL,
			max_tokens INTEGER DEFAULT 4096,
			temperature REAL DEFAULT 0.7,
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,

		// Skills storage has moved to GitHub (see github.Store).
		// The local skills table is kept only for backward compatibility
		// with older deployments; runtime reads come from GitHub exclusively.
		`CREATE TABLE IF NOT EXISTS skills (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			version TEXT DEFAULT '1.0.0',
			type TEXT DEFAULT 'knowledge',
			enabled INTEGER DEFAULT 1,
			triggers TEXT DEFAULT '[]',
			tools TEXT DEFAULT '[]',
			readme TEXT DEFAULT '',
			metadata TEXT DEFAULT '{}',
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec schema stmt: %w\nSQL: %s", err, stmt[:min(80, len(stmt))])
		}
	}

	// Incremental migrations — silently ignore "duplicate column" errors.
	migrations := []string{
		`ALTER TABLE agent_sessions ADD COLUMN user_id TEXT`,
		`ALTER TABLE agent_sessions ADD COLUMN agent_id TEXT`,
		`ALTER TABLE agent_sessions ADD COLUMN execution_status TEXT DEFAULT 'idle'`,
		`ALTER TABLE agent_sessions ADD COLUMN channel_name TEXT`,
		`ALTER TABLE agent_sessions ADD COLUMN channel_conversation_id TEXT`,
		`ALTER TABLE agent_sessions ADD COLUMN session_key TEXT`,
		`ALTER TABLE agent_sessions ADD COLUMN work_dir TEXT`,
		`ALTER TABLE agent_sessions ADD COLUMN context TEXT DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN initiator TEXT`,
		`ALTER TABLE messages ADD COLUMN sender_name TEXT`,
		`ALTER TABLE messages ADD COLUMN sender_id TEXT`,
		`ALTER TABLE messages ADD COLUMN attachments_json TEXT DEFAULT '[]'`,
		`ALTER TABLE agent_configs ADD COLUMN user_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agent_configs ADD COLUMN provider TEXT`,
		`ALTER TABLE agent_configs ADD COLUMN model TEXT`,
		`ALTER TABLE users ADD COLUMN type TEXT NOT NULL DEFAULT 'human'`,
		`ALTER TABLE user_memory ADD COLUMN agent_id TEXT DEFAULT ''`,
		`ALTER TABLE user_memory_facts ADD COLUMN agent_id TEXT DEFAULT ''`,
		`ALTER TABLE skills ADD COLUMN metadata TEXT DEFAULT '{}'`,

		// Context compaction tracking (031)
		`CREATE TABLE IF NOT EXISTS context_compactions (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			summary TEXT NOT NULL,
			archived_before_time INTEGER NOT NULL,
			archived_message_count INTEGER,
			token_count_before INTEGER,
			token_count_after INTEGER,
			compact_model TEXT,
			created_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_compactions_session ON context_compactions(session_id)`,

		// Delayed tasks for proactive agent capability
		`CREATE TABLE IF NOT EXISTS delayed_tasks (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			channel TEXT NOT NULL DEFAULT '',
			channel_user_id TEXT NOT NULL DEFAULT '',
			channel_conversation_id TEXT NOT NULL DEFAULT '',
			task TEXT NOT NULL,
			execute_at INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_delayed_tasks_status_time ON delayed_tasks(status, execute_at)`,

		// Session-level memory facts (session-memory-facts)
		`CREATE TABLE IF NOT EXISTS session_facts (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			fact TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT 'general',
			created_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_session_facts_session ON session_facts(session_id)`,
	}
	for _, m := range migrations {
		db.Exec(m) // nolint: ignore "duplicate column" / "already exists" errors
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
