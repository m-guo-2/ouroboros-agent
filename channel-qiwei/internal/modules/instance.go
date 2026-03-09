package modules

func buildInstanceActions() map[string]string {
	return map[string]string{
		"create-device": "/client/createClient",
		"resume":        "/client/restoreClient",
		"stop":          "/client/stopClient",
		"set-callback":  "/client/setCallback",
	}
}
