---
name: feishu-agent
description: "团队协作与即时通讯。发消息、管理群组与成员、查找用户、预约/管理会议、创建与编辑文档、管理知识库和云空间。当需要向同事或群组传达信息、查找用户、组织会议、创建协作文档、或访问知识库时使用。"
---

# 团队协作与即时通讯

## 调用方式

通过 `http_request` 工具执行 curl 命令：

```bash
curl -X POST http://localhost:1999/api/feishu/action \
  -H "Content-Type: application/json" \
  -d '{"action":"<action>","params":{...}}'
```

`command` 参数为完整 curl 命令字符串，JSON 引号需按 JSON 规则转义。

## 能力速查

### 发消息

| 想做什么 | action + 必填 params |
|---------|---------------------|
| 给某人/群发文字 | `send_text` — receive_id, text（可选 receive_id_type，默认 chat_id） |
| 发富文本消息 | `send_rich_text` — receive_id, title, content（二维数组） |
| 发卡片消息 | `send_card` — receive_id + template_id/template_variable 或 card_content |
| 发简单卡片 | `send_default_card` — receive_id, title, content |
| 发图片 | 先 `upload_image_from_url(image_url)` 获取 image_key → `send_image(receive_id, image_key)` |
| 发文件 | 先 `upload_file_from_url(file_url, file_name, file_type)` 获取 file_key → `send_file(receive_id, file_key)` |
| 发语音 | 先 `upload_file_from_url(..., file_type=opus)` 获取 file_key → `send_audio(receive_id, file_key)` |
| 回复某条消息 | `reply_message` — message_id, content（可选 msg_type） |
| 撤回消息 | `recall_message` — message_id |
| 看消息记录 | `get_message_list` — chat_id（可选 page_size, page_token） |
| 给消息加/删表情 | `add_reaction` / `delete_reaction` — message_id, emoji_type/reaction_id |

### 群组与联系人

| 想做什么 | action + 必填 params |
|---------|---------------------|
| 看有哪些群 | `list_bot_chats` |
| 找某个群 | `search_chats` — query |
| 查群详情/成员 | `get_chat_info` / `get_chat_members` — chat_id |
| 建群 | `create_chat` — name（可选 description, user_id_list） |
| 查某人信息 | `get_user_info` — user_id |
| 通过邮箱/手机找人 | `batch_get_user_id` — emails 或 mobiles |

### 开会

| 想做什么 | action + 必填 params |
|---------|---------------------|
| 预约会议 | `reserve_meeting` — topic, end_time（可选 start_time, invitees, settings） |
| 邀人入会 | `invite_to_meeting` — meeting_id, invitees |
| 结束会议 | `end_meeting` — meeting_id |
| 录制会议 | `start_recording` / `stop_recording` — meeting_id |
| 查会议/录制 | `get_meeting` / `get_meeting_recording` — meeting_id |

### 文档与知识库

| 想做什么 | action + 必填 params |
|---------|---------------------|
| 创建文档 | `create_document` — title |
| 看文档内容 | `get_document_content` — document_id |
| 往文档里加内容 | `append_document` — document_id, block_id, blocks。根级追加时 block_id = document_id |
| 浏览知识库 | `get_wiki_spaces` → `get_wiki_node(space_id, node_token)` |
| 创建知识库页面 | `create_wiki_node` — space_id, title |
| 管理云空间文件 | `get_root_folder` → `get_folder_contents(folder_token)` / `create_folder(name)` |

## 关键约束

- **ID 格式**：chat_id = `oc_xxx`（群聊，默认 receive_id_type），open_id = `ou_xxx`，email = `user@company.com`
- **时间戳**：秒级 Unix 字符串
- **emoji_type 可选值**：THUMBSUP, HEART, SMILE, LAUGH, CLAP, FIRE, OK, ROCKET, MUSCLE, PARTY
