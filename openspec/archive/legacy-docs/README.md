# Legacy Documentation Archive

这些文档是项目早期阶段的设计文档，已被 `openspec/reference/` 中的文档取代。

保留在此仅作为历史参考。**不应作为当前系统的描述依据。**

## 已废弃文档

| 文档 | 原始用途 | 废弃原因 |
|------|---------|---------|
| ARCHITECTURE.md | 系统架构总览 v2.0.0 | 包含大量 TypeScript/SDK/双进程时代的过时内容。精华已提炼到 `reference/architecture.md` 和 `reference/design-principles.md` |
| PRODUCT_REQUIREMENTS.md | PRD v1.2.0 | 描述的 bootstrap/自举架构已演进为 Go 单体，大部分需求已实现 |
| IMPLEMENTATION_PLAN.md | 实施计划 | 描述的 SDK/TypeScript 实施路径已完全废弃 |
| UNIFIED_CHANNEL_ARCHITECTURE.md | 统一渠道设计 | 核心设计已实现并纳入 `reference/architecture.md` |
| AGENT_REFACTOR_DESIGN.md | ReAct 引擎重构设计 | 核心模式（事件驱动、CQRS、强制工具回复）已实现并纳入参考文档 |
| design-message-history.md | 消息历史设计 | 设计已实现，关键约束已纳入 `reference/design-principles.md` |

## 当前文档入口

请查看 [openspec/README.md](../../README.md)
