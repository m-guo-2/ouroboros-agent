# Moli - 产品需求文档 (PRD)

> 基于 Claude Agent SDK 的自演化 AI Agent 平台

---

## 📋 文档信息

| 项目 | 内容 |
|------|------|
| 项目名称 | Moli Agent |
| 版本 | v1.2.0 |
| 更新日期 | 2024-12-30 |
| 状态 | 架构设计阶段 |

---

## 🎯 产品愿景

构建一个**自演化的 AI Agent 平台**，能够：
1. 拥有完整的系统操作能力（文件、代码、命令）
2. 支持多模型切换（通过流量劫持）
3. 提供流式对话体验
4. **可以修改自身业务代码实现能力增强（自举）**
5. **支持热更新，业务层可被 Agent 修改和重启**

**核心理念**：Moli 是一个让 AI Agent 在真实沟通环境中持续工作、协作与演进的平台。

---

## 🏗️ 自举架构 (Bootstrap Architecture)

### 设计理念

采用**执行引擎 + 业务控制器**分离的架构：

- **Orchestrator** = 执行者（手）— 有 Agent 能力，执行实际操作
- **Server** = 指挥者（脑）— 做业务决策，向 Orchestrator 下发指令

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         自举架构 (Bootstrap Architecture)                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   ┌───────────────────────────────────────────────────────────────┐    │
│   │           agent (Agent 执行引擎)                  │    │
│   │                         Port: 1996                             │    │
│   ├───────────────────────────────────────────────────────────────┤    │
│   │                                                                │    │
│   │   核心能力：                                                    │    │
│   │   ┌─────────────────┐  ┌─────────────────┐                    │    │
│   │   │ Claude Agent SDK│  │   API Proxy     │                    │    │
│   │   │ 完整工具封装    │  │   流量劫持      │                    │    │
│   │   │ 执行实际操作    │  │   Port: 1998    │                    │    │
│   │   └─────────────────┘  └─────────────────┘                    │    │
│   │                                                                │    │
│   │   ┌─────────────────┐  ┌─────────────────┐                    │    │
│   │   │   进程管理      │  │   配置管理       │                    │    │
│   │   │   启停 server   │  │   环境变量       │                    │    │
│   │   │   健康监控      │  │   热更新配置     │                    │    │
│   │   └─────────────────┘  └─────────────────┘                    │    │
│   │                                                                │    │
│   │   特点：                                                       │    │
│   │   • 稳定不变，不会自我更新                                      │    │
│   │   • 接收指令并执行（被动执行者）                                │    │
│   │   • 可以修改 server 代码                                 │    │
│   │   • 可以重启 server                                      │    │
│   │                                                                │    │
│   └───────────────────────────────────────────────────────────────┘    │
│                              ▲                                         │
│                              │                                         │
│                    下发指令（调用 Agent API）                           │
│                    "帮我修改 xxx.ts"                                    │
│                    "重启 server"                                        │
│                              │                                         │
│   ┌───────────────────────────────────────────────────────────────┐    │
│   │           server (业务控制器)                            │    │
│   │                         Port: 1997                             │    │
│   ├───────────────────────────────────────────────────────────────┤    │
│   │                                                                │    │
│   │   核心能力：                                                    │    │
│   │   ┌─────────────────┐  ┌─────────────────┐                    │    │
│   │   │   业务逻辑      │  │   对话 API      │                    │    │
│   │   │   决策中心      │  │   面向用户      │                    │    │
│   │   │   任务规划      │  │   SSE 流式      │                    │    │
│   │   └─────────────────┘  └─────────────────┘                    │    │
│   │                                                                │    │
│   │   ┌─────────────────┐  ┌─────────────────┐                    │    │
│   │   │   会话管理      │  │  Orchestrator   │                    │    │
│   │   │   历史记录      │  │   客户端        │                    │    │
│   │   │   持久化        │  │   下发指令      │                    │    │
│   │   └─────────────────┘  └─────────────────┘                    │    │
│   │                                                                │    │
│   │   特点：                                                       │    │
│   │   • 承载所有业务逻辑                                           │    │
│   │   • 决定让 Agent 做什么（主动指挥者）                           │    │
│   │   • 可以被 orchestrator 修改和重启                             │    │
│   │   • 支持热更新 / 重新编译                                      │    │
│   │                                                                │    │
│   └───────────────────────────────────────────────────────────────┘    │
│                              ▲                                         │
│                              │                                         │
│                         用户对话                                        │
│                              │                                         │
│   ┌───────────────────────────────────────────────────────────────┐    │
│   │                    admin (前端)                            │    │
│   │                         Port: 5173                             │    │
│   ├───────────────────────────────────────────────────────────────┤    │
│   │   • 对话 UI                                                    │    │
│   │   • 模型选择（通过 server → orchestrator 配置）                 │    │
│   │   • 工具调用展示                                                │    │
│   │   • 会话管理                                                    │    │
│   └───────────────────────────────────────────────────────────────┘    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 自举流程

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           自举流程示例                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   场景：用户说 "给我加一个新功能：支持导出对话记录"                        │
│                                                                         │
│   ┌─────┐      ┌──────────┐      ┌──────────────┐                      │
│   │ 用户 │ ──→ │  前端    │ ──→ │ server │                       │
│   └─────┘      └──────────┘      └──────┬───────┘                      │
│                                         │                               │
│                                         │ 1. 理解需求                    │
│                                         │ 2. 规划任务                    │
│                                         │ 3. 构造指令                    │
│                                         ▼                               │
│                              ┌──────────────────────┐                   │
│                              │ 调用 Orchestrator API │                   │
│                              │                      │                   │
│                              │ POST /api/agent/chat │                   │
│                              │ {                    │                   │
│                              │   "message": "请修改  │                   │
│                              │   server/src/  │                   │
│                              │   routes/export.ts， │                   │
│                              │   添加导出功能..."    │                   │
│                              │ }                    │                   │
│                              └──────────┬───────────┘                   │
│                                         │                               │
│                                         ▼                               │
│                              ┌──────────────────────┐                   │
│                              │ agent   │                   │
│                              │                      │                   │
│                              │ Claude Agent SDK 执行 │                   │
│                              │ • Read 现有代码       │                   │
│                              │ • Edit 添加新功能     │                   │
│                              │ • Write 保存文件      │                   │
│                              └──────────┬───────────┘                   │
│                                         │                               │
│                                         │ 4. 代码修改完成                 │
│                                         ▼                               │
│                              ┌──────────────────────┐                   │
│                              │ 调用 Orchestrator API │                   │
│                              │                      │                   │
│                              │ POST /api/process/   │                   │
│                              │       restart-server │                   │
│                              └──────────┬───────────┘                   │
│                                         │                               │
│                                         ▼                               │
│                              ┌──────────────────────┐                   │
│                              │ agent   │                   │
│                              │                      │                   │
│                              │ 5. 重启 server │                   │
│                              │    (重新编译、启动)   │                   │
│                              └──────────┬───────────┘                   │
│                                         │                               │
│                                         ▼                               │
│                              ┌──────────────────────┐                   │
│                              │ server (新版)  │                   │
│                              │                      │                   │
│                              │ 6. 新功能已生效 ✅    │                   │
│                              └──────────────────────┘                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 职责划分对照表

