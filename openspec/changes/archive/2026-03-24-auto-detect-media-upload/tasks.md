## 1. 文件路径检测与类型推断

- [x] 1.1 在 `content_upload.go` 中新增 `looksLikeFilePath(content string) bool` 函数：不含 `://`，不含换行符和多空格，以 `/` 开头或包含 `/`
- [x] 1.2 在 `content_upload.go` 中新增 `inferMessageTypeFromFile(filePath string) string` 函数：对存在的文件根据扩展名返回 `image` / `file`，不存在则返回空字符串
- [x] 1.3 定义图片扩展名映射表（`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.bmp`）

## 2. send_channel_message handler 集成

- [x] 2.1 在 `processor.go` 的 `send_channel_message` handler 中，调用 `resolveMediaContent` 之前，插入自动检测逻辑：当 `messageType` 为空或 `text` 时，调用 `looksLikeFilePath` + `inferMessageTypeFromFile`，修正 `messageType`
- [x] 2.2 确保 LLM 已显式指定 `messageType`（image/file/voice）时跳过自动检测，保持向后兼容

## 3. 测试

- [x] 3.1 为 `looksLikeFilePath` 编写单元测试：覆盖绝对路径、相对路径、普通文本、含空格多行文本、HTTP URL 等场景
- [x] 3.2 为 `inferMessageTypeFromFile` 编写单元测试：覆盖图片扩展名、非图片扩展名、文件不存在、目录路径等场景
- [x] 3.3 为 `send_channel_message` handler 中的自动检测集成编写测试：验证 messageType 修正 + resolveMediaContent 调用链路
