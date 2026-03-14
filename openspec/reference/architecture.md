# Moli Agent — 系统架构

> 造一个通讯基础设施，让 AI Agent 像人一样在里面工作和协作。

| 项目 | 内容 |
|------|------|
| 项目名称 | Moli Agent |
| 最后更新 | 2026-03-14 |
| 状态 | 对齐当前代码实现 |

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

不设计两套体系。同一张表、同一套渠道绑定、同一套 session 管理。

**2. Session 按参与者隔离 — 每人看到的世界不同**

同一个群的讨论，PM Agent 和 Coder Agent 各有自己的 session、系统提示、记忆。同一场会议，每个人理解和记住的不一样。

**3. 协作靠 prompt 涌现 — 系统不做编排**

系统不知道 PM 和 Coder 之间有协作关系。协作方式写在各自的 systemPrompt 里，自然发生。

**4. 一个 Agent 多个渠道 — 无处不在**

PM Agent 可以同时在飞书群里讨论需求、在企微群里跟客户沟通、在 WebUI 上操作。渠道是手段，Agent 是主体。

**5. 事件驱动 — Agent 不是 Chatbot**

Agent 不仅响应用户消息，还响应系统事件（定时任务到期、异步任务完成等）。工作流统一为：唤醒 Session → 加载 Context → 追加 Event → 触发 ReAct Loop → 保存 Context。

### 类比

| 概念 | 类比 |
|------|------|
| 系统 | 公司的基础设施（办公室、通讯工具、HR 系统） |
| Agent Profile | 员工档案（姓名、职位、技能） |
| systemPrompt | 岗位说明书（职责、权限、汇报关系） |
| skills | 可以使用的工具（飞书 API、企微 API、Shell 等） |
| session | 员工的工作笔记本 |
| memory | 员工的长期记忆与经验 |
| 渠道（飞书/企微/WebUI） | 公司配的工作通讯工具 |

**一句话 recap**：造基础设施（消息管道 + 身份 + 记忆），不造协作逻辑。让 Agent 像人一样自主工作，协作从 prompt 中涌现。

---

## 二、系统架构

### 当前部署架构

```
┌─────────────────────────────────────────────────────────────────┐
│                         用户 & Agent                            │
│                          │                                      │
│            ┌─────────────┼─────────────┐                        │
│            │             │             │                        │
│        飞书消息       企微消息       WebUI                       │
│            │             │             │                        │
│   ┌────────▼───┐  ┌──────▼────┐       │                        │
│   │ channel-    │  │ channel-  │       │                        │
│   │ feishu      │  │ qiwei     │       │                        │
│   │ :1999       │  │ :2000     │       │                        │
│   └────────┬───┘  └──────┬────┘       │                        │
│            │ IncomingMsg  │             │                        │
│            └─────────────┼─────────────┘                        │
│                          ▼                                      │
│              ┌───────────────────────┐                          │
│              │      agent (Go)       │  ← 单体 Go 进程          │
│              │       :1997           │                          │
│              │                       │                          │
│              │  Dispatcher            │  入口：去重→用户→会话→入队│
│              │  Runner                │  执行：Worker + Processor│
│              │  Engine                │  引擎：ReAct + LLM + Tools│
│              │  API                   │  管理：Admin REST API    │
│              │  Channels              │  出口：SendToChannel     │
│              │  Storage (SQLite)      │  存储：所有持久化数据     │
│              │  SPA (admin/dist)      │  前端：静态资源托管       │
│              └───────────────────────┘                          │
└─────────────────────────────────────────────────────────────────┘
```

### 三个进程

| 进程 | 端口 | 语言 | 职责 |
|------|------|------|------|
| **agent** | 1997 | Go | 单体后端：API、Dispatcher、Runner、Engine、Storage、SPA 托管 |
| **channel-feishu** | 1999 | Go | 飞书渠道适配器：事件接收（Webhook/WS）、消息发送、Lark API |
| **channel-qiwei** | 2000 | Go | 企微渠道适配器：回调接收、媒体管线（OSS + ASR）、消息发送 |

admin 前端构建产物（`admin/dist`）由 agent 进程直接托管为静态资源。

### Go 模块结构

| 模块 | 路径 | 说明 |
|------|------|------|
| agent | `agent/go.mod` | 主进程，依赖 shared/logger |
| channel-feishu | `channel-feishu/go.mod` | Lark oapi-sdk-go |
| channel-qiwei | `channel-qiwei/go.mod` | 依赖 shared/logger, shared/oss |
| shared/logger | `shared/logger/go.mod` | 结构化日志 + SQLite 日志存储 |
| shared/oss | `shared/oss/go.mod` | MinIO/S3 对象存储抽象 |

---

## 三、核心数据流

### 消息处理全流程

