package storage

import (
	"database/sql"

	"agent/internal/timeutil"
)

// ModelRecord holds a single row from the models table.
// API returns camelCase; DB uses snake_case.
type ModelRecord struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	Enabled     bool    `json:"enabled"`
	APIKey      string  `json:"-"` // never exposed
	BaseURL     string  `json:"baseUrl,omitempty"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"maxTokens"`
	Temperature float64 `json:"temperature"`
}

// ModelResponse is the shape returned to admin API (hasApiKey instead of apiKey).
type ModelResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	Enabled     bool    `json:"enabled"`
	Configured  bool    `json:"configured"`
	HasAPIKey   bool    `json:"hasApiKey"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"maxTokens"`
	Temperature float64 `json:"temperature"`
	BaseURL     string  `json:"baseUrl,omitempty"`
}

func modelToResponse(m ModelRecord) ModelResponse {
	return ModelResponse{
		ID:          m.ID,
		Name:        m.Name,
		Provider:    m.Provider,
		Enabled:     m.Enabled,
		Configured:  m.APIKey != "",
		HasAPIKey:   m.APIKey != "",
		Model:       m.Model,
		MaxTokens:   m.MaxTokens,
		Temperature: m.Temperature,
		BaseURL:     m.BaseURL,
	}
}

// GetAllModels returns all models for admin list.
func GetAllModels() ([]ModelResponse, error) {
	rows, err := DB.Query(`SELECT id, name, provider, enabled,
		CASE WHEN api_key IS NULL OR api_key = '' THEN 0 ELSE 1 END,
		COALESCE(base_url,''), model, COALESCE(max_tokens, 4096), COALESCE(temperature, 0.7)
		FROM models ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ModelResponse
	for rows.Next() {
		var m ModelRecord
		var hasKey int
		if err := rows.Scan(&m.ID, &m.Name, &m.Provider, &m.Enabled, &hasKey, &m.BaseURL, &m.Model, &m.MaxTokens, &m.Temperature); err != nil {
			return nil, err
		}
		m.APIKey = ""
		m.Enabled = m.Enabled
		res := modelToResponse(m)
		res.HasAPIKey = hasKey == 1
		res.Configured = res.HasAPIKey
		list = append(list, res)
	}
	return list, rows.Err()
}

// GetModelByID returns a single model by ID. API key is never returned.
func GetModelByID(id string) (*ModelResponse, error) {
	var m ModelRecord
	var hasKey int
	err := DB.QueryRow(`SELECT id, name, provider, enabled,
		CASE WHEN api_key IS NULL OR api_key = '' THEN 0 ELSE 1 END,
		COALESCE(base_url,''), model, COALESCE(max_tokens, 4096), COALESCE(temperature, 0.7)
		FROM models WHERE id = ?`, id).Scan(
		&m.ID, &m.Name, &m.Provider, &m.Enabled, &hasKey, &m.BaseURL, &m.Model, &m.MaxTokens, &m.Temperature,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	res := modelToResponse(m)
	res.HasAPIKey = hasKey == 1
	res.Configured = res.HasAPIKey
	return &res, nil
}

// GetEnabledModels returns only enabled and configured models (for dropdowns).
func GetEnabledModels() ([]ModelResponse, error) {
	rows, err := DB.Query(`SELECT id, name, provider, enabled,
		COALESCE(base_url,''), model, COALESCE(max_tokens, 4096), COALESCE(temperature, 0.7)
		FROM models WHERE enabled = 1 AND api_key IS NOT NULL AND api_key != '' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ModelResponse
	for rows.Next() {
		var m ModelRecord
		var enabled int
		if err := rows.Scan(&m.ID, &m.Name, &m.Provider, &enabled, &m.BaseURL, &m.Model, &m.MaxTokens, &m.Temperature); err != nil {
			return nil, err
		}
		m.Enabled = enabled == 1
		res := modelToResponse(m)
		res.HasAPIKey = true
		res.Configured = true
		list = append(list, res)
	}
	return list, rows.Err()
}

// GetModelByIDWithAPIKey returns the full record including API key (for engine use).
func GetModelByIDWithAPIKey(id string) (*ModelRecord, error) {
	var m ModelRecord
	var enabled int
	err := DB.QueryRow(`SELECT id, name, provider, enabled, COALESCE(api_key,''),
		COALESCE(base_url,''), model, COALESCE(max_tokens, 4096), COALESCE(temperature, 0.7)
		FROM models WHERE id = ?`, id).Scan(
		&m.ID, &m.Name, &m.Provider, &enabled, &m.APIKey, &m.BaseURL, &m.Model, &m.MaxTokens, &m.Temperature,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.Enabled = enabled == 1
	return &m, nil
}

// UpdateModel updates model fields. Pass empty string for apiKey to leave unchanged.
func UpdateModel(id string, updates map[string]interface{}) (*ModelResponse, error) {
	setParts := []string{"updated_at = ?"}
	args := []interface{}{timeutil.NowMs()}

	if v, ok := updates["apiKey"].(string); ok {
		setParts = append(setParts, "api_key = ?")
		args = append(args, v)
	}
	if v, ok := updates["baseUrl"].(string); ok {
		setParts = append(setParts, "base_url = ?")
		args = append(args, v)
	}
	if v, ok := updates["model"].(string); ok {
		setParts = append(setParts, "model = ?")
		args = append(args, v)
	}
	if v := intFromAny(updates["maxTokens"]); v >= 0 {
		setParts = append(setParts, "max_tokens = ?")
		args = append(args, v)
	}
	if v, ok := updates["temperature"].(float64); ok {
		setParts = append(setParts, "temperature = ?")
		args = append(args, v)
	}
	if v, ok := updates["enabled"].(bool); ok {
		en := 0
		if v {
			en = 1
		}
		setParts = append(setParts, "enabled = ?")
		args = append(args, en)
	}
	if len(setParts) <= 1 {
		return GetModelByID(id)
	}
	args = append(args, id)
	var sb string
	for i, p := range setParts {
		if i > 0 {
			sb += ", "
		}
		sb += p
	}
	if _, err := DB.Exec("UPDATE models SET "+sb+" WHERE id = ?", args...); err != nil {
		return nil, err
	}
	return GetModelByID(id)
}

func intFromAny(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	default:
		return -1
	}
}
