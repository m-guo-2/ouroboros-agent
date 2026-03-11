package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

type searchTargetsRequest struct {
	Query           string `json:"query"`
	Limit           int    `json:"limit"`
	IncludeContacts *bool  `json:"includeContacts,omitempty"`
	IncludeGroups   *bool  `json:"includeGroups,omitempty"`
}

type listOrGetConversationsRequest struct {
	ConversationID string `json:"conversationId,omitempty"`
	MsgSvrID       string `json:"msgSvrId,omitempty"`
	CurrentSeq     int64  `json:"currentSeq,omitempty"`
	PageSize       int    `json:"pageSize,omitempty"`
}

type parseMessageRequest struct {
	Message     map[string]any `json:"message,omitempty"`
	MessageType string         `json:"messageType,omitempty"`
	MsgData     map[string]any `json:"msgData,omitempty"`
	ResourceURI string         `json:"resourceUri,omitempty"`
	LocalPath   string         `json:"localPath,omitempty"`
}

type facadeSendMessageRequest struct {
	ChannelConversationID string         `json:"channelConversationId,omitempty"`
	ChannelUserID         string         `json:"channelUserId,omitempty"`
	MessageType           string         `json:"messageType,omitempty"`
	Content               string         `json:"content"`
	ChannelMeta           map[string]any `json:"channelMeta,omitempty"`
}

