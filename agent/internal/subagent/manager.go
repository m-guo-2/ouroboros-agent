package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"agent/internal/engine"
	"agent/internal/logger"
	"agent/internal/types"
)

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

type Job struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Profile       string    `json:"profile"`
	Task          string    `json:"task"`
	Status        JobStatus `json:"status"`
	ParentTraceID string    `json:"parentTraceId"`
	SubTraceID    string    `json:"subTraceId"`
	SessionID     string    `json:"sessionId"`
	CreatedAt     int64     `json:"createdAt"`
	UpdatedAt     int64     `json:"updatedAt"`
	Result        string    `json:"result,omitempty"`
	Error         string    `json:"error,omitempty"`
	DetailDir     string    `json:"detailDir"`
}

type StartRequest struct {
	Name          string
	Profile       string
	Task          string
	Model         string
	LLMClient     engine.LLMClient
	Messages      []types.AgentMessage
	Tools         []types.RegisteredTool
	ParentTraceID string
	SessionID     string
	Timeout       time.Duration
	OnCompleted   func(*Job)
	OnFailed      func(*Job)
}

type Manager struct {
	rootDir string

	mu   sync.RWMutex
	jobs map[string]*Job
}

var defaultManager = NewManager(filepath.Join("data", "subagents"))

func DefaultManager() *Manager {
	return defaultManager
}

func NewManager(rootDir string) *Manager {
	return &Manager{
		rootDir: rootDir,
		jobs:    make(map[string]*Job),
	}
}

func (m *Manager) Start(req StartRequest) (*Job, error) {
	if strings.TrimSpace(req.Task) == "" {
		return nil, fmt.Errorf("task is required")
	}
	if req.LLMClient == nil {
		return nil, fmt.Errorf("llm client is required")
	}
	if req.Timeout <= 0 {
		req.Timeout = 10 * time.Minute
	}
	profile, err := normalizeProfile(req.Profile)
	if err != nil {
		return nil, err
	}
	req.Profile = profile

	if req.Name == "" {
		req.Name = profileDisplayName(profile)
	}

	now := time.Now().UnixMilli()
	jobID := fmt.Sprintf("subjob-%d", time.Now().UnixNano())
	subTraceID := fmt.Sprintf("subtrace-%d", time.Now().UnixNano())
	detailDir := filepath.Join(m.rootDir, jobID)

	job := &Job{
		ID:            jobID,
		Name:          req.Name,
		Profile:       req.Profile,
		Task:          req.Task,
		Status:        JobQueued,
		ParentTraceID: req.ParentTraceID,
		SubTraceID:    subTraceID,
		SessionID:     req.SessionID,
		CreatedAt:     now,
		UpdatedAt:     now,
		DetailDir:     detailDir,
	}

	m.mu.Lock()
	m.jobs[jobID] = job
	m.mu.Unlock()

	if err := os.MkdirAll(detailDir, 0o755); err != nil {
		return nil, err
	}
	_ = m.appendEvent(jobID, map[string]interface{}{
		"type":      "queued",
		"timestamp": time.Now().UnixMilli(),
		"task":      req.Task,
	})
	_ = m.writeJobMeta(job)

	go m.run(jobID, req)
	return cloneJob(job), nil
}

func (m *Manager) Get(jobID string) (*Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return nil, false
	}
	return cloneJob(job), true
}

func (m *Manager) run(jobID string, req StartRequest) {
	if err := m.update(jobID, func(j *Job) {
		j.Status = JobRunning
		j.UpdatedAt = time.Now().UnixMilli()
	}); err == nil {
		_ = m.appendEvent(jobID, map[string]interface{}{
			"type":      "running",
			"timestamp": time.Now().UnixMilli(),
		})
	}

	baseCtx, cancel := context.WithTimeout(context.Background(), req.Timeout)
	defer cancel()

	ctx := logger.WithTrace(baseCtx, m.subTraceID(jobID), req.SessionID)

	toolDefs := filterToolsByProfile(req.Profile, req.Tools)
	subPrompt := buildSubagentSystemPrompt(req.Profile)
	subMessages := append(copyMessages(req.Messages), taskMessage(req.Task))

	loopResult, err := engine.RunAgentLoop(ctx, engine.AgentLoopConfig{
		LLMClient:     req.LLMClient,
		SystemPrompt:  subPrompt,
		Messages:      subMessages,
		Tools:         toolDefs,
		Model:         req.Model,
		MaxIterations: 15,
	})
	if err != nil {
		m.markFailedWithCallback(jobID, err.Error(), req.OnFailed)
		return
	}
	if loopResult == nil {
		m.markFailedWithCallback(jobID, "subagent run returned nil result", req.OnFailed)
		return
	}

	result := strings.TrimSpace(loopResult.FinalText)
	if result == "" {
		result = "subagent 已完成，但没有产出可用总结。"
	}

	_ = os.WriteFile(filepath.Join(m.jobDetailDir(jobID), "result.txt"), []byte(result), 0o644)
	_ = m.appendEvent(jobID, map[string]interface{}{
		"type":      "completed",
		"timestamp": time.Now().UnixMilli(),
		"resultLen": len(result),
	})

	_ = m.update(jobID, func(j *Job) {
		j.Status = JobCompleted
		j.Result = result
		j.Error = ""
		j.UpdatedAt = time.Now().UnixMilli()
	})

	if req.OnCompleted != nil {
		if job, ok := m.Get(jobID); ok {
			go req.OnCompleted(job)
		}
	}
}

