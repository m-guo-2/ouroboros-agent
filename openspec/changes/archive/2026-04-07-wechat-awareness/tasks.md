## 0. Channel 侧：修复名称解析（群成员昵称 + base64 解码）

- [x] 0.1 在 `channel-qiwei/events.go` 的 `fetchUserName`、`loadExternalContacts`、`loadInternalContacts` 中，对 `nickname`/`realName`/`alias`/`remark`/`name` 字段统一加 `decodeMaybeBase64()` 解码
- [x] 0.2 在回调消息解析处（`SenderNickname` 赋值），对 `senderNickname`/`senderName` 也加 `decodeMaybeBase64()` 解码
- [x] 0.3 为 `resolveUserName` 增加群成员名称 fallback：当联系人 API 查不到名字时，如果有 `fromRoomId` 上下文，尝试从 `/room/batchGetRoomDetail` 的 `memberList` 中查找该 userId 的 `name` 字段并缓存
- [x] 0.4 在 `resolveGroupName` 返回空时，确保 `conversationName` 不会降级为 senderName（群聊场景应保留 roomId 作为最差 fallback 而非个人名）

## 1. Agent 侧：IncomingMessage 增加 conversationType

- [x] 1.1 在 `agent/internal/dispatcher/dispatcher.go` 的 `IncomingMessage` 结构体中增加 `ConversationType string` 字段（JSON tag `conversationType`）

## 2. Channel 侧：文本消息提取 atList 和 mentionedSelf

- [x] 2.1 在 `channel-qiwei/events.go` 的文本消息路径中，从 `msg.MsgData["atList"]` 提取 @列表，写入 `channelMeta["atList"]`
- [x] 2.2 获取自身 userId（优先从 app 结构体缓存，fallback 从回调顶层 `userId` 字段），比对 atList 中是否包含自身 userId，写入 `channelMeta["mentionedSelf"]`
- [x] 2.3 在 `channel-qiwei` 启动流程中增加调用 `/user/getProfile` 获取并缓存自身 userId 的逻辑

## 3. Channel 侧：群事件携带成员身份

- [x] 3.1 在 `channel-qiwei/events.go` 的 `handleGroupEvent` 中，从 `msg.MsgData["changedMemberList"]` base64 解码并按分号分割，提取 userId 列表
- [x] 3.2 修改 `reportGroupEvent` 函数签名，增加 `payload map[string]any` 参数，将 `memberIds` 和 `operatorId`（来自 `msg.SenderID`）写入 payload
- [x] 3.3 确保 base64 解码失败或空值时 `memberIds` 为空数组，不影响事件投递

## 4. Channel 侧：好友申请自动通过 + 推送 new_contact

- [x] 4.1 在 `channel-qiwei/events.go` 的 `handleSystemEvent` case 2357 中，提取 `contactId`、`contactNickname`、`contactType`，构造参数调用平台 `/contact/agreeContact`
- [x] 4.2 自动通过成功后，复用 `reportGroupEvent` 模式 POST `/api/channels/group-event`，eventType 为 `"new_contact"`，payload 含 contactId/contactNickname/contactType
- [x] 4.3 在 `agent/internal/dispatcher/group_event.go` 的 `validGroupEventTypes` 中增加 `"new_contact": true`

## 5. Channel 侧：新增 facade 端点

- [x] 5.1 在 `channel-qiwei/facade_handlers.go` 中新增 `handleGetGroupDetail` handler：接收 `roomIds`，调平台 `/room/batchGetRoomDetail`，精简返回值（roomId/roomName/announcement/createUserId/memberCount/members）
- [x] 5.2 在 `channel-qiwei/facade_handlers.go` 中新增 `handleGetContactDetail` handler：接收 `userIds`，调平台 `/contact/batchGetUserinfo`，精简返回值（userId/nickname/realName/alias/corpId/gender/avatarUrl）
- [x] 5.3 在 `channel-qiwei/server.go` 的 `routes()` 中注册 `/api/qiwei/get_group_detail` 和 `/api/qiwei/get_contact_detail`

## 6. Agent 侧：注册新查询工具

- [x] 6.1 在 `agent/internal/runner/wecom_builtin_tools.go` 中注册 `wecom_get_group_detail` 工具，使用 `createWecomHTTPToolExecutor("get_group_detail")`，schema 参数为 `roomIds: []string`
- [x] 6.2 在 `agent/internal/runner/wecom_builtin_tools.go` 中注册 `wecom_get_contact_detail` 工具，使用 `createWecomHTTPToolExecutor("get_contact_detail")`，schema 参数为 `userIds: []string`

## 7. 文档更新

- [x] 7.1 更新 `docs/qiwei-platform.md` 的回调处理章节，反映 atList 提取、群事件成员身份、好友申请自动通过的变更
- [x] 7.2 更新 `docs/qiwei-platform.md` 的服务路由表和模块代理映射，增加新的 facade 端点
