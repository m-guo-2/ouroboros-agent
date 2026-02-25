/**
 * 用户身份解析服务
 * 负责将渠道用户ID解析为统一用户ID，支持影子用户自动创建和跨渠道绑定
 */

import { userDb, userChannelDb, bindingCodeDb, userMemoryDb, userMemoryFactDb } from "./database";
import type { ChannelType } from "./channel-types";

/**
 * 解析渠道用户到统一用户
 * 如果渠道用户未绑定，自动创建影子用户
 */
export function resolveUser(
  channelType: ChannelType,
  channelUserId: string,
  senderName?: string
): { userId: string; isNew: boolean } {
  // 1. 查找已有的渠道绑定
  const existing = userChannelDb.findByChannelUser(channelType, channelUserId);
  if (existing) {
    return { userId: existing.userId, isNew: false };
  }

  // 2. 自动创建影子用户
  const userId = crypto.randomUUID();
  const displayName = senderName || `${channelType}:${channelUserId.substring(0, 8)}`;

  userDb.create({
    id: userId,
    name: displayName,
    metadata: { isShadow: true, originChannel: channelType },
  });

  // 3. 创建渠道绑定
  userChannelDb.create({
    id: crypto.randomUUID(),
    userId,
    channelType,
    channelUserId,
    displayName,
  });

  // 4. 初始化空的记忆摘要
  userMemoryDb.upsert(userId, "");

  return { userId, isNew: true };
}

/**
 * 生成绑定码（6位字母数字，5分钟有效）
 */
export function generateBindingCode(userId: string, targetChannel: ChannelType): string {
  const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"; // 排除易混淆字符
  let code = "";
  for (let i = 0; i < 6; i++) {
    code += chars[Math.floor(Math.random() * chars.length)];
  }

  const expiresAt = new Date(Date.now() + 5 * 60 * 1000).toISOString();

  bindingCodeDb.create({
    code,
    userId,
    targetChannel,
    expiresAt,
  });

  return code;
}

/**
 * 使用绑定码绑定渠道账号
 * 如果当前渠道用户已有影子用户，将影子用户的数据合并到目标用户
 */
export function redeemBindingCode(
  code: string,
  channelType: ChannelType,
  channelUserId: string,
  displayName?: string
): { success: boolean; userId?: string; error?: string } {
  // 1. 验证绑定码
  const bindingCode = bindingCodeDb.getValidCode(code);
  if (!bindingCode) {
    return { success: false, error: "绑定码无效或已过期" };
  }

  if (bindingCode.targetChannel !== channelType) {
    return { success: false, error: `此绑定码用于 ${bindingCode.targetChannel} 渠道，当前渠道为 ${channelType}` };
  }

  // 2. 检查该渠道用户是否已绑定到其他用户
  const existingBinding = userChannelDb.findByChannelUser(channelType, channelUserId);

  if (existingBinding) {
    if (existingBinding.userId === bindingCode.userId) {
      // 已经绑定到同一用户
      bindingCodeDb.markUsed(code);
      return { success: true, userId: bindingCode.userId };
    }

    // 该渠道账号已绑定到一个影子用户，需要合并
    const shadowUserId = existingBinding.userId;
    const shadowUser = userDb.getById(shadowUserId);

    if (shadowUser?.metadata && (shadowUser.metadata as Record<string, unknown>).isShadow) {
      // 合并影子用户数据到目标用户
      mergeShadowUser(shadowUserId, bindingCode.userId);
    } else {
      return { success: false, error: "该渠道账号已绑定到另一个非影子用户，请先解绑" };
    }
  } else {
    // 3. 创建新的渠道绑定
    userChannelDb.create({
      id: crypto.randomUUID(),
      userId: bindingCode.userId,
      channelType,
      channelUserId,
      displayName,
    });
  }

  // 4. 标记绑定码已使用
  bindingCodeDb.markUsed(code);

  return { success: true, userId: bindingCode.userId };
}

/**
 * 合并影子用户到目标用户
 * 将影子用户的渠道绑定、记忆事实迁移到目标用户，然后删除影子用户
 */
function mergeShadowUser(shadowUserId: string, targetUserId: string): void {
  // 迁移渠道绑定
  userChannelDb.transferToUser(shadowUserId, targetUserId);

  // 迁移记忆事实
  userMemoryFactDb.transferToUser(shadowUserId, targetUserId);

  // 合并记忆摘要（追加影子用户的摘要到目标用户）
  const shadowMemory = userMemoryDb.getByUserId(shadowUserId);
  const targetMemory = userMemoryDb.getByUserId(targetUserId);
  if (shadowMemory?.summary) {
    const merged = targetMemory?.summary
      ? `${targetMemory.summary}\n\n[合并自影子用户] ${shadowMemory.summary}`
      : shadowMemory.summary;
    userMemoryDb.upsert(targetUserId, merged);
  }

  // 删除影子用户（级联删除残留数据）
  userDb.delete(shadowUserId);
}

/**
 * 解绑渠道账号
 */
export function unbindChannel(userId: string, channelBindingId: string): boolean {
  const channels = userChannelDb.getByUserId(userId);
  const target = channels.find(c => c.id === channelBindingId);
  if (!target) return false;

  // 确保至少保留一个渠道绑定
  if (channels.length <= 1) {
    return false;
  }

  return userChannelDb.delete(channelBindingId);
}

/**
 * 获取用户的完整信息（含所有渠道绑定）
 */
export function getUserWithChannels(userId: string) {
  const user = userDb.getById(userId);
  if (!user) return null;

  const channels = userChannelDb.getByUserId(userId);
  return { ...user, channels };
}
