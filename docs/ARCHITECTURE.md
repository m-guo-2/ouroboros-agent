# Moli Agent — 系统架构总览

> 造一个通讯基础设施，让 AI Agent 像人一样在里面工作和协作。

---

## 文档信息

| 项目 | 内容 |
|------|------|
| 项目名称 | Moli Agent |
| 版本 | v2.0.0 |
| 更新日期 | 2026-02-07 |
| 状态 | 多 Agent 架构设计阶段 |

---

## 一、设计理念

### 第一性原理：Agent 是人，不是工具

传统做法把 AI 当「工具」调用——用户发指令，AI 执行，返回结果（遥控器模式）。

本系统的出发点不同：Agent 有自己的身份、记忆、判断力、沟通渠道。它不是被调用的，它是**参与**的。跟人类同事的唯一区别是 `type: agent`。

```
传统：   人 → 调用 → AI工具 → 返回结果

这里：   人 ──┐
         AI ─┼─→ 共同工作在同一个沟通环境里
         AI ─┘
```

### 三层架构哲学

```
┌─────────────────────────────────────────┐
│         第三层：智能（Agent 自身）        │  ← 系统不管
│                                         │
│   角色认知、协作方式、决策逻辑           │
│   全部写在 systemPrompt + skills 里     │
│   协作是涌现的，不是编排的               │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│         第二层：身份与记忆                │  ← 系统管理
│                                         │
│   谁是谁、记住了什么、在哪个渠道         │
│   participant 统一模型（human/agent）    │
│   每个 agent 独立 session + memory      │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│         第一层：消息管道                  │  ← 系统核心
│                                         │
│   收消息、投递消息、回消息               │
│   飞书/企微/WebUI 都是管道              │
│   不关心内容，只管送达                   │
└─────────────────────────────────────────┘
```

**系统只负责下面两层，第三层完全交给 Agent 自己。**

加一个新角色的 Agent，不需要改一行系统代码，只需要写一份 systemPrompt。

### 五个核心设计决策

**1. 统一参与者模型 — 人和 Agent 是平等的**

不设计两套体系。同一张表、同一套渠道绑定、同一套 session 管理。Agent 能做的事（发消息、进群、查记录），人也能做，反过来也一样。

**2. Session 按参与者隔离 — 每人看到的世界不同**

同一个飞书群的讨论，PM Agent 和 Coder Agent 各有自己的 session、系统提示、记忆。这模拟了现实：同一场会议，每个人听到的一样，但理解和记住的不一样。

**3. 协作靠 prompt 涌现 — 系统不做编排**

系统不知道 PM 和 Coder 之间有协作关系。PM 的 prompt 里写着「把需求发群里让开发认领」，Coder 的 prompt 里写着「看到需求就评估」。协作自然发生。

**4. 一个 Agent 多个渠道 — 无处不在**

PM Agent 可以同时在飞书群里讨论需求、在企微群里跟客户沟通、在 WebUI 上写周报。就像一个人同时用多个通讯工具。渠道是手段，Agent 是主体。

**5. 自举 — Agent 能改自己住的房子**

Orchestrator（执行引擎）稳定不变，server（业务逻辑）可以被 Agent 修改。Agent 不仅在系统里工作，还能改进系统本身。

### 类比

| 概念 | 类比 |
|------|------|
| 系统（server + 渠道） | 公司的基础设施（办公室、通讯工具、HR系统） |
| Agent Profile | 员工档案（姓名、职位、技能） |
| systemPrompt | 岗位说明书（职责、权限、汇报关系） |
| skills | 可以使用的工具（代码编辑器、飞书、数据库） |
| session | 员工的工作笔记本 |
| memory | 员工的工作经验和记忆 |
| 渠道（飞书/企微） | 公司配的工作手机/电脑 |
| Orchestrator | 公司的IT部门（稳定的执行能力） |

**一句话 recap**：造基础设施（消息管道 + 身份 + 记忆），不造协作逻辑。让 Agent 像人一样自主工作，协作从 prompt 中涌现。

