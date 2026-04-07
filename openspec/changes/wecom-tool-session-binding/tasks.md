## 1. 基础设施

- [ ] 1.1 在 `wecom_builtin_tools.go` 中新增 `resolveSessionConversationID(request ProcessRequest) string` 函数，优先返回 `ChannelConversationID`，为空则返回 `ChannelUserID`

## 2. wecom_send_message 改造

- [ ] 2.1 修改 `createWecomSendMessageExecutor()` 签名为 `createWecomSendMessageExecutor(request ProcessRequest)`，通过闭包捕获 session 上下文
- [ ] 2.2 executor 内部从 `request.ChannelConversationID` / `request.ChannelUserID` 获取会话 ID，替换原来从 `input` 读取的逻辑
- [ ] 2.3 从 schema properties 中移除 `channelConversationId` 和 `channelUserId`
- [ ] 2.4 更新 `registerWecomBuiltinTools` 中的调用点，传入 `request`

## 3. wecom_revoke_message 改造

- [ ] 3.1 将 `createWecomModuleActionExecutor("message", "revoke")` 替换为专用的 session-aware executor，通过闭包捕获 `ProcessRequest`
- [ ] 3.2 executor 使用 `resolveSessionConversationID(request)` 自动注入 `chatId`
- [ ] 3.3 从 schema 中移除 `chatId`，required 改为仅 `["msgServerId"]`

## 4. wecom_get_group_detail 改造

- [ ] 4.1 替换 executor 为 session-aware 版本，从 `request.ChannelConversationID` 构建 `roomIds` 数组
- [ ] 4.2 增加私聊 session 检测：当 `ChannelConversationID` 为空或等于 `ChannelUserID` 时，返回"当前不在群聊中"错误
- [ ] 4.3 从 schema 中移除 `roomIds` 参数和 required 声明

## 5. wecom_list_or_get_conversations 改造

- [ ] 5.1 替换 executor 为 session-aware 版本，默认使用 `resolveSessionConversationID(request)` 作为 `conversationId`
- [ ] 5.2 新增 `listRecent` 布尔参数到 schema，当 `listRecent=true` 时忽略注入的 conversationId，走列会话模式
- [ ] 5.3 从 schema 中移除 `conversationId` 参数
- [ ] 5.4 更新工具 description，说明默认行为是读取当前会话历史

## 6. 验证

- [ ] 6.1 确认 `wecom_search_targets`、`wecom_parse_message`、`inspect_attachment`、`wecom_get_contact_detail` 无变化
- [ ] 6.2 编译通过，无 lint 错误
