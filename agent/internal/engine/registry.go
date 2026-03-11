package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"agent/internal/storage"
	"agent/internal/types"

	sharedlogger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

type McpServerConfig struct {
	Name    string
	BaseURL string
	APIKey  string
}

type mcpToolsResponse struct {
	Tools []struct {
		Name        string           `json:"name"`
		Description string           `json:"description"`
		InputSchema types.JSONSchema `json:"inputSchema"`
	} `json:"tools"`
}

func createShellExecutor() types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		cmdVal, ok := input["command"]
		if !ok {
			return nil, fmt.Errorf("shell: missing command in input")
		}
		cmdStr, ok := cmdVal.(string)
		if !ok || cmdStr == "" {
			return nil, fmt.Errorf("shell: command must be non-empty string")
		}

		cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
		out, err := cmd.CombinedOutput()
		text := string(out)
		if err != nil {
			return nil, fmt.Errorf("shell failed: %w\n%s", err, text)
		}

		var jsonResult interface{}
		if err := json.Unmarshal(out, &jsonResult); err == nil {
			return jsonResult, nil
		}
		return text, nil
	}
}

func createSkillHTTPExecutor(executor storage.SkillToolExecutor) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		if executor.URL == "" {
			return nil, fmt.Errorf("HTTP executor missing url")
		}

		method := executor.Method
		if method == "" {
			method = "POST"
		}

		var reqBody io.Reader
		if method != "GET" {
			b, err := json.Marshal(input)
			if err != nil {
				return nil, err
			}
			reqBody = bytes.NewReader(b)
		}

		req, err := http.NewRequestWithContext(ctx, method, executor.URL, reqBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		client := sharedlogger.NewClient("skill-http", 30*time.Second)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBytes, _ := io.ReadAll(resp.Body)
		text := string(respBytes)

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("HTTP tool failed: %d %s", resp.StatusCode, text)
		}

		var jsonResult interface{}
		if err := json.Unmarshal(respBytes, &jsonResult); err == nil {
			return jsonResult, nil
		}
		return text, nil
	}
}

func createMcpToolExecutor(config McpServerConfig, toolName string) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		url := fmt.Sprintf("%s/tools/%s/call", config.BaseURL, toolName)
		
		body := map[string]interface{}{
			"arguments": input,
		}
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if config.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+config.APIKey)
		}

		client := sharedlogger.NewClient("mcp-tool", 60*time.Second)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBytes, _ := io.ReadAll(resp.Body)
		text := string(respBytes)

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("MCP tool %s failed: %d %s", toolName, resp.StatusCode, text)
		}

		var jsonResult map[string]interface{}
		if err := json.Unmarshal(respBytes, &jsonResult); err == nil {
			if content, ok := jsonResult["content"]; ok {
				return content, nil
			}
			if result, ok := jsonResult["result"]; ok {
				return result, nil
			}
			return jsonResult, nil
		}
		return text, nil
	}
}

type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]types.RegisteredTool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]types.RegisteredTool),
	}
}

func (r *ToolRegistry) GetAll() []types.RegisteredTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	tools := make([]types.RegisteredTool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

func (r *ToolRegistry) Get(name string) (types.RegisteredTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	t, ok := r.tools[name]
	return t, ok
}

func (r *ToolRegistry) GetDefinitions() []types.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	defs := make([]types.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition)
	}
	return defs
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, input map[string]interface{}) (interface{}, error) {
	t, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return t.Execute(ctx, input)
}

func (r *ToolRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	_, ok := r.tools[name]
	return ok
}

func (r *ToolRegistry) RegisterBuiltin(name, description string, inputSchema types.JSONSchema, executor types.ToolExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.tools[name] = types.RegisteredTool{
		Definition: types.ToolDefinition{
			Name:        name,
			Description: description,
			InputSchema: inputSchema,
		},
		Execute:    executor,
		Source:     "builtin",
		SourceName: "system",
	}
}

func (r *ToolRegistry) RegisterSkills(skillsCtx *storage.SkillContext, internalHandlers map[string]types.ToolExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()

	re := regexp.MustCompile(`\[Skill: (.+?)\]`)

	for _, toolDef := range skillsCtx.Tools {
		executor, ok := skillsCtx.ToolExecutors[toolDef.Name]
		if !ok {
			continue
		}

		var execute types.ToolExecutor
		if executor.Type == "shell" {
			execute = createShellExecutor()
		} else if executor.Type == "http" {
			execute = createSkillHTTPExecutor(executor)
		} else if executor.Type == "internal" {
			handlerName := executor.Handler
			if handlerName == "" {
				handlerName = toolDef.Name
			}
			handler, ok := internalHandlers[handlerName]
			if !ok {
				// skip missing internal handlers
				continue
			}
			execute = handler
		} else {
			// unsupported executor type
			continue
		}

		sourceName := "unknown"
		matches := re.FindStringSubmatch(toolDef.Description)
		if len(matches) > 1 {
			sourceName = matches[1]
		}

		r.tools[toolDef.Name] = types.RegisteredTool{
			Definition: types.ToolDefinition{
				Name:        toolDef.Name,
				Description: toolDef.Description,
				InputSchema: toolDef.InputSchema,
			},
			Execute:    execute,
			Source:     "skill",
			SourceName: sourceName,
		}
	}
}

func (r *ToolRegistry) RegisterMcpServer(ctx context.Context, config McpServerConfig) int {
	req, err := http.NewRequestWithContext(ctx, "POST", config.BaseURL+"/tools/list", bytes.NewReader([]byte("{}")))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}

	client := sharedlogger.NewClient("mcp-list", 10*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0
	}

	var data mcpToolsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, tool := range data.Tools {
		name := fmt.Sprintf("mcp_%s_%s", config.Name, tool.Name)
		r.tools[name] = types.RegisteredTool{
			Definition: types.ToolDefinition{
				Name:        name,
				Description: fmt.Sprintf("[MCP: %s] %s", config.Name, tool.Description),
				InputSchema: tool.InputSchema,
			},
			Execute:    createMcpToolExecutor(config, tool.Name),
			Source:     "mcp",
			SourceName: config.Name,
		}
	}

	return len(data.Tools)
}

func (r *ToolRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.tools = make(map[string]types.RegisteredTool)
}
