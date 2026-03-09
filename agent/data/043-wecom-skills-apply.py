#!/usr/bin/env python3
"""043-wecom-skills-apply.py — 企微 Skill 数据写入（v2: 结构化 tool definitions）"""
import sqlite3, json, os

DB_PATH = os.path.join(os.path.dirname(__file__), "../../data/config.db")

def T(name, desc, schema, method, required=None):
    """Shorthand for building a tool definition."""
    s = {"type": "object", "properties": schema}
    if required:
        s["required"] = required
    return {
        "name": name,
        "description": desc,
        "inputSchema": s,
        "executor": {"type": "http", "apiMethod": method},
    }

def P(typ, desc, **kw):
    """Shorthand for a property definition."""
    d = {"type": typ, "description": desc}
    d.update(kw)
    return d

# ============================================================
# wecom-core — 4 核心工具（增强 description）
# ============================================================
WECOM_CORE_TOOLS = [
    {
        "name": "wecom_send_message",
        "description": "向指定企微联系人或群聊主动发送消息。支持文字(text)、图片(image)、文件(file)三种消息类型。私聊时填 channelUserId，群聊时填 channelConversationId；如果只需回复当前会话用户，请优先使用 send_channel_message 而非此工具。messageType 默认为 text。发送图片时 content 填图片 URL，发送文件时 content 填文件 URL 并在 channelMeta 中指定 fileName。",
        "inputSchema": {
            "type": "object",
            "properties": {
                "channelConversationId": P("string", "群聊 ID（群消息时填写）"),
                "channelUserId": P("string", "联系人 ID（私聊时填写）"),
                "messageType": P("string", "消息类型：text（默认）/ image / file"),
                "content": P("string", "消息内容。text 填文字；image 填图片 URL；file 填文件 URL"),
                "channelMeta": P("object", "附加信息，如 file 类型时传 {\"fileName\": \"报告.pdf\"}"),
            },
            "required": ["content"],
        },
        "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/send", "method": "POST"},
    },
    {
        "name": "wecom_search_contact",
        "description": "搜索企微联系人。输入关键词（姓名、备注、手机号等），返回匹配的联系人列表，每个结果包含 userId、昵称、备注等信息。这是定位联系人 ID 的主要方式——后续发消息、拉群、修改备注等操作都需要先通过此工具获取目标用户的 userId。搜索范围覆盖个人微信好友和企业微信联系人。",
        "inputSchema": {
            "type": "object",
            "properties": {
                "params": {
                    "type": "object",
                    "description": "搜索参数",
                    "properties": {
                        "keyword": P("string", "搜索关键词，可以是姓名、备注、手机号或微信号"),
                    },
                    "required": ["keyword"],
                },
            },
            "required": ["params"],
        },
        "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/contact/search", "method": "POST"},
    },
    {
        "name": "wecom_list_groups",
        "description": "获取企微所有群聊列表。返回当前账号加入的全部群聊信息，包括群 ID（roomId）、群名称等。这是群管理操作的起点——后续发群消息、管理群成员、修改群设置等都需要先获取 roomId。如需群的详细信息（成员列表、群主、公告等），加载 wecom-group-mgmt 技能使用 batch_get_room_detail。",
        "inputSchema": {
            "type": "object",
            "properties": {
                "params": P("object", "查询参数（可选）"),
            },
        },
        "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/group/list", "method": "POST"},
    },
    {
        "name": "wecom_api",
        "description": "企微通用 API 透传工具。当通过 load_skill 加载扩展技能后，按照技能文档中的 tool 定义，传入对应的 method（API 方法路径）和 params（参数对象）执行操作。method 格式如 /room/createRoom、/msg/revokeMsg 等。params 中无需传 guid，系统自动注入。当没有专用工具可用时，此工具是执行所有企微操作的通道。",
        "inputSchema": {
            "type": "object",
            "properties": {
                "method": P("string", "API 方法路径，如 /msg/revokeMsg、/room/createRoom"),
                "params": P("object", "方法参数（不含 guid，系统自动注入）"),
            },
            "required": ["method"],
        },
        "executor": {"type": "http", "url": "http://localhost:2000/api/qiwei/do", "method": "POST"},
    },
]

WECOM_CORE_README = """## 企微核心工具

### wecom_send_message
主动向联系人或群发送消息。与 send_channel_message（回复当前会话）的区别：
- 回复当前对话用户 → send_channel_message
- 主动找人/找群发消息 → wecom_send_message

### wecom_search_contact
搜索联系人获取 userId。几乎所有操作的第一步都是「找到这个人的 ID」。

### wecom_list_groups
获取群列表获取 roomId。群操作的第一步。

### wecom_api
万能通道。加载扩展技能后，按技能中的 tool 定义传入 method 和 params 执行。"""

