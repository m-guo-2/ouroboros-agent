package modules

func buildGroupActions() map[string]string {
	return map[string]string{
		"list":                  "group/list",
		"batch-detail":          "group/getDetailBatch",
		"create":                "group/create",
		"rename":                "group/rename",
		"remark":                "group/remark",
		"set-nickname":          "group/setNickname",
		"add-member":            "group/addMember",
		"remove-member":         "group/removeMember",
		"qrcode":                "group/getQrCode",
		"set-notice":            "group/setNotice",
		"add-admin":             "group/addAdmin",
		"remove-admin":          "group/removeAdmin",
		"quit":                  "group/quit",
		"transfer-owner":        "group/transferOwner",
		"dismiss":               "group/dismiss",
		"get-openid":            "group/getOpenId",
		"enable-rename":         "group/enableRename",
		"enable-invite-confirm": "group/enableInviteConfirm",
		"accept-invite-by-link": "group/acceptInviteByLink",
		"invite-member":         "group/inviteMember",
		"toggle-invite-confirm": "group/toggleInviteConfirm",
	}
}
