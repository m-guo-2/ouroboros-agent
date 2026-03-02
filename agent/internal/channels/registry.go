// Package channels implements outgoing message dispatch to channel adapters
// (Feishu, QiWei, WebUI). It replaces channel-registry.ts.
package channels

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"agent/internal/storage"
)

// OutgoingMessage mirrors the TS OutgoingMessage type sent to channel adapters.
type OutgoingMessage struct {
	Channel                 string      `json:"channel"`
	ChannelUserID           string      `json:"channelUserId"`
	Content                 string      `json:"content"`
	MessageType             string      `json:"messageType,omitempty"`
	ChannelConversationID   string      `json:"channelConversationId,omitempty"`
	ReplyToChannelMessageID string      `json:"replyToChannelMessageId,omitempty"`
	SessionID               string      `json:"sessionId,omitempty"`
	TraceID                 string      `json:"traceId,omitempty"`
	Mentions                interface{} `json:"mentions,omitempty"`
}

// Adapter is anything that can send a message to an external channel.
type Adapter interface {
	Send(msg OutgoingMessage) error
	HealthCheck() bool
}

var (
	adaptersMu sync.RWMutex
	adapters   = make(map[string]Adapter)

	// WebuiSingleton is exported so the SSE handler in api/ can subscribe.
	WebuiSingleton = &WebuiAdapter{}
)

// Register adds or replaces an adapter for the given channel name.
func Register(channelType string, a Adapter) {
	adaptersMu.Lock()
	defer adaptersMu.Unlock()
	adapters[channelType] = a
}

// GetAdapter returns the adapter registered for channelType, or nil.
func GetAdapter(channelType string) Adapter {
	adaptersMu.RLock()
	defer adaptersMu.RUnlock()
	return adapters[channelType]
}

// RegisteredChannels returns the list of currently registered channel names.
func RegisteredChannels() []string {
	adaptersMu.RLock()
	defer adaptersMu.RUnlock()
	out := make([]string, 0, len(adapters))
	for k := range adapters {
		out = append(out, k)
	}
	return out
}

// SendToChannel stores the outgoing message in the DB, then dispatches via the adapter.
func SendToChannel(msg OutgoingMessage) error {
	adaptersMu.RLock()
	adapter, ok := adapters[msg.Channel]
	adaptersMu.RUnlock()

	if !ok {
		return fmt.Errorf("no adapter registered for channel: %s", msg.Channel)
	}

	if msg.MessageType == "" {
		msg.MessageType = "text"
	}

	msgID := newMsgID()
	// Best-effort DB write — never block the send on a DB error.
	_, _ = storage.DB.Exec(
		`INSERT INTO messages (id, session_id, role, content, message_type, channel, trace_id, initiator, status)
		 VALUES (?, ?, 'assistant', ?, ?, ?, ?, 'agent', 'sending')`,
		msgID, msg.SessionID, msg.Content, msg.MessageType, msg.Channel, msg.TraceID,
	)

	err := adapter.Send(msg)
	if err != nil {
		_, _ = storage.DB.Exec(`UPDATE messages SET status = 'failed' WHERE id = ?`, msgID)
		return err
	}
	_, _ = storage.DB.Exec(`UPDATE messages SET status = 'sent' WHERE id = ?`, msgID)
	return nil
}

func newMsgID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("out-%x%d", b, time.Now().UnixNano()%1e6)
}

// InitBuiltinAdapters registers feishu, qiwei, and webui adapters.
// getSetting is a closure over storage.GetSettingValue so this package stays
// independent of the storage package at init time.
func InitBuiltinAdapters(getSetting func(key string) string) {
	feishuPort := getSetting("general.feishu_port")
	if feishuPort == "" {
		feishuPort = "1999"
	}
	Register("feishu", &HTTPAdapter{
		SendURL:   fmt.Sprintf("http://localhost:%s/api/feishu/send", feishuPort),
		HealthURL: fmt.Sprintf("http://localhost:%s/api/health", feishuPort),
	})

	qiweiPort := getSetting("general.qiwei_port")
	if qiweiPort == "" {
		qiweiPort = "2000"
	}
	Register("qiwei", &HTTPAdapter{
		SendURL:   fmt.Sprintf("http://localhost:%s/api/qiwei/send", qiweiPort),
		HealthURL: fmt.Sprintf("http://localhost:%s/api/health", qiweiPort),
	})

	Register("webui", WebuiSingleton)
}

// ---------------------------------------------------------------------------
// HTTPAdapter — sends messages to a channel adapter via HTTP POST.
// ---------------------------------------------------------------------------

type HTTPAdapter struct {
	SendURL   string
	HealthURL string
	client    *http.Client
}

func (a *HTTPAdapter) httpClient() *http.Client {
	if a.client == nil {
		a.client = &http.Client{Timeout: 15 * time.Second}
	}
	return a.client
}

func (a *HTTPAdapter) Send(msg OutgoingMessage) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	resp, err := a.httpClient().Post(a.SendURL, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("channel adapter returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (a *HTTPAdapter) HealthCheck() bool {
	resp, err := a.httpClient().Get(a.HealthURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// ---------------------------------------------------------------------------
// WebuiAdapter — in-process fan-out for WebUI SSE clients.
// ---------------------------------------------------------------------------

type WebuiAdapter struct {
	mu          sync.RWMutex
	subscribers map[string][]chan OutgoingMessage
}

func (a *WebuiAdapter) Send(msg OutgoingMessage) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.subscribers == nil {
		return nil
	}
	for _, ch := range a.subscribers[msg.ChannelUserID] {
		select {
		case ch <- msg:
		default:
		}
	}
	return nil
}

func (a *WebuiAdapter) HealthCheck() bool { return true }

// Subscribe returns a channel that receives messages for userID.
// The caller is responsible for calling Unsubscribe when done.
func (a *WebuiAdapter) Subscribe(userID string) chan OutgoingMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.subscribers == nil {
		a.subscribers = make(map[string][]chan OutgoingMessage)
	}
	ch := make(chan OutgoingMessage, 32)
	a.subscribers[userID] = append(a.subscribers[userID], ch)
	return ch
}

// Unsubscribe removes a previously subscribed channel.
func (a *WebuiAdapter) Unsubscribe(userID string, ch chan OutgoingMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.subscribers == nil {
		return
	}
	list := a.subscribers[userID]
	for i, c := range list {
		if c == ch {
			a.subscribers[userID] = append(list[:i], list[i+1:]...)
			close(ch)
			return
		}
	}
}