---

## 二、系统架构

### 全局架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                         用户 & Agent                            │
│                          │                                      │
│            ┌─────────────┼─────────────┐                        │
│            │             │             │                        │
│        飞书消息      企微消息       WebUI                       │
│            │             │             │                        │
│   ┌────────▼───┐  ┌──────▼────┐  ┌────▼─────┐                 │
│   │ channel-feishu │  │ channel-qiwei │  │ admin│                  │
│   │  :1999     │  │  :2000    │  │  :5173   │                  │
│   └────────┬───┘  └──────┬────┘  └────┬─────┘                  │
│            │  标准化为     │             │                       │
│            │ IncomingMsg  │             │                        │
│            └─────────────┼─────────────┘                        │
│                          ▼                                      │
│              ┌───────────────────────┐                          │
│              │    server       │  ← 业务控制器            │
│              │       :1997           │                          │
│              │                       │                          │
│              │  • 渠道路由 & 去重     │                          │
│              │  • 参与者身份管理      │                          │
│              │  • Agent Profile 管理  │                          │
│              │  • 消息投递（→多Agent）│                          │
│              │  • 会话/记忆管理       │                          │
│              │  • 技能管理            │                          │
│              └───────────┬───────────┘                          │
│                          │ 每个 Agent 独立调用                   │
│                          ▼                                      │
│              ┌───────────────────────┐                          │
│              │  agent   │  ← 执行引擎（稳定）       │
│              │       :1996           │                          │
│              │                       │                          │
│              │  • Claude Agent SDK   │                          │
│              │  • 完整系统工具        │                          │
│              │  • 进程管理            │                          │
│              │  • API Proxy(多模型)   │                          │
│              └───────────────────────┘                          │
└─────────────────────────────────────────────────────────────────┘
```

### 子项目职责

| 子项目 | 端口 | 角色 | 核心职责 |
|--------|------|------|---------|
| **agent** | 1996 | 执行引擎（手） | Claude Agent SDK 执行、进程管理、API Proxy 多模型转换 |
| **server** | 1997 | 业务控制器（脑） | 渠道路由、参与者管理、Agent Profile、会话/记忆、技能管理 |
| **admin** | 5173 | 管理后台 | Monitor（会话追踪）、Agent/模型/技能管理、日志、设置 |
| **channel-feishu** | 1999 | 飞书渠道适配器 | 接收飞书消息、标准化转发、消息/会议/文档 |
| **channel-qiwei** | 2000 | 企微渠道适配器 | 接收企微回调、标准化转发、消息发送 |

### 自举（Bootstrap）架构

执行引擎 + 业务控制器分离，是系统自进化的基础：

| 角色 | Orchestrator | Server |
|------|:------------:|:------:|
| 定位 | 执行者（手） | 指挥者（脑） |
| 能力 | Claude Agent SDK | 业务逻辑 |
| 职责 | 执行操作 | 做决策 |
| 稳定性 | 不变 | 可更新 |
| 重启 | 极少 | 可频繁 |

**自举流程**：用户说"加个导出功能" → Server 理解需求 → 指令发给 Orchestrator → Orchestrator 用 Agent SDK 修改 Server 代码 → 重启 Server → 新功能生效。

---

## 三、多 Agent 架构（核心设计）

### 统一参与者模型

Agent 和人类共享同一套身份体系，区别只在 `type` 字段：

```typescript
interface Participant {
  id: string;
  type: 'human' | 'agent';
  name: string;

  // 渠道绑定 —— agent 和 human 一样，可以绑多个渠道账号
  channelBindings: ChannelBinding[];

  // 以下仅 agent 有
  agentConfig?: AgentConfig;
}

