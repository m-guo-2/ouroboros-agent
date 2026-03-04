package modules

func buildInstanceActions() map[string]string {
	return map[string]string{
		"create-device": "instance/create",
		"resume":        "instance/resume",
		"stop":          "instance/stop",
		"set-callback":  "instance/setCallback",
	}
}