# ============================================================
# wecom-group-mgmt — 19 工具
# ============================================================
WECOM_GROUP_TOOLS = [
    T("list_rooms",
      "获取当前账号的所有企微群聊列表。当需要查看自己加入了哪些群、查找某个群的 roomId 时使用。返回群聊数组，每个元素包含 roomId、群名称等基本信息。如果群数量较多，建议先获取列表再通过 batch_get_room_detail 查看具体群信息。通过 wecom_api 调用，method: /room/getRoomList。",
      {}, "/room/getRoomList"),
    T("batch_get_room_detail",
      "批量获取群聊的详细信息，包括群名称、群主、成员列表、群公告等。一次可查询多个群，避免逐个请求。返回每个群的详细数据对象，包含 roomId、roomName、memberList、owner 等字段。通过 wecom_api 调用，method: /room/batchGetRoomDetail。",
      {"roomIds": P("array", "要查询的群聊 ID 列表。可通过 list_rooms 获取 roomId", items={"type": "string"})},
      "/room/batchGetRoomDetail", ["roomIds"]),
    T("create_room",
      "创建企微群聊。至少需要 2 个成员才能建群。成功后返回新群的 roomId，可用于后续群管理操作（如改名、设公告、加人等）。群名称可选，不填则由企微根据成员昵称自动生成。通过 wecom_api 调用，method: /room/createRoom。",
      {"memberIds": P("array", "群初始成员的用户 ID 列表，至少 2 人。可通过 wecom_search_contact 获取用户 ID", items={"type": "string"}),
       "roomName": P("string", "群名称，可选。不填则由企微自动生成")},
      "/room/createRoom", ["memberIds"]),
    T("rename_room",
      "修改群聊名称（所有人可见）。只有群主或管理员有权修改。注意与 set_room_remark_name 区分：rename_room 修改所有人可见的群名称，set_room_remark_name 仅修改自己可见的群备注。通过 wecom_api 调用，method: /room/modifyRoomName。",
      {"roomId": P("string", "目标群聊 ID"), "name": P("string", "新的群名称")},
      "/room/modifyRoomName", ["roomId", "name"]),
    T("set_room_remark_name",
      "修改群聊的备注名称（仅自己可见）。适用于群名不直观或有多个相似群需要区分的场景。不影响其他群成员看到的群名。通过 wecom_api 调用，method: /room/modifyRoomRemarkName。",
      {"roomId": P("string", "目标群聊 ID"), "remarkName": P("string", "新的群备注名称，仅自己可见")},
      "/room/modifyRoomRemarkName", ["roomId", "remarkName"]),
    T("set_room_nickname",
      "修改自己在群内的显示昵称。例如在工作群显示职位、在项目群标注角色。此昵称仅在该群内生效，不影响其他群或私聊的显示名。通过 wecom_api 调用，method: /room/modifyRoomNickname。",
      {"roomId": P("string", "目标群聊 ID"), "nickname": P("string", "新的群内昵称")},
      "/room/modifyRoomNickname", ["roomId", "nickname"]),
    T("invite_room_member",
      "邀请新成员加入群聊。可一次邀请多人。如果群开启了邀请确认，被邀请人需要同意后才能入群。需要操作者具备邀请权限（群主、管理员，或群设置允许普通成员邀请）。通过 wecom_api 调用，method: /room/inviteRoomMember。",
      {"roomId": P("string", "目标群聊 ID"),
       "memberIds": P("array", "要邀请的用户 ID 列表", items={"type": "string"})},
      "/room/inviteRoomMember", ["roomId", "memberIds"]),
    T("remove_room_member",
      "从群聊中移除成员（踢人）。可一次移除多人。仅群主和管理员有权执行。被移除的成员将立即离群且不再接收群消息。此操作不可撤销，建议执行前先确认。通过 wecom_api 调用，method: /room/removeRoomMember。",
      {"roomId": P("string", "目标群聊 ID"),
       "memberIds": P("array", "要移除的成员用户 ID 列表", items={"type": "string"})},
      "/room/removeRoomMember", ["roomId", "memberIds"]),
    T("get_room_qrcode",
      "获取群聊的二维码图片。当需要分享群二维码让他人扫码入群时使用。二维码有时效限制，过期后需重新获取。200 人以上的群可能无法通过二维码入群。通过 wecom_api 调用，method: /room/getRoomQrCode。",
      {"roomId": P("string", "目标群聊 ID")},
      "/room/getRoomQrCode", ["roomId"]),
    T("set_room_notice",
      "设置或修改群公告。新公告发布后会通知所有群成员。仅群主和管理员有权修改。传入空字符串可清除当前公告。通过 wecom_api 调用，method: /room/modifyRoomNotice。",
      {"roomId": P("string", "目标群聊 ID"), "notice": P("string", "群公告内容，支持换行。传空字符串可清除公告")},
      "/room/modifyRoomNotice", ["roomId", "notice"]),
    T("add_room_admin",
      "设置群管理员。将指定成员提升为群管理员，使其拥有踢人、改群名、发公告等管理权限。仅群主有权操作。可一次设置多人。通过 wecom_api 调用，method: /room/roomAddAdmin。",
      {"roomId": P("string", "目标群聊 ID"),
       "memberIds": P("array", "要设为管理员的成员用户 ID 列表", items={"type": "string"})},
      "/room/roomAddAdmin", ["roomId", "memberIds"]),
    T("remove_room_admin",
      "取消群管理员身份。将管理员降级为普通成员。仅群主有权操作。被取消管理员的成员仍保留在群中。通过 wecom_api 调用，method: /room/roomRemoveAdmin。",
      {"roomId": P("string", "目标群聊 ID"),
       "memberIds": P("array", "要取消管理员的成员用户 ID 列表", items={"type": "string"})},
      "/room/roomRemoveAdmin", ["roomId", "memberIds"]),
    T("quit_room",
      "主动退出群聊。此操作不可撤销，退出后需被邀请才能重新加入。如果退出者是群主，建议先通过 transfer_room_owner 转让群主身份。通过 wecom_api 调用，method: /room/quitRoom。",
      {"roomId": P("string", "要退出的群聊 ID")},
      "/room/quitRoom", ["roomId"]),
    T("transfer_room_owner",
      "转让群主身份给其他群成员。仅当前群主有权执行。转让后原群主变为普通成员。此操作不可撤销，建议执行前向用户确认目标人选。通过 wecom_api 调用，method: /room/changeRoomMaster。",
      {"roomId": P("string", "目标群聊 ID"), "memberId": P("string", "新群主的用户 ID（单个 ID，不是数组）")},
      "/room/changeRoomMaster", ["roomId", "memberId"]),
    T("dismiss_room",
      "解散群聊。群聊将被永久删除，所有成员自动退出，历史消息不可恢复。仅群主有权操作。这是最高危操作，执行前必须向用户明确确认后果。通过 wecom_api 调用，method: /room/dismissRoom。",
      {"roomId": P("string", "要解散的群聊 ID")},
      "/room/dismissRoom", ["roomId"]),
    T("get_room_openid",
      "获取群聊的 OpenID（跨平台唯一标识）。当需要在跨系统场景中引用群聊时使用。通过 wecom_api 调用，method: /room/openid。",
      {"roomId": P("string", "目标群聊 ID")},
      "/room/openid", ["roomId"]),
    T("enable_room_rename",
      "控制是否允许普通群成员修改群名称。开启后所有成员可改群名，关闭后仅群主和管理员可改。仅群主和管理员有权修改此设置。通过 wecom_api 调用，method: /room/enableChangeRoomName。",
      {"roomId": P("string", "目标群聊 ID"), "enable": P("integer", "1=允许群成员修改群名，0=仅群主/管理员可修改")},
      "/room/enableChangeRoomName", ["roomId", "enable"]),
    T("toggle_invite_confirm",
      "开启或关闭群聊的邀请确认机制。开启后新成员被邀请时需确认同意才能入群；关闭后邀请即入群。仅群主和管理员有权修改。通过 wecom_api 调用，method: /room/openInviteConfirm。",
      {"roomId": P("string", "目标群聊 ID"), "enable": P("integer", "1=开启邀请确认，0=关闭（邀请直接入群）")},
      "/room/openInviteConfirm", ["roomId", "enable"]),
    T("accept_invite_by_link",
      "通过邀请链接同意加入群聊。适用于自动化场景，如机器人自动加入指定群聊。链接失效或群已满员时操作会失败。通过 wecom_api 调用，method: /room/agreeInviteByLink。",
      {"url": P("string", "群邀请链接 URL，从邀请消息中获取")},
      "/room/agreeInviteByLink", ["url"]),
]

