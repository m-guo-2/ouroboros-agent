package modules

func buildMessageActions() map[string]string {
	return map[string]string{
		"send-text":          "/msg/sendText",
		"send-hyper-text":    "/msg/sendHyperText",
		"send-image":         "/msg/sendImage",
		"send-gif":           "/msg/sendGif",
		"send-video":         "/msg/sendVideo",
		"send-file":          "/msg/sendFile",
		"send-voice":         "/msg/sendVoice",
		"send-link":          "/msg/sendLink",
		"send-mini-program":  "/msg/sendWeapp",
		"send-card":          "/msg/sendPersonalCard",
		"send-channel-video": "/msg/sendFeedVideo",
		"send-location":      "/msg/sendLocation",
		"revoke":             "/msg/revokeMsg",
		"update-status":      "/msg/statusModify",
		"list-top":           "/msg/roomTopMessageList",
		"add-top":            "/msg/roomTopMessageSet",
		"remove-top":         "/msg/roomTopMessageSet",
		"mass-send":          "/msg/sendGroupMsg",
		"mass-send-status":   "/msg/sendGroupMsgStatus",
		"mass-send-rule":     "/msg/sendGroupMsgRule",
		"sync-history":       "/msg/syncMsg",
	}
}
