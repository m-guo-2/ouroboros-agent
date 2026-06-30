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
	AccountID       string `json:"account_id,omitempty"`
	Query           string `json:"query"`
	Limit           int    `json:"limit"`
	IncludeContacts *bool  `json:"includeContacts,omitempty"`
	IncludeGroups   *bool  `json:"includeGroups,omitempty"`
}

type listOrGetConversationsRequest struct {
	AccountID      string `json:"account_id,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	MsgSvrID       string `json:"msgSvrId,omitempty"`
	CurrentSeq     int64  `json:"currentSeq,omitempty"`
	PageSize       int    `json:"pageSize,omitempty"`
}

type parseMessageRequest struct {
	AccountID   string         `json:"account_id,omitempty"`
	Message     map[string]any `json:"message,omitempty"`
	MessageType string         `json:"messageType,omitempty"`
	MsgData     map[string]any `json:"msgData,omitempty"`
	ResourceURI string         `json:"resourceUri,omitempty"`
	LocalPath   string         `json:"localPath,omitempty"`
	Goal        string         `json:"goal,omitempty"`
}

type facadeSendMessageRequest struct {
	AccountID             string          `json:"account_id,omitempty"`
	ChannelConversationID string          `json:"channelConversationId,omitempty"`
	ChannelUserID         string          `json:"channelUserId,omitempty"`
	MessageType           string          `json:"messageType,omitempty"`
	Content               string          `json:"content"`
	ChannelMeta           map[string]any  `json:"channelMeta,omitempty"`
	Mentions              []mentionTarget `json:"mentions,omitempty"`
}

// resolveRuntimeForFacade picks the runtime for a facade/admin-adjacent
// endpoint that has no explicit conversation id. Priority matches the rest
// of the code: explicit account_id > default single account > error.
func (a *app) resolveRuntimeForFacade(accountID string) (*accountRuntime, error) {
	reg := a.currentRegistry()
	if strings.TrimSpace(accountID) != "" {
		return resolveByAccountID(reg, accountID)
	}
	if rt, ok := reg.Default(); ok {
		return rt, nil
	}
	return nil, ErrAmbiguousAccount
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
	AnalysisGoal  string         `json:"-"`
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
	SubmitAudioTranscription(ctx context.Context, audioData []byte, audioFormat string) (string, error)
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
	rt, err := a.resolveRuntimeForFacade(req.AccountID)
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
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
		contacts, err := a.searchContacts(r.Context(), rt, req.Query)
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
		groups, err := a.listGroups(r.Context(), rt)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
			return
		}
		query := strings.ToLower(strings.TrimSpace(req.Query))
		for _, item := range groups {
			if len(resp.Targets) >= limit {
				break
			}
			name := decodeMaybeBase64(firstNonEmpty(anyToString(item["roomName"]), anyToString(item["name"])))
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

	// The conversationId — if present — already carries an @shortHash
	// suffix in multi-account deployments, so it's the strongest routing
	// hint. Fall back to account_id, then to the default account.
	rt, toID, err := a.resolveOutgoingFromRequest(req.AccountID, req.ConversationID)
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	if toID == "" {
		toID = req.ConversationID
	}

	if strings.TrimSpace(req.ConversationID) == "" {
		params := map[string]any{}
		if req.CurrentSeq != 0 {
			params["currentSeq"] = req.CurrentSeq
		}
		if req.PageSize > 0 {
			params["pageSize"] = req.PageSize
		}
		res, err := rt.client.doAPIRaw(r.Context(), "/session/getSessionPage", params)
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

	params := map[string]any{"toId": toID}
	if strings.TrimSpace(req.MsgSvrID) != "" {
		params["msgSvrId"] = req.MsgSvrID
	}
	res, err := rt.client.doAPIRaw(r.Context(), "/msg/syncMsg", params)
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
	rt, err := a.resolveRuntimeForFacade(req.AccountID)
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
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
	parsed, err := a.parseMessage(r.Context(), rt, msgType, msgData, raw, resourceURI, req.Goal)
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

	composite := firstNonEmpty(req.ChannelConversationID, req.ChannelUserID)
	if composite == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "channelConversationId or channelUserId is required"})
		return
	}
	rt, toID, err := a.resolveOutgoingFromRequest(req.AccountID, composite)
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	if toID == "" {
		toID = composite
	}

	messageType := strings.TrimSpace(req.MessageType)
	if messageType == "" {
		messageType = "text"
	}

	var method string
	var params map[string]any
	mentionInfo := prepareMentionSend(req.Mentions, req.ChannelConversationID, req.ChannelUserID, messageType)

	if isMediaMessageType(messageType) {
		method, params, err = a.resolveMediaSendParams(r.Context(), rt, messageType, toID, req.Content, req.ChannelMeta)
	} else {
		method, params, err = toFacadeQiweiMessageRequest(req, toID)
		applyMentionParams(&method, params, mentionInfo)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}

	logger.Business(r.Context(), "facade 发送开始",
		"accountId", rt.AccountID(),
		"method", method,
		"toId", toID,
		"messageType", messageType,
		"content", req.Content,
	)

	fallbackMethod, fallbackParams := mentionFallback(method, params, mentionInfo)
	data, mentionStatus, mentionError, err := a.sendWithMentionFallback(r.Context(), rt, method, params, fallbackMethod, fallbackParams, mentionInfo)
	if err != nil {
		logger.Error(r.Context(), "facade 发送失败",
			"method", method,
			"toId", toID,
			"error", err.Error(),
		)
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}

	logger.Business(r.Context(), "facade 发送成功",
		"method", method,
		"toId", toID,
		"mentionStatus", mentionStatus,
		"mentionError", mentionError,
	)

	out := map[string]any{
		"method": method,
		"data":   data,
	}
	if mentionStatus != "" {
		out["mentionStatus"] = mentionStatus
	}
	if mentionError != "" {
		out["mentionError"] = mentionError
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: out})
}

func (a *app) handleGetGroupDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	var req struct {
		AccountID string   `json:"account_id,omitempty"`
		RoomIDs   []string `json:"roomIds"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	if len(req.RoomIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "roomIds is required"})
		return
	}
	rt, err := a.resolveRuntimeForFacade(req.AccountID)
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}

	res, err := rt.client.doAPIRaw(r.Context(), "/room/batchGetRoomDetail", map[string]any{
		"roomIdList": req.RoomIDs,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}
	var wrapper struct {
		RoomList []map[string]any `json:"roomList"`
	}
	if err := unmarshalSafe(res.Data, &wrapper); err != nil {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}

	type member struct {
		UserID string `json:"userId"`
		Name   string `json:"name"`
	}
	type groupDetail struct {
		RoomID       string   `json:"roomId"`
		RoomName     string   `json:"roomName"`
		Announcement string   `json:"announcement"`
		CreateUserID string   `json:"createUserId"`
		MemberCount  int      `json:"memberCount"`
		Members      []member `json:"members"`
	}

	groups := make([]groupDetail, 0, len(wrapper.RoomList))
	for _, room := range wrapper.RoomList {
		roomName := decodeMaybeBase64(anyToString(room["roomName"]))
		announcement := decodeMaybeBase64(anyToString(room["announcement"]))
		var members []member
		if rawMembers, ok := room["memberList"].([]any); ok {
			for _, rm := range rawMembers {
				m, ok := rm.(map[string]any)
				if !ok {
					continue
				}
				members = append(members, member{
					UserID: anyToString(m["userId"]),
					Name:   decodeMaybeBase64(anyToString(m["name"])),
				})
			}
		}
		groups = append(groups, groupDetail{
			RoomID:       anyToString(room["roomId"]),
			RoomName:     roomName,
			Announcement: announcement,
			CreateUserID: anyToString(room["createUserId"]),
			MemberCount:  int(anyToInt64(room["memberCount"])),
			Members:      members,
		})
	}

	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: groups})
}

