import express from "express";
import cors from "cors";
import { resolve } from "path";
import { appConfig } from "./config";
import chatRoutes from "./routes/chat";
import modelRoutes from "./routes/models";
import agentRoutes from "./routes/agent";
import agentSessionRoutes from "./routes/agent-sessions";
import settingsRoutes from "./routes/settings";
import channelRoutes from "./routes/channels";
import userRoutes from "./routes/users";
import skillRoutes from "./routes/skills";
import agentProfileRoutes from "./routes/agent-profiles";
import agentWorkspaceRoutes from "./routes/agent-workspace";
import logRoutes from "./routes/logs";
import monitorRoutes from "./routes/monitor";
import messageRoutes from "./routes/messages";
import dataRoutes from "./routes/data";
import lifecycleRoutes from "./routes/lifecycle";
import tracesRoutes from "./routes/traces";
import { serviceManager } from "./services/service-manager";
import { logger } from "./services/logger";
import { traceMiddleware } from "./services/logger/middleware";
import { initializeAdapters } from "./services/channel-registry";
import { getModelById } from "./config";
import { settingsDb, initializeDefaultAgent } from "./services/database";
import { orchestratorClient } from "./services/orchestrator-client";

const app = express();

// 中间件
app.use(cors());
app.use(express.json());

// 日志链路追踪（在所有路由之前挂载）
app.use(traceMiddleware("server"));

// API 路由
app.use("/api", chatRoutes);                    // 原有会话对话（使用模型适配器）
app.use("/api/models", modelRoutes);            // 模型管理
app.use("/api/agent", agentRoutes);             // Agent 模式（通过 Orchestrator）
app.use("/api/agent-sessions", agentSessionRoutes);  // Agent 会话存储
app.use("/api/settings", settingsRoutes);       // 统一配置管理
app.use("/api/channels", channelRoutes);         // 统一渠道入口
app.use("/api/users", userRoutes);               // 用户管理 & 绑定
app.use("/api/skills", skillRoutes);             // Skill 管理 & 分发
app.use("/api/agents", agentProfileRoutes);      // Agent Profile CRUD（多 Agent 架构）
app.use("/api/agents", agentWorkspaceRoutes);    // Agent Workspace: 笔记/任务/产出物
app.use("/api/logs", logRoutes);                  // 结构化日志查询（可观测性）
app.use("/api/monitor", monitorRoutes);            // Monitor 实时观测流
app.use("/api/messages", messageRoutes);            // 消息查询（独立消息表）
app.use("/api/data", dataRoutes);                    // Data API（供 Agent App 调用）
app.use("/api/lifecycle", lifecycleRoutes);           // Agent 生命周期管理
app.use("/api/traces", tracesRoutes);                  // 执行链路追踪（Agent 上报 + 历史查询）

// 初始化渠道适配器
initializeAdapters();

// 初始化默认 Agent（多 Agent 架构）
initializeDefaultAgent();

// ==================== 服务管理 API ====================

// 获取所有服务状态
app.get("/api/services", (_req, res) => {
  res.json({ success: true, data: serviceManager.getAllStatus() });
});

// 获取单个服务状态
app.get("/api/services/:name", (req, res) => {
  const info = serviceManager.getStatus(req.params.name);
  if (!info) {
    res.status(404).json({ success: false, error: "服务不存在" });
    return;
  }
  res.json({ success: true, data: info });
});

// 获取服务日志（内存缓冲）
app.get("/api/services/:name/logs", (req, res) => {
  const lines = parseInt(req.query.lines as string) || 50;
  const logs = serviceManager.getLogs(req.params.name, lines);
  res.json({ success: true, data: logs });
});

// 获取服务日志文件内容（持久化日志，支持尾部读取）
app.get("/api/services/:name/logfile", (req, res) => {
  const lines = parseInt(req.query.lines as string) || 100;
  const result = serviceManager.readLogFile(req.params.name, lines);
  res.json({ success: true, path: result.path, data: result.lines });
});

// SSE 实时日志流
app.get("/api/services/:name/logs/stream", (req, res) => {
  const name = req.params.name;
  const service = serviceManager.getStatus(name);
  if (!service) {
    res.status(404).json({ success: false, error: "服务不存在" });
    return;
  }

  // 禁用 Express/Node 的响应压缩（如有）
  req.headers["accept-encoding"] = "identity";

  // SSE headers
  res.writeHead(200, {
    "Content-Type": "text/event-stream",
    "Cache-Control": "no-cache, no-transform",
    "Connection": "keep-alive",
    "X-Accel-Buffering": "no",
  });
  // 立即发送 headers，确保 SSE 连接建立
  res.flushHeaders();

  /** 安全写入 + flush，确保数据立即推送到客户端 */
  const sseWrite = (data: string) => {
    try {
      res.write(data);
      // flush 确保数据不被代理/中间件缓冲
      if (typeof (res as any).flush === "function") {
        (res as any).flush();
      }
    } catch {
      // 连接可能已关闭
    }
  };

  // 先发送历史日志
  const existingLogs = serviceManager.getLogs(name, 500);
  const now = Date.now();
  for (const line of existingLogs) {
    const isStderr = line.startsWith("[stderr]");
    sseWrite(`data: ${JSON.stringify({ type: "log", content: line, timestamp: now, stream: isStderr ? "stderr" : "stdout" })}\n\n`);
  }

  // 发送连接确认
  sseWrite(`data: ${JSON.stringify({ type: "connected", service: name, timestamp: Date.now() })}\n\n`);

  // 监听新日志
  const onLog = (logLine: { timestamp: number; content: string; stream: string }) => {
    sseWrite(`data: ${JSON.stringify({ type: "log", ...logLine })}\n\n`);
  };

  // 监听服务状态变化
  const onStatus = (status: string) => {
    sseWrite(`data: ${JSON.stringify({ type: "status", status, timestamp: Date.now() })}\n\n`);
  };

  serviceManager.on(`log:${name}`, onLog);
  serviceManager.on(`status:${name}`, onStatus);

  // 心跳保活（也兼作连接活性检测）
  const heartbeat = setInterval(() => {
    try {
      res.write(`: heartbeat\n\n`);
      if (typeof (res as any).flush === "function") {
        (res as any).flush();
      }
    } catch {
      cleanup();
    }
  }, 15000);

  const cleanup = () => {
    serviceManager.off(`log:${name}`, onLog);
    serviceManager.off(`status:${name}`, onStatus);
    clearInterval(heartbeat);
  };

  req.on("close", cleanup);
  req.on("error", cleanup);
});