WECOM_GROUP_README = """## 企微群管理

通过 `wecom_api` 工具调用群管理相关操作。所有参数中无需传 `guid`（系统自动注入）。

### 常见工作流

**创建并配置群聊：**
1. `wecom_search_contact` 搜索要拉入的成员，获取用户 ID
2. `create_room` 建群，拿到 roomId
3. `set_room_notice` 设置群公告
4. `rename_room` 修改群名称（如建群时未指定）

**查询群信息：**
1. `list_rooms` 获取所有群列表
2. 在列表中找到目标群的 roomId
3. `batch_get_room_detail` 查看群详细信息（成员、群主、公告等）

**群成员管理：**
1. `batch_get_room_detail` 确认当前群成员列表
2. `invite_room_member` 添加新成员 / `remove_room_member` 移除成员
3. `add_room_admin` 设置管理员 / `remove_room_admin` 取消管理员

### 注意事项

- **危险操作需二次确认：** `dismiss_room`（解散群）、`remove_room_member`（踢人）、`transfer_room_owner`（转让群主）、`quit_room`（退群）均不可撤销
- **权限要求：** 大部分管理操作需要群主或管理员身份
- **群名 vs 备注：** `rename_room` 改的是全员可见的群名，`set_room_remark_name` 改的是仅自己可见的备注名"""

# ============================================================
# wecom-contact-mgmt — 13 工具
# ============================================================
WECOM_CONTACT_TOOLS = [
    T("batch_get_contact_info",
      "批量获取联系人详细信息，包括昵称、头像、备注、性别、地区等。传入用户 ID 列表，返回每个用户的详细资料。适合在执行操作前做信息核实。通过 wecom_api 调用，method: /contact/batchGetUserinfo。",
      {"userIds": P("array", "要查询的用户 ID 列表，建议单次不超过 100 个", items={"type": "string"})},
      "/contact/batchGetUserinfo", ["userIds"]),
    T("list_personal_contacts",
      "获取个人微信联系人完整列表。返回所有已添加的个人微信好友信息（userId、昵称、备注、头像等）。注意：此接口返回的是个人微信好友，不包括企业微信通讯录中的同事。通过 wecom_api 调用，method: /contact/getWxContactList。",
      {}, "/contact/getWxContactList"),
    T("list_enterprise_contacts",
      "获取企业微信通讯录中的联系人列表。返回所有企业内部同事和已添加的企业微信外部联系人。注意：不包括个人微信好友。通过 wecom_api 调用，method: /contact/getWxWorkContactList。",
      {}, "/contact/getWxWorkContactList"),
    T("search_contact",
      "通过关键词搜索联系人，支持按姓名、备注、手机号、微信号等匹配。返回匹配的联系人列表。这是最常用的联系人查找方式，通常作为后续操作（发消息、修改备注等）的第一步。通过 wecom_api 调用，method: /contact/searchContact。",
      {"keyword": P("string", "搜索关键词，可以是姓名、备注、手机号或微信号")},
      "/contact/searchContact", ["keyword"]),
    T("add_personal_wechat_friend",
      "通过搜索添加个人微信好友。输入手机号或微信号发送好友申请，可附带验证消息。添加后需等待对方通过验证。频繁添加可能触发风控限制。通过 wecom_api 调用，method: /contact/addSearchWxContact。",
      {"keyword": P("string", "要搜索的手机号或微信号"),
       "verifyContent": P("string", "好友验证消息，留空则使用默认验证语")},
      "/contact/addSearchWxContact", ["keyword"]),
    T("add_enterprise_wechat_friend",
      "通过搜索添加企业微信联系人。输入对方手机号、企业微信号或姓名进行搜索并发送添加请求。企业微信添加通常无需验证消息。通过 wecom_api 调用，method: /contact/addSearchWxWorkContact。",
      {"keyword": P("string", "搜索关键词，可以是手机号、企业微信号或姓名")},
      "/contact/addSearchWxWorkContact", ["keyword"]),
    T("add_contact_by_card",
      "通过名片添加联系人。当收到他人分享的联系人名片时，使用此接口发起好友添加。名片中的 v3（encryptUserName）是必须的标识字段。通过 wecom_api 调用，method: /contact/addCardContact。",
      {"v3": P("string", "名片中的加密用户名 encryptUserName，从名片消息的 XML 数据中提取"),
       "v4": P("string", "名片中的加密票据 encryptTicket（可选）"),
       "verifyContent": P("string", "好友验证消息（可选）")},
      "/contact/addCardContact", ["v3"]),
    T("re_add_deleted_contact",
      "重新添加已删除的联系人，无需对方再次确认即可恢复好友关系。仅对之前主动删除的联系人有效。恢复后对话记录不会恢复。通过 wecom_api 调用，method: /contact/addDeletedContact。",
      {"userId": P("string", "之前删除的联系人 ID")},
      "/contact/addDeletedContact", ["userId"]),
    T("approve_friend_request",
      "通过好友申请。需要提供请求中的 encryptUserName 和 ticket（通常从好友申请的回调通知中获取）。通过后对方立即出现在好友列表中。建议先确认用户确实希望通过该申请。通过 wecom_api 调用，method: /contact/agreeContact。",
      {"encryptUserName": P("string", "好友申请中的加密用户名，从申请通知回调数据中获取"),
       "ticket": P("string", "好友申请中的验证票据，从申请通知回调数据中获取")},
      "/contact/agreeContact", ["encryptUserName", "ticket"]),
    T("update_personal_contact",
      "修改个人微信联系人的备注名。修改仅影响本地显示，对方不会收到通知。仅对个人微信好友有效，企业微信联系人请使用 update_enterprise_contact。通过 wecom_api 调用，method: /contact/updateWxContact。",
      {"userId": P("string", "要修改的联系人 ID"), "remark": P("string", "新的备注名")},
      "/contact/updateWxContact", ["userId", "remark"]),
    T("update_enterprise_contact",
      "修改企业微信联系人的备注名。修改仅影响本地展示，对方不会收到通知。仅对企业微信联系人有效，个人微信好友请使用 update_personal_contact。通过 wecom_api 调用，method: /contact/updateWxWorkContact。",
      {"userId": P("string", "要修改的企业联系人 ID"), "remark": P("string", "新的备注名")},
      "/contact/updateWxWorkContact", ["userId", "remark"]),
    T("delete_contact",
      "删除联系人。删除后无法查看对方朋友圈，对话记录保留但无法继续发送消息。可通过 re_add_deleted_contact 重新添加但对话记录不恢复。执行前务必与用户确认。通过 wecom_api 调用，method: /contact/deleteContact。",
      {"userId": P("string", "要删除的联系人 ID")},
      "/contact/deleteContact", ["userId"]),
    T("get_contact_openid",
      "获取联系人的 OpenID（跨平台唯一标识）。当需要将企微联系人与其他微信生态系统（公众号、小程序等）进行用户匹配时使用。通过 wecom_api 调用，method: /contact/openid。",
      {"userId": P("string", "要查询 OpenID 的联系人 ID")},
      "/contact/openid", ["userId"]),
]

