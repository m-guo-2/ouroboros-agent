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
| [026](./026-subagent-cancel-and-impact-feedback.md) | Subagent 取消与影响反馈 | 架构决策 / 代码变更 | 2026-02-28 |
| [027](./027-feishu-action-registry-refactor.md) | 飞书独立服务 Action 路由重构 | 代码变更 | 2026-03-02 |
| [028](./028-feishu-service-go-migration.md) | 飞书独立服务 Go 迁移 | 架构决策 / 代码重构 | 2026-03-02 |
| [029](./029-monitor-user-input-model-output-json-pretty.md) | Monitor 补齐用户输入与模型输出主视图 | 代码变更 | 2026-03-02 |
| [030](./030-skill-detail-manifest-fallback-guard.md) | SkillDetail 缺失 manifest 的兜底防崩 | 代码变更 | 2026-03-02 |
| [031](./031-context-compaction.md) | 上下文压缩（Context Compaction） | 架构决策 / 代码变更 | 2026-03-03 |
| [032](./032-redis-message-queue-multi-instance.md) | Redis 消息队列 + HRW 多实例调度 | 架构决策 | 2026-03-04 |
| [033](./033-qiwei-go-migration-and-full-suite-adapter.md) | 企微独立服务 Go 全量迁移与模块化 API 适配 | 架构决策 / 代码重构 | 2026-03-04 |
| [034](./034-runtime-secret-redaction-and-db-ignore.md) | 运行时密钥脱敏与 SQLite 落库防泄漏 | 代码变更 | 2026-03-04 |
| [035](./035-agent-create-user-id-not-null-compat.md) | Agent 创建接口补齐 user_id 兼容修复 | 代码变更 | 2026-03-04 |
| [036](./036-qiwei-skill-http-request-doc-driven.md) | 企微 Skill：http_request 文档驱动接入 | 代码变更 | 2026-03-04 |
| [037](./037-qiwei-callback-receive-log.md) | 企微回调接收日志补充 | 代码变更 | 2026-03-04 |
| [038](./038-vscode-debug-config-for-qiwei.md) | VSCode 调试配置：channel-qiwei | 代码变更 | 2026-03-04 |
| [037](./037-model-discovery-use-official-provider-endpoints.md) | 模型发现固定走官方 Provider API | 代码变更 | 2026-03-04 |
| [039](./039-qiwei-callback-payload-compatibility.md) | 企微回调载荷兼容解析与 ACK 策略修正 | 代码变更 | 2026-03-04 |
| [040](./040-qiwei-callback-async-context-fix.md) | 企微回调异步处理 context canceled 修复 | Bug 修复 | 2026-03-04 |
| [041](./041-qiwei-official-doc-method-alignment.md) | 企微官方文档 Method 全量对齐 | 代码变更 | 2026-03-04 |
| [042](./042-admin-textarea-auto-resize.md) | Admin 文本编辑区自动扩高 | 代码变更 | 2026-03-04 |
| [043](./043-wecom-skill-progressive-loading.md) | 企微 Skill 渐进式加载与工具合并 | 架构决策 / 代码变更 | 2026-03-04 |
| [044](./044-skill-design-philosophy.md) | Skill 设计哲学与核心理念 | 架构设计 / 设计规范 | 2026-03-04 |
| [045](./045-session-message-absorb-replan.md) | Session 消息吸纳与重新规划 | 架构决策 / 代码变更 | 2026-03-05 |
| [046](./046-monitor-three-panel-redesign.md) | Monitor 可观测性前端三栏布局重设计 | 架构决策 / 代码变更 | 2026-03-05 |
| [047](./047-systemprompt-admin-transparent.md) | SystemPrompt 透明化：Admin 写什么就是什么 | 架构决策 / 代码变更 | 2026-03-05 |
| [048](./048-direct-push-main-by-explicit-request.md) | Main 分支显式直推放开 | 讨论结论 | 2026-03-06 |
