# 飞书独立服务 Go 迁移

- **日期**：2026-03-02
- **类型**：架构决策 / 代码重构
- **状态**：已实施

## 背景

`channel-feishu` 原实现为 TypeScript/Bun 服务。  
本次目标是保留飞书服务的完整功能，并将该服务代码完全迁移为 Go，同时保持其“独立服务”形态，不并入主 `agent` 进程。

## 决策

将 `channel-feishu` 全量替换为 Go 服务实现，删除服务内全部 TypeScript 代码与 Bun 配置。

## 变更内容

- 新增 Go 服务实现（`channel-feishu/*.go`）：
  - `main.go`：服务启动、优雅关闭
  - `config.go`：环境变量加载和配置校验
  - `server.go`：HTTP 路由与统一发送逻辑
  - `api_handlers.go`：Action/Message/Meeting/Document/Wiki/Drive 全量路由处理
  - `events.go`：Webhook 事件处理、WebSocket 长连接、消息转发 Agent
  - `feishu_client.go`：飞书 token 管理、JSON API 调用、multipart 上传
  - `models.go`：请求/响应模型
- 新增 `channel-feishu/go.mod`，使用 `github.com/larksuite/oapi-sdk-go/v3` 支持事件分发与长连接。
- 删除 `channel-feishu/src/**/*.ts`、`channel-feishu/package.json`、`channel-feishu/tsconfig.json`、`channel-feishu/bun.lock`。
- 更新 `channel-feishu/README.md` 为 Go 版本说明。

## 考虑过的替代方案

- 方案 A：保留 TS 服务并仅修复类型问题。  
  否决原因：不满足“最终没有 TS 代码”的目标。
- 方案 B：合并进 `agent` 单体。  
  否决原因：与“feishu 是单独服务”的明确约束冲突。

## 影响

- `channel-feishu` 目录不再依赖 Bun/TypeScript，部署路径统一为 Go。
- API 路径与 action 名称保持兼容，调用方无需改协议。
- 服务内实现语言统一为 Go，后续维护成本集中到同一技术栈。
