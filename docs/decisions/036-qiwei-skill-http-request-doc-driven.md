# 企微 Skill：http_request 文档驱动接入

- **日期**：2026-03-04
- **类型**：代码变更
- **状态**：已实施

## 背景

`channel-qiwei` 已完成 Go 化并提供全量模块 API，但技能层缺少与之对应的可调用说明。现有数据库中仅有 `feishu-agent`，采用 `http_request + SKILL 文档` 的单工具模式。

为保证企微能力可被 Agent 直接调用，且后续 action 扩展成本最低，需要新增同风格的 `qiwei-agent` skill。

## 决策

沿用飞书 skill 的「单工具 + 文档驱动」模式，在 skills 表新增 `qiwei-agent`：

- tool 使用 `http_request`，executor 为 `shell`
- readme 内定义 `baseUrl`、请求格式、模块 action 列表和 curl 示例
- 类型设为 `action`，并补充企微相关触发词

## 变更内容

- 在运行中的 Agent 数据库（`/api/skills`）创建 `qiwei-agent` 记录
- skill 核心字段：
  - `id/name`: `qiwei-agent`
  - `type`: `action`
  - `enabled`: `true`
  - `tools`: `http_request`（shell 执行 curl）
  - `readme`: 覆盖 `instance/login/user/contact/group/message/cdn/moment/tag/session` 全模块 action
- readme 约定两种调用入口：
  - `POST /api/qiwei/{module}/{action}`（推荐日常）
  - `POST /api/qiwei/do`（method 兜底）

## 考虑过的替代方案

1. 为每个 action 建独立 tool
   - 缺点：维护成本高，新增 action 需要频繁改技能结构。
2. 仅提供通用 method，不维护 action 文档
   - 缺点：可发现性差，模型调用稳定性不如模块 action 明确。

## 影响

- Agent 现可按 `qiwei-agent` 文档直接构造 curl 调用企微服务。
- 后续新增企微 action 时，主要更新 skill readme 即可，无需调整执行器框架。
- 与 `feishu-agent` 形成一致的渠道技能设计，降低跨渠道维护心智负担。
