package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

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
	toID := msg.ChannelConversationID
	if toID == "" {
		toID = msg.ChannelUserID
	}
	if toID == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "channelConversationId or channelUserId is required"})
		return
	}

	method, params, err := toQiweiMessageRequest(msg, toID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	res, err := a.client.doAPIRaw(r.Context(), method, params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
		return
	}

	var data any
	if len(res.Data) > 0 {
		_ = unmarshalSafe(res.Data, &data)
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: data})
}

func toQiweiMessageRequest(msg outgoingMessage, toID string) (string, map[string]any, error) {
	meta := msg.ChannelMeta
	if meta == nil {
		meta = map[string]any{}
	}

	switch msg.MessageType {
	case "text":
		params := map[string]any{"toId": toID, "content": msg.Content}
		if reply := mapValue(meta["reply"]); len(reply) > 0 {
			params["reply"] = reply
		}
		return "/msg/sendText", params, nil
	case "rich_text":
		params := map[string]any{"toId": toID, "content": msg.Content}
		if reply := mapValue(meta["reply"]); len(reply) > 0 {
			params["reply"] = reply
		}
		return "/msg/sendHyperText", params, nil
	case "image":
		return "/msg/sendImage", map[string]any{
			"toId":   toID,
			"imgUrl": msg.Content,
		}, nil
	case "file":
		fileName := "file"
		if v, ok := meta["fileName"].(string); ok && strings.TrimSpace(v) != "" {
			fileName = v
		}
		return "/msg/sendFile", map[string]any{
			"toId":     toID,
			"fileUrl":  msg.Content,
			"fileName": fileName,
		}, nil
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

func (a *app) handleDoAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	var req struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	if strings.TrimSpace(req.Method) == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "method is required"})
		return
	}
	a.handleModuleCall(w, r.Context(), req.Method, req.Params)
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
	actions, ok := a.registry[moduleName]
	if !ok {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "unknown module"})
		return
	}
	method, ok := actions[action]
	if !ok {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "unknown action"})
		return
	}

	var req struct {
		Params map[string]any `json:"params"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}
	a.handleModuleCall(w, r.Context(), method, req.Params)
}

func (a *app) handleModuleCall(w http.ResponseWriter, ctx context.Context, method string, params map[string]any) {
	res, err := a.client.doAPIRaw(ctx, method, params)
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
	}})
}
