package modules

func buildUserActions() map[string]string {
	return map[string]string{
		"create-qr":          "user/createQrCode",
		"get-profile":        "user/getProfile",
		"update-profile":     "user/updateProfile",
		"get-corp-info":      "user/getCorpInfo",
		"logout":             "user/logout",
		"list-favorites":     "user/favorite/list",
		"add-favorite-gif":   "user/favorite/addGif",
		"get-openid":         "user/getOpenId",
		"refresh-profile":    "user/refreshProfile",
		"get-contact-qrcode": "user/getContactQrCode",
	}
}
