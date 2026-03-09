package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
)

type app struct {
	cfg       Config
	feishu    *feishuClient
	http      *http.Client
	userCache *ttlCache
	chatCache *ttlCache

	eventDispatcher  *dispatcher.EventDispatcher
	eventHTTPHandler http.HandlerFunc
}

func newApp(cfg Config) *app {
	return &app{
		cfg:    cfg,
		feishu: newFeishuClient(cfg),
		http: &http.Client{
			Timeout: 25 * time.Second,
		},
		userCache: newTTLCache(10 * time.Minute),
		chatCache: newTTLCache(10 * time.Minute),
	}
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/api/health", a.handleHealth)
	mux.HandleFunc("/webhook/event", a.handleWebhookEvent)

	// REST routes
	mux.HandleFunc("/api/feishu/send", a.handleSend)
	mux.HandleFunc("/api/feishu/action", a.handleAction)
	mux.HandleFunc("/api/feishu/action/list", a.handleActionList)
	mux.HandleFunc("/api/feishu/message/", a.handleMessageRoutes)
	mux.HandleFunc("/api/feishu/message/chat", a.handleCreateChat)
	mux.HandleFunc("/api/feishu/meeting/reserve", a.handleReserveMeeting)
	mux.HandleFunc("/api/feishu/meeting/", a.handleMeetingRoutes)
	mux.HandleFunc("/api/feishu/document/wiki/spaces", a.handleGetWikiSpaces)
	mux.HandleFunc("/api/feishu/document/wiki/node", a.handleCreateWikiNode)
	mux.HandleFunc("/api/feishu/document/wiki/", a.handleWikiRoutes)
	mux.HandleFunc("/api/feishu/document/drive/files", a.handleDriveFiles)
	mux.HandleFunc("/api/feishu/document/drive/folder", a.handleCreateFolder)
	mux.HandleFunc("/api/feishu/document/", a.handleDocumentRoutes)
	mux.HandleFunc("/api/feishu/document", a.handleCreateDocument)

	return withJSONMiddleware(mux)
}

func withJSONMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"service":   "channel-feishu",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (a *app) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	var raw map[string]any
	if err := decodeJSON(r.Body, &raw); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
		return
	}

	// legacy format: content is string
	if _, isLegacy := raw["content"].(string); isLegacy {
		if err := a.sendLegacy(r.Context(), raw); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{Success: true})
		return
	}

	var req sendRequest
	if err := mapToStruct(raw, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid request"})
		return
	}
	if req.ReceiveID == "" || req.Content == nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "receiveId and content are required"})
		return
	}
	if req.ReceiveIDType == "" {
		req.ReceiveIDType = "chat_id"
	}

	content, msgType, err := buildMessageContent(req.Content, req.Mentions)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}

	if req.ReplyToMessageID != "" {
		if err := a.replyMessage(r.Context(), req.ReplyToMessageID, msgType, content); err != nil {
			// fallback to normal send
			if err2 := a.sendMessage(r.Context(), req.ReceiveIDType, req.ReceiveID, msgType, content); err2 != nil {
				writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err2.Error()})
				return
			}
		}
	} else {
		if err := a.sendMessage(r.Context(), req.ReceiveIDType, req.ReceiveID, msgType, content); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true})
}

func (a *app) sendLegacy(ctx context.Context, raw map[string]any) error {
	msg := outgoingMessage{}
	if err := mapToStruct(raw, &msg); err != nil {
		return err
	}
	if msg.Content == "" {
		return errors.New("content is required")
	}

	content, msgType := normalizeLongText(msg.Content)
	if msg.MessageType != "text" {
		content = `{"text":` + quote(msg.Content) + `}`
		msgType = "text"
	}

	if msg.ReplyToChannelMessageID != "" {
		if err := a.replyMessage(ctx, msg.ReplyToChannelMessageID, msgType, content); err == nil {
			return nil
		}
	}

	if msg.ChannelConversationID != "" {
		return a.sendMessage(ctx, "chat_id", msg.ChannelConversationID, msgType, content)
	}
	return a.sendMessage(ctx, "open_id", msg.ChannelUserID, msgType, content)
}

