package modules

type Registry map[string]map[string]string

func BuildRegistry() Registry {
	return Registry{
		"instance": buildInstanceActions(),
		"login":    buildLoginActions(),
		"user":     buildUserActions(),
		"contact":  buildContactActions(),
		"group":    buildGroupActions(),
		"message":  buildMessageActions(),
		"cdn":      buildCDNActions(),
		"moment":   buildMomentActions(),
		"tag":      buildTagActions(),
		"session":  buildSessionActions(),
	}
}
