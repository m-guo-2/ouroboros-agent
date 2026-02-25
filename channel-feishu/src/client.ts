import * as lark from "@larksuiteoapi/node-sdk";
import { feishuConfig } from "./config";

// Feishu Client 单例
let _client: lark.Client | null = null;

export function getClient(): lark.Client {
  if (!_client) {
    _client = new lark.Client({
      appId: feishuConfig.appId,
      appSecret: feishuConfig.appSecret,
      appType: lark.AppType.SelfBuild,
      domain: lark.Domain.Feishu,
      loggerLevel: feishuConfig.logLevel as unknown as lark.LoggerLevel,
    });
  }
  return _client;
}

export { lark };
