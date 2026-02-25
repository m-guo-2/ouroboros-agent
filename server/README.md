# 🐍 Ouroboros Agent

> *衔尾蛇 — 自我循环、自我演化的 AI Agent 服务*

一个支持多模型的统一 AI Agent 服务器，采用适配器模式设计，可轻松扩展更多模型。

**核心理念**：通过内置的 Agent SDK，这个项目可以不断更新自己的功能，实现真正的**自举**（Self-bootstrapping）——用 Agent 来开发 Agent！

## ✨ 特性

- 🔌 **多模型支持** - Claude、OpenAI、Kimi（Moonshot）、GLM（智谱）
- 🎯 **统一 API** - 一套接口调用所有模型
- ⚡ **流式输出** - 实时返回生成内容
- 💬 **会话管理** - 支持多轮对话上下文
- 🔧 **适配器模式** - 轻松扩展新模型
- 🚀 **Bun 驱动** - 极速启动，现代化开发体验

## 🚀 快速开始

### 前置要求

- [Bun](https://bun.sh/) v1.0+

### 安装

```bash
# 克隆项目
git clone https://github.com/gm-stack/ouroboros-agent.git
cd ouroboros-agent

# 安装依赖
bun install
```

### 配置

创建 `.env` 文件并配置你的 API Key：

```env
# OpenAI
OPENAI_API_KEY=sk-xxx

# Claude (Anthropic)
CLAUDE_API_KEY=sk-ant-xxx

# Kimi (Moonshot)
KIMI_API_KEY=sk-xxx

# GLM (智谱)
GLM_API_KEY=xxx

# 服务器端口（可选）
PORT=3001
```

### 启动

```bash
# 开发模式（热重载）
bun run dev

# 生产模式
bun run start
```

服务启动后访问 `http://localhost:3001`

## 📖 API 文档

### 健康检查

```http
GET /api/health
```

### 创建会话

```http
POST /api/conversations
Content-Type: application/json

{
  "modelId": "claude-sonnet-4-20250514",
  "title": "新对话"
}
```

### 发送消息

```http
POST /api/conversations/:id/chat
Content-Type: application/json

{
  "message": "你好！"
}
```

### 获取可用模型

```http
GET /api/models
```

## 🏗️ 项目结构

```
ouroboros-agent/
├── src/
│   ├── index.ts           # 入口文件
│   ├── config/            # 配置管理
│   ├── routes/            # API 路由
│   │   ├── chat.ts        # 聊天相关路由
│   │   └── models.ts      # 模型相关路由
│   └── services/
│       ├── agent.ts       # Agent 核心服务
│       └── models/        # 模型适配器
│           ├── base.ts    # 基础适配器接口
│           ├── claude.ts  # Claude 适配器
│           ├── openai.ts  # OpenAI 适配器
│           ├── kimi.ts    # Kimi 适配器
│           ├── glm.ts     # GLM 适配器
│           └── registry.ts # 模型注册表
├── package.json
└── tsconfig.json
```

## 🔮 自举愿景

这个项目的终极目标是实现**自举**——让 Agent 能够：

1. 📝 分析自己的代码
2. 🐛 发现并修复 Bug
3. ✨ 添加新功能
4. 🔄 持续演化

就像衔尾蛇（Ouroboros）一样，首尾相连，自我循环，永恒演化。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📜 License

MIT

---

<p align="center">
  <i>用 Agent 开发 Agent，让代码自己演化 🐍</i>
</p>
