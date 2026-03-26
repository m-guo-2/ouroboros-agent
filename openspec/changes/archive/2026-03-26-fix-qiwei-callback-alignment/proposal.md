## Why

`channel-qiwei` 的消息回调处理逻辑与官方"消息回调内容说明"文档存在多处不一致，导致：

1. **msgType 20（大文件 / 一般文件）和 msgType 22（大视频 / 一般视频）完全丢失** — 代码中没有映射，这类消息被静默跳过。
2. **群生命周期事件（群名变更 1001、成员被移除 1003、成员退群 1005、群解散 1023）被丢弃** — 文档显示这些事件以 `cmd=15000` 到达，但 `handleNormalMessage` 中无对应处理，全部走"跳过不支持的消息类型"。
3. **新增群成员 1002 路由可能错误** — 代码仅在 `handleSystemEvent`（cmd=15500）中处理，但文档示例显示 cmd=15000。
4. **企微文件消息 `fileNameExt` 字段未使用** — 导致 `.docx` 等文件在 OSS 中被存为 `.dat`（已部分修复，但 `fileNameExt` 仍未纳入）。

这些问题影响 agent 对群环境的感知能力和富媒体文件的正确接收。

## What Changes

- 在 `userMessageTypeMap` 中补充 `msgType 20 → "file"` 和 `msgType 22 → "video"`
- 在 `handleNormalMessage` 中增加对群生命周期事件（1001, 1003, 1005, 1023）的识别和转发，使 agent 能感知群环境变化
- 将 `1002`（新增群成员）的处理同时兼容 `cmd=15000` 和 `cmd=15500` 两种路径
- 在 `normalizeMediaDescriptor` 中读取并利用 `fileNameExt` 字段辅助推断文件名
- 在 `mediaClassifications` 中补充 `msgType 20` 和 `msgType 22` 的分类

## Capabilities

### New Capabilities
- `group-lifecycle-events`: 处理群名变更、成员移除、成员退群、群解散等群生命周期事件，转发给 agent

### Modified Capabilities
- `callback-message-routing`: 修正 msgType 映射表和消息路由逻辑，补全缺失的文件/视频类型，兼容群事件的双 cmd 路径
- `media-filename-resolution`: 利用 `fileNameExt` 字段增强企微文件消息的文件名推断

## Impact

- **代码**: `channel-qiwei/events.go`（userMessageTypeMap、handleNormalMessage、handleSystemEvent）、`channel-qiwei/media_pipeline.go`（mediaClassifications、normalizeMediaDescriptor）
- **Agent 行为**: agent 将开始收到群生命周期事件通知，需要下游 agent server 能处理 `messageType: "system"` 类型的群事件消息
- **兼容性**: 保留 `handleSystemEvent` 中 1002 的处理作为兼容措施，防止 bridge 实际行为与文档不一致的情况
