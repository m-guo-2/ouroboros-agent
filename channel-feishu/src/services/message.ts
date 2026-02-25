import { getClient, lark } from "../client";
import type {
  SendTextParams,
  SendRichTextParams,
  SendCardParams,
  SendImageParams,
  SendFileParams,
  ReplyMessageParams,
  UploadImageParams,
  UploadFileParams,
} from "../types";

const client = () => getClient();

// ==================== 发送消息 ====================

/** 发送文本消息 */
export async function sendText(params: SendTextParams) {
  const res = await client().im.message.create({
    params: {
      receive_id_type: params.receiveIdType,
    },
    data: {
      receive_id: params.receiveId,
      content: JSON.stringify({ text: params.text }),
      msg_type: "text",
    },
  });
  return res;
}

/** 发送富文本消息（post） */
export async function sendRichText(params: SendRichTextParams) {
  const res = await client().im.message.create({
    params: {
      receive_id_type: params.receiveIdType,
    },
    data: {
      receive_id: params.receiveId,
      content: JSON.stringify({
        zh_cn: {
          title: params.title,
          content: params.content,
        },
      }),
      msg_type: "post",
    },
  });
  return res;
}

/** 发送交互卡片消息 */
export async function sendCard(params: SendCardParams) {
  // 使用模板消息
  if (params.templateId) {
    const res = await client().im.message.create({
      params: {
        receive_id_type: params.receiveIdType,
      },
      data: {
        receive_id: params.receiveId,
        content: JSON.stringify({
          type: "template",
          data: {
            template_id: params.templateId,
            template_variable: params.templateVariable || {},
          },
        }),
        msg_type: "interactive",
      },
    });
    return res;
  }

  // 使用自定义卡片内容
  if (params.cardContent) {
    const res = await client().im.message.create({
      params: {
        receive_id_type: params.receiveIdType,
      },
      data: {
        receive_id: params.receiveId,
        content: JSON.stringify(params.cardContent),
        msg_type: "interactive",
      },
    });
    return res;
  }

  throw new Error("Must provide either templateId or cardContent");
}

/** 使用 SDK 内置卡片发送消息 */
export async function sendDefaultCard(params: {
  receiveId: string;
  receiveIdType: string;
  title: string;
  content: string;
}) {
  const res = await client().im.message.create({
    params: {
      receive_id_type: params.receiveIdType as any,
    },
    data: {
      receive_id: params.receiveId,
      content: lark.messageCard.defaultCard({
        title: params.title,
        content: params.content,
      }),
      msg_type: "interactive",
    },
  });
  return res;
}

/** 发送图片消息 */
export async function sendImage(params: SendImageParams) {
  const res = await client().im.message.create({
    params: {
      receive_id_type: params.receiveIdType,
    },
    data: {
      receive_id: params.receiveId,
      content: JSON.stringify({ image_key: params.imageKey }),
      msg_type: "image",
    },
  });
  return res;
}

/** 发送文件消息 */
export async function sendFile(params: SendFileParams) {
  const res = await client().im.message.create({
    params: {
      receive_id_type: params.receiveIdType,
    },
    data: {
      receive_id: params.receiveId,
      content: JSON.stringify({ file_key: params.fileKey }),
      msg_type: "file",
    },
  });
  return res;
}

/** 发送音频消息 */
export async function sendAudio(params: {
  receiveId: string;
  receiveIdType: string;
  fileKey: string;
}) {
  const res = await client().im.message.create({
    params: {
      receive_id_type: params.receiveIdType as any,
    },
    data: {
      receive_id: params.receiveId,
      content: JSON.stringify({ file_key: params.fileKey }),
      msg_type: "audio",
    },
  });
  return res;
}

/** 发送视频消息 */
export async function sendVideo(params: {
  receiveId: string;
  receiveIdType: string;
  fileKey: string;
  imageKey: string;
}) {
  const res = await client().im.message.create({
    params: {
      receive_id_type: params.receiveIdType as any,
    },
    data: {
      receive_id: params.receiveId,
      content: JSON.stringify({
        file_key: params.fileKey,
        image_key: params.imageKey,
      }),
      msg_type: "media",
    },
  });
  return res;
}

// ==================== 回复消息 ====================

/** 回复指定消息 */
export async function replyMessage(params: ReplyMessageParams) {
  const res = await client().im.message.reply({
    path: {
      message_id: params.messageId,
    },
    data: {
      content: params.content,
      msg_type: params.msgType,
    },
  });
  return res;
}

// ==================== 文件上传 ====================

/** 上传图片 */
export async function uploadImage(params: UploadImageParams) {
  const res = await client().im.image.create({
    data: {
      image_type: params.imageType,
      image: params.image,
    },
  });
  return res;
}

/** 上传文件 */
export async function uploadFile(params: UploadFileParams) {
  const res = await client().im.file.create({
    data: {
      file_type: params.fileType,
      file_name: params.fileName,
      file: params.file,
      ...(params.duration ? { duration: params.duration } : {}),
    },
  });
  return res;
}

/**
 * 从 URL 下载图片后上传到飞书，返回 image_key。
 * 适合 Agent 调用——Agent 只需提供图片 URL 即可。
 */
export async function uploadImageFromUrl(
  imageUrl: string,
  imageType: string = "message",
) {
  const response = await fetch(imageUrl);
  if (!response.ok) {
    throw new Error(`Failed to download image from ${imageUrl}: ${response.status}`);
  }
  const buffer = Buffer.from(await response.arrayBuffer());

  const res = await client().im.image.create({
    data: {
      image_type: imageType as any,
      image: buffer,
    },
  });
  return res;
}

