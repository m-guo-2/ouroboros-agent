package runner

import (
	"testing"

	"agent/internal/storage"
)

func TestApplyPersonaOverrideKeepsAgentPrompt(t *testing.T) {
	agent := &storage.AgentConfig{SystemPrompt: "agent prompt"}
	personaPrompt := "persona prompt"

	applyPersonaOverride(agent, &storage.Persona{SystemPrompt: &personaPrompt})

	if agent.SystemPrompt != "agent prompt" {
		t.Fatalf("system prompt = %q, want inherited agent prompt", agent.SystemPrompt)
	}
}