// 启动服务
app.post("/api/services/:name/start", async (req, res) => {
  const result = await serviceManager.start(req.params.name);
  res.json(result);
});

// 停止服务
app.post("/api/services/:name/stop", async (req, res) => {
  const result = await serviceManager.stop(req.params.name);
  res.json(result);
});

// 重启服务
app.post("/api/services/:name/restart", async (req, res) => {
  const result = await serviceManager.restart(req.params.name);
  res.json(result);
});

// 健康检查
app.get("/health", (_req, res) => {
  res.json({ status: "ok", timestamp: new Date().toISOString() });
});

app.get("/api/health", (_req, res) => {
  res.json({ status: "ok", timestamp: new Date().toISOString() });
});

// 生产环境：静态文件服务
if (!appConfig.isDev) {
  const staticPath = resolve(__dirname, "../../admin/dist");
  app.use(express.static(staticPath));

  // SPA fallback
  app.get("*", (_req, res) => {
    res.sendFile(resolve(staticPath, "index.html"));
  });
}

// 启动服务器
app.listen(appConfig.port, () => {
  logger.boundary("service_start", `server started on port ${appConfig.port}`, {
    port: appConfig.port,
    mode: appConfig.isDev ? "development" : "production",
  });

  const logConsole = process.env.LOG_CONSOLE === "1" || process.env.LOG_CONSOLE === "true";
  const logLevel = process.env.LOG_CONSOLE_LEVEL || "boundary";
  const rawServiceLogConsole = process.env.SERVICE_LOG_CONSOLE;
  const serviceLogSessionId = (process.env.SERVICE_LOG_SESSION_ID || "").trim();
  const serviceLogTraceId = (process.env.SERVICE_LOG_TRACE_ID || "").trim();
  // 与 service-manager.ts 保持一致：未设置时默认开启，显式 0/false 关闭
  const serviceLogConsole =
    rawServiceLogConsole === undefined
      ? true
      : rawServiceLogConsole === "1" || rawServiceLogConsole === "true";
  const serviceLogFilter = [
    serviceLogSessionId ? `session=${serviceLogSessionId}` : "",
    serviceLogTraceId ? `trace=${serviceLogTraceId}` : "",
  ].filter(Boolean).join(", ");

  console.log(`
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║   🤖 Agent Server is running!                             ║
║                                                           ║
║   Local:   http://localhost:${appConfig.port}                      ║
║   API:     http://localhost:${appConfig.port}/api                  ║
║                                                           ║
║   Mode:    ${appConfig.isDev ? "Development" : "Production"}                              ║
║   Logs:    data/logs/{boundary,business,detail}/           ║
║   Console: ${logConsole ? `ON (level: ${logLevel})`.padEnd(34) : "OFF (set LOG_CONSOLE=1 to enable)  "}║
║   SvcLog:  ${serviceLogConsole ? "ON (child services mirrored)".padEnd(34) : "OFF (set SERVICE_LOG_CONSOLE=1)   "}║
║   Filter:  ${serviceLogFilter ? serviceLogFilter.slice(0, 34).padEnd(34) : "none".padEnd(34)}║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
  `);

  // 启动后恢复 Agent 默认模型配置（延迟等待 Orchestrator 就绪）
  applyDefaultAgentModel();
});

/**
 * 从 settings 读取已保存的 Agent 默认模型，并配置到 Orchestrator
 * 延迟执行以等待 Orchestrator 启动
 */
async function applyDefaultAgentModel() {
  const modelId = settingsDb.get("agent.default_model");
  if (!modelId) return;

  const model = getModelById(modelId);
  if (!model || !model.apiKey) return;

  // 延迟 5 秒，等待 Orchestrator 启动
  setTimeout(async () => {
    try {
      await orchestratorClient.configureModel({
        baseUrl: model.baseUrl,
        apiKey: model.apiKey,
        model: model.model,
      });
      console.log(`🔄 Agent default model restored: ${model.name} (${model.provider})`);
    } catch (error) {
      // Orchestrator 可能还没启动，静默忽略
      console.log(`⚠️ Could not apply default agent model (orchestrator may not be ready yet)`);
    }
  }, 5000);
}
