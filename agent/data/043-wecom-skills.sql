-- 043-wecom-skills.sql
-- 企微 Skill 渐进式加载数据初始化
-- 对应决策文档: docs/decisions/043-wecom-skill-progressive-loading.md
--
-- 使用方式: sqlite3 data/config.db < agent/data/043-wecom-skills.sql
-- 注意: QIWEI_URL 默认 http://localhost:2000，按实际部署调整

-- ============================================================
-- 1. 更新 moli agent 的 systemPrompt 和 skills 绑定
-- ============================================================

UPDATE agent_configs
SET system_prompt = '你是 moli，一个运行在企业微信上的 AI 助手。你通过企微与用户沟通，帮助处理日常工作中的消息收发、联系人查询、群组管理等事务。

## 核心原则

- 你的文字输出（推理过程、思考内容）对用户完全不可见
- 与外界的所有交互必须通过工具调用完成
- 需要回复用户时，调用 send_channel_message 工具发送消息
- 可以分多次发送，不必等所有工作完成后才回复
- 处理时间较长时，先发一条确认消息再继续

## send_channel_message 参数

- content（必填）：要发送的消息内容
- messageType（可选）：text（默认）/ image / file / rich_text
- channel / channelUserId / channelConversationId：默认取自消息来源，通常无需手动填写
- replyToChannelMessageId（可选）：要回复的上游消息 ID

## 行为准则

- 用自然、简洁的中文交流，像同事间的对话
- 遇到不确定的事情，诚实说明而非编造
- 涉及敏感操作（删除联系人、解散群等）时，先确认再执行
- 当需要使用扩展能力时，通过 load_skill 加载对应技能文档

## 消息格式协议

历史消息只包含客观事件记录，不包含你过去的推理过程：
- user 消息 = 某个用户发来的内容。消息头格式：[昵称 (渠道ID) | via 渠道 | type=消息类型]。via 表示来源渠道（feishu/wecom/webui），type 仅在非文本消息时出现（image/file/audio 等）
- assistant 消息（tool_use block）= 你过去执行的工具调用动作
- tool_result block = 工具执行返回的客观结果
- 你过去通过 send_channel_message 发送的内容会出现在对应的 tool_use 和 tool_result 中

{{skills}}',
    skills = '["feishu-agent","wecom-core"]',
    updated_at = CURRENT_TIMESTAMP
WHERE id = 'default-agent-config';

-- ============================================================
-- 2. 删除旧的 qiwei-agent skill（如果存在）
-- ============================================================

DELETE FROM skills WHERE id = 'qiwei-agent';

-- ============================================================
-- 3. 创建 wecom-core（永久加载的核心 skill）
-- ============================================================

