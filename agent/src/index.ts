/**
 * Agent — 自定义 ReAct 执行引擎
 *
 * 纯手写 while 循环，不依赖任何 Agent SDK：
 * - 接收 Server 派发的消息
 * - 拼上下文（systemPrompt + skills）
 * - 直接调用 LLM API（Anthropic / OpenAI 兼容）
 * - 工具调用通过 ToolRegistry 路由（Skill + MCP + Builtin）
 * - 每一步 Thought / Action / Observation 100% 可观测
 * - 回写 Server（append-only trace events）
 */

import "dotenv/config";
import express from "express";
import cors from "cors";
import processRoutes from "./routes/process";
import healthRoutes from "./routes/health";
import { createProxyRouter } from "./services/api-proxy";
import { ServerClient } from "./services/server-client";
import { cleanupInterruptedSessions, gracefulShutdown, setAgentPort } from "./engine/runner";
import type { Server } from "node:http";

const app = express();
const PORT = parseInt(process.env.AGENT_PORT || process.env.AGENT_APP_PORT || "1996", 10);
const VERSION = process.env.AGENT_VERSION || process.env.AGENT_APP_VERSION || "1.0.0";

// 告知 runner 本进程端口（非 Anthropic provider 通过本地 proxy 转换格式）
setAgentPort(PORT);

// 中间件
app.use(cors());
app.use(express.json());

// 路由
app.use(processRoutes);
app.use(healthRoutes);

// API Proxy — 非 Anthropic 兼容的 provider 通过此代理做格式转换
// Agent 引擎始终使用 Anthropic Messages API 格式，proxy 负责 Anthropic ↔ OpenAI 转换
app.use("/v1", createProxyRouter());

// HTTP Server 引用（用于 shutdown 时关闭）
let httpServer: Server;

// 启动
httpServer = app.listen(PORT, async () => {
  console.log(`
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║   Agent is running!                                   ║
║                                                           ║
║   Local:    http://localhost:${PORT}                         ║
║   Version:  ${VERSION}                                       ║
║   Health:   http://localhost:${PORT}/health                  ║
║   Engine:   ReAct Loop (custom)                              ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
  `);

  // 启动后向 Server 注册自己
  try {
    const server = new ServerClient();
    const agentId = `agent-${PORT}`;
    await server.register(agentId, `http://localhost:${PORT}`, VERSION);
    console.log(`Registered with Server as "${agentId}"`);

    // 启动心跳
    heartbeatTimer = setInterval(async () => {
      try {
        await server.heartbeat(agentId);
      } catch {
        // 静默：Server 可能暂时不可用
      }
    }, 30_000);

  } catch {
    console.log("Could not register with Server (may not be ready yet)");
  }

  // 清理中断的 session（标记为 completed，消息已通过 append-only 持久化）
  try {
    const cleaned = await cleanupInterruptedSessions();
    if (cleaned > 0) {
      console.log(`Cleaned up ${cleaned} interrupted session(s)`);
    }
  } catch (err) {
    console.log("Could not cleanup interrupted sessions:", err);
  }
});

// ==================== 优雅退出 ====================

let heartbeatTimer: ReturnType<typeof setInterval> | undefined;
let isShuttingDown = false;

async function shutdown(signal: string): Promise<void> {
  if (isShuttingDown) return;
  isShuttingDown = true;

  console.log(`[agent] ${signal} received, starting graceful shutdown...`);

  // 1. 停止心跳
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer);
    heartbeatTimer = undefined;
  }

  // 2. 停止接受新连接
  httpServer?.close((err) => {
    if (err) {
      console.warn("[agent] Error closing HTTP server:", err);
    } else {
      console.log("[agent] HTTP server closed");
    }
  });

  // 3. 终止所有 Agent Loop，等待 worker 退出
  try {
    await gracefulShutdown();
  } catch (err) {
    console.error("[agent] Error during engine shutdown:", err);
  }

  console.log("[agent] Shutdown complete, exiting.");
  process.exit(0);
}

process.on("SIGTERM", () => void shutdown("SIGTERM"));
process.on("SIGINT", () => void shutdown("SIGINT"));
