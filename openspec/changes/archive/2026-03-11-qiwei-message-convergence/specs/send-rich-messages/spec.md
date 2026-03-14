## ADDED Requirements

### Requirement: Adapter SHALL support sending link card messages

messageType="link" 的 outgoingMessage SHALL 被映射到 QiWei `/msg/sendLink` API。

#### Scenario: 发送链接卡片消息
- **WHEN** agent 发送 messageType="link" 的消息，channelMeta 中包含 title/desc/linkUrl/iconUrl
- **THEN** adapter 调用 `/msg/sendLink`，参数为 `{toId, title, desc, linkUrl, iconUrl}`

#### Scenario: 发送链接卡片缺少必需字段
- **WHEN** messageType="link" 但 channelMeta 中缺少 title 或 linkUrl
- **THEN** adapter 返回错误 "link message requires title and linkUrl in channelMeta"

### Requirement: Adapter SHALL support sending location messages

messageType="location" 的 outgoingMessage SHALL 被映射到 QiWei `/msg/sendLocation` API。

#### Scenario: 发送定位消息
- **WHEN** agent 发送 messageType="location" 的消息，channelMeta 中包含 title/address/latitude/longitude
- **THEN** adapter 调用 `/msg/sendLocation`，参数为 `{toId, title, address, latitude, longitude}`

#### Scenario: 发送定位缺少经纬度
- **WHEN** messageType="location" 但 channelMeta 中缺少 latitude 或 longitude
- **THEN** adapter 返回错误 "location message requires latitude and longitude in channelMeta"

### Requirement: Adapter SHALL support sending miniapp messages

messageType="miniapp" 的 outgoingMessage SHALL 被映射到 QiWei `/msg/sendWeapp` API。所需参数 SHALL 从 channelMeta 中透传。

#### Scenario: 发送小程序消息（参数来自接收回调）
- **WHEN** agent 发送 messageType="miniapp" 的消息，channelMeta 中包含从接收时保存的完整小程序参数（appId, title, desc, pagePath, username, thumbUrl, coverFileAesKey, coverFileId, coverFileSize）
- **THEN** adapter 调用 `/msg/sendWeapp`，参数从 channelMeta 透传

#### Scenario: 小程序参数不完整
- **WHEN** messageType="miniapp" 但 channelMeta 中缺少 appId 或 username
- **THEN** adapter 返回错误 "miniapp message requires appId and username in channelMeta"

### Requirement: Adapter SHALL support message reply (引用回复)

发送 text 或 rich_text 消息时，如果 channelMeta 中包含 reply 字段，adapter SHALL 将其透传到 QiWei sendText/sendHyperText API 的 params 中。

#### Scenario: 发送带引用回复的文本消息
- **WHEN** agent 发送 messageType="text" 的消息，channelMeta 中包含 reply 对象（含 type/msgServerId/userId/timeStamp/msgUniqueIdentifier/msgData）
- **THEN** adapter 调用 `/msg/sendText`，params 中包含 content、toId 和完整的 reply 对象

#### Scenario: 非文本消息尝试使用引用回复
- **WHEN** agent 发送 messageType="image" 的消息，channelMeta 中包含 reply 字段
- **THEN** adapter 忽略 reply 字段（QiWei API 仅支持 text/rich_text 引用回复），正常发送图片

### Requirement: Adapter SHALL support message revocation (撤回消息)

adapter SHALL 提供撤回消息的能力，通过 module action 路径或专用 API 调用 QiWei `/msg/revokeMsg`。

#### Scenario: 撤回消息
- **WHEN** agent 调用撤回功能，提供 chatId 和 msgServerId
- **THEN** adapter 调用 `/msg/revokeMsg`，参数为 `{chatId, msgServerId}`

#### Scenario: 撤回不存在的消息
- **WHEN** agent 提供的 msgServerId 不存在或已过期
- **THEN** adapter 将 QiWei API 的错误信息原样返回给 agent

### Requirement: Agent-side wecom_send_message tool SHALL cover new message types

agent 侧的 `wecom_send_message` 工具描述 SHALL 更新，明确列出所有支持的 messageType 枚举值。

#### Scenario: 工具描述反映所有可发送类型
- **WHEN** agent 加载 wecom_send_message 工具定义
- **THEN** 工具的 messageType 描述中包含：text, rich_text, image, file, voice, link, location, miniapp

### Requirement: Agent SHALL have a dedicated wecom_revoke_message tool

agent 侧 SHALL 新增 `wecom_revoke_message` 工具，语义明确地用于撤回已发送的消息。

#### Scenario: 使用撤回工具
- **WHEN** agent 调用 wecom_revoke_message，提供 chatId 和 msgServerId
- **THEN** 工具通过 adapter 调用 QiWei `/msg/revokeMsg`，返回成功或失败信息

#### Scenario: 撤回工具缺少参数
- **WHEN** agent 调用 wecom_revoke_message 但未提供 chatId 或 msgServerId
- **THEN** 工具返回参数校验错误

### Requirement: Main reply path SHALL support new send types

`toQiweiMessageRequest`（主回复路径）SHALL 扩展支持 link/location 类型，使 agent 通过 `send_channel_message` 也能发送这些类型。

#### Scenario: agent 通过 send_channel_message 发送链接
- **WHEN** agent 调用 send_channel_message，messageType="link"，channelMeta 中包含 link 所需字段
- **THEN** adapter 的 toQiweiMessageRequest 能正确映射到 `/msg/sendLink` 调用
