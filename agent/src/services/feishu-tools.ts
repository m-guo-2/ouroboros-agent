/**
 * Feishu Tools — Agent 内置飞书工具
 *
 * 通过 HTTP 调用 channel-feishu 的统一 Action 端点 (POST /api/feishu/action)。
 * 这些工具作为 MCP tools 注册到 Claude Agent SDK，供 Agent 直接调用。
 *
 * 工具覆盖：
 * - 消息：发送文本、发送富文本、回复消息
 * - 文档：创建文档、读取文档内容、追加文档内容
 * - 表情：添加/删除表情回复
 * - 查询：查群信息、查用户信息
 */

import { z, type ZodRawShape } from "zod";

/** channel-feishu 服务地址 */
const FEISHU_ACTION_URL =
  process.env.FEISHU_ACTION_URL || "http://localhost:1999/api/feishu/action";

// ==================== Feishu Action 调用器 ====================

/**
 * 调用 channel-feishu 的统一 Action 端点。
 * 所有飞书工具最终都通过此函数发出 HTTP 请求。
 */
async function callFeishuAction(
  action: string,
  params: Record<string, unknown>,
): Promise<unknown> {
  const response = await fetch(FEISHU_ACTION_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action, params }),
  });

  const text = await response.text();
  if (!response.ok) {
    throw new Error(`Feishu action "${action}" failed: ${response.status} ${text}`);
  }

  try {
    const json = JSON.parse(text);
    if (json.success === false) {
      throw new Error(`Feishu action "${action}" error: ${json.error || text}`);
    }
    return json.data ?? json;
  } catch {
    return text;
  }
}

// ==================== 工具定义 ====================

export interface FeishuToolDef {
  name: string;
  description: string;
  shape: ZodRawShape;
  action: string;
  /** 将 SDK 传入的 args 映射为 feishu action params（可选，默认透传） */
  mapArgs?: (args: Record<string, unknown>) => Record<string, unknown>;
}

/**
 * 所有内置飞书工具定义。
 *
 * 命名规则：feishu_<action>
 * 每个工具对应 channel-feishu 的一个 action。
 */
