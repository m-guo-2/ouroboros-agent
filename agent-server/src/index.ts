import express from "express";
import cors from "cors";
import { resolve } from "path";
import { appConfig } from "./config";
import chatRoutes from "./routes/chat";
import modelRoutes from "./routes/models";

const app = express();

// 中间件
app.use(cors());
app.use(express.json());

// API 路由
app.use("/api", chatRoutes);
app.use("/api/models", modelRoutes);

// 健康检查
app.get("/api/health", (_req, res) => {
  res.json({ status: "ok", timestamp: new Date().toISOString() });
});

// 生产环境：静态文件服务
if (!appConfig.isDev) {
  const staticPath = resolve(__dirname, "../../agent-web/dist");
  app.use(express.static(staticPath));
  
  // SPA fallback
  app.get("*", (_req, res) => {
    res.sendFile(resolve(staticPath, "index.html"));
  });
}

// 启动服务器
app.listen(appConfig.port, () => {
  console.log(`
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║   🤖 Agent Server is running!                             ║
║                                                           ║
║   Local:   http://localhost:${appConfig.port}                      ║
║   API:     http://localhost:${appConfig.port}/api                  ║
║                                                           ║
║   Mode:    ${appConfig.isDev ? "Development" : "Production"}                              ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
  `);
});
