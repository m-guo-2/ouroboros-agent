package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"

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

// RegisterSkillInternalTools registers the skill system's internal tools
// (load_skill, load_skill_reference, run_script) into the registry.
func (r *ToolRegistry) RegisterSkillInternalTools(internalHandlers map[string]types.ToolExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, handler := range internalHandlers {
		r.tools[name] = types.RegisteredTool{
			Definition: types.ToolDefinition{
				Name:        name,
				Description: skillToolDescriptions[name],
				InputSchema: skillToolSchemas[name],
			},
			Execute:    handler,
			Source:     "builtin",
			SourceName: "skill-system",
		}
	}
}

var skillToolDescriptions = map[string]string{
	"load_skill":           "加载指定技能的完整文档、脚本列表和参考资料索引。当技能简介不足以完成任务时，使用此工具获取详细说明。",
	"load_skill_reference": "获取指定技能的详细参考文档。当 load_skill 返回的文档不够详细时，根据其 references 列表加载具体的参考文件。",
	"run_script":           "执行指定技能的脚本。skill_id 和 script 为必填参数，args 为传给脚本的命令行参数。async=true 时后台执行并立即返回，完成后系统自动通知。",
}

var skillToolSchemas = map[string]types.JSONSchema{
	"load_skill": {
		Type: "object",
		Properties: map[string]interface{}{
			"skill_id": map[string]interface{}{"type": "string", "description": "要加载的技能 ID"},
		},
		Required: []string{"skill_id"},
	},
	"load_skill_reference": {
		Type: "object",
		Properties: map[string]interface{}{
			"skill_id":  map[string]interface{}{"type": "string", "description": "技能 ID"},
			"reference": map[string]interface{}{"type": "string", "description": "参考文件名（从 load_skill 返回的 references 列表中选择）"},
		},
		Required: []string{"skill_id", "reference"},
	},
	"run_script": {
		Type: "object",
		Properties: map[string]interface{}{
			"skill_id": map[string]interface{}{"type": "string", "description": "技能 ID（从 load_skill 获取）"},
			"script":   map[string]interface{}{"type": "string", "description": "scripts/ 目录下的脚本文件名"},
			"args":     map[string]interface{}{"type": "string", "description": "传给脚本的命令行参数"},
			"async":    map[string]interface{}{"type": "boolean", "description": "是否异步执行。长耗时脚本（如生成PPT）设为 true，立即返回，完成后系统自动通知"},
		},
		Required: []string{"skill_id", "script"},
	},
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
