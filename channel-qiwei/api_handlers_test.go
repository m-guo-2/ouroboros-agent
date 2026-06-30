package main

import "testing"

func TestPrepareMentionSendIgnoresPrivateChat(t *testing.T) {
	info := prepareMentionSend([]mentionTarget{{UserID: "user-1", Name: "张三"}}, "", "user-private", "text")
	if info.Enabled {
		t.Fatalf("private chat mention should not be enabled")
	}
	if info.Status != "ignored_private_chat" {
		t.Fatalf("status = %q, want ignored_private_chat", info.Status)
	}
}

func TestApplyMentionParamsUsesHyperTextSegments(t *testing.T) {
	info := prepareMentionSend([]mentionTarget{
		{UserID: "user-1", Name: "张三"},
		{UserID: "user-2"},
		{UserID: "@all", Name: "所有人"},
	}, "room-1", "sender-1", "text")
	if !info.Enabled {
		t.Fatalf("group text mention should be enabled: %+v", info)
	}

	method := "/msg/sendText"
	params := map[string]any{"toId": "room-1", "content": "这个问题看一下"}
	applyMentionParams(&method, params, info)

	if method != "/msg/sendHyperText" {
		t.Fatalf("method = %q, want /msg/sendHyperText", method)
	}
	content, ok := params["content"].([]map[string]any)
	if !ok {
		t.Fatalf("content missing or wrong type: %+v", params["content"])
	}
	if len(content) != 3 {
		t.Fatalf("unexpected content: %+v", content)
	}
	if content[0]["subtype"] != 1 || content[0]["text"] != "user-1" {
		t.Fatalf("unexpected first mention segment: %+v", content[0])
	}
	if content[1]["subtype"] != 1 || content[1]["text"] != "user-2" {
		t.Fatalf("unexpected second mention segment: %+v", content[1])
	}
	if content[2]["subtype"] != 0 || content[2]["text"] != " 这个问题看一下" {
		t.Fatalf("unexpected text segment: %+v", content[2])
	}
}

func TestMentionFallbackKeepsReadablePrefix(t *testing.T) {
	info := prepareMentionSend([]mentionTarget{{UserID: "user-1", Name: "张三"}}, "room-1", "sender-1", "text")
	method := "/msg/sendText"
	params := map[string]any{"toId": "room-1", "content": "这个问题看一下"}
	applyMentionParams(&method, params, info)

	fallbackMethod, fallbackParams := mentionFallback(method, params, info)
	if fallbackMethod != "/msg/sendText" {
		t.Fatalf("fallback method = %q, want /msg/sendText", fallbackMethod)
	}
	if fallbackParams["content"] != "@张三 这个问题看一下" {
		t.Fatalf("fallback content = %q", fallbackParams["content"])
	}
}

func TestPrepareMentionSendDoesNotBlockUnsupportedMessageType(t *testing.T) {
	info := prepareMentionSend([]mentionTarget{{UserID: "user-1"}}, "room-1", "sender-1", "image")
	if info.Enabled {
		t.Fatalf("image mentions should not be enabled")
	}
	if info.Status != "ignored_unsupported_message_type" {
		t.Fatalf("status = %q, want ignored_unsupported_message_type", info.Status)
	}
}
