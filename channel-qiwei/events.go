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
	0:   "text",
	1:   "text",
	2:   "text",
	3:   "image",
	6:   "location",
	7:   "image",
	13:  "link",
	14:  "image",
	15:  "file",
	16:  "voice",
	23:  "video",
	26:  "red_packet",
	29:  "sticker",
	34:  "voice",
	41:  "card",
	43:  "video",
	// msgType 49 (appmsg) is handled in handleNormalMessage switch by subType.
	// 49:  handled per subType (quote=57, file=6, link=5, etc.)
	78:  "miniapp",
	101: "image",
	102: "file",
	103: "video",
	104: "sticker",
	123: "mixed",
	141: "channel_msg",
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
	switch msg.Cmd {
	case 15000:
		return a.handleNormalMessage(ctx, msg)
	case 15500:
		return a.handleSystemEvent(ctx, msg)
	case 11016:
		logger.Detail(ctx, "账号状态变化", "tag", tagCallback, "msgType", msg.MsgType, "guid", msg.GUID)
		return nil
	case 20000:
		logger.Detail(ctx, "API 异步消息", "tag", tagCallback, "msgType", msg.MsgType, "guid", msg.GUID)
		return nil
	default:
		logger.Detail(ctx, "未处理的 cmd 类型", "tag", tagCallback, "cmd", msg.Cmd, "msgType", msg.MsgType)
		return nil
	}
}

// textMessageTypes is the set of msgTypes that carry plain text content.
var textMessageTypes = map[int]bool{0: true, 1: true, 2: true}

// richContentTypes carry structured data that should be extracted into readable text,
// not routed through the media download pipeline.
var richContentTypes = map[string]bool{
	"link": true, "location": true, "card": true,
	"red_packet": true, "miniapp": true, "channel_msg": true,
}