```
用户消息 (飞书 / 企微 / WebUI)
  → Channel 适配器标准化为 IncomingMessage
  → POST /api/channels/incoming
  → Dispatcher:
      去重(messageId+agentId)
      → 用户身份解析(影子用户/绑定)
      → Agent 配置加载
      → Session 定位/创建(agentId × userId × channel)
      → 消息持久化到 messages 表
      → 入队到 Runner
  → Runner Worker:
      Session 级队列 → popRequest
      → Processor.processOneEvent
  → Processor:
      加载 session context (messages JSON)
      → 构建 system prompt ({{skills}} 模板)
      → 注册 tools (builtin + skills + MCP)
      → 执行 Absorb Loop:
          ┌─→ Execute: engine.RunAgentLoop (LLM ↔ Tools)
          │   → Checkpoint: token 估算、compaction、session 保存
          │   → Absorb: popAllPending 新消息
          └─── 循环直到无新消息或达到 MaxAbsorbRounds
  → send_channel_message tool
      → channels.SendToChannel
      → HTTP POST 到对应 Channel 适配器
  → 用户收到回复
```

### 上下文与消息的读写分离 (CQRS)

三种数据各自独立，解决了早期"一张表满足所有需求"的混乱：

| 存储 | 用途 | 位置 |
|------|------|------|
| **Session Context** | LLM 的记忆：完整 AgentMessage[] | `agent_sessions.messages` (JSON) |
| **Chat Messages** | 用户可见的对话记录 | `messages` 表 |
| **Execution Traces** | 开发者调试用的执行轨迹 | SQLite per-day DB (shared/logger) + JSONL |

**核心约束**：
- Session Context 存储完整的 tool_use、tool_result，是 LLM 的真实记忆
- Messages 表只存用户可见的交互（人类消息 + Agent 显式回复），不存模型思考
- Traces 记录每一步 thought/action/observation，大体量异步写入

### 强制工具回复模式

```
Text output   = 内部思考 (CoT)，用户不可见
Tool call     = 外部行为 (send_channel_message)，用户可见
No tool call  = 任务完成，Loop 自然结束
```

模型必须且只能调用 `send_channel_message` 才能将信息发给用户。避免"反向幻觉"——模型以为心里想的话用户已经看到了。

---

## 四、Agent 内部架构 (agent/)

### 包结构

| 包 | 职责 |
|---|---|
| `cmd/agent/main.go` | HTTP server、路由注册、SPA 托管、graceful shutdown |
| `internal/dispatcher/` | 入站管线：去重 → 用户解析 → Agent 配置 → Session → 消息保存 → 入队 |
| `internal/runner/` | Worker（Session 级队列、空闲淘汰）、Processor（ReAct 执行）、Scheduler（定时任务）|
| `internal/engine/` | ReAct 引擎：LLM client (Anthropic/OpenAI)、Agent Loop、Tool Registry |
| `internal/engine/ostools/` | 内置 OS 工具：shell、read_file、write_file、list_dir、grep、save_memory、recall_context |
| `internal/channels/` | 出站适配器：Registry、HTTP 适配器（Feishu/Qiwei）、WebuiAdapter |
| `internal/subagent/` | 子 Agent 管理：web_research、developer、file_analysis profiles |
| `internal/storage/` | SQLite 全量 CRUD |
| `internal/api/` | Admin REST API |
| `internal/config/` | 配置加载 |
| `internal/sanitize/` | 运行时密钥脱敏 |
| `internal/github/` | GitHub Skill 仓库同步 |

### ReAct 引擎 (engine/)

自定义 `while` 循环，取代早期的 Claude Agent SDK：

- **100% 透明**：每一步 Thought、Action、Observation 都通过事件流上报
- **无副作用**：引擎不持有状态，每次调用接近纯函数
- **多模型**：Anthropic 原生 + OpenAI 兼容（GPT/DeepSeek 等）
- **工具注册**：builtin tools + skill tools + MCP tools

### 工具体系

| 类别 | 工具 |
|------|------|
| **OS Tools** | `shell`, `read_file`, `write_file`, `list_dir`, `grep` |
| **Memory** | `save_memory`, `recall_context` |
| **Communication** | `send_channel_message` |
| **Search** | `tavily_search` (builtin) |
| **Subagent** | `run_subagent_async`, `get_subagent_status`, `cancel_subagent` |
| **WeCom** | `wecom_search_targets`, `wecom_list_or_get_conversations`, `wecom_parse_message`, `inspect_attachment`, `wecom_send_message` |
| **Delayed Tasks** | `set_delayed_task`, `cancel_delayed_task`, `list_delayed_tasks` |
| **Skills** | 动态加载（always / on_demand 两种模式）|

### Absorb-Replan 循环

处理消息不是简单的请求-响应，而是一个循环：

```
Execute → Checkpoint → Absorb → (repeat if new messages)
```

- **Execute**: RunAgentLoop（LLM + Tools）
- **Checkpoint**: token 估算、必要时 compaction、session 保存
- **Absorb**: 检查队列中是否有新消息（popAllPending），有则合并到 context 继续处理
- **Max Rounds**: 防止无限循环

### Context Compaction

当 session context 超过 token 阈值时，触发 LLM 摘要压缩：
- 保留最近的消息，压缩旧消息为摘要
- 压缩前 flush session facts（save_memory 的安全网）
- 压缩记录存入 `context_compactions` 表

---

## 五、渠道架构

### 统一消息契约

