## ADDED Requirements

### Requirement: wecom_send_message 从 session 注入会话 ID
`wecom_send_message` 的 JSON Schema SHALL 不包含 `channelConversationId` 和 `channelUserId` 参数。executor SHALL 从 `ProcessRequest` 的 `ChannelConversationID` 和 `ChannelUserID` 注入这两个值到发送 payload 中。

#### Scenario: 群聊中发送消息
- **WHEN** agent 在群聊 session 中调用 `wecom_send_message`，input 中不含 `channelConversationId`
- **THEN** executor 自动将 `ProcessRequest.ChannelConversationID`（群 ID）写入 payload 的 `channelConversationId`，消息发送到当前群

#### Scenario: 私聊中发送消息
- **WHEN** agent 在私聊 session 中调用 `wecom_send_message`，input 中不含 `channelUserId`
- **THEN** executor 自动将 `ProcessRequest.ChannelUserID` 写入 payload 的 `channelUserId`，消息发送给当前私聊对象

#### Scenario: Schema 不暴露会话参数
- **WHEN** LLM 获取 `wecom_send_message` 的工具定义
- **THEN** schema 的 properties 中不存在 `channelConversationId` 和 `channelUserId`

---

### Requirement: wecom_revoke_message 从 session 注入 chatId
`wecom_revoke_message` 的 JSON Schema SHALL 不包含 `chatId` 参数。executor SHALL 自动注入：优先取 `ProcessRequest.ChannelConversationID`，为空则取 `ProcessRequest.ChannelUserID`。

#### Scenario: 撤回当前会话消息
- **WHEN** agent 调用 `wecom_revoke_message`，只提供 `msgServerId`
- **THEN** executor 自动填充 `chatId` 为当前 session 的会话 ID，撤回请求发送成功

#### Scenario: Schema 仅保留 msgServerId
- **WHEN** LLM 获取 `wecom_revoke_message` 的工具定义
- **THEN** schema 的 required 数组仅包含 `msgServerId`，properties 中不存在 `chatId`

---

### Requirement: wecom_get_group_detail 从 session 注入 roomId
`wecom_get_group_detail` 的 JSON Schema SHALL 不包含 `roomIds` 参数。executor SHALL 从 `ProcessRequest.ChannelConversationID` 构建 `roomIds` 数组。

#### Scenario: 群聊中查询当前群详情
- **WHEN** agent 在群聊 session 中调用 `wecom_get_group_detail`，不提供任何参数
- **THEN** executor 自动将 `ProcessRequest.ChannelConversationID` 作为 `roomIds[0]`，返回当前群详情

#### Scenario: 私聊 session 中调用返回错误
- **WHEN** agent 在私聊 session 中调用 `wecom_get_group_detail`
- **THEN** executor SHALL 返回错误信息表明当前不在群聊中，不向下游发送请求

---

### Requirement: wecom_list_or_get_conversations 默认读取当前会话历史
`wecom_list_or_get_conversations` 的 JSON Schema SHALL 不包含 `conversationId` 参数。默认行为 SHALL 为读取当前会话的历史消息。新增 `listRecent` 布尔参数用于切换到列出最近会话模式。

#### Scenario: 默认读取当前会话历史
- **WHEN** agent 调用 `wecom_list_or_get_conversations`，不提供 `listRecent` 参数
- **THEN** executor 使用 `ProcessRequest` 的会话 ID 作为 `conversationId`，返回当前会话的历史消息

#### Scenario: 翻页读取历史消息
- **WHEN** agent 调用 `wecom_list_or_get_conversations`，提供 `msgSvrId` 作为翻页起点
- **THEN** executor 使用 session 会话 ID + 提供的 `msgSvrId`，返回更早的历史消息

#### Scenario: 列出最近会话
- **WHEN** agent 调用 `wecom_list_or_get_conversations`，设置 `listRecent: true`
- **THEN** executor 忽略 session 会话 ID，返回最近会话列表

---

### Requirement: resolveSessionConversationID 工具函数
SHALL 提供 `resolveSessionConversationID(request ProcessRequest) string` 函数，返回当前 session 的会话 ID。逻辑为：优先返回 `ChannelConversationID`，为空则返回 `ChannelUserID`。

#### Scenario: 群聊 session
- **WHEN** `ProcessRequest.ChannelConversationID` 为群 ID（非空）
- **THEN** 返回 `ChannelConversationID`

#### Scenario: 私聊 session
- **WHEN** `ProcessRequest.ChannelConversationID` 为空
- **THEN** 返回 `ChannelUserID`

---

### Requirement: 不影响无会话参数的工具
`wecom_search_targets`、`wecom_parse_message`、`inspect_attachment`、`wecom_get_contact_detail` 这些工具 SHALL 保持不变——它们的参数与会话无关，不需要注入。

#### Scenario: wecom_search_targets 无变化
- **WHEN** agent 调用 `wecom_search_targets`
- **THEN** 工具行为和 schema 与改造前完全一致
