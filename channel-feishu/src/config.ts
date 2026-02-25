import { config } from "dotenv";
import { resolve } from "path";

// 加载 .env 文件
config({ path: resolve(import.meta.dir, "../../.env") });

export const feishuConfig = {
  // 飞书应用凭证
  appId: process.env.FEISHU_APP_ID || "",
  appSecret: process.env.FEISHU_APP_SECRET || "",

  // 事件订阅加密 key（Webhook 模式需要，长连接模式可选）
  encryptKey: process.env.FEISHU_ENCRYPT_KEY || "",
  verificationToken: process.env.FEISHU_VERIFICATION_TOKEN || "",

  // 服务配置
  port: parseInt(process.env.FEISHU_BOT_PORT || "1999", 10),
  isDev: process.env.NODE_ENV !== "production",

  // 日志级别
  logLevel: process.env.FEISHU_LOG_LEVEL || "info",

  // Agent 联动配置
  agentEnabled: process.env.AGENT_ENABLED !== "false", // 默认开启
  agentServerUrl: process.env.AGENT_SERVER_URL || "http://localhost:1997",

  // 多 Agent 架构：本 bot 对应的 Agent ID（agent_configs.id）
  // 群内每个 bot 是一个独立 Agent，各自收到消息后携带各自的 agentId 转发
  agentId: process.env.AGENT_ID || "",
};

// 验证必要配置
export function validateConfig(): void {
  const required = ["appId", "appSecret"] as const;
  const missing = required.filter((key) => !feishuConfig[key]);

  if (missing.length > 0) {
    console.error(
      `❌ Missing required Feishu config: ${missing.join(", ")}`
    );
    console.error(
      "   Please set FEISHU_APP_ID and FEISHU_APP_SECRET in your .env file"
    );
    process.exit(1);
  }
}
