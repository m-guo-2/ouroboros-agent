# Ouroboros Agent

> *衔尾蛇 — 自我循环、自我演化的 AI Agent 平台*

构建通讯基础设施，让 AI Agent 像人一样在里面工作和协作。

## 核心理念

**Agent 是人，不是工具。** 系统不编排协作，只提供基础设施。Agent 有自己的身份、记忆、渠道，协作从 prompt 中涌现。

```
传统：   人 → 调用 → AI工具 → 返回结果

这里：   人 ──┐
         AI ─┼─→ 共同工作在同一个沟通环境里
         AI ─┘
```

## 架构

```
┌──────────────────────────────────────────────────────────┐
│                                                          │
│   渠道层                                                 │
│   ┌──────────────┐  ┌──────────────┐  ┌────────┐       │
│   │channel-feishu│  │channel-qiwei │  │ admin  │        │
│   │    :1999     │  │    :2000     │  │ :5173  │        │
│   └──────┬───────┘  └──────┬───────┘  └───┬────┘        │
│          └─────────────────┼──────────────┘              │
│                            ▼                             │
│   server :1997  (业务控制器)                              │
│   • 渠道路由 • 身份管理 • Agent Profile                  │
│   • 会话/记忆 • 消息投递(→多Agent) • 技能管理            │
│                            │                             │
│                            ▼                             │
│   agent :1996  (执行引擎)                                │
│   • Claude Agent SDK • API Proxy(多模型)                 │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### 自举（Bootstrap）

业务代码可以被 Agent 自己修改并热更新：

- **Agent** = 执行引擎（围绕 Claude Agent SDK）
- **Server** = 可演化的业务控制器（可被 Agent 修改和重启）

### 多 Agent 设计

- **统一参与者模型** — human/agent 共享同一套身份、渠道、session 体系
- **独立 Session** — 同一个群里多个 Agent，各自有独立上下文和记忆
- **协作涌现** — 系统不做编排，协作逻辑写在各自的 systemPrompt 里
- **一 Agent 多渠道** — 同一个 Agent 可同时在飞书、企微、WebUI 上工作

## 技术栈

| 层级 | 技术 |
|------|------|
| 运行时 | Bun |
| 后端 | Express + TypeScript |
| 前端 | React 19 + Vite + TypeScript |
| 数据库 | SQLite (bun:sqlite) |
| Agent 引擎 | @anthropic-ai/claude-agent-sdk |
| 多模型 | API Proxy (Claude / GPT-4o / DeepSeek / 百川 / Kimi / GLM) |

## 快速开始

```bash
# 安装 Bun
curl -fsSL https://bun.sh/install | bash

# 安装依赖
bun run install:all

# 配置环境变量
cp .env.example .env
# 编辑 .env，填入 ANTHROPIC_API_KEY 等

# 启动（开发模式）
bun run dev
```

### 端口

| 服务 | 端口 | 说明 |
|------|------|------|
| agent | 1996 | 执行引擎 |
| server | 1997 | 业务控制器 |
| admin | 5173 | 管理后台 |
| channel-feishu | 1999 | 飞书渠道 |
| channel-qiwei | 2000 | 企微渠道 |

## 项目结构

```
.
├── agent/                 # 执行引擎（Claude Agent SDK）
│   └── SDK Runner, API Proxy, Context Composer
│
├── server/                # 业务控制器（可演化）
│   └── 渠道路由, 身份管理, 会话/记忆, Agent Profile
│
├── admin/                 # 管理后台
│   └── Monitor, Agent管理, 模型/技能/日志/设置
│
├── channel-feishu/        # 飞书渠道适配器
│   └── 消息, 会议, 文档
│
├── channel-qiwei/         # 企微渠道适配器
│   └── 消息收发
│
└── docs/                  # 文档
    ├── ARCHITECTURE.md              # 系统架构总览 ← 主文档
    ├── PRODUCT_REQUIREMENTS.md      # 产品需求（自举架构）
    ├── UNIFIED_CHANNEL_ARCHITECTURE.md  # 统一渠道架构
    ├── IMPLEMENTATION_PLAN.md       # 实施计划
    └── decisions/                   # 设计决策记录
```

## 文档

| 文档 | 说明 |
|------|------|
| **[系统架构总览](./docs/ARCHITECTURE.md)** | 理念、设计、现状、演进路线 — 看这一份就够 |
| [产品需求文档](./docs/PRODUCT_REQUIREMENTS.md) | 自举架构详细设计 |
| [统一渠道架构](./docs/UNIFIED_CHANNEL_ARCHITECTURE.md) | 渠道抽象、用户身份、记忆系统 |
| [设计决策记录](./docs/decisions/) | 重要架构决策 |

## License

MIT
