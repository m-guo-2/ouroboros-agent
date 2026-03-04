package main

import (
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
}

type incomingMessage struct {
	Channel                 string         `json:"channel"`
	ChannelUserID           string         `json:"channelUserId"`
	ChannelMessageID        string         `json:"channelMessageId"`
	ChannelConversationID   string         `json:"channelConversationId,omitempty"`
	ChannelConversationName string         `json:"channelConversationName,omitempty"`
	ConversationType        string         `json:"conversationType,omitempty"`
	MessageType             string         `json:"messageType"`
	Content                 string         `json:"content"`
	SenderName              string         `json:"senderName,omitempty"`
	Timestamp               int64          `json:"timestamp"`
	ChannelMeta             map[string]any `json:"channelMeta,omitempty"`
	AgentID                 string         `json:"agentId,omitempty"`
}

type qiweiCallbackBody struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg"`
	Data []qiweiCallbackMessage `json:"data"`
}

type qiweiCallbackMessage struct {
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

func parseIntOrDefault(v string, fallback int) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
