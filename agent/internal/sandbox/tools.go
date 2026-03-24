package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agent/internal/engine"
	"agent/internal/types"
)

// RegisterTools registers the 4 sandbox tools into the ToolRegistry.
// The tools operate on the sandbox for the current session.
func RegisterTools(registry *engine.ToolRegistry, sb *Sandbox) {
	registry.RegisterBuiltin("execute_command",
		"在沙箱工作区中执行 shell 命令（与宿主机隔离，仅限沙箱目录）。可用于运行脚本、安装依赖、数据处理等。工作目录固定为沙箱根目录。环境变量为沙箱创建时的快照。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"command":         map[string]interface{}{"type": "string", "description": "要执行的 shell 命令"},
				"timeout_seconds": map[string]interface{}{"type": "integer", "description": "超时秒数（可选，默认 300）"},
			},
			Required: []string{"command"},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			command, _ := input["command"].(string)
			command = strings.TrimSpace(command)
			if command == "" {
				return nil, fmt.Errorf("command is required")
			}

			timeout := defaultExecTimeout
			if t, ok := input["timeout_seconds"].(float64); ok && t > 0 {
				timeout = time.Duration(int(t)) * time.Second
			}

			output, exitCode, err := sb.Exec(ctx, command, timeout)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"output":    output,
				"exit_code": exitCode,
			}, nil
		},
	)

	registry.RegisterBuiltin("sandbox_write_file",
		"向沙箱工作区写入文件。自动创建父目录。路径相对于沙箱根目录。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"path":    map[string]interface{}{"type": "string", "description": "文件路径（相对于沙箱根目录）"},
				"content": map[string]interface{}{"type": "string", "description": "文件内容"},
			},
			Required: []string{"path", "content"},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			path, _ := input["path"].(string)
			content, _ := input["content"].(string)
			if strings.TrimSpace(path) == "" {
				return nil, fmt.Errorf("path is required")
			}
			if err := sb.WriteFile(path, content); err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
				"path":    path,
			}, nil
		},
	)

	registry.RegisterBuiltin("sandbox_read_file",
		"读取沙箱工作区中的文件内容。路径相对于沙箱根目录，只能访问沙箱内部文件，无法读取宿主系统文件（如 /etc/environment）。如需查看系统环境变量，请使用 execute_command。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{"type": "string", "description": "文件路径（相对于沙箱根目录）"},
			},
			Required: []string{"path"},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			path, _ := input["path"].(string)
			if strings.TrimSpace(path) == "" {
				return nil, fmt.Errorf("path is required")
			}
			content, err := sb.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"path":    path,
				"content": content,
			}, nil
		},
	)

	registry.RegisterBuiltin("sandbox_list_files",
		"列出沙箱工作区中指定目录的内容。路径相对于沙箱根目录，默认列出根目录。只能列出沙箱内部目录，无法浏览宿主文件系统。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{"type": "string", "description": "目录路径（可选，默认为沙箱根目录）"},
			},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			path, _ := input["path"].(string)
			if path == "" {
				path = "."
			}
			entries, err := sb.ListFiles(path)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"path":    path,
				"entries": entries,
			}, nil
		},
	)
}
