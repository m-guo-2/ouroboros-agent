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
      receive_id_type: params.receiveIdType,
    },
    data: {
      receive_id: params.receiveId,
      content: JSON.stringify({
        config: { wide_screen_mode: true },
        elements: [
          {
            tag: "div",
            text: {
              tag: "plain_text",
              content: params.content,
            },
          },
        ],
        header: {
          title: {
            tag: "plain_text",
            content: params.title,
          },
          template: "blue",
        },
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

/** 回复消息 */
export async function replyMessage(params: ReplyMessageParams) {
  const res = await client().im.message.reply({
    path: {
      message_id: params.messageId,
    },
    data: {
      content: JSON.stringify(
        params.msgType === "image"
          ? { image_key: params.content }
          : params.msgType === "file"
          ? { file_key: params.content }
          : { text: params.content }
      ),
      msg_type: params.msgType || "text",
    },
  });
  return res;
}

// ==================== 媒体上传 ====================

/**
 * 从 URL 下载图片后上传到飞书，返回 image_key。
 * 支持 message/avatar 两种类型。
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
  // 返回 image_key 而不是完整响应
  return res?.data?.image_key || res;
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
  // 返回 file_key 而不是完整响应
  return res?.data?.file_key || res;
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

/** 创建群组 */
export async function createChat(params: {
  name: string;
  description?: string;
  userIdList?: string[];
}) {
  const res = await client().im.chat.create({
    data: {
      name: params.name,
      description: params.description,
      user_id_list: params.userIdList,
    },
  });
  return res;
}

/** 获取群信息 */
export async function getChatInfo(chatId: string) {
  const res = await client().im.chat.get({
    path: {
      chat_id: chatId,
    },
  });
  return res;
}

/** 获取群成员列表 */
export async function getChatMembers(chatId: string) {
  const res = await client().im.chat.members({
    path: {
      chat_id: chatId,
    },
  });
  return res;
}

/** 获取机器人所在的所有群列表 */
export async function listBotChats() {
  const res = await client().im.chat.list();
  return res;
}

/** 搜索群组 */
export async function searchChats(query: string, pageSize = 20) {
  const res = await client().im.chat.search({
    params: {
      query,
      page_size: pageSize,
    },
  });
  return res;
}

// ==================== 用户管理 ====================

/** 通过邮箱或手机号批量查找用户 ID */
export async function batchGetUserId(params: {
  emails?: string[];
  mobiles?: string[];
}) {
  const res = await client().contact.users.batchGetId({
    params: {
      user_id_type: "open_id",
    },
    data: {
      ...(params.emails ? { emails: params.emails } : {}),
      ...(params.mobiles ? { mobiles: params.mobiles } : {}),
    },
  });
  return res;
}

/** 获取用户详细信息 */
export async function getUserInfo(userId: string, userIdType = "open_id") {
  const res = await client().contact.user.get({
    params: {
      user_id_type: userIdType as any,
    },
    path: {
      user_id: userId,
    },
  });
  return res;
}

// ==================== 表情回复 ====================

/** 给消息添加表情回复 */
export async function addReaction(messageId: string, emojiType: string) {
  const res = await client().im.message.reaction.create({
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
  const res = await client().im.message.reaction.delete({
    path: {
      message_id: messageId,
      reaction_id: reactionId,
    },
  });
  return res;
}

/** 获取消息的表情回复列表 */
export async function getReactions(messageId: string, emojiType?: string) {
  const res = await client().im.message.reaction.list({
    path: {
      message_id: messageId,
    },
    params: {
      ...(emojiType ? { reaction_type: emojiType } : {}),
    },
  });
  return res;
}
