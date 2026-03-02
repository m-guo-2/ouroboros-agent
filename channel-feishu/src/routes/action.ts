/**
 * 统一 Action 端点
 * Agent 通过单一接口调用所有飞书能力
 *
 * POST /api/feishu/action
 * Body: { action: string, params: Record<string, unknown> }
 */

import { Router, type Request, type Response } from "express";
import * as messageService from "../services/message";
import * as meetingService from "../services/meeting";
import * as documentService from "../services/document";
import type { ReceiveIdType } from "../types";

const router = Router();

/**
 * Action 注册表
 * key: action name
 * value: handler function
 */
const actionHandlers: Record<
  string,
  (params: Record<string, unknown>) => Promise<unknown>
> = {
  // ==================== 消息能力 ====================

  /** 发送文本消息 */
  send_text: async (p) =>
    messageService.sendText({
      receiveId: p.receive_id as string,
      receiveIdType: ((p.receive_id_type as string) || "chat_id") as ReceiveIdType,
      text: p.text as string,
    }),

  /** 发送富文本消息 */
  send_rich_text: async (p) =>
    messageService.sendRichText({
      receiveId: p.receive_id as string,
      receiveIdType: ((p.receive_id_type as string) || "chat_id") as ReceiveIdType,
      title: p.title as string,
      content: p.content as any[][],
    }),

  /** 发送交互卡片消息 */
  send_card: async (p) =>
    messageService.sendCard({
      receiveId: p.receive_id as string,
      receiveIdType: ((p.receive_id_type as string) || "chat_id") as ReceiveIdType,
      templateId: p.template_id as string | undefined,
      templateVariable: p.template_variable as Record<string, string> | undefined,
      cardContent: p.card_content as Record<string, unknown> | undefined,
    }),

  /** 发送简单卡片消息 */
  send_default_card: async (p) =>
    messageService.sendDefaultCard({
      receiveId: p.receive_id as string,
      receiveIdType: ((p.receive_id_type as string) || "chat_id") as ReceiveIdType,
      title: p.title as string,
      content: p.content as string,
    }),

  /** 发送图片消息 */
  send_image: async (p) =>
    messageService.sendImage({
      receiveId: p.receive_id as string,
      receiveIdType: ((p.receive_id_type as string) || "chat_id") as ReceiveIdType,
      imageKey: p.image_key as string,
    }),

  /** 发送文件消息 */
  send_file: async (p) =>
    messageService.sendFile({
      receiveId: p.receive_id as string,
      receiveIdType: ((p.receive_id_type as string) || "chat_id") as ReceiveIdType,
      fileKey: p.file_key as string,
    }),

  /** 发送语音消息 */
  send_audio: async (p) =>
    messageService.sendAudio({
      receiveId: p.receive_id as string,
      receiveIdType: ((p.receive_id_type as string) || "chat_id") as ReceiveIdType,
      fileKey: p.file_key as string,
    }),

  /** 从 URL 上传图片到飞书，返回 image_key */
  upload_image_from_url: async (p) =>
    messageService.uploadImageFromUrl(
      p.image_url as string,
      (p.image_type as string) || "message",
    ),

  /** 从 URL 上传文件到飞书，返回 file_key */
  upload_file_from_url: async (p) =>
    messageService.uploadFileFromUrl({
      fileUrl: p.file_url as string,
      fileName: p.file_name as string,
      fileType: p.file_type as string,
      duration: p.duration as string | undefined,
    }),

  /** 回复消息 */
  reply_message: async (p) =>
    messageService.replyMessage({
      messageId: p.message_id as string,
      content: p.content as string,
      msgType: (p.msg_type as string) || "text",
    }),

  /** 撤回消息 */
  recall_message: async (p) =>
    messageService.recallMessage(p.message_id as string),

  /** 获取消息详情 */
  get_message: async (p) =>
    messageService.getMessage(p.message_id as string),

  /** 获取会话消息列表 */
  get_message_list: async (p) =>
    messageService.getMessageList(
      p.chat_id as string,
      (p.page_size as number) || 20,
      p.page_token as string | undefined
    ),

  // ==================== 表情回复 ====================

  /** 给消息添加表情回复 */
  add_reaction: async (p) =>
    messageService.addReaction(
      p.message_id as string,
      p.emoji_type as string,
    ),

  /** 删除表情回复 */
  delete_reaction: async (p) =>
    messageService.deleteReaction(
      p.message_id as string,
      p.reaction_id as string,
    ),

  /** 获取消息的表情回复列表 */
  get_reactions: async (p) =>
    messageService.getReactions(
      p.message_id as string,
      p.emoji_type as string | undefined,
    ),

  // ==================== 群组能力 ====================

  /** 创建群组 */
  create_chat: async (p) =>
    messageService.createChat({
      name: p.name as string,
      description: p.description as string | undefined,
      userIdList: p.user_id_list as string[] | undefined,
    }),

  /** 获取群信息（名称、描述、群主等） */
  get_chat_info: async (p) =>
    messageService.getChatInfo(p.chat_id as string),

  /** 获取群成员列表 */
  get_chat_members: async (p) =>
    messageService.getChatMembers(p.chat_id as string),

  /** 获取机器人所在的所有群列表 */
  list_bot_chats: async () =>
    messageService.listBotChats(),

  /** 搜索群组（按关键词） */
  search_chats: async (p) =>
    messageService.searchChats(
      p.query as string,
      (p.page_size as number) || 20
    ),

  // ==================== 用户/联系人能力 ====================

  /** 通过邮箱或手机号查找用户 ID */
  batch_get_user_id: async (p) =>
    messageService.batchGetUserId({
      emails: p.emails as string[] | undefined,
      mobiles: p.mobiles as string[] | undefined,
    }),

  /** 获取用户详细信息 */
  get_user_info: async (p) =>
    messageService.getUserInfo(
      p.user_id as string,
      (p.user_id_type as string) || "open_id"
    ),

  // ==================== 会议能力 ====================

  /** 预约会议 */
  reserve_meeting: async (p) =>
    meetingService.reserveMeeting({
      topic: p.topic as string,
      startTime: (p.start_time as string) || String(Math.floor(Date.now() / 1000)),
      endTime: p.end_time as string,
      invitees: p.invitees as Array<{ id: string; idType: "open_id" | "user_id" | "union_id" }> | undefined,
      settings: p.settings as { password?: string } | undefined,
    }),

  /** 获取会议详情 */
  get_meeting: async (p) =>
    meetingService.getMeeting(p.meeting_id as string),

  /** 邀请参会人 */
  invite_to_meeting: async (p) =>
    meetingService.inviteToMeeting(
      p.meeting_id as string,
      p.invitees as Array<{ id: string; userType?: number }>
    ),

  /** 结束会议 */
  end_meeting: async (p) =>
    meetingService.endMeeting(p.meeting_id as string),

  /** 开始录制 */
  start_recording: async (p) =>
    meetingService.startRecording(p.meeting_id as string),

  /** 停止录制 */
  stop_recording: async (p) =>
    meetingService.stopRecording(p.meeting_id as string),

  /** 获取录制列表 */
  get_meeting_recording: async (p) =>
    meetingService.getMeetingRecordingList(p.meeting_id as string),

  // ==================== 文档能力 ====================

  /** 创建文档 */
  create_document: async (p) =>
    documentService.createDocument({
      title: p.title as string,
      folderToken: p.folder_token as string | undefined,
    }),

  /** 获取文档信息 */
  get_document: async (p) =>
    documentService.getDocument(p.document_id as string),

  /** 获取文档纯文本内容 */
  get_document_content: async (p) =>
    documentService.getDocumentRawContent(p.document_id as string),

  /** 获取文档所有块 */
  get_document_blocks: async (p) =>
    documentService.getDocumentBlocks(p.document_id as string),

  /** 追加文档内容 */
  append_document: async (p) =>
    documentService.appendDocumentBlocks(
      p.document_id as string,
      p.block_id as string,
      p.blocks as any[]
    ),

  // ==================== 知识库能力 ====================

  /** 获取知识库列表 */
  get_wiki_spaces: async () =>
    documentService.getWikiSpaces(),

  /** 获取知识库节点 */
  get_wiki_node: async (p) =>
    documentService.getWikiNode(
      p.space_id as string,
      p.node_token as string
    ),

  /** 创建知识库节点 */
  create_wiki_node: async (p) =>
    documentService.createWikiNode({
      spaceId: p.space_id as string,
      parentNodeToken: p.parent_node_token as string | undefined,
      title: p.title as string,
      nodeType: p.node_type as "origin" | "shortcut" | undefined,
    }),

  // ==================== 云空间能力 ====================

  /** 获取根文件夹 */
  get_root_folder: async () =>
    documentService.getRootFolder(),

  /** 获取文件夹内容 */
  get_folder_contents: async (p) =>
    documentService.getFolderContents(p.folder_token as string),

  /** 创建文件夹 */
  create_folder: async (p) =>
    documentService.createFolder(
      p.name as string,
      p.folder_token as string | undefined
    ),
};

