# 实施计划

> 基于自举架构的 Moli Agent 实施方案

---

## 🎯 目标

**核心改造**：实现执行引擎（orchestrator）+ 业务控制器（server）分离

```
架构目标：

┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│   agent (执行引擎)                                 │
│   ├── Claude Agent SDK（完整工具）                               │
│   ├── 执行 API（接收指令，执行操作）                              │
│   ├── 进程管理（启停 server）                                    │
│   └── API Proxy（流量劫持）                                      │
│                          ▲                                      │
│                          │ 下发指令                              │
│                          │                                      │
│   server (业务控制器)                                      │
│   ├── 用户对话 API（面向前端）                                    │
│   ├── 业务逻辑（任务规划、决策）                                  │
│   ├── Orchestrator 客户端（调用执行引擎）                         │
│   └── 会话管理                                                  │
│                          ▲                                      │
│                          │ 用户对话                              │
│                          │                                      │
│   admin (前端)                                              │
│   └── 对话 UI、模型选择、工具展示                                 │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 📋 任务分解

### Phase 1: Orchestrator - 执行引擎

#### 任务 1.1: Claude Agent SDK 封装

**文件**: `agent/src/services/claude-agent.ts`

```typescript
import { query } from "@anthropic-ai/claude-agent-sdk";

const SYSTEM_PROMPT = `你是 Moli Agent 的执行引擎。

你接收来自业务控制器的指令并执行。你拥有完整的系统操作能力：
- 读写文件、编辑代码
- 执行 Shell 命令
- 搜索代码库

你可以修改 server 的代码，但不要修改 agent。

执行完成后，清晰地汇报结果。`;

export class ClaudeAgentService {
  private sessionId: string | null = null;
  private abortController: AbortController | null = null;
  
  /**
   * 执行指令（流式）
   */
  async *execute(instruction: string): AsyncGenerator<AgentEvent> {
    this.abortController = new AbortController();
    
    const q = query({
      prompt: instruction,
      options: {
        tools: { type: "preset", preset: "claude_code" },
        systemPrompt: { type: "preset", preset: "claude_code", append: SYSTEM_PROMPT },
        permissionMode: "bypassPermissions",
        allowDangerouslySkipPermissions: true,
        cwd: PROJECT_ROOT,
        additionalDirectories: [PROJECT_ROOT],
        abortController: this.abortController,
        ...(this.sessionId ? { resume: this.sessionId } : {}),
      },
    });
    
    for await (const msg of q) {
      // 会话初始化
      if (msg.type === 'system' && msg.subtype === 'init') {
        this.sessionId = msg.session_id;
        yield { type: 'session', sessionId: msg.session_id };
      }
      
      // 助手消息（思考过程、回复）
      if (msg.type === 'assistant') {
        const text = this.extractText(msg.message.content);
        if (text) {
          yield { type: 'content', text };
        }
        
        // 工具调用
        for (const block of msg.message.content) {
          if (block.type === 'tool_use') {
            yield { type: 'tool_call', tool: block.name, input: block.input };
          }
        }
      }
      
      // 工具结果
      if (msg.type === 'tool_result') {
        yield { type: 'tool_result', tool: msg.tool_name, result: msg.result };
      }
      
      // 最终结果
      if (msg.type === 'result') {
        yield {
          type: 'done',
          success: msg.subtype === 'success',
          result: msg.result,
          usage: msg.usage,
        };
      }
    }
  }
  
  /**
   * 执行指令（非流式，等待完成）
   */
  async executeSync(instruction: string): Promise<AgentResult> {
    const events: AgentEvent[] = [];
    let result: AgentResult = { success: false };
    
    for await (const event of this.execute(instruction)) {
      events.push(event);
      if (event.type === 'done') {
        result = {
          success: event.success,
          result: event.result,
          usage: event.usage,
          events,
        };
      }
    }
    
    return result;
  }
  
  interrupt(): void {
    this.abortController?.abort();
  }
  
  resetSession(): void {
    this.sessionId = null;
  }
  
  private extractText(content: any[]): string {
    return content
      .filter(b => b.type === 'text')
      .map(b => b.text)
      .join('');
  }
}

