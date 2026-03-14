## ADDED Requirements

### Requirement: User message metadata excludes channel identifier

`formatUserMessage` 生成的元信息前缀 SHALL NOT 包含 `via {channel}` 标签。元信息仅保留 `msg_id`、`type`、`reply_to_msg_id` 等与消息内容相关的字段。

#### Scenario: Normal text message formatting

- **WHEN** 一条来自 qiwei 通道的纯文本消息经过 `formatUserMessage` 格式化
- **THEN** 输出前缀为 `[msg_id=xxx]`，不包含 `via qiwei`

#### Scenario: Message with non-text type

- **WHEN** 一条 messageType 为 `image` 的消息经过 `formatUserMessage` 格式化
- **THEN** 输出前缀为 `[msg_id=xxx | type=image]`，不包含 `via qiwei`

#### Scenario: Message with quoted reply

- **WHEN** 一条带有 channelMeta.quotedMessage 的消息经过 `formatUserMessage` 格式化
- **THEN** 输出前缀中包含 `reply_to_msg_id=yyy`，不包含 `via qiwei`

### Requirement: send_channel_message tool schema hides channel parameter

`send_channel_message` 工具的 JSON Schema SHALL NOT 将 `channel` 作为可见参数暴露给 LLM。channel 值 SHALL 由执行器从当前请求上下文中自动获取。

#### Scenario: Tool schema inspection

- **WHEN** LLM 收到 `send_channel_message` 工具定义
- **THEN** properties 中不包含 `channel` 键

#### Scenario: Tool execution without channel param

- **WHEN** LLM 调用 `send_channel_message` 且未传入 `channel`
- **THEN** 执行器使用 `request.Channel` 作为默认值，消息正常送达

### Requirement: send_channel_message return value excludes channel

`send_channel_message` 工具的返回值 SHALL NOT 包含 `channel` 字段。

#### Scenario: Successful send response

- **WHEN** `send_channel_message` 执行成功
- **THEN** 返回的 JSON 中不包含 `channel` 键

### Requirement: Wecom builtin tools register unconditionally

`registerWecomBuiltinTools` SHALL NOT 检查 `request.Channel` 值。wecom 系列工具（wecom_search_targets、wecom_list_or_get_conversations、wecom_parse_message、inspect_attachment、wecom_send_message、wecom_revoke_message）SHALL 在所有请求中无条件注册。

#### Scenario: Non-qiwei channel request

- **WHEN** 一条 channel 为 `webui` 的请求进入 runner
- **THEN** wecom 系列工具仍然在 ToolRegistry 中注册可用

### Requirement: System prompt excludes channel references

`moli-system-prompt.md` 中的消息格式说明和工具参数说明 SHALL NOT 提及 channel / 渠道概念。

#### Scenario: Prompt message format description

- **WHEN** LLM 读取 system prompt 中的消息格式协议
- **THEN** 格式说明为 `[msg_id=消息ID]`，不包含 `via 渠道`

#### Scenario: Prompt tool parameter description

- **WHEN** LLM 读取 system prompt 中 send_channel_message 参数说明
- **THEN** 不包含 `channel` 参数的描述