所有渠道适配器将平台消息标准化为 `IncomingMessage`，出站时接受 `OutgoingMessage`。agent 进程完全不感知渠道协议细节。

### channel-feishu

- Lark oapi-sdk-go 接入
- 支持 Webhook + WebSocket 两种事件接收模式
- 消息类型：文本、富文本、图片、文件
- 扩展能力：会议管理、文档操作、群组管理

### channel-qiwei

- 企微回调解密 + 异步转发
- **媒体管线**：
  - 消息分类（text/image/voice/file/video/sticker/link/location/miniapp/mixed）
  - 下载计划（planMediaDownload）→ OSS 物化 → 资源 URI
  - 语音前置转写（OSS → ASR via Volcengine）
  - 结构化附件（attachments[]）
- **四接口沟通门面**：search_targets、list_or_get_conversations、parse_message、send_message
- **按需解析**：`inspect_attachment` 工具，Agent 主动请求解析图片/文件

### 用户身份统一

- **影子用户**：首次通过某渠道交流时自动创建
- **绑定码**：6 位绑定码跨渠道关联
- **影子合并**：绑定时自动合并数据（渠道绑定、记忆事实、会话）
- **记忆共享**：绑定后同一用户在所有渠道共享记忆

---

## 六、记忆系统

### 两层结构

| 层级 | 存储 | 粒度 |
|------|------|------|
| **用户记忆摘要** | `user_memory` | Agent × User：整体认知概述 |
| **用户记忆事实** | `user_memory_facts` | Agent × User：细粒度事实（preference / context / relationship / skill）|
| **会话事实** | `session_facts` | Session 级：当前会话中 save_memory 保存的事实 |

### Memory 工具

- `save_memory`：Agent 主动保存重要信息到 session_facts
- `recall_context`：Agent 主动召回相关记忆
- Compaction 前自动 flush session facts 到 user_memory_facts（安全网）

---

## 七、数据模型

### 核心表

| 表 | 用途 |
|---|---|
| `agent_configs` | Agent 配置（systemPrompt, model, skills, channels） |
| `agent_sessions` | 会话（session_key, agent_id, user_id, messages JSON） |
| `messages` | 用户可见的对话记录 |
| `users` | 统一用户（type: human/agent） |
| `user_channels` | 渠道绑定（userId ↔ channelType + channelUserId） |
| `user_memory` | 用户记忆摘要（per agent × user） |
| `user_memory_facts` | 结构化记忆事实 |
| `session_facts` | 会话级事实 |
| `models` | 模型配置 |
| `skills` | 技能定义 |
| `settings` | 全局配置 |
| `processed_messages` | 消息去重 |
| `context_compactions` | 上下文压缩记录 |
| `delayed_tasks` | 延时任务 |

### 日志存储

| 存储 | 实现 | 用途 |
|------|------|------|
| 文件日志 | JSONL files | Boundary/Business/Detail 三级日志 |
| SQLite 日志 | Per-day SQLite DB (shared/logger) | Traces、LLM I/O，支持 Monitor 查询 |
| LLM I/O | 独立文件 | 完整的模型输入/输出捕获 |

---

## 八、Admin API

所有管理接口挂载在 `/api` 下：

| 路由 | 功能 |
|------|------|
| `/api/channels/incoming` | 统一渠道入站（POST） |
| `/api/data/channels/send` | send_channel_message 门面 |
| `/api/agent-sessions` | 会话管理 |
| `/api/messages` | 消息查询 |
| `/api/agents` | Agent 配置 CRUD |
| `/api/models` | 模型管理 |
| `/api/skills` | 技能管理 |
| `/api/skills/refresh` | GitHub 技能刷新 |
| `/api/users` | 用户管理 |
| `/api/settings` | 全局配置 |
| `/api/settings/provider-models` | 模型发现 |
| `/api/traces` | 执行轨迹查询 |
| `/api/services` | 子服务生命周期 |
| `/api/channels` | 渠道状态 |
| `/health` | 健康检查 |
| `/drain` | 优雅下线 |

---

## 九、Admin 前端 (admin/)

React 19 + Vite + TypeScript SPA：

| 页面 | 功能 |
|------|------|
| `/monitor` | 三栏布局：Session 列表 / 对话时间线 / 决策检查器（LLM I/O、Absorb 轮次、Compaction）|
| `/agents` | Agent 配置管理 |
| `/models` | 模型配置 |
| `/skills` | 技能列表与详情 |
| `/settings` | 全局设置 |

---

## 十、技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go (agent, channel-feishu, channel-qiwei) |
| 前端 | React 19 + Vite + TypeScript |
| 数据库 | SQLite (modernc.org/sqlite, 纯 Go) |
| 对象存储 | MinIO (shared/oss) |
| 日志 | slog + SQLite per-day + JSONL |
| LLM | Anthropic (原生) + OpenAI 兼容 (GPT/DeepSeek 等) |
| 搜索 | Tavily (内置) |
| 语音 | Volcengine ASR |
| 飞书 SDK | larksuite/oapi-sdk-go/v3 |
| 部署 | systemd + nginx 反向代理 |
