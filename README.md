# 🐍 Ouroboros Agent

> *衔尾蛇 — 自我循环、自我演化的 AI Agent 平台*

一个基于 TypeScript 的多模型 AI Agent 服务平台，支持 Claude、OpenAI、Kimi、GLM 等多种模型。

**核心理念**：通过内置的 Agent 能力，这个项目可以不断更新自己的功能，实现真正的**自举**（Self-bootstrapping）——用 Agent 来开发 Agent！

## ✨ 功能特性

- 🤖 **多模型支持** — Claude / OpenAI / Kimi / GLM 一键切换
- 💬 **流式对话** — 实时输出，打字机效果
- 🎨 **现代 UI** — 简洁优雅的对话界面
- ⚙️ **在线配置** — 无需重启即可配置 API Key
- 📝 **会话管理** — 多轮对话，历史记录
- 🔌 **适配器模式** — 轻松扩展更多模型

## 🔮 自举愿景

这个项目的终极目标是实现**自举**——让 Agent 能够：

1. 📝 分析自己的代码
2. 🐛 发现并修复 Bug
3. ✨ 添加新功能
4. 🔄 持续演化

就像衔尾蛇（Ouroboros）一样，首尾相连，自我循环，永恒演化。

## 🛠️ 技术栈

- **后端**: Bun + Express + TypeScript
- **前端**: React 19 + Vite + TypeScript
- **端口**: 1997

## 快速开始

### 1. 安装依赖

```bash
# 安装 Bun（如果尚未安装）
curl -fsSL https://bun.sh/install | bash

# 安装项目依赖
bun run install:all
```

### 2. 配置环境变量

在项目根目录创建 `.env` 文件：

```env
PORT=1997
NODE_ENV=development

# Claude (Anthropic)
ANTHROPIC_API_KEY=your-key

# OpenAI
OPENAI_API_KEY=your-key
OPENAI_BASE_URL=https://api.openai.com/v1

# Kimi (月之暗面)
MOONSHOT_API_KEY=your-key

# GLM (智谱)
ZHIPU_API_KEY=your-key
```

> 💡 也可以在启动后通过 Web 界面配置 API Key

### 3. 开发模式运行

```bash
# 同时启动后端和前端开发服务器
bun run dev
```

- 后端 API: http://localhost:1997
- 前端开发: http://localhost:5173

### 4. 生产部署

```bash
# 构建前端
bun run build

# 启动生产服务
bun run start
```

访问 http://your-server:1997 即可使用。

## 项目结构

```
.
├── agent-server/          # 后端服务
│   ├── src/
│   │   ├── index.ts       # 入口
│   │   ├── config/        # 配置
│   │   ├── routes/        # API 路由
│   │   └── services/      # 业务逻辑
│   │       └── models/    # 模型适配器
│   └── package.json
│
├── agent-web/             # 前端应用
│   ├── src/
│   │   ├── App.tsx        # 主组件
│   │   ├── components/    # UI 组件
│   │   └── api/           # API 调用
│   └── package.json
│
├── .env                   # 环境变量（需手动创建）
└── package.json           # 根目录脚本
```

## API 接口

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/models` | GET | 获取模型列表 |
| `/api/models/:id` | PATCH | 更新模型配置 |
| `/api/conversations` | GET/POST | 会话管理 |
| `/api/conversations/:id/chat` | POST | 发送消息（SSE） |

## 支持的模型

| Provider | 模型 | 说明 |
|----------|------|------|
| Claude | claude-sonnet-4-20250514 | Anthropic 最新模型 |
| OpenAI | gpt-4o | GPT-4o 模型 |
| Kimi | moonshot-v1-8k | 月之暗面 8K 上下文 |
| GLM | glm-4 | 智谱清言 GLM-4 |

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📜 License

MIT

---

<p align="center">
  <i>用 Agent 开发 Agent，让代码自己演化 🐍</i>
</p>
