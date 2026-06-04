package skillexec

import (
	"context"
	"time"
)

// ScriptExecutor executes skill scripts.
// Current implementation: LocalExecutor (sh -c).
// Future: SandboxExecutor (Daytona or similar isolated runtime).
type ScriptExecutor interface {
	Execute(ctx context.Context, req ScriptRequest) (*ScriptResult, error)
}

// ScriptRequest describes a script execution request.
type ScriptRequest struct {
	BasePath string            // local disk path to the skill directory
	Script   string            // script filename within scripts/ subdirectory
	Args     string            // command-line arguments string
	Env      map[string]string // environment variables for the script process; when set, replaces the default inherited env
	Timeout  time.Duration     // optional execution timeout; defaults to LocalExecutor's default
}

// ScriptResult holds the output of a script execution.
type ScriptResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}
