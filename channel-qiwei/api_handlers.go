package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// resolveOutgoingFromRequest picks the runtime for an outgoing request.
// Priority: explicit account_id > channelConversationId suffix > single
// default account. Any mismatch returns a descriptive error the handler can
// surface as a 4xx to the caller.
func (a *app) resolveOutgoingFromRequest(accountID, composite string) (*accountRuntime, string, error) {
	reg := a.currentRegistry()
	if strings.TrimSpace(accountID) != "" {
		rt, err := resolveByAccountID(reg, accountID)
		if err != nil {
			return nil, "", err
		}
		raw, _ := decodeConversationID(composite)
		return rt, raw, nil
	}
	if strings.TrimSpace(composite) != "" {
		return resolveOutgoingTarget(reg, composite)
	}
	if rt, ok := reg.Default(); ok {
		return rt, "", nil
	}
	return nil, "", ErrAmbiguousAccount
}

func (a *app) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	var msg outgoingMessage
	if err := decodeJSON(r.Body, &msg); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	if strings.TrimSpace(msg.Content) == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "content is required"})
		return
	}
	if strings.TrimSpace(msg.MessageType) == "" {
		msg.MessageType = "text"
	}

	composite := msg.ChannelConversationID
	if composite == "" {
		composite = msg.ChannelUserID
	}
	if composite == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "channelConversationId or channelUserId is required"})
		return
	}

	rt, toID, err := a.resolveOutgoingFromRequest(msg.AccountID, composite)
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	if toID == "" {
		// account_id was provided but conversation id had no suffix;
		// fall back to the raw composite (it's a plain id).
		toID = composite
	}

	var method string
	var params map[string]any
	mentionInfo := prepareMentionSend(msg.Mentions, msg.ChannelConversationID, msg.ChannelUserID, msg.MessageType)

	if isMediaMessageType(msg.MessageType) {
		method, params, err = a.resolveMediaSendParams(r.Context(), rt, msg.MessageType, toID, msg.Content, msg.ChannelMeta)
	} else {
		method, params, err = toQiweiMessageRequest(msg, toID)
		applyMentionParams(&method, params, mentionInfo)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	fallbackMethod, fallbackParams := mentionFallback(method, params, mentionInfo)
	data, mentionStatus, mentionError, err := a.sendWithMentionFallback(r.Context(), rt, method, params, fallbackMethod, fallbackParams, mentionInfo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: sendResponseData(data, mentionStatus, mentionError)})
}

func statusForRouting(err error) int {
	if err == nil {
		return http.StatusOK
	}
	switch {
	case errors.Is(err, ErrAmbiguousAccount), errors.Is(err, ErrUnknownAccount):
		return http.StatusBadRequest
	default:
		return http.StatusBadRequest
	}
}

