# Feishu Bot Service

飞书机器人服务 — 接收飞书消息，提供消息发送（多模态）、视频会议、文档操作等能力。

## 功能概览

### 消息能力
- **接收消息**：通过 WebSocket 长连接实时接收飞书消息事件
- **统一发送消息**：文本、富文本、卡片、图片、文件、音频、视频，一个接口搞定
- **@用户**：发送时可指定 @的用户列表
- **引用回复**：发送时可指定引用的消息ID
- **撤回消息**：撤回已发送的消息
- **消息查询**：获取消息详情、会话消息列表
- **群组管理**：创建群组、获取群信息、获取群成员

### 会议能力
- **预约会议**：创建视频会议，支持设置主题、时间、参会人
- **获取会议详情**：查看会议信息和参会人
- **邀请参会人**：向进行中的会议添加参会人
- **结束会议**：结束进行中的会议
- **会议录制**：开始/停止/获取会议录制

### 文档能力
- **创建文档**：创建新的飞书文档
- **获取文档内容**：获取文档信息、纯文本内容、文档块
- **追加文档内容**：向文档中追加段落、代码块、分割线等内容
- **知识库操作**：获取知识库列表、创建知识库节点
- **云空间操作**：获取文件列表、创建文件夹

## 快速开始

### 1. 创建飞书应用