INSERT OR REPLACE INTO skills (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
VALUES (
  'wecom-core',
  '企微核心能力',
  '面向 agent-human communication 的 4 个企微语义化工具：搜索对象、读取会话、解析消息、发送消息',
  '1.0.0',
  'action',
  1,
  '[]',
  -- tools: 4 个合并后的核心工具
  '[
    {
      "name": "wecom_send_message",
      "description": "向指定企微联系人或群聊主动发送消息。支持 text、rich_text、image、file、voice。私聊时填 channelUserId，群聊时填 channelConversationId；如果只是回复当前会话，请优先使用 send_channel_message。这个工具屏蔽了企微底层 sendText/sendHyperText/sendFile 等差异，是 agent 主动沟通的统一发送入口。",
      "inputSchema": {
        "type": "object",
        "properties": {
          "channelConversationId": {"type": "string", "description": "群聊 ID（群消息时填写）"},
          "channelUserId": {"type": "string", "description": "联系人 ID（私聊时填写）"},
          "messageType": {"type": "string", "description": "消息类型：text（默认）/ rich_text / image / file / voice"},
          "content": {"type": "string", "description": "消息内容。text/rich_text 填文字或富文本内容；image/file/voice 填可访问 URL"},
          "channelMeta": {"type": "object", "description": "附加信息，如 file 类型时传 {\"fileName\": \"报告.pdf\"}"}
        },
        "required": ["content"]
      },
      "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/send_message", "method": "POST"}
    },
    {
      "name": "wecom_search_targets",
      "description": "搜索企微中的沟通对象，统一覆盖联系人和群聊。输入关键词后返回可直接沟通的 targets，每个结果都带 type、id、name，避免模型先记联系人接口、再记群接口。适用于“找张三”“找产品群”“看看有没有这个客户”这类定位目标场景。",
      "inputSchema": {
        "type": "object",
        "properties": {
          "query": {"type": "string", "description": "搜索关键词，可以是姓名、备注、手机号、企业名或群名。不填时返回默认列表"},
          "limit": {"type": "integer", "description": "返回结果上限，默认 20"},
          "includeContacts": {"type": "boolean", "description": "是否搜索联系人，默认 true"},
          "includeGroups": {"type": "boolean", "description": "是否搜索群聊，默认 true"}
        }
      },
      "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/search_targets", "method": "POST"}
    },
    {
      "name": "wecom_list_or_get_conversations",
      "description": "统一处理企微会话读取。不给 conversationId 时，返回最近会话列表；给了 conversationId 时，返回该会话的历史消息。这样模型只需记住一个会话工具，不需要分别记 list_sessions 和 sync_history。适用于先浏览最近对话，再深入读取某个会话上下文。",
      "inputSchema": {
        "type": "object",
        "properties": {
          "conversationId": {"type": "string", "description": "目标会话 ID。留空时列最近会话；填写后读取该会话历史消息"},
          "msgSvrId": {"type": "string", "description": "读取历史消息时的翻页起点。留空则从最新消息开始"},
          "currentSeq": {"type": "number", "description": "列会话时的分页游标，首次传 0"},
          "pageSize": {"type": "number", "description": "列会话时每页数量，默认使用服务端默认值"}
        }
      },
      "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/list_or_get_conversations", "method": "POST"}
    },
    {
      "name": "wecom_parse_message",
      "description": "解析企微消息内容，统一处理文本、图片、文件、语音等输入。可传原始 message、messageType+msgData，或直接传 qiwei 已准备好的 resourceUri；旧的 localPath 仍兼容。语音默认返回转写文本；图片/文件在拿到资源地址后可进一步解析内容。适用于非纯文本消息，尤其是图片、文件、语音需要进一步理解时。",
      "inputSchema": {
        "type": "object",
        "properties": {
          "message": {"type": "object", "description": "原始消息对象。推荐直接传 list_or_get_conversations 返回的某条 raw message"},
          "messageType": {"type": "string", "description": "消息类型。若未传 message，可单独传 text / image / file / voice / rich_text"},
          "msgData": {"type": "object", "description": "消息载荷。若未传完整 message，可用这个字段传原始 msgData"},
          "resourceUri": {"type": "string", "description": "qiwei 已准备好的资源地址，例如 oss://bucket/key。适合对图片/文件做二次理解时直接传入"},
          "localPath": {"type": "string", "description": "兼容旧参数，效果等同于 resourceUri"}
        }
      },
      "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/parse_message", "method": "POST"}
    },
    {
      "name": "inspect_attachment",
      "description": "按 attachmentId 按需分析当前会话里的结构化附件。图片可做 describe_image 或 ocr_image，文件可做 extract_text 或 summarize_document；语音已经在入口层前置转写，不需要通过这个工具处理。",
      "inputSchema": {
        "type": "object",
        "properties": {
          "attachmentId": {"type": "string", "description": "附件 ID。来自用户消息 [attachments] 段中的 id 字段"},
          "task": {"type": "string", "description": "分析任务：describe_image / ocr_image / extract_text / summarize_document / summarize_video"}
        },
        "required": ["attachmentId"]
      },
      "executor": {"type": "builtin", "handler": "inspect_attachment"}
    },
    {
      "name": "wecom_api",
      "description": "兼容入口：企微通用 API 透传工具。仅当 load_skill 加载的旧扩展技能明确要求 method + params 调用时再使用。日常沟通优先使用 wecom_search_targets、wecom_list_or_get_conversations、inspect_attachment、wecom_parse_message、wecom_send_message 这些语义化工具。",
      "inputSchema": {
        "type": "object",
        "properties": {
          "method": {"type": "string", "description": "API 方法路径，如 /msg/revokeMsg、/room/createRoom"},
          "params": {"type": "object", "description": "方法参数（不含 guid，系统自动注入）"}
        },
        "required": ["method"]
      },
      "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/do", "method": "POST"}
    }
  ]',
  -- readme
  '## 企微核心能力

### wecom_send_message
向指定联系人或群聊发送消息。常用场景：
- 私聊：填 channelUserId
- 群聊：填 channelConversationId
- messageType 不填默认 text

### wecom_search_targets
统一搜索联系人和群聊，先定位沟通对象，再进行发送或读历史。

### wecom_list_or_get_conversations
统一读取会话。不给 conversationId 时看最近会话，给 conversationId 时看该会话历史消息。

### wecom_parse_message
统一解析企微消息，尤其用于图片、文件、语音等非纯文本内容。

### inspect_attachment
按 attachmentId 按需分析当前会话里的结构化附件。优先用于图片、文件、视频的进一步理解；如果只是看当前消息正文里的链接，不要自行猜参数，优先使用这个工具。

### wecom_api
兼容旧扩展技能的透传入口。只有 load_skill 文档明确要求 method + params 时才使用，日常沟通不要优先选它。',
  '{}'
);