func (a *app) sendMessage(ctx context.Context, receiveIDType, receiveID, msgType, content string) error {
	q := url.Values{}
	q.Set("receive_id_type", receiveIDType)
	_, err := a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/im/v1/messages", q, map[string]any{
		"receive_id": receiveID,
		"msg_type":   msgType,
		"content":    content,
	})
	return err
}

func (a *app) replyMessage(ctx context.Context, messageID, msgType, content string) error {
	_, err := a.feishu.doJSON(ctx, http.MethodPost, "/open-apis/im/v1/messages/"+messageID+"/reply", nil, map[string]any{
		"msg_type": msgType,
		"content":  content,
	})
	return err
}

func decodeJSON(body io.Reader, out any) error {
	dec := json.NewDecoder(body)
	dec.UseNumber()
	return dec.Decode(out)
}

func mapToStruct(in map[string]any, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func buildMessageContent(content map[string]any, mentions []string) (string, string, error) {
	t := stringValue(content["type"])
	switch t {
	case "text":
		text := stringValue(content["text"])
		if len(mentions) > 0 {
			parts := make([]string, 0, len(mentions))
			for _, uid := range mentions {
				parts = append(parts, fmt.Sprintf(`<at user_id="%s"></at>`, uid))
			}
			text = strings.Join(parts, " ") + " " + text
		}
		c, mt := normalizeLongText(text)
		return c, mt, nil
	case "rich_text":
		title := stringValue(content["title"])
		rows, _ := content["content"].([]any)
		if len(mentions) > 0 {
			atRows := make([]map[string]string, 0, len(mentions))
			for _, uid := range mentions {
				atRows = append(atRows, map[string]string{"tag": "at", "user_id": uid})
			}
			if len(rows) > 0 {
				first, _ := rows[0].([]any)
				merged := make([]any, 0, len(atRows)+len(first))
				for _, e := range atRows {
					merged = append(merged, e)
				}
				merged = append(merged, first...)
				rows[0] = merged
			} else {
				line := make([]any, 0, len(atRows))
				for _, e := range atRows {
					line = append(line, e)
				}
				rows = append(rows, line)
			}
		}
		raw, _ := json.Marshal(map[string]any{
			"zh_cn": map[string]any{
				"title":   title,
				"content": rows,
			},
		})
		return string(raw), "post", nil
	case "card":
		if templateID := stringValue(content["templateId"]); templateID != "" {
			raw, _ := json.Marshal(map[string]any{
				"type": "template",
				"data": map[string]any{
					"template_id":       templateID,
					"template_variable": mapStringAny(content["templateVariable"]),
				},
			})
			return string(raw), "interactive", nil
		}
		card := content["cardContent"]
		if card == nil {
			return "", "", errors.New("card message requires templateId or cardContent")
		}
		raw, _ := json.Marshal(card)
		return string(raw), "interactive", nil
	case "image":
		raw, _ := json.Marshal(map[string]any{"image_key": stringValue(content["imageKey"])})
		return string(raw), "image", nil
	case "file":
		raw, _ := json.Marshal(map[string]any{"file_key": stringValue(content["fileKey"])})
		return string(raw), "file", nil
	case "audio":
		raw, _ := json.Marshal(map[string]any{"file_key": stringValue(content["fileKey"])})
		return string(raw), "audio", nil
	case "video":
		raw, _ := json.Marshal(map[string]any{
			"file_key":  stringValue(content["fileKey"]),
			"image_key": stringValue(content["imageKey"]),
		})
		return string(raw), "media", nil
	default:
		return "", "", fmt.Errorf("unsupported content type: %s", t)
	}
}