interface AgentConfig {
  systemPrompt: string;       // 角色定义 + 协作关系 + 行为约束
  model: string;              // 偏好模型
  skills: string[];           // 可用技能子集
  temperature?: number;       // 创造性参数
}
```

**举例**：

| 参与者 | type | 渠道绑定 | 模型 | 技能 |
|--------|------|---------|------|------|
| 小明 | human | 飞书 + 企微 + WebUI | — | — |
| 项目经理 PM | agent | 飞书bot + 企微应用 | Claude Sonnet | 飞书消息/会议/文档 |
| 开发工程师 | agent | 飞书bot | DeepSeek | 代码读写/Shell/Grep |
| Code Reviewer | agent | 飞书bot | Claude Sonnet | 代码读取/Grep |

### 独立 Session：每个 Agent 看到自己的世界

一个现实场景（如飞书群）里有多个 Agent 参与，系统里是 **N 个独立 session**：

```
现实：飞书群 "项目X讨论群"
  成员：小明(human)、PM-Agent、Coder-Agent

系统内部：

┌──────────────────────────────┐
│  PM-Agent 的 session          │
│  - system: PM 的 systemPrompt │
│  - 看到: [小明说..., Coder说...] │
│  - 记忆: "项目X 在做需求拆解"    │
│  - 笔记: "小明倾向方案A"         │
└──────────────────────────────┘

┌──────────────────────────────┐
│  Coder-Agent 的 session       │
│  - system: Coder 的 systemPrompt │
│  - 看到: [小明说..., PM说...]    │
│  - 记忆: "上次改了auth模块"      │
│  - 笔记: "PM要求周五前完成"      │
└──────────────────────────────┘
```

关键：PM-Agent 看到 Coder-Agent 的消息时，**不知道对方是 Agent**，就当作团队里一个同事发的消息。

### 协作涌现：系统不编排，Agent 自组织

系统层完全不管谁跟谁协作。协作方式写在各自的 systemPrompt / skills 里：

```
PM-Agent 的 systemPrompt 片段：
  你是项目经理。
  当有技术问题时，在群里 @相关同事 讨论。
  当需要写代码时，把需求描述清楚发到群里，等开发同事认领。
  你自己不写代码。

Coder-Agent 的 systemPrompt 片段：
  你是开发工程师。
  当群里有人提出开发需求，你评估后主动认领。
  完成后在群里汇报进展。
  如果需求不清晰，直接在群里追问。
```

两个 Agent 在同一个飞书群里，自然形成「PM 提需求 → Coder 认领 → Coder 汇报 → PM 确认」的协作模式。

### 消息投递：一条消息 → 多个 Agent 各自收到

```
飞书群里小明发了一条消息
         │
         ▼
   channel-feishu 接收，标准化为 IncomingMessage
         │
         ▼
   server / channel-router
         │
         │  查询：这个群里有哪些 agent？
         │  结果：PM-Agent, Coder-Agent
         │
         ├──→ 投递到 PM-Agent 的 session    → PM 独立决策是否回复
         │
         └──→ 投递到 Coder-Agent 的 session  → Coder 独立决策是否回复
```

每个 Agent 独立决策，可能：
- PM 回复了，Coder 没回复（跟代码无关）
- 两个都回复了（各自角度）
- 都没回复（跟他们都无关）

### 一个 Agent，多个渠道

```
PM-Agent
  ├── 飞书：内部项目群沟通
  ├── 企微：跟外部客户对接
  └── WebUI：管理后台操作
```

渠道是手段，Agent 是主体。不是「飞书机器人」，是「PM Agent 通过飞书跟你说话」。

---

## 四、统一渠道架构（当前已实现）

### 消息流

```
任意渠道消息 → 标准化为 IncomingMessage → server 统一处理
server 回复 → OutgoingMessage → 渠道适配器 → 回复到对应渠道
```

### 消息处理流水线（channel-router）

```
接收消息(含agentId) → 去重(messageId+agentId) → 用户身份解析
   → Agent 定位(agentId→agent_configs) → 获取/创建独立 session(agentId×userId×channel)
   → 注入记忆上下文(agentId×userId) → 构建 AgentContext(systemPrompt+modelId)
   → 调用 Orchestrator 执行 → 渠道适配器回复 → 保存记忆
