package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var userMessageTypeMap = map[int]string{
	1:  "text",
	3:  "image",
	34: "voice",
	43: "video",
	47: "sticker",
	49: "file",
}

func (a *app) handleWebhookCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	var body qiweiCallbackBody
	if err := decodeJSON(r.Body, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid callback body"})
		return
	}

	// Callback must be acknowledged quickly, process payload asynchronously.
	writeJSON(w, http.StatusOK, map[string]any{"code": 200, "msg": "ok"})

	for _, msg := range body.Data {
		msg := msg
		go func() {
			if err := a.handleCallbackMessage(r.Context(), msg); err != nil {
				fmt.Printf("failed to process callback msg=%s err=%v\n", msg.MsgSvrID, err)
			}
		}()
	}
}

func (a *app) handleCallbackMessage(ctx context.Context, msg qiweiCallbackMessage) error {
	if msg.MsgSvrID != "" && a.dedupe.Seen(msg.MsgSvrID) {
		return nil
	}

	messageType := userMessageTypeMap[msg.MsgType]
	if messageType == "" {
		return nil
	}
	isGroup := msg.FromRoomID != "" && msg.FromRoomID != "0"
	conversationType := "p2p"
	if isGroup {
		conversationType = "group"
	}

	content := ""
	if msg.MsgType == 1 {
		content = strings.TrimSpace(stringValue(msg.MsgData["content"]))
		if content == "" {
			return nil
		}
	} else {
		raw, _ := json.Marshal(msg.MsgData)
		content = string(raw)
	}

	replyToID := msg.SenderID
	if isGroup {
		replyToID = msg.FromRoomID
	}

	if !a.cfg.AgentEnabled {
		if msg.MsgType == 1 {
			_, err := a.client.doAPIRaw(ctx, "msg/sendText", map[string]any{
				"toId":    replyToID,
				"content": "收到消息: " + content,
			})
			return err
		}
		return nil
	}

	ts := msg.CreateTime * 1000
	if ts == 0 {
		ts = time.Now().UnixMilli()
	}
	in := incomingMessage{
		Channel:                 "qiwei",
		ChannelUserID:           msg.SenderID,
		ChannelMessageID:        msg.MsgSvrID,
		ChannelConversationID:   replyToID,
		ChannelConversationName: msg.SenderNickname,
		ConversationType:        conversationType,
		MessageType:             messageType,
		Content:                 content,
		SenderName:              msg.SenderNickname,
		Timestamp:               ts,
		ChannelMeta: map[string]any{
			"guid":       msg.GUID,
			"msgType":    msg.MsgType,
			"fromRoomId": msg.FromRoomID,
		},
		AgentID: a.cfg.AgentID,
	}
	return a.forwardToAgent(ctx, in)
}

func (a *app) forwardToAgent(ctx context.Context, in incomingMessage) error {
	raw, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.AgentServer+"/api/channels/incoming", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("agent server error: %d", resp.StatusCode)
	}
	return nil
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}