WECOM_CONTACT_README = """## 联系人管理

管理企微中的个人微信好友和企业微信联系人，覆盖查询、添加、修改、删除全生命周期。

### 常见工作流

**查找并确认联系人：** `search_contact` 按关键词搜索 → 拿到 userId 后用 `batch_get_contact_info` 获取完整资料 → 确认身份后执行后续操作。

**添加好友：** 个人微信 → `add_personal_wechat_friend`（需等待验证）；企业微信 → `add_enterprise_wechat_friend`；从名片 → `add_contact_by_card`。

**通过好友申请：** 收到回调 → 提取 encryptUserName 和 ticket → `approve_friend_request`。

### 注意事项

- **个人 vs 企业：** 查询和修改需使用对应的接口（personal/enterprise）
- **敏感操作：** `delete_contact` 执行前务必与用户确认
- **频率限制：** 连续添加好友可能触发微信风控"""

# ============================================================
# wecom-message-mgmt — 21 工具
# ============================================================
WECOM_MESSAGE_TOOLS = [
    T("send_text", "发送纯文本消息。日常回复请优先使用 send_channel_message，主动发消息用 wecom_send_message。此工具作为底层 API 参考保留。通过 wecom_api 调用，method: /msg/sendText。",
      {"toId": P("string", "接收方 ID。私聊传联系人 ID，群聊传群 ID"), "content": P("string", "文本内容")},
      "/msg/sendText", ["toId", "content"]),
    T("send_hyper_text", "发送富文本消息（HyperText）。适用于包含格式化内容（加粗、链接、@提及等）的场景，content 使用微信富文本 XML 格式。常见用途：在群里 @某人、发送带内嵌链接的公告。通过 wecom_api 调用，method: /msg/sendHyperText。",
      {"toId": P("string", "接收方 ID"), "content": P("string", "富文本消息内容，微信 XML 格式")},
      "/msg/sendHyperText", ["toId", "content"]),
    T("send_image", "发送图片消息。需提供可公开访问的图片 URL。wecom_send_message 已支持 messageType=image，此为底层 API 参考。通过 wecom_api 调用，method: /msg/sendImage。",
      {"toId": P("string", "接收方 ID"), "imgUrl": P("string", "图片的公开可访问 URL")},
      "/msg/sendImage", ["toId", "imgUrl"]),
    T("send_gif", "发送 GIF 动图消息。与 send_image 的区别在于接收端会以动图形式展示。通过 wecom_api 调用，method: /msg/sendGif。",
      {"toId": P("string", "接收方 ID"), "imgUrl": P("string", "GIF 动图的公开可访问 URL")},
      "/msg/sendGif", ["toId", "imgUrl"]),
    T("send_video", "发送视频消息。需提供视频文件 URL，建议不超过 20MB。通过 wecom_api 调用，method: /msg/sendVideo。",
      {"toId": P("string", "接收方 ID"), "videoUrl": P("string", "视频文件的公开可访问 URL")},
      "/msg/sendVideo", ["toId", "videoUrl"]),
    T("send_file", "发送文件消息。支持各类文件格式（PDF、Word、Excel 等）。wecom_send_message 已支持 messageType=file，此为底层 API 参考。通过 wecom_api 调用，method: /msg/sendFile。",
      {"toId": P("string", "接收方 ID"), "fileUrl": P("string", "文件的公开可访问 URL"), "fileName": P("string", "文件显示名称含扩展名")},
      "/msg/sendFile", ["toId", "fileUrl", "fileName"]),
    T("send_voice", "发送语音消息。需提供语音文件 URL。通过 wecom_api 调用，method: /msg/sendVoice。",
      {"toId": P("string", "接收方 ID"), "voiceUrl": P("string", "语音文件的公开可访问 URL")},
      "/msg/sendVoice", ["toId", "voiceUrl"]),
    T("send_link", "发送链接卡片消息，包含标题、描述、缩略图和跳转链接。比纯文本 URL 更美观。适用于分享文章、网页、活动链接等场景。通过 wecom_api 调用，method: /msg/sendLink。",
      {"toId": P("string", "接收方 ID"), "title": P("string", "链接卡片标题"), "desc": P("string", "描述文字（可选）"), "linkUrl": P("string", "点击后跳转的目标 URL"), "imgUrl": P("string", "卡片缩略图 URL（可选）")},
      "/msg/sendLink", ["toId", "title", "linkUrl"]),
    T("send_mini_program", "发送微信小程序卡片。接收方点击可直接打开小程序页面。需要知道目标小程序的 appId 和页面路径。通过 wecom_api 调用，method: /msg/sendWeapp。",
      {"toId": P("string", "接收方 ID"), "appId": P("string", "小程序 AppID"), "pagePath": P("string", "小程序页面路径"), "title": P("string", "卡片显示标题"), "imgUrl": P("string", "封面图 URL（可选）")},
      "/msg/sendWeapp", ["toId", "appId", "pagePath", "title"]),
    T("send_personal_card", "发送个人名片消息，将某个联系人的名片分享给他人。接收方可通过名片添加好友。通过 wecom_api 调用，method: /msg/sendPersonalCard。",
      {"toId": P("string", "接收方 ID"), "userId": P("string", "要分享名片的联系人 ID")},
      "/msg/sendPersonalCard", ["toId", "userId"]),
    T("send_channel_video", "发送视频号视频。将微信视频号中的视频分享出去。需要知道视频的 feedId。通过 wecom_api 调用，method: /msg/sendFeedVideo。",
      {"toId": P("string", "接收方 ID"), "feedId": P("string", "视频号视频的唯一标识 ID")},
      "/msg/sendFeedVideo", ["toId", "feedId"]),
    T("send_location", "发送地理位置消息。在聊天中显示地图标注点，接收方可点击导航。适用于分享会议地点、门店位置等。通过 wecom_api 调用，method: /msg/sendLocation。",
      {"toId": P("string", "接收方 ID"), "longitude": P("string", "经度"), "latitude": P("string", "纬度"), "label": P("string", "地点名称")},
      "/msg/sendLocation", ["toId", "longitude", "latitude", "label"]),
    T("revoke_message", "撤回一条已发送的消息。只能撤回自己发送的消息，且有时间限制（通常 2 分钟内）。撤回后对方显示「对方撤回了一条消息」。通过 wecom_api 调用，method: /msg/revokeMsg。",
      {"msgSvrId": P("string", "要撤回的消息的服务端 ID"), "toId": P("string", "消息接收方 ID")},
      "/msg/revokeMsg", ["msgSvrId", "toId"]),
    T("update_message_status", "修改消息的状态标记（已读/未读等）。通过 wecom_api 调用，method: /msg/statusModify。",
      {"toId": P("string", "会话 ID"), "status": P("integer", "目标状态值")},
      "/msg/statusModify", ["toId", "status"]),
    T("list_top_messages", "获取群聊中当前所有被置顶的消息列表。仅对群聊有效。通过 wecom_api 调用，method: /msg/roomTopMessageList。",
      {"roomId": P("string", "群聊 ID")},
      "/msg/roomTopMessageList", ["roomId"]),
    T("set_top_message", "置顶或取消置顶群消息。通过 action 参数区分：1=置顶，0=取消置顶。需要群管理员或群主权限。通过 wecom_api 调用，method: /msg/roomTopMessageSet。",
      {"roomId": P("string", "群聊 ID"), "msgSvrId": P("string", "目标消息的服务端 ID"), "action": P("integer", "1=置顶，0=取消置顶")},
      "/msg/roomTopMessageSet", ["roomId", "msgSvrId", "action"]),
    T("mass_send", "群发消息到多个联系人或群聊。有频率限制，避免过于频繁。返回群发任务 ID，可通过 mass_send_status 查询进度。通过 wecom_api 调用，method: /msg/sendGroupMsg。",
      {"toIds": P("array", "接收方 ID 列表", items={"type": "string"}), "content": P("string", "群发内容"), "msgType": P("integer", "消息类型：1=文本, 2=图片, 3=链接")},
      "/msg/sendGroupMsg", ["toIds", "content", "msgType"]),
    T("mass_send_status", "查询群发消息的发送状态和进度。返回各接收方的送达情况。通过 wecom_api 调用，method: /msg/sendGroupMsgStatus。",
      {"msgId": P("string", "群发任务 ID，从 mass_send 返回结果获取")},
      "/msg/sendGroupMsgStatus", ["msgId"]),
    T("mass_send_rule", "管理群发消息规则（定时群发等）。通过 wecom_api 调用，method: /msg/sendGroupMsgRule。",
      {"ruleId": P("string", "规则 ID（查询/删除时传入）"), "action": P("string", "操作类型")},
      "/msg/sendGroupMsgRule"),
    T("sync_history", "同步指定会话的历史消息记录。通过 msgSvrId 定位起始消息向前拉取。不传 msgSvrId 则从最新消息开始。返回消息数组（含发送者、内容、时间等）。通过 wecom_api 调用，method: /msg/syncMsg。",
      {"toId": P("string", "会话 ID，私聊传联系人 ID，群聊传群 ID"), "msgSvrId": P("string", "起始消息服务端 ID（可选），不传则从最新消息开始")},
      "/msg/syncMsg", ["toId"]),
]

