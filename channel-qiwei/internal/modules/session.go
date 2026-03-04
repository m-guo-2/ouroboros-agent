package modules

func buildSessionActions() map[string]string {
	return map[string]string{
		"list":       "session/list",
		"edit-group": "session/group/edit",
		"get-group":  "session/group/get",
	}
}
