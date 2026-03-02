// Package ostools provides built-in OS interaction tools for the agent,
// allowing it to run shell commands, read/write files, and navigate the
// filesystem — similar to how a person uses a terminal.
package ostools

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"agent/internal/engine"
	"agent/internal/types"
)

const (
	maxStdoutBytes = 64 * 1024 // 64 KB per command output
	maxStderrBytes = 16 * 1024 // 16 KB
	cwdMarkerPrefix = "__AGNT_CWD_"
	maxScanToken    = 4 * 1024 * 1024 // 4 MB single line limit for grep/read
)

// limitWriter wraps a writer with a hard byte cap; excess bytes are silently dropped.
type limitWriter struct {
	w   io.Writer
	n   int64
	cap int64
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n >= l.cap {
		return len(p), nil
	}
	if remaining := l.cap - l.n; int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := l.w.Write(p)
	l.n += int64(n)
	return n, err
}

// ShellSession tracks the current working directory across tool calls within a
// session. Because each exec.Command is a new process, we inject a CWD-capture
// snippet into every script and update the tracked path on each run.
type ShellSession struct {
	mu  sync.Mutex
	cwd string
}

// NewShellSession creates a session rooted at workDir.
func NewShellSession(workDir string) *ShellSession {
	if workDir == "" {
		workDir = os.TempDir()
	}
	// Pre-create the directory so the agent has somewhere to work.
	_ = os.MkdirAll(workDir, 0o755)
	return &ShellSession{cwd: workDir}
}

// CWD returns the current tracked working directory (thread-safe).
func (s *ShellSession) CWD() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cwd
}

// ShellResult is the structured return value of a shell execution.
type ShellResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	CWD      string `json:"cwd"`
}

// Run executes command in the tracked CWD and updates it for subsequent calls.
// Wrapping strategy:
//  1. `cd` to the tracked CWD first.
//  2. Run the user command inside `{ }` grouping so that any `cd` inside the
//     command updates the outer shell's directory (unlike a subshell).
//  3. Capture exit code before printing the CWD marker so we can restore it.
func (s *ShellSession) Run(ctx context.Context, command string, timeoutSecs int) (ShellResult, error) {
	if timeoutSecs <= 0 {
		timeoutSecs = 30
	}
	if timeoutSecs > 300 {
		timeoutSecs = 300
	}
	tCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	s.mu.Lock()
	cwd := s.cwd
	s.mu.Unlock()

	_ = os.MkdirAll(cwd, 0o755)

	// The wrapper script:
	//   - Changes to the tracked CWD (falls back to /tmp on failure)
	//   - Runs user command in { } group (keeps cd side-effects)
	//   - Saves exit code
	//   - Prints the CWD marker + current pwd to stdout so we can parse it
	//   - Exits with the captured code
	marker := fmt.Sprintf("%s%d_%d__", cwdMarkerPrefix, time.Now().UnixNano(), rand.Intn(100000))
	script := fmt.Sprintf(
		"cd %s 2>/dev/null || cd /tmp\n{ %s\n}\n__ec__=$?\nprintf '\\n%s\\n'\npwd\nexit $__ec__",
		shellQuote(cwd), command, marker,
	)

	cmd := exec.CommandContext(tCtx, "sh", "-c", script)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &limitWriter{w: &stdoutBuf, cap: maxStdoutBytes}
	cmd.Stderr = &limitWriter{w: &stderrBuf, cap: maxStderrBytes}

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return ShellResult{}, fmt.Errorf("shell exec: %w", runErr)
		}
	}

	stdoutStr := stdoutBuf.String()

	newCwd := cwd
	if parsedStdout, parsedCwd, ok := parseMarkerCWD(stdoutStr, marker); ok {
		stdoutStr = parsedStdout
		newCwd = parsedCwd
	}

	if newCwd != "" {
		s.mu.Lock()
		s.cwd = newCwd
		s.mu.Unlock()
	}

	return ShellResult{
		Stdout:   stdoutStr,
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		CWD:      newCwd,
	}, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func parseMarkerCWD(stdout, marker string) (cleanStdout string, cwd string, ok bool) {
	needle := "\n" + marker + "\n"
	idx := strings.LastIndex(stdout, needle)
	if idx < 0 {
		return stdout, "", false
	}
	rest := stdout[idx+len(needle):]
	nl := strings.Index(rest, "\n")
	if nl < 0 {
		return stdout, "", false
	}
	parsedCwd := strings.TrimSpace(rest[:nl])
	if parsedCwd == "" {
		return stdout, "", false
	}
	return strings.TrimRight(stdout[:idx], "\n"), parsedCwd, true
}