// 类型定义
export type AgentEvent =
  | { type: 'session'; sessionId: string }
  | { type: 'content'; text: string }
  | { type: 'tool_call'; tool: string; input: object }
  | { type: 'tool_result'; tool: string; result: any }
  | { type: 'done'; success: boolean; result?: string; usage?: object };

export interface AgentResult {
  success: boolean;
  result?: string;
  usage?: object;
  events?: AgentEvent[];
}
```

#### 任务 1.2: Agent 执行 API

**文件**: `agent/src/routes/agent.ts`

```typescript
import { Router } from "express";
import { ClaudeAgentService } from "../services/claude-agent";

const router = Router();
const agentService = new ClaudeAgentService();

/**
 * POST /api/agent/chat
 * 执行指令（非流式）
 */
router.post("/chat", async (req, res) => {
  const { message } = req.body;
  
  if (!message) {
    return res.status(400).json({ error: "message is required" });
  }
  
  try {
    const result = await agentService.executeSync(message);
    res.json(result);
  } catch (error) {
    res.status(500).json({ 
      success: false, 
      error: (error as Error).message 
    });
  }
});

/**
 * POST /api/agent/chat/stream
 * 执行指令（流式 SSE）
 */
router.post("/chat/stream", async (req, res) => {
  const { message } = req.body;
  
  if (!message) {
    return res.status(400).json({ error: "message is required" });
  }
  
  // 设置 SSE 头
  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");
  res.setHeader("Access-Control-Allow-Origin", "*");
  
  try {
    for await (const event of agentService.execute(message)) {
      res.write(`event: ${event.type}\n`);
      res.write(`data: ${JSON.stringify(event)}\n\n`);
    }
  } catch (error) {
    res.write(`event: error\n`);
    res.write(`data: ${JSON.stringify({ error: (error as Error).message })}\n\n`);
  }
  
  res.end();
});

/**
 * POST /api/agent/interrupt
 * 中断当前执行
 */
router.post("/interrupt", (_req, res) => {
  agentService.interrupt();
  res.json({ success: true });
});

/**
 * POST /api/agent/reset
 * 重置会话
 */
router.post("/reset", (_req, res) => {
  agentService.resetSession();
  res.json({ success: true });
});

export default router;
```

#### 任务 1.3: 进程管理服务

**文件**: `agent/src/services/process-manager.ts`

```typescript
import { spawn, ChildProcess } from "child_process";
import path from "path";

const PROJECT_ROOT = path.resolve(__dirname, "../../../");

export class ProcessManager {
  private serverProcess: ChildProcess | null = null;
  private isRestarting = false;
  
  /**
   * 启动 server
   */
  async startServer(): Promise<{ success: boolean; message: string }> {
    if (this.serverProcess) {
      return { success: false, message: "Server already running" };
    }
    
    console.log("[ProcessManager] Starting server...");
    
    this.serverProcess = spawn("bun", ["run", "dev"], {
      cwd: path.join(PROJECT_ROOT, "server"),
      stdio: ["ignore", "pipe", "pipe"],
      env: {
        ...process.env,
        ANTHROPIC_BASE_URL: "http://localhost:1998/v1",
        ORCHESTRATOR_URL: "http://localhost:1996",
      },
    });
    
    this.serverProcess.stdout?.on("data", (data) => {
      console.log(`[server] ${data}`);
    });
    
    this.serverProcess.stderr?.on("data", (data) => {
      console.error(`[server] ${data}`);
    });
    
    this.serverProcess.on("exit", (code) => {
      console.log(`[ProcessManager] server exited with code ${code}`);
      this.serverProcess = null;
      
      // 非正常退出且非重启中，自动重启
      if (code !== 0 && !this.isRestarting) {
        console.log("[ProcessManager] Auto-restarting server...");
        setTimeout(() => this.startServer(), 1000);
      }
    });
    
    // 等待启动
    await this.waitForServer();
    
    return { success: true, message: "Server started" };
  }
  