| 能力 | agent (执行引擎) | server (业务控制器) |
|------|:-----------------------------:|:-------------------------:|
| **Claude Agent SDK** | ✓ 完整封装，执行实际操作 | ✗ 无（通过 API 调用 orchestrator） |
| **工具集** | 全部工具 (claude_code preset) | ✗ |
| **API Proxy** | ✓ 流量劫持，多模型支持 | ✗ |
| **进程管理** | ✓ 管理 server 生命周期 | ✗ |
| **对话 API** | ✓ 接收指令的 Chat API | ✓ 面向用户的 Chat API |
| **业务逻辑** | ✗ 纯执行，不做决策 | ✓ 所有业务决策 |
| **会话管理** | ✗ | ✓ 用户会话、历史记录 |
| **任务规划** | ✗ | ✓ 理解需求、规划任务 |
| **配置管理** | ✓ 环境变量、模型配置 | ✗ 从 orchestrator 获取 |
| **可被修改** | ✗ 稳定不变 | ✓ 可被 orchestrator 修改 |
| **重启频率** | 极少 | 可频繁（热更新） |

### 关键设计点

| 问题 | 解决方案 |
|------|----------|
| Agent 能力在哪里？ | 在 orchestrator，server 通过 API 调用它 |
| 谁做决策？ | Server 做业务决策，orchestrator 只执行 |
| 代码怎么更新？ | Server 指令 orchestrator 修改代码并重启 server |
| 稳定性如何保证？ | Orchestrator 不变，只有 server 会被修改 |

---

## 📚 Claude Agent SDK 能力概览

### 核心工具集 (Claude Code Preset)

