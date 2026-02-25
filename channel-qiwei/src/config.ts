import { config } from "dotenv";
import { resolve } from "path";

// 加载 .env 文件（项目根目录）
config({ path: resolve(import.meta.dir, "../../.env") });

export const qiweiConfig = {
  // QiWe 平台 API 配置
  apiBaseUrl: process.env.QIWEI_API_BASE_URL || "https://api.qiweapi.com",
  token: process.env.QIWEI_TOKEN || "",
  guid: process.env.QIWEI_GUID || "",

  // 服务配置
  port: parseInt(process.env.QIWEI_BOT_PORT || "2000", 10),

  // Agent 联动配置
  agentEnabled: process.env.AGENT_ENABLED !== "false",
  agentServerUrl: process.env.AGENT_SERVER_URL || "http://localhost:1997",

  // 多 Agent 架构：本 bot 对应的 Agent ID（agent_configs.id）
  agentId: process.env.AGENT_ID || "",
};

/**
 * 验证必要配置
 */
export function validateConfig(): void {
  const required = ["token", "guid"] as const;
  const missing = required.filter((key) => !qiweiConfig[key]);

  if (missing.length > 0) {
    console.error(
      `❌ Missing required QiWei config: ${missing.join(", ")}`
    );
    console.error(
      "   Please set QIWEI_TOKEN and QIWEI_GUID in your .env file"
    );
    process.exit(1);
  }
}
