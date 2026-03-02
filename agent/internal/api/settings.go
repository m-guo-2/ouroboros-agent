package api

import (
	"net/http"
	"strings"

	"agent/internal/storage"
)

// GET/PUT /api/settings
func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		all, err := storage.GetAllSettings()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, all)
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
		ok(w, all)
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