  /**
   * 停止 server
   */
  async stopServer(): Promise<{ success: boolean; message: string }> {
    if (!this.serverProcess) {
      return { success: false, message: "Server not running" };
    }
    
    console.log("[ProcessManager] Stopping server...");
    
    return new Promise((resolve) => {
      this.serverProcess!.once("exit", () => {
        this.serverProcess = null;
        resolve({ success: true, message: "Server stopped" });
      });
      
      this.serverProcess!.kill("SIGTERM");
      
      // 超时强制杀死
      setTimeout(() => {
        if (this.serverProcess) {
          this.serverProcess.kill("SIGKILL");
        }
      }, 5000);
    });
  }
  
  /**
   * 重启 server（重新编译）
   */
  async restartServer(): Promise<{ success: boolean; message: string }> {
    console.log("[ProcessManager] Restarting server...");
    
    this.isRestarting = true;
    
    try {
      await this.stopServer();
      await new Promise(resolve => setTimeout(resolve, 1000));
      const result = await this.startServer();
      return { success: true, message: "Server restarted" };
    } finally {
      this.isRestarting = false;
    }
  }
  
  /**
   * 获取 server 状态
   */
  getServerStatus(): { running: boolean; pid?: number } {
    return {
      running: this.serverProcess !== null,
      pid: this.serverProcess?.pid,
    };
  }
  
  /**
   * 等待 server 启动
   */
  private async waitForServer(timeout = 30000): Promise<void> {
    const start = Date.now();
    
    while (Date.now() - start < timeout) {
      try {
        const res = await fetch("http://localhost:1997/health");
        if (res.ok) {
          console.log("[ProcessManager] server is ready");
          return;
        }
      } catch {
        // 继续等待
      }
      await new Promise(resolve => setTimeout(resolve, 500));
    }
    
    console.warn("[ProcessManager] Timeout waiting for server");
  }
}
```

#### 任务 1.4: 进程管理 API

**文件**: `agent/src/routes/process.ts`

```typescript
import { Router } from "express";
import { processManager } from "../services/process-manager";

const router = Router();

/**
 * GET /api/process/status
 * 获取服务状态
 */
router.get("/status", (_req, res) => {
  res.json({
    orchestrator: { running: true },
    server: processManager.getServerStatus(),
  });
});

/**
 * POST /api/process/start-server
 * 启动 server
 */
router.post("/start-server", async (_req, res) => {
  const result = await processManager.startServer();
  res.json(result);
});

/**
 * POST /api/process/stop-server
 * 停止 server
 */
router.post("/stop-server", async (_req, res) => {
  const result = await processManager.stopServer();
  res.json(result);
});

/**
 * POST /api/process/restart-server
 * 重启 server（重新编译）
 */
router.post("/restart-server", async (_req, res) => {
  const result = await processManager.restartServer();
  res.json(result);
});

export default router;
```

---

### Phase 2: Server - 业务控制器

#### 任务 2.1: Orchestrator 客户端

**文件**: `server/src/services/orchestrator-client.ts`

```typescript
const ORCHESTRATOR_URL = process.env.ORCHESTRATOR_URL || "http://localhost:1996";

export interface AgentEvent {
  type: string;
  [key: string]: any;
}

export interface AgentResult {
  success: boolean;
  result?: string;
  usage?: object;
  events?: AgentEvent[];
}

/**
 * Orchestrator 客户端
 * 用于向执行引擎下发指令
 */
