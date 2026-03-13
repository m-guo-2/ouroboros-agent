## 1. formatUserMessage 移除 via channel

- [x] 1.1 `processor.go`: 删除 `formatUserMessage` 中 `if channel != ""` 拼接 `via %s` 的逻辑块
- [x] 1.2 `processor.go`: 确认 `[msg_id=... | type=...]` 前缀在无 channel 时格式正确（无多余分隔符）

## 2. send_channel_message 工具精简

- [x] 2.1 `processor.go`: 从 `send_channel_message` 的 Properties map 中移除 `"channel"` 键
- [x] 2.2 `processor.go`: 从 `send_channel_message` 执行器返回值中移除 `"channel"` 字段
- [x] 2.3 `processor.go`: 删除执行器中 `if ch, ok := input["channel"].(string)` 的覆盖逻辑，直接使用 `request.Channel`

## 3. wecom builtin tools 移除门控

- [x] 3.1 `wecom_builtin_tools.go`: 删除 `if request.Channel != "qiwei" { return }` 判断

## 4. System prompt 更新

- [x] 4.1 `moli-system-prompt.md`: 消息格式说明从 `[via 渠道 | msg_id=消息ID]` 改为 `[msg_id=消息ID]`
- [x] 4.2 `moli-system-prompt.md`: send_channel_message 参数说明中移除 `channel /`

## 5. 验证

- [x] 5.1 `go build ./...` 编译 agent 通过
- [x] 5.2 确认 `formatUserMessage` 输出不包含 `via` 前缀
