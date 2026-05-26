package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

const tagCallback = "callback"

var userMessageTypeMap = map[int]string{
	0:  "text",
	1:  "text",
	2:  "text",
	3:  "image",
	6:  "location",
	7:  "image",
	13: "link",
	14: "image",
	15: "file",
	16: "voice",
	20: "file",
	22: "video",
	23: "video",
	26: "red_packet",
	29: "sticker",
	34: "voice",
	41: "card",
	43: "video",
	// msgType 49 (appmsg) is handled in handleNormalMessage switch by subType.
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
	logRawCallbackBody(ctx, rawBody)
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
	// Resolve the account runtime up front: every downstream handler needs
	// it for API calls, dedupe and session routing. Unknown/disabled guids
	// are buffered for operator visibility (admin API exposes the buffer)
	// and silently dropped here so we don't starve the callback ack.
	rt, ok := a.currentRegistry().GetByGUID(msg.GUID)
	if !ok {
		a.unknownGuids.Record(unknownGuidEvent{
			GUID:     msg.GUID,
			Cmd:      msg.Cmd,
			MsgType:  msg.MsgType,
			MsgSvrID: msg.MsgSvrID,
			At:       time.Now().Unix(),
		})
		logger.Warn(ctx, "未注册的企微账号，丢弃回调",
			"tag", tagCallback, "guid", msg.GUID,
			"cmd", msg.Cmd, "msgType", msg.MsgType, "msg", msg.MsgSvrID)
		return nil
	}

	// 历史上 decodeOneMessage 会把 cmd=0 归一为 15000（正常消息）。这里
	// 把同样的归一化前移到公共入口，便于测试或其它调用方直接构造
	// qiweiCallbackMessage 不设置 Cmd 时也能走正常消息路径。
	cmd := msg.Cmd
	if cmd == 0 {
		cmd = 15000
	}
	switch cmd {
	case 15000:
		return a.handleNormalMessage(ctx, rt, msg)
	case 15500:
		return a.handleSystemEvent(ctx, rt, msg)
	case 11016:
		logger.Detail(ctx, "账号状态变化", "tag", tagCallback, "msgType", msg.MsgType, "guid", msg.GUID, "accountId", rt.AccountID())
		return nil
	case 20000:
		logger.Detail(ctx, "API 异步消息", "tag", tagCallback, "msgType", msg.MsgType, "guid", msg.GUID, "accountId", rt.AccountID())
		return nil
	default:
		logger.Detail(ctx, "未处理的 cmd 类型", "tag", tagCallback, "cmd", msg.Cmd, "msgType", msg.MsgType)
		return nil
	}
}

// groupEventTypes maps msgTypes that represent group lifecycle events.
var groupEventTypes = map[int]string{
	1001: "group_name_changed",
	1002: "member_joined",
	1003: "member_removed",
	1005: "member_quit",
	1023: "group_dissolved",
}

// knownIgnoredMsgTypes are documented msgTypes that agents don't act on.
var knownIgnoredMsgTypes = map[int]bool{
	146:  true, // 直播
	2001: true, // 已读通知
	2005: true, // 未读通知
}

// textMessageTypes is the set of msgTypes that carry plain text content.
var textMessageTypes = map[int]bool{0: true, 1: true, 2: true}

// richContentTypes carry structured data that should be extracted into readable text,
// not routed through the media download pipeline.
var richContentTypes = map[string]bool{
	"link": true, "location": true, "card": true,
	"red_packet": true, "miniapp": true, "channel_msg": true,
}

