# Moli Agent — OpenSpec 文档中心

所有项目文档的唯一入口。

---

## 参考文档 (reference/)

对齐当前代码实现的系统描述，开发时以此为准。

| 文档 | 说明 |
|------|------|
| [architecture.md](reference/architecture.md) | **系统架构** — 部署架构、数据流、数据模型、API、技术栈 |
| [design-principles.md](reference/design-principles.md) | **设计原则** — 10 条核心设计原则，指导所有后续开发决策 |
| [concurrent-message-design.md](reference/concurrent-message-design.md) | **并发消息处理** — 从延迟绑定到 Absorb-Replan 的设计演进与 Trade-off |

---

## 活跃变更 (changes/)

尚未实现的功能提案，按优先级排列：

| 变更 | 描述 | 状态 |
|------|------|------|
| [add-admin-authentication](changes/add-admin-authentication/) | Admin 管理界面登录认证 | 待实现 |
| [group-prompt-override](changes/group-prompt-override/) | 按群组（session_key）覆盖 system_prompt 和 skills | 待实现 |
| [browser-sandbox-subagent](changes/browser-sandbox-subagent/) | 浏览器沙盒子 Agent（snapshot→ref→act 模型） | 待实现 |

---

## 决策记录 (decisions/)

63 条 ADR（Architecture Decision Records），记录项目演进中的每个重要决策。

详见 [decisions/README.md](decisions/README.md)

### 关键 ADR 速查

| ADR | 主题 | 影响 |
|-----|------|------|
| [001](decisions/001-multi-agent-architecture.md) | 多 Agent 架构 | 奠基：Agent 是参与者，统一参与者模型 |
| [008](decisions/008-react-agent-engine.md) | 自定义 ReAct 引擎 | 抛弃 SDK 黑盒，手写 while 循环 |
| [011](decisions/011-session-context-and-logging-refactor.md) | 读写分离 | Session Context / Messages / Traces 三分 |
| [021](decisions/021-server-agent-go-monolith.md) | Go 单体合并 | Server + Agent 合并为一个 Go 进程 |
| [031](decisions/031-context-compaction.md) | 上下文压缩 | Token 感知摘要 + recall_context |
| [044](decisions/044-skill-design-philosophy.md) | Skill 设计哲学 | 原子性、两级加载、工具自足 |
| [045](decisions/045-session-message-absorb-replan.md) | Absorb-Replan | Execute → Checkpoint → Absorb 循环 |

---

## 独立规格 (specs/)

不依附于特定变更的可复用规格：

| Spec | 说明 |
|------|------|
| [shared-oss-storage](specs/shared-oss-storage/spec.md) | 共享 OSS 存储接口规格 |

---

## 归档 (archive/)

### 已完成变更 (changes/archive/)

已实现并归档的 OpenSpec 变更，按日期排列：

| 日期 | 变更 | 说明 |
|------|------|------|
| 2026-03-13 | [session-memory-facts](changes/archive/2026-03-13-session-memory-facts/) | 会话事实存储与 save_memory 工具 |
| 2026-03-13 | [fix-skill-loading-mechanism](changes/archive/2026-03-13-fix-skill-loading-mechanism/) | 修复 {{skills}} 占位符泄露 + always/on_demand 模式 |
| 2026-03-13 | [remove-agent-channel-awareness](changes/archive/2026-03-13-remove-agent-channel-awareness/) | 从 LLM 可见面移除渠道信息 |
| 2026-03-12 | [log-sqlite-store](changes/archive/2026-03-12-log-sqlite-store/) | SQLite 日志存储 + Monitor 扁平时间线 |
| 2026-03-11 | [qiwei-message-convergence](changes/archive/2026-03-11-qiwei-message-convergence/) | 企微富消息类型 + 系统事件路由 |
| 2026-03-10 | [agent-proactive-delayed-tasks](changes/archive/2026-03-10-agent-proactive-delayed-tasks/) | 主动延时任务 + 30s 调度器 |
| 2026-03-10 | [add-shared-oss-module](changes/archive/2026-03-10-add-shared-oss-module/) | 共享 OSS 模块 (MinIO) |
| 2026-03-09 | [refactor-qiwei-media-pipeline](changes/archive/2026-03-09-refactor-qiwei-media-pipeline/) | 企微媒体管线重构（结构化附件 + 按需解析）|
| 2026-03-06 | [builtin-tavily-capability](changes/archive/2026-03-06-builtin-tavily-capability/) | Tavily 内置搜索 + web_research 子 Agent |
| 2026-03-05 | [session-message-absorb-replan](changes/archive/2026-03-05-session-message-absorb-replan/) | Execute → Checkpoint → Absorb 循环 |
| 2026-03-05 | [monitor-frontend-optimization](changes/archive/2026-03-05-monitor-frontend-optimization/) | Monitor 三栏布局 + Compaction/Absorb 可视化 |
| 2026-03-05 | [systemprompt-admin-transparent](changes/archive/2026-03-05-systemprompt-admin-transparent/) | SystemPrompt Admin 所见即所得 |

### 历史设计文档 (archive/legacy-docs/)

项目早期的设计文档，已被 `reference/` 取代。仅作历史参考。

详见 [archive/legacy-docs/README.md](archive/legacy-docs/README.md)

---

## 其他文档

| 位置 | 说明 |
|------|------|
| [docs/proxy-setup-guide.md](../docs/proxy-setup-guide.md) | 代理服务器搭建运维指南 |
| [deploy/](../deploy/) | 部署配置（systemd、nginx、env） |
