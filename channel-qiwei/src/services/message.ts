/**
 * 企微消息服务
 * 封装各种消息发送能力，通过 QiWe API 发送
 */

import { doApi } from "../client";
import { qiweiConfig } from "../config";

/**
 * 发送文本消息
 */
export async function sendText(toId: string, content: string) {
  return doApi("/msg/sendText", {
    toId,
    content,
  });
}

/**
 * 发送混合文本消息（支持 @人）
 * atList: 需要 @的用户 ID 列表
 */
export async function sendHyperText(
  toId: string,
  content: string,
  atList?: string[]
) {
  return doApi("/msg/sendHyperText", {
    toId,
    content,
    ...(atList ? { atList } : {}),
  });
}

/**
 * 发送图片消息
 * imgUrl: 图片 URL（支持 http/https）
 */
export async function sendImage(toId: string, imgUrl: string) {
  return doApi("/msg/sendImage", {
    toId,
    imgUrl,
  });
}

/**
 * 发送文件消息
 * fileUrl: 文件 URL
 * fileName: 文件名
 */
export async function sendFile(toId: string, fileUrl: string, fileName: string) {
  return doApi("/msg/sendFile", {
    toId,
    fileUrl,
    fileName,
  });
}

/**
 * 发送链接卡片消息
 */
export async function sendLink(
  toId: string,
  title: string,
  desc: string,
  linkUrl: string,
  thumbUrl?: string
) {
  return doApi("/msg/sendLink", {
    toId,
    title,
    desc,
    linkUrl,
    ...(thumbUrl ? { thumbUrl } : {}),
  });
}

/**
 * 撤回消息
 */
export async function revokeMessage(msgSvrId: string, toId: string) {
  return doApi("/msg/revokeMsg", {
    msgSvrId,
    toId,
  });
}

/**
 * 发送群 @消息
 */
export async function sendGroupAtText(
  roomId: string,
  content: string,
  atList?: string[]
) {
  return doApi("/msg/sendHyperText", {
    toId: roomId,
    content,
    ...(atList ? { atList } : {}),
  });
}