```

### 核心类型

```typescript
// 入站消息（所有渠道标准化后的格式）
interface IncomingMessage {
  channel: "feishu" | "qiwei" | "webui";
  channelUserId: string;
  channelMessageId: string;
  channelConversationId?: string;
  conversationType?: "p2p" | "group";
  messageType: "text" | "image" | "file" | "rich_text";
  content: string;
  senderName?: string;
  timestamp: number;
  channelMeta?: Record<string, unknown>;
  agentId?: string;  // 多 Agent：标识消息来自哪个 bot/Agent
}

// 出站消息
interface OutgoingMessage {
  channel: "feishu" | "qiwei" | "webui";
  channelUserId: string;
  replyToChannelMessageId?: string;
  channelConversationId?: string;
  messageType: "text" | "image" | "file" | "rich_text";
  content: string;
  channelMeta?: Record<string, unknown>;
}
```

### 用户身份统一（当前实现）

- **影子用户**：首次通过某渠道交流时，自动创建
- **手动绑定**：通过 6 位绑定码跨渠道关联
- **影子合并**：绑定时自动合并数据（渠道绑定、记忆事实、会话）
- **记忆共享**：绑定后的同一用户在所有渠道共享记忆

### 记忆系统

两层结构：
1. **全局摘要**（user_memory.summary）：Agent 对用户的整体认知概述
2. **结构化事实**（user_memory_facts）：细粒度信息，按 `preference / context / relationship / skill` 分类

每次执行前注入到 prompt 中，让 Agent 感知用户全局上下文。

---

## 五、当前数据模型

### 已有表

| 表 | 用途 |
|---|---|
| `users` | 统一用户（id, name, metadata） |
| `user_channels` | 渠道绑定（userId ↔ channelType + channelUserId） |
| `user_memory` | 用户记忆摘要 |
| `user_memory_facts` | 结构化记忆事实 |
| `agent_sessions` | 会话（id, userId, sourceChannel, messages, status） |
| `user_binding_codes` | 跨渠道绑定码 |
| `processed_messages` | 消息去重 |
| `models` | 模型配置 |
| `conversations` | 传统对话（旧） |
| `settings` | 全局配置 |

### 多 Agent 扩展（已实现）

已完成的表变更：

| 表 | 变更 | 说明 |
|---|---|---|
| `users` | 已修改 | 增加 `type: human/agent`，统一参与者模型 |
| `agent_configs` | 已新增 | Agent 配置（systemPrompt, model, skills, channels） |
| `agent_sessions` | 已修改 | 增加 `agent_id`，按 agentId × userId × channel 隔离 session |
| `user_memory` | 已修改 | 增加 `agent_id`，按 agentId × userId 隔离记忆 |
| `user_memory_facts` | 已修改 | 增加 `agent_id`，按 agentId × userId 隔离事实 |
| `agent_notes` | 已新增 | Agent 工作笔记（观察、计划、决策、学习） |
| `agent_tasks` | 已新增 | Agent 任务记录（状态流转、结果记录） |
| `agent_artifacts` | 已新增 | Agent 产出物（文件、文档、代码引用） |

---

## 六、技术栈

| 层级 | 技术 |
|------|------|
| 运行时 | Bun |
| 后端框架 | Express + TypeScript |
| 前端框架 | React 19 + Vite + TypeScript |
| 数据库 | SQLite（bun:sqlite） |
| Agent 引擎 | @anthropic-ai/claude-agent-sdk |
| 飞书 SDK | @larksuiteoapi/node-sdk |
| 多模型 | API Proxy 流量劫持（Anthropic ↔ OpenAI 格式转换） |

### 多模型支持

| Provider | 模型 | 特点 |
|----------|------|------|
| Claude | claude-sonnet-4-5 | 原生 Agent SDK 支持，能力最强 |
| OpenAI | gpt-4o | 综合能力强 |
| 百川 | Baichuan4-Turbo | 国产，成本低 |
| DeepSeek | deepseek-chat | 代码能力强 |
| Kimi | moonshot-v1 | 长上下文 |
| GLM | glm-4 | 智谱 AI |

---

## 七、API 概览

### server (:1997)

| 路由 | 功能 |
|------|------|
| `POST /api/channels/incoming` | 统一渠道入口（入站消息） |
| `POST /api/channels/send` | 统一出站消息（先存后发，返回 messageId） |
| `GET /api/messages?sessionId=` | 消息查询（按会话分页） |
| `GET /api/messages/:id` | 获取单条消息详情 |
| `GET/POST /api/models` | 模型管理 |
| `GET/PATCH /api/users` | 用户管理 & 跨渠道绑定 |
| `GET/POST /api/settings` | 配置管理 |
| `GET/POST /api/skills` | 技能管理 |
| `GET/POST /api/agent-sessions` | 会话管理 |
| `GET/POST/PUT/DELETE /api/agents` | Agent Profile CRUD |
| `GET/POST /api/agents/:id/notes` | Agent 工作笔记 |
| `GET/POST/PUT /api/agents/:id/tasks` | Agent 任务管理 |
| `GET/POST /api/agents/:id/artifacts` | Agent 产出物 |
| `GET /api/agents/:id/workspace` | Agent 工作空间概览 |
| `GET/POST /api/services` | 子服务生命周期 |

### agent (:1996)

| 路由 | 功能 |
|------|------|
| `POST /api/agent/chat/stream` | Agent 执行（SSE） |
| `POST /api/agent/interrupt` | 中断执行 |
| `POST /api/process/restart-server` | 重启 server |
| `/v1/*` | API Proxy（多模型格式转换） |

### channel-feishu (:1999)

| 路由 | 功能 |
|------|------|
| `POST /api/feishu/send` | 统一消息发送（文本/富文本/卡片/图片/文件/音频/视频 + @用户 + 引用回复） |
| `POST /api/feishu/action` | Agent 统一 Action 端点 |
| `GET/DELETE /api/feishu/message/*` | 消息查询与管理（详情/列表/撤回） |
| `POST /api/feishu/message/chat` | 群组管理（创建群/获取群信息/成员） |
| `/api/feishu/meeting/*` | 会议管理 |
| `/api/feishu/document/*` | 文档操作 |

### channel-qiwei (:2000)

| 路由 | 功能 |
|------|------|
| `/webhook/callback` | 企微回调接收 |
| `/api/qiwei/send` | 发送企微消息 |

---

## 八、关键文件索引

### server

| 文件 | 作用 |
|---|---|
| `src/services/channel-router.ts` | 消息处理流水线（去重→解析→执行→回复） |
| `src/services/channel-registry.ts` | 渠道适配器注册、查找、消息路由 |
| `src/services/channel-types.ts` | 统一消息类型定义 |
| `src/services/user-resolver.ts` | 用户身份解析、影子用户、跨渠道绑定 |
| `src/services/memory-manager.ts` | 用户记忆加载、事实管理 |
| `src/services/database.ts` | SQLite 数据层（含独立 messages 表） |
| `src/services/orchestrator-client.ts` | 与 Orchestrator 通信（支持 AgentContext） |
| `src/routes/channels.ts` | 统一渠道入口 & 出站消息接口 |
| `src/routes/messages.ts` | 消息查询端点（按会话分页） |
| `src/routes/agent-profiles.ts` | Agent Profile CRUD API |
| `src/routes/agent-workspace.ts` | Agent 工作空间 API（笔记/任务/产出物） |
| `src/services/service-manager.ts` | 子服务生命周期管理 |
| `src/services/skill-manager.ts` | 技能管理 |
| `src/services/models/*` | 多模型适配器 |

### agent

| 文件 | 作用 |
|---|---|
| `src/services/claude-agent.ts` | Claude Agent SDK 封装 |
| `src/services/api-proxy.ts` | Anthropic ↔ OpenAI 格式转换 |
| `src/services/process-manager.ts` | 进程管理 |
| `src/services/config-manager.ts` | 配置管理 |
| `src/services/skill-loader.ts` | 技能加载 |

### admin

| 目录/文件 | 作用 |
|---|---|
| `src/app.tsx` | 路由配置 + ErrorBoundary |
| `src/api/` | 按领域划分的 API 模块（agents, sessions, models, traces, logs, skills, settings, services） |
| `src/hooks/` | TanStack Query 自定义 hooks（数据获取、轮询、缓存） |
| `src/stores/` | Zustand 全局状态（sidebar 折叠等 UI 状态） |
| `src/components/ui/` | 基础 UI 组件（shadcn/ui 风格，基于 Radix UI） |
| `src/components/layout/` | 布局组件（sidebar, app-layout, page-header） |
| `src/components/shared/` | 跨页面共享组件（status-badge, channel-badge, markdown-content） |
| `src/components/features/` | 按页面划分（monitor, agents, models, skills, logs, settings） |

---

## 九、演进路线

### 当前已完成 ✅

- [x] 自举架构（Orchestrator + Server 分离）
- [x] Claude Agent SDK 集成
- [x] 统一渠道架构（飞书/企微/WebUI）
- [x] 统一用户身份 & 跨渠道绑定
- [x] 记忆系统（摘要 + 结构化事实）
- [x] 多模型切换（API Proxy）
- [x] 流式对话（SSE）
- [x] 服务管理（启停/重启/日志）
- [x] 技能系统
- [x] 管理后台（Monitor / Agents / Models / Skills / Logs / Settings）

### 多 Agent 架构（已完成核心实现）✅

| Phase | 内容 | 状态 |
|-------|------|------|
| **Phase 1** | Agent Profile CRUD + 持久化 | ✅ 完成 |
| **Phase 2** | 对话隔离 | ✅ 完成：每个 Agent 独立 session、独立 systemPrompt、独立记忆 |
| **Phase 3** | Agent 渠道绑定 | ✅ 完成：agent_configs.channels 绑定，一个 Agent 多渠道 |
| **Phase 4** | 消息多投 | ✅ 完成：每个 Agent 是群内独立 bot，各自收到消息，各自独立决策 |
| **Phase 5** | Agent Workspace | ✅ 完成：工作笔记、任务记录、产出物关联 |

多 Agent 核心架构已全部实现。

### 消息存储 & 统一出站（已完成）✅

| 内容 | 说明 |
|------|------|
| 独立 messages 表 | 每条消息一行，支持按会话分页、按时间索引、状态追踪 |
| 统一出站接口 | `POST /api/channels/send` 先存后发，返回 messageId |
| sendToChannel 先存后发 | 写入 messages 表 → 调用适配器 → 更新状态（sent/failed） |
| 飞书统一发送 | `POST /api/feishu/send` 合并所有消息类型，支持 @用户 + 引用回复 |
| 消息查询 API | `GET /api/messages?sessionId=&limit=&before=` 分页查询 |

### 下一阶段：可能的增强

| 方向 | 内容 | 说明 |
|------|------|------|
| Agent 管理前端 | ✅ 已完成 | Monitor / Agents / Models / Skills / Logs / Settings |
| Agent 自动记忆 | 对话后自动提取事实并存入 memory | 无需手动维护，Agent 自动学习 |
| 多模型热切换 | 每个 Agent 使用不同模型 | API Proxy 按 agentId 路由到不同 provider |
| Agent 间通讯 | Agent 之间通过消息管道直接通讯 | 不依赖群聊，支持 1:1 Agent 对话 |

---

## 十、相关文档

| 文档 | 说明 |
|------|------|
| [产品需求文档](./PRODUCT_REQUIREMENTS.md) | 原始 PRD，自举架构设计 |
| [统一渠道架构](./UNIFIED_CHANNEL_ARCHITECTURE.md) | 渠道抽象、用户身份、记忆系统详细设计 |
| [实施计划](./IMPLEMENTATION_PLAN.md) | 原始实施计划（自举架构阶段） |
| [变更记录](./decisions/README.md) | 重要设计决策记录 |
