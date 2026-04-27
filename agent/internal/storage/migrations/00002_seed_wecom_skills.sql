-- +goose Up
-- 企微 (WeCom) 渐进式 skill 加载的种子数据。原 agent/data/043-wecom-skills.sql
-- 在 SQLite 时代由运维手工执行；现在交给 goose 在 schema 应用之后保证一次性
-- 落库。运行时已经不消费 skills.tools / skills.triggers 字段（它们在
-- 00001_init.sql 中拆到了独立表，但 runtime 没有读路径），因此这里仅写入
-- skills 表的元数据 + agent_configs.system_prompt + agent_skill_bindings。
-- 真正的工具实现位于 agent/internal/runner/wecom_builtin_tools.go。

-- +goose StatementBegin
DELETE FROM skills WHERE id = 'qiwei-agent';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'wecom-core',
    '企微核心能力',
    '面向 agent-human communication 的 4 个企微语义化工具：搜索对象、读取会话、解析消息、发送消息',
    '1.0.0',
    'action',
    1,
    '## 企微核心能力\n\n### wecom_send_message\n向指定联系人或群聊发送消息。常用场景：\n- 私聊：填 channelUserId\n- 群聊：填 channelConversationId\n- messageType 不填默认 text\n\n### wecom_search_targets\n统一搜索联系人和群聊，先定位沟通对象，再进行发送或读历史。\n\n### wecom_list_or_get_conversations\n统一读取会话。不给 conversationId 时看最近会话，给 conversationId 时看该会话历史消息。\n\n### wecom_parse_message\n统一解析企微消息，尤其用于图片、文件、语音等非纯文本内容。\n\n### inspect_attachment\n按 attachmentId 按需分析当前会话里的结构化附件。优先用于图片、文件、视频的进一步理解；如果只是看当前消息正文里的链接，不要自行猜参数，优先使用这个工具。\n\n### wecom_api\n兼容旧扩展技能的透传入口。只有 load_skill 文档明确要求 method + params 时才使用，日常沟通不要优先选它。',
    '{}',
    0, 0, 0
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'wecom-group-mgmt',
    '群管理',
    '创建群聊、管理群成员、设置群公告、转让群主等群组管理操作',
    '1.0.0', 'knowledge', 1,
    '## 群管理\n\n通过 wecom_api 工具调用以下方法。所有 params 中无需传 guid（系统自动注入）。\n\n### 创建群\n- method: `/room/createRoom`\n- params: `{memberIds: ["id1","id2"], roomName: "群名"}`\n\n### 修改群名\n- method: `/room/modifyRoomName`\n- params: `{roomId: "群ID", name: "新群名"}`\n\n### 修改群公告\n- method: `/room/modifyRoomNotice`\n- params: `{roomId: "群ID", notice: "公告内容"}`\n\n### 添加群成员\n- method: `/room/inviteRoomMember`\n- params: `{roomId: "群ID", memberIds: ["id1"]}`\n\n### 移除群成员\n- method: `/room/removeRoomMember`\n- params: `{roomId: "群ID", memberIds: ["id1"]}`\n\n### 设置群管理员\n- method: `/room/roomAddAdmin`\n- params: `{roomId: "群ID", memberIds: ["id1"]}`\n\n### 取消群管理员\n- method: `/room/roomRemoveAdmin`\n- params: `{roomId: "群ID", memberIds: ["id1"]}`\n\n### 转让群主\n- method: `/room/changeRoomMaster`\n- params: `{roomId: "群ID", memberId: "新群主ID"}`\n\n### 解散群\n- method: `/room/dismissRoom`\n- params: `{roomId: "群ID"}`\n\n### 退出群\n- method: `/room/quitRoom`\n- params: `{roomId: "群ID"}`\n\n### 获取群二维码\n- method: `/room/getRoomQrCode`\n- params: `{roomId: "群ID"}`\n\n### 设置群内昵称\n- method: `/room/modifyRoomNickname`\n- params: `{roomId: "群ID", nickname: "昵称"}`',
    '{}', 0, 0, 0
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'wecom-contact-mgmt',
    '联系人管理',
    '添加好友、通过好友申请、修改联系人信息、删除联系人等',
    '1.0.0', 'knowledge', 1,
    '## 联系人管理\n\n通过 wecom_api 工具调用以下方法。\n\n### 获取联系人详情（批量）\n- method: `/contact/batchGetUserinfo`\n- params: `{userIds: ["id1","id2"]}`\n\n### 列出个人微信联系人\n- method: `/contact/getWxContactList`\n- params: `{}`\n\n### 列出企业微信联系人\n- method: `/contact/getWxWorkContactList`\n- params: `{}`\n\n### 添加个人微信好友\n- method: `/contact/addSearchWxContact`\n- params: `{keyword: "手机号或微信号", verifyContent: "验证消息"}`\n\n### 添加企业微信好友\n- method: `/contact/addSearchWxWorkContact`\n- params: `{keyword: "搜索词"}`\n\n### 通过好友申请\n- method: `/contact/agreeContact`\n- params: `{encryptUserName: "加密用户名", ticket: "ticket"}`\n\n### 修改个人联系人备注\n- method: `/contact/updateWxContact`\n- params: `{userId: "联系人ID", remark: "新备注"}`\n\n### 修改企业联系人备注\n- method: `/contact/updateWxWorkContact`\n- params: `{userId: "联系人ID", remark: "新备注"}`\n\n### 删除联系人\n- method: `/contact/deleteContact`\n- params: `{userId: "联系人ID"}`',
    '{}', 0, 0, 0
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'wecom-message-mgmt',
    '消息管理',
    '撤回消息、置顶消息、群发消息、同步历史消息等',
    '1.0.0', 'knowledge', 1,
    '## 消息管理\n\n通过 wecom_api 工具调用以下方法。\n\n### 撤回消息\n- method: `/msg/revokeMsg`\n- params: `{msgSvrId: "消息ID", toId: "接收者ID"}`\n\n### 置顶消息\n- method: `/msg/roomTopMessageSet`\n- params: `{roomId: "群ID", msgSvrId: "消息ID", action: 1}`\n- action: 1=置顶, 0=取消置顶\n\n### 列出置顶消息\n- method: `/msg/roomTopMessageList`\n- params: `{roomId: "群ID"}`\n\n### 群发消息\n- method: `/msg/sendGroupMsg`\n- params: `{toIds: ["id1","id2"], content: "消息内容", msgType: 1}`\n\n### 查询群发状态\n- method: `/msg/sendGroupMsgStatus`\n- params: `{msgId: "群发ID"}`\n\n### 同步历史消息\n- method: `/msg/syncMsg`\n- params: `{toId: "会话ID", msgSvrId: "起始消息ID"}`\n\n### 发送富文本消息\n- method: `/msg/sendHyperText`\n- params: `{toId: "接收者ID", content: "消息XML"}`\n\n### 发送链接消息\n- method: `/msg/sendLink`\n- params: `{toId: "接收者ID", title: "标题", desc: "描述", linkUrl: "URL", imgUrl: "缩略图URL"}`\n\n### 发送位置\n- method: `/msg/sendLocation`\n- params: `{toId: "接收者ID", longitude: "经度", latitude: "纬度", label: "地名"}`\n\n### 发送名片\n- method: `/msg/sendPersonalCard`\n- params: `{toId: "接收者ID", userId: "名片用户ID"}`',
    '{}', 0, 0, 0
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'wecom-moment',
    '朋友圈',
    '浏览朋友圈、发布动态、点赞、评论等朋友圈操作',
    '1.0.0', 'knowledge', 1,
    '## 朋友圈\n\n通过 wecom_api 工具调用以下方法。\n\n### 浏览朋友圈\n- method: `/sns/getSnsRecord`\n- params: `{maxId: 0}` （分页，首次传 0）\n\n### 获取动态详情\n- method: `/sns/getSnsDetail`\n- params: `{snsIds: ["动态ID1"]}`\n\n### 发布朋友圈（需先上传媒体）\n1. 上传媒体: method: `/sns/upload`, params: `{fileUrl: "图片URL"}`\n2. 发布: method: `/sns/postSns`, params: `{content: "文字内容", mediaList: [上传返回的媒体信息]}`\n\n### 删除朋友圈\n- method: `/sns/deleteSns`\n- params: `{snsId: "动态ID"}`\n\n### 点赞\n- method: `/sns/snsLike`\n- params: `{snsId: "动态ID"}`\n\n### 评论\n- method: `/sns/snsComment`\n- params: `{snsId: "动态ID", content: "评论内容"}`\n\n### 删除评论\n- method: `/sns/deleteSnsComment`\n- params: `{snsId: "动态ID", commentId: "评论ID"}`',
    '{}', 0, 0, 0
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'wecom-cdn',
    '文件传输',
    '上传文件到 CDN、下载企微/个微文件、CDN 链接转换',
    '1.0.0', 'knowledge', 1,
    '## 文件传输\n\n通过 wecom_api 工具调用以下方法。\n\n### 通过 URL 上传文件\n- method: `/cloud/cdnBigUploadByUrl`\n- params: `{fileUrl: "文件URL"}`\n\n### 异步上传（大文件）\n- method: `/cloud/cdnUploadByUrlAsync`\n- params: `{fileUrl: "文件URL"}`\n\n### 下载企微文件\n- method: `/cloud/wxWorkDownload`\n- params: `{fileId: "文件ID"}`\n\n### 异步下载企微文件\n- method: `/cloud/wxWorkDownloadAsync`\n- params: `{fileId: "文件ID"}`\n\n### 下载个微文件\n- method: `/cloud/wxDownload`\n- params: `{fileId: "文件ID"}`\n\n### CDN 文件转 URL\n- method: `/cloud/cdnWxDownload`\n- params: `{cdnKey: "CDN密钥"}`',
    '{}', 0, 0, 0
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'wecom-tag',
    '标签管理',
    '查看标签列表、编辑个人标签、编辑客户标签',
    '1.0.0', 'knowledge', 1,
    '## 标签管理\n\n通过 wecom_api 工具调用以下方法。\n\n### 同步标签列表\n- method: `/label/syncLabelList`\n- params: `{}`\n\n### 编辑个人标签\n- method: `/label/editLabel`\n- params: `{labelId: "标签ID", labelName: "标签名", memberIds: ["联系人ID"]}`\n\n### 编辑客户标签\n- method: `/label/contactEditLabel`\n- params: `{userId: "客户ID", labelIds: ["标签ID"]}`',
    '{}', 0, 0, 0
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'wecom-session',
    '会话管理',
    '查看会话列表、管理会话分组',
    '1.0.0', 'knowledge', 1,
    '## 会话管理\n\n通过 wecom_api 工具调用以下方法。\n\n### 获取会话列表（分页）\n- method: `/session/getSessionPage`\n- params: `{pageNum: 1, pageSize: 20}`\n\n### 获取会话分组\n- method: `/session/getSessionList`\n- params: `{}`\n\n### 编辑会话分组\n- method: `/session/setSessionCmd`\n- params: `{sessionId: "会话ID", cmd: "操作类型"}`',
    '{}', 0, 0, 0
);
-- +goose StatementEnd

-- 把 default-agent-config 的 system_prompt 重写为 moli 提示词，并补一条
-- wecom-core 绑定。两条语句对不存在的 default-agent-config 都是 no-op，
-- 所以本 migration 在新初始化的环境也能安全执行。

-- +goose StatementBegin
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

{{skills}}'
WHERE id = 'default-agent-config' AND deleted_at = 0;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO agent_skill_bindings
    (id, agent_id, skill_id, mode, position, created_at, deleted_at)
SELECT
    CONCAT('asb-default-wecom-core'),
    'default-agent-config',
    'wecom-core',
    'always',
    0,
    0,
    0
FROM dual
WHERE EXISTS (
    SELECT 1 FROM agent_configs
    WHERE id = 'default-agent-config' AND deleted_at = 0
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM agent_skill_bindings WHERE id = 'asb-default-wecom-core';
DELETE FROM skills WHERE id IN (
    'wecom-core', 'wecom-group-mgmt', 'wecom-contact-mgmt',
    'wecom-message-mgmt', 'wecom-moment', 'wecom-cdn',
    'wecom-tag', 'wecom-session'
);
-- +goose StatementEnd
