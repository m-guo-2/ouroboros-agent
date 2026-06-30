package sandbox

import (
	"context"
	"testing"

	"agent/internal/engine"
)

func TestSandboxSetEnvToolPersistsForExecuteCommand(t *testing.T) {
	sb := newTestSandbox(t)
	registry := engine.NewToolRegistry()
	RegisterTools(registry, sb)

	if _, err := registry.Execute(context.Background(), "sandbox_set_env", map[string]interface{}{
		"env": map[string]interface{}{"SANDBOX_TOOL_VALUE": "from-tool"},
	}); err != nil {
		t.Fatalf("sandbox_set_env: %v", err)
	}

	result, err := registry.Execute(context.Background(), "execute_command", map[string]interface{}{
		"command": "printf %s \"$SANDBOX_TOOL_VALUE\"",
	})
	if err != nil {
		t.Fatalf("execute_command: %v", err)
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if got := resultMap["output"]; got != "from-tool" {
		t.Fatalf("expected persisted env output, got %v", got)
	}
}

func TestExecuteCommandEnvToolInputIsOneShot(t *testing.T) {
	sb := newTestSandbox(t)
	registry := engine.NewToolRegistry()
	RegisterTools(registry, sb)

	result, err := registry.Execute(context.Background(), "execute_command", map[string]interface{}{
		"command": "printf %s \"$SANDBOX_ONESHOT_VALUE\"",
		"env":     map[string]interface{}{"SANDBOX_ONESHOT_VALUE": "one-shot"},
	})
	if err != nil {
		t.Fatalf("execute_command with env: %v", err)
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if got := resultMap["output"]; got != "one-shot" {
		t.Fatalf("expected one-shot env output, got %v", got)
	}

	result, err = registry.Execute(context.Background(), "execute_command", map[string]interface{}{
		"command": "printf %s \"${SANDBOX_ONESHOT_VALUE:-missing}\"",
	})
	if err != nil {
		t.Fatalf("execute_command after one-shot env: %v", err)
	}
	resultMap, ok = result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if got := resultMap["output"]; got != "missing" {
		t.Fatalf("expected one-shot env not to persist, got %v", got)
	}
}
