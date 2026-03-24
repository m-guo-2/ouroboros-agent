package subagent

import (
	"context"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"agent/internal/engine"
	"agent/internal/engine/ostools"
	"agent/internal/logger"
	"agent/internal/types"
)

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
)

type Impact struct {
	Timestamp int64                  `json:"timestamp"`
	Tool      string                 `json:"tool"`
	Summary   string                 `json:"summary"`
	Detail    map[string]interface{} `json:"detail,omitempty"`
}

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
	Impacts       []Impact  `json:"impacts,omitempty"`
}

// CompactFunc compresses messages when context window fills up.
// Returns (compressed messages, was compacted, error).
// If nil, no compression is performed.
type CompactFunc func(ctx context.Context, messages []types.AgentMessage) ([]types.AgentMessage, bool, error)

type StartRequest struct {
	Name            string
	Profile         string
	Task            string
	Context         string
	Model           string
	LLMClient       engine.LLMClient
	Tools           []types.RegisteredTool
	SkillsSnippet   string // Level 1 skill metadata index to inject into subagent prompt
	ParentTraceID   string
	SessionID       string
	Timeout         time.Duration
	OnDone          func(*Job)
	CompactMessages CompactFunc
}

type Manager struct {
	rootDir string

	mu      sync.RWMutex
	jobs    map[string]*Job
	cancels map[string]context.CancelFunc
}

var defaultManager = NewManager(filepath.Join("data", "subagents"))

func DefaultManager() *Manager {
	return defaultManager
}

func NewManager(rootDir string) *Manager {
	return &Manager{
		rootDir: rootDir,
		jobs:    make(map[string]*Job),
		cancels: make(map[string]context.CancelFunc),
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

func (m *Manager) ListBySession(sessionID string) []*Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Job
	for _, job := range m.jobs {
		if job.SessionID == sessionID {
			result = append(result, cloneJob(job))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt > result[j].CreatedAt
	})
	return result
}

func (m *Manager) ReadEvents(jobID string) ([]map[string]interface{}, error) {
	dir := m.jobDetailDir(jobID)
	path := filepath.Join(dir, "events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]interface{}{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []map[string]interface{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev map[string]interface{}
		if json.Unmarshal(line, &ev) == nil {
			events = append(events, ev)
		}
	}
	if events == nil {
		events = []map[string]interface{}{}
	}
	return events, nil
}

func (m *Manager) Cancel(jobID string, reason string) (*Job, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "canceled by main agent"
	}

	m.mu.Lock()
	job, ok := m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("subagent job not found: %s", jobID)
	}
	if job.Status == JobCompleted || job.Status == JobFailed || job.Status == JobCanceled {
		out := cloneJob(job)
		m.mu.Unlock()
		return out, nil
	}
	cancelFn, hasCancel := m.cancels[jobID]
	job.UpdatedAt = time.Now().UnixMilli()
	m.mu.Unlock()

	_ = m.appendEvent(jobID, map[string]interface{}{
		"type":      "cancel_requested",
		"timestamp": time.Now().UnixMilli(),
		"reason":    reason,
	})
	if hasCancel && cancelFn != nil {
		cancelFn()
	}
	if err := m.recordImpact(jobID, Impact{
		Timestamp: time.Now().UnixMilli(),
		Tool:      "cancel_subagent",
		Summary:   "main agent requested cancellation",
		Detail: map[string]interface{}{
			"reason": reason,
		},
	}); err != nil {
		return nil, err
	}
	if out, ok := m.Get(jobID); ok {
		return out, nil
	}
	return nil, fmt.Errorf("subagent job not found: %s", jobID)
}