func (a *app) handleNormalMessage(ctx context.Context, rt *accountRuntime, msg qiweiCallbackMessage) error {
	// Group lifecycle events — report to agent-server data store, don't forward as messages.
	if eventType, ok := groupEventTypes[msg.MsgType]; ok {
		return a.handleGroupEvent(ctx, rt, eventType, msg)
	}

	// Known non-actionable msgTypes — log and skip.
	if knownIgnoredMsgTypes[msg.MsgType] {
		logger.Detail(ctx, "忽略非 agent 消息类型", "tag", tagCallback, "msgType", msg.MsgType, "msg", msg.MsgSvrID)
		return nil
	}

	if msg.MsgSvrID != "" && rt.dedupe.Seen(msg.MsgSvrID) {
		logger.Detail(ctx, "跳过重复消息", "tag", tagCallback, "msg", msg.MsgSvrID, "accountId", rt.AccountID())
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
		if replyMap := mapValue(msg.MsgData["reply"]); len(replyMap) > 0 {
			replyContent := strings.TrimSpace(anyToString(replyMap["content"]))
			replyMsgID := anyToString(replyMap["msgId"])
			if replyContent != "" || replyMsgID != "" {
				messageType = "quote"
				channelMeta = map[string]any{
					"quotedMessage": map[string]any{
						"msgSvrId":   replyMsgID,
						"content":    replyContent,
						"senderName": "",
					},
				}
			}
		}
		atList := extractAtList(msg.MsgData["atList"])
		if len(atList) > 0 {
			if channelMeta == nil {
				channelMeta = map[string]any{}
			}
			channelMeta["atList"] = atList
			selfID := rt.SelfUserID()
			if selfID == "" {
				selfID = anyToString(msg.MsgData["userId"])
			}
			if selfID != "" {
				for _, uid := range atList {
					if uid == selfID {
						channelMeta["mentionedSelf"] = true
						break
					}
				}
			}
		}

	case msg.MsgType == 49:
		content, channelMeta, messageType = a.handleAppMessage(ctx, rt, msg)
		if content == "" && messageType == "file" {
			prepared := a.prepareMediaForAgent(ctx, rt, msg.MsgType, "file", msg.MsgData)
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
		content, attachments = a.handleMixedMessage(ctx, rt, msg)

	case messageType == "sticker":
		prepared := a.prepareMediaForAgent(ctx, rt, msg.MsgType, "image", msg.MsgData)
		if prepared.ResourceURI != "" {
			content = prepared.Content
			attachments = attachmentsFromPreparedMedia(msg.MsgSvrID, "image", prepared)
			messageType = "sticker"
		} else {
			content = "[表情]"
		}

	default:
		prepared := a.prepareMediaForAgent(ctx, rt, msg.MsgType, messageType, msg.MsgData)
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

	if a.contactSync != nil && rt.gateway != nil && msg.SenderID != "" && !rt.gateway.HasContact(ctx, msg.SenderID) {
		a.contactSync.EnqueueContact(rt, msg.SenderID, "message-miss")
	}

	senderName := msg.SenderNickname
	if senderName == "" && msg.SenderID != "" {
		senderName = a.resolveUserNameInRoom(ctx, rt, msg.SenderID, msg.FromRoomID)
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
	conversationName := senderName
	if isGroup {
		replyToID = msg.FromRoomID
		gn := a.resolveGroupName(ctx, rt, msg.FromRoomID)
		if gn != "" {
			conversationName = gn
		} else {
			conversationName = msg.FromRoomID
		}
	}

	if !a.cfg.AgentEnabled {
		logger.Business(ctx, "echo 模式", "tag", tagCallback, "msg", msg.MsgSvrID, "to", replyToID, "type", messageType)
		if textMessageTypes[msg.MsgType] {
			_, err := rt.client.doAPIRaw(ctx, "/msg/sendText", map[string]any{
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
		ChannelAccountID:        rt.AccountID(),
		ChannelAccountShortHash: rt.ShortHash(),
		ChannelUserID:           msg.SenderID,
		ChannelMessageID:        msg.MsgSvrID,
		ChannelConversationID:   encodeConversationID(replyToID, rt.ShortHash()),
		ChannelConversationName: conversationName,
		ConversationType:        conversationType,
		MessageType:             messageType,
		Content:                 content,
		SenderName:              senderName,
		Timestamp:               ts,
		ChannelMeta:             channelMeta,
		Attachments:             attachments,
		AgentID:                 firstNonEmpty(rt.AgentID(), a.cfg.AgentID),
		ChannelIdentity:         rt.channelIdentity(),
	}
	logger.Business(ctx, "转发消息到 agent",
		"tag", tagCallback,
		"msg", msg.MsgSvrID,
		"accountId", rt.AccountID(),
		"conversation", in.ChannelConversationID,
		"type", in.MessageType,
		"sender", senderName,
	)
	err := a.forwardToAgent(ctx, rt, in)
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
func (a *app) handleAppMessage(ctx context.Context, rt *accountRuntime, msg qiweiCallbackMessage) (string, map[string]any, string) {
	_ = rt // appmsg parsing today is pure; keep the signature to match the
	//       routing convention and make it easy to pull from rt later.
	subType := int(anyToInt64(msg.MsgData["subType"]))
	if subType == 0 {
		subType = int(anyToInt64(msg.MsgData["type"]))
	}
	if subType == 0 {
		subType = int(anyToInt64(msg.MsgData["appmsgtype"]))
	}

	// Fallback: extract subType from XML in content field (common for personal WeChat bridges).
	if subType == 0 {
		if parsed, ok := tryParseAppMsgXML(msg.MsgData); ok {
			subType = parsed.Type
		}
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
// It first tries flat fields in msgData (pre-parsed by bridge), then falls back
// to parsing XML from the content field (common for personal WeChat bridges).
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

	// Fallback: parse XML when flat fields are missing or content looks like XML.
	if len(referMsg) == 0 || looksLikeXML(replyText) {
		if parsed, ok := tryParseAppMsgXML(msgData); ok {
			// Only fill referMsg from XML when bridge didn't provide one.
			if len(referMsg) == 0 && (parsed.ReferMsg.SvrID != "" || parsed.ReferMsg.Content != "") {
				referMsg = map[string]any{
					"svrid":        parsed.ReferMsg.SvrID,
					"content":      parsed.ReferMsg.Content,
					"chatnickname": parsed.ReferMsg.ChatNickname,
					"displayname":  parsed.ReferMsg.DisplayName,
					"fromusr":      parsed.ReferMsg.FromUsr,
				}
			}
			// Always prefer the parsed title over raw XML content.
			replyText = parsed.Title
		}
	}

	if len(referMsg) == 0 {
		if replyText != "" {
			return replyText, nil
		}
		return "[引用消息]", nil
	}

	quotedContent := strings.TrimSpace(decodeMaybeBase64(anyToString(referMsg["content"])))
	quotedSender := strings.TrimSpace(decodeMaybeBase64(firstNonEmpty(
		anyToString(referMsg["displayName"]),
		anyToString(referMsg["displayname"]),
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

	if replyText == "" {
		replyText = "[引用消息]"
	}

	return replyText, meta
}

// appMsgXML maps the XML structure used by personal WeChat for appmsg (type 49).
type appMsgXML struct {
	XMLName xml.Name       `xml:"msg"`
	AppMsg  appMsgInnerXML `xml:"appmsg"`
}

type appMsgInnerXML struct {
	Title    string      `xml:"title"`
	Type     int         `xml:"type"`
	ReferMsg referMsgXML `xml:"refermsg"`
}

type referMsgXML struct {
	Type         int    `xml:"type"`
	SvrID        string `xml:"svrid"`
	FromUsr      string `xml:"fromusr"`
	ChatNickname string `xml:"chatnickname"`
	DisplayName  string `xml:"displayname"`
	Content      string `xml:"content"`
}

// tryParseAppMsgXML attempts to parse XML from msgData["content"] (possibly base64-encoded).
func tryParseAppMsgXML(msgData map[string]any) (appMsgInnerXML, bool) {
	raw := strings.TrimSpace(decodeMaybeBase64(anyToString(msgData["content"])))
	if raw == "" {
		return appMsgInnerXML{}, false
	}
	if !looksLikeXML(raw) {
		return appMsgInnerXML{}, false
	}
	var msg appMsgXML
	if err := xml.Unmarshal([]byte(raw), &msg); err != nil {
		return appMsgInnerXML{}, false
	}
	if msg.AppMsg.Type == 0 && msg.AppMsg.Title == "" {
		return appMsgInnerXML{}, false
	}
	return msg.AppMsg, true
}

func looksLikeXML(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "<")
}

func firstNonNilMap(values ...any) map[string]any {
	for _, v := range values {
		if m, ok := v.(map[string]any); ok && len(m) > 0 {
			return m
		}
	}
	return nil
}

func (a *app) handleMixedMessage(ctx context.Context, rt *accountRuntime, msg qiweiCallbackMessage) (string, []incomingAttachment) {
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
			prepared := a.prepareMediaForAgent(ctx, rt, subType, "image", subData)
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

func (a *app) handleSystemEvent(ctx context.Context, rt *accountRuntime, msg qiweiCallbackMessage) error {
	// Group events may also arrive via cmd=15500; handle them the same way.
	if eventType, ok := groupEventTypes[msg.MsgType]; ok {
		return a.handleGroupEvent(ctx, rt, eventType, msg)
	}

	switch msg.MsgType {
	case 2357:
		contactID := anyToString(msg.MsgData["contactId"])
		contactNickname := decodeMaybeBase64(anyToString(msg.MsgData["contactNickname"]))
		contactType := anyToString(msg.MsgData["contactType"])
		logger.Business(ctx, "好友申请 - 自动通过",
			"tag", tagCallback,
			"accountId", rt.AccountID(),
			"msgType", msg.MsgType,
			"contactNickname", contactNickname,
			"contactId", contactID,
			"contactType", contactType,
		)
		if contactID != "" {
			go a.autoAcceptFriendRequest(context.Background(), rt, contactID, contactNickname, contactType)
		}
		return nil
	case 2132:
		logger.Business(ctx, "好友申请(简)", "tag", tagCallback, "msgType", msg.MsgType, "msg", msg.MsgSvrID)
		return nil
	default:
		logger.Detail(ctx, "系统事件(忽略)", "tag", tagCallback, "msgType", msg.MsgType, "msg", msg.MsgSvrID)
		return nil
	}
}

func (a *app) handleGroupEvent(ctx context.Context, rt *accountRuntime, eventType string, msg qiweiCallbackMessage) error {
	roomID := msg.FromRoomID
	if roomID == "" || roomID == "0" {
		logger.Warn(ctx, "群事件缺少 roomId",
			"tag", tagCallback,
			"accountId", rt.AccountID(),
			"msgType", msg.MsgType,
			"eventType", eventType,
		)
		return nil
	}

	// Detect bot entering a new group: first time seeing this room (per account).
	isNewRoom := rt.roomStore.Add(roomID)
	reportType := eventType
	if isNewRoom && eventType == "member_joined" {
		reportType = "group_joined"
	}

	groupName := ""
	if eventType == "group_name_changed" {
		rt.nameCache.Delete("room:" + roomID)
		groupName = a.resolveGroupName(ctx, rt, roomID)
	}
	if isNewRoom && groupName == "" {
		groupName = a.resolveGroupName(ctx, rt, roomID)
	}

	memberIDs := parseChangedMemberList(msg.MsgData["changedMemberList"])
	eventPayload := map[string]any{
		"memberIds": memberIDs,
	}
	if msg.SenderID != "" {
		eventPayload["operatorId"] = msg.SenderID
	}

	logger.Business(ctx, "群事件上报",
		"tag", tagCallback,
		"accountId", rt.AccountID(),
		"eventType", reportType,
		"originalEvent", eventType,
		"msgType", msg.MsgType,
		"roomId", roomID,
		"groupName", groupName,
		"isNewRoom", isNewRoom,
		"memberIds", memberIDs,
	)
	if a.contactSync != nil {
		a.contactSync.EnqueueRoom(rt, roomID, reportType)
	}

	if !a.cfg.AgentEnabled || a.cfg.AgentServer == "" {
		logger.Detail(ctx, "群事件跳过上报(agent 未启用)",
			"tag", tagCallback,
			"accountId", rt.AccountID(),
			"roomId", roomID,
		)
		return nil
	}

	return a.reportGroupEvent(ctx, rt, reportType, roomID, groupName, eventPayload)
}

// parseChangedMemberList decodes the changedMemberList field (may be base64, semicolon-separated).
func parseChangedMemberList(raw any) []string {
	if raw == nil {
		return []string{}
	}
	s := strings.TrimSpace(anyToString(raw))
	if s == "" {
		return []string{}
	}
	decoded := decodeMaybeBase64(s)
	if decoded == "" {
		return []string{}
	}
	var out []string
	for _, id := range strings.Split(decoded, ";") {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func (a *app) reportGroupEvent(ctx context.Context, rt *accountRuntime, eventType, channelGroupID, groupName string, eventPayload map[string]any) error {
	agentID := a.cfg.AgentID
	reportedGroupID := channelGroupID
	if rt != nil {
		if id := rt.AgentID(); id != "" {
			agentID = id
		}
		reportedGroupID = encodeConversationID(channelGroupID, rt.ShortHash())
	}
	payload := map[string]any{
		"channel":        "qiwei",
		"agentId":        agentID,
		"channelGroupId": reportedGroupID,
		"eventType":      eventType,
		"groupName":      groupName,
	}
	if rt != nil {
		payload["channelIdentity"] = rt.channelIdentity()
		payload["channelAccountId"] = rt.AccountID()
		payload["channelAccountShortHash"] = rt.ShortHash()
	}
	if len(eventPayload) > 0 {
		payload["payload"] = eventPayload
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.AgentServer+"/api/channels/group-event", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		logger.Warn(ctx, "群事件上报失败", "tag", tagCallback, "eventType", eventType, "error", err.Error())
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		logger.Warn(ctx, "群事件上报响应异常", "tag", tagCallback, "eventType", eventType, "status", resp.StatusCode)
	}
	return nil
}

func (a *app) autoAcceptFriendRequest(ctx context.Context, rt *accountRuntime, contactID, contactNickname, contactType string) {
	_, err := rt.client.doAPIRaw(ctx, "/contact/agreeContact", map[string]any{
		"contactId": contactID,
	})
	if err != nil {
		logger.Warn(ctx, "自动通过好友申请失败",
			"tag", tagCallback,
			"accountId", rt.AccountID(),
			"contactId", contactID,
			"error", err.Error(),
		)
		return
	}
	logger.Business(ctx, "自动通过好友申请成功",
		"tag", tagCallback,
		"accountId", rt.AccountID(),
		"contactId", contactID,
		"contactNickname", contactNickname,
	)
	if a.contactSync != nil {
		a.contactSync.EnqueueContact(rt, contactID, "friend-accepted")
	}

	if !a.cfg.AgentEnabled || a.cfg.AgentServer == "" {
		return
	}
	_ = a.reportGroupEvent(ctx, rt, "new_contact", contactID, "", map[string]any{
		"contactId":       contactID,
		"contactNickname": contactNickname,
		"contactType":     contactType,
	})
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

func (a *app) resolveUserName(ctx context.Context, rt *accountRuntime, userID string) string {
	return a.resolveUserNameInRoom(ctx, rt, userID, "")
}

func (a *app) resolveUserNameInRoom(ctx context.Context, rt *accountRuntime, userID, roomID string) string {
	if rt == nil || rt.gateway == nil {
		return ""
	}
	name, err := rt.gateway.ResolveName(ctx, userID, roomID)
	if err != nil {
		logger.Warn(ctx, "联系人姓名解析失败",
			"accountId", rt.AccountID(),
			"userId", userID,
			"roomId", roomID,
			"error", err.Error(),
		)
		return ""
	}
	return name
}

// fetchMemberNameFromRoom queries /room/batchGetRoomDetail and caches all member names.
func (a *app) fetchMemberNameFromRoom(ctx context.Context, rt *accountRuntime, userID, roomID string) string {
	res, err := rt.client.doAPIRaw(ctx, "/room/batchGetRoomDetail", map[string]any{
		"roomIdList": []string{roomID},
	})
	if err != nil {
		logger.Warn(ctx, "查询群成员名称失败",
			"accountId", rt.AccountID(),
			"userId", userID,
			"roomId", roomID,
			"error", err.Error(),
		)
		return ""
	}
	var wrapper struct {
		RoomList []struct {
			RoomID     string           `json:"roomId"`
			MemberList []map[string]any `json:"memberList"`
		} `json:"roomList"`
	}
	if err := unmarshalSafe(res.Data, &wrapper); err != nil {
		return ""
	}
	for _, room := range wrapper.RoomList {
		for _, m := range room.MemberList {
			uid := anyToString(m["userId"])
			name := decodeMaybeBase64(anyToString(m["name"]))
			if uid != "" && name != "" {
				rt.nameCache.Set(uid, name)
			}
		}
	}
	if v, ok := rt.nameCache.Get(userID); ok {
		return v
	}
	return ""
}

func (a *app) loadSelfUserID(ctx context.Context, rt *accountRuntime) {
	res, err := rt.client.doAPIRaw(ctx, "/user/getProfile", nil)
	if err != nil {
		logger.Warn(ctx, "获取自身用户信息失败", "accountId", rt.AccountID(), "error", err.Error())
		return
	}
	var profile struct {
		UserID string `json:"userId"`
	}
	if err := unmarshalSafe(res.Data, &profile); err != nil {
		logger.Warn(ctx, "解析自身用户信息失败", "accountId", rt.AccountID(), "error", err.Error())
		return
	}
	if profile.UserID != "" {
		rt.setSelfUserID(profile.UserID)
		logger.Business(ctx, "缓存自身 userId",
			"accountId", rt.AccountID(),
			"selfUserID", profile.UserID,
		)
	}
}

func (a *app) loadContactsOnce(ctx context.Context, rt *accountRuntime) {
	if rt == nil || rt.gateway == nil {
		return
	}
	rt.contactsMu.Lock()
	if time.Since(rt.contactsLoadedAt) < 5*time.Minute {
		rt.contactsMu.Unlock()
		return
	}
	rt.contactsMu.Unlock()

	if err := rt.gateway.SyncContacts(ctx); err != nil {
		logger.Warn(ctx, "联系人同步失败",
			"accountId", rt.AccountID(),
			"error", err.Error(),
		)
		return
	}

	rt.contactsMu.Lock()
	rt.contactsLoadedAt = time.Now()
	rt.contactsMu.Unlock()
}

func (a *app) loadExternalContacts(ctx context.Context, rt *accountRuntime) {
	res, err := rt.client.doAPIRaw(ctx, "/contact/getWxContactList", nil)
	if err != nil {
		logger.Warn(ctx, "加载外部联系人失败", "accountId", rt.AccountID(), "error", err.Error())
		return
	}
	var wrapper struct {
		ContactList []map[string]any `json:"contactList"`
	}
	if err := unmarshalSafe(res.Data, &wrapper); err != nil {
		return
	}
	for _, c := range wrapper.ContactList {
		uid := anyToString(c["userId"])
		name := firstNonEmpty(
			decodeMaybeBase64(anyToString(c["nickname"])),
			decodeMaybeBase64(anyToString(c["realName"])),
			decodeMaybeBase64(anyToString(c["remark"])),
			decodeMaybeBase64(anyToString(c["alias"])),
		)
		if uid != "" && name != "" {
			rt.nameCache.Set(uid, name)
		}
	}
	logger.Business(ctx, "加载外部联系人",
		"accountId", rt.AccountID(),
		"cached", len(wrapper.ContactList),
	)
}

func (a *app) loadInternalContacts(ctx context.Context, rt *accountRuntime) {
	res, err := rt.client.doAPIRaw(ctx, "/contact/getWxWorkContactList", nil)
	if err != nil {
		logger.Warn(ctx, "加载内部联系人失败", "accountId", rt.AccountID(), "error", err.Error())
		return
	}
	var wrapper struct {
		ContactList []map[string]any `json:"contactList"`
	}
	if err := unmarshalSafe(res.Data, &wrapper); err != nil {
		return
	}
	cached := 0
	for _, c := range wrapper.ContactList {
		uid := anyToString(c["userId"])
		name := firstNonEmpty(
			decodeMaybeBase64(anyToString(c["nickname"])),
			decodeMaybeBase64(anyToString(c["realName"])),
			decodeMaybeBase64(anyToString(c["remark"])),
			decodeMaybeBase64(anyToString(c["name"])),
		)
		if uid != "" && uid != "0" && name != "" {
			rt.nameCache.Set(uid, name)
			cached++
		}
	}
	logger.Business(ctx, "加载内部联系人", "accountId", rt.AccountID(), "cached", cached)
}

func (a *app) resolveGroupName(ctx context.Context, rt *accountRuntime, roomID string) string {
	if rt == nil || rt.gateway == nil {
		return ""
	}
	room, err := rt.gateway.ResolveRoom(ctx, roomID)
	if err != nil {
		logger.Warn(ctx, "获取群详情失败",
			"accountId", rt.AccountID(),
			"roomId", roomID,
			"error", err.Error(),
		)
		return ""
	}
	return room.Name
}

func (a *app) fetchUserName(ctx context.Context, rt *accountRuntime, userID string) string {
	return a.resolveUserName(ctx, rt, userID)
}

func (a *app) forwardToAgent(ctx context.Context, rt *accountRuntime, in incomingMessage) error {
	_ = rt // rt currently only influences fields baked into `in` by the caller;
	//       kept for symmetry and future per-account transport customization.
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
	if err := unmarshalSafe(trimmed, &payload); err != nil {
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
		SenderNickname: decodeMaybeBase64(firstNonEmpty(anyToString(in["senderNickname"]), anyToString(in["senderName"]))),
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

func logRawCallbackBody(ctx context.Context, rawBody []byte) {
	if len(rawBody) == 0 {
		logger.Detail(ctx, "callback 原始负载", "tag", tagCallback, "bytes", 0)
		return
	}
	logger.Detail(ctx, "callback 原始负载 "+string(rawBody),
		"tag", tagCallback,
		"bytes", len(rawBody),
	)
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

// extractAtList parses the atList field from callback msgData.
// The field may be a base64-encoded semicolon-separated string, a plain string,
// or a JSON array.
func extractAtList(raw any) []string {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		var out []string
		for _, item := range v {
			if s := strings.TrimSpace(anyToString(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		decoded := decodeMaybeBase64(v)
		if decoded == "" {
			return nil
		}
		var out []string
		for _, s := range strings.Split(decoded, ";") {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
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