func (a *app) handleGetContactDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	var req struct {
		AccountID string   `json:"account_id,omitempty"`
		UserIDs   []string `json:"userIds"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	if len(req.UserIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "userIds is required"})
		return
	}
	rt, err := a.resolveRuntimeForFacade(req.AccountID)
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}

	res, err := rt.client.doAPIRaw(r.Context(), "/contact/batchGetUserinfo", map[string]any{
		"userIdList": req.UserIDs,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}
	var wrapper struct {
		ContactList []map[string]any `json:"contactList"`
	}
	if err := unmarshalSafe(res.Data, &wrapper); err != nil {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}

	type contactDetail struct {
		UserID    string `json:"userId"`
		Nickname  string `json:"nickname"`
		RealName  string `json:"realName"`
		Alias     string `json:"alias"`
		CorpID    string `json:"corpId"`
		Gender    string `json:"gender"`
		AvatarURL string `json:"avatarUrl"`
	}

	contacts := make([]contactDetail, 0, len(wrapper.ContactList))
	for _, c := range wrapper.ContactList {
		contacts = append(contacts, contactDetail{
			UserID:    anyToString(c["userId"]),
			Nickname:  decodeMaybeBase64(anyToString(c["nickname"])),
			RealName:  decodeMaybeBase64(anyToString(c["realName"])),
			Alias:     decodeMaybeBase64(anyToString(c["alias"])),
			CorpID:    anyToString(c["corpId"]),
			Gender:    anyToString(c["gender"]),
			AvatarURL: anyToString(c["avatarUrl"]),
		})
	}

	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: contacts})
}

func (a *app) searchContacts(ctx context.Context, rt *accountRuntime, query string) ([]map[string]any, error) {
	if strings.TrimSpace(query) != "" {
		res, err := rt.client.doAPIRaw(ctx, "/contact/searchContact", map[string]any{"keyword": query})
		if err != nil {
			return nil, err
		}
		data, err := decodeAPIData(res.Data)
		if err != nil {
			return nil, err
		}
		return extractItems(data, "contactList", "list", "rows", "data"), nil
	}

	externalRes, err := rt.client.doAPIRaw(ctx, "/contact/getWxContactList", nil)
	if err != nil {
		return nil, err
	}
	internalRes, err := rt.client.doAPIRaw(ctx, "/contact/getWxWorkContactList", nil)
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

func (a *app) listGroups(ctx context.Context, rt *accountRuntime) ([]map[string]any, error) {
	res, err := rt.client.doAPIRaw(ctx, "/room/getRoomList", nil)
	if err != nil {
		return nil, err
	}
	data, err := decodeAPIData(res.Data)
	if err != nil {
		return nil, err
	}
	return extractItems(data, "roomList", "list", "rows", "data"), nil
}

func (a *app) parseMessage(ctx context.Context, rt *accountRuntime, msgType string, msgData map[string]any, raw map[string]any, resourceURI, goal string) (parsedMessage, error) {
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
		text, err := a.parsePreparedResource(ctx, msgType, resourceURI, goal)
		if err != nil {
			return parsedMessage{}, err
		}
		out.Text = text
		return out, nil
	}

	prepared := a.prepareMediaForAgent(ctx, rt, int(anyToInt64(raw["msgType"])), msgType, msgData)
	if prepared.MessageType != "" {
		out.MessageType = prepared.MessageType
	}
	out.Text = prepared.Content
	return out, nil
}

func (a *app) parsePreparedResource(ctx context.Context, msgType, resourceURI, goal string) (string, error) {
	attachment := parsedAttachment{
		ResourceURI:  resourceURI,
		LocalPath:    resourceURI,
		Name:         resourceBaseName(resourceURI),
		MIMEType:     mime.TypeByExtension(strings.ToLower(filepath.Ext(resourceURI))),
		AnalysisGoal: strings.TrimSpace(goal),
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
	mimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	name := resolveDownloadedAttachmentName(attachment, resp.Header, mimeType)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	}
	attachment.Name = name
	return a.uploadDownloadedAttachment(ctx, attachment, resp.Body, mimeType, resp.ContentLength)
}

func resolveDownloadedAttachmentName(attachment parsedAttachment, headers http.Header, contentType string) string {
	name := strings.TrimSpace(attachment.Name)
	if isGenericAttachmentName(name) || filepath.Ext(name) == "" {
		if headerName := attachmentNameFromContentDisposition(headers.Get("Content-Disposition")); headerName != "" {
			name = headerName
		}
	}
	if isGenericAttachmentName(name) || filepath.Ext(name) == "" {
		if sourceName := attachmentNameFromURL(attachment.SourceURL); sourceName != "" {
			name = sourceName
		}
	}
	if name == "" {
		name = defaultAttachmentName(attachment.Kind)
	}
	if filepath.Ext(name) != "" && !isGenericAttachmentName(name) {
		return name
	}

	ext := firstNonEmpty(
		strings.ToLower(filepath.Ext(attachmentNameFromContentDisposition(headers.Get("Content-Disposition")))),
		strings.ToLower(filepath.Ext(attachmentNameFromURL(attachment.SourceURL))),
		extensionFromContentType(contentType),
		extensionFromContentType(attachment.MIMEType),
	)
	if ext == "" {
		return name
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if stem == "" {
		stem = strings.TrimSuffix(defaultAttachmentName(attachment.Kind), filepath.Ext(defaultAttachmentName(attachment.Kind)))
	}
	return stem + ext
}

func attachmentNameFromContentDisposition(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	return sanitizeAttachmentFileName(firstNonEmpty(params["filename"], params["filename*"]))
}

func attachmentNameFromURL(sourceURL string) string {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return ""
	}
	candidate := sourceURL
	if idx := strings.Index(candidate, "?"); idx >= 0 {
		candidate = candidate[:idx]
	}
	if idx := strings.Index(candidate, "#"); idx >= 0 {
		candidate = candidate[:idx]
	}
	return sanitizeAttachmentFileName(filepath.Base(candidate))
}

func sanitizeAttachmentFileName(name string) string {
	name = strings.TrimSpace(strings.Trim(name, `"`))
	if name == "" {
		return ""
	}
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if base == "." || base == "/" {
		return ""
	}
	return base
}