1. 前往 [飞书开放平台](https://open.feishu.cn/app/) 创建应用
2. 获取 `App ID` 和 `App Secret`
3. 在「权限管理」中添加以下权限：
   - `im:message` — 发送和接收消息
   - `im:message:send_as_bot` — 以机器人身份发送消息
   - `im:chat` — 群组管理
   - `vc:meeting` — 视频会议管理
   - `docx:document` — 文档管理
   - `wiki:wiki` — 知识库管理
   - `drive:drive` — 云空间管理
4. 在「事件订阅」中添加 `im.message.receive_v1` 事件
5. 在「机器人」功能中启用机器人

### 2. 配置环境变量

```bash
cp .env.example ../.env  # 环境变量放在项目根目录
```

编辑 `.env` 文件，填入飞书应用凭证：

```env
FEISHU_APP_ID=cli_xxxxxxxxxxxxxxxx
FEISHU_APP_SECRET=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### 3. 安装依赖并启动

```bash
cd channel-feishu
bun install
bun run dev
```

服务启动后：
- REST API: `http://localhost:1999/api/feishu`
- WebSocket 长连接会自动与飞书建立连接，接收消息事件

## 架构

```
channel-feishu/
├── src/
│   ├── index.ts              # 入口：Express + WSClient
│   ├── config.ts             # 配置管理
│   ├── client.ts             # 飞书 Client 单例
│   ├── types/
│   │   └── index.ts          # TypeScript 类型定义
│   ├── events/
│   │   ├── index.ts          # EventDispatcher 创建
│   │   └── message.ts        # 消息事件处理
│   ├── services/
│   │   ├── message.ts        # 消息服务（发送/回复/上传等）
│   │   ├── meeting.ts        # 会议服务（预约/邀请/录制等）
│   │   └── document.ts       # 文档服务（创建/编辑/知识库等）
│   └── routes/
│       ├── index.ts          # 路由聚合
│       ├── send.ts           # 统一消息发送端点
│       ├── action.ts         # Agent 统一 Action 端点
│       ├── message.ts        # 消息查询与管理 API
│       ├── meeting.ts        # 会议 API 路由
│       └── document.ts       # 文档 API 路由
├── package.json
├── tsconfig.json
└── .env.example
```

### 事件接收模式

本服务**优先使用 WebSocket 长连接模式**（推荐）：

| 特性 | 长连接模式 | Webhook 模式 |
|------|-----------|-------------|
| 需要公网 IP | ❌ 不需要 | ✅ 需要 |
| 本地开发 | ✅ 直接可用 | ❌ 需要内网穿透 |
| 加密解密 | ❌ SDK 自动处理 | ✅ 需要配置 |
| 签名验证 | ❌ 连接时验证 | ✅ 每次请求验证 |

如果 WebSocket 连接失败，服务会自动回退到 Webhook 模式。

---

## API 参考

**Base URL**: `http://localhost:1999`

所有接口统一返回格式：

```json
{
  "success": true,
  "data": { ... }
}
```

错误时：

```json
{
  "success": false,
  "error": "错误描述"
}
```

---

### 统一消息发送

#### `POST /api/feishu/send`

所有消息发送都通过这一个接口完成。通过 `content.type` 区分消息类型。

**请求参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `receiveId` | string | 是 | 发送目标：群 chat_id 或用户 open_id |
| `receiveIdType` | string | 否 | ID 类型，默认 `"chat_id"`。可选：`open_id` / `user_id` / `union_id` / `email` / `chat_id` |
| `replyToMessageId` | string | 否 | 引用回复的消息ID，传入后消息会以引用形式发送 |
| `mentions` | string[] | 否 | 要 @的用户ID列表（open_id），仅文本和富文本消息有效 |
| `content` | object | 是 | 消息内容，见下方各类型说明 |

---

#### 发送文本消息

```bash
curl -X POST http://localhost:1999/api/feishu/send \
  -H "Content-Type: application/json" \
  -d '{
    "receiveId": "oc_xxx",
    "content": {
      "type": "text",
      "text": "Hello from bot!"
    }
  }'
```

**带 @用户 + 引用回复：**

```bash
curl -X POST http://localhost:1999/api/feishu/send \
  -H "Content-Type: application/json" \
  -d '{
    "receiveId": "oc_xxx",
    "replyToMessageId": "om_xxx",
    "mentions": ["ou_user1", "ou_user2"],
    "content": {
      "type": "text",
      "text": "请查看一下这个问题"
    }
  }'
```

> 长文本（≥800字符）会自动转为飞书富文本格式以保留换行。

---

#### 发送富文本消息

飞书 post 格式，支持文字、链接、@人、图片等混排。

```bash
curl -X POST http://localhost:1999/api/feishu/send \
  -H "Content-Type: application/json" \
  -d '{
    "receiveId": "oc_xxx",
    "content": {
      "type": "rich_text",
      "title": "公告标题",
      "content": [
        [
          { "tag": "text", "text": "这是一段" },
          { "tag": "a", "text": "链接", "href": "https://example.com" },
          { "tag": "at", "user_id": "ou_xxx", "user_name": "张三" }
        ],
        [
          { "tag": "text", "text": "第二行内容" }
        ]
      ]
    }
  }'
```

**富文本元素类型：**

| tag | 说明 | 字段 |
|-----|------|------|
| `text` | 文本 | `text`, `style?` |
| `a` | 链接 | `text`, `href` |
| `at` | @用户 | `user_id`, `user_name?` |
| `img` | 图片 | `image_key`, `width?`, `height?` |
| `media` | 媒体 | `file_key`, `image_key?` |
| `emotion` | 表情 | `emoji_type` |

---

#### 发送卡片消息（模板）

使用飞书搭建工具预设的卡片模板：

```bash
curl -X POST http://localhost:1999/api/feishu/send \
  -H "Content-Type: application/json" \
  -d '{
    "receiveId": "oc_xxx",
    "content": {
      "type": "card",
      "templateId": "AAqk1234567890",
      "templateVariable": {
        "title": "卡片标题",
        "content": "卡片正文"
      }
    }
  }'
```

#### 发送卡片消息（自定义JSON）

传入完整的卡片 JSON 结构：

```bash
curl -X POST http://localhost:1999/api/feishu/send \
  -H "Content-Type: application/json" \
  -d '{
    "receiveId": "oc_xxx",
    "content": {
      "type": "card",
      "cardContent": {
        "header": {
          "title": { "tag": "plain_text", "content": "通知" }
        },
        "elements": [
          { "tag": "div", "text": { "tag": "lark_md", "content": "这是卡片内容" } }
        ]
      }
    }
  }'
```

---

#### 发送图片消息

图片需先通过飞书上传接口获取 `image_key`。

```bash
curl -X POST http://localhost:1999/api/feishu/send \
  -H "Content-Type: application/json" \
  -d '{
    "receiveId": "oc_xxx",
    "content": {
      "type": "image",
      "imageKey": "img_v2_xxx"
    }
  }'
```

---

#### 发送文件消息

文件需先通过飞书上传接口获取 `file_key`。

```bash
curl -X POST http://localhost:1999/api/feishu/send \
  -H "Content-Type: application/json" \
  -d '{
    "receiveId": "oc_xxx",
    "content": {
      "type": "file",
      "fileKey": "file_v2_xxx"
    }
  }'
```

---

#### 发送音频消息

```bash
curl -X POST http://localhost:1999/api/feishu/send \
  -H "Content-Type: application/json" \
  -d '{
    "receiveId": "oc_xxx",
    "content": {
      "type": "audio",
      "fileKey": "file_v2_xxx"
    }
  }'
```

---

#### 发送视频消息

```bash
curl -X POST http://localhost:1999/api/feishu/send \
  -H "Content-Type: application/json" \
  -d '{
    "receiveId": "oc_xxx",
    "content": {
      "type": "video",
      "fileKey": "file_v2_xxx",
      "imageKey": "img_v2_xxx"
    }
  }'
```

---

### 消息查询与管理

#### `GET /api/feishu/message/:messageId` — 获取消息详情

```bash
curl http://localhost:1999/api/feishu/message/om_xxx
```

---

#### `GET /api/feishu/message/list/:chatId` — 获取会话消息列表

```bash
# 默认获取最近 20 条
curl http://localhost:1999/api/feishu/message/list/oc_xxx

# 分页
curl "http://localhost:1999/api/feishu/message/list/oc_xxx?pageSize=50&pageToken=xxx"
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `pageSize` | number | 否 | 每页条数，默认 20 |
| `pageToken` | string | 否 | 分页 token |

---

#### `DELETE /api/feishu/message/:messageId` — 撤回消息

```bash
curl -X DELETE http://localhost:1999/api/feishu/message/om_xxx
```

---

### 群组管理

#### `GET /api/feishu/message/chat/:chatId` — 获取群信息

```bash
curl http://localhost:1999/api/feishu/message/chat/oc_xxx
```

---

#### `GET /api/feishu/message/chat/:chatId/members` — 获取群成员列表

```bash
curl http://localhost:1999/api/feishu/message/chat/oc_xxx/members
```

---

#### `POST /api/feishu/message/chat` — 创建群组

```bash
curl -X POST http://localhost:1999/api/feishu/message/chat \
  -H "Content-Type: application/json" \
  -d '{
    "name": "项目讨论群",
    "description": "用于日常项目沟通",
    "userIdList": ["ou_user1", "ou_user2"]
  }'
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 群名称 |
| `description` | string | 否 | 群描述 |
| `userIdList` | string[] | 否 | 初始成员 open_id 列表 |

