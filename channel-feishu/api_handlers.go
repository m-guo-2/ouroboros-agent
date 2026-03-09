package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type actionDef struct {
	Name     string
	Category string
	Handler  func(context.Context, map[string]any) (any, error)
}

func (a *app) actionDefs() []actionDef {
	defs := []actionDef{
		// message
		{"send_text", "message", func(ctx context.Context, p map[string]any) (any, error) {
			req := sendRequest{
				ReceiveID:     stringValue(p["receive_id"]),
				ReceiveIDType: stringValue(p["receive_id_type"]),
				Content: map[string]any{
					"type": "text",
					"text": stringValue(p["text"]),
				},
			}
			if req.ReceiveIDType == "" {
				req.ReceiveIDType = "chat_id"
			}
			content, msgType, err := buildMessageContent(req.Content, nil)
			if err != nil {
				return nil, err
			}
			return nil, a.sendMessage(ctx, req.ReceiveIDType, req.ReceiveID, msgType, content)
		}},
		{"send_rich_text", "message", func(ctx context.Context, p map[string]any) (any, error) {
			req := sendRequest{
				ReceiveID:     stringValue(p["receive_id"]),
				ReceiveIDType: stringValue(p["receive_id_type"]),
				Content: map[string]any{
					"type":    "rich_text",
					"title":   stringValue(p["title"]),
					"content": p["content"],
				},
			}
			if req.ReceiveIDType == "" {
				req.ReceiveIDType = "chat_id"
			}
			content, msgType, err := buildMessageContent(req.Content, nil)
			if err != nil {
				return nil, err
			}
			return nil, a.sendMessage(ctx, req.ReceiveIDType, req.ReceiveID, msgType, content)
		}},
		{"send_card", "message", func(ctx context.Context, p map[string]any) (any, error) {
			req := sendRequest{
				ReceiveID:     stringValue(p["receive_id"]),
				ReceiveIDType: stringValue(p["receive_id_type"]),
				Content: map[string]any{
					"type":             "card",
					"templateId":       p["template_id"],
					"templateVariable": p["template_variable"],
					"cardContent":      p["card_content"],
				},
			}
			if req.ReceiveIDType == "" {
				req.ReceiveIDType = "chat_id"
			}
			content, msgType, err := buildMessageContent(req.Content, nil)
			if err != nil {
				return nil, err
			}
			return nil, a.sendMessage(ctx, req.ReceiveIDType, req.ReceiveID, msgType, content)
		}},
		{"send_default_card", "message", func(ctx context.Context, p map[string]any) (any, error) {
			card := map[string]any{
				"config": map[string]any{"wide_screen_mode": true},
				"elements": []any{
					map[string]any{
						"tag": "div",
						"text": map[string]any{
							"tag":     "plain_text",
							"content": stringValue(p["content"]),
						},
					},
				},
				"header": map[string]any{
					"title":    map[string]any{"tag": "plain_text", "content": stringValue(p["title"])},
					"template": "blue",
				},
			}
			req := sendRequest{
				ReceiveID:     stringValue(p["receive_id"]),
				ReceiveIDType: stringValue(p["receive_id_type"]),
				Content:       map[string]any{"type": "card", "cardContent": card},
			}
			if req.ReceiveIDType == "" {
				req.ReceiveIDType = "chat_id"
			}
			content, msgType, err := buildMessageContent(req.Content, nil)
			if err != nil {
				return nil, err
			}
			return nil, a.sendMessage(ctx, req.ReceiveIDType, req.ReceiveID, msgType, content)
		}},
		{"send_image", "message", func(ctx context.Context, p map[string]any) (any, error) {
			raw, _ := json.Marshal(map[string]any{"image_key": stringValue(p["image_key"])})
			return nil, a.sendMessage(ctx, defaultType(p), stringValue(p["receive_id"]), "image", string(raw))
		}},
		{"send_file", "message", func(ctx context.Context, p map[string]any) (any, error) {
			raw, _ := json.Marshal(map[string]any{"file_key": stringValue(p["file_key"])})
			return nil, a.sendMessage(ctx, defaultType(p), stringValue(p["receive_id"]), "file", string(raw))
		}},
		{"send_audio", "message", func(ctx context.Context, p map[string]any) (any, error) {
			raw, _ := json.Marshal(map[string]any{"file_key": stringValue(p["file_key"])})
			return nil, a.sendMessage(ctx, defaultType(p), stringValue(p["receive_id"]), "audio", string(raw))
		}},
		{"upload_image_from_url", "message", a.actionUploadImageFromURL},
		{"upload_file_from_url", "message", a.actionUploadFileFromURL},
		{"reply_message", "message", func(ctx context.Context, p map[string]any) (any, error) {
			msgType := stringValue(p["msg_type"])
			if msgType == "" {
				msgType = "text"
			}
			content := stringValue(p["content"])
			if msgType == "text" {
				content = `{"text":` + quote(content) + `}`
			}
			return nil, a.replyMessage(ctx, stringValue(p["message_id"]), msgType, content)
		}},
		{"recall_message", "message", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodDelete, "/open-apis/im/v1/messages/"+stringValue(p["message_id"]), nil, nil)
		}},
		{"get_message", "message", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/im/v1/messages/"+stringValue(p["message_id"]), nil, nil)
		}},
		{"get_message_list", "message", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			q.Set("container_id_type", "chat")
			q.Set("container_id", stringValue(p["chat_id"]))
			pageSize := intValue(p["page_size"])
			if pageSize <= 0 {
				pageSize = 20
			}
			q.Set("page_size", strconv.Itoa(pageSize))
			if t := stringValue(p["page_token"]); t != "" {
				q.Set("page_token", t)
			}
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/im/v1/messages", q, nil)
		}},
		{"add_reaction", "reaction", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/im/v1/messages/"+stringValue(p["message_id"])+"/reactions", nil, map[string]any{
				"reaction_type": map[string]any{"emoji_type": stringValue(p["emoji_type"])},
			})
		}},
		{"delete_reaction", "reaction", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodDelete, "/open-apis/im/v1/messages/"+stringValue(p["message_id"])+"/reactions/"+stringValue(p["reaction_id"]), nil, nil)
		}},
		{"get_reactions", "reaction", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			if t := stringValue(p["emoji_type"]); t != "" {
				q.Set("reaction_type", t)
			}
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/im/v1/messages/"+stringValue(p["message_id"])+"/reactions", q, nil)
		}},
		{"create_chat", "chat", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/im/v1/chats", nil, map[string]any{
				"name":         p["name"],
				"description":  p["description"],
				"user_id_list": p["user_id_list"],
			})
		}},
		{"get_chat_info", "chat", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/im/v1/chats/"+stringValue(p["chat_id"]), nil, nil)
		}},
		{"get_chat_members", "chat", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/im/v1/chats/"+stringValue(p["chat_id"])+"/members", nil, nil)
		}},
		{"list_bot_chats", "chat", func(ctx context.Context, p map[string]any) (any, error) {
			_ = p
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/im/v1/chats", nil, nil)
		}},
		{"search_chats", "chat", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			q.Set("query", stringValue(p["query"]))
			size := intValue(p["page_size"])
			if size <= 0 {
				size = 20
			}
			q.Set("page_size", strconv.Itoa(size))
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/im/v1/chats/search", q, nil)
		}},
		{"batch_get_user_id", "user", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			q.Set("user_id_type", "open_id")
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/contact/v3/users/batch_get_id", q, map[string]any{
				"emails":  p["emails"],
				"mobiles": p["mobiles"],
			})
		}},
		{"get_user_info", "user", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			t := stringValue(p["user_id_type"])
			if t == "" {
				t = "open_id"
			}
			q.Set("user_id_type", t)
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/contact/v3/users/"+stringValue(p["user_id"]), q, nil)
		}},
		{"reserve_meeting", "meeting", a.actionReserveMeeting},
		{"get_meeting", "meeting", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			q.Set("with_participants", "true")
			q.Set("user_id_type", "open_id")
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/vc/v1/meetings/"+stringValue(p["meeting_id"]), q, nil)
		}},
		{"invite_to_meeting", "meeting", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			q.Set("user_id_type", "open_id")
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/vc/v1/meetings/"+stringValue(p["meeting_id"])+"/invite", q, map[string]any{
				"invitees": p["invitees"],
			})
		}},
		{"end_meeting", "meeting", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/vc/v1/meetings/"+stringValue(p["meeting_id"])+"/end", nil, map[string]any{})
		}},
		{"start_recording", "meeting", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/vc/v1/meetings/"+stringValue(p["meeting_id"])+"/recordings/start", nil, map[string]any{})
		}},
		{"stop_recording", "meeting", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/vc/v1/meetings/"+stringValue(p["meeting_id"])+"/recordings/stop", nil, map[string]any{})
		}},
		{"get_meeting_recording", "meeting", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/vc/v1/meetings/"+stringValue(p["meeting_id"])+"/recordings", nil, nil)
		}},
		{"create_document", "document", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/docx/v1/documents", nil, map[string]any{
				"title":        p["title"],
				"folder_token": p["folder_token"],
			})
		}},
		{"get_document", "document", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/docx/v1/documents/"+stringValue(p["document_id"]), nil, nil)
		}},
		{"get_document_content", "document", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/docx/v1/documents/"+stringValue(p["document_id"])+"/raw_content", nil, nil)
		}},
		{"get_document_blocks", "document", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			q.Set("page_size", "500")
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/docx/v1/documents/"+stringValue(p["document_id"])+"/blocks", q, nil)
		}},
		{"append_document", "document", func(ctx context.Context, p map[string]any) (any, error) {
			children, err := convertBlocks(p["blocks"])
			if err != nil {
				return nil, err
			}
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/docx/v1/documents/"+stringValue(p["document_id"])+"/blocks/"+stringValue(p["block_id"])+"/children", nil, map[string]any{
				"children": children,
				"index":    -1,
			})
		}},
		{"get_wiki_spaces", "wiki", func(ctx context.Context, p map[string]any) (any, error) {
			_ = p
			q := url.Values{}
			q.Set("page_size", "50")
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/wiki/v2/spaces", q, nil)
		}},
		{"get_wiki_node", "wiki", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			q.Set("page_size", "50")
			if t := stringValue(p["node_token"]); t != "" {
				q.Set("parent_node_token", t)
			}
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/wiki/v2/spaces/"+stringValue(p["space_id"])+"/nodes", q, nil)
		}},
		{"create_wiki_node", "wiki", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/wiki/v2/spaces/"+stringValue(p["space_id"])+"/nodes", nil, map[string]any{
				"obj_type":          "docx",
				"parent_node_token": p["parent_node_token"],
				"node_type":         defaultString(stringValue(p["node_type"]), "origin"),
				"title":             p["title"],
			})
		}},
		{"get_root_folder", "drive", func(ctx context.Context, p map[string]any) (any, error) {
			_ = p
			q := url.Values{}
			q.Set("page_size", "50")
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/drive/v1/files", q, nil)
		}},
		{"get_folder_contents", "drive", func(ctx context.Context, p map[string]any) (any, error) {
			q := url.Values{}
			q.Set("page_size", "50")
			q.Set("folder_token", stringValue(p["folder_token"]))
			return a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/drive/v1/files", q, nil)
		}},
		{"create_folder", "drive", func(ctx context.Context, p map[string]any) (any, error) {
			return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/drive/v1/files/create_folder", nil, map[string]any{
				"name":         p["name"],
				"folder_token": p["folder_token"],
			})
		}},
	}
	return defs
}

