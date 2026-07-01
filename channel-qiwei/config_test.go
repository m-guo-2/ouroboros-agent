package main

import "testing"

func TestApplyQiweiEnvLoadsVolcSpeechConfig(t *testing.T) {
	t.Setenv("VOLC_SPEECH_APP_KEY", "app")
	t.Setenv("VOLC_SPEECH_ACCESS_KEY", "token")
	t.Setenv("VOLC_SPEECH_RESOURCE_ID", "resource")
	t.Setenv("VOLC_SPEECH_SUBMIT_URL", "https://example.com/submit")
	t.Setenv("VOLC_SPEECH_QUERY_URL", "https://example.com/query")

	cfg := configDefaults()
	applyQiweiEnv(&cfg)
	cfg.flatten()

	if cfg.VolcSpeechAppKey != "app" ||
		cfg.VolcSpeechAccessKey != "token" ||
		cfg.VolcSpeechResourceID != "resource" ||
		cfg.VolcSpeechSubmitURL != "https://example.com/submit" ||
		cfg.VolcSpeechQueryURL != "https://example.com/query" {
		t.Fatalf("speech env was not applied: %+v", cfg.Volc.Speech)
	}
}