---

### 会议 API

#### `POST /api/feishu/meeting/reserve` — 预约会议

```bash
curl -X POST http://localhost:1999/api/feishu/meeting/reserve \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "项目周会",
    "startTime": "1700000000",
    "endTime": "1700003600",
    "invitees": [
      { "id": "ou_xxx", "idType": "open_id" }
    ],
    "settings": {
      "password": "123456"
    }
  }'
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `topic` | string | 是 | 会议主题 |
| `startTime` | string | 是 | 开始时间（Unix 时间戳，秒） |
| `endTime` | string | 是 | 结束时间（Unix 时间戳，秒） |
| `invitees` | array | 否 | 参会人列表，每项含 `id` 和 `idType` |
| `settings.password` | string | 否 | 入会密码 |

---

#### `GET /api/feishu/meeting/:meetingId` — 获取会议详情

```bash
curl http://localhost:1999/api/feishu/meeting/7xxx
```

---

#### `POST /api/feishu/meeting/:meetingId/invite` — 邀请参会人

```bash
curl -X POST http://localhost:1999/api/feishu/meeting/7xxx/invite \
  -H "Content-Type: application/json" \
  -d '{
    "invitees": [
      { "id": "ou_xxx", "userType": 1 }
    ]
  }'
```

---

#### `POST /api/feishu/meeting/:meetingId/end` — 结束会议

```bash
curl -X POST http://localhost:1999/api/feishu/meeting/7xxx/end
```

---

#### `GET /api/feishu/meeting/:meetingId/recording` — 获取会议录制

```bash
curl http://localhost:1999/api/feishu/meeting/7xxx/recording
```

---

#### `POST /api/feishu/meeting/:meetingId/recording/start` — 开始录制

```bash
curl -X POST http://localhost:1999/api/feishu/meeting/7xxx/recording/start
```

---

#### `POST /api/feishu/meeting/:meetingId/recording/stop` — 停止录制

```bash
curl -X POST http://localhost:1999/api/feishu/meeting/7xxx/recording/stop
```

---

### 文档 API

#### `POST /api/feishu/document` — 创建文档

```bash
curl -X POST http://localhost:1999/api/feishu/document \
  -H "Content-Type: application/json" \
  -d '{
    "title": "会议纪要 2025-01-01",
    "folderToken": "fldcnxxxxxxx"
  }'
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `title` | string | 是 | 文档标题 |
| `folderToken` | string | 否 | 目标文件夹 token |

