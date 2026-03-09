package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/core/httpserverext"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

var mentionRegex = regexp.MustCompile(`@_user_\d+`)

var userMessageTypes = map[string]struct{}{
	"text": {}, "image": {}, "audio": {}, "media": {}, "file": {}, "sticker": {},
	"post": {}, "share_chat": {}, "share_user": {}, "location": {}, "merge_forward": {},
}

func (a *app) initEventBridge() {
	d := dispatcher.NewEventDispatcher(a.cfg.VerificationToken, a.cfg.EncryptKey)
	d.OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		return a.onMessageEvent(ctx, event)
	})
	a.eventDispatcher = d
	a.eventHTTPHandler = httpserverext.NewEventHandlerFunc(
		d,
		larkevent.WithLogLevel(parseSDKLogLevel(a.cfg.LogLevel)),
	)
}

func (a *app) startWS(ctx context.Context) {
	wsClient := larkws.NewClient(
		a.cfg.AppID,
		a.cfg.AppSecret,
		larkws.WithEventHandler(a.eventDispatcher),
		larkws.WithLogLevel(parseSDKLogLevel(a.cfg.LogLevel)),
	)

	go func() {
		if err := wsClient.Start(ctx); err != nil {
			fmt.Printf("❌ WebSocket 连接失败: %v\n", err)
			fmt.Println("⚠️  将继续使用 Webhook 模式")
		}
	}()
}

func (a *app) handleWebhookEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	if a.eventHTTPHandler == nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: "event handler not initialized"})
		return
	}
	a.eventHTTPHandler(w, r)
}

func (a *app) onMessageEvent(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	var payload struct {
		Event struct {
			Message struct {
				MessageID   string `json:"message_id"`
				ChatID      string `json:"chat_id"`
				MessageType string `json:"message_type"`
				Content     string `json:"content"`
				CreateTime  string `json:"create_time"`
				Mentions    []struct {
					Key string `json:"key"`
					ID  struct {
						OpenID string `json:"open_id"`
					} `json:"id"`
				} `json:"mentions"`
			} `json:"message"`
			Sender struct {
				SenderID struct {
					OpenID string `json:"open_id"`
				} `json:"sender_id"`
				SenderType string `json:"sender_type"`
				TenantKey  string `json:"tenant_key"`
			} `json:"sender"`
		} `json:"event"`
	}
	raw, _ := json.Marshal(event)
	_ = json.Unmarshal(raw, &payload)

	msg := payload.Event.Message
	sender := payload.Event.Sender
	if msg.MessageID == "" || sender.SenderID.OpenID == "" {
		return nil
	}
	if sender.SenderType != "" && sender.SenderType != "user" {
		return nil
	}
	if _, ok := userMessageTypes[msg.MessageType]; !ok {
		return nil
	}

	chatType := "p2p"
	if strings.HasPrefix(msg.ChatID, "oc_") {
		chatType = "group"
	}

	content := msg.Content
	if msg.MessageType == "text" {
		var textObj map[string]any
		if err := json.Unmarshal([]byte(msg.Content), &textObj); err != nil {
			return nil
		}
		text := mentionRegex.ReplaceAllString(stringValue(textObj["text"]), "")
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		content = text
	}

	senderName := a.resolveUserName(ctx, sender.SenderID.OpenID)
	chatName := ""
	if chatType == "group" {
		chatName = a.resolveChatName(ctx, msg.ChatID)
	}

	if senderName != "" {
		content = senderName + ": " + content
	}

	if a.cfg.AgentEnabled {
		ts := time.Now().UnixMilli()
		if msg.CreateTime != "" {
			if n, err := strconv.ParseInt(msg.CreateTime, 10, 64); err == nil {
				ts = n
			}
		}
		incoming := incomingMessage{
			Channel:               "feishu",
			ChannelUserID:         sender.SenderID.OpenID,
			ChannelMessageID:      msg.MessageID,
			ChannelConversationID: msg.ChatID,
			ChannelConversation:   chatName,
			ConversationType:      chatType,
			MessageType:           msg.MessageType,
			Content:               content,
			SenderName:            senderName,
			Timestamp:             ts,
			ChannelMeta: map[string]any{
				"chatType":   chatType,
				"tenantKey":  sender.TenantKey,
				"senderType": sender.SenderType,
			},
			AgentID: a.cfg.AgentID,
		}
		if err := a.forwardToAgent(ctx, incoming); err != nil {
			_ = a.replyMessage(ctx, msg.MessageID, "text", `{"text":"⚠️ Agent 服务暂不可用，请稍后重试"}`)
		}
		return nil
	}

	if msg.MessageType == "text" {
		echo := `{"text":"🤖 收到你的消息: ` + quote(content) + `"}`
		_ = a.replyMessage(ctx, msg.MessageID, "text", echo)
	}
	return nil
}

func (a *app) forwardToAgent(ctx context.Context, in incomingMessage) error {
	raw, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.AgentServerURL+"/api/channels/incoming", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("agent server error: %d", resp.StatusCode)
	}
	return nil
}

func (a *app) resolveUserName(ctx context.Context, openID string) string {
	if v, ok := a.userCache.Get(openID); ok {
		return v
	}
	q := url.Values{}
	q.Set("user_id_type", "open_id")
	res, err := a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/contact/v3/users/"+openID, q, nil)
	if err != nil {
		return ""
	}
	name := stringValue(mapStringAny(mapStringAny(res["data"])["user"])["name"])
	if name != "" {
		a.userCache.Set(openID, name)
	}
	return name
}

func (a *app) resolveChatName(ctx context.Context, chatID string) string {
	if v, ok := a.chatCache.Get(chatID); ok {
		return v
	}
	res, err := a.feishu.doJSON(ctx, http.MethodGet, "/open-apis/im/v1/chats/"+chatID, nil, nil)
	if err != nil {
		return ""
	}
	name := stringValue(mapStringAny(res["data"])["name"])
	if name != "" {
		a.chatCache.Set(chatID, name)
	}
	return name
}

func parseSDKLogLevel(level string) larkcore.LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return larkcore.LogLevelDebug
	case "warn":
		return larkcore.LogLevelWarn
	case "error":
		return larkcore.LogLevelError
	default:
		return larkcore.LogLevelInfo
	}
}
