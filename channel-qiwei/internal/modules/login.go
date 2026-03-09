package modules

func buildLoginActions() map[string]string {
	return map[string]string{
		"get-qr":      "/login/getLoginQrcode",
		"check-qr":    "/login/checkLoginQrCode",
		"verify-code": "/login/verifyLoginQrcode",
		"user-login":  "/login/manualLogin",
		"user-status": "/login/checkLogin",
	}
}
