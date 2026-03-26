## ADDED Requirements

### Requirement: Map msgType 20 to file
系统 SHALL 在 `userMessageTypeMap` 中将 msgType=20 映射为 "file"，并在 `mediaClassifications` 中将其分类为企微文件（source=qw, kind=file）。

#### Scenario: Receive large file message (msgType 20)
- **WHEN** 收到 cmd=15000, msgType=20 的回调消息
- **THEN** 系统将其识别为 "file" 类型，走媒体下载管线进行文件物化，最终将 resourceURI 转发给 agent

#### Scenario: msgType 20 with standard QW file fields
- **WHEN** 收到 msgType=20 的消息，msgData 包含 fileId, fileAeskey, fileMd5, fileSize 字段
- **THEN** 系统能通过现有 QW 媒体下载策略正常下载和上传文件

### Requirement: Map msgType 22 to video
系统 SHALL 在 `userMessageTypeMap` 中将 msgType=22 映射为 "video"，并在 `mediaClassifications` 中将其分类为企微视频（source=qw, kind=video）。

#### Scenario: Receive large video message (msgType 22)
- **WHEN** 收到 cmd=15000, msgType=22 的回调消息
- **THEN** 系统将其识别为 "video" 类型，走媒体下载管线进行视频物化，最终将 resourceURI 转发给 agent

### Requirement: Skip known non-actionable msgTypes gracefully
系统 SHALL 对文档中列出但 agent 无需处理的 msgType（146 直播、2001 已读通知、2005 未读通知）记录 Detail 级别日志而非 Warn 级别。

#### Scenario: Receive read receipt notification (msgType 2001)
- **WHEN** 收到 cmd=15000, msgType=2001 的回调消息
- **THEN** 系统记录 Detail 日志 "忽略已读通知"，不转发给 agent，不产生 Warn 日志
