package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const tagCallback = "callback"

var userMessageTypeMap = map[int]string{
	1:  "text",
	2:  "text",
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

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 200, "msg": "ok"})
		return
	}
	messages, err := parseCallbackMessages(rawBody)
	if err != nil {
		a.log.Warn("callback parse failed", "tag", tagCallback, "err", err, "body", truncateBody(rawBody, 600))
		writeJSON(w, http.StatusOK, map[string]any{"code": 200, "msg": "ok"})
		return
	}
	a.log.Info("callback received", "tag", tagCallback, "messages", len(messages))

	writeJSON(w, http.StatusOK, map[string]any{"code": 200, "msg": "ok"})

	for _, msg := range messages {
		msg := msg
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := a.handleCallbackMessage(ctx, msg); err != nil {
				a.log.Error("callback process failed", "tag", tagCallback, "msg", msg.MsgSvrID, "err", err)
			}
		}()
	}
}

func (a *app) handleCallbackMessage(ctx context.Context, msg qiweiCallbackMessage) error {
	if msg.MsgSvrID != "" && a.dedupe.Seen(msg.MsgSvrID) {
		a.log.Debug("skip duplicate", "tag", tagCallback, "msg", msg.MsgSvrID)
		return nil
	}

	messageType := userMessageTypeMap[msg.MsgType]
	if messageType == "" {
		a.log.Debug("skip unsupported msgType", "tag", tagCallback, "msgType", msg.MsgType, "msg", msg.MsgSvrID)
		return nil
	}
	isGroup := msg.FromRoomID != "" && msg.FromRoomID != "0"
	conversationType := "p2p"
	if isGroup {
		conversationType = "group"
	}

	content := ""
	if msg.MsgType == 1 || msg.MsgType == 2 {
		content = strings.TrimSpace(stringValue(msg.MsgData["content"]))
		if content == "" {
			a.log.Debug("skip empty text", "tag", tagCallback, "msg", msg.MsgSvrID)
			return nil
		}
	} else {
		raw, _ := json.Marshal(msg.MsgData)
		content = string(raw)
	}

	senderName := msg.SenderNickname
	if senderName == "" && msg.SenderID != "" {
		senderName = a.resolveUserName(ctx, msg.SenderID)
	}
	if senderName != "" {
		content = senderName + ": " + content
	}

	replyToID := msg.SenderID
	if isGroup {
		replyToID = msg.FromRoomID
	}

	if !a.cfg.AgentEnabled {
		a.log.Info("echo mode", "tag", tagCallback, "msg", msg.MsgSvrID, "to", replyToID, "type", messageType)
		if msg.MsgType == 1 {
			_, err := a.client.doAPIRaw(ctx, "/msg/sendText", map[string]any{
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
		ChannelConversationName: senderName,
		ConversationType:        conversationType,
		MessageType:             messageType,
		Content:                 content,
		SenderName:              senderName,
		Timestamp:               ts,
		ChannelMeta: map[string]any{
			"guid":       msg.GUID,
			"msgType":    msg.MsgType,
			"fromRoomId": msg.FromRoomID,
		},
		AgentID: a.cfg.AgentID,
	}
	a.log.Info("forwarding to agent", "tag", tagCallback, "msg", msg.MsgSvrID, "conversation", in.ChannelConversationID, "type", in.MessageType, "sender", senderName)
	err := a.forwardToAgent(ctx, in)
	if err != nil {
		return err
	}
	a.log.Debug("forwarded", "tag", tagCallback, "msg", msg.MsgSvrID)
	return nil
}

func (a *app) resolveUserName(ctx context.Context, userID string) string {
	if v, ok := a.nameCache.Get(userID); ok {
		return v
	}

	// batchGetUserinfo only returns the bot's own profile, not arbitrary users.
	// Load the full contact lists and cache every userId → name mapping.
	a.loadContactsOnce(ctx)

	if v, ok := a.nameCache.Get(userID); ok {
		return v
	}
	return ""
}

// loadContactsOnce fetches external + internal contact lists and populates nameCache.
// Guarded by contactsLoaded to avoid repeated bulk fetches within the cache TTL window.
func (a *app) loadContactsOnce(ctx context.Context) {
	a.contactsMu.Lock()
	if time.Since(a.contactsLoadedAt) < 5*time.Minute {
		a.contactsMu.Unlock()
		return
	}
	a.contactsMu.Unlock()

	a.loadExternalContacts(ctx)
	a.loadInternalContacts(ctx)

	a.contactsMu.Lock()
	a.contactsLoadedAt = time.Now()
	a.contactsMu.Unlock()
}

func (a *app) loadExternalContacts(ctx context.Context) {
	res, err := a.client.doAPIRaw(ctx, "/contact/getWxContactList", nil)
	if err != nil {
		a.log.Warn("loadExternalContacts failed", "err", err)
		return
	}
	var wrapper struct {
		ContactList []map[string]any `json:"contactList"`
	}
	if err := json.Unmarshal(res.Data, &wrapper); err != nil {
		return
	}
	for _, c := range wrapper.ContactList {
		uid := anyToString(c["userId"])
		name := firstNonEmpty(
			anyToString(c["nickname"]),
			anyToString(c["realName"]),
			anyToString(c["remark"]),
			anyToString(c["alias"]),
		)
		if uid != "" && name != "" {
			a.nameCache.Set(uid, name)
		}
	}
	a.log.Info("loadExternalContacts", "cached", len(wrapper.ContactList))
}

func (a *app) loadInternalContacts(ctx context.Context) {
	res, err := a.client.doAPIRaw(ctx, "/contact/getWxWorkContactList", nil)
	if err != nil {
		a.log.Warn("loadInternalContacts failed", "err", err)
		return
	}
	var wrapper struct {
		ContactList []map[string]any `json:"contactList"`
	}
	if err := json.Unmarshal(res.Data, &wrapper); err != nil {
		return
	}
	cached := 0
	for _, c := range wrapper.ContactList {
		uid := anyToString(c["userId"])
		name := firstNonEmpty(
			anyToString(c["nickname"]),
			anyToString(c["realName"]),
			anyToString(c["remark"]),
			anyToString(c["name"]),
		)
		if uid != "" && uid != "0" && name != "" {
			a.nameCache.Set(uid, name)
			cached++
		}
	}
	a.log.Info("loadInternalContacts", "cached", cached)
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

func parseCallbackMessages(raw []byte) ([]qiweiCallbackMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}

	// Fast path: standard callback envelope {code,msg,data:[...]}
	var standard qiweiCallbackBody
	if err := json.Unmarshal(trimmed, &standard); err == nil && len(standard.Data) > 0 {
		return standard.Data, nil
	}

	// Compatibility path: tolerate data object/string/single message payload.
	var payload any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, err
	}

	switch v := payload.(type) {
	case []any:
		return decodeMessageArray(v)
	case map[string]any:
		if isVerificationPayload(v) {
			return nil, nil
		}
		// Verification callback may only contain a text prompt.
		if strings.Contains(stringValue(v["content"]), "验证回调地址是否可用") {
			return nil, nil
		}
		if data, ok := v["data"]; ok {
			switch dv := data.(type) {
			case []any:
				return decodeMessageArray(dv)
			case map[string]any:
				msg, err := decodeOneMessage(dv)
				if err != nil {
					return nil, err
				}
				return []qiweiCallbackMessage{msg}, nil
			case string:
				if strings.Contains(dv, "验证回调地址是否可用") {
					return nil, nil
				}
				return nil, fmt.Errorf("unsupported callback data string")
			}
		}

		// Some callback implementations may push a single message directly.
		if _, hasMsgType := v["msgType"]; hasMsgType {
			msg, err := decodeOneMessage(v)
			if err != nil {
				return nil, err
			}
			return []qiweiCallbackMessage{msg}, nil
		}
		return nil, fmt.Errorf("unsupported callback payload shape")
	default:
		return nil, fmt.Errorf("unsupported callback payload type")
	}
}

func isVerificationPayload(v map[string]any) bool {
	content := firstNonEmpty(
		stringValue(v["content"]),
		stringValue(v["testMsg"]),
		stringValue(v["message"]),
		stringValue(v["msg"]),
	)
	if strings.Contains(content, "验证回调地址是否可用") || strings.Contains(content, "回调地址链接成功") {
		return true
	}
	if strings.TrimSpace(stringValue(v["token"])) != "" && strings.TrimSpace(stringValue(v["testMsg"])) != "" {
		return true
	}
	return false
}

func decodeMessageArray(items []any) ([]qiweiCallbackMessage, error) {
	out := make([]qiweiCallbackMessage, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		msg, err := decodeOneMessage(m)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, nil
}

func decodeOneMessage(in map[string]any) (qiweiCallbackMessage, error) {
	// QiWe callback fields are not stable across versions:
	// some ids may be numeric, and field names may use msgServerId/timestamp/senderName.
	msg := qiweiCallbackMessage{
		GUID:           anyToString(in["guid"]),
		MsgType:        int(anyToInt64(in["msgType"])),
		MsgData:        mapValue(in["msgData"]),
		SenderID:       firstNonEmpty(anyToString(in["senderId"]), anyToString(in["senderID"])),
		SenderNickname: firstNonEmpty(anyToString(in["senderNickname"]), anyToString(in["senderName"])),
		FromRoomID:     anyToString(in["fromRoomId"]),
		MsgSvrID:       firstNonEmpty(anyToString(in["msgSvrId"]), anyToString(in["msgServerId"])),
		CreateTime:     firstNonZero(anyToInt64(in["createTime"]), anyToInt64(in["timestamp"])),
	}
	if msg.FromRoomID == "" {
		msg.FromRoomID = "0"
	}
	return msg, nil
}

func truncateBody(raw []byte, max int) string {
	s := string(bytes.TrimSpace(raw))
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return fmt.Sprintf("%.0f", t)
	case float32:
		return fmt.Sprintf("%.0f", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case int32:
		return fmt.Sprintf("%d", t)
	case uint64:
		return fmt.Sprintf("%d", t)
	case uint32:
		return fmt.Sprintf("%d", t)
	case uint:
		return fmt.Sprintf("%d", t)
	default:
		return ""
	}
}

func anyToInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case uint64:
		return int64(t)
	case uint:
		return int64(t)
	case float64:
		return int64(t)
	case float32:
		return int64(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0
		}
		return n
	case string:
		var n json.Number = json.Number(t)
		i, err := n.Int64()
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

func mapValue(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstNonZero(values ...int64) int64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
