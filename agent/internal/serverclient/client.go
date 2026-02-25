package serverclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"agent/internal/types"
)

type AgentConfig struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	SystemPrompt string `json:"systemPrompt"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	Skills      []string `json:"skills"`
	Channels    []struct {
		ChannelType       string `json:"channelType"`
		ChannelIdentifier string `json:"channelIdentifier"`
	} `json:"channels"`
	IsActive    bool `json:"isActive"`
}

type ProviderCredentials struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey"`
	BaseURL  string `json:"baseUrl"`
}

type MemoryData struct {
	Summary string `json:"summary"`
	Facts   []struct {
		ID        string `json:"id"`
		Category  string `json:"category"`
		Fact      string `json:"fact"`
		CreatedAt string `json:"createdAt,omitempty"`
	} `json:"facts"`
}

type SessionData struct {
	ID                    string        `json:"id"`
	Title                 string        `json:"title"`
	SDKSessionID          string        `json:"sdkSessionId,omitempty"`
	UserID                string        `json:"userId,omitempty"`
	AgentID               string        `json:"agentId,omitempty"`
	SourceChannel         string        `json:"sourceChannel,omitempty"`
	SessionKey            string        `json:"sessionKey,omitempty"`
	ChannelConversationID string        `json:"channelConversationId,omitempty"`
	WorkDir               string        `json:"workDir,omitempty"`
	ExecutionStatus       string        `json:"executionStatus,omitempty"`
	Context               string        `json:"context,omitempty"` // JSON stringified AgentMessage[]
	Messages              []interface{} `json:"messages"`
	CreatedAt             string        `json:"createdAt,omitempty"`
	UpdatedAt             string        `json:"updatedAt,omitempty"`
}

type MessageData struct {
	ID         string        `json:"id"`
	SessionID  string        `json:"sessionId"`
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	MessageType string        `json:"messageType,omitempty"`
	Channel    string        `json:"channel,omitempty"`
	ToolCalls  []interface{} `json:"toolCalls,omitempty"`
	SenderName string        `json:"senderName,omitempty"`
	SenderID   string        `json:"senderId,omitempty"`
	CreatedAt  string        `json:"createdAt,omitempty"`
}

type SkillToolExecutor struct {
	Type    string `json:"type"` // "http" | "script" | "internal"
	URL     string `json:"url,omitempty"`
	Method  string `json:"method,omitempty"`
	Command string `json:"command,omitempty"`
	Handler string `json:"handler,omitempty"`
}

type SkillContext struct {
	SystemPromptAddition string                       `json:"systemPromptAddition"`
	Tools                []types.ToolDefinition       `json:"tools"`
	ToolExecutors        map[string]SkillToolExecutor `json:"toolExecutors"`
	SkillDocs            map[string]string            `json:"skillDocs"`
}

type Client struct {
	baseURL         string
	httpClient      *http.Client
	channelSendToken string
	channelSendSource string
}

func NewClient() *Client {
	baseURL := os.Getenv("AGENT_SERVER_URL")
	if baseURL == "" {
		baseURL = "http://localhost:1997"
	}
	
	sendToken := os.Getenv("AGENT_CHANNEL_SEND_TOKEN")
	if sendToken == "" {
		sendToken = os.Getenv("AGENT_SEND_TOKEN")
	}
	if sendToken == "" {
		sendToken = "local-agent-send-token"
	}

	sendSource := os.Getenv("AGENT_SEND_SOURCE")
	if sendSource == "" {
		sendSource = "agent-sdk-runner"
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		channelSendToken: sendToken,
		channelSendSource: sendSource,
	}
}

func (c *Client) get(ctx context.Context, path string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server API error: %d", resp.StatusCode)
	}
	
	var res struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	if target != nil && len(res.Data) > 0 && string(res.Data) != "null" {
		return json.Unmarshal(res.Data, target)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, body interface{}, headers map[string]string, target interface{}) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server API error: %d - %s", resp.StatusCode, string(respBody))
	}

	if target != nil {
		var res struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return err
		}
		if len(res.Data) > 0 && string(res.Data) != "null" {
			return json.Unmarshal(res.Data, target)
		}
	}
	return nil
}