const (
	subagentMaxIterations = 25
	subagentMaxReentries  = 3
)

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
	m.setCancel(jobID, cancel)
	defer m.clearCancel(jobID)

	ctx := logger.WithTrace(baseCtx, m.subTraceID(jobID), req.SessionID)

	toolDefs := filterToolsByProfile(req.Profile, req.Tools)
	toolDefs = append(toolDefs, ostools.NewRecallContextTool(req.SessionID))
	toolDefs = append(toolDefs, ostools.NewSaveMemoryTool(req.SessionID))
	toolDefs = m.wrapToolsWithImpact(jobID, toolDefs)
	subPrompt := buildSubagentSystemPrompt(req.Profile)
	if req.SkillsSnippet != "" {
		subPrompt += "\n\n" + req.SkillsSnippet
	}
	messages := buildInitialMessages(req.Context, req.Task)

	var loopResult *engine.AgentLoopResult
	var loopErr error

	for reentry := 0; reentry <= subagentMaxReentries; reentry++ {
		loopResult, loopErr = engine.RunAgentLoop(ctx, engine.AgentLoopConfig{
			LLMClient:     req.LLMClient,
			SystemPrompt:  subPrompt,
			Messages:      messages,
			Tools:         toolDefs,
			Model:         req.Model,
			MaxIterations: subagentMaxIterations,
		})

		if loopErr != nil {
			break
		}
		if loopResult == nil {
			break
		}

		messages = loopResult.Messages

		if loopResult.FinalText != "" || ctx.Err() != nil {
			break
		}
		if !loopResult.HitMaxIterations {
			break
		}

		if reentry == subagentMaxReentries {
			logger.Warn(ctx, "子 agent 达到最大 re-entry 次数",
				"jobId", jobID, "reentry", reentry)
			break
		}

		if req.CompactMessages != nil {
			compacted, didCompact, compactErr := req.CompactMessages(ctx, messages)
			if compactErr != nil {
				logger.Warn(ctx, "子 agent 上下文压缩失败",
					"jobId", jobID, "error", compactErr.Error())
			} else if didCompact {
				messages = compacted
				logger.Business(ctx, "子 agent 上下文压缩",
					"traceEvent", "compact", "jobId", jobID)
			}
		}

		logger.Business(ctx, "子 agent re-entry",
			"traceEvent", "subagent_reentry", "jobId", jobID, "reentry", reentry+1)
	}

	if loopErr != nil {
		if errors.Is(loopErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			impacts := m.getImpacts(jobID)
			result := buildCanceledResult(loopResult, impacts)
			_ = os.WriteFile(filepath.Join(m.jobDetailDir(jobID), "result.txt"), []byte(result), 0o644)
			_ = m.appendEvent(jobID, map[string]interface{}{
				"type":      "canceled",
				"timestamp": time.Now().UnixMilli(),
				"resultLen": len(result),
			})
			_ = m.update(jobID, func(j *Job) {
				j.Status = JobCanceled
				j.Result = result
				j.UpdatedAt = time.Now().UnixMilli()
			})
			m.fireDone(jobID, req.OnDone)
			return
		}
		m.markFailed(jobID, loopErr.Error())
		m.fireDone(jobID, req.OnDone)
		return
	}
	if loopResult == nil {
		m.markFailed(jobID, "subagent run returned nil result")
		m.fireDone(jobID, req.OnDone)
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

	m.fireDone(jobID, req.OnDone)
}

func (m *Manager) fireDone(jobID string, onDone func(*Job)) {
	if onDone != nil {
		if job, ok := m.Get(jobID); ok {
			go onDone(job)
		}
	}
}

func (m *Manager) getImpacts(jobID string) []Impact {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[jobID]
	if !ok || len(job.Impacts) == 0 {
		return nil
	}
	out := make([]Impact, len(job.Impacts))
	copy(out, job.Impacts)
	return out
}

func buildInitialMessages(contextHint, task string) []types.AgentMessage {
	var messages []types.AgentMessage
	if strings.TrimSpace(contextHint) != "" {
		messages = append(messages, types.AgentMessage{
			Role: "user",
			Content: []types.ContentBlock{
				{Type: "text", Text: "[背景信息]\n" + contextHint},
			},
		})
	}
	messages = append(messages, taskMessage(task))
	return messages
}

func buildCanceledResult(loopResult *engine.AgentLoopResult, impacts []Impact) string {
	var sb strings.Builder
	sb.WriteString("[子任务被中断]\n")

	if len(impacts) == 0 && loopResult == nil {
		sb.WriteString("尚未开始执行，无可记录操作。")
		return sb.String()
	}

	if len(impacts) > 0 {
		sb.WriteString("已执行操作：\n")
		for i, imp := range impacts {
			if i >= 15 {
				sb.WriteString(fmt.Sprintf("- ... 其余 %d 条操作省略\n", len(impacts)-15))
				break
			}
			sb.WriteString("- " + strings.TrimSpace(imp.Summary) + "\n")
		}
	} else {
		sb.WriteString("已执行操作：无\n")
	}

	if loopResult != nil {
		lastText := extractLastAssistantText(loopResult.Messages)
		if lastText != "" {
			const maxLen = 500
			if len(lastText) > maxLen {
				lastText = lastText[:maxLen] + "..."
			}
			sb.WriteString("\n最后状态：" + lastText)
		}
	}

	return sb.String()
}

