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
	switch msg.MessageType {
	case "text", "rich_text":
		return "/msg/sendText", map[string]any{
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
