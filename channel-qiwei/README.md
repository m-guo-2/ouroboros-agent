# channel-qiwei (Go)

QiWe 渠道服务（Go 版），负责：

1. 接收 QiWe 回调消息（`/webhook/callback`）
2. 转发标准化消息到 Agent（`/api/channels/incoming`）
3. 接收 Agent 回调并向企微发送消息（`/api/qiwei/send`）
4. 提供 QiWe 全模块 API 代理（`/api/qiwei/{module}/{action}`、`/api/qiwei/do`）

## 运行

在仓库根目录：

```bash
make qiwei
```

或在当前目录：

```bash
go run .
```

## 环境变量

参考 `.env.example`：

- `QIWEI_API_BASE_URL`：QiWe API 地址
- `QIWEI_TOKEN`：QiWe Token
- `QIWEI_GUID`：实例 GUID
- `QIWEI_BOT_PORT`：服务端口（默认 `2000`）
- `QIWEI_HTTP_TIMEOUT_SECONDS`：HTTP 超时秒数（默认 `25`）
- `AGENT_ENABLED`：是否启用 Agent 转发（默认 `true`）
- `AGENT_SERVER_URL`：Agent 服务地址（默认 `http://localhost:1997`）
- `AGENT_ID`：当前渠道绑定的 Agent ID（可选）

## 核心接口

- `POST /webhook/callback`
  - 接收企微回调并异步处理，快速返回 `{ code: 200, msg: "ok" }`
- `POST /api/qiwei/send`
  - 接收主服务 `OutgoingMessage`，发送回企微
- `POST /api/qiwei/do`
  - 直接调用任意 QiWe method，格式 `{ method, params }`
- `POST /api/qiwei/{module}/{action}`
  - 模块化调用（instance/login/user/contact/group/message/cdn/moment/tag/session）

## 模块映射

模块 action 映射定义在：

- `internal/modules/instance.go`
- `internal/modules/login.go`
- `internal/modules/user.go`
- `internal/modules/contact.go`
- `internal/modules/group.go`
- `internal/modules/message.go`
- `internal/modules/cdn.go`
- `internal/modules/moment.go`
- `internal/modules/tag.go`
- `internal/modules/session.go`
