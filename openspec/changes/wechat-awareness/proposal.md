## Why

Agent 当前对微信的感知是"半盲"的：不知道自己在私聊还是群聊、不知道是否被 @、不知道谁入群/退群、无法查看群详情和联系人信息、无法处理好友申请。这使得 agent 在群聊场景下无法做出有上下文的响应，与"像人一样使用微信"的目标差距明显。

平台 API 已经提供了这些能力（@mention 列表、群事件成员身份、群/联系人详情查询、好友申请回调），但 channel-qiwei → agent 的数据通路没有打通。

## What Changes

**消息入站增强（channel → agent）**
- 透传 `conversationType`（p2p / group）到 agent IncomingMessage
- 提取并传递 `atList`（@提及列表）和 `mentionedSelf`（是否 @ 自己）到 channelMeta
- 群生命周期事件携带受影响的成员 ID 列表和操作者 ID

**硬编码自动化（channel-qiwei 自行处理）**
- 好友申请（msgType=2357）自动通过，并向 agent 推送 `new_contact` 事件
- 联系人信息变动（msgType=2131/2188）、群主转让（1022）、管理员变动（1043）仅刷新本地缓存

**新增 agent 查询工具（拉取模式）**
- `wecom_get_group_detail`：查看群详情（群名、公告、成员列表+角色、群主）
- `wecom_get_contact_detail`：查看联系人详情（昵称、真实姓名、企业、头像）

## Capabilities

### New Capabilities

- `inbound-message-enrichment`: 消息入站增强 — conversationType / atList / mentionedSelf 透传
- `group-event-enrichment`: 群事件增强 — 成员身份、操作者 ID、新事件类型
- `auto-accept-friend`: 好友申请自动通过 + new_contact 事件推送
- `query-tools`: 新增 agent 查询工具 — wecom_get_group_detail / wecom_get_contact_detail

### Modified Capabilities

_(无现有 spec 需要修改)_

## Impact

- **channel-qiwei 服务**：`events.go`（回调解析增强）、`facade_handlers.go`（新 facade 端点）、`models.go`（数据模型扩展）
- **agent 服务**：`dispatcher/dispatcher.go`（接收 conversationType）、`dispatcher/group_event.go`（接收 payload）、`runner/wecom_builtin_tools.go`（注册新工具）
- **API 契约**：incoming message JSON 增加字段（向后兼容）、group event JSON 增加 payload（向后兼容）
- **外部依赖**：QiWe 平台 API `/room/batchGetRoomDetail`、`/contact/batchGetUserinfo`、`/contact/agreeContact`
