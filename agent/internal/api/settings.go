package api

import (
	"net/http"
	"strings"

	"agent/internal/storage"
)

type settingKeyDef struct {
	Key         string              `json:"key"`
	Label       string              `json:"label"`
	Secret      bool                `json:"secret,omitempty"`
	Placeholder string              `json:"placeholder,omitempty"`
	Description string              `json:"description,omitempty"`
	Type        string              `json:"type,omitempty"`
	Options     []map[string]string `json:"options,omitempty"`
	ProviderKey string              `json:"providerKey,omitempty"`
}

type settingGroup struct {
	Label string          `json:"label"`
	Keys  []settingKeyDef `json:"keys"`
}

// GET /api/settings/provider-models?provider=xxx
func handleSettingsProviderModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	if provider == "" {
		apiErr(w, http.StatusBadRequest, "missing provider")
		return
	}
	cred, err := storage.GetProviderCredentials(provider)
	if err != nil {
		apiErr(w, http.StatusBadRequest, "不支持的提供商")
		return
	}
	if strings.TrimSpace(cred.APIKey) == "" {
		apiErr(w, http.StatusBadRequest, "请先在设置页配置该提供商的 API Key")
		return
	}
	list, err := fetchAvailableModels(provider, cred.APIKey, cred.BaseURL)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "获取模型列表失败，请检查 API Key 是否正确")
		return
	}
	if list == nil {
		list = []availableModel{}
	}
	ok(w, list)
}

// GET/PUT /api/settings
func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		all, err := storage.GetAllSettings()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeSettingsResponse(w, all)
	case http.MethodPut:
		var body map[string]string
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON; expected object with string values")
			return
		}
		if err := storage.SetMultipleSettings(body); err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		all, _ := storage.GetAllSettings()
		writeSettingsResponse(w, all)
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET/PUT/DELETE /api/settings/{key}
func handleSettingsWithKey(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/settings/")
	if key == "" {
		apiErr(w, http.StatusBadRequest, "missing key")
		return
	}
	switch r.Method {
	case http.MethodGet:
		val, err := storage.GetSettingValue(key)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, map[string]string{"key": key, "value": val})
	case http.MethodPut:
		var body struct {
			Value string `json:"value"`
		}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON; expected {value: string}")
			return
		}
		if err := storage.SetSettingValue(key, body.Value); err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, map[string]string{"key": key, "value": body.Value})
	case http.MethodDelete:
		deleted, err := storage.DeleteSettingValue(key)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			apiErr(w, http.StatusNotFound, "key not found")
			return
		}
		ok(w, map[string]bool{"deleted": true})
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func writeSettingsResponse(w http.ResponseWriter, values map[string]string) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    storage.WithDefaultSettings(values),
		"groups":  buildSettingGroups(),
	})
}

func buildSettingGroups() map[string]settingGroup {
	return map[string]settingGroup{
		"anthropic": {
			Label: "Anthropic",
			Keys: []settingKeyDef{
				{Key: "api_key.anthropic", Label: "Anthropic API Key", Secret: true, Placeholder: "sk-ant-...", Description: "用于 Claude / Anthropic 模型调用"},
				{Key: "base_url.anthropic", Label: "Anthropic Base URL", Placeholder: "https://api.anthropic.com", Description: "可选，自定义兼容 Anthropic 的代理地址"},
			},
		},
		"openai": {
			Label: "OpenAI",
			Keys: []settingKeyDef{
				{Key: "api_key.openai", Label: "OpenAI API Key", Secret: true, Placeholder: "sk-...", Description: "用于 OpenAI 兼容模型调用"},
				{Key: "base_url.openai", Label: "OpenAI Base URL", Placeholder: "https://api.openai.com/v1", Description: "可选，自定义 OpenAI 兼容地址"},
			},
		},
		"moonshot": {
			Label: "Moonshot / Kimi",
			Keys: []settingKeyDef{
				{Key: "api_key.moonshot", Label: "Moonshot API Key", Secret: true, Placeholder: "sk-...", Description: "用于 Moonshot / Kimi 模型调用"},
				{Key: "base_url.moonshot", Label: "Moonshot Base URL", Placeholder: "https://api.moonshot.cn/v1", Description: "可选，自定义 Moonshot 兼容地址"},
			},
		},
		"zhipu": {
			Label: "Zhipu / GLM",
			Keys: []settingKeyDef{
				{Key: "api_key.zhipu", Label: "Zhipu API Key", Secret: true, Placeholder: "xxx", Description: "用于 GLM / 智谱模型调用"},
				{Key: "base_url.zhipu", Label: "Zhipu Base URL", Placeholder: "https://open.bigmodel.cn/api/paas/v4", Description: "可选，自定义 GLM API 地址"},
			},
		},
		"tavily": {
			Label: "Tavily Web Search",
			Keys: []settingKeyDef{
				{Key: "enabled.tavily", Label: "启用 Tavily", Type: "select", Description: "关闭后 tavily_search 和 web_research 将返回明确错误", Options: []map[string]string{{"value": "false", "label": "关闭"}, {"value": "true", "label": "开启"}}},
				{Key: "api_key.tavily", Label: "Tavily API Key", Secret: true, Placeholder: "tvly-...", Description: "用于联网检索和 research 子代理"},
				{Key: "base_url.tavily", Label: "Tavily Base URL", Placeholder: storage.DefaultTavilyBaseURL, Description: "通常保持默认即可"},
				{Key: "tavily.search_depth", Label: "默认 Search Depth", Type: "select", Description: "控制速度与召回质量", Options: []map[string]string{{"value": "basic", "label": "basic"}, {"value": "advanced", "label": "advanced"}, {"value": "fast", "label": "fast"}, {"value": "ultra-fast", "label": "ultra-fast"}}},
				{Key: "tavily.search_topic", Label: "默认 Search Topic", Type: "select", Description: "检索主题", Options: []map[string]string{{"value": "general", "label": "general"}, {"value": "news", "label": "news"}, {"value": "finance", "label": "finance"}}},
				{Key: "tavily.max_results", Label: "默认 Max Results", Placeholder: "5", Description: "默认返回结果数；运行时仍会被平台上限限制"},
			},
		},
	}
}
