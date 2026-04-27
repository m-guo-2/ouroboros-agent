package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"

	"agent/internal/timeutil"
)

type providerKeyConfig struct {
	apiKey, baseURL, defaultBaseURL string
}

// providerCredentialsKey maps provider aliases to their settings table keys
// and default base URLs used when the user hasn't configured one.
var providerCredentialsKey = map[string]providerKeyConfig{
	"anthropic":  {"api_key.anthropic", "base_url.anthropic", "https://api.anthropic.com"},
	"claude":     {"api_key.anthropic", "base_url.anthropic", "https://api.anthropic.com"},
	"openai":     {"api_key.openai", "base_url.openai", "https://api.openai.com/v1"},
	"moonshot":   {"api_key.moonshot", "base_url.moonshot", "https://api.moonshot.cn/v1"},
	"kimi":       {"api_key.moonshot", "base_url.moonshot", "https://api.moonshot.cn/v1"},
	"zhipu":      {"api_key.zhipu", "base_url.zhipu", "https://open.bigmodel.cn/api/paas/v4"},
	"glm":        {"api_key.zhipu", "base_url.zhipu", "https://open.bigmodel.cn/api/paas/v4"},
	"deepseek":   {"api_key.deepseek", "base_url.deepseek", "https://api.deepseek.com"},
	"volcengine": {"api_key.volcengine", "base_url.volcengine", "https://ark.cn-beijing.volces.com/api/v3"},
	"ark":        {"api_key.volcengine", "base_url.volcengine", "https://ark.cn-beijing.volces.com/api/v3"},
}

const agentParentSelectSQL = `SELECT id, COALESCE(model_id,''), display_name, COALESCE(system_prompt,''),
	COALESCE(provider,''), COALESCE(model,''), is_active
	FROM agent_configs`

// scanAgentCoreConfig scans the core agent_configs columns only. The JSON-derived
// fields (skills, channels, hooks, subagent_models, subagent_skills) are loaded
// from their dedicated tables by loadAgentChildren().
func scanAgentCoreConfig(scan func(...interface{}) error) (AgentConfig, error) {
	var cfg AgentConfig
	var isActive int
	if err := scan(&cfg.ID, &cfg.ModelID, &cfg.DisplayName, &cfg.SystemPrompt,
		&cfg.Provider, &cfg.Model, &isActive); err != nil {
		return cfg, err
	}
	cfg.IsActive = isActive == 1
	return cfg, nil
}

