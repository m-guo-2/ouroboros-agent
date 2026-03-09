package modules

func buildGroupActions() map[string]string {
	return map[string]string{
		"list":                  "/room/getRoomList",
		"batch-detail":          "/room/batchGetRoomDetail",
		"create":                "/room/createRoom",
		"rename":                "/room/modifyRoomName",
		"remark":                "/room/modifyRoomRemarkName",
		"set-nickname":          "/room/modifyRoomNickname",
		"add-member":            "/room/inviteRoomMember",
		"remove-member":         "/room/removeRoomMember",
		"qrcode":                "/room/getRoomQrCode",
		"set-notice":            "/room/modifyRoomNotice",
		"add-admin":             "/room/roomAddAdmin",
		"remove-admin":          "/room/roomRemoveAdmin",
		"quit":                  "/room/quitRoom",
		"transfer-owner":        "/room/changeRoomMaster",
		"dismiss":               "/room/dismissRoom",
		"get-openid":            "/room/openid",
		"enable-rename":         "/room/enableChangeRoomName",
		"enable-invite-confirm": "/room/openInviteConfirm",
		"accept-invite-by-link": "/room/agreeInviteByLink",
		"invite-member":         "/room/inviteRoomMember",
		"toggle-invite-confirm": "/room/openInviteConfirm",
	}
}