func resolvePath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

// RegisterOSTools adds OS-interaction tools to the registry:
//
//   - shell      – run any shell command; CWD persists across calls
//   - read_file  – read file contents with optional line-range
//   - write_file – create or overwrite a file (append mode supported)
//   - list_dir   – list a directory's entries with metadata
//   - grep       – search files for a pattern, returns structured matches with context
func RegisterOSTools(registry *engine.ToolRegistry, session *ShellSession) {
	registerShell(registry, session)
	registerReadFile(registry, session)
	registerWriteFile(registry, session)
	registerListDir(registry, session)
	registerGrep(registry, session)
}

// ── shell ─────────────────────────────────────────────────────────────────────

func registerShell(registry *engine.ToolRegistry, session *ShellSession) {
	registry.RegisterBuiltin(
		"shell",
		"在 shell 中执行命令。工作目录在会话内持续保持（cd 会生效）。"+
			"返回 stdout、stderr、exit_code 以及执行后的当前目录 cwd。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "要执行的 shell 命令，支持管道、重定向、多行",
				},
				"timeout_seconds": map[string]interface{}{
					"type":        "integer",
					"description": "超时秒数，默认 30，最大 300",
				},
			},
			Required: []string{"command"},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			command, _ := input["command"].(string)
			if strings.TrimSpace(command) == "" {
				return nil, fmt.Errorf("shell: command is required")
			}
			timeoutSecs := 30
			if t, ok := input["timeout_seconds"].(float64); ok && t > 0 {
				timeoutSecs = int(t)
			}
			result, err := session.Run(ctx, command, timeoutSecs)
			if err != nil {
				return nil, err
			}
			return result, nil
		},
	)
}

// ── read_file ─────────────────────────────────────────────────────────────────

func registerReadFile(registry *engine.ToolRegistry, session *ShellSession) {
	registry.RegisterBuiltin(
		"read_file",
		"读取文件内容。支持 start_line / end_line 指定行范围，适合避免一次加载超大文件。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "文件路径（相对路径基于当前工作目录）",
				},
				"start_line": map[string]interface{}{
					"type":        "integer",
					"description": "起始行号（1-based），默认 1",
				},
				"end_line": map[string]interface{}{
					"type":        "integer",
					"description": "结束行号（1-based），默认读完",
				},
			},
			Required: []string{"path"},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			pathStr, _ := input["path"].(string)
			if pathStr == "" {
				return nil, fmt.Errorf("read_file: path is required")
			}
			fullPath := resolvePath(session.CWD(), pathStr)

			data, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, fmt.Errorf("read_file: %w", err)
			}

			lines := strings.Split(string(data), "\n")
			totalLines := len(lines)

			startLine, endLine := 1, totalLines
			if s, ok := input["start_line"].(float64); ok && s >= 1 {
				startLine = int(s)
			}
			if e, ok := input["end_line"].(float64); ok && e >= 1 {
				endLine = int(e)
			}
			// Clamp
			if startLine < 1 {
				startLine = 1
			}
			if startLine > totalLines {
				startLine = totalLines
			}
			if endLine < startLine {
				endLine = startLine
			}
			if endLine > totalLines {
				endLine = totalLines
			}

			content := strings.Join(lines[startLine-1:endLine], "\n")
			truncated := false
			if len(content) > maxStdoutBytes {
				content = content[:maxStdoutBytes] + "\n...[truncated]"
				truncated = true
			}

			return map[string]interface{}{
				"path":        fullPath,
				"content":     content,
				"start_line":  startLine,
				"end_line":    endLine,
				"total_lines": totalLines,
				"truncated":   truncated,
			}, nil
		},
	)
}

// ── write_file ────────────────────────────────────────────────────────────────

