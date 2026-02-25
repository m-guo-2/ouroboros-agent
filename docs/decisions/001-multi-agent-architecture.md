# 决策记录 001：多 Agent 架构

| 项目 | 内容 |
|------|------|
| 日期 | 2026-02-07 |
| 状态 | 已确认，待实施 |
| 类型 | 架构设计 |
| 决策者 | guomiao |

---

## 背景

系统当前是单 Agent 架构——只有一个 Agent 身份，处理所有用户请求。需要支持多个 Agent 身份，各自有不同的角色、能力和记忆。例如「项目经理 Agent」负责需求管理，「开发工程师 Agent」负责写代码。

## 方案讨论

### 被否决的方案：系统编排多 Agent 协作

最初考虑让 `server` 充当「PM-Agent」来编排其他 Agent。

**否决原因**：
- 把协作逻辑硬编码到系统里，不灵活
- 本质上还是在做工具调用，不是真正的多主体

### 最终方案：Agent 作为一等参与者，协作涌现

**核心决策**：

1. **Agent 是人，不是工具** — 统一 participant 模型，human/agent 平等
2. **系统不做协作编排** — 协作关系写在各自的 systemPrompt + skills 里
3. **Agent 之间互不感知身份** — 在同一个 session 中，每个 Agent 把其他参与者当人类对待
4. **Session 按 Agent 隔离** — 同一个群的消息，每个 Agent 各有独立 session 和上下文
5. **一个 Agent 可绑多个渠道** — Agent 同时出现在飞书、企微、WebUI

## 设计要点

### 系统三层职责分离

| 层级 | 内容 | 谁负责 |
|------|------|--------|
| 消息管道 | 收消息、投递、回复 | 系统 |
| 身份与记忆 | 参与者管理、session、memory | 系统 |
| 智能决策 | 角色认知、协作方式 | Agent 自身（prompt/skills） |

系统只管前两层，不触碰第三层。

### 数据模型变更

- `users` → 扩展为 `participants`（增加 `type: human/agent`）
- 新增 `agent_configs`（systemPrompt, model, skills）
- `agent_sessions` 增加 `agentId`
- `user_memory` 按 `agentId × userId` 隔离

### 消息投递逻辑

群消息到达后，查询该群中绑定的所有 Agent，**分别投递**到各自的 session，各 Agent 独立决策是否回复。

## 实施计划

5 个 Phase，渐进式实施，每个 Phase 可独立交付：

1. Agent Profile CRUD + 持久化
2. 对话隔离（独立 session + systemPrompt）
3. Agent 渠道绑定（一个 Agent 多渠道）
4. 消息多投（群消息 → 多 Agent）
5. Agent Workspace（工作记忆、任务、产出物）

## 影响范围

| 子项目 | 影响 |
|--------|------|
| server | 主要改动——数据库、API、路由逻辑 |
| admin | 新增 Agent 管理 UI |
| agent | 小改——接收不同 Agent 的 systemPrompt |
| channel-feishu | 小改——可能需要支持多 bot app 配置 |
| channel-qiwei | 小改——类似飞书 |