export const orchestratorClient = {
  /**
   * 执行指令（非流式）
   */
  async execute(instruction: string): Promise<AgentResult> {
    const response = await fetch(`${ORCHESTRATOR_URL}/api/agent/chat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: instruction }),
    });
    
    if (!response.ok) {
      throw new Error(`Orchestrator error: ${response.statusText}`);
    }
    
    return response.json();
  },
  
  /**
   * 执行指令（流式）
   */
  async *executeStream(instruction: string): AsyncGenerator<AgentEvent> {
    const response = await fetch(`${ORCHESTRATOR_URL}/api/agent/chat/stream`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: instruction }),
    });
    
    if (!response.ok) {
      throw new Error(`Orchestrator error: ${response.statusText}`);
    }
    
    const reader = response.body?.getReader();
    if (!reader) throw new Error("No response body");
    
    const decoder = new TextDecoder();
    let buffer = "";
    
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";
      
      for (const line of lines) {
        if (line.startsWith("data: ")) {
          try {
            const data = JSON.parse(line.slice(6));
            yield data as AgentEvent;
          } catch {}
        }
      }
    }
  },
  
  /**
   * 中断执行
   */
  async interrupt(): Promise<void> {
    await fetch(`${ORCHESTRATOR_URL}/api/agent/interrupt`, {
      method: "POST",
    });
  },
  
  /**
   * 配置模型
   */
  async configureModel(config: { provider: string; apiKey?: string; model?: string }): Promise<void> {
    await fetch(`${ORCHESTRATOR_URL}/api/proxy/configure`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(config),
    });
  },
  
  /**
   * 请求重启自己
   */
  async restartSelf(): Promise<void> {
    await fetch(`${ORCHESTRATOR_URL}/api/process/restart-server`, {
      method: "POST",
    });
  },
};
```

#### 任务 2.2: 用户对话 API

**文件**: `server/src/routes/chat.ts`

```typescript
import { Router } from "express";
import { orchestratorClient, AgentEvent } from "../services/orchestrator-client";

const router = Router();

// 简单的会话存储（实际应使用数据库）
const sessions = new Map<string, { messages: any[] }>();

/**
 * POST /api/chat/stream
 * 用户对话（流式）
 */
router.post("/stream", async (req, res) => {
  const { message, sessionId = "default" } = req.body;
  
  if (!message) {
    return res.status(400).json({ error: "message is required" });
  }
  
  // 获取或创建会话
  if (!sessions.has(sessionId)) {
    sessions.set(sessionId, { messages: [] });
  }
  const session = sessions.get(sessionId)!;
  
  // 记录用户消息
  session.messages.push({ role: "user", content: message });
  
  // 设置 SSE 头
  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");
  res.setHeader("Access-Control-Allow-Origin", "*");
  
  try {
    // 构造给 Agent 的指令
    // 这里可以加入更多业务逻辑：任务规划、上下文注入等
    const instruction = buildInstruction(message, session.messages);
    
    let assistantContent = "";
    const toolCalls: any[] = [];
    
    // 调用 Orchestrator 执行
    for await (const event of orchestratorClient.executeStream(instruction)) {
      // 转发事件给前端
      res.write(`event: ${event.type}\n`);
      res.write(`data: ${JSON.stringify(event)}\n\n`);
      
      // 收集内容
      if (event.type === "content") {
        assistantContent += event.text;
      }
      if (event.type === "tool_call") {
        toolCalls.push({ tool: event.tool, input: event.input });
      }
    }
    
    // 记录助手消息
    session.messages.push({
      role: "assistant",
      content: assistantContent,
      toolCalls,
    });
    
  } catch (error) {
    res.write(`event: error\n`);
    res.write(`data: ${JSON.stringify({ error: (error as Error).message })}\n\n`);
  }
  
  res.end();
});

/**
 * POST /api/chat/interrupt
 * 中断执行
 */
router.post("/interrupt", async (_req, res) => {
  await orchestratorClient.interrupt();
  res.json({ success: true });
});

/**
 * GET /api/chat/sessions/:id
 * 获取会话历史
 */
router.get("/sessions/:id", (req, res) => {
  const session = sessions.get(req.params.id);
  if (!session) {
    return res.status(404).json({ error: "Session not found" });
  }
  res.json(session);
});

/**
 * DELETE /api/chat/sessions/:id
 * 清除会话
 */
router.delete("/sessions/:id", (req, res) => {
  sessions.delete(req.params.id);
  res.json({ success: true });
});

/**
 * 构造指令
 * 这里可以加入复杂的业务逻辑
 */
function buildInstruction(userMessage: string, history: any[]): string {
  // 简单版：直接传递用户消息
  // 实际可以：分析意图、注入上下文、任务分解等
  return userMessage;
}

export default router;
```

#### 任务 2.3: 模型配置 API

**文件**: `server/src/routes/models.ts`

```typescript
import { Router } from "express";
import { orchestratorClient } from "../services/orchestrator-client";

const router = Router();

/**
 * GET /api/models
 * 获取支持的模型列表
 */
router.get("/", async (_req, res) => {
  // 从 orchestrator 获取
  try {
    const response = await fetch("http://localhost:1996/api/proxy/models");
    const data = await response.json();
    res.json(data);
  } catch (error) {
    res.status(500).json({ error: "Failed to fetch models" });
  }
});

/**
 * POST /api/models/configure
 * 配置当前模型
 */
router.post("/configure", async (req, res) => {
  const { provider, apiKey, model } = req.body;
  
  try {
    await orchestratorClient.configureModel({ provider, apiKey, model });
    res.json({ success: true });
  } catch (error) {
    res.status(500).json({ error: (error as Error).message });
  }
});

export default router;
```

---

### Phase 3: 前端适配

#### 任务 3.1: API 客户端

**文件**: `admin/src/api/client.ts`

```typescript
const SERVER_BASE = "http://localhost:1997/api";

export interface AgentEvent {
  type: string;
  text?: string;
  tool?: string;
  input?: object;
  result?: any;
  success?: boolean;
  [key: string]: any;
}

/**
 * 流式对话
 */
export async function streamChat(
  message: string,
  sessionId: string,
  onEvent: (event: AgentEvent) => void,
  onError?: (error: Error) => void
): Promise<void> {
  try {
    const response = await fetch(`${SERVER_BASE}/chat/stream`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message, sessionId }),
    });

    const reader = response.body?.getReader();
    if (!reader) throw new Error("No response body");

    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (line.startsWith("data: ")) {
          try {
            const data = JSON.parse(line.slice(6));
            onEvent(data as AgentEvent);
          } catch {}
        }
      }
    }
  } catch (error) {
    onError?.(error instanceof Error ? error : new Error(String(error)));
  }
}

/**
 * 中断执行
 */
export async function interruptChat(): Promise<void> {
  await fetch(`${SERVER_BASE}/chat/interrupt`, { method: "POST" });
}

/**
 * 获取模型列表
 */
export async function getModels(): Promise<{ models: Model[] }> {
  const response = await fetch(`${SERVER_BASE}/models`);
  return response.json();
}

/**
 * 配置模型
 */
export async function configureModel(config: {
  provider: string;
  apiKey?: string;
  model?: string;
}): Promise<void> {
  await fetch(`${SERVER_BASE}/models/configure`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
}

export interface Model {
  id: string;
  name: string;
  provider: string;
  default?: boolean;
}
```

#### 任务 3.2: 模型选择器

**文件**: `admin/src/components/ModelSelector.tsx`

```tsx
import { useState, useEffect } from "react";
import { getModels, configureModel, Model } from "../api/client";

export function ModelSelector() {
  const [models, setModels] = useState<Model[]>([]);
  const [selected, setSelected] = useState<string>("claude");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    getModels()
      .then(data => {
        setModels(data.models);
        const defaultModel = data.models.find(m => m.default);
        if (defaultModel) setSelected(defaultModel.id);
      })
      .catch(console.error);
  }, []);

  const handleSelect = async (modelId: string) => {
    setLoading(true);
    try {
      await configureModel({ provider: modelId });
      setSelected(modelId);
    } catch (error) {
      console.error("Failed to configure model:", error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="model-selector">
      <label>模型：</label>
      <select 
        value={selected} 
        onChange={e => handleSelect(e.target.value)}
        disabled={loading}
      >
        {models.map(m => (
          <option key={m.id} value={m.id}>
            {m.name}
          </option>
        ))}
      </select>
      {loading && <span className="loading">切换中...</span>}
    </div>
  );
}
```

#### 任务 3.3: 工具调用展示

**文件**: `admin/src/components/ToolCallDisplay.tsx`

```tsx
import { useState } from "react";

interface ToolCallProps {
  tool: string;
  input: object;
  result?: any;
}

const TOOL_ICONS: Record<string, string> = {
  Bash: "⚙️",
  Read: "📖",
  Write: "✏️",
  Edit: "📝",
  Grep: "🔍",
  Glob: "📂",
  Task: "🔄",
  WebFetch: "🌐",
  WebSearch: "🔎",
};

export function ToolCallDisplay({ tool, input, result }: ToolCallProps) {
  const [expanded, setExpanded] = useState(false);
  const icon = TOOL_ICONS[tool] || "🔧";
  
  return (
    <div 
      className="tool-call" 
      onClick={() => setExpanded(!expanded)}
    >
      <div className="tool-call__header">
        <span className="tool-call__icon">{icon}</span>
        <span className="tool-call__name">{tool}</span>
        <span className="tool-call__toggle">{expanded ? "▼" : "▶"}</span>
      </div>
      
      {expanded && (
        <div className="tool-call__details">
          <div className="tool-call__input">
            <strong>输入：</strong>
            <pre>{JSON.stringify(input, null, 2)}</pre>
          </div>
          {result !== undefined && (
            <div className="tool-call__result">
              <strong>输出：</strong>
              <pre>
                {typeof result === "string" 
                  ? result 
                  : JSON.stringify(result, null, 2)}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
```

---

## 📁 文件变更清单

### 新增文件

| 文件 | 说明 |
|------|------|
| `agent/src/services/claude-agent.ts` | Claude Agent SDK 封装 |
| `agent/src/routes/agent.ts` | Agent 执行 API |
| `agent/src/routes/process.ts` | 进程管理 API |
| `server/src/services/orchestrator-client.ts` | Orchestrator 客户端 |
| `server/src/routes/chat.ts` | 用户对话 API |
| `server/src/routes/models.ts` | 模型配置 API |
| `admin/src/components/ModelSelector.tsx` | 模型选择器 |
| `admin/src/components/ToolCallDisplay.tsx` | 工具调用展示 |

### 修改文件

| 文件 | 修改内容 |
|------|----------|
| `agent/src/index.ts` | 注册路由，启动时拉起 server |
| `agent/src/services/process-manager.ts` | 完善进程管理 |
| `server/src/index.ts` | 注册新路由 |
| `admin/src/api/client.ts` | 更新 API 客户端 |
| `admin/src/App.tsx` | 使用新 API |

---

## ✅ 验收检查清单

### 执行引擎验收

- [ ] Orchestrator 运行 Claude Agent SDK
- [ ] `/api/agent/chat` 能接收指令并执行
- [ ] `/api/agent/chat/stream` 能流式返回结果
- [ ] 工具调用正常（Bash、Read、Write、Edit）
- [ ] API Proxy 能切换模型

### 业务控制器验收

- [ ] Server 能调用 Orchestrator 执行指令
- [ ] `/api/chat/stream` 能与用户对话
- [ ] 会话管理正常
- [ ] 能配置模型

### 自举验收

- [ ] 用户说 "修改 xxx 代码"
- [ ] Server 向 Orchestrator 下发修改指令
- [ ] Orchestrator 执行代码修改
- [ ] Server 请求重启
- [ ] 新代码生效

### 前端验收

- [ ] 能与 Server 对话
- [ ] 能看到工具调用过程
- [ ] 能切换模型

---

## 🚀 启动命令

```bash
# 1. 启动 Orchestrator（会自动拉起 Server）
cd agent && bun run dev

# 2. 启动前端
cd admin && bun run dev

# 或分开调试
cd agent && bun run dev  # Terminal 1
cd server && bun run dev         # Terminal 2
cd admin && bun run dev            # Terminal 3
```

测试地址：
- Orchestrator: http://localhost:1996
- Server: http://localhost:1997
- 前端: http://localhost:5173

---

## 🧪 测试自举

在前端对话中输入：

```
请在 server 中添加一个 /api/ping 接口，返回当前时间
```

预期：
1. Server 收到请求，构造指令发给 Orchestrator
2. Orchestrator 的 Agent 读取代码、创建新路由
3. Server 请求 Orchestrator 重启自己
4. 新接口生效，访问 http://localhost:1997/api/ping 返回时间