// ==================== 统一端点 ====================

/**
 * POST /api/feishu/action
 *
 * Body:
 * {
 *   "action": "send_text",
 *   "params": {
 *     "receive_id": "oc_xxx",
 *     "text": "Hello!"
 *   }
 * }
 */
router.post("/", async (req: Request, res: Response) => {
  const { action, params = {} } = req.body;

  if (!action) {
    res.status(400).json({
      success: false,
      error: "Missing 'action' field",
      available_actions: Object.keys(actionHandlers),
    });
    return;
  }

  const handler = actionHandlers[action];
  if (!handler) {
    res.status(400).json({
      success: false,
      error: `Unknown action: ${action}`,
      available_actions: Object.keys(actionHandlers),
    });
    return;
  }

  try {
    const result = await handler(params);
    res.json({ success: true, data: result });
  } catch (err: any) {
    console.error(`[Action Error] ${action}:`, err);
    console.error(`[Action Error Detail] ${action}:`, JSON.stringify(err, null, 2));
    res.status(500).json({
      success: false,
      error: err.message || "Internal error",
      action,
    });
  }
});

/**
 * GET /api/feishu/action/list
 * 列出所有可用的 action
 */
router.get("/list", (_req: Request, res: Response) => {
  const actions = Object.keys(actionHandlers).map((name) => {
    // 从 action 名称推断分类
    let category = "other";
    if (name.startsWith("send_") || name.startsWith("reply_") || name.startsWith("recall_") || name.startsWith("get_message")) {
      category = "message";
    } else if (name.includes("chat") || name.includes("member")) {
      category = "chat";
    } else if (name.includes("meeting") || name.includes("recording")) {
      category = "meeting";
    } else if (name.includes("document") || name.includes("append")) {
      category = "document";
    } else if (name.includes("wiki")) {
      category = "wiki";
    } else if (name.includes("folder")) {
      category = "drive";
    }
    return { name, category };
  });

  res.json({
    success: true,
    total: actions.length,
    actions,
  });
});

export default router;
