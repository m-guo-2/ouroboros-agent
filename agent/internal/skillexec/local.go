package skillexec

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultTimeout = 300 * time.Second

// LocalExecutor runs scripts via sh on the host machine.
type LocalExecutor struct{}

func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{}
}

func (e *LocalExecutor) Execute(ctx context.Context, req ScriptRequest) (*ScriptResult, error) {
	scriptPath := filepath.Join(req.BasePath, "scripts", req.Script)

	cmdStr := scriptPath
	if req.Args != "" {
		cmdStr = scriptPath + " " + req.Args
	}

	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = req.BasePath

	if len(req.Env) > 0 {
		cmd.Env = cmd.Environ()
		for k, v := range req.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("execute script %s: %w", req.Script, err)
		}
	}

	return &ScriptResult{
		Output:   output,
		ExitCode: exitCode,
	}, nil
}