export const FEISHU_TOOL_DEFS: FeishuToolDef[] = [
  // ==================== 消息 ====================
  {
    name: "feishu_send_text",
    description:
      "向飞书用户或群聊发送文本消息。receive_id 默认为群聊 chat_id（oc_xxx），" +
      "发给个人需设置 receive_id_type 为 open_id 或 email。",
    shape: {
      receive_id: z.string().describe("接收方 ID（群聊 chat_id 或用户 open_id / email）"),
      text: z.string().describe("消息文本内容"),
      receive_id_type: z
        .enum(["chat_id", "open_id", "user_id", "union_id", "email"])
        .optional()
        .describe("接收方 ID 类型，默认 chat_id"),
    },
    action: "send_text",
  },

  {
    name: "feishu_send_rich_text",
    description:
      "发送飞书富文本消息（支持加粗、链接、@人等）。content 是二维数组，外层为段落，内层为行内元素。",
    shape: {
      receive_id: z.string().describe("接收方 ID"),
      title: z.string().describe("富文本标题"),
      content: z
        .array(z.array(z.any()))
        .describe(
          '二维数组。每个元素格式如 {tag:"text",text:"内容"} / {tag:"a",href:"url",text:"链接"} / {tag:"at",user_id:"ou_xxx"}',
        ),
      receive_id_type: z
        .enum(["chat_id", "open_id", "user_id", "union_id", "email"])
        .optional()
        .describe("接收方 ID 类型，默认 chat_id"),
    },
    action: "send_rich_text",
  },

  {
    name: "feishu_send_card",
    description:
      "发送飞书简单卡片通知消息（含标题和内容正文）。适合快速通知场景。",
    shape: {
      receive_id: z.string().describe("接收方 ID"),
      title: z.string().describe("卡片标题"),
      content: z.string().describe("卡片正文内容"),
      receive_id_type: z
        .enum(["chat_id", "open_id", "user_id", "union_id", "email"])
        .optional()
        .describe("接收方 ID 类型，默认 chat_id"),
    },
    action: "send_default_card",
  },

  {
    name: "feishu_reply_message",
    description: "回复飞书中的指定消息。需要消息的 message_id。",
    shape: {
      message_id: z.string().describe("要回复的消息 ID（om_xxx）"),
      content: z.string().describe("回复内容（JSON 字符串，如 {\"text\":\"回复文字\"}）"),
      msg_type: z.string().optional().describe("消息类型，默认 text"),
    },
    action: "reply_message",
  },

  // ==================== 表情回复 ====================
  {
    name: "feishu_add_reaction",
    description:
      "给飞书消息添加表情回复（emoji reaction）。常用 emoji_type：" +
      "THUMBSUP（👍）、HEART（❤️）、SMILE（😊）、LAUGH（😂）、" +
      "CLAP（👏）、FIRE（🔥）、OK（👌）、ROCKET（🚀）、" +
      "FISTBUMP（🤜🤛）、MUSCLE（💪）、PARTY（🎉）",
    shape: {
      message_id: z.string().describe("目标消息 ID（om_xxx）"),
      emoji_type: z.string().describe("表情类型，如 THUMBSUP / SMILE / HEART / FIRE / ROCKET"),
    },
    action: "add_reaction",
  },

  {
    name: "feishu_delete_reaction",
    description: "删除飞书消息上的表情回复。",
    shape: {
      message_id: z.string().describe("目标消息 ID"),
      reaction_id: z.string().describe("要删除的 reaction ID"),
    },
    action: "delete_reaction",
  },

  // ==================== 图片 ====================
  {
    name: "feishu_upload_image",
    description:
      "从 URL 下载图片并上传到飞书，返回 image_key。后续用 feishu_send_image 发送。" +
      "支持 jpg/png/gif/webp 等常见图片格式。",
    shape: {
      image_url: z.string().describe("图片的完整 URL（http/https）"),
      image_type: z
        .enum(["message", "avatar"])
        .optional()
        .describe("图片用途，默认 message"),
    },
    action: "upload_image_from_url",
  },

  {
    name: "feishu_send_image",
    description:
      "向飞书用户或群聊发送图片消息。需要先通过 feishu_upload_image 获取 image_key。",
    shape: {
      receive_id: z.string().describe("接收方 ID（chat_id / open_id）"),
      image_key: z.string().describe("飞书图片 key（通过 feishu_upload_image 获取）"),
      receive_id_type: z
        .enum(["chat_id", "open_id", "user_id", "union_id", "email"])
        .optional()
        .describe("接收方 ID 类型，默认 chat_id"),
    },
    action: "send_image",
  },

  // ==================== 语音 / 文件 ====================
  {
    name: "feishu_upload_file",
    description:
      "从 URL 下载文件并上传到飞书，返回 file_key。" +
      "file_type 可选：opus（语音）、mp4（视频）、pdf、doc、xls、ppt、stream（通用二进制）。" +
      "上传语音时需额外传 duration（毫秒字符串）。",
    shape: {
      file_url: z.string().describe("文件的完整 URL（http/https）"),
      file_name: z.string().describe("文件名（含扩展名，如 audio.opus、report.pdf）"),
      file_type: z
        .enum(["opus", "mp4", "pdf", "doc", "xls", "ppt", "stream"])
        .describe("文件类型"),
      duration: z.string().optional().describe("语音时长（毫秒），仅 opus 类型需要"),
    },
    action: "upload_file_from_url",
  },

  {
    name: "feishu_send_audio",
    description:
      "向飞书用户或群聊发送语音消息。需先通过 feishu_upload_file（file_type=opus）获取 file_key。",
    shape: {
      receive_id: z.string().describe("接收方 ID（chat_id / open_id）"),
      file_key: z.string().describe("飞书文件 key（通过 feishu_upload_file 获取）"),
      receive_id_type: z
        .enum(["chat_id", "open_id", "user_id", "union_id", "email"])
        .optional()
        .describe("接收方 ID 类型，默认 chat_id"),
    },
    action: "send_audio",
  },

  {
    name: "feishu_send_file",
    description:
      "向飞书用户或群聊发送文件消息。需先通过 feishu_upload_file 获取 file_key。",
    shape: {
      receive_id: z.string().describe("接收方 ID（chat_id / open_id）"),
      file_key: z.string().describe("飞书文件 key（通过 feishu_upload_file 获取）"),
      receive_id_type: z
        .enum(["chat_id", "open_id", "user_id", "union_id", "email"])
        .optional()
        .describe("接收方 ID 类型，默认 chat_id"),
    },
    action: "send_file",
  },

  // ==================== 文档 ====================
  {
    name: "feishu_create_document",
    description:
      "创建飞书文档。返回 document_id，可用于后续读写操作。可选指定 folder_token 创建到特定目录。",
    shape: {
      title: z.string().describe("文档标题"),
      folder_token: z.string().optional().describe("目标文件夹 token（可选，默认根目录）"),
    },
    action: "create_document",
  },

  {
    name: "feishu_get_document_content",
    description: "获取飞书文档的纯文本内容。返回文档的全文文本。",
    shape: {
      document_id: z.string().describe("文档 ID"),
    },
    action: "get_document_content",
  },

  {
    name: "feishu_get_document",
    description: "获取飞书文档的元信息（标题、创建时间、修改时间等）。",
    shape: {
      document_id: z.string().describe("文档 ID"),
    },
    action: "get_document",
  },

  {
    name: "feishu_append_document",
    description:
      "向飞书文档追加内容块。支持段落、标题、代码块、分割线等。" +
      "block_id 追加到文档根级别时与 document_id 相同。",
    shape: {
      document_id: z.string().describe("文档 ID"),
      block_id: z.string().describe("父块 ID（追加到文档根级别时填 document_id）"),
      blocks: z
        .array(z.any())
        .describe(
          '内容块数组。格式如 [{blockType:"paragraph",text:"内容",style:"heading1"}, {blockType:"divider"}]',
        ),
    },
    action: "append_document",
  },

  // ==================== 信息查询 ====================
  {
    name: "feishu_search_chats",
    description: "按关键词搜索飞书群组。返回匹配的群名称和 chat_id。",
    shape: {
      query: z.string().describe("搜索关键词（模糊匹配群名称）"),
      page_size: z.number().optional().describe("返回数量，默认 20"),
    },
    action: "search_chats",
  },

  {
    name: "feishu_get_chat_info",
    description: "获取飞书群详细信息（群名称、描述、群主、成员数量等）。",
    shape: {
      chat_id: z.string().describe("群聊 ID（oc_xxx）"),
    },
    action: "get_chat_info",
  },

  {
    name: "feishu_get_chat_members",
    description: "获取飞书群全部成员列表（姓名、open_id、角色）。",
    shape: {
      chat_id: z.string().describe("群聊 ID（oc_xxx）"),
    },
    action: "get_chat_members",
  },

  {
    name: "feishu_list_bot_chats",
    description: "列出机器人所在的所有飞书群组（群名、chat_id、描述）。",
    shape: {},
    action: "list_bot_chats",
  },

  {
    name: "feishu_get_user_info",
    description: "获取飞书用户详细信息（姓名、头像、部门、工号等）。",
    shape: {
      user_id: z.string().describe("用户 ID（open_id 格式 ou_xxx）"),
      user_id_type: z.string().optional().describe("ID 类型，默认 open_id"),
    },
    action: "get_user_info",
  },

  {
    name: "feishu_batch_get_user_id",
    description: "通过邮箱或手机号批量查找飞书用户的 open_id。",
    shape: {
      emails: z.array(z.string()).optional().describe("邮箱列表"),
      mobiles: z.array(z.string()).optional().describe("手机号列表"),
    },
    action: "batch_get_user_id",
  },
];

// ==================== 工具执行器 ====================

/**
 * 执行飞书工具调用。
 * 根据工具定义中的 action 字段调用 channel-feishu。
 */
export async function executeFeishuTool(
  def: FeishuToolDef,
  args: Record<string, unknown>,
): Promise<unknown> {
  const params = def.mapArgs ? def.mapArgs(args) : args;
  return callFeishuAction(def.action, params);
}

/**
 * 获取所有飞书工具名称集合（用于去重判断）。
 */
export function getFeishuToolNames(): Set<string> {
  return new Set(FEISHU_TOOL_DEFS.map((d) => d.name));
}