| 工具 | 用途 | 详情 |
|------|------|------|
| **Bash** | Shell 命令执行 | 执行任意命令，支持后台进程 |
| **Read** | 文件读取 | 支持文本、图片、PDF、Jupyter Notebook |
| **Write** | 文件写入 | 创建或覆盖文件 |
| **Edit** | 精确编辑 | 字符串替换，保持文件其他部分不变 |
| **Grep** | 代码搜索 | 基于 ripgrep，支持正则表达式 |
| **Glob** | 文件匹配 | 按模式查找文件 |
| **Task** | 子任务 | 启动子 Agent 处理复杂任务 |
| **WebFetch** | 网页获取 | 获取网页内容 |
| **WebSearch** | 网页搜索 | 搜索互联网 |
| **Skill** | 技能调用 | 调用预定义的技能 |

### 权限模式

| 模式 | 说明 | 使用场景 |
|------|------|----------|
| `default` | 标准权限检查 | 生产环境 |
| `acceptEdits` | 自动批准文件编辑 | 开发环境 |
| `bypassPermissions` | 完全绕过权限 | 自举操作（需信任） |

### API 端点配置（多模型支持）

**关键限制**：Claude Agent SDK **原生只支持 Claude 模型**

```bash
# 通过环境变量配置 API 端点
export ANTHROPIC_BASE_URL="http://localhost:1998/v1"
```

→ 要使用其他模型，**必须通过 API Proxy 转换格式**

---

## 🔄 API 格式转换（流量劫持）

### 转换流程

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           API 格式转换流程                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   agent 内的 Claude Agent SDK                              │
│        │                                                                │
│        │ Anthropic Messages API                                         │
│        │ POST /v1/messages                                              │
│        ▼                                                                │
│   ┌──────────────────────────────────────────────────────────────┐     │
│   │           API Proxy (Port 1998)                               │     │
│   │                                                               │     │
│   │   convertAnthropicToOpenAI()                                  │     │
│   │   • system prompt 提取                                        │     │
│   │   • messages 格式转换                                         │     │
│   │   • tools → functions 转换                                    │     │
│   │                                                               │     │
│   └──────────────────────────────────────────────────────────────┘     │
│        │                                                                │
│        │ OpenAI Chat Completions API                                   │
│        ▼                                                                │
│   目标模型 (百川/OpenAI/DeepSeek/通义)                                  │
│        │                                                                │
│        │ 响应                                                           │
│        ▼                                                                │
│   ┌──────────────────────────────────────────────────────────────┐     │
│   │   convertOpenAIToAnthropic()                                  │     │
│   │   • content 格式转换                                          │     │
│   │   • tool_calls → tool_use 转换                                │     │
│   └──────────────────────────────────────────────────────────────┘     │
│        │                                                                │
│        ▼                                                                │
│   Claude Agent SDK (正常处理)                                           │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 支持的模型

| Provider | 模型 | 特点 |
|----------|------|------|
| Claude (原生) | claude-sonnet-4-5 | 官方模型，能力最强 |
| 百川 | Baichuan4-Turbo | 国产模型，成本低 |
| OpenAI | gpt-4o | 综合能力强 |
| DeepSeek | deepseek-chat | 代码能力强 |
| 通义 | qwen-max | 阿里模型 |

---

## 📋 需求清单

### P0 - 核心功能（必须实现）

#### 1. Orchestrator 作为执行引擎

**目标**：Orchestrator 运行完整的 Claude Agent SDK，提供执行 API

| API | 说明 |
|-----|------|
| `POST /api/agent/chat` | 接收指令，执行 Agent 操作 |
| `POST /api/agent/chat/stream` | 流式执行（SSE） |
| `POST /api/agent/interrupt` | 中断当前执行 |
| `GET /api/agent/session` | 获取会话状态 |

#### 2. Server 作为业务控制器

**目标**：Server 承载业务逻辑，通过调用 Orchestrator 完成 Agent 操作

```typescript
// server 调用 orchestrator
const orchestratorClient = {
  async executeTask(instruction: string): Promise<AgentResult> {
    const response = await fetch("http://localhost:1996/api/agent/chat", {
      method: "POST",
      body: JSON.stringify({ message: instruction }),
    });
    return response.json();
  },
  
  async restartSelf(): Promise<void> {
    await fetch("http://localhost:1996/api/process/restart-server", {
      method: "POST",
    });
  }
};
```

#### 3. 前端对话连接 Server

**目标**：前端通过 Server 的对话 API 与用户交互

| 改动点 | 说明 |
|--------|------|
| Chat API | 前端 → Server `/api/chat/stream` |
| 模型配置 | Server → Orchestrator `/api/proxy/configure` |

#### 4. 模型切换 UI

**目标**：在前端提供模型选择

```
用户选择模型 → Server → Orchestrator 配置 Proxy → 后续执行使用新模型
```

