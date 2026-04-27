// Package migrator: agent-scope table migrations.
//
// Each tableMigration here is hand-written to bridge two real schemas:
//
//   - source: the production SQLite catalog (see comments above each
//     selectAt for the actual column list dumped from sqlite_master);
//   - target: agent/internal/storage/migrations/00001_init.sql.
//
// Conversions performed:
//   - TEXT timestamps (RFC3339 or numeric-as-string) → BIGINT epoch ms via
//     parseTextTimestampMs;
//   - JSON columns split into the new child tables defined in the target
//     schema (see agent_split.go);
//   - dead columns (e.g. agent_sessions.messages) dropped silently;
//   - soft-delete columns (`deleted_at`) injected as 0.
package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

func agentTables() []tableMigration {
	return []tableMigration{
		settingsTable(),
		usersTable(),
		userChannelsTable(),
		modelsTable(),
		skillsTable(),
		agentConfigsTable(),
		agentPersonasTable(),
		channelGroupsTable(),
		groupPersonaAssignmentsTable(),
		agentSessionsTable(),
		sessionActiveSkillsTable(),
		sessionEventsTable(),
		sessionFactsTable(),
		messagesTable(),
		contextCompactionsTable(),
		contextCompactionArchivesTable(),
		processedMessagesTable(),
		delayedTasksTable(),
		userMemoryTable(),
		userMemoryFactsTable(),
	}
}

// ---------------------------------------------------------------
// settings: (key, value, updated_at:TEXT)
// ---------------------------------------------------------------