WECOM_MESSAGE_README = """## 消息管理

企微高级消息操作，覆盖多媒体消息发送、消息撤回、群消息置顶、批量群发、历史消息同步等能力。

> 基础的文字/图片/文件发送已由 `wecom_send_message` 覆盖，本技能聚焦于更丰富的消息类型和消息生命周期管理。

### 消息类型总览

| 类型 | 工具 | 说明 |
|------|------|------|
| 富文本 | send_hyper_text | 格式化内容、@提及 |
| 动图 | send_gif | GIF 表情/动图 |
| 视频 | send_video | 视频文件 |
| 语音 | send_voice | 语音消息 |
| 链接卡片 | send_link | 标题+描述+缩略图+URL |
| 小程序 | send_mini_program | 微信小程序卡片 |
| 名片 | send_personal_card | 分享联系人名片 |
| 视频号 | send_channel_video | 视频号内容分享 |
| 位置 | send_location | 地理位置消息 |

### 常见工作流

**群发消息并追踪：** `mass_send` 发起群发 → 拿到 msgId → `mass_send_status` 查询发送进度。

**置顶/取消置顶：** `set_top_message` action=1 置顶 / action=0 取消 → `list_top_messages` 查看当前置顶。

**查看历史消息：** `sync_history` 拉取最新消息 → 用返回中最早消息的 msgSvrId 继续翻页。

### 注意事项

- **撤回时限：** 通常限 2 分钟内，超时失败。只能撤回自己的消息
- **群发频控：** 短时间内大量群发可能触发企微风控，建议分批
- **置顶权限：** 需要群管理员或群主
- **媒体 URL：** 必须公开可访问。内网文件需先通过 wecom-cdn 技能上传
- **toId 含义：** 私聊=联系人 ID，群聊=群 ID"""