### P1 - 重要功能

#### 5. 工具调用可视化

**目标**：前端展示 Orchestrator 执行的工具调用

```
🤖 正在执行...
  📂 Glob: **/*.ts (找到 23 个文件)
  📖 Read: src/index.ts
  ✏️ Edit: 修改了 3 行
  ⚙️ Bash: bun run build
✅ 完成
```

#### 6. 会话管理

| 功能 | 说明 |
|------|------|
| 用户会话 | Server 管理用户对话历史 |
| Agent 会话 | Orchestrator 管理 Agent session_id |
| 持久化 | 支持断点续传 |

#### 7. 进程管理完善

| 功能 | 说明 |
|------|------|
| 启动 server | Orchestrator 可启动 server |
| 停止 server | Orchestrator 可停止 server |
| 重启 server | Orchestrator 可重启 server（重新编译） |
| 健康检查 | Orchestrator 监控 server 状态 |

### P2 - 增强功能

#### 8. 热更新支持

- 代码修改后自动重新编译
- 最小化停机时间

#### 9. 操作审计日志

| 字段 | 说明 |
|------|------|
| timestamp | 时间戳 |
| source | 指令来源（哪个 server） |
| instruction | 执行的指令 |
| tools_used | 使用的工具 |
| result | 执行结果 |

#### 10. 安全边界

```typescript
{
  allowedDirectories: ["/project/server"],  // 只能修改 server
  disallowedPaths: ["/project/agent"],  // 不能修改 orchestrator
  disallowedCommands: ["rm -rf /", "sudo"],
}
```

---

## 📐 实施计划

### 阶段一：执行引擎搭建（2-3 天）

- [ ] Orchestrator 完整 Claude Agent SDK 封装
- [ ] Orchestrator 提供 Agent 执行 API
- [ ] Orchestrator 提供进程管理 API
- [ ] API Proxy 集成

### 阶段二：业务控制器开发（2 天）

- [ ] Server 实现 Orchestrator 客户端
- [ ] Server 实现用户对话 API
- [ ] Server 实现业务逻辑（任务规划）
- [ ] 会话管理

### 阶段三：前端适配（1-2 天）

- [ ] 前端连接 Server 对话 API
- [ ] 模型选择 UI
- [ ] 工具调用展示

### 阶段四：自举验证（1 天）

- [ ] 测试通过对话修改 Server 代码
- [ ] 测试重启 Server
- [ ] 验证新功能生效

---

## 📊 验收标准

### 架构验收

| 验收点 | 标准 |
|--------|------|
| Orchestrator 稳定性 | 运行期间不会自我修改 |
| Server 可更新 | 可通过 Orchestrator 修改和重启 |
| 职责分离 | Orchestrator 只执行，Server 做决策 |

### 功能验收

| 功能点 | 验收标准 |
|--------|----------|
| 基础对话 | 用户 → Server → 响应 |
| Agent 执行 | Server → Orchestrator → 执行 → 返回结果 |
| 自举 | 用户请求 → Server 规划 → Orchestrator 修改代码 → 重启 → 生效 |

### 性能指标

| 指标 | 目标 |
|------|------|
| 首字延迟 | < 2s |
| Server 重启时间 | < 5s（含编译） |
| API 调用延迟 | < 100ms |

---

## 📎 附录

### A. 端口分配

| 端口 | 服务 | 职责 |
|------|------|------|
| 1996 | agent | 执行引擎：Agent SDK、进程管理、API Proxy |
| 1997 | server | 业务控制器：对话 API、业务逻辑 |
| 1998 | api-proxy | API 格式转换（由 orchestrator 管理） |
| 5173 | admin | 前端开发服务器 |

### B. 相关文件

| 文件 | 说明 |
|------|------|
| `agent/src/services/claude-agent.ts` | Claude Agent SDK 封装 |
| `agent/src/services/api-proxy.ts` | API 代理服务 |
| `agent/src/services/process-manager.ts` | 进程管理 |
| `agent/src/routes/agent.ts` | Agent 执行 API |
| `server/src/services/orchestrator-client.ts` | Orchestrator 客户端 |
| `server/src/routes/chat.ts` | 用户对话 API |
| `admin/src/api/client.ts` | 前端 API 客户端 |

### C. 参考文档

- [Claude Agent SDK 官方文档](https://docs.anthropic.com/en/docs/agent-sdk/overview)
- [Anthropic Messages API](https://docs.anthropic.com/en/api/messages)
- [OpenAI Chat Completions API](https://platform.openai.com/docs/api-reference/chat)