func toQiweiMessageRequest(msg outgoingMessage, toID string) (string, map[string]any, error) {
	meta := msg.ChannelMeta
	if meta == nil {
		meta = map[string]any{}
	}

	// Auto-construct reply object from replyToChannelMessageId when channelMeta.reply is absent.
	replyObj := mapValue(meta["reply"])
	if len(replyObj) == 0 && strings.TrimSpace(msg.ReplyToChannelMessageID) != "" {
		replyObj = map[string]any{"msgSvrId": strings.TrimSpace(msg.ReplyToChannelMessageID)}
	}

	switch msg.MessageType {
	case "text":
		params := map[string]any{"toId": toID, "content": msg.Content}
		if len(replyObj) > 0 {
			params["reply"] = replyObj
		}
		return "/msg/sendText", params, nil
	case "rich_text":
		params := map[string]any{"toId": toID, "content": msg.Content}
		if len(replyObj) > 0 {
			params["reply"] = replyObj
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
		return "", nil, fmt.Errorf("unsupported messageType: %s", msg.MessageType)
	}
}

type mentionSendInfo struct {
	Enabled bool
	Status  string
	Error   string
	Targets []mentionTarget
}

func prepareMentionSend(raw []mentionTarget, channelConversationID, channelUserID, messageType string) mentionSendInfo {
	if len(raw) == 0 {
		return mentionSendInfo{}
	}
	if !isGroupConversation(channelConversationID, channelUserID) {
		return mentionSendInfo{Status: "ignored_private_chat"}
	}
	targets := normalizeMentionTargets(raw)
	if len(targets) == 0 {
		return mentionSendInfo{Status: "degraded", Error: "no valid mention userId"}
	}
	if !supportsMentionMessageType(messageType) {
		return mentionSendInfo{Status: "ignored_unsupported_message_type", Targets: targets}
	}
	return mentionSendInfo{Enabled: true, Targets: targets}
}

func normalizeMentionTargets(raw []mentionTarget) []mentionTarget {
	out := make([]mentionTarget, 0, len(raw))
	seen := make(map[string]bool)
	for _, item := range raw {
		userID := strings.TrimSpace(item.UserID)
		if userID == "" || isMentionAllTarget(userID) || seen[userID] {
			continue
		}
		seen[userID] = true
		out = append(out, mentionTarget{
			UserID: userID,
			Name:   strings.TrimSpace(item.Name),
		})
	}
	return out
}

func isMentionAllTarget(userID string) bool {
	switch strings.ToLower(strings.TrimSpace(userID)) {
	case "all", "@all", "notify@all", "所有人", "@所有人":
		return true
	default:
		return false
	}
}

func isGroupConversation(channelConversationID, channelUserID string) bool {
	convID := strings.TrimSpace(channelConversationID)
	return convID != "" && convID != strings.TrimSpace(channelUserID)
}

func supportsMentionMessageType(messageType string) bool {
	switch strings.TrimSpace(messageType) {
	case "", "text", "rich_text", "hyper_text":
		return true
	default:
		return false
	}
}

func applyMentionParams(method *string, params map[string]any, info mentionSendInfo) {
	if !info.Enabled || len(info.Targets) == 0 || params == nil {
		return
	}
	if method != nil {
		*method = "/msg/sendHyperText"
	}
	if content := anyToString(params["content"]); content != "" {
		params["content"] = hyperTextMentionContent(content, info.Targets)
	}
}

func hyperTextMentionContent(content string, targets []mentionTarget) []map[string]any {
	out := make([]map[string]any, 0, len(targets)+1)
	for _, target := range targets {
		out = append(out, map[string]any{"subtype": 1, "text": target.UserID})
	}
	if strings.TrimSpace(content) != "" {
		out = append(out, map[string]any{"subtype": 0, "text": " " + strings.TrimSpace(content)})
	}
	return out
}

func contentWithMentionPrefix(content string, targets []mentionTarget) string {
	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		label := strings.TrimSpace(target.Name)
		if label == "" {
			label = target.UserID
		}
		labels = append(labels, "@"+label)
	}
	if len(labels) == 0 {
		return content
	}
	return strings.Join(labels, " ") + " " + strings.TrimSpace(content)
}

func mentionFallback(method string, params map[string]any, info mentionSendInfo) (string, map[string]any) {
	fallbackParams := cloneSendParams(params)
	if info.Enabled {
		fallbackParams["content"] = contentWithMentionPrefix(sendTextContent(fallbackParams["content"]), info.Targets)
		return "/msg/sendText", fallbackParams
	}
	return method, fallbackParams
}