// loadAgentChildren populates the relational-split fields for an agent config
// from the child tables. Missing tables return empty slices/maps, never nil.
func loadAgentChildren(q dbQ, cfg *AgentConfig) error {
	cfg.Skills = []string{}
	cfg.Channels = []ChannelBinding{}
	cfg.Hooks = []Hook{}
	cfg.SubagentModels = nil
	cfg.SubagentSkills = nil

	// agent_skill_bindings (ordered)
	rows, err := q.Query(`SELECT skill_id FROM agent_skill_bindings
		WHERE agent_id = ? AND deleted_at = 0 ORDER BY position ASC, created_at ASC`, cfg.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		cfg.Skills = append(cfg.Skills, id)
	}
	rows.Close()

	// agent_channels
	rows, err = q.Query(`SELECT channel_type, channel_identifier FROM agent_channels
		WHERE agent_id = ? AND deleted_at = 0 ORDER BY position ASC`, cfg.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cb ChannelBinding
		if err := rows.Scan(&cb.ChannelType, &cb.ChannelIdentifier); err != nil {
			rows.Close()
			return err
		}
		cfg.Channels = append(cfg.Channels, cb)
	}
	rows.Close()

	// agent_hooks
	rows, err = q.Query(`SELECT event, action_type, COALESCE(skill_id,'') FROM agent_hooks
		WHERE agent_id = ? ORDER BY event, position ASC, id ASC`, cfg.ID)
	if err != nil {
		return err
	}
	type hookRow struct {
		Event   string
		Action  HookAction
	}
	var hookRows []hookRow
	for rows.Next() {
		var hr hookRow
		if err := rows.Scan(&hr.Event, &hr.Action.Type, &hr.Action.SkillID); err != nil {
			rows.Close()
			return err
		}
		hookRows = append(hookRows, hr)
	}
	rows.Close()
	// Group actions by event while preserving first-seen order.
	seenEvents := map[string]int{}
	for _, hr := range hookRows {
		if idx, ok := seenEvents[hr.Event]; ok {
			cfg.Hooks[idx].Actions = append(cfg.Hooks[idx].Actions, hr.Action)
			continue
		}
		seenEvents[hr.Event] = len(cfg.Hooks)
		cfg.Hooks = append(cfg.Hooks, Hook{Event: hr.Event, Actions: []HookAction{hr.Action}})
	}

	// agent_subagent_models
	rows, err = q.Query(`SELECT subagent_key, COALESCE(provider,''), COALESCE(model,'')
		FROM agent_subagent_models WHERE agent_id = ? AND deleted_at = 0`, cfg.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k, p, m string
		if err := rows.Scan(&k, &p, &m); err != nil {
			rows.Close()
			return err
		}
		if cfg.SubagentModels == nil {
			cfg.SubagentModels = map[string]SubagentModelConfig{}
		}
		cfg.SubagentModels[k] = SubagentModelConfig{Provider: p, Model: m}
	}
	rows.Close()

	// agent_subagent_skill_bindings (ordered within subagent_key)
	rows, err = q.Query(`SELECT subagent_key, skill_id FROM agent_subagent_skill_bindings
		WHERE agent_id = ? AND deleted_at = 0 ORDER BY subagent_key, position ASC`, cfg.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k, sid string
		if err := rows.Scan(&k, &sid); err != nil {
			rows.Close()
			return err
		}
		if cfg.SubagentSkills == nil {
			cfg.SubagentSkills = map[string][]string{}
		}
		cfg.SubagentSkills[k] = append(cfg.SubagentSkills[k], sid)
	}
	rows.Close()

	return nil
}

