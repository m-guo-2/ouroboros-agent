# 统一渠道架构（Unified Channel Architecture）

## 概述

统一渠道架构将飞书、企微、WebUI 三个通信渠道抽象为同构的渠道适配器。每个渠道都具备两项核心能力：

1. **接收消息推送** → 归一化后转发给 Agent 处理
2. **被 Agent 回调** → 将 AI 回复送达端侧用户

同时，架构实现了 **统一用户身份**：同一用户通过不同渠道交流时，Agent 具备跨渠道的全局记忆和用户认知。

---

## 架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            End Users                                    │
│   [飞书 App]        [企微 App]         [WebUI Browser]                  │
└─────┬────────────────┬──────────────────┬───────────────────────────────┘
      │                │                  │
      ▼                ▼                  ▼
┌──────────┐   ┌──────────┐   ┌────────────────────────────────────────┐
│channel-feishu│   │channel-qiwei │   │              admin                 │
│ :1999    │   │ :2000    │   │         (React Frontend)               │
│          │   │          │   │  localStorage UUID → channelUserId     │
│ Webhook  │   │ Callback │   │  REST API + TanStack Query 轮询        │
│ ↓        │   │ ↓        │   │                                        │
│ Normalize│   │ Normalize│   └─────────────────┬──────────────────────┘
│ POST ──────────POST ───────────────────────────│
└─────┬────┘   └─────┬────┘                     │
      │              │                           │
      ▼              ▼                           ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         server (:1997)                            │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    POST /api/channels/incoming                   │   │
│  │     (IncomingMessage → 202 Accepted → async processing)         │   │
│  └──────────────────────────┬──────────────────────────────────────┘   │
│                              │                                          │
│  ┌───────────────────────────▼──────────────────────────────────────┐  │
│  │                     Channel Router                                │  │
│  │                                                                   │  │
│  │   1. 去重 (processed_messages)                                    │  │
│  │   2. 用户解析 (user-resolver → users + user_channels)             │  │
│  │   3. 会话管理 (agent_sessions, DB-only)                           │  │
│  │   4. 记忆注入 (memory-manager → user_memory + facts)              │  │
│  │   5. 执行 (orchestrator-client → agent)              │  │
│  │   6. 保存消息                                                     │  │
│  │   7. 路由回复 (channel-registry → ChannelAdapter.send)            │  │
│  └───────────────────────────┬──────────────────────────────────────┘  │
│                              │                                          │
│  ┌───────────────────────────▼──────────────────────────────────────┐  │
│  │                    Channel Registry                               │  │
│  │                                                                   │  │
│  │   ┌─────────────┐  ┌─────────────┐  ┌──────────────────────┐    │  │
│  │   │FeishuAdapter│  │QiweiAdapter │  │  WebuiAdapter         │    │  │
│  │   │HTTP → :1999 │  │HTTP → :2000 │  │  EventBus (in-proc)  │    │  │
│  │   └─────────────┘  └─────────────┘  └──────────────────────┘    │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                      SQLite (data/config.db)                      │  │
│  │                                                                   │  │
│  │  users · user_channels · user_memory · user_memory_facts          │  │
│  │  agent_sessions · user_binding_codes · processed_messages         │  │
│  │  models · conversations · settings                                │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                     Service Manager                               │  │
│  │    管理 orchestrator / channel-feishu / channel-qiwei 子进程生命周期       │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
                 ┌────────────────────────┐
                 │  agent    │
                 │  (:1996)              │
                 │  Claude / LLM 执行引擎 │
                 └────────────────────────┘
