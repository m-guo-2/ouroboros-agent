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
  '日常沟通基础：发消息、查联系人、查群列表、通用 API 调用',
  '1.0.0',
  'action',
  1,
  '[]',
  -- tools: 4 个合并后的核心工具
  '[
    {
      "name": "wecom_send_message",
      "description": "向企微联系人或群聊发送消息。支持文字、图片、文件等类型。这是主动发消息的工具（区别于 send_channel_message 回复当前会话）。",
      "inputSchema": {
        "type": "object",
        "properties": {
          "channelConversationId": {"type": "string", "description": "群聊 ID（群消息时填写）"},
          "channelUserId": {"type": "string", "description": "联系人 ID（私聊时填写）"},
          "messageType": {"type": "string", "description": "消息类型：text（默认）/ image / file"},
          "content": {"type": "string", "description": "消息内容。text 填文字；image 填图片 URL；file 填文件 URL"},
          "channelMeta": {"type": "object", "description": "附加信息，如 file 类型时传 {\"fileName\": \"报告.pdf\"}"}
        },
        "required": ["content"]
      },
      "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/send", "method": "POST"}
    },
    {
      "name": "wecom_search_contact",
      "description": "搜索企微联系人。返回匹配的联系人列表（含 ID、昵称等信息）。",
      "inputSchema": {
        "type": "object",
        "properties": {
          "params": {
            "type": "object",
            "description": "搜索参数",
            "properties": {
              "keyword": {"type": "string", "description": "搜索关键词（姓名、备注等）"}
            },
            "required": ["keyword"]
          }
        },
        "required": ["params"]
      },
      "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/contact/search", "method": "POST"}
    },
    {
      "name": "wecom_list_groups",
      "description": "获取企微群聊列表。返回所有群聊的 ID、名称等信息。",
      "inputSchema": {
        "type": "object",
        "properties": {
          "params": {
            "type": "object",
            "description": "查询参数（可选）"
          }
        }
      },
      "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/group/list", "method": "POST"}
    },
    {
      "name": "wecom_api",
      "description": "企微通用 API 调用。当需要执行 load_skill 加载的技能文档中描述的操作时，通过此工具传入 method 和 params 执行。",
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

### wecom_search_contact
搜索联系人，用于查找某人的 ID 以便后续发消息或管理操作。

### wecom_list_groups
列出所有群聊，获取群 ID 和名称。

### wecom_api
万能 API 透传工具。当你通过 load_skill 了解到某个操作的 method 和 params 后，通过此工具执行。',
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