---

#### `GET /api/feishu/document/:documentId` — 获取文档信息

```bash
curl http://localhost:1999/api/feishu/document/doxcnxxxxxxx
```

---

#### `GET /api/feishu/document/:documentId/raw` — 获取文档纯文本

```bash
curl http://localhost:1999/api/feishu/document/doxcnxxxxxxx/raw
```

---

#### `GET /api/feishu/document/:documentId/blocks` — 获取文档块

```bash
curl http://localhost:1999/api/feishu/document/doxcnxxxxxxx/blocks
```

---

#### `POST /api/feishu/document/:documentId/blocks` — 追加文档内容

```bash
curl -X POST http://localhost:1999/api/feishu/document/doxcnxxxxxxx/blocks \
  -H "Content-Type: application/json" \
  -d '{
    "blockId": "doxcnxxxxxxx",
    "blocks": [
      { "blockType": "paragraph", "text": "标题", "style": "heading1" },
      { "blockType": "paragraph", "text": "这是正文内容" },
      { "blockType": "code", "code": "console.log(\"hello\")", "language": "javascript" },
      { "blockType": "divider" }
    ]
  }'
```

**支持的块类型：**

| blockType | 说明 | 字段 |
|-----------|------|------|
| `paragraph` | 段落 | `text`, `style?`（heading1-4 / normal） |
| `code` | 代码块 | `code`, `language?` |
| `callout` | 高亮块 | `text` |
| `divider` | 分割线 | 无 |

---

### 知识库 API

#### `GET /api/feishu/document/wiki/spaces` — 获取知识库列表

```bash
curl http://localhost:1999/api/feishu/document/wiki/spaces
```

---

#### `GET /api/feishu/document/wiki/:spaceId/nodes` — 获取知识库节点

```bash
curl "http://localhost:1999/api/feishu/document/wiki/7xxx/nodes?parentNodeToken=wikcnxxx"
```

---

#### `POST /api/feishu/document/wiki/node` — 创建知识库节点

```bash
curl -X POST http://localhost:1999/api/feishu/document/wiki/node \
  -H "Content-Type: application/json" \
  -d '{
    "spaceId": "7xxx",
    "parentNodeToken": "wikcnxxx",
    "title": "新文档节点"
  }'
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `spaceId` | string | 是 | 知识库 space_id |
| `parentNodeToken` | string | 否 | 父节点 token，不填则在根目录 |
| `title` | string | 是 | 节点标题 |
| `nodeType` | string | 否 | `"origin"` 或 `"shortcut"` |

---

### 云空间 API

#### `GET /api/feishu/document/drive/files` — 获取文件列表

```bash
# 获取根目录
curl http://localhost:1999/api/feishu/document/drive/files