# ============================================================
# wecom-moment — 8 工具
# ============================================================
WECOM_MOMENT_TOOLS = [
    T("browse_moments", "浏览朋友圈动态列表。支持分页：首次传 maxId=0，后续传上一页最后一条的 snsId 继续翻页。返回动态摘要信息，包含发布者、内容预览、点赞评论数等。通过 wecom_api 调用，method: /sns/getSnsRecord。",
      {"maxId": P("integer", "分页游标。首次传 0，翻页时传上一页最后一条动态的 snsId")},
      "/sns/getSnsRecord", ["maxId"]),
    T("get_moment_details", "批量获取朋友圈动态的完整详情，包括完整内容、高清媒体列表、所有点赞和评论明细。单次最多传 20 个 snsId。通过 wecom_api 调用，method: /sns/getSnsDetail。",
      {"snsIds": P("array", "要查询的动态 ID 列表，单次最多 20 个", items={"type": "string"})},
      "/sns/getSnsDetail", ["snsIds"]),
    T("upload_moment_media", "上传图片或视频到朋友圈媒体库。这是发布带媒体朋友圈的前置步骤——必须先上传拿到媒体信息，再传入 publish_moment 发布。返回的媒体对象需原样保存。通过 wecom_api 调用，method: /sns/upload。",
      {"fileUrl": P("string", "媒体文件的可访问 URL，支持 jpg、png、mp4 等")},
      "/sns/upload", ["fileUrl"]),
    T("publish_moment", "发布一条朋友圈动态。纯文字只传 content；带图片/视频需同时传 content 和 mediaList（先调 upload_moment_media 上传）。返回新动态的 snsId。通过 wecom_api 调用，method: /sns/postSns。",
      {"content": P("string", "朋友圈文字内容"), "mediaList": P("array", "媒体列表，每个元素是 upload_moment_media 返回的媒体对象", items={"type": "object"})},
      "/sns/postSns", ["content"]),
    T("delete_moment", "删除一条自己发布的朋友圈动态。删除后不可恢复，所有点赞和评论也会一并删除。操作前建议先确认。通过 wecom_api 调用，method: /sns/deleteSns。",
      {"snsId": P("string", "要删除的动态 ID")},
      "/sns/deleteSns", ["snsId"]),
    T("like_moment", "为一条朋友圈动态点赞。重复点赞可能会取消赞。通过 wecom_api 调用，method: /sns/snsLike。",
      {"snsId": P("string", "要点赞的动态 ID")},
      "/sns/snsLike", ["snsId"]),
    T("comment_moment", "对朋友圈动态发表评论（纯文本）。返回 commentId，可用于后续删除。通过 wecom_api 调用，method: /sns/snsComment。",
      {"snsId": P("string", "要评论的动态 ID"), "content": P("string", "评论内容")},
      "/sns/snsComment", ["snsId", "content"]),
    T("delete_moment_comment", "删除朋友圈上自己发表的评论。不可恢复。commentId 可从 get_moment_details 获取。通过 wecom_api 调用，method: /sns/deleteSnsComment。",
      {"snsId": P("string", "动态 ID"), "commentId": P("string", "要删除的评论 ID")},
      "/sns/deleteSnsComment", ["snsId", "commentId"]),
]

WECOM_MOMENT_README = """## 朋友圈

### 常见工作流

**浏览：** `browse_moments` (maxId=0) → 翻页用上一页最后一条的 snsId → `get_moment_details` 查看详情。

**发布带图：** 逐一 `upload_moment_media` 上传图片 → 收集媒体对象 → `publish_moment` 发布。

**互动：** `like_moment` 点赞 / `comment_moment` 评论 → `delete_moment_comment` 删除评论。

### 注意事项

- 发布带媒体的朋友圈必须先上传后发布（两步），不能跳过上传
- 分页使用 maxId 机制，首次必须传 0
- 删除动态/评论不可恢复"""