func (m *Manager) markFailed(jobID, errMsg string) {
	_ = m.appendEvent(jobID, map[string]interface{}{
		"type":      "failed",
		"timestamp": time.Now().UnixMilli(),
		"error":     errMsg,
	})
	_ = m.update(jobID, func(j *Job) {
		j.Status = JobFailed
		j.Error = errMsg
		j.UpdatedAt = time.Now().UnixMilli()
	})
}

func (m *Manager) markFailedWithCallback(jobID, errMsg string, cb func(*Job)) {
	m.markFailed(jobID, errMsg)
	if cb != nil {
		if job, ok := m.Get(jobID); ok {
			go cb(job)
		}
	}
}

func (m *Manager) subTraceID(jobID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if j, ok := m.jobs[jobID]; ok {
		return j.SubTraceID
	}
	return ""
}

func (m *Manager) jobDetailDir(jobID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if j, ok := m.jobs[jobID]; ok {
		return j.DetailDir
	}
	return filepath.Join(m.rootDir, jobID)
}

func (m *Manager) update(jobID string, updater func(*Job)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return fmt.Errorf("job not found: %s", jobID)
	}
	updater(job)
	return m.writeJobMeta(job)
}

func (m *Manager) appendEvent(jobID string, event map[string]interface{}) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	path := filepath.Join(m.jobDetailDir(jobID), "events.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func (m *Manager) writeJobMeta(job *Job) error {
	b, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(job.DetailDir, "job.json")
	return os.WriteFile(path, b, 0o644)
}

func buildSubagentSystemPrompt(profile string) string {
	base := defaultPromptByProfile(profile)
	const extra = `
## Subagent 执行约束

- 你是主 Agent 派生的 subagent，不直接面向最终用户。
- 你的最终输出必须是给主 Agent 的自然语言总结，突出关键信息与建议。
- 不要调用 send_channel_message；你只需要完成任务并返回总结。
- 如果遇到阻塞，明确说明阻塞点与下一步建议。`

	return base + "\n\n" + strings.TrimSpace(extra)
}

func taskMessage(task string) types.AgentMessage {
	content := "这是主 Agent 下发给你的子任务：\n\n" + task + "\n\n请完成后给出自然语言总结。"
	return types.AgentMessage{
		Role: "user",
		Content: []types.ContentBlock{
			{Type: "text", Text: content},
		},
	}
}

func filterToolsByProfile(profile string, tools []types.RegisteredTool) []types.RegisteredTool {
	allowed := allowedToolsForProfile(profile)
	out := make([]types.RegisteredTool, 0, len(tools))
	for _, t := range tools {
		name := t.Definition.Name
		if !allowed[name] {
			continue
		}
		out = append(out, t)
	}
	return out
}

func allowedToolsForProfile(profile string) map[string]bool {
	switch profile {
	case "file_analysis":
		return map[string]bool{
			"read_file": true,
			"list_dir":  true,
			"grep":      true,
		}
	default: // developer
		return map[string]bool{
			"shell":      true,
			"read_file":  true,
			"write_file": true,
			"list_dir":   true,
			"grep":       true,
		}
	}
}

func copyMessages(in []types.AgentMessage) []types.AgentMessage {
	out := make([]types.AgentMessage, len(in))
	copy(out, in)
	return out
}

func cloneJob(in *Job) *Job {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func normalizeProfile(v string) (string, error) {
	p := strings.TrimSpace(strings.ToLower(v))
	if p == "" {
		return "developer", nil
	}
	switch p {
	case "developer", "file_analysis":
		return p, nil
	default:
		return "", fmt.Errorf("unsupported subagent profile: %s (allowed: developer, file_analysis)", v)
	}
}

func profileDisplayName(profile string) string {
	switch profile {
	case "file_analysis":
		return "file-analysis-subagent"
	default:
		return "developer-subagent"
	}
}

func defaultPromptByProfile(profile string) string {
	switch profile {
	case "file_analysis":
		return `你是 file_analysis 子代理。你的职责是：
- 阅读、检索、归纳文件内容与代码结构；
- 识别关键文件、关键片段、依赖关系与潜在风险；
- 给出简洁结论与可执行建议。

输出要求：
- 使用自然语言总结；
- 重点说明“发现了什么、依据是什么、建议下一步做什么”；
- 不直接面向最终用户。`
	default:
		return `你是 developer 子代理。你的职责是：
- 围绕给定任务进行实现方案分析与代码修改建议；
- 必要时执行工具调用来验证实现路径；
- 输出可被主代理直接消费的自然语言总结。

输出要求：
- 使用自然语言总结；
- 重点说明“你做了什么、为什么这样做、还有什么风险/待办”；
- 不直接面向最终用户。`
	}
}
