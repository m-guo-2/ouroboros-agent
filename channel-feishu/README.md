# Feishu Bot Service (Go)

飞书独立渠道服务，现已完全迁移为 Go 实现。

## 运行

1. 配置根目录 `.env`（可参考 `channel-feishu/.env.example`）
2. 启动服务：

```bash
cd channel-feishu
go run .
```

默认地址：

- `http://localhost:1999/health`
- `http://localhost:1999/api/health`
- `http://localhost:1999/api/feishu/*`
- `http://localhost:1999/webhook/event`

## 能力范围

- 消息：发送/回复/撤回/查询、群信息、群成员、建群、表情回复
- 媒体：图片/文件上传（URL 下载后上传飞书）
- 会议：预约、详情、邀请、结束、录制开始/停止/查询
- 文档：创建、获取、获取 raw、获取 blocks、追加 blocks
- 知识库：获取 spaces、获取 nodes、创建 node
- 云空间：获取文件、创建文件夹
- Action：`POST /api/feishu/action` + `GET /api/feishu/action/list`
- 事件：Webhook 事件处理 + WebSocket 长连接（失败时可继续使用 Webhook）
- Agent 联动：转发到 `POST {AGENT_SERVER_URL}/api/channels/incoming`

## 兼容性

- 保留 `POST /api/feishu/send` 新旧两种格式（`SendRequest` + legacy `OutgoingMessage`）。
- 保留原有 `action` 名称集合，调用方无需迁移。