/**
 * 从 URL 下载文件后上传到飞书，返回 file_key。
 * 支持音频（opus）、视频（mp4）、通用文件（doc/xls/pdf 等）。
 */
export async function uploadFileFromUrl(params: {
  fileUrl: string;
  fileName: string;
  fileType: string;
  duration?: string;
}) {
  const response = await fetch(params.fileUrl);
  if (!response.ok) {
    throw new Error(`Failed to download file from ${params.fileUrl}: ${response.status}`);
  }
  const buffer = Buffer.from(await response.arrayBuffer());

  const res = await client().im.file.create({
    data: {
      file_type: params.fileType as any,
      file_name: params.fileName,
      file: buffer,
      ...(params.duration ? { duration: params.duration } : {}),
    },
  });
  return res;
}

// ==================== 消息管理 ====================

/** 撤回消息 */
export async function recallMessage(messageId: string) {
  const res = await client().im.message.delete({
    path: {
      message_id: messageId,
    },
  });
  return res;
}

/** 获取消息详情 */
export async function getMessage(messageId: string) {
  const res = await client().im.message.get({
    path: {
      message_id: messageId,
    },
  });
  return res;
}

/** 获取会话消息列表 */
export async function getMessageList(
  chatId: string,
  pageSize = 20,
  pageToken?: string
) {
  const res = await client().im.message.list({
    params: {
      container_id_type: "chat",
      container_id: chatId,
      page_size: pageSize,
      ...(pageToken ? { page_token: pageToken } : {}),
    },
  });
  return res;
}

// ==================== 群组管理 ====================

/** 获取群信息 */
export async function getChatInfo(chatId: string) {
  const res = await client().im.chat.get({
    path: {
      chat_id: chatId,
    },
  });
  return res;
}

/** 获取群成员列表（自动分页，返回全量） */
export async function getChatMembers(chatId: string, pageSize = 100) {
  const allMembers: any[] = [];
  let pageToken: string | undefined;
  let hasMore = true;

  while (hasMore) {
    const res = await client().im.chatMembers.get({
      path: {
        chat_id: chatId,
      },
      params: {
        member_id_type: "open_id",
        page_size: pageSize,
        ...(pageToken ? { page_token: pageToken } : {}),
      },
    });

    const items = res?.data?.items || [];
    allMembers.push(...items);
    hasMore = res?.data?.has_more ?? false;
    pageToken = res?.data?.page_token;
  }

  return {
    code: 0,
    data: {
      items: allMembers,
      member_list: allMembers,
      total: allMembers.length,
    },
  };
}

/** 获取机器人所在的群列表（自动分页，返回全量） */
export async function listBotChats(pageSize = 100) {
  const allChats: any[] = [];
  let pageToken: string | undefined;
  let hasMore = true;

  while (hasMore) {
    const res = await client().im.chat.list({
      params: {
        page_size: pageSize,
        ...(pageToken ? { page_token: pageToken } : {}),
      },
    });

    const items = res?.data?.items || [];
    allChats.push(...items);
    hasMore = res?.data?.has_more ?? false;
    pageToken = res?.data?.page_token;
  }

  return {
    code: 0,
    data: {
      items: allChats,
      total: allChats.length,
    },
  };
}

/** 搜索群组（按关键词） */
export async function searchChats(query: string, pageSize = 20) {
  const res = await client().im.chat.search({
    params: {
      query,
      page_size: pageSize,
    },
  });
  return res;
}

/** 通过手机号或邮箱批量查找用户 ID */
export async function batchGetUserId(params: {
  emails?: string[];
  mobiles?: string[];
}) {
  const res = await client().contact.user.batchGetId({
    params: {
      user_id_type: "open_id",
    },
    data: {
      emails: params.emails || [],
      mobiles: params.mobiles || [],
    },
  });
  return res;
}

/** 获取用户详细信息 */
export async function getUserInfo(userId: string, userIdType: string = "open_id") {
  const res = await client().contact.user.get({
    path: {
      user_id: userId,
    },
    params: {
      user_id_type: userIdType as any,
    },
  });
  return res;
}

// ==================== 表情回复 ====================

/** 给消息添加表情回复 */
export async function addReaction(messageId: string, emojiType: string) {
  const res = await client().im.messageReaction.create({
    path: {
      message_id: messageId,
    },
    data: {
      reaction_type: {
        emoji_type: emojiType,
      },
    },
  });
  return res;
}

/** 删除表情回复 */
export async function deleteReaction(messageId: string, reactionId: string) {
  const res = await client().im.messageReaction.delete({
    path: {
      message_id: messageId,
      reaction_id: reactionId,
    },
  });
  return res;
}

/** 获取消息的表情回复列表 */
export async function getReactions(messageId: string, emojiType?: string) {
  const res = await client().im.messageReaction.list({
    path: {
      message_id: messageId,
    },
    params: {
      ...(emojiType ? { reaction_type: emojiType } : {}),
    },
  });
  return res;
}

// ==================== 群组管理 ====================

/** 创建群组 */
export async function createChat(params: {
  name: string;
  description?: string;
  userIdList?: string[];
}) {
  const res = await client().im.chat.create({
    params: {
      user_id_type: "open_id",
    },
    data: {
      name: params.name,
      description: params.description,
      user_id_list: params.userIdList,
    },
  });
  return res;
}
