package main

type apiResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
	Total   int    `json:"total,omitempty"`
	Actions any    `json:"actions,omitempty"`
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

type sendRequest struct {
	ReceiveID        string         `json:"receiveId"`
	ReceiveIDType    string         `json:"receiveIdType"`
	ReplyToMessageID string         `json:"replyToMessageId"`
	Mentions         []string       `json:"mentions"`
	Content          map[string]any `json:"content"`
}

type incomingMessage struct {
	Channel               string         `json:"channel"`
	ChannelUserID         string         `json:"channelUserId"`
	ChannelMessageID      string         `json:"channelMessageId"`
	ChannelConversationID string         `json:"channelConversationId,omitempty"`
	ChannelConversation   string         `json:"channelConversationName,omitempty"`
	ConversationType      string         `json:"conversationType,omitempty"`
	MessageType           string         `json:"messageType"`
	Content               string         `json:"content"`
	SenderName            string         `json:"senderName,omitempty"`
	Timestamp             int64          `json:"timestamp"`
	ChannelMeta           map[string]any `json:"channelMeta,omitempty"`
	AgentID               string         `json:"agentId,omitempty"`
}
