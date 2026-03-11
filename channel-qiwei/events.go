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

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

const tagCallback = "callback"

var userMessageTypeMap = map[int]string{
	1:   "text",
	2:   "text",
	3:   "image",
	14:  "image",
	15:  "file",
	16:  "voice",
	23:  "video",
	34:  "voice",
	43:  "video",
	49:  "file",
	101: "image",
	102: "file",
	103: "video",
}

func (a *app) handleWebhookCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	ctx := r.Context()

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 200, "msg": "ok"})
		return
	}
	messages, err := parseCallbackMessages(rawBody)
	if err != nil {
		logger.Warn(ctx, "callback 解析失败", "tag", tagCallback, "error", err.Error(), "body", string(rawBody))
		writeJSON(w, http.StatusOK, map[string]any{"code": 200, "msg": "ok"})
		return
	}
	logger.Business(ctx, "callback 接收", "tag", tagCallback, "messages", len(messages))

	writeJSON(w, http.StatusOK, map[string]any{"code": 200, "msg": "ok"})

	for _, msg := range messages {
		msg := msg
		go func() {
			msgCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			// Carry requestId from the webhook request into the async goroutine.
			if rid := logger.GetRequestID(ctx); rid != "" {
				msgCtx = logger.WithRequestID(msgCtx, rid)
			}
			if err := a.handleCallbackMessage(msgCtx, msg); err != nil {
				logger.Error(msgCtx, "callback 处理失败", "tag", tagCallback, "msg", msg.MsgSvrID, "error", err.Error())
			}
		}()
	}
}

func (a *app) handleCallbackMessage(ctx context.Context, msg qiweiCallbackMessage) error {
	if msg.MsgSvrID != "" && a.dedupe.Seen(msg.MsgSvrID) {
		logger.Detail(ctx, "跳过重复消息", "tag", tagCallback, "msg", msg.MsgSvrID)
		return nil
	}

	messageType := userMessageTypeMap[msg.MsgType]
	if messageType == "" {
		rawMsgData, _ := json.Marshal(msg.MsgData)
		logger.Warn(ctx, "跳过不支持的消息类型",
			"tag", tagCallback,
			"msgType", msg.MsgType,
			"msg", msg.MsgSvrID,
			"senderId", msg.SenderID,
			"fromRoomId", msg.FromRoomID,
			"msgData", string(rawMsgData),
		)
		return nil
	}
	isGroup := msg.FromRoomID != "" && msg.FromRoomID != "0"
	conversationType := "p2p"
	if isGroup {
		conversationType = "group"
	}

	content := ""
	var attachments []incomingAttachment
	if msg.MsgType == 1 || msg.MsgType == 2 {
		content = strings.TrimSpace(stringValue(msg.MsgData["content"]))
		if content == "" {
			rawMsgData, _ := json.Marshal(msg.MsgData)
			logger.Warn(ctx, "跳过空文本消息",
				"tag", tagCallback,
				"msg", msg.MsgSvrID,
				"senderId", msg.SenderID,
				"fromRoomId", msg.FromRoomID,
				"msgData", string(rawMsgData),
			)
			return nil
		}
	} else {
		prepared := a.prepareMediaForAgent(ctx, msg.MsgType, messageType, msg.MsgData)
		if prepared.MessageType != "" {
			messageType = prepared.MessageType
		}
		if messageType == "voice" {
			content = strings.TrimSpace(prepared.Content)
		} else {
			if strings.TrimSpace(prepared.ResourceURI) == "" {
				logger.Error(ctx, "媒体上传失败，跳过",
					"tag", tagCallback,
					"msg", msg.MsgSvrID,
					"type", messageType,
				)
				return nil
			}
			content = strings.TrimSpace(prepared.ResourceURI)
			attachments = attachmentsFromPreparedMedia(msg.MsgSvrID, messageType, prepared)
		}
	}

	senderName := msg.SenderNickname
	if senderName == "" && msg.SenderID != "" {
		senderName = a.resolveUserName(ctx, msg.SenderID)
	}
	msgTime := time.Now()
	if msg.CreateTime > 0 {
		msgTime = time.Unix(msg.CreateTime, 0)
	}
	prefix := formatSenderPrefix(senderName, msg.SenderID, msgTime)
	if messageType == "voice" {
		content = formatVoiceContent(prefix, content)
	} else {
		content = prefix + content
	}

	replyToID := msg.SenderID
	if isGroup {
		replyToID = msg.FromRoomID
	}

	if !a.cfg.AgentEnabled {
		logger.Business(ctx, "echo 模式", "tag", tagCallback, "msg", msg.MsgSvrID, "to", replyToID, "type", messageType)
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
		Attachments:             attachments,
		AgentID:                 a.cfg.AgentID,
	}
	logger.Business(ctx, "转发消息到 agent",
		"tag", tagCallback,
		"msg", msg.MsgSvrID,
		"conversation", in.ChannelConversationID,
		"type", in.MessageType,
		"sender", senderName,
	)
	err := a.forwardToAgent(ctx, in)
	if err != nil {
		return err
	}
	logger.Detail(ctx, "转发完成", "tag", tagCallback, "msg", msg.MsgSvrID)
	return nil
}

func attachmentsFromPreparedMedia(messageID, messageType string, prepared preparedMedia) []incomingAttachment {
	resourceURI := strings.TrimSpace(prepared.ResourceURI)
	if resourceURI == "" {
		return nil
	}
	switch messageType {
	case "image", "file", "video":
	default:
		return nil
	}
	return []incomingAttachment{{
		ID:                strings.TrimSpace(messageID) + ":0",
		Kind:              messageType,
		ResourceURI:       resourceURI,
		DisplayName:       strings.TrimSpace(prepared.Name),
		MIMEType:          strings.TrimSpace(prepared.MIMEType),
		SourceMessageType: strings.TrimSpace(messageType),
	}}
}

func formatSenderPrefix(name, id string, t time.Time) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	if name == "" {
		name = firstNonEmpty(id, "未知用户")
	}
	ts := t.Local().Format("2006-01-02 15:04:05")
	if id == "" {
		return name + " " + ts + ":"
	}
	return name + "[" + id + "] " + ts + ":"
}

func formatVoiceContent(prefix, transcript string) string {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return prefix + "转写失败(语音消息)"
	}
	return prefix + transcript + "(语音消息)"
}

func (a *app) resolveUserName(ctx context.Context, userID string) string {
	if v, ok := a.nameCache.Get(userID); ok {
		return v
	}
	a.loadContactsOnce(ctx)
	if v, ok := a.nameCache.Get(userID); ok {
		return v
	}
	return ""
}

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
		logger.Warn(ctx, "加载外部联系人失败", "error", err.Error())
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
	logger.Business(ctx, "加载外部联系人", "cached", len(wrapper.ContactList))
}

func (a *app) loadInternalContacts(ctx context.Context) {
	res, err := a.client.doAPIRaw(ctx, "/contact/getWxWorkContactList", nil)
	if err != nil {
		logger.Warn(ctx, "加载内部联系人失败", "error", err.Error())
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
	logger.Business(ctx, "加载内部联系人", "cached", cached)
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
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("agent server error: %d body=%s", resp.StatusCode, string(respBody))
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

	var standard qiweiCallbackBody
	if err := json.Unmarshal(trimmed, &standard); err == nil && len(standard.Data) > 0 {
		return standard.Data, nil
	}

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
