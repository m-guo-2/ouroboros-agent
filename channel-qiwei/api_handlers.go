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

	if isMediaMessageType(msg.MessageType) {
		method, params, err = a.resolveMediaSendParams(r.Context(), rt, msg.MessageType, toID, msg.Content, msg.ChannelMeta)
	} else {
		method, params, err = toQiweiMessageRequest(msg, toID)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	res, err := rt.client.doAPIRaw(r.Context(), method, params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
		return
	}

	var data any
	if len(res.Data) > 0 {
		_ = unmarshalSafe(res.Data, &data)
	}
	if errMsg := checkSendSuccess(data); errMsg != "" {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: errMsg, Data: data})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: data})
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
		"code":   res.Code,
		"msg":    res.Msg,
		"data":   data,
		"method": method,
		"accountId": rt.AccountID(),
	}})
}
