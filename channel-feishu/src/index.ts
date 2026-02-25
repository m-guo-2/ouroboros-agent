import express from "express";
import cors from "cors";
import { feishuConfig, validateConfig } from "./config";
import { getClient, lark } from "./client";
import { createEventDispatcher, onMessage } from "./events";
import feishuRoutes from "./routes";

// ==================== 验证配置 ====================
validateConfig();

// ==================== Express 服务器 ====================
const app = express();
app.use(cors());
app.use(express.json({ limit: "50mb" }));

// 飞书 API 路由
app.use("/api/feishu", feishuRoutes);

// 健康检查
app.get("/health", (_req, res) => {
  res.json({
    status: "ok",
    service: "channel-feishu",
    timestamp: new Date().toISOString(),
  });
});

app.get("/api/health", (_req, res) => {
  res.json({
    status: "ok",
    service: "channel-feishu",
    timestamp: new Date().toISOString(),
  });
});

// Webhook 事件回调端点（备用，主要使用长连接模式）
const eventDispatcher = createEventDispatcher();
app.use("/webhook/event", lark.adaptExpress(eventDispatcher, { autoChallenge: true }));

// ==================== 启动 WebSocket 长连接 ====================
async function startWSClient() {
  try {
    const wsClient = new lark.WSClient({
      appId: feishuConfig.appId,
      appSecret: feishuConfig.appSecret,
      loggerLevel: lark.LoggerLevel.info,
    });

    await wsClient.start({
      eventDispatcher,
    });

    console.log("🔌 WebSocket 长连接已建立");
  } catch (err) {
    console.error("❌ WebSocket 连接失败:", err);
    console.log("⚠️  将使用 Webhook 模式，请确保配置了公网回调地址");
  }
}

// ==================== 启动服务 ====================
app.listen(feishuConfig.port, () => {
  console.log(`
╔═══════════════════════════════════════════════════════════════╗
║                                                               ║
║   🤖 飞书 Bot 服务已启动!                                       ║
║                                                               ║
║   Local:      http://localhost:${feishuConfig.port}                        ║
║   API:        http://localhost:${feishuConfig.port}/api/feishu              ║
║   Webhook:    http://localhost:${feishuConfig.port}/webhook/event           ║
║                                                               ║
║   Mode:       ${feishuConfig.isDev ? "Development" : "Production"}                                    ║
║                                                               ║
║   ── Agent 调用入口 ─────────────────────────────────────      ║
║                                                               ║
║   POST /api/feishu/action   统一 Action 端点（推荐）          ║
║   GET  /api/feishu/action/list   列出所有可用 action          ║
║                                                               ║
║   ── 统一发送 ─────────────────────────────────────────────     ║
║                                                               ║
║   POST /api/feishu/send   统一消息发送（文本/富文本/卡片/      ║
║                           图片/文件/音频/视频 + @用户 + 引用）  ║
║                                                               ║
║   ── 消息查询与管理 ──────────────────────────────────────     ║
║                                                               ║
║     GET    /api/feishu/message/:id         获取消息详情         ║
║     GET    /api/feishu/message/list/:chatId 获取消息列表        ║
║     DELETE /api/feishu/message/:id         撤回消息             ║
║     GET    /api/feishu/message/chat/:id    获取群信息           ║
║     GET    /api/feishu/message/chat/:id/members 获取群成员      ║
║     POST   /api/feishu/message/chat        创建群组             ║
║                                                               ║
║   会议:                                                        ║
║     POST /api/feishu/meeting/reserve       预约会议             ║
║     GET  /api/feishu/meeting/:id           获取会议详情         ║
║     POST /api/feishu/meeting/:id/invite    邀请参会人           ║
║     POST /api/feishu/meeting/:id/end       结束会议             ║
║                                                               ║
║   文档:                                                        ║
║     POST /api/feishu/document              创建文档             ║
║     GET  /api/feishu/document/:id          获取文档信息         ║
║     POST /api/feishu/document/:id/blocks   追加文档内容         ║
║     GET  /api/feishu/document/wiki/spaces  获取知识库列表       ║
║     POST /api/feishu/document/wiki/node    创建知识库节点       ║
║                                                               ║
╚═══════════════════════════════════════════════════════════════╝
  `);

  // 启动 WebSocket 长连接
  startWSClient();
});

// ==================== 导出 ====================
export { onMessage } from "./events";
export { getClient } from "./client";
export * as messageService from "./services/message";
export * as meetingService from "./services/meeting";
export * as documentService from "./services/document";