func settingsTable() tableMigration {
	return tableMigration{
		source: "settings", target: "settings",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `SELECT key, value, COALESCE(updated_at, '') FROM settings`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var k, updatedRaw string
			var v sql.NullString
			if err := row.Scan(&k, &v, &updatedRaw); err != nil {
				return 0, err
			}
			updatedAt, err := parseTextTimestampMs(updatedRaw)
			if err != nil {
				return 0, fmt.Errorf("settings %q updated_at: %w", k, err)
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO settings (`key`, value, updated_at) VALUES (?, ?, ?)",
				k, v, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// users: (id, name, type, avatar_url, metadata, created_at:INT, updated_at:INT)
// ---------------------------------------------------------------

func usersTable() tableMigration {
	return tableMigration{
		source: "users", target: "users",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id,
				       COALESCE(name, ''),
				       COALESCE(type, 'human'),
				       COALESCE(avatar_url, ''),
				       COALESCE(metadata, '{}'),
				       COALESCE(created_at, 0),
				       COALESCE(updated_at, 0)
				FROM users`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, name, typ, avatar, meta string
			var createdAt, updatedAt int64
			if err := row.Scan(&id, &name, &typ, &avatar, &meta, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			meta = defaultJSONObject(meta)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO users
				(id, name, type, avatar_url, metadata, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
				id, name, typ, avatar, meta, createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// user_channels: (id, user_id, channel_type, channel_user_id,
//                 display_name, channel_meta, created_at:INT)
// ---------------------------------------------------------------

func userChannelsTable() tableMigration {
	return tableMigration{
		source: "user_channels", target: "user_channels",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, user_id, channel_type, channel_user_id,
				       COALESCE(display_name, ''),
				       COALESCE(channel_meta, '{}'),
				       COALESCE(created_at, 0)
				FROM user_channels`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, userID, channelType, channelUserID, displayName, meta string
			var createdAt int64
			if err := row.Scan(&id, &userID, &channelType, &channelUserID, &displayName, &meta, &createdAt); err != nil {
				return 0, err
			}
			meta = defaultJSONObject(meta)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_channels
				(id, user_id, channel_type, channel_user_id, display_name, channel_meta, created_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
				id, userID, channelType, channelUserID, displayName, meta, createdAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// models: legacy schema declared TEXT timestamps but stored ms strings.
// ---------------------------------------------------------------

func modelsTable() tableMigration {
	return tableMigration{
		source: "models", target: "models",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, name, provider,
				       COALESCE(enabled, 1),
				       COALESCE(api_key, ''),
				       COALESCE(base_url, ''),
				       COALESCE(model, ''),
				       COALESCE(max_tokens, 4096),
				       COALESCE(temperature, 0.7),
				       COALESCE(created_at, ''),
				       COALESCE(updated_at, '')
				FROM models`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, name, provider, apiKey, baseURL, model string
			var enabled, maxTokens int64
			var temperature float64
			var createdAtRaw, updatedAtRaw string
			if err := row.Scan(&id, &name, &provider, &enabled, &apiKey, &baseURL, &model,
				&maxTokens, &temperature, &createdAtRaw, &updatedAtRaw); err != nil {
				return 0, err
			}
			createdAt, err := parseTextTimestampMs(createdAtRaw)
			if err != nil {
				return 0, fmt.Errorf("model %s created_at: %w", id, err)
			}
			updatedAt, err := parseTextTimestampMs(updatedAtRaw)
			if err != nil {
				return 0, fmt.Errorf("model %s updated_at: %w", id, err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO models
				(id, name, provider, enabled, api_key, base_url, model,
				 max_tokens, temperature, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				id, name, provider, enabled, apiKey, baseURL, model,
				maxTokens, temperature, createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// skills: source has version:INT and TEXT timestamps. triggers/tools
// JSON columns exist on the source but the target moved to dedicated
// child tables that the runtime now seeds from snapshots; we leave
// them empty.
// ---------------------------------------------------------------

func skillsTable() tableMigration {
	return tableMigration{
		source: "skills", target: "skills",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, name,
				       COALESCE(description, ''),
				       COALESCE(version, 1),
				       COALESCE(type, 'knowledge'),
				       COALESCE(enabled, 1),
				       COALESCE(readme, ''),
				       COALESCE(metadata, '{}'),
				       COALESCE(created_at, ''),
				       COALESCE(updated_at, '')
				FROM skills`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, name, desc, typ, readme, meta string
			var version, enabled int64
			var createdAtRaw, updatedAtRaw string
			if err := row.Scan(&id, &name, &desc, &version, &typ, &enabled, &readme, &meta, &createdAtRaw, &updatedAtRaw); err != nil {
				return 0, err
			}
			createdAt, err := parseTextTimestampMs(createdAtRaw)
			if err != nil {
				return 0, fmt.Errorf("skill %s created_at: %w", id, err)
			}
			updatedAt, err := parseTextTimestampMs(updatedAtRaw)
			if err != nil {
				return 0, fmt.Errorf("skill %s updated_at: %w", id, err)
			}
			versionStr := strconv.FormatInt(version, 10)
			meta = defaultJSONObject(meta)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO skills
				(id, name, description, version, type, enabled, readme, metadata,
				 created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				id, name, desc, versionStr, typ, enabled, readme, meta,
				createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// agent_configs: parent + 5 split children. Source timestamps are INT.
// ---------------------------------------------------------------

func agentConfigsTable() tableMigration {
	children := []string{
		"agent_skill_bindings",
		"agent_hooks",
		"agent_channels",
		"agent_subagent_models",
		"agent_subagent_skill_bindings",
	}
	return tableMigration{
		source: "agent_configs", target: "agent_configs",
		children: children,
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id,
				       COALESCE(user_id, ''),
				       COALESCE(display_name, ''),
				       COALESCE(system_prompt, ''),
				       COALESCE(model_id, ''),
				       COALESCE(provider, ''),
				       COALESCE(model, ''),
				       COALESCE(is_active, 1),
				       COALESCE(skills, '[]'),
				       COALESCE(channels, '[]'),
				       COALESCE(hooks, '[]'),
				       COALESCE(subagent_models, '{}'),
				       COALESCE(subagent_skills, '{}'),
				       COALESCE(created_at, 0),
				       COALESCE(updated_at, 0)
				FROM agent_configs`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, childCounts map[string]int64) (int64, error) {
			var id, userID, name, prompt, modelID, provider, model string
			var isActive int64
			var skills, channels, hooks, subModels, subSkills string
			var createdAt, updatedAt int64
			if err := row.Scan(&id, &userID, &name, &prompt, &modelID, &provider, &model, &isActive,
				&skills, &channels, &hooks, &subModels, &subSkills, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO agent_configs
				(id, user_id, display_name, system_prompt, model_id, provider, model,
				 is_active, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				id, userID, name, prompt, modelID, provider, model, isActive, createdAt, updatedAt); err != nil {
				return 0, err
			}

			n, err := insertAgentSkillBindings(ctx, tx, id, skills, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["agent_skill_bindings"] += n

			n, err = insertAgentChannels(ctx, tx, id, channels, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["agent_channels"] += n

			n, err = insertAgentHooks(ctx, tx, id, hooks, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["agent_hooks"] += n

			n, err = insertAgentSubagentModels(ctx, tx, id, subModels, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["agent_subagent_models"] += n

			n, err = insertAgentSubagentSkills(ctx, tx, id, subSkills, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["agent_subagent_skill_bindings"] += n

			return 1, nil
		},
		verifyChildren: func(ctx context.Context, db *sql.DB) (map[string]int64, error) {
			out := map[string]int64{}
			rows, err := db.QueryContext(ctx, `
				SELECT COALESCE(skills, '[]'),
				       COALESCE(channels, '[]'),
				       COALESCE(hooks, '[]'),
				       COALESCE(subagent_models, '{}'),
				       COALESCE(subagent_skills, '{}')
				FROM agent_configs`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			for rows.Next() {
				var skills, channels, hooks, subModels, subSkills string
				if err := rows.Scan(&skills, &channels, &hooks, &subModels, &subSkills); err != nil {
					return nil, err
				}
				out["agent_skill_bindings"] += jsonArrayCount(skills)
				out["agent_channels"] += jsonArrayCount(channels)
				out["agent_hooks"] += countAgentHookRows(hooks)
				out["agent_subagent_models"] += jsonObjectCount(subModels)
				out["agent_subagent_skill_bindings"] += jsonObjectFlatCount(subSkills)
			}
			return out, rows.Err()
		},
	}
}

// ---------------------------------------------------------------
// agent_personas: parent + 3 split children. Source timestamps are TEXT.
// ---------------------------------------------------------------

func agentPersonasTable() tableMigration {
	children := []string{
		"persona_skill_bindings",
		"persona_subagent_models",
		"persona_subagent_skill_bindings",
	}
	return tableMigration{
		source: "agent_personas", target: "agent_personas",
		children: children,
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, agent_id,
				       COALESCE(display_name, ''),
				       COALESCE(system_prompt, ''),
				       COALESCE(provider, ''),
				       COALESCE(model, ''),
				       COALESCE(skills, '[]'),
				       COALESCE(subagent_models, '{}'),
				       COALESCE(subagent_skills, '{}'),
				       COALESCE(created_at, ''),
				       COALESCE(updated_at, '')
				FROM agent_personas`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, childCounts map[string]int64) (int64, error) {
			var id, agentID, name, prompt, provider, model string
			var skills, subModels, subSkills string
			var createdAtRaw, updatedAtRaw string
			if err := row.Scan(&id, &agentID, &name, &prompt, &provider, &model,
				&skills, &subModels, &subSkills, &createdAtRaw, &updatedAtRaw); err != nil {
				return 0, err
			}
			createdAt, err := parseTextTimestampMs(createdAtRaw)
			if err != nil {
				return 0, fmt.Errorf("persona %s created_at %q: %w", id, createdAtRaw, err)
			}
			updatedAt, err := parseTextTimestampMs(updatedAtRaw)
			if err != nil {
				return 0, fmt.Errorf("persona %s updated_at %q: %w", id, updatedAtRaw, err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO agent_personas
				(id, agent_id, display_name, system_prompt, provider, model,
				 created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				id, agentID, name, prompt, provider, model, createdAt, updatedAt); err != nil {
				return 0, err
			}

			n, err := insertPersonaSkillBindings(ctx, tx, id, skills, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["persona_skill_bindings"] += n

			n, err = insertPersonaSubagentModels(ctx, tx, id, subModels, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["persona_subagent_models"] += n

			n, err = insertPersonaSubagentSkills(ctx, tx, id, subSkills, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["persona_subagent_skill_bindings"] += n

			return 1, nil
		},
		verifyChildren: func(ctx context.Context, db *sql.DB) (map[string]int64, error) {
			out := map[string]int64{}
			rows, err := db.QueryContext(ctx, `
				SELECT COALESCE(skills, '[]'),
				       COALESCE(subagent_models, '{}'),
				       COALESCE(subagent_skills, '{}')
				FROM agent_personas`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			for rows.Next() {
				var skills, subModels, subSkills string
				if err := rows.Scan(&skills, &subModels, &subSkills); err != nil {
					return nil, err
				}
				out["persona_skill_bindings"] += jsonArrayCount(skills)
				out["persona_subagent_models"] += jsonObjectCount(subModels)
				out["persona_subagent_skill_bindings"] += jsonObjectFlatCount(subSkills)
			}
			return out, rows.Err()
		},
	}
}

// ---------------------------------------------------------------
// channel_groups: 1:1, source already INT timestamps.
// ---------------------------------------------------------------

func channelGroupsTable() tableMigration {
	return tableMigration{
		source: "channel_groups", target: "channel_groups",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, agent_id, channel, channel_group_id,
				       COALESCE(group_name, ''),
				       COALESCE(status, 'active'),
				       COALESCE(created_at, 0),
				       COALESCE(updated_at, 0)
				FROM channel_groups`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, agentID, channel, gid, name, status string
			var createdAt, updatedAt int64
			if err := row.Scan(&id, &agentID, &channel, &gid, &name, &status, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO channel_groups
				(id, agent_id, channel, channel_group_id, group_name, status,
				 created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				id, agentID, channel, gid, name, status, createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// group_persona_assignments: TEXT timestamps → BIGINT.
// ---------------------------------------------------------------

func groupPersonaAssignmentsTable() tableMigration {
	return tableMigration{
		source: "group_persona_assignments", target: "group_persona_assignments",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, agent_id, session_key,
				       COALESCE(group_name, ''),
				       COALESCE(persona_id, ''),
				       COALESCE(created_at, ''),
				       COALESCE(updated_at, '')
				FROM group_persona_assignments`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, agentID, sessionKey, name, personaID string
			var createdAtRaw, updatedAtRaw string
			if err := row.Scan(&id, &agentID, &sessionKey, &name, &personaID, &createdAtRaw, &updatedAtRaw); err != nil {
				return 0, err
			}
			createdAt, err := parseTextTimestampMs(createdAtRaw)
			if err != nil {
				return 0, fmt.Errorf("assignment %s created_at: %w", id, err)
			}
			updatedAt, err := parseTextTimestampMs(updatedAtRaw)
			if err != nil {
				return 0, fmt.Errorf("assignment %s updated_at: %w", id, err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO group_persona_assignments
				(id, agent_id, session_key, group_name, persona_id,
				 created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
				id, agentID, sessionKey, name, personaID, createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// agent_sessions: drop dead `messages` JSON column.
// ---------------------------------------------------------------

func agentSessionsTable() tableMigration {
	return tableMigration{
		source: "agent_sessions", target: "agent_sessions",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id,
				       COALESCE(title, '新对话'),
				       COALESCE(sdk_session_id, ''),
				       COALESCE(user_id, ''),
				       COALESCE(agent_id, ''),
				       COALESCE(source_channel, 'webui'),
				       COALESCE(execution_status, 'idle'),
				       COALESCE(mode, 'normal'),
				       COALESCE(channel_name, ''),
				       COALESCE(channel_conversation_id, ''),
				       COALESCE(session_key, ''),
				       COALESCE(work_dir, ''),
				       COALESCE(context, ''),
				       COALESCE(event_cursor, 0),
				       COALESCE(created_at, 0),
				       COALESCE(updated_at, 0)
				FROM agent_sessions`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, title, sdkSessionID, userID, agentID, sourceCh, execStatus, mode string
			var channelName, channelConvID, sessionKey, workDir, contextStr string
			var eventCursor, createdAt, updatedAt int64
			if err := row.Scan(&id, &title, &sdkSessionID, &userID, &agentID, &sourceCh, &execStatus, &mode,
				&channelName, &channelConvID, &sessionKey, &workDir, &contextStr,
				&eventCursor, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO agent_sessions
				(id, title, sdk_session_id, user_id, agent_id, source_channel, execution_status, mode,
				 channel_name, channel_conversation_id, session_key, work_dir, context,
				 event_cursor, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				id, title, sdkSessionID, userID, agentID, sourceCh, execStatus, mode,
				channelName, channelConvID, sessionKey, workDir, contextStr,
				eventCursor, createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// session_active_skills: 1:1.
// ---------------------------------------------------------------

func sessionActiveSkillsTable() tableMigration {
	return tableMigration{
		source: "session_active_skills", target: "session_active_skills",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, session_id, skill_id,
				       COALESCE(source, 'hook'),
				       COALESCE(activation_order, 0),
				       COALESCE(created_at, 0)
				FROM session_active_skills
				ORDER BY session_id, rowid ASC`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, sessionID, skillID, source string
			var activationOrder, createdAt int64
			if err := row.Scan(&id, &sessionID, &skillID, &source, &activationOrder, &createdAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO session_active_skills
				(id, session_id, skill_id, source, activation_order, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
				id, sessionID, skillID, source, activationOrder, createdAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// session_events: (seq, session_id, message_id) — preserve seq.
// ---------------------------------------------------------------

func sessionEventsTable() tableMigration {
	return tableMigration{
		source: "session_events", target: "session_events",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `SELECT seq, session_id, message_id FROM session_events ORDER BY seq`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var seq, messageID int64
			var sessionID string
			if err := row.Scan(&seq, &sessionID, &messageID); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO session_events (seq, session_id, message_id) VALUES (?, ?, ?)`,
				seq, sessionID, messageID); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// session_facts: 1:1 with category column preserved.
// ---------------------------------------------------------------

func sessionFactsTable() tableMigration {
	return tableMigration{
		source: "session_facts", target: "session_facts",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, session_id, fact,
				       COALESCE(category, 'general'),
				       COALESCE(created_at, 0)
				FROM session_facts`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, createdAt int64
			var sessionID, fact, category string
			if err := row.Scan(&id, &sessionID, &fact, &category, &createdAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO session_facts
				(id, session_id, fact, category, created_at)
				VALUES (?, ?, ?, ?, ?)`,
				id, sessionID, fact, category, createdAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// messages: parent + tool_calls/attachments_json splits.
// ---------------------------------------------------------------

func messagesTable() tableMigration {
	children := []string{"message_tool_calls", "message_attachments"}
	return tableMigration{
		source: "messages", target: "messages",
		children: children,
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, session_id,
				       COALESCE(role, ''),
				       COALESCE(content, ''),
				       COALESCE(message_type, 'text'),
				       COALESCE(channel, ''),
				       COALESCE(channel_message_id, ''),
				       COALESCE(reply_to_message_id, ''),
				       COALESCE(tool_calls, '[]'),
				       COALESCE(trace_id, ''),
				       COALESCE(initiator, ''),
				       COALESCE(sender_name, ''),
				       COALESCE(sender_id, ''),
				       COALESCE(attachments_json, '[]'),
				       COALESCE(channel_meta, '{}'),
				       COALESCE(status, 'sent'),
				       COALESCE(created_at, 0)
				FROM messages
				ORDER BY id`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, childCounts map[string]int64) (int64, error) {
			var id, createdAt int64
			var sessionID, role, content, msgType, channel, channelMsgID, replyTo string
			var toolCalls, traceID, initiator, senderName, senderID string
			var attachments, channelMeta, status string
			if err := row.Scan(&id, &sessionID, &role, &content, &msgType, &channel, &channelMsgID, &replyTo,
				&toolCalls, &traceID, &initiator, &senderName, &senderID,
				&attachments, &channelMeta, &status, &createdAt); err != nil {
				return 0, err
			}
			channelMeta = defaultJSONObject(channelMeta)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO messages
				(id, session_id, role, content, message_type, channel, channel_message_id, reply_to_message_id,
				 trace_id, initiator, sender_name, sender_id, channel_meta, status, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, sessionID, role, content, msgType, channel, channelMsgID, replyTo,
				traceID, initiator, senderName, senderID, channelMeta, status, createdAt); err != nil {
				return 0, err
			}

			n, err := insertMessageToolCalls(ctx, tx, id, toolCalls, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["message_tool_calls"] += n

			n, err = insertMessageAttachments(ctx, tx, id, attachments, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["message_attachments"] += n

			return 1, nil
		},
		verifyChildren: func(ctx context.Context, db *sql.DB) (map[string]int64, error) {
			out := map[string]int64{}
			rows, err := db.QueryContext(ctx, `
				SELECT COALESCE(tool_calls, '[]'),
				       COALESCE(attachments_json, '[]')
				FROM messages`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			for rows.Next() {
				var toolCalls, attachments string
				if err := rows.Scan(&toolCalls, &attachments); err != nil {
					return nil, err
				}
				out["message_tool_calls"] += countToolUseBlocks(toolCalls)
				out["message_attachments"] += jsonArrayCount(attachments)
			}
			return out, rows.Err()
		},
	}
}

// ---------------------------------------------------------------
// context_compactions: 1:1 with the full BIGINT-timestamp schema.
// ---------------------------------------------------------------

func contextCompactionsTable() tableMigration {
	return tableMigration{
		source: "context_compactions", target: "context_compactions",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, session_id,
				       COALESCE(summary, ''),
				       COALESCE(archived_before_time, 0),
				       COALESCE(archived_message_count, 0),
				       COALESCE(token_count_before, 0),
				       COALESCE(token_count_after, 0),
				       COALESCE(compact_model, ''),
				       COALESCE(created_at, 0)
				FROM context_compactions`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, archivedBefore, msgCount, tokensBefore, tokensAfter, createdAt int64
			var sessionID, summary, compactModel string
			if err := row.Scan(&id, &sessionID, &summary, &archivedBefore, &msgCount,
				&tokensBefore, &tokensAfter, &compactModel, &createdAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO context_compactions
				(id, session_id, summary, archived_before_time, archived_message_count,
				 token_count_before, token_count_after, compact_model, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, sessionID, summary, archivedBefore, msgCount,
				tokensBefore, tokensAfter, compactModel, createdAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// ---------------------------------------------------------------
// context_compaction_archives: parent + archived_messages split.
// ---------------------------------------------------------------

func contextCompactionArchivesTable() tableMigration {
	children := []string{"context_compaction_archived_messages"}
	return tableMigration{
		source: "context_compaction_archives", target: "context_compaction_archives",
		children: children,
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, session_id, compaction_id,
				       COALESCE(archived_messages, '[]'),
				       COALESCE(message_count, 0),
				       COALESCE(created_at, 0)
				FROM context_compaction_archives`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, childCounts map[string]int64) (int64, error) {
			var id, compactionID, msgCount, createdAt int64
			var sessionID, archived string
			if err := row.Scan(&id, &sessionID, &compactionID, &archived, &msgCount, &createdAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO context_compaction_archives
				(id, session_id, compaction_id, message_count, created_at)
				VALUES (?, ?, ?, ?, ?)`,
				id, sessionID, compactionID, msgCount, createdAt); err != nil {
				return 0, err
			}
			n, err := insertArchivedMessages(ctx, tx, id, archived, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["context_compaction_archived_messages"] += n
			return 1, nil
		},
		verifyChildren: func(ctx context.Context, db *sql.DB) (map[string]int64, error) {
			out := map[string]int64{}
			rows, err := db.QueryContext(ctx, `SELECT COALESCE(archived_messages, '[]') FROM context_compaction_archives`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			for rows.Next() {
				var s string
				if err := rows.Scan(&s); err != nil {
					return nil, err
				}
				out["context_compaction_archived_messages"] += jsonArrayCount(s)
			}
			return out, rows.Err()
		},
	}
}

// ---------------------------------------------------------------
// processed_messages / delayed_tasks / user_memory(_facts).
// ---------------------------------------------------------------

func processedMessagesTable() tableMigration {
	return tableMigration{
		source: "processed_messages", target: "processed_messages",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT channel_message_id, channel_type, COALESCE(processed_at, 0)
				FROM processed_messages`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var ct, mid string
			var pAt int64
			if err := row.Scan(&mid, &ct, &pAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO processed_messages (channel_message_id, channel_type, processed_at)
				 VALUES (?, ?, ?)`,
				mid, ct, pAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

func delayedTasksTable() tableMigration {
	return tableMigration{
		source: "delayed_tasks", target: "delayed_tasks",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, session_id, agent_id, user_id,
				       COALESCE(channel, ''),
				       COALESCE(channel_user_id, ''),
				       COALESCE(channel_conversation_id, ''),
				       task,
				       execute_at,
				       COALESCE(status, 'pending'),
				       COALESCE(created_at, 0),
				       COALESCE(updated_at, 0)
				FROM delayed_tasks`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, executeAt, createdAt, updatedAt int64
			var sessionID, agentID, userID, channel, channelUserID, channelConvID, task, status string
			if err := row.Scan(&id, &sessionID, &agentID, &userID, &channel, &channelUserID, &channelConvID,
				&task, &executeAt, &status, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO delayed_tasks
				(id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id,
				 task, execute_at, status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, sessionID, agentID, userID, channel, channelUserID, channelConvID,
				task, executeAt, status, createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

func userMemoryTable() tableMigration {
	return tableMigration{
		source: "user_memory", target: "user_memory",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, user_id,
				       COALESCE(agent_id, ''),
				       COALESCE(summary, ''),
				       COALESCE(updated_at, 0)
				FROM user_memory`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, userID, agentID, summary string
			var updatedAt int64
			if err := row.Scan(&id, &userID, &agentID, &summary, &updatedAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_memory
				(id, user_id, agent_id, summary, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, 0)`,
				id, userID, agentID, summary, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

func userMemoryFactsTable() tableMigration {
	return tableMigration{
		source: "user_memory_facts", target: "user_memory_facts",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, user_id,
				       COALESCE(agent_id, ''),
				       category, fact,
				       COALESCE(source_channel, ''),
				       COALESCE(source_session_id, ''),
				       COALESCE(created_at, 0),
				       COALESCE(expires_at, 0)
				FROM user_memory_facts`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, createdAt, expiresAt int64
			var userID, agentID, category, fact, sourceChannel, sourceSession string
			if err := row.Scan(&id, &userID, &agentID, &category, &fact, &sourceChannel, &sourceSession,
				&createdAt, &expiresAt); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_memory_facts
				(id, user_id, agent_id, category, fact, source_channel, source_session_id,
				 created_at, expires_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, userID, agentID, category, fact, sourceChannel, sourceSession,
				createdAt, expiresAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}