# ============================================================
# wecom-cdn — 9 工具
# ============================================================
WECOM_CDN_TOOLS = [
    T("cdn_upload_by_url", "通过文件 URL 上传到 CDN（同步）。返回 fileId 和 cdnKey。适合中小文件（<50MB）。通过 wecom_api 调用，method: /cloud/cdnBigUploadByUrl。",
      {"fileUrl": P("string", "要上传的文件 URL，必须公开可访问")},
      "/cloud/cdnBigUploadByUrl", ["fileUrl"]),
    T("cdn_upload_by_url_async", "通过文件 URL 异步上传到 CDN。适用于大文件（>50MB），立即返回任务 ID。通过 wecom_api 调用，method: /cloud/cdnUploadByUrlAsync。",
      {"fileUrl": P("string", "要上传的文件 URL")},
      "/cloud/cdnUploadByUrlAsync", ["fileUrl"]),
    T("cdn_upload", "直接上传文件数据到 CDN。接受 base64 编码数据，适用于文件不在公网的场景。通过 wecom_api 调用，method: /cloud/cdnBigUpload。",
      {"fileData": P("string", "文件的 base64 编码字符串"), "fileName": P("string", "文件名含扩展名")},
      "/cloud/cdnBigUpload", ["fileData", "fileName"]),
    T("download_qw_file", "下载企业微信内部文件（同步）。通过 fileId 下载聊天中收发的文件，返回下载 URL。fileId 从消息回调 msgData 获取。通过 wecom_api 调用，method: /cloud/wxWorkDownload。",
      {"fileId": P("string", "企业微信文件 ID")},
      "/cloud/wxWorkDownload", ["fileId"]),
    T("download_qw_file_async", "异步下载企业微信内部文件。适用于大文件，立即返回任务 ID。通过 wecom_api 调用，method: /cloud/wxWorkDownloadAsync。",
      {"fileId": P("string", "企业微信文件 ID")},
      "/cloud/wxWorkDownloadAsync", ["fileId"]),
    T("download_cdn_large_async", "异步下载 CDN 大文件。通过文件 URL 从 CDN 下载。通过 wecom_api 调用，method: /cloud/cdnBigFileDownloadByUrlAsync。",
      {"fileUrl": P("string", "CDN 文件的 URL 地址")},
      "/cloud/cdnBigFileDownloadByUrlAsync", ["fileUrl"]),
    T("download_gw_file", "下载个人微信转发的文件（同步）。个微文件使用不同存储体系，不能用 download_qw_file。通过 wecom_api 调用，method: /cloud/wxDownload。",
      {"fileId": P("string", "个人微信文件 ID")},
      "/cloud/wxDownload", ["fileId"]),
    T("download_gw_file_async", "异步下载个人微信转发的文件。大文件场景使用。通过 wecom_api 调用，method: /cloud/wxDownloadAsync。",
      {"fileId": P("string", "个人微信文件 ID")},
      "/cloud/wxDownloadAsync", ["fileId"]),
    T("cdn_to_url", "将 CDN 文件的 cdnKey 转换为可访问的下载 URL。返回的 URL 有时效性，过期需重新调用。通过 wecom_api 调用，method: /cloud/cdnWxDownload。",
      {"cdnKey": P("string", "CDN 文件标识密钥")},
      "/cloud/cdnWxDownload", ["cdnKey"]),
]

WECOM_CDN_README = """## 文件传输 CDN

### 文件体系

| 来源 | 说明 | 下载接口 |
|------|------|---------|
| 企业微信文件 | 企微内部收发 | download_qw_file / download_qw_file_async |
| 个人微信文件 | 从个微转发 | download_gw_file / download_gw_file_async |
| CDN 文件 | 上传到 CDN | cdn_to_url 转为可访问 URL |

### 常见工作流

**上传：** 有 URL → `cdn_upload_by_url`（同步）或 `cdn_upload_by_url_async`（大文件）；有数据 → `cdn_upload`（base64）。

**下载聊天文件：** 从消息回调获取 fileId → 企微文件用 download_qw_file，个微文件用 download_gw_file。

**CDN 转 URL：** 持有 cdnKey → `cdn_to_url` 获取可访问 URL。

### 注意事项

- 企微文件和个微文件使用不同接口，混用会报错
- cdn_to_url 返回的 URL 有时效性
- 大文件建议用 async 版本"""

# ============================================================
# wecom-tag — 3 工具
# ============================================================
WECOM_TAG_TOOLS = [
    T("list_tags", "同步并获取完整标签列表。返回所有标签的 labelId、labelName、成员数等。这是标签管理的起点——编辑标签或给联系人打标签前先调用此接口了解有哪些标签可用。通过 wecom_api 调用，method: /label/syncLabelList。",
      {}, "/label/syncLabelList"),
    T("edit_personal_tag", "创建或编辑个人标签。labelId 不传或为 0 时新建标签，传已有 ID 则编辑。可同时指定要添加/移除的成员。通过 wecom_api 调用，method: /label/editLabel。",
      {"labelId": P("number", "标签 ID。不传或 0=新建，传已有 ID=编辑"),
       "labelName": P("string", "标签名称，新建时必填"),
       "addUserIds": P("array", "要添加到此标签的联系人 ID 列表", items={"type": "string"}),
       "delUserIds": P("array", "要从此标签移除的联系人 ID 列表", items={"type": "string"})},
      "/label/editLabel"),
    T("edit_customer_tag", "从联系人维度管理标签。为指定联系人设置标签列表（全量替换，非增量）。当用户说「给张三打上 XX 标签」时使用。注意 labelIds 会覆盖该联系人的所有标签。通过 wecom_api 调用，method: /label/contactEditLabel。",
      {"userId": P("string", "目标联系人 ID"),
       "labelIds": P("array", "要关联的标签 ID 列表（全量替换）", items={"type": "number"})},
      "/label/contactEditLabel", ["userId", "labelIds"]),
]

