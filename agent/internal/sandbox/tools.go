package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agent/internal/engine"
	"agent/internal/types"
)

// RegisterTools registers sandbox tools into the ToolRegistry.
// The tools operate on the sandbox for the current session.
func RegisterTools(registry *engine.ToolRegistry, sb *Sandbox) {
	registry.RegisterBuiltin("execute_command",
		"在沙箱工作区中执行 shell 命令（与宿主机隔离，仅限沙箱目录）。可用于运行脚本、安装依赖、数据处理等。工作目录固定为沙箱根目录。env 参数仅对本次执行有效；需要持久注入请使用 sandbox_set_env。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"command":         map[string]interface{}{"type": "string", "description": "要执行的 shell 命令"},
				"timeout_seconds": map[string]interface{}{"type": "integer", "description": "超时秒数（可选，默认 300）"},
				"env":             map[string]interface{}{"type": "object", "description": "本次命令的一次性环境变量覆盖，键名必须符合 shell 变量名规则"},
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
			env, err := parseEnv(input["env"])
			if err != nil {
				return nil, err
			}

			output, exitCode, err := sb.ExecWithEnv(ctx, command, timeout, env)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"output":    output,
				"exit_code": exitCode,
			}, nil
		},
	)

	registry.RegisterBuiltin("sandbox_set_env",
		"为当前沙箱持久注入或清除环境变量。设置后会影响后续 execute_command 和 skill run_script；不会修改宿主进程环境，也不会写入文件。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"env":   map[string]interface{}{"type": "object", "description": "要设置的环境变量，键名必须符合 shell 变量名规则"},
				"unset": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "要清除的环境变量名"},
			},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			env, err := parseEnv(input["env"])
			if err != nil {
				return nil, err
			}
			unset, err := parseUnset(input["unset"])
			if err != nil {
				return nil, err
			}
			if len(env) == 0 && len(unset) == 0 {
				return nil, fmt.Errorf("env or unset is required")
			}
			if len(env) > 0 {
				if err := sb.SetEnv(env); err != nil {
					return nil, err
				}
			}
			if len(unset) > 0 {
				if err := sb.UnsetEnv(unset); err != nil {
					return nil, err
				}
			}
			return map[string]interface{}{
				"success": true,
				"set":     len(env),
				"unset":   unset,
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

func parseEnv(raw interface{}) (map[string]string, error) {
	if raw == nil {
		return nil, nil
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("env must be an object")
	}
	env := make(map[string]string, len(obj))
	for k, v := range obj {
		switch typed := v.(type) {
		case string:
			env[k] = typed
		case fmt.Stringer:
			env[k] = typed.String()
		default:
			env[k] = fmt.Sprint(v)
		}
	}
	if err := validateEnvMap(env); err != nil {
		return nil, err
	}
	return env, nil
}

func parseUnset(raw interface{}) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unset must be an array")
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		name, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("unset values must be strings")
		}
		result = append(result, name)
	}
	return result, nil
}