# 获取指定文件夹
curl "http://localhost:1999/api/feishu/document/drive/files?folderToken=fldcnxxx"
```

---

#### `POST /api/feishu/document/drive/folder` — 创建文件夹

```bash
curl -X POST http://localhost:1999/api/feishu/document/drive/folder \
  -H "Content-Type: application/json" \
  -d '{
    "name": "新文件夹",
    "folderToken": "fldcnxxx"
  }'
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 文件夹名称 |
| `folderToken` | string | 否 | 父文件夹 token，不填则在根目录 |

---

### Agent Action 端点

Agent 统一调用入口，通过 `action` 字段分发到对应能力。

#### `POST /api/feishu/action` — 执行 Action

```bash
curl -X POST http://localhost:1999/api/feishu/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "send_text",
    "params": {
      "receive_id": "oc_xxx",
      "text": "Hello!"
    }
  }'
```

#### `GET /api/feishu/action/list` — 列出所有可用 Action

```bash
curl http://localhost:1999/api/feishu/action/list
```

**可用 Action 列表：**

| Action | 说明 | 关键参数 |
|--------|------|----------|
| `send_text` | 发送文本消息 | `receive_id`, `text` |
| `send_rich_text` | 发送富文本消息 | `receive_id`, `title`, `content` |
| `send_card` | 发送卡片消息 | `receive_id`, `template_id` 或 `card_content` |
| `send_default_card` | 发送简单卡片 | `receive_id`, `title`, `content` |
| `send_image` | 发送图片消息 | `receive_id`, `image_key` |
| `send_file` | 发送文件消息 | `receive_id`, `file_key` |
| `reply_message` | 回复消息 | `message_id`, `content` |
| `recall_message` | 撤回消息 | `message_id` |
| `get_message` | 获取消息详情 | `message_id` |
| `get_message_list` | 获取消息列表 | `chat_id` |
| `create_chat` | 创建群组 | `name` |
| `get_chat_info` | 获取群信息 | `chat_id` |
| `get_chat_members` | 获取群成员 | `chat_id` |
| `reserve_meeting` | 预约会议 | `topic`, `start_time`, `end_time` |
| `get_meeting` | 获取会议详情 | `meeting_id` |
| `invite_to_meeting` | 邀请参会人 | `meeting_id`, `invitees` |
| `end_meeting` | 结束会议 | `meeting_id` |
| `start_recording` | 开始录制 | `meeting_id` |
| `stop_recording` | 停止录制 | `meeting_id` |
| `get_meeting_recording` | 获取录制列表 | `meeting_id` |
| `create_document` | 创建文档 | `title` |
| `get_document` | 获取文档信息 | `document_id` |
| `get_document_content` | 获取文档纯文本 | `document_id` |
| `get_document_blocks` | 获取文档块 | `document_id` |
| `append_document` | 追加文档内容 | `document_id`, `block_id`, `blocks` |
| `get_wiki_spaces` | 获取知识库列表 | 无 |
| `get_wiki_node` | 获取知识库节点 | `space_id`, `node_token` |
| `create_wiki_node` | 创建知识库节点 | `space_id`, `title` |
| `get_root_folder` | 获取根文件夹 | 无 |
| `get_folder_contents` | 获取文件夹内容 | `folder_token` |
| `create_folder` | 创建文件夹 | `name` |

---

### 健康检查

```bash
curl http://localhost:1999/health
# 或
curl http://localhost:1999/api/health
```

---

## 自定义消息处理

通过 `onMessage` 注册自定义消息处理器：

```typescript
import { onMessage } from './events';

onMessage(async (msg) => {
  if (msg.messageType === 'text') {
    const content = JSON.parse(msg.content);
    console.log(`收到文本: ${content.text}`);
    // 你的自定义处理逻辑...
  }
});
```

## 技术栈

- **Runtime**: [Bun](https://bun.sh/)
- **Framework**: Express 5
- **SDK**: [@larksuiteoapi/node-sdk](https://www.npmjs.com/package/@larksuiteoapi/node-sdk)
- **Language**: TypeScript
- **Event Mode**: WebSocket 长连接（主） + Webhook 回调（备）
