## Why

当前 `channel-qiwei` 媒体管线已经解决了企微/个微下载契约混乱的问题，也把语音前置转写收口到了入口层；但图片、文件、视频进入 agent 时仍然被压平成“占位文本 + 资源地址”字符串。这样会把“是否需要继续理解附件内容”这件事交给模型从正文里自行猜测，导致 agent 明明拿到了可用资源链接，仍然可能直接回复“我没有视觉能力”，而不是稳定地调用解析能力。

这次设计要继续沿用现有媒体管线的基础分层，但把 agent-facing 边界从“文本提示协议”升级为“结构化附件协议”：语音仍然前置转写；图片、文件、视频只前置做资源准备与附件建模，不做语义预解析；当用户意图确实依赖附件内容时，agent 再通过统一的按需附件分析能力显式调用理解。

## What Changes

- 保留现有统一媒体管线的前半段：消息分类、字段归一化、下载规划、下载执行、OSS 物化、语音转写。
- 明确新的 agent-facing 媒体边界：
  - `voice` 在 `channel-qiwei` 内前置转写，agent 只收到文本。
  - `image/file/video` 只前置准备资源和结构化附件，不前置做 OCR/视觉/文档语义理解。
- 将 `channel-qiwei -> agent` 的输入契约从单一 `content string` 扩展为“文本 + 附件列表”，附件至少包含稳定 `attachmentId`、`kind`、`resourceUri`、可选名称和可选 MIME 等最小字段。
- 新增统一的按需附件分析能力，替代当前让模型自己拼 `wecom_parse_message(resourceUri=...)` 的弱协议。该能力以 attachment 为一等对象执行 OCR、图片描述、文档抽取、视频摘要等任务。
- 调整 `wecom_parse_message` 的角色：从主路径解析入口降级为兼容层或内部复用入口，由新的统一附件分析能力承接 agent 主流程。
- 在 agent runtime 中补充 attachment-aware 约束：当一轮回答明显依赖附件内容时，必须先调用附件分析能力，而不是直接输出“看不到/不能识别”的兜底话术。
- 继续维持 `qw/gw` 来源、下载策略、错误分类和日志字段只存在于入口内部，不进入 agent-facing payload。
- 为新的结构化附件契约、按需分析工具和语音前置转写/非语音按需分析组合增加测试与回归样本。

## Capabilities

### New Capabilities
- `attachment-on-demand-inspection`: 为 agent 提供统一的按需附件理解能力，基于结构化附件而不是正文中的资源地址字符串执行 OCR、图片理解、文档抽取与后续可扩展的视频理解。

### Modified Capabilities
- `qiwei-media-pipeline`: 继续负责企微渠道的媒体分类、下载、OSS 物化、语音转文本和诊断，但 agent-facing 输出从“占位 + 资源地址文本”升级为“文本 + 结构化附件”。

## Impact

- 主要影响代码：`channel-qiwei/events.go`、`channel-qiwei/models.go`、`agent/internal/dispatcher`、`agent/internal/runner/processor.go`、`agent/internal/runner/wecom_builtin_tools.go`，以及新增的附件分析 builtin/tool 相关模块。
- 影响行为：语音继续前置转写；图片、文件、视频仍先上传到 OSS，但转发给 agent 时不再仅依赖正文里的资源地址提示，而是通过结构化附件暴露给 runtime。
- 影响接口边界：`channel-qiwei` 与 agent 的入站协议需要扩展附件结构；agent 侧需要新增统一附件分析能力，并弱化 `wecom_parse_message` 作为主流程入口的职责。
- 影响验证方式：除现有下载契约回归外，还需要验证“需要附件内容时一定触发分析工具、普通闲聊时不会无谓分析、语音仍不暴露附件状态”。
