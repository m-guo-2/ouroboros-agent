import { lark } from "../client";
import { feishuConfig } from "../config";
import { handleMessageReceive } from "./message";

/**
 * 创建事件分发器
 * 注册飞书事件的处理函数
 *
 * 策略：
 *   - 用户消息事件 (im.message.receive_v1) → 投递到 Agent
 *   - 其他事件 → 仅接收并记录日志，不投递到 Agent
 */
export function createEventDispatcher(): InstanceType<
  typeof lark.EventDispatcher
> {
  const dispatcher = new lark.EventDispatcher({
    encryptKey: feishuConfig.encryptKey || undefined,
    verificationToken: feishuConfig.verificationToken || undefined,
  });

  // ==================== 用户消息事件 → 投递到 Agent ====================
  dispatcher.register({
    "im.message.receive_v1": async (data: any) => {
      await handleMessageReceive(data);
    },
  });

  // ==================== 其他事件 → 仅接收记录，不投递 ====================

  // 消息已读事件
  dispatcher.register({
    "im.message.message_read_v1": async (data: any) => {
      console.log("📋 [事件-仅记录] 消息已读:", JSON.stringify(data).substring(0, 200));
    },
  });

  // 消息撤回事件
  dispatcher.register({
    "im.message.recalled_v1": async (data: any) => {
      console.log("📋 [事件-仅记录] 消息撤回:", JSON.stringify(data).substring(0, 200));
    },
  });

  // 机器人进群事件
  dispatcher.register({
    "im.chat.member.bot.added_v1": async (data: any) => {
      console.log("📋 [事件-仅记录] 机器人被加入群聊:", JSON.stringify(data).substring(0, 200));
    },
  });

  // 机器人出群事件
  dispatcher.register({
    "im.chat.member.bot.deleted_v1": async (data: any) => {
      console.log("📋 [事件-仅记录] 机器人被移出群聊:", JSON.stringify(data).substring(0, 200));
    },
  });

  // 群信息变更事件
  dispatcher.register({
    "im.chat.updated_v1": async (data: any) => {
      console.log("📋 [事件-仅记录] 群信息变更:", JSON.stringify(data).substring(0, 200));
    },
  });

  // 群成员变动事件
  dispatcher.register({
    "im.chat.member.user.added_v1": async (data: any) => {
      console.log("📋 [事件-仅记录] 群成员加入:", JSON.stringify(data).substring(0, 200));
    },
  });
  dispatcher.register({
    "im.chat.member.user.deleted_v1": async (data: any) => {
      console.log("📋 [事件-仅记录] 群成员退出:", JSON.stringify(data).substring(0, 200));
    },
  });

  // 消息表情回复事件
  dispatcher.register({
    "im.message.reaction.created_v1": async (data: any) => {
      console.log("📋 [事件-仅记录] 消息表情回复:", JSON.stringify(data).substring(0, 200));
    },
  });
  dispatcher.register({
    "im.message.reaction.deleted_v1": async (data: any) => {
      console.log("📋 [事件-仅记录] 消息表情回复删除:", JSON.stringify(data).substring(0, 200));
    },
  });

  console.log("📡 事件分发器已创建，已注册事件:");
  console.log("   - im.message.receive_v1 (用户消息 → 投递到 Agent)");
  console.log("   - im.message.message_read_v1 (消息已读 → 仅记录)");
  console.log("   - im.message.recalled_v1 (消息撤回 → 仅记录)");
  console.log("   - im.chat.member.bot.added/deleted_v1 (机器人进出群 → 仅记录)");
  console.log("   - im.chat.updated_v1 (群信息变更 → 仅记录)");
  console.log("   - im.chat.member.user.added/deleted_v1 (群成员变动 → 仅记录)");
  console.log("   - im.message.reaction.created/deleted_v1 (表情回复 → 仅记录)");

  return dispatcher;
}

export { onMessage } from "./message";
