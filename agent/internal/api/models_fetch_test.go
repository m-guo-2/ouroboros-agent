package api

import "testing"

func TestVolcengineFallbackModelsIncludesSeedreamLite(t *testing.T) {
	models := volcengineFallbackModels()
	for _, model := range models {
		if model.ID == "doubao-seedream-5-0-lite" {
			if model.Provider != "volcengine" {
				t.Fatalf("provider = %q, want volcengine", model.Provider)
			}
			return
		}
	}
	t.Fatal("expected doubao-seedream-5-0-lite in volcengine fallback models")
}