-- ============================================================
-- 4. 创建按需加载的扩展 Skills
-- ============================================================

-- 4.1 群管理
INSERT OR REPLACE INTO skills (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
VALUES (
  'wecom-group-mgmt',
  '群管理',
  '创建群聊、管理群成员、设置群公告、转让群主等群组管理操作',
  '1.0.0',
  'knowledge',
  1,
  '[]',
  '[]',
  '## 群管理

通过 wecom_api 工具调用以下方法。所有 params 中无需传 guid（系统自动注入）。

### 创建群
- method: `/room/createRoom`
- params: `{memberIds: ["id1","id2"], roomName: "群名"}`

### 修改群名
- method: `/room/modifyRoomName`
- params: `{roomId: "群ID", name: "新群名"}`

### 修改群公告
- method: `/room/modifyRoomNotice`
- params: `{roomId: "群ID", notice: "公告内容"}`

### 添加群成员
- method: `/room/inviteRoomMember`
- params: `{roomId: "群ID", memberIds: ["id1"]}`

### 移除群成员
- method: `/room/removeRoomMember`
- params: `{roomId: "群ID", memberIds: ["id1"]}`

### 设置群管理员
- method: `/room/roomAddAdmin`
- params: `{roomId: "群ID", memberIds: ["id1"]}`

### 取消群管理员
- method: `/room/roomRemoveAdmin`
- params: `{roomId: "群ID", memberIds: ["id1"]}`

### 转让群主
- method: `/room/changeRoomMaster`
- params: `{roomId: "群ID", memberId: "新群主ID"}`

### 解散群
- method: `/room/dismissRoom`
- params: `{roomId: "群ID"}`

### 退出群
- method: `/room/quitRoom`
- params: `{roomId: "群ID"}`

### 获取群二维码
- method: `/room/getRoomQrCode`
- params: `{roomId: "群ID"}`

### 设置群内昵称
- method: `/room/modifyRoomNickname`
- params: `{roomId: "群ID", nickname: "昵称"}`',
  '{}'
);

-- 4.2 联系人管理
INSERT OR REPLACE INTO skills (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
VALUES (
  'wecom-contact-mgmt',
  '联系人管理',
  '添加好友、通过好友申请、修改联系人信息、删除联系人等',
  '1.0.0',
  'knowledge',
  1,
  '[]',
  '[]',
  '## 联系人管理

通过 wecom_api 工具调用以下方法。

### 获取联系人详情（批量）
- method: `/contact/batchGetUserinfo`
- params: `{userIds: ["id1","id2"]}`

### 列出个人微信联系人
- method: `/contact/getWxContactList`
- params: `{}`

### 列出企业微信联系人
- method: `/contact/getWxWorkContactList`
- params: `{}`

### 添加个人微信好友
- method: `/contact/addSearchWxContact`
- params: `{keyword: "手机号或微信号", verifyContent: "验证消息"}`

### 添加企业微信好友
- method: `/contact/addSearchWxWorkContact`
- params: `{keyword: "搜索词"}`

### 通过好友申请
- method: `/contact/agreeContact`
- params: `{encryptUserName: "加密用户名", ticket: "ticket"}`

### 修改个人联系人备注
- method: `/contact/updateWxContact`
- params: `{userId: "联系人ID", remark: "新备注"}`

### 修改企业联系人备注
- method: `/contact/updateWxWorkContact`
- params: `{userId: "联系人ID", remark: "新备注"}`

### 删除联系人
- method: `/contact/deleteContact`
- params: `{userId: "联系人ID"}`',
  '{}'
);

-- 4.3 消息管理
INSERT OR REPLACE INTO skills (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
VALUES (
  'wecom-message-mgmt',
  '消息管理',
  '撤回消息、置顶消息、群发消息、同步历史消息等',
  '1.0.0',
  'knowledge',
  1,
  '[]',
  '[]',
  '## 消息管理

通过 wecom_api 工具调用以下方法。

### 撤回消息
- method: `/msg/revokeMsg`
- params: `{msgSvrId: "消息ID", toId: "接收者ID"}`

### 置顶消息
- method: `/msg/roomTopMessageSet`
- params: `{roomId: "群ID", msgSvrId: "消息ID", action: 1}`
- action: 1=置顶, 0=取消置顶

### 列出置顶消息
- method: `/msg/roomTopMessageList`
- params: `{roomId: "群ID"}`

### 群发消息
- method: `/msg/sendGroupMsg`
- params: `{toIds: ["id1","id2"], content: "消息内容", msgType: 1}`

### 查询群发状态
- method: `/msg/sendGroupMsgStatus`
- params: `{msgId: "群发ID"}`

### 同步历史消息
- method: `/msg/syncMsg`
- params: `{toId: "会话ID", msgSvrId: "起始消息ID"}`

### 发送富文本消息
- method: `/msg/sendHyperText`
- params: `{toId: "接收者ID", content: "消息XML"}`

### 发送链接消息
- method: `/msg/sendLink`
- params: `{toId: "接收者ID", title: "标题", desc: "描述", linkUrl: "URL", imgUrl: "缩略图URL"}`

### 发送位置
- method: `/msg/sendLocation`
- params: `{toId: "接收者ID", longitude: "经度", latitude: "纬度", label: "地名"}`

### 发送名片
- method: `/msg/sendPersonalCard`
- params: `{toId: "接收者ID", userId: "名片用户ID"}`',
  '{}'
);

-- 4.4 朋友圈
INSERT OR REPLACE INTO skills (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
VALUES (
  'wecom-moment',
  '朋友圈',
  '浏览朋友圈、发布动态、点赞、评论等朋友圈操作',
  '1.0.0',
  'knowledge',
  1,
  '[]',
  '[]',
  '## 朋友圈

通过 wecom_api 工具调用以下方法。

### 浏览朋友圈
- method: `/sns/getSnsRecord`
- params: `{maxId: 0}` （分页，首次传 0）

### 获取动态详情
- method: `/sns/getSnsDetail`
- params: `{snsIds: ["动态ID1"]}`

### 发布朋友圈（需先上传媒体）
1. 上传媒体: method: `/sns/upload`, params: `{fileUrl: "图片URL"}`
2. 发布: method: `/sns/postSns`, params: `{content: "文字内容", mediaList: [上传返回的媒体信息]}`

### 删除朋友圈
- method: `/sns/deleteSns`
- params: `{snsId: "动态ID"}`

### 点赞
- method: `/sns/snsLike`
- params: `{snsId: "动态ID"}`

### 评论
- method: `/sns/snsComment`
- params: `{snsId: "动态ID", content: "评论内容"}`

### 删除评论
- method: `/sns/deleteSnsComment`
- params: `{snsId: "动态ID", commentId: "评论ID"}`',
  '{}'
);

-- 4.5 文件传输
INSERT OR REPLACE INTO skills (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
VALUES (
  'wecom-cdn',
  '文件传输',
  '上传文件到 CDN、下载企微/个微文件、CDN 链接转换',
  '1.0.0',
  'knowledge',
  1,
  '[]',
  '[]',
  '## 文件传输

通过 wecom_api 工具调用以下方法。

### 通过 URL 上传文件
- method: `/cloud/cdnBigUploadByUrl`
- params: `{fileUrl: "文件URL"}`

### 异步上传（大文件）
- method: `/cloud/cdnUploadByUrlAsync`
- params: `{fileUrl: "文件URL"}`

### 下载企微文件
- method: `/cloud/wxWorkDownload`
- params: `{fileId: "文件ID"}`

### 异步下载企微文件
- method: `/cloud/wxWorkDownloadAsync`
- params: `{fileId: "文件ID"}`

### 下载个微文件
- method: `/cloud/wxDownload`
- params: `{fileId: "文件ID"}`

### CDN 文件转 URL
- method: `/cloud/cdnWxDownload`
- params: `{cdnKey: "CDN密钥"}`',
  '{}'
);

-- 4.6 标签管理
INSERT OR REPLACE INTO skills (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
VALUES (
  'wecom-tag',
  '标签管理',
  '查看标签列表、编辑个人标签、编辑客户标签',
  '1.0.0',
  'knowledge',
  1,
  '[]',
  '[]',
  '## 标签管理

通过 wecom_api 工具调用以下方法。

### 同步标签列表
- method: `/label/syncLabelList`
- params: `{}`

### 编辑个人标签
- method: `/label/editLabel`
- params: `{labelId: "标签ID", labelName: "标签名", memberIds: ["联系人ID"]}`

### 编辑客户标签
- method: `/label/contactEditLabel`
- params: `{userId: "客户ID", labelIds: ["标签ID"]}`',
  '{}'
);

-- 4.7 会话管理
INSERT OR REPLACE INTO skills (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
VALUES (
  'wecom-session',
  '会话管理',
  '查看会话列表、管理会话分组',
  '1.0.0',
  'knowledge',
  1,
  '[]',
  '[]',
  '## 会话管理

通过 wecom_api 工具调用以下方法。

### 获取会话列表（分页）
- method: `/session/getSessionPage`
- params: `{pageNum: 1, pageSize: 20}`

### 获取会话分组
- method: `/session/getSessionList`
- params: `{}`

### 编辑会话分组
- method: `/session/setSessionCmd`
- params: `{sessionId: "会话ID", cmd: "操作类型"}`',
  '{}'
);
