package modules

func buildSessionActions() map[string]string {
	return map[string]string{
		"list":       "/session/getSessionPage",
		"edit-group": "/session/setSessionCmd",
		"get-group":  "/session/getSessionList",
	}
}
