package api

import (
	"net/http"
	"strings"

	"agent/internal/storage"
)

// GET /api/models
func handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	models, err := storage.GetAllModels()
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if models == nil {
		models = []storage.ModelResponse{}
	}
	ok(w, models)
}

// GET /api/models/enabled | GET /api/models/:id | PATCH /api/models/:id | GET /api/models/:id/available-models
func handleModelsWithID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/models/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.SplitN(path, "/", 2)

	// GET /api/models/enabled
	if parts[0] == "enabled" {
		if r.Method != http.MethodGet {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		models, err := storage.GetEnabledModels()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if models == nil {
			models = []storage.ModelResponse{}
		}
		ok(w, models)
		return
	}

	id := parts[0]
	if id == "" {
		apiErr(w, http.StatusBadRequest, "missing model id")
		return
	}

	// GET /api/models/:id/available-models
	if len(parts) == 2 && parts[1] == "available-models" {
		if r.Method != http.MethodGet {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		rec, err := storage.GetModelByIDWithAPIKey(id)
		if err != nil || rec == nil {
			apiErr(w, http.StatusNotFound, "model not found")
			return
		}
		if rec.APIKey == "" {
			apiErr(w, http.StatusBadRequest, "请先配置 API Key 后再获取模型列表")
			return
		}
		list, err := fetchAvailableModels(rec.Provider, rec.APIKey, rec.BaseURL)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "获取模型列表失败，请检查 API Key 是否正确")
			return
		}
		ok(w, list)
		return
	}

	switch r.Method {
	case http.MethodGet:
		m, err := storage.GetModelByID(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if m == nil {
			apiErr(w, http.StatusNotFound, "model not found")
			return
		}
		ok(w, m)
	case http.MethodPatch:
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		updated, err := storage.UpdateModel(id, body)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if updated == nil {
			apiErr(w, http.StatusNotFound, "model not found")
			return
		}
		ok(w, updated)
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