func registerWriteFile(registry *engine.ToolRegistry, session *ShellSession) {
	registry.RegisterBuiltin(
		"write_file",
		"写入内容到文件。默认覆盖整个文件；append=true 时追加到末尾。自动创建父目录。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "文件路径（相对路径基于当前工作目录）",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "要写入的内容",
				},
				"append": map[string]interface{}{
					"type":        "boolean",
					"description": "true 时追加到文件末尾，false（默认）时覆盖整个文件",
				},
			},
			Required: []string{"path", "content"},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			pathStr, _ := input["path"].(string)
			if pathStr == "" {
				return nil, fmt.Errorf("write_file: path is required")
			}
			content, ok := input["content"].(string)
			if !ok {
				return nil, fmt.Errorf("write_file: content is required")
			}

			fullPath := resolvePath(session.CWD(), pathStr)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return nil, fmt.Errorf("write_file mkdir: %w", err)
			}

			flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
			if appendMode, _ := input["append"].(bool); appendMode {
				flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
			}

			f, err := os.OpenFile(fullPath, flag, 0o644)
			if err != nil {
				return nil, fmt.Errorf("write_file open: %w", err)
			}
			defer f.Close()

			n, err := f.WriteString(content)
			if err != nil {
				return nil, fmt.Errorf("write_file write: %w", err)
			}
			return map[string]interface{}{
				"success":       true,
				"path":          fullPath,
				"bytes_written": n,
			}, nil
		},
	)
}

// ── list_dir ──────────────────────────────────────────────────────────────────

func registerListDir(registry *engine.ToolRegistry, session *ShellSession) {
	registry.RegisterBuiltin(
		"list_dir",
		"列出目录内容，包含每个条目的类型（file/dir/symlink）、大小和修改时间。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "目录路径，默认为当前工作目录",
				},
			},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			dirPath := session.CWD()
			if p, _ := input["path"].(string); p != "" {
				dirPath = resolvePath(session.CWD(), p)
			}

			entries, err := os.ReadDir(dirPath)
			if err != nil {
				return nil, fmt.Errorf("list_dir: %w", err)
			}

			type fileEntry struct {
				Name     string `json:"name"`
				Type     string `json:"type"`
				Size     int64  `json:"size,omitempty"`
				Modified string `json:"modified"`
			}

			result := make([]fileEntry, 0, len(entries))
			for _, entry := range entries {
				info, err := entry.Info()
				if err != nil {
					continue
				}
				fType := "file"
				if entry.IsDir() {
					fType = "dir"
				} else if info.Mode()&os.ModeSymlink != 0 {
					fType = "symlink"
				}
				fe := fileEntry{
					Name:     entry.Name(),
					Type:     fType,
					Modified: info.ModTime().Format(time.RFC3339),
				}
				if !entry.IsDir() {
					fe.Size = info.Size()
				}
				result = append(result, fe)
			}

			return map[string]interface{}{
				"path":    dirPath,
				"entries": result,
				"count":   len(result),
			}, nil
		},
	)
}

// ── grep ──────────────────────────────────────────────────────────────────────

const defaultMaxMatches = 100

// grepMatch is one matched line with optional surrounding context.
type grepMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Match   string `json:"match"`
	Before  []string `json:"before,omitempty"`
	After   []string `json:"after,omitempty"`
}