// GetAgentConfig retrieves an agent's full configuration by ID.
func GetAgentConfig(agentID string) (*AgentConfig, error) {
	row := DB.QueryRow(agentParentSelectSQL+` WHERE id = ? AND deleted_at = 0`, agentID)
	cfg, err := scanAgentCoreConfig(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := loadAgentChildren(DB, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetAllAgents returns all agent configs ordered by creation time.
func GetAllAgents() ([]AgentConfig, error) {
	return queryAgents(agentParentSelectSQL + ` WHERE deleted_at = 0 ORDER BY created_at DESC`)
}

// GetActiveAgents returns all active agent configs.
func GetActiveAgents() ([]AgentConfig, error) {
	return queryAgents(agentParentSelectSQL + ` WHERE is_active = 1 AND deleted_at = 0 ORDER BY created_at DESC`)
}

func queryAgents(q string, args ...interface{}) ([]AgentConfig, error) {
	rows, err := DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentConfig
	for rows.Next() {
		cfg, err := scanAgentCoreConfig(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Second pass: load children (small N, simpler than per-row query muxing).
	for i := range out {
		if err := loadAgentChildren(DB, &out[i]); err != nil {
			return nil, err
		}
	}
	if out == nil {
		out = []AgentConfig{}
	}
	return out, nil
}

// CreateAgentConfig inserts a new agent config and all its child-table rows
// in a single transaction. Returns the freshly loaded config.
func CreateAgentConfig(cfg AgentConfig) (*AgentConfig, error) {
	if cfg.ID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		cfg.ID = fmt.Sprintf("agent-%x", b)
	}
	isActive := 0
	if cfg.IsActive {
		isActive = 1
	}
	now := timeutil.NowMs()

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO agent_configs
		 (id, user_id, model_id, display_name, system_prompt, provider, model, is_active, created_at, updated_at, deleted_at)
		 VALUES (?, '', ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		cfg.ID, cfg.ModelID, cfg.DisplayName, cfg.SystemPrompt, cfg.Provider, cfg.Model,
		isActive, now, now,
	); err != nil {
		return nil, err
	}
	if err := replaceAgentChildren(tx, cfg.ID, &cfg, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetAgentConfig(cfg.ID)
}

// UpdateAgentConfig applies partial updates to an agent config. Scalar columns
// are patched in-place; child-table arrays are fully replaced when their keys
// appear in the update map.
func UpdateAgentConfig(agentID string, updates map[string]interface{}) (*AgentConfig, error) {
	colMap := map[string]string{
		"displayName":  "display_name",
		"systemPrompt": "system_prompt",
		"modelId":      "model_id",
		"provider":     "provider",
		"model":        "model",
		"isActive":     "is_active",
	}
	now := timeutil.NowMs()

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for key, val := range updates {
		col, ok := colMap[key]
		if !ok {
			continue
		}
		v := val
		if key == "isActive" {
			if b, ok := val.(bool); ok {
				if b {
					v = 1
				} else {
					v = 0
				}
			}
		}
		if _, err := tx.Exec(
			fmt.Sprintf("UPDATE agent_configs SET %s = ?, updated_at = ? WHERE id = ? AND deleted_at = 0", col),
			v, now, agentID,
		); err != nil {
			return nil, err
		}
	}

	// Child replacements. Empty slice / map is legitimate: means "clear".
	if skills, ok := updates["skills"]; ok {
		ids := toStringSlice(skills)
		if err := replaceAgentSkillBindings(tx, agentID, ids, now); err != nil {
			return nil, err
		}
	}
	if channels, ok := updates["channels"]; ok {
		cbs := toChannelBindings(channels)
		if err := replaceAgentChannels(tx, agentID, cbs, now); err != nil {
			return nil, err
		}
	}
	if hooks, ok := updates["hooks"]; ok {
		hs := toHooks(hooks)
		if err := replaceAgentHooks(tx, agentID, hs, now); err != nil {
			return nil, err
		}
	}
	if sm, ok := updates["subagentModels"]; ok {
		models := toSubagentModels(sm)
		if err := replaceAgentSubagentModels(tx, agentID, models, now); err != nil {
			return nil, err
		}
	}
	if ss, ok := updates["subagentSkills"]; ok {
		m := toSubagentSkills(ss)
		if err := replaceAgentSubagentSkillBindings(tx, agentID, m, now); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(`UPDATE agent_configs SET updated_at = ? WHERE id = ? AND deleted_at = 0`, now, agentID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetAgentConfig(agentID)
}

// DeleteAgentConfig soft-deletes an agent config and its child bindings.
// Physical removal is handled by a separate purge path (admin tooling).
func DeleteAgentConfig(agentID string) (bool, error) {
	now := timeutil.NowMs()
	tx, err := DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE agent_configs SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at = 0`,
		now, now, agentID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, tx.Commit()
	}
	if _, err := tx.Exec(`UPDATE agent_skill_bindings SET deleted_at = ? WHERE agent_id = ? AND deleted_at = 0`, now, agentID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE agent_channels SET deleted_at = ? WHERE agent_id = ? AND deleted_at = 0`, now, agentID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE agent_subagent_models SET deleted_at = ? WHERE agent_id = ? AND deleted_at = 0`, now, agentID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE agent_subagent_skill_bindings SET deleted_at = ? WHERE agent_id = ? AND deleted_at = 0`, now, agentID); err != nil {
		return false, err
	}
	// agent_hooks is a "parent's array attribute" table — no soft-delete column;
	// purge them physically on agent delete since they are meaningless alone.
	if _, err := tx.Exec(`DELETE FROM agent_hooks WHERE agent_id = ?`, agentID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// GetProviderCredentials reads API key and base URL from settings for a given provider alias.
// If no base URL is configured, the provider's official endpoint is used as default.
func GetProviderCredentials(provider string) (*ProviderCredentials, error) {
	cfg, ok := providerCredentialsKey[strings.ToLower(provider)]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}

	apiKey, err := GetSettingValue(cfg.apiKey)
	if err != nil {
		return nil, err
	}
	baseURL, _ := GetSettingValue(cfg.baseURL)
	if strings.TrimSpace(baseURL) == "" {
		baseURL = cfg.defaultBaseURL
	}

	return &ProviderCredentials{
		Provider: provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}, nil
}

// --- child-table replacement helpers (txn-scoped) -------------------------

// replaceAgentChildren fully rewrites all five child tables for a given agent.
// Used by Create, where we start from a clean slate.
func replaceAgentChildren(tx *sql.Tx, agentID string, cfg *AgentConfig, now int64) error {
	if err := replaceAgentSkillBindings(tx, agentID, cfg.Skills, now); err != nil {
		return err
	}
	if err := replaceAgentChannels(tx, agentID, cfg.Channels, now); err != nil {
		return err
	}
	if err := replaceAgentHooks(tx, agentID, cfg.Hooks, now); err != nil {
		return err
	}
	if err := replaceAgentSubagentModels(tx, agentID, cfg.SubagentModels, now); err != nil {
		return err
	}
	if err := replaceAgentSubagentSkillBindings(tx, agentID, cfg.SubagentSkills, now); err != nil {
		return err
	}
	return nil
}

func replaceAgentSkillBindings(tx *sql.Tx, agentID string, skillIDs []string, now int64) error {
	// Soft-delete all current live rows, then insert fresh ones.
	if _, err := tx.Exec(`UPDATE agent_skill_bindings SET deleted_at = ? WHERE agent_id = ? AND deleted_at = 0`, now, agentID); err != nil {
		return err
	}
	for i, id := range skillIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		rowID := fmt.Sprintf("asb-%s-%s-%d", shortHash(agentID, id), id, now)
		if _, err := tx.Exec(
			`INSERT INTO agent_skill_bindings (id, agent_id, skill_id, mode, position, created_at, deleted_at)
			 VALUES (?, ?, ?, '', ?, ?, 0)`,
			rowID, agentID, id, i, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func replaceAgentChannels(tx *sql.Tx, agentID string, channels []ChannelBinding, now int64) error {
	if _, err := tx.Exec(`UPDATE agent_channels SET deleted_at = ? WHERE agent_id = ? AND deleted_at = 0`, now, agentID); err != nil {
		return err
	}
	for i, cb := range channels {
		ct := strings.TrimSpace(cb.ChannelType)
		cid := strings.TrimSpace(cb.ChannelIdentifier)
		if ct == "" || cid == "" {
			continue
		}
		rowID := fmt.Sprintf("ach-%s-%d-%d", agentID, i, now)
		if _, err := tx.Exec(
			`INSERT INTO agent_channels (id, agent_id, channel_type, channel_identifier, position, created_at, deleted_at)
			 VALUES (?, ?, ?, ?, ?, ?, 0)`,
			rowID, agentID, ct, cid, i, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func replaceAgentHooks(tx *sql.Tx, agentID string, hooks []Hook, now int64) error {
	// agent_hooks has no soft-delete; physical wipe + re-insert.
	if _, err := tx.Exec(`DELETE FROM agent_hooks WHERE agent_id = ?`, agentID); err != nil {
		return err
	}
	for _, h := range hooks {
		event := strings.TrimSpace(h.Event)
		if event == "" {
			continue
		}
		for i, a := range h.Actions {
			if _, err := tx.Exec(
				`INSERT INTO agent_hooks (agent_id, event, action_type, skill_id, position, created_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				agentID, event, a.Type, a.SkillID, i, now,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceAgentSubagentModels(tx *sql.Tx, agentID string, models map[string]SubagentModelConfig, now int64) error {
	if _, err := tx.Exec(`UPDATE agent_subagent_models SET deleted_at = ? WHERE agent_id = ? AND deleted_at = 0`, now, agentID); err != nil {
		return err
	}
	for k, v := range models {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		rowID := fmt.Sprintf("asm-%s-%s-%d", agentID, k, now)
		if _, err := tx.Exec(
			`INSERT INTO agent_subagent_models (id, agent_id, subagent_key, provider, model, created_at, updated_at, deleted_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
			rowID, agentID, k, v.Provider, v.Model, now, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func replaceAgentSubagentSkillBindings(tx *sql.Tx, agentID string, m map[string][]string, now int64) error {
	if _, err := tx.Exec(`UPDATE agent_subagent_skill_bindings SET deleted_at = ? WHERE agent_id = ? AND deleted_at = 0`, now, agentID); err != nil {
		return err
	}
	for k, ids := range m {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		for i, sid := range ids {
			sid = strings.TrimSpace(sid)
			if sid == "" {
				continue
			}
			rowID := fmt.Sprintf("assb-%s-%s-%s-%d", agentID, k, sid, now)
			if _, err := tx.Exec(
				`INSERT INTO agent_subagent_skill_bindings (id, agent_id, subagent_key, skill_id, position, created_at, deleted_at)
				 VALUES (?, ?, ?, ?, ?, ?, 0)`,
				rowID, agentID, k, sid, i, now,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- update-map coercion helpers -----------------------------------------

func toStringSlice(v interface{}) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func toChannelBindings(v interface{}) []ChannelBinding {
	switch x := v.(type) {
	case []ChannelBinding:
		return x
	case []interface{}:
		out := make([]ChannelBinding, 0, len(x))
		for _, item := range x {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			cb := ChannelBinding{
				ChannelType:       stringField(m, "type", "channelType"),
				ChannelIdentifier: stringField(m, "identifier", "channelIdentifier"),
			}
			if cb.ChannelType != "" || cb.ChannelIdentifier != "" {
				out = append(out, cb)
			}
		}
		return out
	}
	return nil
}

func toHooks(v interface{}) []Hook {
	switch x := v.(type) {
	case []Hook:
		return x
	case []interface{}:
		out := make([]Hook, 0, len(x))
		for _, item := range x {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			h := Hook{Event: stringField(m, "event")}
			if actions, ok := m["actions"].([]interface{}); ok {
				for _, a := range actions {
					am, ok := a.(map[string]interface{})
					if !ok {
						continue
					}
					h.Actions = append(h.Actions, HookAction{
						Type:    stringField(am, "type"),
						SkillID: stringField(am, "skillId"),
					})
				}
			}
			out = append(out, h)
		}
		return out
	}
	return nil
}

func toSubagentModels(v interface{}) map[string]SubagentModelConfig {
	switch x := v.(type) {
	case map[string]SubagentModelConfig:
		return x
	case map[string]interface{}:
		out := map[string]SubagentModelConfig{}
		for k, val := range x {
			m, ok := val.(map[string]interface{})
			if !ok {
				continue
			}
			out[k] = SubagentModelConfig{
				Provider: stringField(m, "provider"),
				Model:    stringField(m, "model"),
			}
		}
		return out
	}
	return nil
}

func toSubagentSkills(v interface{}) map[string][]string {
	switch x := v.(type) {
	case map[string][]string:
		return x
	case map[string]interface{}:
		out := map[string][]string{}
		for k, val := range x {
			out[k] = toStringSlice(val)
		}
		return out
	}
	return nil
}

func stringField(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// dbQ is the subset of *sql.DB / *sql.Tx used by load helpers.
type dbQ interface {
	Query(q string, args ...interface{}) (*sql.Rows, error)
}

// shortHash returns a short deterministic-ish key derived from inputs. Not
// cryptographic; collision here only matters within one agent so natural-key
// pairs already make IDs unique. Use last 6 chars of crypto/rand.
func shortHash(parts ...string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
