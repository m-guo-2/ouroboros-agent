package storage

const defaultSandboxTemplateID = "office-worker"

func normalizeSandboxTemplateID(id string) string {
	if id == "" {
		return defaultSandboxTemplateID
	}
	return id
}

func ptrStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ResolveEffectiveSandboxTemplateID(agent *AgentConfig, persona *Persona, assignment *GroupAssignment) string {
	id := ""
	if agent != nil {
		id = agent.SandboxTemplateID
	}
	if persona != nil && ptrStringValue(persona.SandboxTemplateID) != "" {
		id = *persona.SandboxTemplateID
	}
	if assignment != nil && ptrStringValue(assignment.SandboxTemplateID) != "" {
		id = *assignment.SandboxTemplateID
	}
	return normalizeSandboxTemplateID(id)
}

func ResolveSessionSandboxTemplateID(agentID, sessionKey string) string {
	agent, err := GetAgentConfig(agentID)
	if err != nil || agent == nil {
		return defaultSandboxTemplateID
	}
	var assignment *GroupAssignment
	var persona *Persona
	if sessionKey != "" {
		assignment, _ = GetGroupAssignment(agentID, sessionKey)
		if assignment != nil && assignment.PersonaID != nil && *assignment.PersonaID != "" {
			persona, _ = GetPersona(*assignment.PersonaID)
		}
	}
	return ResolveEffectiveSandboxTemplateID(agent, persona, assignment)
}
