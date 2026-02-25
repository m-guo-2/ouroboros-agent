// ==================== 统一发送接口类型 ====================

/** 消息内容类型（联合类型，按 type 区分） */
export type SendMessageContent =
  | { type: "text"; text: string }
  | { type: "rich_text"; title?: string; content: RichTextContent[][] }
  | { type: "card"; templateId: string; templateVariable?: Record<string, string> }
  | { type: "card"; cardContent: object }
  | { type: "image"; imageKey: string }
  | { type: "file"; fileKey: string }
  | { type: "audio"; fileKey: string }
  | { type: "video"; fileKey: string; imageKey: string };

/**
 * 统一消息发送请求
 * POST /api/feishu/send
 */
export interface SendRequest {
  /** 发送目标：群ID (chat_id) 或 用户ID (open_id) */
  receiveId: string;
  /** ID 类型，默认 "chat_id" */
  receiveIdType?: ReceiveIdType;
  /** 引用回复的消息ID（可选） */
  replyToMessageId?: string;
  /** @的用户ID列表（可选，仅文本/富文本有效） */
  mentions?: string[];
  /** 消息内容（JSON 对象，按 type 区分消息类型） */
  content: SendMessageContent;
}

// ==================== 消息相关类型 ====================

/** 发送文本消息的参数 */
export interface SendTextParams {
  /** 接收者 ID（chat_id / open_id / user_id / union_id / email） */
  receiveId: string;
  /** 接收者 ID 类型 */
  receiveIdType: ReceiveIdType;
  /** 消息文本内容 */
  text: string;
}

/** 发送富文本消息的参数 */
export interface SendRichTextParams {
  receiveId: string;
  receiveIdType: ReceiveIdType;
  /** 富文本内容标题 */
  title: string;
  /** 富文本内容（飞书 post 格式） */
  content: RichTextContent[][];
}

/** 富文本内容元素 */
export type RichTextContent =
  | { tag: "text"; text: string; style?: string[] }
  | { tag: "a"; text: string; href: string }
  | { tag: "at"; user_id: string; user_name?: string }
  | { tag: "img"; image_key: string; width?: number; height?: number }
  | { tag: "media"; file_key: string; image_key?: string }
  | { tag: "emotion"; emoji_type: string };

/** 发送交互卡片消息的参数 */
export interface SendCardParams {
  receiveId: string;
  receiveIdType: ReceiveIdType;
  /** 卡片模板 ID（使用模板消息时） */
  templateId?: string;
  /** 模板变量（使用模板消息时） */
  templateVariable?: Record<string, string>;
  /** 卡片 JSON 内容（不使用模板时） */
  cardContent?: object;
}

/** 发送图片消息的参数 */
export interface SendImageParams {
  receiveId: string;
  receiveIdType: ReceiveIdType;
  /** 图片 key（需先上传） */
  imageKey: string;
}

/** 发送文件消息的参数 */
export interface SendFileParams {
  receiveId: string;
  receiveIdType: ReceiveIdType;
  /** 文件 key（需先上传） */
  fileKey: string;
}

/** 上传图片的参数 */
export interface UploadImageParams {
  /** 图片类型: message（用于发送消息）/ avatar（用于设置头像） */
  imageType: "message" | "avatar";
  /** 图片二进制数据 */
  image: Buffer;
}

/** 上传文件的参数 */
export interface UploadFileParams {
  /** 文件类型 */
  fileType: "opus" | "mp4" | "pdf" | "doc" | "xls" | "ppt" | "stream";
  /** 文件名 */
  fileName: string;
  /** 文件二进制数据 */
  file: Buffer;
  /** 文件时长（音频/视频，单位毫秒） */
  duration?: number;
}

/** 回复消息的参数 */
export interface ReplyMessageParams {
  /** 被回复消息的 ID */
  messageId: string;
  /** 消息类型 */
  msgType: string;
  /** 消息内容（JSON 字符串） */
  content: string;
}

/** 接收者 ID 类型 */
export type ReceiveIdType =
  | "open_id"
  | "user_id"
  | "union_id"
  | "email"
  | "chat_id";

// ==================== 会议相关类型 ====================

/** 创建会议的参数 */
export interface CreateMeetingParams {
  /** 会议主题 */
  topic: string;
  /** 开始时间（Unix 时间戳，秒） */
  startTime?: string;
  /** 结束时间（Unix 时间戳，秒） */
  endTime?: string;
  /** 参会人 open_id 列表 */
  invitees?: string[];
}

/** 预约会议的参数 */
export interface ReserveMeetingParams {
  /** 会议主题 */
  topic: string;
  /** 开始时间（Unix 时间戳，秒） */
  startTime: string;
  /** 结束时间（Unix 时间戳，秒） */
  endTime: string;
  /** 参会人列表 */
  invitees?: Array<{
    id: string;
    idType: "open_id" | "user_id" | "union_id";
  }>;
  /** 会议设置 */
  settings?: {
    /** 入会密码 */
    password?: string;
  };
}

// ==================== 文档相关类型 ====================

/** 创建文档的参数 */
export interface CreateDocumentParams {
  /** 文档标题 */
  title: string;
  /** 文档所在文件夹 token（可选） */
  folderToken?: string;
}

/** 追加文档内容的参数 */
export interface AppendDocumentContentParams {
  /** 文档 token */
  documentId: string;
  /** 要追加的内容块 */
  blocks: DocumentBlock[];
}

/** 文档内容块 */
export type DocumentBlock =
  | {
      blockType: "paragraph";
      /** 段落文本内容 */
      text: string;
      /** 文本样式 */
      style?: "heading1" | "heading2" | "heading3" | "heading4" | "normal";
    }
  | {
      blockType: "code";
      /** 代码内容 */
      code: string;
      /** 编程语言 */
      language?: string;
    }
  | {
      blockType: "callout";
      /** 高亮内容 */
      text: string;
    }
  | {
      blockType: "divider";
    };

/** 创建知识库节点的参数 */
export interface CreateWikiNodeParams {
  /** 知识库 space_id */
  spaceId: string;
  /** 父节点 token（可选，不填则在根目录） */
  parentNodeToken?: string;
  /** 节点标题 */
  title: string;
  /** 节点类型 */
  nodeType?: "origin" | "shortcut";
}

// ==================== 事件回调类型 ====================

/** 消息接收事件处理函数 */
export type MessageHandler = (
  message: ReceivedMessage
) => Promise<void> | void;

/** 收到的消息结构 */
export interface ReceivedMessage {
  /** 消息 ID */
  messageId: string;
  /** 消息所在的 chat_id */
  chatId: string;
  /** 消息类型 */
  messageType: string;
  /** 消息内容（JSON 字符串） */
  content: string;
  /** 发送者信息 */
  sender: {
    senderId: {
      open_id: string;
      user_id: string;
      union_id: string;
    };
    senderType: string;
    tenantKey: string;
  };
  /** 创建时间 */
  createTime: string;
  /** 是否 @了机器人 */
  mentionedBot: boolean;
}

// ==================== API 响应 ====================

export interface ApiResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
}
