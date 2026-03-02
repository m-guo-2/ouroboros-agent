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
│   渠道适配器（独立进程）                                  │
│   ┌──────────────┐  ┌──────────────┐                    │
│   │channel-feishu│  │channel-qiwei │                    │
│   │    :1999     │  │    :2000     │                    │
│   └──────┬───────┘  └──────┬───────┘                    │
│          └─────────────────┘                            │
│                    │  POST /api/channels/incoming        │
│                    ▼                                     │
│   agent :1997  (Go 单体二进制)                           │
│   • 渠道路由 • 身份管理 • Agent Profile                  │
│   • 会话/记忆 • 消息投递 • 技能管理                      │
│   • Claude Agent SDK • 多模型 API Proxy                  │
│   • 静态托管 admin SPA (/admin)                          │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### 多 Agent 设计

- **统一参与者模型** — human/agent 共享同一套身份、渠道、session 体系
- **独立 Session** — 同一个群里多个 Agent，各自有独立上下文和记忆
- **协作涌现** — 系统不做编排，协作逻辑写在各自的 systemPrompt 里
- **一 Agent 多渠道** — 同一个 Agent 可同时在飞书、企微、WebUI 上工作

## 技术栈

| 层级 | 技术 |
|------|------|
| 核心后端 | Go 1.24 单体二进制 |
| 前端 | React 19 + Vite + TypeScript |
| 数据库 | SQLite (`modernc.org/sqlite`) |
| Agent 引擎 | Anthropic Claude API（直接调用）|
| 多模型 | API Proxy (Claude / GPT-4o / DeepSeek / 百川 / Kimi / GLM) |
| 渠道适配器 | TypeScript（飞书 / 企微）|

## 快速开始

```bash
# 构建 admin 前端
cd admin && npm install && npm run build && cd ..

# 构建 Go 二进制
cd agent && go build -o ../bin/agent ./cmd/agent && cd ..

# 配置环境变量
export ANTHROPIC_API_KEY=sk-ant-xxx
export DB_PATH=./data/config.db   # 可选，默认为 ./data/config.db

# 启动
./bin/agent
# 访问 http://localhost:1997 → admin SPA
# API 基础路径：http://localhost:1997/api/

# 启动飞书渠道（独立进程）
cd channel-feishu && npm install && npm run dev
```

### 端口

| 服务 | 端口 | 说明 |
|------|------|------|
| agent (含 admin SPA) | 1997 | 主进程，所有 API + 管理界面 |
| channel-feishu | 1999 | 飞书渠道适配器 |
| channel-qiwei | 2000 | 企微渠道适配器 |

## 项目结构

```
.
├── agent/                 # Go 单体二进制（主进程）
│   ├── cmd/agent/         # main.go
│   └── internal/
│       ├── api/           # 管理 API handlers
│       ├── channels/      # 渠道出向适配器
│       ├── dispatcher/    # 消息入向分发
│       ├── engine/        # LLM 推理引擎
│       ├── logger/        # 结构化日志
│       ├── runner/        # 任务调度
│       └── storage/       # SQLite CRUD
│
├── admin/                 # 管理后台 SPA（构建后由 agent 静态托管）
│   └── Monitor, Agent管理, 模型/技能/日志/设置
│
├── channel-feishu/        # 飞书渠道适配器（独立进程）
│   └── 消息, 会议, 文档
│
├── channel-qiwei/         # 企微渠道适配器（独立进程）
│   └── 消息收发
│
├── data/                  # 运行时数据（.gitignore）
│   ├── config.db          # SQLite 数据库
│   └── logs/              # JSONL 执行日志
│
└── docs/                  # 文档
    └── decisions/         # 设计决策记录
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