func (a *app) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	var req struct {
		Action string         `json:"action"`
		Params map[string]any `json:"params"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}

	defMap := make(map[string]actionDef)
	for _, d := range a.actionDefs() {
		defMap[d.Name] = d
	}
	d, ok := defMap[req.Action]
	if !ok {
		names := make([]string, 0, len(defMap))
		for name := range defMap {
			names = append(names, name)
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success":           false,
			"error":             "Unknown action: " + req.Action,
			"available_actions": names,
		})
		return
	}

	result, err := d.Handler(r.Context(), req.Params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success":     false,
			"error":       err.Error(),
			"errorDetail": err.Error(),
			"action":      req.Action,
		})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: result})
}

func (a *app) handleActionList(w http.ResponseWriter, r *http.Request) {
	_ = r
	defs := a.actionDefs()
	out := make([]map[string]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, map[string]string{"name": d.Name, "category": d.Category})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"total":   len(out),
		"actions": out,
	})
}

func (a *app) handleMessageRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/feishu/message/")
	if strings.HasPrefix(path, "list/") && r.Method == http.MethodGet {
		chatID := strings.TrimPrefix(path, "list/")
		q := url.Values{}
		q.Set("container_id_type", "chat")
		q.Set("container_id", chatID)
		size := parseIntOrDefault(r.URL.Query().Get("pageSize"), 20)
		q.Set("page_size", strconv.Itoa(size))
		if token := r.URL.Query().Get("pageToken"); token != "" {
			q.Set("page_token", token)
		}
		res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/im/v1/messages", q, nil)
		a.respondData(w, res, err)
		return
	}
	if strings.HasPrefix(path, "chat/") {
		sub := strings.TrimPrefix(path, "chat/")
		if strings.HasSuffix(sub, "/members") && r.Method == http.MethodGet {
			chatID := strings.TrimSuffix(sub, "/members")
			res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/im/v1/chats/"+chatID+"/members", nil, nil)
			a.respondData(w, res, err)
			return
		}
		if r.Method == http.MethodGet {
			res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/im/v1/chats/"+sub, nil, nil)
			a.respondData(w, res, err)
			return
		}
	}

	messageID := path
	switch r.Method {
	case http.MethodGet:
		res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/im/v1/messages/"+messageID, nil, nil)
		a.respondData(w, res, err)
	case http.MethodDelete:
		res, err := a.feishu.doJSON(r.Context(), http.MethodDelete, "/open-apis/im/v1/messages/"+messageID, nil, nil)
		a.respondData(w, res, err)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
	}
}

func (a *app) handleCreateChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	if stringValue(body["name"]) == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "name is required"})
		return
	}
	res, err := a.feishu.doJSON(r.Context(), http.MethodPost, "/open-apis/im/v1/chats", nil, map[string]any{
		"name":         body["name"],
		"description":  body["description"],
		"user_id_list": body["userIdList"],
	})
	a.respondData(w, res, err)
}

func (a *app) handleReserveMeeting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	res, err := a.actionReserveMeeting(r.Context(), body)
	a.respondData(w, wrapData(res), err)
}

func (a *app) actionReserveMeeting(ctx context.Context, p map[string]any) (any, error) {
	topic := stringValue(p["topic"])
	start := stringValue(p["startTime"])
	if start == "" {
		start = stringValue(p["start_time"])
	}
	end := stringValue(p["endTime"])
	if end == "" {
		end = stringValue(p["end_time"])
	}
	if topic == "" || start == "" || end == "" {
		return nil, fmt.Errorf("topic, startTime, and endTime are required")
	}

	inviteesAny, _ := p["invitees"].([]any)
	callees := make([]map[string]any, 0, len(inviteesAny))
	for _, v := range inviteesAny {
		m := mapStringAny(v)
		id := stringValue(m["id"])
		if id == "" {
			continue
		}
		callees = append(callees, map[string]any{
			"id":        id,
			"user_type": 1,
		})
	}
	settings := map[string]any{
		"topic": topic,
	}
	if s := mapStringAny(p["settings"]); stringValue(s["password"]) != "" {
		settings["meeting_connect_setting"] = map[string]any{"password": stringValue(s["password"])}
	}
	q := url.Values{}
	q.Set("user_id_type", "open_id")
	return a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/vc/v1/reserves/apply", q, map[string]any{
		"start_time":       start,
		"end_time":         end,
		"meeting_settings": settings,
	})
}

func (a *app) handleMeetingRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/feishu/meeting/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "not found"})
		return
	}
	meetingID := parts[0]

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		q := url.Values{}
		q.Set("with_participants", "true")
		q.Set("user_id_type", "open_id")
		res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/vc/v1/meetings/"+meetingID, q, nil)
		a.respondData(w, res, err)
	case len(parts) == 2 && parts[1] == "invite" && r.Method == http.MethodPost:
		var body map[string]any
		if err := decodeJSON(r.Body, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
			return
		}
		q := url.Values{}
		q.Set("user_id_type", "open_id")
		res, err := a.feishu.doJSON(r.Context(), http.MethodPost, "/open-apis/vc/v1/meetings/"+meetingID+"/invite", q, map[string]any{"invitees": body["invitees"]})
		a.respondData(w, res, err)
	case len(parts) == 2 && parts[1] == "end" && r.Method == http.MethodPost:
		res, err := a.feishu.doJSON(r.Context(), http.MethodPost, "/open-apis/vc/v1/meetings/"+meetingID+"/end", nil, map[string]any{})
		a.respondData(w, res, err)
	case len(parts) == 2 && parts[1] == "recording" && r.Method == http.MethodGet:
		res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/vc/v1/meetings/"+meetingID+"/recordings", nil, nil)
		a.respondData(w, res, err)
	case len(parts) == 3 && parts[1] == "recording" && parts[2] == "start" && r.Method == http.MethodPost:
		res, err := a.feishu.doJSON(r.Context(), http.MethodPost, "/open-apis/vc/v1/meetings/"+meetingID+"/recordings/start", nil, map[string]any{})
		a.respondData(w, res, err)
	case len(parts) == 3 && parts[1] == "recording" && parts[2] == "stop" && r.Method == http.MethodPost:
		res, err := a.feishu.doJSON(r.Context(), http.MethodPost, "/open-apis/vc/v1/meetings/"+meetingID+"/recordings/stop", nil, map[string]any{})
		a.respondData(w, res, err)
	default:
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "not found"})
	}
}

func (a *app) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	if stringValue(body["title"]) == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "title is required"})
		return
	}
	res, err := a.feishu.doJSON(r.Context(), http.MethodPost, "/open-apis/docx/v1/documents", nil, map[string]any{
		"title":        body["title"],
		"folder_token": body["folderToken"],
	})
	a.respondData(w, res, err)
}

func (a *app) handleDocumentRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/feishu/document/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "not found"})
		return
	}
	documentID := parts[0]

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/docx/v1/documents/"+documentID, nil, nil)
		a.respondData(w, res, err)
	case len(parts) == 2 && parts[1] == "raw" && r.Method == http.MethodGet:
		res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/docx/v1/documents/"+documentID+"/raw_content", nil, nil)
		a.respondData(w, res, err)
	case len(parts) == 2 && parts[1] == "blocks" && r.Method == http.MethodGet:
		q := url.Values{}
		q.Set("page_size", "500")
		res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/docx/v1/documents/"+documentID+"/blocks", q, nil)
		a.respondData(w, res, err)
	case len(parts) == 2 && parts[1] == "blocks" && r.Method == http.MethodPost:
		var body map[string]any
		if err := decodeJSON(r.Body, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
			return
		}
		blockID := stringValue(body["blockId"])
		if blockID == "" {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "blockId is required"})
			return
		}
		children, err := convertBlocks(body["blocks"])
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
			return
		}
		res, err := a.feishu.doJSON(r.Context(), http.MethodPost, "/open-apis/docx/v1/documents/"+documentID+"/blocks/"+blockID+"/children", nil, map[string]any{
			"children": children,
			"index":    -1,
		})
		a.respondData(w, res, err)
	default:
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "not found"})
	}
}

func (a *app) handleGetWikiSpaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	q := url.Values{}
	q.Set("page_size", "50")
	res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/wiki/v2/spaces", q, nil)
	a.respondData(w, res, err)
}

func (a *app) handleWikiRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/feishu/document/wiki/")
	if !strings.HasSuffix(path, "/nodes") {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "not found"})
		return
	}
	spaceID := strings.TrimSuffix(path, "/nodes")
	q := url.Values{}
	q.Set("page_size", "50")
	if parent := r.URL.Query().Get("parentNodeToken"); parent != "" {
		q.Set("parent_node_token", parent)
	}
	res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/wiki/v2/spaces/"+spaceID+"/nodes", q, nil)
	a.respondData(w, res, err)
}

func (a *app) handleCreateWikiNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	spaceID := stringValue(body["spaceId"])
	title := stringValue(body["title"])
	if spaceID == "" || title == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "spaceId and title are required"})
		return
	}
	res, err := a.feishu.doJSON(r.Context(), http.MethodPost, "/open-apis/wiki/v2/spaces/"+spaceID+"/nodes", nil, map[string]any{
		"obj_type":          "docx",
		"parent_node_token": body["parentNodeToken"],
		"node_type":         defaultString(stringValue(body["nodeType"]), "origin"),
		"title":             title,
	})
	a.respondData(w, res, err)
}

func (a *app) handleDriveFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	q := url.Values{}
	q.Set("page_size", "50")
	if folder := r.URL.Query().Get("folderToken"); folder != "" {
		q.Set("folder_token", folder)
	}
	res, err := a.feishu.doJSON(r.Context(), http.MethodGet, "/open-apis/drive/v1/files", q, nil)
	a.respondData(w, res, err)
}

func (a *app) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	name := stringValue(body["name"])
	if name == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "name is required"})
		return
	}
	res, err := a.feishu.doJSON(r.Context(), http.MethodPost, "/open-apis/drive/v1/files/create_folder", nil, map[string]any{
		"name":         name,
		"folder_token": body["folderToken"],
	})
	a.respondData(w, res, err)
}

func (a *app) actionUploadImageFromURL(ctx context.Context, p map[string]any) (any, error) {
	imageURL := stringValue(p["image_url"])
	if imageURL == "" {
		return nil, fmt.Errorf("image_url is required")
	}
	resp, err := a.http.Get(imageURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download image failed: %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	key, err := a.feishu.uploadImage(ctx, defaultString(stringValue(p["image_type"]), "message"), data, "image_from_url")
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (a *app) actionUploadFileFromURL(ctx context.Context, p map[string]any) (any, error) {
	fileURL := stringValue(p["file_url"])
	fileName := stringValue(p["file_name"])
	fileType := stringValue(p["file_type"])
	duration := stringValue(p["duration"])
	if fileURL == "" || fileName == "" || fileType == "" {
		return nil, fmt.Errorf("file_url, file_name, file_type are required")
	}
	resp, err := a.http.Get(fileURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download file failed: %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	key, err := a.feishu.uploadFile(ctx, fileType, fileName, data, duration)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (a *app) respondData(w http.ResponseWriter, data any, err error) {
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: data})
}

func wrapData(v any) map[string]any {
	return map[string]any{"data": v}
}

func defaultType(p map[string]any) string {
	v := stringValue(p["receive_id_type"])
	if v == "" {
		return "chat_id"
	}
	return v
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func convertBlocks(v any) ([]map[string]any, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("blocks must be an array")
	}
	children := make([]map[string]any, 0, len(items))
	for _, item := range items {
		block := mapStringAny(item)
		blockType := stringValue(block["blockType"])
		switch blockType {
		case "paragraph":
			textRun := map[string]any{"content": stringValue(block["text"]), "text_element_style": map[string]any{}}
			style := stringValue(block["style"])
			headingLevel := 0
			switch style {
			case "heading1":
				headingLevel = 1
			case "heading2":
				headingLevel = 2
			case "heading3":
				headingLevel = 3
			case "heading4":
				headingLevel = 4
			}
			if headingLevel > 0 {
				children = append(children, map[string]any{
					"block_type": headingLevel + 2,
					"heading": map[string]any{
						"elements": []any{map[string]any{"text_run": textRun}},
					},
				})
			} else {
				children = append(children, map[string]any{
					"block_type": 2,
					"paragraph": map[string]any{
						"elements": []any{map[string]any{"text_run": textRun}},
					},
				})
			}
		case "code":
			children = append(children, map[string]any{
				"block_type": 14,
				"code": map[string]any{
					"elements": []any{
						map[string]any{"text_run": map[string]any{
							"content":            stringValue(block["code"]),
							"text_element_style": map[string]any{},
						}},
					},
					"style": map[string]any{
						"language": mapCodeLanguage(stringValue(block["language"])),
					},
				},
			})
		case "callout":
			children = append(children, map[string]any{
				"block_type": 19,
				"callout": map[string]any{
					"elements": []any{map[string]any{"text_run": map[string]any{
						"content":            stringValue(block["text"]),
						"text_element_style": map[string]any{},
					}}},
				},
			})
		case "divider":
			children = append(children, map[string]any{"block_type": 22, "divider": map[string]any{}})
		default:
			children = append(children, map[string]any{
				"block_type": 2,
				"paragraph": map[string]any{
					"elements": []any{map[string]any{"text_run": map[string]any{
						"content":            stringValue(block["text"]),
						"text_element_style": map[string]any{},
					}}},
				},
			})
		}
	}
	return children, nil
}

func mapCodeLanguage(lang string) int {
	lang = strings.ToLower(strings.TrimSpace(lang))
	m := map[string]int{
		"plaintext": 1, "bash": 7, "c": 9, "c#": 10, "c++": 11, "css": 15, "dart": 18,
		"dockerfile": 20, "go": 24, "html": 28, "java": 30, "javascript": 31, "json": 33,
		"kotlin": 35, "markdown": 39, "python": 49, "ruby": 52, "rust": 53, "shell": 56,
		"sql": 58, "swift": 60, "typescript": 63, "xml": 66, "yaml": 67,
	}
	if v, ok := m[lang]; ok {
		return v
	}
	return 1
}

type ttlCache struct {
	ttl  time.Duration
	mu   syncMapMutex
	data map[string]cacheEntry
}

type cacheEntry struct {
	Value   string
	Expires time.Time
}

type syncMapMutex struct{ ch chan struct{} }

func newSyncMapMutex() syncMapMutex {
	m := syncMapMutex{ch: make(chan struct{}, 1)}
	m.ch <- struct{}{}
	return m
}

func (m *syncMapMutex) Lock()   { <-m.ch }
func (m *syncMapMutex) Unlock() { m.ch <- struct{}{} }

func newTTLCache(ttl time.Duration) *ttlCache {
	return &ttlCache{
		ttl:  ttl,
		mu:   newSyncMapMutex(),
		data: map[string]cacheEntry{},
	}
}

func (c *ttlCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.data[key]
	if !ok || time.Now().After(entry.Expires) {
		delete(c.data, key)
		return "", false
	}
	return entry.Value, true
}

func (c *ttlCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = cacheEntry{Value: value, Expires: time.Now().Add(c.ttl)}
}