// grepFile searches a single file and appends matches to *results.
// It returns true if the match cap was reached.
func grepFile(path string, re *regexp.Regexp, contextLines, maxMatches int, results *[]grepMatch) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	type pendingAfter struct {
		matchIndex int
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxScanToken)

	var prevLines []string
	var pending []pendingAfter
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()

		// Fill "after" context for previous matches.
		if contextLines > 0 && len(pending) > 0 {
			nextPending := pending[:0]
			for _, p := range pending {
				m := &(*results)[p.matchIndex]
				if len(m.After) < contextLines {
					m.After = append(m.After, line)
				}
				if len(m.After) < contextLines {
					nextPending = append(nextPending, p)
				}
			}
			pending = nextPending
		}

		if re.MatchString(line) {
			m := grepMatch{
				File:  path,
				Line:  lineNo,
				Match: line,
			}
			if contextLines > 0 && len(prevLines) > 0 {
				m.Before = append([]string(nil), prevLines...)
			}
			*results = append(*results, m)
			if contextLines > 0 {
				pending = append(pending, pendingAfter{matchIndex: len(*results) - 1})
			}
			if len(*results) >= maxMatches {
				return true, nil
			}
		}

		if contextLines > 0 {
			prevLines = append(prevLines, line)
			if len(prevLines) > contextLines {
				prevLines = prevLines[len(prevLines)-contextLines:]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func registerGrep(registry *engine.ToolRegistry, session *ShellSession) {
	registry.RegisterBuiltin(
		"grep",
		"在文件或目录中搜索匹配模式的行，返回结构化结果（文件路径、行号、匹配内容及上下文）。"+
			"适合在大文件中定位关键内容，比 read_file 更高效。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "搜索模式（正则表达式）",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "要搜索的文件或目录路径，默认为当前工作目录",
				},
				"recursive": map[string]interface{}{
					"type":        "boolean",
					"description": "是否递归搜索子目录，默认 true",
				},
				"case_insensitive": map[string]interface{}{
					"type":        "boolean",
					"description": "是否忽略大小写，默认 false",
				},
				"fixed_strings": map[string]interface{}{
					"type":        "boolean",
					"description": "将 pattern 视为普通字符串而非正则，默认 false",
				},
				"context_lines": map[string]interface{}{
					"type":        "integer",
					"description": "每个匹配项前后各显示的上下文行数（类似 grep -C），默认 0",
				},
				"max_matches": map[string]interface{}{
					"type":        "integer",
					"description": "最多返回的匹配数，默认 100",
				},
				"include": map[string]interface{}{
					"type":        "string",
					"description": "文件名 glob 过滤，例如 *.go、*.ts，仅当 path 为目录时生效",
				},
			},
			Required: []string{"pattern"},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			patternStr, _ := input["pattern"].(string)
			if patternStr == "" {
				return nil, fmt.Errorf("grep: pattern is required")
			}

			// Build regexp
			if fixed, _ := input["fixed_strings"].(bool); fixed {
				patternStr = regexp.QuoteMeta(patternStr)
			}
			flags := ""
			if ci, _ := input["case_insensitive"].(bool); ci {
				flags = "(?i)"
			}
			re, err := regexp.Compile(flags + patternStr)
			if err != nil {
				return nil, fmt.Errorf("grep: invalid pattern: %w", err)
			}

			// Resolve search path
			searchPath := session.CWD()
			if p, _ := input["path"].(string); p != "" {
				searchPath = resolvePath(session.CWD(), p)
			}

			recursive := true
			if r, ok := input["recursive"].(bool); ok {
				recursive = r
			}

			contextLines := 0
			if c, ok := input["context_lines"].(float64); ok && c > 0 {
				contextLines = int(c)
				if contextLines > 10 {
					contextLines = 10
				}
			}

			maxMatches := defaultMaxMatches
			if m, ok := input["max_matches"].(float64); ok && m > 0 {
				maxMatches = int(m)
				if maxMatches > 500 {
					maxMatches = 500
				}
			}

			includeGlob, _ := input["include"].(string)

			var matches []grepMatch
			capped := false

			info, err := os.Stat(searchPath)
			if err != nil {
				return nil, fmt.Errorf("grep: %w", err)
			}

			if !info.IsDir() {
				// Single file
				capped, err = grepFile(searchPath, re, contextLines, maxMatches, &matches)
				if err != nil {
					return nil, fmt.Errorf("grep: %w", err)
				}
			} else {
				// Directory walk
				walkErr := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return nil // skip unreadable entries
					}
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if d.IsDir() {
						if !recursive && path != searchPath {
							return filepath.SkipDir
						}
						return nil
					}
					// Apply include glob filter
					if includeGlob != "" {
						matched, _ := filepath.Match(includeGlob, d.Name())
						if !matched {
							return nil
						}
					}
					var hit bool
					hit, err = grepFile(path, re, contextLines, maxMatches-len(matches), &matches)
					if err != nil {
						return nil // skip files we can't read
					}
					if hit || len(matches) >= maxMatches {
						capped = true
						return filepath.SkipAll
					}
					return nil
				})
				if walkErr != nil && walkErr != context.Canceled {
					return nil, fmt.Errorf("grep walk: %w", walkErr)
				}
			}

			return map[string]interface{}{
				"matches":     matches,
				"total":       len(matches),
				"capped":      capped,
				"pattern":     patternStr,
				"search_path": searchPath,
			}, nil
		},
	)
}