func sendTextContent(raw any) string {
	if content := anyToString(raw); content != "" {
		return content
	}
	var parts []string
	switch v := raw.(type) {
	case []map[string]any:
		for _, item := range v {
			if anyToInt64(item["subtype"]) == 0 {
				parts = append(parts, anyToString(item["text"]))
			}
		}
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok && anyToInt64(m["subtype"]) == 0 {
				parts = append(parts, anyToString(m["text"]))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func cloneSendParams(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (a *app) sendWithMentionFallback(ctx context.Context, rt *accountRuntime, method string, params map[string]any, fallbackMethod string, fallbackParams map[string]any, info mentionSendInfo) (any, string, string, error) {
	data, err := sendQiweiRaw(ctx, rt, method, params)
	if err == nil {
		if info.Enabled {
			return data, "attempted", "", nil
		}
		return data, info.Status, info.Error, nil
	}
	if !info.Enabled {
		return nil, "", "", err
	}

	fallbackData, fallbackErr := sendQiweiRaw(ctx, rt, fallbackMethod, fallbackParams)
	if fallbackErr != nil {
		return nil, "", "", fallbackErr
	}
	return fallbackData, "degraded", err.Error(), nil
}

func sendQiweiRaw(ctx context.Context, rt *accountRuntime, method string, params map[string]any) (any, error) {
	res, err := rt.client.doAPIRaw(ctx, method, params)
	if err != nil {
		return nil, err
	}
	var data any
	if len(res.Data) > 0 {
		if err := unmarshalSafe(res.Data, &data); err != nil {
			return nil, err
		}
	}
	if errMsg := checkSendSuccess(data); errMsg != "" {
		return nil, fmt.Errorf("%s", errMsg)
	}
	return data, nil
}

func sendResponseData(data any, mentionStatus, mentionError string) any {
	if mentionStatus == "" && mentionError == "" {
		return data
	}
	out := map[string]any{
		"data":          data,
		"mentionStatus": mentionStatus,
	}
	if mentionError != "" {
		out["mentionError"] = mentionError
	}
	return out
}

// doAPIRequest is the shared body shape for the generic /do and module
// passthrough endpoints. account_id is the explicit opt-in; callers who
// keep using channelConversationId inside params get the same routing for
// free.
type doAPIRequest struct {
	Method    string         `json:"method,omitempty"`
	AccountID string         `json:"account_id,omitempty"`
	Params    map[string]any `json:"params"`
}

// resolveRuntimeFromParams extracts a routing key from a module/do body and
// returns the corresponding runtime. If no hint is present and there's a
// single default account, we use it.
func (a *app) resolveRuntimeFromParams(req doAPIRequest) (*accountRuntime, error) {
	if id := strings.TrimSpace(req.AccountID); id != "" {
		return resolveByAccountID(a.currentRegistry(), id)
	}
	if req.Params != nil {
		if v, ok := req.Params["channelConversationId"]; ok {
			if composite := anyToString(v); composite != "" {
				rt, _, err := resolveOutgoingTarget(a.currentRegistry(), composite)
				return rt, err
			}
		}
	}
	if rt, ok := a.currentRegistry().Default(); ok {
		return rt, nil
	}
	return nil, ErrAmbiguousAccount
}

func (a *app) handleDoAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	var req doAPIRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	if strings.TrimSpace(req.Method) == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "method is required"})
		return
	}
	rt, err := a.resolveRuntimeFromParams(req)
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	a.handleModuleCall(w, r.Context(), rt, req.Method, req.Params)
}

func (a *app) handleModuleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/qiwei/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "path should be /api/qiwei/{module}/{action}"})
		return
	}
	moduleName, action := parts[0], parts[1]
	actions, ok := a.modules[moduleName]
	if !ok {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "unknown module"})
		return
	}
	method, ok := actions[action]
	if !ok {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "unknown action"})
		return
	}

	var req doAPIRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	rt, err := a.resolveRuntimeFromParams(req)
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	a.handleModuleCall(w, r.Context(), rt, method, req.Params)
}

func (a *app) handleModuleCall(w http.ResponseWriter, ctx context.Context, rt *accountRuntime, method string, params map[string]any) {
	res, err := rt.client.doAPIRaw(ctx, method, params)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}
	var data any
	if len(res.Data) > 0 {
		_ = unmarshalSafe(res.Data, &data)
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]any{
		"code":      res.Code,
		"msg":       res.Msg,
		"data":      data,
		"method":    method,
		"accountId": rt.AccountID(),
	}})
}
