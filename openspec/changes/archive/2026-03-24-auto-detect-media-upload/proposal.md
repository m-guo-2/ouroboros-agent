## Why

`send_channel_message` 工具当前依赖 LLM 传入正确的 `messageType`（如 `image`/`file`）才会触发本地文件上传到 OSS。当 developer subagent 通过 shell 生成了一张图片并在结果中返回本地路径时，主 agent 通常不知道需要用 `messageType: "image"` 发送，导致图片路径被当作纯文本传出，无法展示给用户。

这是一个代码层面的职责缺失：文件类型检测和上传是内部机制，不应依赖 LLM 的 prompt 引导来保证正确性。

## What Changes

- `send_channel_message` 处理函数中，当 `content` 是一个存在的本地文件路径时，自动检测文件类型（图片、通用文件等），根据扩展名推断 `messageType`，并触发 OSS 上传 + presigned URL 生成
- 如果 LLM 已经传了正确的 `messageType`，保持原有行为不变（向后兼容）
- `resolveMediaContent` 入口逻辑调整：不再仅在 `isMediaMessageType` 时才处理，对未指定/text 类型也检测是否为本地文件

## Capabilities

### New Capabilities
- `auto-detect-media-upload`: 在 `send_channel_message` 内部自动检测本地文件路径并推断媒体类型，无需 LLM 显式指定 `messageType` 即可完成 OSS 上传和发送

### Modified Capabilities

## Impact

- `agent/internal/runner/content_upload.go` — `resolveMediaContent` 入口逻辑调整
- `agent/internal/runner/processor.go` — `send_channel_message` handler 调整 messageType 自动修正
- 下游 channel 层无需修改（仍接收 HTTP URL + 正确的 messageType）
- 需确认 OSS 环境变量已配置，否则自动检测到本地文件后仍会报错（与现有行为一致）