```

---

## 数据模型

### users — 统一用户

| 列 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | UUID |
| name | TEXT | 显示名 |
| metadata | TEXT (JSON) | 扩展信息，如 `{ isShadow: true }` |
| createdAt | TEXT | ISO 8601 创建时间 |
| updatedAt | TEXT | ISO 8601 更新时间 |

### user_channels — 渠道绑定

| 列 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | UUID |
| userId | TEXT FK | 关联 `users.id` |
| channelType | TEXT | `feishu` / `qiwei` / `webui` |
| channelUserId | TEXT | 渠道内部用户标识 |
| displayName | TEXT | 渠道侧的用户名 |
| lastActiveAt | TEXT | 最近活跃时间 |
| createdAt | TEXT | 绑定时间 |

**唯一约束**: `(channelType, channelUserId)` — 一个渠道账号只能绑定到一个统一用户。

### user_memory — 用户记忆摘要

| 列 | 类型 | 说明 |
|---|---|---|
| userId | TEXT PK | 关联 `users.id` |
| summary | TEXT | Agent 维护的用户全局摘要 |
| updatedAt | TEXT | 最后更新时间 |

### user_memory_facts — 结构化记忆事实

| 列 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | UUID |
| userId | TEXT FK | 关联 `users.id` |
| category | TEXT | `preference` / `context` / `relationship` / `skill` |
| fact | TEXT | 事实内容 |
| sourceChannel | TEXT | 来源渠道（可选） |
| sourceSessionId | TEXT | 来源会话（可选） |
| expiresAt | TEXT | 过期时间（可选） |
| createdAt | TEXT | 创建时间 |

### agent_sessions — 会话（已增强）

| 列 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | UUID |
| title | TEXT | 会话标题 |
| userId | TEXT | 关联 `users.id` |
| sourceChannel | TEXT | 会话来源渠道 |
| messages | TEXT (JSON) | 消息数组 |
| status | TEXT | `active` / `archived` |
| createdAt | TEXT | 创建时间 |
| updatedAt | TEXT | 更新时间 |

**变更**: 移除了旧的内存 `Map` 会话存储，统一使用 SQLite。新增 `userId` 和 `sourceChannel` 列。

### user_binding_codes — 绑定码

| 列 | 类型 | 说明 |
|---|---|---|
| code | TEXT PK | 6位字母数字绑定码 |
| userId | TEXT FK | 发起绑定的用户 |
| targetChannel | TEXT | 目标渠道类型 |
| expiresAt | TEXT | 过期时间（5分钟有效） |
| usedAt | TEXT | 使用时间 |
| createdAt | TEXT | 创建时间 |

### processed_messages — 消息去重

| 列 | 类型 | 说明 |
|---|---|---|
| channelMessageId | TEXT PK | 渠道侧消息ID |
| channel | TEXT | 来源渠道 |
| processedAt | TEXT | 处理时间 |

---

## 渠道接口规范

### IncomingMessage（入站消息）

所有渠道接收到的消息都必须归一化为此格式，POST 到 `server` 的 `/api/channels/incoming` 端点。

```typescript
interface IncomingMessage {
  channel: "feishu" | "qiwei" | "webui";
  channelUserId: string;       // 渠道内部用户标识
  channelMessageId: string;    // 渠道消息ID（用于去重）
  channelConversationId?: string; // 渠道会话ID
  conversationType?: "p2p" | "group";
  messageType: "text" | "image" | "file" | "rich_text";
  content: string;
  senderName?: string;
  timestamp: number;           // 毫秒时间戳
  channelMeta?: Record<string, unknown>;
}
```

### OutgoingMessage（出站消息）

Agent 处理完毕后，通过 Channel Registry 路由回复：

```typescript
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

### ChannelAdapter（渠道适配器接口）

```typescript
interface ChannelAdapter {
  type: ChannelType;
  send(message: OutgoingMessage): Promise<void>;
  healthCheck?(): Promise<boolean>;
}
```

---

## API 参考

### 渠道消息 API

#### `POST /api/channels/incoming`
接收归一化消息，立即返回 `202 Accepted`，异步处理。

**请求体**: `IncomingMessage`

**响应**: `{ success: true, message: "Message accepted for processing" }`

#### `GET /api/channels/health`
检查所有渠道适配器的健康状态。

**响应**:
```json
{
  "success": true,
  "channels": ["feishu", "qiwei", "webui"],
  "health": { "feishu": true, "qiwei": false, "webui": true }
}
```

### 用户管理 API

#### `GET /api/users`
获取用户列表。支持按渠道查询：`?channelType=webui&channelUserId=xxx`

#### `GET /api/users/:id`
获取用户详情（含所有渠道绑定）。

#### `PATCH /api/users/:id`
更新用户显示名。

**请求体**: `{ displayName: "新名字" }`

#### `GET /api/users/:id/memory`
获取用户记忆（摘要 + 结构化事实）。

#### `POST /api/users/:id/binding-code`
生成跨渠道绑定码。

**请求体**: `{ targetChannel: "feishu" }`

**响应**: `{ code: "AB3X9K", expiresAt: "2026-02-07T12:05:00Z" }`

#### `POST /api/users/bind`
使用绑定码关联渠道。

**请求体**: `{ code: "AB3X9K", channelType: "feishu", channelUserId: "ou_xxxx", displayName: "张三" }`

