# 飞书 Agent 能力

通过 `channel-feishu` 服务与飞书交互。所有能力以 `feishu_` 开头的 tool 形式提供。

## 架构

```
Agent --feishu_xxx tool--> skill executor --HTTP POST--> channel-feishu(:1998) --SDK--> 飞书 API
```

## ID 类型速查

| 类型 | 格式 | 说明 |
|------|------|------|
| `chat_id` | `oc_xxx` | 群聊 ID（**默认值**） |
| `open_id` | `ou_xxx` | 用户 open_id |
| `user_id` | - | 用户 user_id |
| `union_id` | - | 跨应用统一 ID |
| `email` | `user@example.com` | 用户邮箱 |

## 能力清单

### 1. 信息查询（重点能力）

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_list_bot_chats` | **列出机器人所在的所有群**（群名、群ID、描述） | 无 |
| `feishu_get_chat_info` | **获取群详细信息**（群名称、描述、群主、成员数） | `chat_id` |
| `feishu_get_chat_members` | **获取群全部成员**（姓名、open_id、角色） | `chat_id` |
| `feishu_search_chats` | **按关键词搜索群组** | `query` |
| `feishu_get_user_info` | **获取用户详细信息**（姓名、头像、部门、工号） | `user_id` |
| `feishu_batch_get_user_id` | **通过邮箱/手机号查找用户 ID** | `emails` 或 `mobiles` |

### 2. 消息

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_send_text` | 发送文本消息 | `receive_id`, `text` |
| `feishu_send_rich_text` | 发送富文本（链接、@、加粗） | `receive_id`, `title`, `content` |
| `feishu_send_default_card` | 发送标题+内容卡片 | `receive_id`, `title`, `content` |
| `feishu_get_message_list` | 获取群聊消息列表 | `chat_id` |

### 3. 群组管理

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_create_chat` | 创建群组 | `name` |

### 4. 会议

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_reserve_meeting` | 预约会议 | `topic`, `end_time` |

### 5. 文档

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_create_document` | 创建文档 | `title` |

## 常用查询场景

### 查看机器人在哪些群里

```json
{ "tool": "feishu_list_bot_chats", "input": {} }
```

返回所有群的 `chat_id`、`name`、`description`、`owner_id` 等。

### 查某个群叫什么名字、有多少人

```json
{ "tool": "feishu_get_chat_info", "input": { "chat_id": "oc_xxx" } }
```

返回 `name`、`description`、`owner_id`、`member_count` 等。

### 查某个群里都有谁

```json
{ "tool": "feishu_get_chat_members", "input": { "chat_id": "oc_xxx" } }
```

返回成员列表，每个成员包含 `member_id`（open_id）、`name`、`tenant_key` 等。

### 按名字搜索群

```json
{ "tool": "feishu_search_chats", "input": { "query": "项目 Alpha" } }
```

### 查某个人的详细信息

```json
{ "tool": "feishu_get_user_info", "input": { "user_id": "ou_xxx" } }
```

返回 `name`、`en_name`、`avatar`、`department_ids`、`employee_no` 等。

### 通过邮箱找到某人的 open_id

```json
{
  "tool": "feishu_batch_get_user_id",
  "input": { "emails": ["zhangsan@company.com"] }
}
```

### 组合查询：某个群里所有人的详细信息

1. `feishu_get_chat_members` 获取成员 open_id 列表
2. 对每个 open_id 调用 `feishu_get_user_info` 获取详情

## 注意事项

- **channel-feishu 必须运行**：tool 执行前会连接 `localhost:1999`，未启动会报错
- **权限依赖**：查询群信息需要机器人在群内；查询用户信息需要应用有通讯录权限
- **ID 解析**：当用户说"查一下 XX 群"时，先用 `feishu_search_chats` 或 `feishu_list_bot_chats` 找到 chat_id，再查详情
- **时间戳用秒级 Unix 时间戳字符串**：如 `"1700000000"`