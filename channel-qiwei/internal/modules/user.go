package modules

func buildUserActions() map[string]string {
	return map[string]string{
		"create-qr":        "/user/getQrcodeCard",
		"get-profile":      "/user/getProfile",
		"update-profile":   "/user/setProfile",
		"get-corp-info":    "/user/getCorpInfo",
		"logout":           "/user/logout",
		"list-favorites":   "/msg/syncCollectionMsg",
		"add-favorite-gif": "/msg/insertCollectionMsg",
	}
}
