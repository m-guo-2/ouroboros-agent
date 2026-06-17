// Package storage holds the agent's persistence layer.
//
// The backend is MySQL 8.0. Schema changes go through goose migrations
// embedded under storage/migrations. Two globals are exported:
//
//	storage.DB  - *sql.DB the whole agent uses
//	storage.Init(cfg) - opens the pool, runs goose.Up
//
// Soft-delete semantics, timestamp semantics, and SQL dialect conventions
// are documented in openspec/changes/migrate-sqlite-to-mysql.
package storage

import "database/sql"

// DB is the process-wide MySQL connection pool, populated by Init.
// Nil until Init succeeds; repo helpers panic-safe because init failure
// is fatal at startup.
var DB *sql.DB

// Init connects to MySQL and applies pending migrations. Returns an
// error; callers log.Fatal on failure because no SQLite fallback
// exists. See InitMySQL in mysql.go for the actual implementation.
func Init(cfg MySQLConfig) error {
	return InitMySQL(cfg)
}

// seedDefaultModels inserts baseline provider rows if absent. Kept
// runtime-side (not in goose) so the catalog can evolve without bumping
// a migration version for every model change.
func seedDefaultModels() error {
	type modelSeed struct {
		id, name, provider, model string
	}
	seeds := []modelSeed{
		{"model-claude-sonnet", "Claude Sonnet 4.5", "claude", "claude-sonnet-4-5"},
		{"model-claude-haiku", "Claude 3.5 Haiku", "claude", "claude-3-5-haiku-20241022"},
		{"model-gpt4o", "GPT-4o", "openai", "gpt-4o"},
		{"model-gpt4o-mini", "GPT-4o Mini", "openai", "gpt-4o-mini"},
		{"model-kimi", "Moonshot v1", "kimi", "moonshot-v1-auto"},
		{"model-glm4", "GLM-4 Plus", "glm", "glm-4-plus"},
		{"model-deepseek-chat", "DeepSeek Chat (V3)", "deepseek", "deepseek-chat"},
		{"model-deepseek-reasoner", "DeepSeek Reasoner (R1)", "deepseek", "deepseek-reasoner"},
		{"model-doubao-pro", "Doubao 1.5 Pro", "volcengine", "doubao-1-5-pro-256k"},
		{"model-doubao-seedream-lite", "Doubao Seedream 5.0 Lite", "volcengine", "doubao-seedream-5-0-lite"},
	}
	for _, s := range seeds {
		_, err := DB.Exec(
			`INSERT IGNORE INTO models
			 (id, name, provider, enabled, model, max_tokens, temperature, created_at, updated_at)
			 VALUES (?, ?, ?, 1, ?, 4096, 0.7, 0, 0)`,
			s.id, s.name, s.provider, s.model,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// SeedDefaults runs any post-migration seeds. Called by main once DB is
// ready; tests may call directly.
func SeedDefaults() error {
	return seedDefaultModels()
}