func extractLastAssistantText(messages []types.AgentMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}
		for _, b := range messages[i].Content {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				return strings.TrimSpace(b.Text)
			}
		}
	}
	return ""
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

func (m *Manager) markCanceled(jobID, reason string) {
	_ = m.appendEvent(jobID, map[string]interface{}{
		"type":      "canceled",
		"timestamp": time.Now().UnixMilli(),
		"reason":    reason,
	})
	_ = m.update(jobID, func(j *Job) {
		j.Status = JobCanceled
		j.Error = reason
		j.UpdatedAt = time.Now().UnixMilli()
	})
}


func (m *Manager) setCancel(jobID string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancels[jobID] = cancel
}

func (m *Manager) clearCancel(jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, jobID)
}

func (m *Manager) recordImpact(jobID string, impact Impact) error {
	return m.update(jobID, func(j *Job) {
		j.Impacts = append(j.Impacts, impact)
		j.UpdatedAt = time.Now().UnixMilli()
	})
}

func (m *Manager) wrapToolsWithImpact(jobID string, tools []types.RegisteredTool) []types.RegisteredTool {
	out := make([]types.RegisteredTool, 0, len(tools))
	for _, t := range tools {
		tool := t
		orig := tool.Execute
		tool.Execute = func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			result, err := orig(ctx, input)
			impact := Impact{
				Timestamp: time.Now().UnixMilli(),
				Tool:      tool.Definition.Name,
				Summary:   summarizeImpact(tool.Definition.Name, input, result, err),
				Detail: map[string]interface{}{
					"input": input,
				},
			}
			if err == nil {
				impact.Detail["success"] = true
			} else {
				impact.Detail["success"] = false
				impact.Detail["error"] = err.Error()
			}
			if e := m.recordImpact(jobID, impact); e == nil {
				_ = m.appendEvent(jobID, map[string]interface{}{
					"type":      "impact",
					"timestamp": impact.Timestamp,
					"tool":      impact.Tool,
					"summary":   impact.Summary,
				})
			}
			return result, err
		}
		out = append(out, tool)
	}
	return out
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

var subagentToolBlacklist = map[string]bool{
	"run_subagent_async":   true,
	"get_subagent_status":  true,
	"cancel_subagent":      true,
}

func filterToolsByProfile(profile string, tools []types.RegisteredTool) []types.RegisteredTool {
	allowed := allowedToolsForProfile(profile)
	out := make([]types.RegisteredTool, 0, len(tools))
	for _, t := range tools {
		name := t.Definition.Name
		if subagentToolBlacklist[name] {
			continue
		}
		if !allowed[name] {
			continue
		}
		out = append(out, t)
	}
	return out
}

// skillTools are available to all subagent profiles when skills are bound.
var skillTools = []string{"load_skill", "load_skill_reference", "run_script"}

func allowedToolsForProfile(profile string) map[string]bool {
	base := map[string]bool{}
	switch profile {
	case "file_analysis":
		base = map[string]bool{
			"read_file": true,
			"list_dir":  true,
			"grep":      true,
		}
	case "web_research":
		base = map[string]bool{
			"tavily_search":  true,
			"recall_context": true,
		}
	case "data_report":
		base = map[string]bool{
			"render_card": true,
			"read_file":   true,
			"list_dir":    true,
		}
	default: // developer
		base = map[string]bool{
			"shell":      true,
			"read_file":  true,
			"write_file": true,
			"list_dir":   true,
			"grep":       true,
		}
	}
	for _, t := range skillTools {
		base[t] = true
	}
	return base
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
	if in.Impacts != nil {
		out.Impacts = make([]Impact, len(in.Impacts))
		copy(out.Impacts, in.Impacts)
	}
	return &out
}

