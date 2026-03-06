# Admin — 管理后台

Moli Agent 平台的管理后台前端，用于监控 Agent 活动和管理平台配置。

## 功能页面

| 页面 | 路由 | 说明 |
|------|------|------|
| **Monitor** | `/monitor` | 统一会话查看器。左侧会话列表，右侧消息交互详情（用户消息 → 执行 trace → 助手回复） |
| **Agents** | `/agents` | Agent 列表和详情管理 |
| **Models** | `/models` | 模型配置管理 |
| **Skills** | `/skills` | 技能列表和详情查看 |
| **Logs** | `/logs` | 结构化日志查看 |
| **Settings** | `/settings` | 系统设置 |

## 技术栈

| 类别 | 技术 |
|------|------|
| 框架 | React 19 + TypeScript |
| 构建 | Vite 7 |
| 样式 | Tailwind CSS 4 + CSS Variables 设计令牌 |
| 组件 | shadcn/ui（本地组件，基于 Radix UI） |
| 数据 | TanStack Query v5（轮询，无 SSE） |
| 状态 | Zustand |
| 路由 | React Router v7 |
| 图标 | Lucide React |

## 架构

```
src/
├── api/            # 按领域划分的 API 模块（agents, sessions, models, ...）
├── hooks/          # TanStack Query 自定义 hooks
├── stores/         # Zustand 全局状态
├── lib/            # 工具函数（cn, timeAgo, formatDuration, ...）
├── components/
│   ├── ui/         # 基础 UI 组件（button, badge, card, ...）
│   ├── layout/     # 布局组件（sidebar, app-layout, page-header）
│   ├── shared/     # 跨页面共享组件（status-badge, markdown-content）
│   └── features/   # 按页面划分的功能组件
│       ├── monitor/
│       ├── agents/
│       ├── models/
│       ├── skills/
│       ├── logs/
│       └── settings/
├── app.tsx         # 路由配置 + ErrorBoundary
└── main.tsx        # 入口
```

## 开发

```bash
cd admin
bun install
bun run dev     # http://localhost:5173
```

API 请求代理到 `http://localhost:1997`（需要先启动 server）。