func (a *app) handleNormalMessage(ctx context.Context, msg qiweiCallbackMessage) error {
	if msg.MsgSvrID != "" && a.dedupe.Seen(msg.MsgSvrID) {
		logger.Detail(ctx, "跳过重复消息", "tag", tagCallback, "msg", msg.MsgSvrID)
		return nil
	}

	messageType := userMessageTypeMap[msg.MsgType]
	if messageType == "" && msg.MsgType != 49 {
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
	var channelMeta map[string]any

	switch {
	case textMessageTypes[msg.MsgType]:
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

	case msg.MsgType == 49:
		content, channelMeta, messageType = a.handleAppMessage(ctx, msg)
		if content == "" && messageType == "file" {
			prepared := a.prepareMediaForAgent(ctx, msg.MsgType, "file", msg.MsgData)
			if prepared.MessageType != "" {
				messageType = prepared.MessageType
			}
			if strings.TrimSpace(prepared.ResourceURI) == "" {
				logger.Error(ctx, "媒体上传失败，跳过",
					"tag", tagCallback, "msg", msg.MsgSvrID, "type", messageType)
				return nil
			}
			content = strings.TrimSpace(prepared.ResourceURI)
			attachments = attachmentsFromPreparedMedia(msg.MsgSvrID, messageType, prepared)
		}

	case richContentTypes[messageType]:
		content, channelMeta = a.extractRichContent(messageType, msg.MsgData)

	case messageType == "mixed":
		content, attachments = a.handleMixedMessage(ctx, msg)

	case messageType == "sticker":
		prepared := a.prepareMediaForAgent(ctx, msg.MsgType, "image", msg.MsgData)
		if prepared.ResourceURI != "" {
			content = prepared.Content
			attachments = attachmentsFromPreparedMedia(msg.MsgSvrID, "image", prepared)
			messageType = "sticker"
		} else {
			content = "[表情]"
		}

	default:
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
		if textMessageTypes[msg.MsgType] {
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
		ChannelMeta:             channelMeta,
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

// extractRichContent converts structured message types (link, location, card, etc.)
// into human-readable text and optional channelMeta for later forwarding.
func (a *app) extractRichContent(messageType string, msgData map[string]any) (string, map[string]any) {
	switch messageType {
	case "link":
		return contentFromLink(msgData), nil
	case "location":
		return contentFromLocation(msgData), nil
	case "card":
		meta := map[string]any{}
		if sid := anyToString(msgData["shared_id"]); sid != "" {
			meta["shared_id"] = sid
		}
		return contentFromCard(msgData), meta
	case "red_packet":
		return contentFromRedPacket(msgData), nil
	case "miniapp":
		content, meta := contentFromMiniapp(msgData)
		return content, meta
	case "channel_msg":
		return contentFromChannelMsg(msgData), nil
	default:
		return "[收到消息]", nil
	}
}

func contentFromLink(msgData map[string]any) string {
	title := decodeMaybeBase64(anyToString(msgData["title"]))
	desc := decodeMaybeBase64(anyToString(msgData["desc"]))
	linkURL := anyToString(msgData["linkUrl"])
	if title == "" {
		title = linkURL
	}
	parts := []string{"[链接] 标题：" + title}
	if desc != "" {
		parts = append(parts, "描述："+desc)
	}
	if linkURL != "" {
		parts = append(parts, "地址："+linkURL)
	}
	return strings.Join(parts, "\n")
}

func contentFromLocation(msgData map[string]any) string {
	title := decodeMaybeBase64(anyToString(msgData["title"]))
	address := decodeMaybeBase64(anyToString(msgData["address"]))
	lat := anyToString(msgData["latitude"])
	lng := anyToString(msgData["longitude"])
	var parts []string
	if title != "" && address != "" {
		parts = append(parts, fmt.Sprintf("[位置] %s %s", title, address))
	} else if address != "" {
		parts = append(parts, "[位置] "+address)
	} else if title != "" {
		parts = append(parts, "[位置] "+title)
	} else {
		parts = append(parts, "[位置]")
	}
	if lat != "" && lng != "" {
		parts = append(parts, fmt.Sprintf("(纬度:%s, 经度:%s)", lat, lng))
	}
	return strings.Join(parts, " ")
}

func contentFromCard(msgData map[string]any) string {
	nickname := decodeMaybeBase64(anyToString(msgData["nickname"]))
	corpName := decodeMaybeBase64(anyToString(msgData["corpName"]))
	if nickname == "" {
		nickname = anyToString(msgData["realName"])
	}
	if nickname == "" {
		return "[名片]"
	}
	if corpName != "" {
		return fmt.Sprintf("[名片] %s 企业：%s", nickname, corpName)
	}
	return "[名片] " + nickname
}

func contentFromRedPacket(msgData map[string]any) string {
	wishing := decodeMaybeBase64(anyToString(msgData["wishingContent"]))
	if wishing != "" {
		return "[红包] " + wishing
	}
	return "[红包]"
}

func contentFromMiniapp(msgData map[string]any) (string, map[string]any) {
	title := decodeMaybeBase64(anyToString(msgData["title"]))
	desc := decodeMaybeBase64(anyToString(msgData["desc"]))
	parts := []string{"[小程序]"}
	if title != "" {
		parts[0] = "[小程序] " + title
	}
	if desc != "" {
		parts = append(parts, desc)
	}
	meta := map[string]any{"miniappData": msgData}
	return strings.Join(parts, "\n"), meta
}

func contentFromChannelMsg(msgData map[string]any) string {
	name := decodeMaybeBase64(anyToString(msgData["channelName"]))
	url := anyToString(msgData["channelUrl"])
	parts := []string{"[视频号]"}
	if name != "" {
		parts[0] = "[视频号] " + name
	}
	if url != "" {
		parts = append(parts, "链接："+url)
	}
	return strings.Join(parts, "\n")
}

// handleAppMessage routes msgType 49 (appmsg) by subType.
func (a *app) handleAppMessage(ctx context.Context, msg qiweiCallbackMessage) (string, map[string]any, string) {
	subType := int(anyToInt64(msg.MsgData["subType"]))
	if subType == 0 {
		subType = int(anyToInt64(msg.MsgData["type"]))
	}
	if subType == 0 {
		subType = int(anyToInt64(msg.MsgData["appmsgtype"]))
	}

	rawMsgData, _ := json.Marshal(msg.MsgData)
	logger.Detail(ctx, "appmsg(49) 路由",
		"tag", tagCallback, "subType", subType,
		"msg", msg.MsgSvrID, "msgData", string(rawMsgData))

	switch subType {
	case 57:
		content, meta := contentFromQuote(msg.MsgData)
		return content, meta, "quote"
	case 5:
		return contentFromLink(msg.MsgData), nil, "link"
	case 33, 36:
		c, m := contentFromMiniapp(msg.MsgData)
		return c, m, "miniapp"
	default:
		return "", nil, "file"
	}
}

// contentFromQuote extracts the quoted message context and the user's reply text.
// Returns the reply content and channelMeta containing the quotedMessage structure.
func contentFromQuote(msgData map[string]any) (string, map[string]any) {
	replyText := strings.TrimSpace(decodeMaybeBase64(anyToString(msgData["content"])))
	if replyText == "" {
		replyText = strings.TrimSpace(decodeMaybeBase64(anyToString(msgData["title"])))
	}

	referMsg := firstNonNilMap(
		msgData["referMsg"],
		msgData["refermsg"],
		msgData["refer_msg"],
		msgData["referMessage"],
	)

	if len(referMsg) == 0 {
		if replyText != "" {
			return replyText, nil
		}
		return "[引用消息]", nil
	}

	quotedContent := strings.TrimSpace(decodeMaybeBase64(anyToString(referMsg["content"])))
	quotedSender := strings.TrimSpace(decodeMaybeBase64(firstNonEmpty(
		anyToString(referMsg["displayName"]),
		anyToString(referMsg["nickname"]),
		anyToString(referMsg["chatnickname"]),
	)))
	quotedMsgID := firstNonEmpty(
		anyToString(referMsg["svrid"]),
		anyToString(referMsg["msgSvrId"]),
		anyToString(referMsg["msgServerId"]),
	)

	meta := map[string]any{
		"quotedMessage": map[string]any{
			"msgSvrId":   quotedMsgID,
			"content":    quotedContent,
			"senderName": quotedSender,
		},
	}

	return replyText, meta
}

func firstNonNilMap(values ...any) map[string]any {
	for _, v := range values {
		if m, ok := v.(map[string]any); ok && len(m) > 0 {
			return m
		}
	}
	return nil
}

func (a *app) handleMixedMessage(ctx context.Context, msg qiweiCallbackMessage) (string, []incomingAttachment) {
	rawData, ok := msg.MsgData["content"]
	if !ok {
		rawData = msg.MsgData["msgData"]
	}
	var subMessages []any
	switch v := rawData.(type) {
	case []any:
		subMessages = v
	default:
		raw, _ := json.Marshal(msg.MsgData)
		var arr []any
		if err := json.Unmarshal(raw, &arr); err == nil {
			subMessages = arr
		}
	}
	if len(subMessages) == 0 {
		return "[图文混合消息]", nil
	}

	var textParts []string
	var attachments []incomingAttachment
	for i, sub := range subMessages {
		subMap, ok := sub.(map[string]any)
		if !ok {
			continue
		}
		subType := int(anyToInt64(subMap["subMsgType"]))
		subData := mapValue(subMap["subMsgData"])

		switch subType {
		case 0, 2:
			text := decodeMaybeBase64(anyToString(subData["content"]))
			if text != "" {
				textParts = append(textParts, text)
			}
		case 7, 14, 101:
			prepared := a.prepareMediaForAgent(ctx, subType, "image", subData)
			if prepared.ResourceURI != "" {
				attachments = append(attachments, incomingAttachment{
					ID:                fmt.Sprintf("%s:%d", msg.MsgSvrID, i),
					Kind:              "image",
					ResourceURI:       prepared.ResourceURI,
					DisplayName:       prepared.Name,
					MIMEType:          prepared.MIMEType,
					SourceMessageType: "image",
				})
			} else {
				textParts = append(textParts, "[图片]")
			}
		}
	}
	content := strings.Join(textParts, " ")
	if content == "" {
		content = "[图文混合消息]"
	}
	return content, attachments
}

func (a *app) handleSystemEvent(ctx context.Context, msg qiweiCallbackMessage) error {
	switch msg.MsgType {
	case 1002:
		return a.handleGroupMemberJoined(ctx, msg)
	case 2357:
		logger.Business(ctx, "好友申请",
			"tag", tagCallback,
			"msgType", msg.MsgType,
			"contactNickname", anyToString(msg.MsgData["contactNickname"]),
			"contactId", anyToString(msg.MsgData["contactId"]),
		)
		return nil
	case 2132:
		logger.Business(ctx, "好友申请(简)", "tag", tagCallback, "msgType", msg.MsgType, "msg", msg.MsgSvrID)
		return nil
	default:
		logger.Detail(ctx, "系统事件(忽略)", "tag", tagCallback, "msgType", msg.MsgType, "msg", msg.MsgSvrID)
		return nil
	}
}

func (a *app) handleGroupMemberJoined(ctx context.Context, msg qiweiCallbackMessage) error {
	roomID := msg.FromRoomID
	if roomID == "" || roomID == "0" {
		logger.Warn(ctx, "入群通知缺少 roomId", "tag", tagCallback, "msgType", msg.MsgType)
		return nil
	}

	if !a.cfg.AgentEnabled {
		logger.Detail(ctx, "入群通知(agent 未启用)", "tag", tagCallback, "roomId", roomID)
		return nil
	}

	ts := msg.CreateTime * 1000
	if ts == 0 {
		ts = time.Now().UnixMilli()
	}
	in := incomingMessage{
		Channel:               "qiwei",
		ChannelUserID:         msg.SenderID,
		ChannelMessageID:      msg.MsgSvrID,
		ChannelConversationID: roomID,
		ConversationType:      "group",
		MessageType:           "system",
		Content:               "[群事件] 新成员加入了群聊",
		Timestamp:             ts,
		AgentID:               a.cfg.AgentID,
	}
	return a.forwardToAgent(ctx, in)
}

func attachmentsFromPreparedMedia(messageID, messageType string, prepared preparedMedia) []incomingAttachment {
	resourceURI := strings.TrimSpace(prepared.ResourceURI)
	if resourceURI == "" {
		return nil
	}
	switch messageType {
	case "image", "file", "video", "sticker":
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

var cst = time.FixedZone("CST", 8*3600)

func formatSenderPrefix(name, id string, t time.Time) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	if name == "" {
		name = firstNonEmpty(id, "未知用户")
	}
	ts := t.In(cst).Format("2006-01-02 15:04:05")
	if id == "" {
		return name + " " + ts + ":"
	}
	return name + "[" + id + "] " + ts + ":"
}

func formatVoiceContent(prefix, transcript string) string {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" || transcript == "[收到语音]" {
		return prefix + "[语音转写失败]"
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
	cmd := int(anyToInt64(in["cmd"]))
	if cmd == 0 {
		cmd = 15000
	}
	msg := qiweiCallbackMessage{
		Cmd:            cmd,
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
