## ADDED Requirements

### Requirement: Use fileNameExt field for filename inference
系统 SHALL 在 `normalizeMediaDescriptor` 处理 msgType=15（企微文件消息）时，读取 msgData 中的 `fileNameExt` 字段。当 fileName 解码后缺少文件扩展名时，SHALL 使用 `fileNameExt` 的值推断并拼接扩展名。

#### Scenario: fileNameExt provides extension when fileName lacks one
- **WHEN** msgType=15 的消息中 fileName 解码后为 "report"（无扩展名），fileNameExt 为 "excel"
- **THEN** 系统将文件名推断为 "report.xlsx"

#### Scenario: fileNameExt maps to known extensions
- **WHEN** fileNameExt 值为下列之一时
- **THEN** 系统 SHALL 按如下映射添加扩展名：
  - "excel" → ".xlsx"
  - "word" → ".docx"
  - "ppt" → ".pptx"
  - "pdf" → ".pdf"
  - "txt" → ".txt"
  - "csv" → ".csv"

#### Scenario: fileName already has valid extension
- **WHEN** msgType=15 的消息中 fileName 解码后为 "report.docx"（已有扩展名）
- **THEN** 系统 SHALL 保留原有扩展名，忽略 fileNameExt 字段

#### Scenario: fileNameExt is empty or unrecognized
- **WHEN** fileNameExt 为空或不在已知映射中（如 "unknown"）
- **THEN** 系统 SHALL 不修改文件名，回退到现有的推断逻辑