func normalizeProfile(v string) (string, error) {
	p := strings.TrimSpace(strings.ToLower(v))
	if p == "" {
		return "developer", nil
	}
	switch p {
	case "developer", "file_analysis", "web_research", "data_report":
		return p, nil
	default:
		return "", fmt.Errorf("unsupported subagent profile: %s (allowed: developer, file_analysis, web_research, data_report)", v)
	}
}

func profileDisplayName(profile string) string {
	switch profile {
	case "file_analysis":
		return "file-analysis-subagent"
	case "web_research":
		return "web-research-subagent"
	case "data_report":
		return "data-report-subagent"
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
	case "web_research":
		return `你是 web_research 子代理。你的职责是：
- 使用 Tavily 执行联网检索，收集最新外部资料与来源；
- 优先返回高信噪比结论，而不是堆砌搜索结果；
- 当结果过多时，先缩窄或改写 query 再继续检索；
- 如果 Tavily 不可用、被禁用或未配置，明确说明阻塞原因与建议。

工具使用约束：
- 优先使用 tavily_search 完成外部检索；
- 只在需要补回会话背景时使用 recall_context；
- 不尝试本地文件写入、Shell 执行或其他高权限操作。

输出要求：
- 使用自然语言总结；
- 重点说明“发现了什么、来源大致是什么、还有哪些不确定性”；
- 不直接面向最终用户。`
	case "data_report":
		return `你是 data_report 子代理，专职将结构化数据转化为可视化卡片图片。

职责：
- 分析主 Agent 传来的数据，理解其结构和语义
- 根据数据特征选择最合适的卡片类型
- 调用 render_card 工具生成卡片图片

模板选择策略（优先使用预置模板）：
- 单个或少数核心指标 → kpi 模板
- 多行多列结构化数据 → table 模板
- 多项目运行状态 → status-board 模板
- 有排名/排序的列表 → ranking 模板
- 按时间顺序的事件 → timeline 模板
- 通用 key-value 信息 → summary 模板
- 以上都不适合 → 自由生成 HTML（使用 render_card 的 html 参数）

自由生成 HTML 的规范：
- 必须是完整的 HTML 文档（含 html/head/body 标签）
- 所有样式内联在 style 标签中
- 设计宽度 600px，高度自适应
- 使用系统字体：-apple-system, "Noto Sans SC", "Microsoft YaHei", sans-serif
- 不依赖外部资源（CDN、字体、图片链接等）

输出格式要求：
- 渲染成功时，输出必须包含以下 JSON 结构信息：
  imageUrl: 图片的 OSS 访问地址
  cardType: 使用的模板名称或 "freeform"
  summary: 一句话描述卡片内容
- 渲染失败时（render_card 返回错误），执行 fallback：
  生成一份格式化纯文本版本（用 Markdown 表格或结构化列表），输出中标记：
  fallback: true
  fallbackText: 格式化文本内容
  reason: 失败原因`

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

func summarizeImpact(tool string, input map[string]interface{}, result interface{}, err error) string {
	if err != nil {
		return fmt.Sprintf("%s failed: %s", tool, err.Error())
	}
	switch tool {
	case "write_file":
		if p, _ := input["path"].(string); p != "" {
			return fmt.Sprintf("updated file: %s", p)
		}
		return "updated a file"
	case "shell":
		if cmd, _ := input["command"].(string); cmd != "" {
			return fmt.Sprintf("executed shell command: %s", cmd)
		}
		return "executed a shell command"
	case "read_file":
		if p, _ := input["path"].(string); p != "" {
			return fmt.Sprintf("read file: %s", p)
		}
		return "read a file"
	case "list_dir":
		if p, _ := input["path"].(string); p != "" {
			return fmt.Sprintf("listed directory: %s", p)
		}
		return "listed a directory"
	case "grep":
		if p, _ := input["path"].(string); p != "" {
			return fmt.Sprintf("searched in path: %s", p)
		}
		return "searched files with grep"
	case "tavily_search":
		if q, _ := input["query"].(string); strings.TrimSpace(q) != "" {
			return fmt.Sprintf("researched web results for query: %s", strings.TrimSpace(q))
		}
		return "researched web results with Tavily"
	default:
		if result == nil {
			return fmt.Sprintf("executed tool: %s", tool)
		}
		return fmt.Sprintf("executed tool: %s", tool)
	}
}