func defaultAttachmentName(kind string) string {
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

func isGenericAttachmentName(name string) bool {
	name = strings.ToLower(sanitizeAttachmentFileName(name))
	switch name {
	case "", "attachment", "attachment.dat", "file", "file.dat", "voice", "voice.mp3":
		return true
	default:
		return false
	}
}

func extensionFromContentType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if mediaType, _, err := mime.ParseMediaType(value); err == nil && mediaType != "" {
		value = mediaType
	}
	switch strings.ToLower(value) {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	}
	exts, err := mime.ExtensionsByType(value)
	if err != nil {
		return ""
	}
	for _, ext := range exts {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext != "" {
			return ext
		}
	}
	return ""
}

func toFacadeQiweiMessageRequest(msg facadeSendMessageRequest, toID string) (string, map[string]any, error) {
	messageType := strings.TrimSpace(msg.MessageType)
	if messageType == "" {
		messageType = "text"
	}
	meta := msg.ChannelMeta
	if meta == nil {
		meta = map[string]any{}
	}

	switch messageType {
	case "text":
		params := map[string]any{"toId": toID, "content": msg.Content}
		if reply := mapValue(meta["reply"]); len(reply) > 0 {
			params["reply"] = reply
		}
		return "/msg/sendText", params, nil
	case "rich_text", "hyper_text":
		params := map[string]any{"toId": toID, "content": msg.Content}
		if reply := mapValue(meta["reply"]); len(reply) > 0 {
			params["reply"] = reply
		}
		return "/msg/sendHyperText", params, nil
	case "link":
		return "/msg/sendLink", map[string]any{
			"toId":    toID,
			"title":   anyToString(meta["title"]),
			"desc":    anyToString(meta["desc"]),
			"linkUrl": firstNonEmpty(anyToString(meta["linkUrl"]), msg.Content),
			"iconUrl": anyToString(meta["iconUrl"]),
		}, nil
	case "location":
		return "/msg/sendLocation", map[string]any{
			"toId":      toID,
			"title":     anyToString(meta["title"]),
			"address":   anyToString(meta["address"]),
			"latitude":  anyToString(meta["latitude"]),
			"longitude": anyToString(meta["longitude"]),
		}, nil
	case "miniapp":
		params := map[string]any{"toId": toID}
		for k, v := range meta {
			if k != "toId" {
				params[k] = v
			}
		}
		return "/msg/sendWeapp", params, nil
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
	if base := attachmentNameFromURL(sourceURL); base != "" && filepath.Ext(base) != "" {
		return base
	}
	return defaultAttachmentName(kind)
}

type volcengineRecognizer struct {
	cfg        Config
	httpClient *http.Client
}

func newVolcengineRecognizer(cfg Config) recognizer {
	return &volcengineRecognizer{
		cfg:        cfg,
		httpClient: logger.NewClient("volcengine", 120*time.Second),
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
	prompt := "请识别图片中的文字、关键信息，并给出简洁摘要。"
	if attachment.AnalysisGoal != "" {
		prompt = "请根据以下目标分析图片，并只返回与目标相关的客观信息。看不清或无法确认的内容要明确说明，不要猜测。\n\n分析目标：" + attachment.AnalysisGoal
	}
	body := map[string]any{
		"model":      r.cfg.VolcVisionModel,
		"max_tokens": 1024,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": prompt,
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

func (r *volcengineRecognizer) SubmitAudioTranscription(ctx context.Context, audioData []byte, audioFormat string) (string, error) {
	if strings.TrimSpace(r.cfg.VolcSpeechAppKey) == "" || strings.TrimSpace(r.cfg.VolcSpeechAccessKey) == "" || strings.TrimSpace(r.cfg.VolcSpeechResourceID) == "" {
		return "", fmt.Errorf("volc speech provider is not configured")
	}
	if len(audioData) == 0 {
		return "", fmt.Errorf("audio transcription requires audio data")
	}
	requestID := fmt.Sprintf("qiwei-audio-%d", time.Now().UnixNano())
	body := map[string]any{
		"user": map[string]any{
			"uid": "channel-qiwei",
		},
		"audio": map[string]any{
			"format": audioFormat,
			"data":   base64.StdEncoding.EncodeToString(audioData),
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
	respBody, _ := io.ReadAll(resp.Body)
	statusCode := resp.Header.Get("X-Api-Status-Code")
	apiMessage := resp.Header.Get("X-Api-Message")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("volc speech submit failed: HTTP %d code=%s msg=%s body=%s", resp.StatusCode, statusCode, apiMessage, string(respBody))
	}
	if statusCode != "" && statusCode != "20000000" {
		return "", fmt.Errorf("volc speech submit rejected: code=%s msg=%s", statusCode, apiMessage)
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
	apiMessage := resp.Header.Get("X-Api-Message")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parsedAttachment{}, false, fmt.Errorf("volc speech query failed: HTTP %d code=%s msg=%s body=%s", resp.StatusCode, statusCode, apiMessage, string(body))
	}
	switch statusCode {
	case "20000001", "20000002":
		return parsedAttachment{}, false, nil
	case "", "20000000", "20000003":
	default:
		return parsedAttachment{}, false, fmt.Errorf("volc speech query failed: code=%s msg=%s", statusCode, apiMessage)
	}

	var payload map[string]any
	if err := unmarshalSafe(body, &payload); err != nil {
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
	if err := unmarshalSafe(bodyBytes, &payload); err != nil {
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