#### `DELETE /api/users/:userId/channels/:channelId`
解绑渠道（至少保留一个绑定）。

### Admin 管理后台

admin 已重构为管理后台（非聊天界面），通过 REST API + TanStack Query 轮询获取数据。
Monitor 页面提供统一会话追踪视图，通过 traces API 查看每条消息的执行链路。

---

## 用户身份绑定流程

### 概念

- **影子用户（Shadow User）**：首次通过某渠道与 Agent 交流时，系统自动为该渠道账号创建一个影子用户，具备独立的 user_id 和会话。
- **手动绑定**：用户在一个渠道（如 WebUI）生成绑定码，在另一个渠道（如飞书）输入该码，将两个渠道账号关联到同一个统一用户。
- **影子用户合并**：绑定时，如果目标渠道账号已有影子用户，影子用户的数据（渠道绑定、记忆事实、会话）会自动合并到主用户。

### 绑定步骤

```
用户（WebUI）                          server                       用户（飞书）
    │                                      │                                   │
    │  1. POST /users/:id/binding-code     │                                   │
    │  { targetChannel: "feishu" }         │                                   │
    │ ──────────────────────────────────▶  │                                   │
    │                                      │                                   │
    │  ◀──── { code: "AB3X9K" }           │                                   │
    │                                      │                                   │
    │  2. 用户在飞书中发送 "绑定 AB3X9K"     │                                   │
    │                                      │  ◀───── IncomingMessage ─────────  │
    │                                      │                                   │
    │                                      │  3. POST /users/bind              │
    │                                      │  { code, channelType, userId }    │
    │                                      │                                   │
    │                                      │  4. 合并影子用户                   │
    │                                      │  5. 统一身份建立完成               │
    │                                      │                                   │
    │     此后，WebUI 和飞书共享同一个       │                                   │
    │     user_id，Agent 可以跨渠道         │                                   │
    │     访问用户的完整记忆                 │                                   │
    │                                      │                                   │
```

### 绑定码规则

- **格式**: 6位字母数字（排除 `0/O/1/I` 等易混淆字符）
- **有效期**: 5 分钟
- **一次性**: 使用后立即标记为已用
- **定向性**: 绑定码指定了目标渠道类型

---

## 记忆系统

### 结构

记忆分为两层：

1. **全局摘要（user_memory.summary）**：Agent 对用户的整体认知概述，如 "这是一位前端工程师，喜欢用 React，正在做一个 SaaS 项目"。
2. **结构化事实（user_memory_facts）**：细粒度的用户信息碎片，按类别分组：
   - `preference` — 用户偏好（如 "偏好暗色主题"）
   - `context` — 上下文信息（如 "当前项目使用 Bun.js"）
   - `relationship` — 关系信息（如 "团队有5人"）
   - `skill` — 技能信息（如 "精通 TypeScript"）

### 注入方式

在每次 Agent 执行前，Channel Router 调用 `loadMemoryContext(userId)` 生成格式化的 prompt 片段：

```
[用户背景]
这是一位前端工程师，正在开发一个 AI Agent 平台。

[已知信息]
偏好: 偏好暗色主题; 使用 Bun.js 作为运行时
上下文: 当前项目使用 React + TypeScript; 后端使用 Express
技能: 精通 TypeScript; 了解 SQLite
```

这个 prompt 片段会被前置于用户的实际消息之前，让 Agent 在每次对话中都能感知用户的全局上下文。

### 跨渠道共享

- 记忆绑定到 **统一用户 ID**（不是渠道用户 ID）
- 在飞书中积累的记忆，在 WebUI 中也能被 Agent 使用
- 绑定后的影子用户记忆会自动合并

---

## 各渠道详情

### 飞书（channel-feishu）

- **消息接收**: WebSocket 长连接 / Webhook
- **消息归一化**: 将飞书消息转为 `IncomingMessage`，POST 到 `/api/channels/incoming`
- **消息发送**: `server` 通过 Feishu Adapter 调用 `POST :1999/api/feishu/send`
- **配置项**: `feishu.app_id`, `feishu.app_secret`, `feishu.encrypt_key`, `feishu.verification_token`
- **默认端口**: 1999

### 企微（channel-qiwei）

