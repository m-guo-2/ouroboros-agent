# QiWe Media Pipeline Tasks

## 1. 媒体语义与模型收口

- [x] 1.1 新增统一的媒体消息分类表，覆盖企微/个微的图片、文件、语音、视频 `msgType`
- [x] 1.2 设计并实现统一的 `AttachmentDescriptor` 与相关枚举，收口来源、类型、字段变体和诊断元数据
- [x] 1.3 将原始 `msgData` 的字段兼容逻辑迁移到归一化层，统一处理 `fileAesKey/fileAeskey`、`fileAuthKey/fileAuthkey`、`fileBigHttpUrl/fileMiddleHttpUrl/fileHttpUrl` 等变体

## 2. 下载规划与执行重构

- [x] 2.1 实现 `DownloadPlanner`，基于 `source + kind + available fields` 选择唯一下载策略
- [x] 2.2 拆分企微与个微下载执行器，分别封装 `/cloud/wxWorkDownload`、`/cloud/cdnWxDownload`、`/cloud/wxDownload` 的参数校验与调用
- [x] 2.3 收敛并替换当前猜测式 fallback 逻辑，让契约不完整时返回结构化规划错误而不是继续盲试
- [x] 2.4 统一附件 OSS 物化与 MIME 推断逻辑，确保图片、文件、视频先进入稳定的对象存储阶段
- [x] 2.5 为语音实现专用资源准备与转写链路，确保最终输出是文本而不是语音附件状态

## 3. 接入 callback 与 parse_message

- [x] 3.1 让 `parse_message` 切换到新媒体管线，支持 `resourceUri`（兼容旧 `localPath`）二次解析；文件/图片类可在拿到 OSS 资源地址后再按需理解内容，语音继续返回转写文本
- [x] 3.2 让 webhook callback 的预下载链路切换到新媒体管线，先转发 agent 可直接消费的“极简占位 + OSS 资源地址”或文本结果
- [x] 3.3 清理 `events.go` 与 `facade_handlers.go` 中重复或过时的媒体判断与下载代码，保留 façade 外部协议不变
- [x] 3.4 收紧 agent 侧 payload，确保不再暴露下载成功、失败、重试、内部诊断字段，不暴露企微/个微来源信息；仅保留最小必要的 OSS 资源地址，不默认暴露 MIME、大小、下载策略等细节

## 4. 可观测性、样本与回归验证

- [x] 4.1 为媒体管线补充结构化日志字段，覆盖分类结果、策略选择、契约校验、执行方法与失败阶段
- [x] 4.2 建立企微/个微图片、文件、语音、视频的真实样本或脱敏样本夹具
- [x] 4.3 为分类、归一化、策略选择和 resolver 执行补充单元测试
- [x] 4.4 为 `callback` 与 `parse_message` 增加端到端回归用例，验证两条入口对同一消息得到一致语义，并且 agent 看到的只是极简占位 + OSS 资源地址或文本
- [ ] 4.5 基于真实运行日志做一次回归验收，确认个微图片/文件不再误走企微下载契约，语音不再向 agent 暴露附件状态

## 5. agent 结构化附件协议

- [x] 5.1 扩展 `channel-qiwei -> agent` 入站消息 schema，支持在文本之外携带结构化 `attachments[]`
- [x] 5.2 为图片、文件、视频定义稳定的附件字段集合，至少覆盖 `attachmentId`、`kind`、`resourceUri`，并明确哪些字段允许暴露给 agent
- [x] 5.3 让 `callback` 链路转发结构化附件，而不是只把资源地址拼进 `content string`
- [x] 5.4 兼容历史与纯文本渠道：在未携带附件时保持现有消息处理与存储逻辑不变

## 6. 按需附件分析能力

- [x] 6.1 新增统一的按需附件分析 builtin/tool，主路径按 `attachmentId` 调用，而不是要求模型从正文里拼 `resourceUri`
- [x] 6.2 让新的附件分析能力复用现有 `parse_message` / recognizer 逻辑，避免重复实现 OCR、文档抽取与后续视频理解入口
- [x] 6.3 将 `wecom_parse_message` 调整为兼容层或桥接层，避免它继续作为 agent 主流程的隐式约定
- [x] 6.4 为附件分析失败定义稳定错误码和返回结构，避免 agent 编造“没有视觉能力/不会转写”之类不准确边界

## 7. runtime 触发约束与回归

- [x] 7.1 在 agent runtime 中增加 attachment-aware 约束：当回答显著依赖附件内容时，必须先调用附件分析能力
- [x] 7.2 让 runtime 能区分“需要附件内容”和“附件存在但当前问题与附件无关”的场景，避免无谓分析
- [x] 7.3 补充回归用例，覆盖“图片问题会触发分析”“普通闲聊不会触发分析”“语音仍然只以文本进入 agent”三类行为
