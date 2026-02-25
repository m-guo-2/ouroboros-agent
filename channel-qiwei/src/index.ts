import express from "express";
import cors from "cors";
import { qiweiConfig, validateConfig } from "./config";
import callbackRoutes from "./routes/callback";
import sendRoutes from "./routes/send";

// ==================== 验证配置 ====================
validateConfig();

// ==================== Express 服务器 ====================
const app = express();
app.use(cors());
app.use(express.json({ limit: "50mb" }));

// QiWe 回调接收端点
app.use("/webhook/callback", callbackRoutes);

// Agent-server 回调发送端点
app.use("/api/qiwei/send", sendRoutes);

// 健康检查
app.get("/health", (_req, res) => {
  res.json({
    status: "ok",
    service: "channel-qiwei",
    timestamp: new Date().toISOString(),
  });
});

app.get("/api/health", (_req, res) => {
  res.json({
    status: "ok",
    service: "channel-qiwei",
    timestamp: new Date().toISOString(),
  });
});

// ==================== 启动服务 ====================
app.listen(qiweiConfig.port, () => {
  console.log(`
╔═══════════════════════════════════════════════════════════════╗
║                                                               ║
║   🤖 企微 Bot 服务已启动!                                       ║
║                                                               ║
║   Local:      http://localhost:${qiweiConfig.port}                        ║
║   Callback:   http://localhost:${qiweiConfig.port}/webhook/callback       ║
║   Send API:   http://localhost:${qiweiConfig.port}/api/qiwei/send         ║
║   Health:     http://localhost:${qiweiConfig.port}/health                 ║
║                                                               ║
║   Agent:      ${qiweiConfig.agentEnabled ? "Enabled ✅" : "Disabled ❌"}                                    ║
║   Agent URL:  ${qiweiConfig.agentServerUrl}                    ║
║                                                               ║
╚═══════════════════════════════════════════════════════════════╝
  `);
});

export { doApi } from "./client";
export * as messageService from "./services/message";
