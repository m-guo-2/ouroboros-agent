package runner

import "strings"

func mentionItemsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "仅在群聊中、且确实需要提醒指定成员处理时使用。必须传明确 userId，不要按姓名猜测；不要为了普通回复、礼貌称呼或泛泛提醒而使用；不支持 @所有人。@ 失败不会阻塞消息发送。",
		"items": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"userId": map[string]interface{}{"type": "string", "description": "要 @ 的群成员 userId，必须来自当前群详情、联系人详情或历史消息等可靠来源"},
				"name":   map[string]interface{}{"type": "string", "description": "可选，仅用于展示和降级文本，不用于定位成员"},
			},
			"required": []string{"userId"},
		},
	}
}

func normalizeToolMentions(raw interface{}) []map[string]string {
	list, ok := raw.([]interface{})
	if !ok || len(list) == 0 {
		return nil
	}
	out := make([]map[string]string, 0, len(list))
	seen := make(map[string]bool)
	for _, item := range list {
		var userID, name string
		switch v := item.(type) {
		case string:
			userID = strings.TrimSpace(v)
		case map[string]interface{}:
			userID, _ = v["userId"].(string)
			if userID == "" {
				userID, _ = v["id"].(string)
			}
			name, _ = v["name"].(string)
		}
		userID = strings.TrimSpace(userID)
		if userID == "" || isMentionAll(userID) || seen[userID] {
			continue
		}
		seen[userID] = true
		mention := map[string]string{"userId": userID}
		if strings.TrimSpace(name) != "" {
			mention["name"] = strings.TrimSpace(name)
		}
		out = append(out, mention)
	}
	return out
}

func isMentionAll(userID string) bool {
	switch strings.ToLower(strings.TrimSpace(userID)) {
	case "all", "@all", "notify@all", "所有人", "@所有人":
		return true
	default:
		return false
	}
}
