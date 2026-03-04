package modules

func buildMessageActions() map[string]string {
	return map[string]string{
		"send-text":          "msg/sendText",
		"send-hyper-text":    "msg/sendHyperText",
		"send-image":         "msg/sendImage",
		"send-gif":           "msg/sendGif",
		"send-video":         "msg/sendVideo",
		"send-file":          "msg/sendFile",
		"send-voice":         "msg/sendVoice",
		"send-link":          "msg/sendLink",
		"send-mini-program":  "msg/sendMiniProgram",
		"send-card":          "msg/sendCard",
		"send-channel-video": "msg/sendChannelVideo",
		"send-location":      "msg/sendLocation",
		"revoke":             "msg/revokeMsg",
		"update-status":      "msg/updateStatus",
		"list-top":           "msg/top/list",
		"add-top":            "msg/top/add",
		"remove-top":         "msg/top/remove",
		"mass-send":          "msg/massSend",
		"mass-send-status":   "msg/massSendStatus",
		"mass-send-rule":     "msg/massSendRule",
		"sync-history":       "msg/syncHistory",
	}
}
