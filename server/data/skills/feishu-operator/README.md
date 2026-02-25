# 飞书高级操作

提供表情回复、多模态消息（图片/语音/文件）、文档读写等高级飞书能力。

> **基础回复**已内置于 `send_channel_message`，无需此 skill。
> **查询与协作**（查群/查人/发文本/建群/约会议）由 `feishu-agent` skill 提供。

## 架构

```
Agent ──feishu_xxx tool──▸ callFeishuAction() ──HTTP POST──▸ channel-feishu(:1998) ──SDK──▸ 飞书 API
```

## 能力清单

### 1. 表情回复（Reaction）

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_add_reaction` | 给消息添加表情回复 | `message_id`, `emoji_type` |
| `feishu_delete_reaction` | 删除消息上的表情回复 | `message_id`, `reaction_id` |

**常用 emoji_type**：`THUMBSUP`（👍）、`HEART`（❤️）、`SMILE`（😊）、`LAUGH`（😂）、`CLAP`（👏）、`FIRE`（🔥）、`OK`（👌）、`ROCKET`（🚀）、`MUSCLE`（💪）、`PARTY`（🎉）

### 2. 图片

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_upload_image` | 从 URL 下载图片并上传到飞书，返回 `image_key` | `image_url` |
| `feishu_send_image` | 发送图片消息（需先获取 `image_key`） | `receive_id`, `image_key` |

**流程**：先 `feishu_upload_image` 获取 key → 再 `feishu_send_image` 发送。

### 3. 语音

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_upload_file` | 从 URL 上传语音文件（file_type=opus） | `file_url`, `file_name`, `file_type` |
| `feishu_send_audio` | 发送语音消息（需先获取 `file_key`） | `receive_id`, `file_key` |

**流程**：先 `feishu_upload_file`（type=opus, 传 duration）→ 再 `feishu_send_audio`。

### 4. 文件

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_upload_file` | 从 URL 上传文件（pdf/doc/xls/ppt/mp4/stream） | `file_url`, `file_name`, `file_type` |
| `feishu_send_file` | 发送文件消息（需先获取 `file_key`） | `receive_id`, `file_key` |

### 5. 消息回复

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_reply_message` | 回复指定消息（引用回复） | `message_id`, `content` |

### 6. 文档读写

| Tool | 用途 | 必填参数 |
|------|------|----------|
| `feishu_get_document` | 获取文档元信息（标题、时间） | `document_id` |
| `feishu_get_document_content` | 获取文档全文文本 | `document_id` |
| `feishu_append_document` | 向文档追加内容块 | `document_id`, `block_id`, `blocks` |

## 典型场景

### 给用户消息点赞

```json
{ "tool": "feishu_add_reaction", "input": { "message_id": "om_xxx", "emoji_type": "THUMBSUP" } }
```

### 发送一张图片

```json
// Step 1: 上传
{ "tool": "feishu_upload_image", "input": { "image_url": "https://example.com/img.png" } }
// 返回 { "image_key": "img_v3_xxx" }

// Step 2: 发送
{ "tool": "feishu_send_image", "input": { "receive_id": "oc_xxx", "image_key": "img_v3_xxx" } }
```

### 读取并追加文档

```json
// 读取
{ "tool": "feishu_get_document_content", "input": { "document_id": "doxcnXXX" } }

// 追加段落
{
  "tool": "feishu_append_document",
  "input": {
    "document_id": "doxcnXXX",
    "block_id": "doxcnXXX",
    "blocks": [{ "blockType": "paragraph", "text": "新增内容" }]
  }
}
```

## 注意事项

- **channel-feishu 服务必须运行**（默认 `localhost:1998`）
- 图片/文件/语音均采用 **URL → 上传 → 发送** 两步流程
- `file_type` 支持：`opus`（语音）、`mp4`（视频）、`pdf`、`doc`、`xls`、`ppt`、`stream`（通用）
- 语音上传时需传 `duration`（毫秒字符串）
- 文档 `append_document` 的 `block_id` 追加到根级别时与 `document_id` 相同
