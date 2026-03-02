# 变更记录索引

本目录记录项目中所有重要的设计决策、代码变更和关键讨论结论。

## 记录总表

| 序号 | 标题 | 类型 | 日期 |
| ------ | ------ | ------ | ------ |
| [001](./001-multi-agent-architecture.md) | 多 Agent 架构 | 架构设计 | 2026-02-07 |
| [002](./002-agent-monitor-observability.md) | Agent Monitor 可观测性界面 | 架构决策 / 代码变更 | 2026-02-08 |
| [003](./003-server-boundary-refactor.md) | Agent-Server 边界重构（SDK-First） | 架构重构 | 2026-02-09 |
| [004](./004-fix-dispatcher-observability-gap.md) | 修复 channel-dispatcher 可观测性断层 | 代码变更 | 2026-02-09 |
| [005](./005-execution-trace-observability.md) | 执行链路追踪：Agent 可观测性重构 | 架构重构 | 2026-02-09 |
| [006](./006-sdk-runner-v2-redesign.md) | SDK Runner v2 重设计 | 架构重构 | 2026-02-10 |
| [007](./007-feishu-query-skill.md) | 飞书信息查询 Skill | 代码变更 | 2026-02-10 |
| [008](./008-react-agent-engine.md) | ReAct Agent 引擎：从 SDK 黑盒到自定义 while 循环 | 架构重构 | 2026-02-24 |
| [009](./009-enhanced-observability-skills-iterations.md) | 增强可观测性：Skills 加载快照 + 迭代轮次追踪 | 代码变更 | 2026-02-24 |
| [010](./010-session-context-continuity.md) | Session 上下文连贯性：完整对话历史加载 | 架构决策 / 代码变更 | 2026-02-24 |
| [012](./012-history-loading-and-user-message-format.md) | 历史消息加载兜底 + 用户消息格式优化 | 代码变更 | 2025-02-25 |
| [013](./013-agent-structured-logging.md) | Agent 结构化日志：双输出 + Context 自动注入 Trace | 架构决策 / 代码变更 | 2026-02-25 |
| [014](./014-anthropic-tool-call-id-sanitization.md) | Anthropic API tool_call_id 校验与消息清洗 | Bug 修复 | 2026-02-25 |
| [015](./015-feishu-skill-http-request-refactor.md) | 飞书 Skill 重构：http_request + SKILL.md | 架构决策 / 代码变更 | 2026-02-25 |
| [016](./016-feishu-channel-message-id-reply.md) | 飞书消息 ID 传递与引用回复 | 代码变更 | 2026-02-26 |
| [017](./017-observability-jsonl-only-no-db.md) | 可观测性简化：JSONL 按需读取，去除落库与实时总线 | 架构重构 | 2026-02-26 |
| [018](./018-observability-iteration-grouped-monitor.md) | 可观测性优化：轻量 llm_call 事件 + Iteration 分组 Monitor | 架构决策 / 代码变更 | 2026-02-26 |
| [019](./019-agent-os-tools-hardening.md) | Agent 默认 OS 工具集强化 | 架构决策 / 代码变更 | 2026-02-26 |
| [020](./020-three-level-logging-llm-io-capture.md) | Agent 三级日志体系 + 完整 LLM I/O 捕获 | 架构重构 | 2026-02-27 |
| [022](./022-monitor-iteration-llm-io-inline.md) | Monitor 按轮次内联展示 LLM I/O | 代码变更 | 2026-02-28 |
| [023](./023-feishu-reply-to-strict-model-controlled.md) | 飞书 reply_to 严格模式（模型显式控制） | 架构决策 / 代码变更 | 2026-02-28 |
| [024](./024-subagent-async-natural-language-aggregation.md) | Subagent 异步工具：自然语言聚合优先 | 架构决策 / 代码变更 | 2026-02-28 |
| [025](./025-subagent-default-profiles-and-system-prompts.md) | Subagent 默认角色与内置系统提示词 | 架构决策 / 代码变更 | 2026-02-28 |
