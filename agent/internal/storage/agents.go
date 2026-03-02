package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// providerCredentialsKey maps provider aliases to their settings table keys.
var providerCredentialsKey = map[string]struct{ apiKey, baseURL string }{
	"anthropic": {"api_key.anthropic", "base_url.anthropic"},
	"claude":    {"api_key.anthropic", "base_url.anthropic"},
	"openai":    {"api_key.openai", "base_url.openai"},
	"moonshot":  {"api_key.moonshot", "base_url.moonshot"},
	"kimi":      {"api_key.moonshot", "base_url.moonshot"},
	"zhipu":     {"api_key.zhipu", "base_url.zhipu"},
	"glm":       {"api_key.zhipu", "base_url.zhipu"},
}

const agentSelectSQL = `SELECT id, COALESCE(model_id,''), display_name, COALESCE(system_prompt,''),
	COALESCE(provider,''), COALESCE(model,''), COALESCE(skills,'[]'), COALESCE(channels,'[]'), is_active`

// scanAgentConfig scans a single row into AgentConfig.
// channels in the DB may be stored with either legacy keys (channelType/channelIdentifier)
// or current keys (type/identifier); both are handled transparently.
func scanAgentConfig(scan func(...interface{}) error) (AgentConfig, error) {
	var cfg AgentConfig
	var skillsJSON, channelsJSON string
	var isActive int
	if err := scan(
		&cfg.ID, &cfg.ModelID, &cfg.DisplayName, &cfg.SystemPrompt,
		&cfg.Provider, &cfg.Model, &skillsJSON, &channelsJSON, &isActive,
	); err != nil {
		return cfg, err
	}
	cfg.IsActive = isActive == 1
	_ = json.Unmarshal([]byte(skillsJSON), &cfg.Skills)

	// Support legacy storage format {"channelType":...,"channelIdentifier":...}
	var rawChannels []map[string]string
	if err := json.Unmarshal([]byte(channelsJSON), &rawChannels); err == nil {
		for _, rc := range rawChannels {
			cb := ChannelBinding{}
			if v, ok := rc["type"]; ok {
				cb.ChannelType = v
			} else if v, ok := rc["channelType"]; ok {
				cb.ChannelType = v
			}
			if v, ok := rc["identifier"]; ok {
				cb.ChannelIdentifier = v
			} else if v, ok := rc["channelIdentifier"]; ok {
				cb.ChannelIdentifier = v
			}
			cfg.Channels = append(cfg.Channels, cb)
		}
	}
	if cfg.Channels == nil {
		cfg.Channels = []ChannelBinding{}
	}
	if cfg.Skills == nil {
		cfg.Skills = []string{}
	}
	return cfg, nil
}

// GetAgentConfig retrieves an agent's full configuration by ID.
func GetAgentConfig(agentID string) (*AgentConfig, error) {
	row := DB.QueryRow(agentSelectSQL+` FROM agent_configs WHERE id = ?`, agentID)
	cfg, err := scanAgentConfig(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetAllAgents returns all agent configs ordered by creation time.
func GetAllAgents() ([]AgentConfig, error) {
	rows, err := DB.Query(agentSelectSQL + ` FROM agent_configs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentConfig
	for rows.Next() {
		cfg, err := scanAgentConfig(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	if out == nil {
		out = []AgentConfig{}
	}
	return out, rows.Err()
}

// GetActiveAgents returns all active agent configs.
func GetActiveAgents() ([]AgentConfig, error) {
	rows, err := DB.Query(agentSelectSQL + ` FROM agent_configs WHERE is_active = 1 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentConfig
	for rows.Next() {
		cfg, err := scanAgentConfig(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// CreateAgentConfig inserts a new agent config.
func CreateAgentConfig(cfg AgentConfig) (*AgentConfig, error) {
	if cfg.ID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		cfg.ID = fmt.Sprintf("agent-%x", b)
	}
	skillsJSON, _ := json.Marshal(cfg.Skills)
	channelsJSON, _ := json.Marshal(cfg.Channels)
	isActive := 0
	if cfg.IsActive {
		isActive = 1
	}
	_, err := DB.Exec(
		`INSERT INTO agent_configs (id, model_id, display_name, system_prompt, provider, model, skills, channels, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cfg.ID, cfg.ModelID, cfg.DisplayName, cfg.SystemPrompt, cfg.Provider, cfg.Model,
		string(skillsJSON), string(channelsJSON), isActive,
	)
	if err != nil {
		return nil, err
	}
	return GetAgentConfig(cfg.ID)
}

// UpdateAgentConfig applies partial updates to an agent config.
func UpdateAgentConfig(agentID string, updates map[string]interface{}) (*AgentConfig, error) {
	colMap := map[string]string{
		"displayName":  "display_name",
		"systemPrompt": "system_prompt",
		"modelId":      "model_id",
		"provider":     "provider",
		"model":        "model",
		"isActive":     "is_active",
	}
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
		if _, err := DB.Exec(
			fmt.Sprintf("UPDATE agent_configs SET %s = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", col),
			v, agentID,
		); err != nil {
			return nil, err
		}
	}
	if skills, ok := updates["skills"]; ok {
		b, _ := json.Marshal(skills)
		DB.Exec("UPDATE agent_configs SET skills = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", string(b), agentID)
	}
	if channels, ok := updates["channels"]; ok {
		b, _ := json.Marshal(channels)
		DB.Exec("UPDATE agent_configs SET channels = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", string(b), agentID)
	}
	return GetAgentConfig(agentID)
}

// DeleteAgentConfig removes an agent config by ID.
func DeleteAgentConfig(agentID string) (bool, error) {
	res, err := DB.Exec("DELETE FROM agent_configs WHERE id = ?", agentID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetProviderCredentials reads API key and base URL from settings for a given provider alias.
func GetProviderCredentials(provider string) (*ProviderCredentials, error) {
	keys, ok := providerCredentialsKey[strings.ToLower(provider)]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}

	apiKey, err := GetSettingValue(keys.apiKey)
	if err != nil {
		return nil, err
	}
	baseURL, _ := GetSettingValue(keys.baseURL)

	return &ProviderCredentials{
		Provider: provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}, nil
}