WECOM_TAG_README = """## 标签管理

### 常见工作流

**查看标签：** `list_tags` 获取全部标签。

**创建标签并添加成员：** `edit_personal_tag`（不传 labelId，填 labelName 和 addUserIds）。

**给联系人打标签：** `list_tags` 获取标签 ID → `edit_customer_tag` 传入联系人 ID 和标签 ID 列表。

### 注意事项

- **两个视角：** `edit_personal_tag` 是标签→成员视角，`edit_customer_tag` 是成员→标签视角
- **edit_customer_tag 的 labelIds 是全量替换**，不是增量追加
- **标签 ID 是数字类型**，不是字符串"""

# ============================================================
# wecom-session — 3 工具
# ============================================================
WECOM_SESSION_TOOLS = [
    T("list_sessions", "分页获取会话列表。每个会话包含 sessionId、类型、最近消息摘要、未读数等。首次 currentSeq 传 0，后续传返回的 seq 值翻页。返回值含 hasMore 字段。通过 wecom_api 调用，method: /session/getSessionPage。",
      {"currentSeq": P("number", "分页游标，首次传 0，后续传上次返回的 seq 值"),
       "pageSize": P("number", "每页数量，默认 20，建议不超过 50")},
      "/session/getSessionPage"),
    T("manage_session", "对指定会话执行管理操作。通过 cmd 参数指定：40=置顶会话，41=取消置顶，3=标记已读，7=删除会话。删除会话仅从列表移除不影响聊天记录，但不可撤销。通过 wecom_api 调用，method: /session/setSessionCmd。",
      {"sessionId": P("string", "目标会话 ID，从 list_sessions 获取"),
       "cmd": P("number", "操作命令：40=置顶，41=取消置顶，3=标记已读，7=删除会话")},
      "/session/setSessionCmd", ["sessionId", "cmd"]),
    T("list_session_groups", "获取会话分组列表（置顶组、普通组等）。与 list_sessions 区别：list_sessions 返回扁平分页列表，本接口返回按分组归类的结构化视图。通过 wecom_api 调用，method: /session/getSessionList。",
      {}, "/session/getSessionList"),
]

WECOM_SESSION_README = """## 会话管理

### 常见工作流

**浏览会话：** `list_sessions` (currentSeq=0) → 查看未读数和最近消息 → 用返回的 seq 翻页。

**置顶：** `list_sessions` 找到 sessionId → `manage_session` (cmd=40) 置顶 / (cmd=41) 取消。

**标记已读：** `manage_session` (cmd=3)。

### 注意事项

- 分页用游标（currentSeq），不是页码
- cmd 值：40=置顶、41=取消置顶、3=标记已读、7=删除
- sessionId 通过 list_sessions 获取，不要猜测格式"""

# ============================================================
# 汇总所有 skills
# ============================================================
ALL_SKILLS = [
    {"id": "wecom-core", "name": "企微核心能力",
     "description": "日常沟通基础：发消息、查联系人、查群列表、通用 API 调用",
     "type": "action", "tools": WECOM_CORE_TOOLS, "readme": WECOM_CORE_README},
    {"id": "wecom-group-mgmt", "name": "群管理",
     "description": "创建群聊、管理群成员、设置群公告、转让群主等群组管理操作",
     "type": "action", "tools": WECOM_GROUP_TOOLS, "readme": WECOM_GROUP_README},
    {"id": "wecom-contact-mgmt", "name": "联系人管理",
     "description": "添加好友、通过好友申请、修改联系人信息、删除联系人等",
     "type": "action", "tools": WECOM_CONTACT_TOOLS, "readme": WECOM_CONTACT_README},
    {"id": "wecom-message-mgmt", "name": "消息管理",
     "description": "撤回消息、置顶消息、群发消息、同步历史消息、发送富媒体消息等",
     "type": "action", "tools": WECOM_MESSAGE_TOOLS, "readme": WECOM_MESSAGE_README},
    {"id": "wecom-moment", "name": "朋友圈",
     "description": "浏览朋友圈、发布动态、点赞、评论等朋友圈操作",
     "type": "action", "tools": WECOM_MOMENT_TOOLS, "readme": WECOM_MOMENT_README},
    {"id": "wecom-cdn", "name": "文件传输",
     "description": "上传文件到 CDN、下载企微/个微文件、CDN 链接转换",
     "type": "action", "tools": WECOM_CDN_TOOLS, "readme": WECOM_CDN_README},
    {"id": "wecom-tag", "name": "标签管理",
     "description": "查看标签列表、编辑个人标签、编辑客户标签",
     "type": "action", "tools": WECOM_TAG_TOOLS, "readme": WECOM_TAG_README},
    {"id": "wecom-session", "name": "会话管理",
     "description": "查看会话列表、管理会话分组、置顶/标记已读",
     "type": "action", "tools": WECOM_SESSION_TOOLS, "readme": WECOM_SESSION_README},
]

def main():
    db = sqlite3.connect(DB_PATH)
    cur = db.cursor()

    for s in ALL_SKILLS:
        tools_json = json.dumps(s["tools"], ensure_ascii=False)
        cur.execute("""
            INSERT OR REPLACE INTO skills
              (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
            VALUES (?, ?, ?, '2.0.0', ?, 1, '[]', ?, ?, '{}')
        """, (s["id"], s["name"], s["description"], s["type"], tools_json, s["readme"].strip()))
        print(f"  ✓ {s['id']}: {s['name']} ({len(s['tools'])} tools)")

    db.commit()

    # Verify
    cur.execute("SELECT id, name, LENGTH(tools), LENGTH(readme) FROM skills WHERE id LIKE 'wecom-%' ORDER BY id")
    print("\n--- 验证 ---")
    for row in cur.fetchall():
        print(f"  {row[0]}: name={row[1]}, tools_len={row[2]}, readme_len={row[3]}")

    db.close()
    print("\nDone.")

if __name__ == "__main__":
    main()