type parsedAttachment struct {
	Kind          string         `json:"kind"`
	Name          string         `json:"name,omitempty"`
	MIMEType      string         `json:"mimeType,omitempty"`
	FileID        string         `json:"fileId,omitempty"`
	FileAESKey    string         `json:"fileAesKey,omitempty"`
	FileAuthKey   string         `json:"fileAuthKey,omitempty"`
	FileMD5       string         `json:"fileMd5,omitempty"`
	FileSize      int64          `json:"fileSize,omitempty"`
	FileType      int            `json:"fileType,omitempty"`
	CDNKey        string         `json:"cdnKey,omitempty"`
	SourceURL     string         `json:"sourceUrl,omitempty"`
	ResourceURI   string         `json:"resourceUri,omitempty"`
	LocalPath     string         `json:"localPath,omitempty"`
	ParseProvider string         `json:"parseProvider,omitempty"`
	ParseStatus   string         `json:"parseStatus,omitempty"`
	ParsedText    string         `json:"parsedText,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	DataURL       string         `json:"-"`
	Raw           map[string]any `json:"raw,omitempty"`
}

type parsedMessage struct {
	MessageType string             `json:"messageType"`
	Text        string             `json:"text,omitempty"`
	Attachments []parsedAttachment `json:"attachments,omitempty"`
	Raw         map[string]any     `json:"raw,omitempty"`
}

type recognizer interface {
	ParseImage(ctx context.Context, attachment parsedAttachment) (parsedAttachment, error)
	ParseDocument(ctx context.Context, attachment parsedAttachment) (parsedAttachment, error)
	SubmitAudioTranscription(ctx context.Context, attachment parsedAttachment) (string, error)
	QueryAudioTranscription(ctx context.Context, taskID string) (parsedAttachment, bool, error)
}

func (a *app) handleSearchTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	var req searchTargetsRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	includeContacts := req.IncludeContacts == nil || *req.IncludeContacts
	includeGroups := req.IncludeGroups == nil || *req.IncludeGroups
	if !includeContacts && !includeGroups {
		includeContacts = true
		includeGroups = true
	}

	type target struct {
		ID   string         `json:"id"`
		Name string         `json:"name"`
		Type string         `json:"type"`
		Raw  map[string]any `json:"raw"`
	}

	resp := struct {
		Query   string   `json:"query"`
		Targets []target `json:"targets"`
	}{
		Query: req.Query,
	}

	if includeContacts {
		contacts, err := a.searchContacts(r.Context(), req.Query)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
			return
		}
		for _, item := range contacts {
			if len(resp.Targets) >= limit {
				break
			}
			id := firstNonEmpty(anyToString(item["userId"]), anyToString(item["id"]))
			if id == "" {
				continue
			}
			name := firstNonEmpty(
				anyToString(item["nickname"]),
				anyToString(item["realName"]),
				anyToString(item["remark"]),
				anyToString(item["name"]),
				id,
			)
			resp.Targets = append(resp.Targets, target{
				ID:   id,
				Name: name,
				Type: "contact",
				Raw:  item,
			})
		}
	}

	if includeGroups && len(resp.Targets) < limit {
		groups, err := a.listGroups(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
			return
		}
		query := strings.ToLower(strings.TrimSpace(req.Query))
		for _, item := range groups {
			if len(resp.Targets) >= limit {
				break
			}
			name := firstNonEmpty(anyToString(item["roomName"]), anyToString(item["name"]))
			if query != "" && !strings.Contains(strings.ToLower(name), query) {
				continue
			}
			id := firstNonEmpty(anyToString(item["roomId"]), anyToString(item["id"]))
			if id == "" {
				continue
			}
			resp.Targets = append(resp.Targets, target{
				ID:   id,
				Name: firstNonEmpty(name, id),
				Type: "group",
				Raw:  item,
			})
		}
	}

	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: resp})
}

func (a *app) handleListOrGetConversations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	var req listOrGetConversationsRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}

	if strings.TrimSpace(req.ConversationID) == "" {
		params := map[string]any{}
		if req.CurrentSeq != 0 {
			params["currentSeq"] = req.CurrentSeq
		}
		if req.PageSize > 0 {
			params["pageSize"] = req.PageSize
		}
		res, err := a.client.doAPIRaw(r.Context(), "/session/getSessionPage", params)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
			return
		}
		data, err := decodeAPIData(res.Data)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]any{
			"mode":          "list",
			"conversations": extractItems(data, "sessionList", "sessions", "list", "rows"),
			"raw":           data,
		}})
		return
	}

	params := map[string]any{"toId": req.ConversationID}
	if strings.TrimSpace(req.MsgSvrID) != "" {
		params["msgSvrId"] = req.MsgSvrID
	}
	res, err := a.client.doAPIRaw(r.Context(), "/msg/syncMsg", params)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}
	data, err := decodeAPIData(res.Data)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}
	items := extractItems(data, "syncMsgList", "messageList", "msgList", "list", "rows")
	normalized := make([]map[string]any, 0, len(items))
	for _, item := range items {
		normalized = append(normalized, normalizeHistoryMessage(item))
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]any{
		"mode":           "messages",
		"conversationId": req.ConversationID,
		"messages":       normalized,
		"raw":            data,
	}})
}

func (a *app) handleParseMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	var req parseMessageRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}

	msgType := strings.TrimSpace(req.MessageType)
	raw := req.Message
	if raw == nil {
		raw = map[string]any{}
	}
	msgData := req.MsgData
	if len(msgData) == 0 {
		if nested := mapValue(raw["msgData"]); len(nested) > 0 {
			msgData = nested
		}
	}
	if len(msgData) == 0 {
		msgData = raw
	}
	if msgType == "" {
		msgType = firstNonEmpty(
			anyToString(raw["messageType"]),
			userMessageTypeMap[int(anyToInt64(raw["msgType"]))],
		)
	}
	if msgType == "" {
		msgType = "unknown"
	}

	resourceURI := strings.TrimSpace(firstNonEmpty(req.ResourceURI, req.LocalPath))
	parsed, err := a.parseMessage(r.Context(), msgType, msgData, raw, resourceURI)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: parsed})
}

func (a *app) handleFacadeSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	var req facadeSendMessageRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "content is required"})
		return
	}

	toID := firstNonEmpty(req.ChannelConversationID, req.ChannelUserID)
	if toID == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "channelConversationId or channelUserId is required"})
		return
	}

	method, params, err := toFacadeQiweiMessageRequest(req, toID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}

	logger.Business(r.Context(), "facade 发送开始",
		"method", method,
		"toId", toID,
		"messageType", firstNonEmpty(req.MessageType, "text"),
		"content", req.Content,
	)

	res, err := a.client.doAPIRaw(r.Context(), method, params)
	if err != nil {
		logger.Error(r.Context(), "facade 发送失败",
			"method", method,
			"toId", toID,
			"error", err.Error(),
		)
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}
	data, err := decodeAPIData(res.Data)
	if err != nil {
		logger.Error(r.Context(), "facade 发送解码失败",
			"method", method,
			"toId", toID,
			"error", err.Error(),
			"rawData", string(res.Data),
		)
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}

	logger.Business(r.Context(), "facade 发送成功",
		"method", method,
		"toId", toID,
		"code", res.Code,
		"msg", res.Msg,
		"data", string(res.Data),
	)

	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]any{
		"method": method,
		"data":   data,
	}})
}

func (a *app) searchContacts(ctx context.Context, query string) ([]map[string]any, error) {
	if strings.TrimSpace(query) != "" {
		res, err := a.client.doAPIRaw(ctx, "/contact/searchContact", map[string]any{"keyword": query})
		if err != nil {
			return nil, err
		}
		data, err := decodeAPIData(res.Data)
		if err != nil {
			return nil, err
		}
		return extractItems(data, "contactList", "list", "rows", "data"), nil
	}

	externalRes, err := a.client.doAPIRaw(ctx, "/contact/getWxContactList", nil)
	if err != nil {
		return nil, err
	}
	internalRes, err := a.client.doAPIRaw(ctx, "/contact/getWxWorkContactList", nil)
	if err != nil {
		return nil, err
	}
	externalData, err := decodeAPIData(externalRes.Data)
	if err != nil {
		return nil, err
	}
	internalData, err := decodeAPIData(internalRes.Data)
	if err != nil {
		return nil, err
	}
	out := extractItems(externalData, "contactList", "list", "rows", "data")
	out = append(out, extractItems(internalData, "contactList", "list", "rows", "data")...)
	return out, nil
}

func (a *app) listGroups(ctx context.Context) ([]map[string]any, error) {
	res, err := a.client.doAPIRaw(ctx, "/room/getRoomList", nil)
	if err != nil {
		return nil, err
	}
	data, err := decodeAPIData(res.Data)
	if err != nil {
		return nil, err
	}
	return extractItems(data, "roomList", "list", "rows", "data"), nil
}

func (a *app) parseMessage(ctx context.Context, msgType string, msgData map[string]any, raw map[string]any, resourceURI string) (parsedMessage, error) {
	out := parsedMessage{
		MessageType: msgType,
		Raw:         raw,
	}

	switch msgType {
	case "text":
		out.Text = strings.TrimSpace(firstNonEmpty(anyToString(msgData["content"]), anyToString(raw["content"])))
		return out, nil
	case "rich_text":
		out.Text = strings.TrimSpace(firstNonEmpty(anyToString(msgData["content"]), anyToString(raw["content"])))
		return out, nil
	}

	if strings.TrimSpace(resourceURI) != "" {
		text, err := a.parsePreparedResource(ctx, msgType, resourceURI)
		if err != nil {
			return parsedMessage{}, err
		}
		out.Text = text
		return out, nil
	}

	prepared := a.prepareMediaForAgent(ctx, int(anyToInt64(raw["msgType"])), msgType, msgData)
	if prepared.MessageType != "" {
		out.MessageType = prepared.MessageType
	}
	out.Text = prepared.Content
	return out, nil
}

func (a *app) parsePreparedResource(ctx context.Context, msgType, resourceURI string) (string, error) {
	attachment := parsedAttachment{
		ResourceURI: resourceURI,
		LocalPath:   resourceURI,
		Name:        resourceBaseName(resourceURI),
		MIMEType:    mime.TypeByExtension(strings.ToLower(filepath.Ext(resourceURI))),
	}
	switch msgType {
	case "image":
		attachment.Kind = "image"
		dataURL, err := a.resourceAsDataURL(ctx, attachment)
		if err != nil {
			return "", err
		}
		attachment.DataURL = dataURL
		parsed, err := a.recognizer.ParseImage(ctx, attachment)
		if err != nil {
			return "", err
		}
		return firstNonEmpty(strings.TrimSpace(parsed.ParsedText), strings.TrimSpace(parsed.Summary), "[图片已解析]"), nil
	case "file":
		attachment.Kind = "document"
		text, err := a.extractPreparedText(ctx, attachment)
		if err == nil && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), nil
		}
		parsed, err := a.recognizer.ParseDocument(ctx, attachment)
		if err != nil {
			return "", err
		}
		return firstNonEmpty(strings.TrimSpace(parsed.ParsedText), strings.TrimSpace(parsed.Summary), "[文件已解析]"), nil
	default:
		return "", fmt.Errorf("resource parsing unsupported for messageType: %s", msgType)
	}
}

func (a *app) downloadAttachment(ctx context.Context, attachment parsedAttachment) (string, string, error) {
	if attachment.SourceURL == "" {
		return "", "", fmt.Errorf("attachment source url is required")
	}
	resp, err := a.http.Get(attachment.SourceURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	name := attachment.Name
	if name == "" {
		name = "attachment"
		switch attachment.Kind {
		case "image":
			name += ".jpg"
		case "audio":
			name += ".mp3"
		case "document":
			name += ".dat"
		default:
			name += ".dat"
		}
	}
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	}
	attachment.Name = name
	return a.uploadDownloadedAttachment(ctx, attachment, resp.Body, mimeType, resp.ContentLength)
}

func toFacadeQiweiMessageRequest(msg facadeSendMessageRequest, toID string) (string, map[string]any, error) {
	messageType := strings.TrimSpace(msg.MessageType)
	if messageType == "" {
		messageType = "text"
	}

	switch messageType {
	case "text":
		return "/msg/sendText", map[string]any{
			"toId":    toID,
			"content": msg.Content,
		}, nil
	case "rich_text", "hyper_text":
		return "/msg/sendHyperText", map[string]any{
			"toId":    toID,
			"content": msg.Content,
		}, nil
	case "image":
		return "/msg/sendImage", map[string]any{
			"toId":   toID,
			"imgUrl": msg.Content,
		}, nil
	case "file":
		fileName := "file"
		if msg.ChannelMeta != nil {
			if v, ok := msg.ChannelMeta["fileName"].(string); ok && strings.TrimSpace(v) != "" {
				fileName = v
			}
		}
		return "/msg/sendFile", map[string]any{
			"toId":     toID,
			"fileUrl":  msg.Content,
			"fileName": fileName,
		}, nil
	case "voice":
		return "/msg/sendVoice", map[string]any{
			"toId":     toID,
			"voiceUrl": msg.Content,
		}, nil
	default:
		return "", nil, fmt.Errorf("unsupported messageType: %s", messageType)
	}
}

func decodeAPIData(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var data any
	if err := unmarshalSafe(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func extractItems(data any, keys ...string) []map[string]any {
	if data == nil {
		return nil
	}
	if list, ok := data.([]any); ok {
		return toMapSlice(list)
	}
	m, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if list, ok := v.([]any); ok {
				return toMapSlice(list)
			}
		}
	}
	return nil
}

func toMapSlice(list []any) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func normalizeHistoryMessage(item map[string]any) map[string]any {
	msgType := firstNonEmpty(anyToString(item["messageType"]), userMessageTypeMap[int(anyToInt64(item["msgType"]))])
	msgData := mapValue(item["msgData"])
	return map[string]any{
		"messageId":   firstNonEmpty(anyToString(item["msgSvrId"]), anyToString(item["msgServerId"]), anyToString(item["messageId"]), anyToString(item["id"])),
		"messageType": firstNonEmpty(msgType, "unknown"),
		"content": firstNonEmpty(
			anyToString(item["content"]),
			anyToString(item["msgContent"]),
			anyToString(msgData["content"]),
		),
		"senderId":   firstNonEmpty(anyToString(item["senderId"]), anyToString(item["fromId"])),
		"senderName": firstNonEmpty(anyToString(item["senderName"]), anyToString(item["nickname"])),
		"timestamp":  firstNonZero(anyToInt64(item["createTime"]), anyToInt64(item["timestamp"])),
		"raw":        item,
	}
}

func (a *app) extractPreparedText(ctx context.Context, attachment parsedAttachment) (string, error) {
	resourceURI := strings.TrimSpace(firstNonEmpty(attachment.ResourceURI, attachment.LocalPath))
	if resourceURI == "" {
		return "", fmt.Errorf("resource uri is required")
	}
	ext := strings.ToLower(filepath.Ext(resourceURI))
	switch ext {
	case ".txt", ".md", ".markdown", ".json", ".csv", ".html", ".htm", ".xml":
		raw, _, err := a.readPreparedResource(ctx, attachment)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	default:
		return "", fmt.Errorf("unsupported text extraction for %s", ext)
	}
}

func summarizeText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 240 {
		return text
	}
	return text[:240] + "..."
}

func firstURL(data any) string {
	switch v := data.(type) {
	case string:
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return v
		}
	case map[string]any:
		for _, key := range []string{"url", "downloadUrl", "fileUrl", "cdnUrl", "cloudUrl", "coverUrl", "bigImgUrl"} {
			if s := anyToString(v[key]); strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
				return s
			}
		}
		for _, nestedKey := range []string{"data", "result"} {
			if nested := firstURL(v[nestedKey]); nested != "" {
				return nested
			}
		}
	case []any:
		for _, item := range v {
			if nested := firstURL(item); nested != "" {
				return nested
			}
		}
	}
	return ""
}

func inferredAttachmentName(kind, sourceURL string) string {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL != "" {
		candidate := sourceURL
		if idx := strings.Index(candidate, "?"); idx >= 0 {
			candidate = candidate[:idx]
		}
		if base := filepath.Base(candidate); base != "." && base != "/" && base != "" {
			if ext := strings.ToLower(filepath.Ext(base)); ext != "" {
				return base
			}
		}
	}

	switch kind {
	case "image":
		return "image.jpg"
	case "audio":
		return "voice.mp3"
	case "document":
		return "file.dat"
	default:
		return "attachment.dat"
	}
}

type volcengineRecognizer struct {
	cfg        Config
	httpClient *http.Client
}

func newVolcengineRecognizer(cfg Config) recognizer {
	return &volcengineRecognizer{
		cfg:        cfg,
		httpClient: logger.NewClient("volcengine", 30*time.Second),
	}
}

func (r *volcengineRecognizer) ParseImage(ctx context.Context, attachment parsedAttachment) (parsedAttachment, error) {
	if strings.TrimSpace(r.cfg.VolcArkAPIKey) == "" || strings.TrimSpace(r.cfg.VolcVisionModel) == "" {
		return parsedAttachment{}, fmt.Errorf("volc image provider is not configured")
	}
	dataURL := strings.TrimSpace(attachment.DataURL)
	if dataURL == "" {
		if strings.TrimSpace(attachment.LocalPath) == "" {
			return parsedAttachment{}, fmt.Errorf("image parsing requires resource data")
		}
		var err error
		dataURL, err = fileAsDataURL(attachment.LocalPath, attachment.MIMEType)
		if err != nil {
			return parsedAttachment{}, err
		}
	}
	body := map[string]any{
		"model": r.cfg.VolcVisionModel,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "请识别图片中的文字、关键信息，并给出简洁摘要。",
					},
					{
						"type": "image_url",
						"image_url": map[string]any{
							"url": dataURL,
						},
					},
				},
			},
		},
	}
	text, err := r.doArkChatCompletion(ctx, body)
	if err != nil {
		return parsedAttachment{}, err
	}
	attachment.ParseProvider = "volc-image"
	attachment.ParseStatus = "parsed"
	attachment.ParsedText = text
	attachment.Summary = summarizeText(text)
	return attachment, nil
}

func (r *volcengineRecognizer) ParseDocument(ctx context.Context, attachment parsedAttachment) (parsedAttachment, error) {
	if strings.TrimSpace(r.cfg.VolcDocumentModel) == "" || strings.TrimSpace(r.cfg.VolcArkAPIKey) == "" {
		return parsedAttachment{}, fmt.Errorf("volc document provider is not configured")
	}
	text, err := readDocumentTextForModel(attachment)
	if err != nil {
		return parsedAttachment{}, err
	}
	body := map[string]any{
		"model": r.cfg.VolcDocumentModel,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": "请阅读下面的文档内容，提取关键信息，并给出简洁摘要。\n\n" +
					"文档内容如下：\n" + text,
			},
		},
	}
	summary, err := r.doArkChatCompletion(ctx, body)
	if err != nil {
		return parsedAttachment{}, err
	}
	attachment.ParseProvider = "volc-document"
	attachment.ParseStatus = "parsed"
	attachment.ParsedText = summary
	attachment.Summary = summarizeText(summary)
	return attachment, nil
}

func (r *volcengineRecognizer) SubmitAudioTranscription(ctx context.Context, attachment parsedAttachment) (string, error) {
	if strings.TrimSpace(r.cfg.VolcSpeechAppKey) == "" || strings.TrimSpace(r.cfg.VolcSpeechAccessKey) == "" || strings.TrimSpace(r.cfg.VolcSpeechResourceID) == "" {
		return "", fmt.Errorf("volc speech provider is not configured")
	}
	if attachment.SourceURL == "" {
		return "", fmt.Errorf("audio transcription requires a source url")
	}
	requestID := fmt.Sprintf("qiwei-audio-%d", time.Now().UnixNano())
	body := map[string]any{
		"user": map[string]any{
			"uid": "channel-qiwei",
		},
		"audio": map[string]any{
			"format": detectAudioFormat(attachment),
			"url":    attachment.SourceURL,
		},
		"request": map[string]any{
			"model_name":      "bigmodel",
			"enable_itn":      true,
			"enable_punc":     true,
			"show_utterances": true,
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.VolcSpeechSubmitURL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-App-Key", r.cfg.VolcSpeechAppKey)
	req.Header.Set("X-Api-Access-Key", r.cfg.VolcSpeechAccessKey)
	req.Header.Set("X-Api-Resource-Id", r.cfg.VolcSpeechResourceID)
	req.Header.Set("X-Api-Request-Id", requestID)
	req.Header.Set("X-Api-Sequence", "-1")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("volc speech submit failed: HTTP %d %s", resp.StatusCode, string(body))
	}
	return requestID, nil
}

func (r *volcengineRecognizer) QueryAudioTranscription(ctx context.Context, taskID string) (parsedAttachment, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.VolcSpeechQueryURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return parsedAttachment{}, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-App-Key", r.cfg.VolcSpeechAppKey)
	req.Header.Set("X-Api-Access-Key", r.cfg.VolcSpeechAccessKey)
	req.Header.Set("X-Api-Resource-Id", r.cfg.VolcSpeechResourceID)
	req.Header.Set("X-Api-Request-Id", taskID)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return parsedAttachment{}, false, err
	}
	defer resp.Body.Close()

	statusCode := resp.Header.Get("X-Api-Status-Code")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parsedAttachment{}, false, fmt.Errorf("volc speech query failed: HTTP %d %s", resp.StatusCode, string(body))
	}
	switch statusCode {
	case "20000001", "20000002":
		return parsedAttachment{}, false, nil
	case "", "20000000":
	default:
		return parsedAttachment{}, false, fmt.Errorf("volc speech query failed: code=%s body=%s", statusCode, string(body))
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return parsedAttachment{}, false, err
	}
	result := mapValue(payload["result"])
	text := strings.TrimSpace(anyToString(result["text"]))
	return parsedAttachment{
		ParseProvider: "volc-speech",
		ParseStatus:   "parsed",
		ParsedText:    text,
		Summary:       summarizeText(text),
		Raw:           payload,
	}, true, nil
}

func (r *volcengineRecognizer) doArkChatCompletion(ctx context.Context, body map[string]any) (string, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.VolcArkBaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.cfg.VolcArkAPIKey)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("volc ark request failed: HTTP %d %s", resp.StatusCode, string(bodyBytes))
	}
	var payload map[string]any
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return "", err
	}
	if rawChoices, ok := payload["choices"].([]any); ok {
		for _, item := range rawChoices {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			message := mapValue(m["message"])
			if content := anyToString(message["content"]); strings.TrimSpace(content) != "" {
				return content, nil
			}
		}
	}
	return "", fmt.Errorf("volc ark response missing choices")
}

func (a *app) resourceAsDataURL(ctx context.Context, attachment parsedAttachment) (string, error) {
	raw, detectedMime, err := a.readPreparedResource(ctx, attachment)
	if err != nil {
		return "", err
	}
	mimeType := strings.TrimSpace(attachment.MIMEType)
	if mimeType == "" {
		mimeType = strings.TrimSpace(firstNonEmpty(attachment.MIMEType, detectedMime))
	}
	if mimeType == "" {
		resourceURI := firstNonEmpty(attachment.ResourceURI, attachment.LocalPath, attachment.Name)
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(resourceURI)))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

func fileAsDataURL(path, mimeType string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

func readDocumentTextForModel(attachment parsedAttachment) (string, error) {
	path := strings.TrimSpace(firstNonEmpty(attachment.LocalPath, attachment.ResourceURI))
	if path == "" {
		return "", fmt.Errorf("document parsing requires a local text file")
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".md", ".markdown", ".json", ".csv", ".html", ".htm", ".xml":
	default:
		return "", fmt.Errorf("binary document understanding is not implemented yet")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", fmt.Errorf("document parsing requires non-empty content")
	}
	const maxRunes = 12000
	runes := []rune(text)
	if len(runes) > maxRunes {
		text = string(runes[:maxRunes])
	}
	return text, nil
}

func detectAudioFormat(attachment parsedAttachment) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(attachment.Name)), ".")
	switch ext {
	case "wav", "mp3", "ogg", "silk":
		return ext
	default:
		return "mp3"
	}
}
