## ADDED Requirements

### Requirement: Auto-detect local file path and infer messageType
当 `send_channel_message` 的 `messageType` 为空或 `text` 时，系统 SHALL 检测 `content` 是否为一个存在的本地文件路径。若文件存在，系统 SHALL 根据扩展名自动推断 `messageType` 并触发 OSS 上传流程。

#### Scenario: content 是本地图片路径且 messageType 未指定
- **WHEN** `messageType` 为空或 `text`，且 `content` 值为一个存在的本地文件路径，且扩展名为 `.png`、`.jpg`、`.jpeg`、`.gif`、`.webp`、`.bmp`
- **THEN** 系统自动将 `messageType` 修正为 `image`，并执行 OSS 上传，content 被替换为 presigned URL

#### Scenario: content 是本地非图片文件路径且 messageType 未指定
- **WHEN** `messageType` 为空或 `text`，且 `content` 值为一个存在的本地文件路径，且扩展名不属于图片类型
- **THEN** 系统自动将 `messageType` 修正为 `file`，并执行 OSS 上传，content 被替换为 presigned URL

#### Scenario: content 是本地路径但文件不存在
- **WHEN** `messageType` 为空或 `text`，且 `content` 看起来像文件路径，但 `os.Stat` 返回 not found
- **THEN** 系统跳过自动检测，按原有逻辑将 content 作为普通文本处理

#### Scenario: content 是普通文本消息
- **WHEN** `messageType` 为空或 `text`，且 `content` 不像文件路径（如不含路径分隔符、不以 `/` 或 `.` 开头）
- **THEN** 系统不执行 `os.Stat`，直接按原有逻辑处理

### Requirement: Backward compatibility with explicit messageType
当 LLM 已显式指定正确的 `messageType`（如 `image`、`file`、`voice`）时，系统 SHALL 保持现有行为不变。

#### Scenario: messageType 已显式指定为 image
- **WHEN** `messageType` 为 `image`，且 `content` 为本地文件路径
- **THEN** 系统直接进入现有的 `resolveMediaContent` 流程，不执行自动检测逻辑

#### Scenario: messageType 已显式指定为 file
- **WHEN** `messageType` 为 `file`，且 `content` 为本地文件路径
- **THEN** 系统直接进入现有的 `resolveMediaContent` 流程，不执行自动检测逻辑

### Requirement: Path-like heuristic to avoid unnecessary stat calls
系统 SHALL 仅对看起来像文件路径的 content 值执行 `os.Stat`，以避免对每条普通文本消息产生不必要的系统调用。

#### Scenario: content 以斜杠开头（绝对路径）
- **WHEN** content 以 `/` 开头且不包含 `://`
- **THEN** 系统判定为潜在文件路径，执行 `os.Stat` 检测

#### Scenario: content 包含路径分隔符（相对路径）
- **WHEN** content 包含 `/` 且不包含 `://` 且不含空格或换行
- **THEN** 系统判定为潜在文件路径，执行 `os.Stat` 检测

#### Scenario: content 是多行文本或含空格的自然语言
- **WHEN** content 包含换行符或多个空格
- **THEN** 系统判定为非文件路径，跳过检测

### Requirement: OSS unavailable error remains consistent
当自动检测到本地文件但 OSS 未配置时，系统 SHALL 返回与现有行为一致的错误信息。

#### Scenario: 检测到本地图片但 OSS 未配置
- **WHEN** 自动检测到 content 为本地图片文件，但 `cardrender.OSSStorage()` 返回 nil 且 `cardrender.Init()` 失败
- **THEN** 返回错误 "文件发送需要 OSS 存储配置，当前不可用"
