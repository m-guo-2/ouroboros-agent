package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

type apiResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
}

type outgoingMessage struct {
	Channel                 string         `json:"channel"`
	ChannelUserID           string         `json:"channelUserId"`
	ReplyToChannelMessageID string         `json:"replyToChannelMessageId,omitempty"`
	ChannelConversationID   string         `json:"channelConversationId,omitempty"`
	MessageType             string         `json:"messageType"`
	Content                 string         `json:"content"`
	ChannelMeta             map[string]any `json:"channelMeta,omitempty"`

	// AccountID lets operators target a specific qiwei account when the
	// conversation id has no suffix (e.g. custom tooling calling /api/qiwei/send
	// directly). Optional — default account is used when unset and only one
	// account is registered.
	AccountID string `json:"account_id,omitempty"`
}

type incomingAttachment struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	ResourceURI       string `json:"resourceUri"`
	DisplayName       string `json:"displayName,omitempty"`
	MIMEType          string `json:"mimeType,omitempty"`
	SourceMessageType string `json:"sourceMessageType,omitempty"`
}

// channelIdentitySelf describes the qiwei account's own identity as fetched
// from /user/getProfile. It is surfaced to the agent for display, e.g. so
// the agent can greet on behalf of "小助手 (爱学习有限公司)" without having
// to know anything about guids or routing keys.
type channelIdentitySelf struct {
	UserID   string `json:"userId,omitempty"`
	Name     string `json:"name,omitempty"`
	Alias    string `json:"alias,omitempty"`
	CorpName string `json:"corpName,omitempty"`
}

// channelIdentity carries display-only metadata about the specific qiwei
// account this message belongs to. Agents MUST NOT use any of these fields
// for routing — the routing key remains the opaque ChannelConversationID.
type channelIdentity struct {
	DisplayName string              `json:"displayName,omitempty"`
	Self        channelIdentitySelf `json:"self,omitempty"`
}

type incomingMessage struct {
	Channel                 string               `json:"channel"`
	ChannelAccountID        string               `json:"channelAccountId,omitempty"`
	ChannelAccountShortHash string               `json:"channelAccountShortHash,omitempty"`
	ChannelUserID           string               `json:"channelUserId"`
	ChannelMessageID        string               `json:"channelMessageId"`
	ChannelConversationID   string               `json:"channelConversationId,omitempty"`
	ChannelConversationName string               `json:"channelConversationName,omitempty"`
	ConversationType        string               `json:"conversationType,omitempty"`
	MessageType             string               `json:"messageType"`
	Content                 string               `json:"content"`
	SenderName              string               `json:"senderName,omitempty"`
	Timestamp               int64                `json:"timestamp"`
	ChannelMeta             map[string]any       `json:"channelMeta,omitempty"`
	Attachments             []incomingAttachment `json:"attachments,omitempty"`
	AgentID                 string               `json:"agentId,omitempty"`
	ChannelIdentity         *channelIdentity     `json:"channelIdentity,omitempty"`
}

type qiweiCallbackBody struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg"`
	Data []qiweiCallbackMessage `json:"data"`
}

type qiweiCallbackMessage struct {
	Cmd            int            `json:"cmd"`
	GUID           string         `json:"guid"`
	MsgType        int            `json:"msgType"`
	MsgData        map[string]any `json:"msgData"`
	SenderID       string         `json:"senderId"`
	SenderNickname string         `json:"senderNickname"`
	FromRoomID     string         `json:"fromRoomId"`
	MsgSvrID       string         `json:"msgSvrId"`
	CreateTime     int64          `json:"createTime"`
}

type qiweiDoAPIResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(body io.Reader, out any) error {
	dec := json.NewDecoder(body)
	dec.UseNumber()
	return dec.Decode(out)
}

// unmarshalSafe decodes JSON bytes using UseNumber() so that numeric values
// are preserved as json.Number instead of float64. This prevents silent
// precision loss for large integer IDs (> 2^53) from QiWe.
func unmarshalSafe(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

func parseIntOrDefault(v string, fallback int) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// checkSendSuccess inspects the response data from /msg/send* APIs.
// Returns a non-empty error message when the platform accepted the request
// (code=0) but the message was not actually delivered (isSendSuccess=0).
func checkSendSuccess(data any) string {
	m, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	raw, exists := m["isSendSuccess"]
	if !exists {
		return ""
	}
	if anyToInt64(raw) != 0 {
		return ""
	}
	return "消息未投递成功 (isSendSuccess=0)"
}