func (c *Client) put(ctx context.Context, path string, body interface{}) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server API error: %d - %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *Client) GetAgentConfig(ctx context.Context, agentID string) (*AgentConfig, error) {
	var cfg AgentConfig
	err := c.get(ctx, "/api/data/agents/"+agentID, &cfg)
	if err != nil {
		return nil, err
	}
	if cfg.ID == "" {
		return nil, nil // Not found or null
	}
	return &cfg, nil
}

func (c *Client) GetProviderCredentials(ctx context.Context, provider string) (*ProviderCredentials, error) {
	var creds ProviderCredentials
	err := c.get(ctx, "/api/data/provider-credentials/"+provider, &creds)
	if err != nil {
		return nil, err
	}
	if creds.Provider == "" {
		return nil, nil
	}
	return &creds, nil
}

func (c *Client) GetSkillsContext(ctx context.Context, agentID string) (*SkillContext, error) {
	var sc SkillContext
	err := c.get(ctx, "/api/data/agents/"+agentID+"/skills-context", &sc)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func (c *Client) FindSessionByKey(ctx context.Context, agentID, sessionKey string) (*SessionData, error) {
	q := url.Values{}
	q.Add("agentId", agentID)
	q.Add("sessionKey", sessionKey)
	var sd SessionData
	err := c.get(ctx, "/api/data/sessions/by-key?"+q.Encode(), &sd)
	if err != nil || sd.ID == "" {
		return nil, err
	}
	return &sd, nil
}

func (c *Client) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	var sd SessionData
	err := c.get(ctx, "/api/data/sessions/"+sessionID, &sd)
	if err != nil || sd.ID == "" {
		return nil, err
	}
	return &sd, nil
}

func (c *Client) GetSessionMessages(ctx context.Context, sessionID string, limit int) ([]MessageData, error) {
	var msgs []MessageData
	path := fmt.Sprintf("/api/data/sessions/%s/messages?limit=%d", sessionID, limit)
	err := c.get(ctx, path, &msgs)
	return msgs, err
}

func (c *Client) CreateSession(ctx context.Context, session map[string]interface{}) (*SessionData, error) {
	var sd SessionData
	err := c.post(ctx, "/api/data/sessions", session, nil, &sd)
	return &sd, err
}

func (c *Client) UpdateSession(ctx context.Context, sessionID string, updates map[string]interface{}) error {
	return c.put(ctx, "/api/data/sessions/"+sessionID, updates)
}

func (c *Client) SaveMessage(ctx context.Context, msg map[string]interface{}) (*MessageData, error) {
	var md MessageData
	err := c.post(ctx, "/api/data/messages", msg, nil, &md)
	return &md, err
}

func (c *Client) SendToChannel(ctx context.Context, msg map[string]interface{}) error {
	headers := map[string]string{
		"x-agent-send-token": c.channelSendToken,
		"x-agent-source":     c.channelSendSource,
	}
	return c.post(ctx, "/api/data/channels/send", msg, headers, nil)
}

func (c *Client) ReportTraceEventSync(ctx context.Context, event types.TraceEventPayload) error {
	return c.post(ctx, "/api/traces/events", event, nil, nil)
}

func (c *Client) ReportTraceEvent(event types.TraceEventPayload) {
	go func() {
		// Fire and forget
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.ReportTraceEventSync(ctx, event)
	}()
}

func (c *Client) Register(ctx context.Context, id, url, version string) error {
	body := map[string]string{
		"id": id,
		"url": url,
	}
	if version != "" {
		body["version"] = version
	}
	return c.post(ctx, "/api/lifecycle/register", body, nil, nil)
}

func (c *Client) Heartbeat(ctx context.Context, id string) error {
	body := map[string]string{"id": id}
	return c.post(ctx, "/api/lifecycle/heartbeat", body, nil, nil)
}
