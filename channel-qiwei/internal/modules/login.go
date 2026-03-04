package modules

func buildLoginActions() map[string]string {
	return map[string]string{
		"get-qr":      "login/getQrCode",
		"check-qr":    "login/checkQrCode",
		"verify-code": "login/checkCode",
		"user-login":  "login/userLogin",
		"user-status": "login/getUserStatus",
	}
}
