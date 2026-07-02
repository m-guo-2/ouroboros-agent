package storage

import "testing"

func TestPersonaWithoutSkillBindingsInheritsAgentDefaults(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	persona, err := CreatePersona(Persona{
		AgentID:     "agent-1",
		DisplayName: "Group Persona",
	})
	if err != nil {
		t.Fatalf("create persona: %v", err)
	}
	if persona.Skills != nil {
		t.Fatalf("expected nil skills override for persona without bindings, got %#v", *persona.Skills)
	}

	updated, err := UpdatePersona(persona.ID, map[string]any{"skills": nil})
	if err != nil {
		t.Fatalf("update persona: %v", err)
	}
	if updated.Skills != nil {
		t.Fatalf("expected nil skills override after clearing bindings, got %#v", *updated.Skills)
	}
}

func TestPersonaWithSkillBindingsReportsOverride(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	ids := []string{"skill-a", "skill-b"}
	persona, err := CreatePersona(Persona{
		AgentID:     "agent-1",
		DisplayName: "Group Persona",
		Skills:      &ids,
	})
	if err != nil {
		t.Fatalf("create persona: %v", err)
	}
	if persona.Skills == nil {
		t.Fatalf("expected skills override")
	}
	if got := *persona.Skills; len(got) != 2 || got[0] != "skill-a" || got[1] != "skill-b" {
		t.Fatalf("unexpected skills override: %#v", got)
	}
}

func TestPersonaSystemPromptIsNotPersisted(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	prompt := "persona prompt should be ignored"
	persona, err := CreatePersona(Persona{
		AgentID:      "agent-1",
		DisplayName:  "Group Persona",
		SystemPrompt: &prompt,
	})
	if err != nil {
		t.Fatalf("create persona: %v", err)
	}
	if persona.SystemPrompt != nil {
		t.Fatalf("expected persona system prompt to be ignored, got %q", *persona.SystemPrompt)
	}

	updated, err := UpdatePersona(persona.ID, map[string]any{"systemPrompt": "still ignored"})
	if err != nil {
		t.Fatalf("update persona: %v", err)
	}
	if updated.SystemPrompt != nil {
		t.Fatalf("expected update to ignore persona system prompt, got %q", *updated.SystemPrompt)
	}
}

func TestResolveEffectiveSandboxTemplateID(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	agent, err := CreateAgentConfig(AgentConfig{
		ID:                "agent-sandbox",
		DisplayName:       "Sandbox Agent",
		SandboxTemplateID: "agent-template",
		IsActive:          true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if got := ResolveEffectiveSandboxTemplateID(agent, nil, nil); got != "agent-template" {
		t.Fatalf("expected agent template, got %q", got)
	}

	personaTemplate := "persona-template"
	persona := &Persona{SandboxTemplateID: &personaTemplate}
	if got := ResolveEffectiveSandboxTemplateID(agent, persona, nil); got != "persona-template" {
		t.Fatalf("expected persona template, got %q", got)
	}

	groupTemplate := "group-template"
	assignment := &GroupAssignment{SandboxTemplateID: &groupTemplate}
	if got := ResolveEffectiveSandboxTemplateID(agent, persona, assignment); got != "group-template" {
		t.Fatalf("expected group template, got %q", got)
	}

	if got := ResolveEffectiveSandboxTemplateID(&AgentConfig{}, nil, nil); got != defaultSandboxTemplateID {
		t.Fatalf("expected default template, got %q", got)
	}
}