- **消息接收**: HTTP POST 回调到 `/webhook/callback`
- **消息归一化**: 将企微消息转为 `IncomingMessage`，POST 到 `/api/channels/incoming`
- **消息发送**: `server` 通过 QiWei Adapter 调用 `POST :2000/api/qiwei/send`
- **API 特点**: 统一 `POST {baseUrl}/api/qw/doApi` 端点，`X-QIWEI-TOKEN` 认证
- **配置项**: `qiwei.token`, `qiwei.guid`, `qiwei.api_base_url`
- **默认端口**: 2000

### Admin 管理后台

- **定位**: 管理和监控面板（非聊天界面）
- **数据获取**: REST API + TanStack Query 轮询（无 SSE）
- **页面**: Monitor（会话追踪）、Agents、Models、Skills、Logs、Settings
- **用户标识**: `localStorage` 中的 UUID（`webui-{uuid}`），仅用于 API 调用标识

---

## 如何添加新渠道

### 步骤 1：定义渠道类型

在 `server/src/services/channel-types.ts` 中扩展 `ChannelType`：

```typescript
export type ChannelType = "feishu" | "qiwei" | "webui" | "your_channel";
```

### 步骤 2：创建渠道 Bot 服务

创建 `your-channel-bot/` 目录，实现以下两个端点：

1. **消息接收**（Webhook / 长连接 / 轮询）→ 归一化为 `IncomingMessage` → POST 到 `server` 的 `/api/channels/incoming`
2. **消息发送端点** `POST /api/your-channel/send` → 接收 `OutgoingMessage` → 调用渠道平台 API 发送消息

### 步骤 3：注册适配器

在 `server/src/services/channel-registry.ts` 中添加：

```typescript
function createYourChannelAdapter(): ChannelAdapter {
  const port = settingsDb.get("general.your_channel_port") || "2001";
  const baseUrl = `http://localhost:${port}`;

  return {
    type: "your_channel",
    async send(message: OutgoingMessage): Promise<void> {
      await fetch(`${baseUrl}/api/your-channel/send`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(message),
      });
    },
    async healthCheck(): Promise<boolean> {
      try {
        const response = await fetch(`${baseUrl}/api/health`);
        return response.ok;
      } catch { return false; }
    },
  };
}
```

在 `initializeAdapters()` 中调用 `registerAdapter(createYourChannelAdapter())`。

### 步骤 4：注册到 Service Manager

在 `server/src/services/service-manager.ts` 中：
1. `registerServices()` 里添加服务定义
2. `checkServiceConfig()` 里添加配置检查
3. `buildServiceEnv()` 里添加环境变量构建

### 步骤 5：添加配置

在 `server/src/routes/settings.ts` 的 `SETTING_GROUPS` 中添加渠道配置组。

### 步骤 6：更新消息路由校验

在 `server/src/routes/channels.ts` 中更新 `validChannels` 数组。

---

## 服务端口规划

| 服务 | 端口 | 配置 Key |
|---|---|---|
| agent | 1996 | `general.orchestrator_port` |
| server | 1997 | `general.server_port` |
| admin (dev) | 1998 | N/A (Vite dev server) |
| channel-feishu | 1999 | `general.feishu_port` |
| channel-qiwei | 2000 | `general.qiwei_port` |

---

## 关键文件索引

| 文件 | 作用 |
|---|---|
| `server/src/services/channel-types.ts` | 统一消息接口和适配器接口定义 |
| `server/src/services/channel-registry.ts` | 渠道适配器注册、查找、消息路由 |
| `server/src/services/channel-router.ts` | 消息处理流水线（去重→解析→执行→回复） |
| `server/src/services/user-resolver.ts` | 用户身份解析、影子用户、跨渠道绑定 |
| `server/src/services/memory-manager.ts` | 用户记忆加载、事实管理、摘要生成 |
| `server/src/services/database.ts` | SQLite 数据层（所有表的 CRUD） |
| `server/src/services/service-manager.ts` | 子服务生命周期管理 |
| `server/src/routes/channels.ts` | `/api/channels/*` 路由 |
| `server/src/routes/users.ts` | `/api/users/*` 路由 |
| `server/src/routes/agent.ts` | `/api/agent/*` 路由 |
| `server/src/routes/settings.ts` | `/api/settings/*` 配置管理路由 |
| `channel-feishu/src/index.ts` | 飞书 Bot 入口 |
| `channel-qiwei/src/index.ts` | 企微 Bot 入口 |
| `admin/src/api/client.ts` | API 基础客户端 |
| `admin/src/components/features/settings/settings-page.tsx` | 设置页面 |
